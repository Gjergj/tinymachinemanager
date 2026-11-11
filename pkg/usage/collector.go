// Package usage samples runtime CPU and guest RAM measurements independently
// from domain XML and allocation. Collection never changes guest configuration.
package usage

import (
	"sync"
	"time"

	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"libvirt.org/go/libvirt"
)

type GuestUsage struct {
	UUID   string      `json:"uuid"`
	CPU    CPUUsage    `json:"cpu"`
	Memory MemoryUsage `json:"memory"`
}

// Percent is relative to the VM's vCPU capacity. nil means unmeasured, while
// a non-nil zero means an observed idle interval. Hypervisor overhead can exceed 100%.
type CPUUsage struct {
	State   string   `json:"state"`
	Percent *float64 `json:"percent,omitempty"`
	Message string   `json:"message,omitempty"`
}

type MemoryUsage struct {
	State      string  `json:"state"`
	UsedKiB    *uint64 `json:"usedKiB,omitempty"`
	TotalKiB   *uint64 `json:"totalKiB,omitempty"`
	RSSKiB     *uint64 `json:"rssKiB,omitempty"`
	LastUpdate int64   `json:"lastUpdate,omitempty"`
	Message    string  `json:"message,omitempty"`
}

type sample struct {
	ID          uint
	CPUTime     uint64
	VCPUs       uint
	At          time.Time
	MemoryAt    time.Time
	Memory      []libvirt.DomainMemoryStat
	MemoryError error
	Inactive    bool
	Err         error
}

// Collector keeps CPU baselines by UUID and runtime ID. Its zero value is ready
// to use. A mutex serializes samples so concurrent UI requests cannot reorder them.
type Collector struct {
	mu       sync.Mutex
	previous map[string]sample
}

func (c *Collector) collect(guests []domains.Domain, read func(domains.Domain) sample) []GuestUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.previous == nil {
		c.previous = make(map[string]sample)
	}
	result := make([]GuestUsage, 0, len(guests))
	seen := make(map[string]bool, len(guests))
	for _, guest := range guests {
		seen[guest.UUID] = true
		usage := GuestUsage{UUID: guest.UUID}
		var current sample
		if guest.Active {
			current = read(guest)
		}
		switch {
		case !guest.Active || current.Inactive:
			delete(c.previous, guest.UUID)
			usage.CPU.State, usage.Memory.State = "stopped", "stopped"
		case current.Err != nil:
			delete(c.previous, guest.UUID)
			usage.CPU = CPUUsage{State: "unavailable", Message: current.Err.Error()}
			usage.Memory = MemoryUsage{State: "unavailable", Message: current.Err.Error()}
		default:
			usage.CPU = c.cpu(guest.UUID, current)
			usage.Memory = memoryUsage(guest, current)
		}
		result = append(result, usage)
	}
	for uuid := range c.previous {
		if !seen[uuid] {
			delete(c.previous, uuid)
		}
	}
	return result
}

func (c *Collector) cpu(uuid string, current sample) CPUUsage {
	if current.VCPUs == 0 || current.At.IsZero() {
		delete(c.previous, uuid)
		return CPUUsage{State: "unavailable", Message: "CPU capacity or sample time was not reported."}
	}
	previous, exists := c.previous[uuid]
	c.previous[uuid] = sample{ID: current.ID, CPUTime: current.CPUTime, VCPUs: current.VCPUs, At: current.At}
	elapsed := current.At.Sub(previous.At)
	if !exists || current.ID != previous.ID || current.VCPUs != previous.VCPUs || current.CPUTime < previous.CPUTime || elapsed <= 0 || elapsed > 15*time.Second {
		return CPUUsage{State: "collecting", Message: "Waiting for a second CPU sample."}
	}
	percent := 100 * float64(current.CPUTime-previous.CPUTime) / float64(elapsed) / float64(current.VCPUs)
	return CPUUsage{State: "ok", Percent: &percent}
}

func memoryUsage(guest domains.Domain, current sample) MemoryUsage {
	values := make(map[libvirt.DomainMemoryStatTags]uint64)
	for _, stat := range current.Memory {
		values[libvirt.DomainMemoryStatTags(stat.Tag)] = stat.Val
	}
	result := MemoryUsage{State: "unavailable", LastUpdate: int64(values[libvirt.DOMAIN_MEMORY_STAT_LAST_UPDATE])}
	if rss, ok := values[libvirt.DOMAIN_MEMORY_STAT_RSS]; ok {
		result.RSSKiB = &rss
	}
	balloon := guest.Devices.MemBalloon
	if balloon == nil || (balloon.Model != "virtio" && balloon.Model != "virtio-transitional" && balloon.Model != "virtio-non-transitional") {
		result.State, result.Message = "unsupported", "Guest memory reporting requires a supported virtio balloon device."
		return result
	}
	if balloon.Stats == nil || balloon.Stats.Period == 0 {
		result.State, result.Message = "disabled", "Guest memory reporting is not enabled."
		return result
	}
	if current.MemoryError != nil {
		result.Message = current.MemoryError.Error()
		return result
	}
	if result.LastUpdate <= 0 {
		result.State, result.Message = "waiting", "Waiting for memory statistics from the guest."
		return result
	}
	memoryAt := current.MemoryAt
	if memoryAt.IsZero() {
		memoryAt = current.At
	}
	age := memoryAt.Sub(time.Unix(result.LastUpdate, 0)).Seconds()
	if age < 0 {
		result.Message = "Guest memory statistics have a future timestamp."
		return result
	}
	budget := max(15.0, 3*float64(balloon.Stats.Period))
	if age > budget {
		result.State, result.Message = "stale", "Waiting for a fresh memory report from the guest."
		return result
	}
	total, totalSet := values[libvirt.DOMAIN_MEMORY_STAT_AVAILABLE]
	usable, usableSet := values[libvirt.DOMAIN_MEMORY_STAT_USABLE]
	if !totalSet || !usableSet || total == 0 || usable > total {
		result.Message = "The guest did not report consistent memory usage statistics."
		return result
	}
	used := total - usable
	result.State, result.UsedKiB, result.TotalKiB = "ok", &used, &total
	return result
}
