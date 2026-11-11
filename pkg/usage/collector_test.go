package usage

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"libvirt.org/go/libvirt"
)

var instant = time.Unix(1800000000, 0)

func guest() domains.Domain {
	return domains.Domain{UUID: "one", Active: true, VCPUs: 4, Devices: domains.Devices{
		MemBalloon: &domains.MemBalloon{Model: "virtio", Stats: &domains.BalloonStats{Period: 5}},
	}}
}

func counter(cpu uint64, elapsed time.Duration) sample {
	return sample{ID: 1, CPUTime: cpu, VCPUs: 4, At: instant.Add(elapsed)}
}

func collect(c *Collector, g domains.Domain, s sample) GuestUsage {
	return c.collect([]domains.Domain{g}, func(domains.Domain) sample { return s })[0]
}

func TestCPUUsesActualElapsedTimeAndVCPUCapacity(t *testing.T) {
	var c Collector
	first := collect(&c, guest(), counter(1e9, 0))
	if first.CPU.State != "collecting" || first.CPU.Percent != nil {
		t.Fatalf("first sample must establish a baseline: %+v", first)
	}
	next := collect(&c, guest(), counter(11e9, 5*time.Second))
	if next.CPU.State != "ok" || next.CPU.Percent == nil || math.Abs(*next.CPU.Percent-50) > 0.00001 {
		t.Fatalf("expected 50%% of four vCPUs: %+v", next.CPU)
	}
	next = collect(&c, guest(), counter(11e9, 10*time.Second))
	if next.CPU.Percent == nil || *next.CPU.Percent != 0 {
		t.Fatalf("actual idle CPU must be zero: %+v", next.CPU)
	}
}

func TestCPUTracksGuestsIndependentlyAndDoesNotClampOverhead(t *testing.T) {
	first, second := guest(), guest()
	second.UUID = "two"
	var c Collector
	c.collect([]domains.Domain{first, second}, func(domains.Domain) sample { return counter(1e9, 0) })
	got := c.collect([]domains.Domain{first, second}, func(g domains.Domain) sample {
		if g.UUID == "one" {
			return counter(41e9, 5*time.Second)
		}
		return counter(6e9, 5*time.Second)
	})
	if got[0].CPU.Percent == nil || *got[0].CPU.Percent != 200 || got[1].CPU.Percent == nil || *got[1].CPU.Percent != 25 {
		t.Fatalf("guest baselines crossed or values were clamped: %+v", got)
	}
}

func TestCPUResetsAcrossGenerationCounterCapacityAndSamplingGaps(t *testing.T) {
	for name, next := range map[string]sample{
		"restart":  {ID: 2, CPUTime: 20e9, VCPUs: 4, At: instant.Add(5 * time.Second)},
		"rollback": counter(0, 5*time.Second),
		"capacity": {ID: 1, CPUTime: 20e9, VCPUs: 8, At: instant.Add(5 * time.Second)},
		"gap":      counter(20e9, 16*time.Second),
		"time":     counter(20e9, 0),
	} {
		t.Run(name, func(t *testing.T) {
			var c Collector
			collect(&c, guest(), counter(1e9, 0))
			got := collect(&c, guest(), next)
			if got.CPU.State != "collecting" || got.CPU.Percent != nil {
				t.Fatalf("invalid baseline was reused: %+v", got.CPU)
			}
		})
	}
}

func TestCPUDiscardsStoppedRemovedAndFailedSamples(t *testing.T) {
	for _, reason := range []string{"stopped", "removed", "failed"} {
		var c Collector
		collect(&c, guest(), counter(1e9, 0))
		switch reason {
		case "stopped":
			g := guest()
			g.Active = false
			got := c.collect([]domains.Domain{g}, func(domains.Domain) sample { t.Fatal("queried stopped guest"); return sample{} })[0]
			if got.CPU.State != "stopped" || got.Memory.State != "stopped" {
				t.Fatalf("stopped guest has live metrics: %+v", got)
			}
		case "removed":
			c.collect(nil, func(domains.Domain) sample { return sample{} })
		case "failed":
			got := collect(&c, guest(), sample{Err: errors.New("counter unavailable")})
			if got.CPU.State != "unavailable" || got.Memory.State != "unavailable" {
				t.Fatalf("source error was hidden: %+v", got)
			}
		}
		if got := collect(&c, guest(), counter(20e9, 10*time.Second)); got.CPU.Percent != nil {
			t.Fatalf("%s baseline was retained: %+v", reason, got.CPU)
		}
	}
}

func memoryStats(total, available uint64, updated time.Time) []libvirt.DomainMemoryStat {
	return []libvirt.DomainMemoryStat{
		{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_AVAILABLE), Val: total},
		{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_USABLE), Val: available},
		{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_LAST_UPDATE), Val: uint64(updated.Unix())},
		{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_RSS), Val: 999999},
	}
}

func TestMemoryUsesFreshGuestStatisticsIncludingZeroAvailable(t *testing.T) {
	for _, available := range []uint64{0, 1024} {
		s := counter(1e9, 0)
		s.Memory = memoryStats(4096, available, instant.Add(-5*time.Second))
		var c Collector
		got := collect(&c, guest(), s).Memory
		if got.State != "ok" || got.UsedKiB == nil || *got.UsedKiB != 4096-available || got.TotalKiB == nil || *got.TotalKiB != 4096 || got.RSSKiB == nil || *got.RSSKiB != 999999 {
			t.Fatalf("guest usage was confused with allocation/RSS/zero: %+v", got)
		}
	}
}

func TestMemoryUsesItsOwnObservationTime(t *testing.T) {
	s := counter(1e9, 0)
	s.Memory = memoryStats(4096, 1024, instant.Add(time.Second))
	s.MemoryAt = instant.Add(2 * time.Second)
	var c Collector
	if got := collect(&c, guest(), s).Memory; got.State != "ok" {
		t.Fatalf("fresh memory read compared with older CPU time: %+v", got)
	}
}

func TestMemoryRejectsMissingStaleFutureAndInconsistentData(t *testing.T) {
	for name, stats := range map[string][]libvirt.DomainMemoryStat{
		"missing":      {{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_RSS), Val: 1234}},
		"stale":        memoryStats(4096, 1024, instant.Add(-16*time.Second)),
		"future":       memoryStats(4096, 1024, instant.Add(time.Second)),
		"inconsistent": memoryStats(4096, 8192, instant),
		"zero total":   memoryStats(0, 0, instant),
	} {
		t.Run(name, func(t *testing.T) {
			var c Collector
			s := counter(1e9, 0)
			s.Memory = stats
			got := collect(&c, guest(), s).Memory
			if got.State == "ok" || got.UsedKiB != nil || got.TotalKiB != nil {
				t.Fatalf("invalid guest memory displayed: %+v", got)
			}
		})
	}
}

func TestMemoryHonorsConfiguredPeriodAndReportsDisabledUnsupportedAndReadErrors(t *testing.T) {
	g := guest()
	g.Devices.MemBalloon.Stats.Period = 60
	s := counter(1e9, 0)
	s.Memory = memoryStats(4096, 1024, instant.Add(-60*time.Second))
	var c Collector
	if got := collect(&c, g, s).Memory; got.State != "ok" {
		t.Fatalf("longer collection period rejected: %+v", got)
	}
	g.Devices.MemBalloon.Stats.Period = 0
	if got := collect(&c, g, s).Memory; got.State != "disabled" || got.UsedKiB != nil {
		t.Fatalf("disabled reporting: %+v", got)
	}
	g.Devices.MemBalloon = nil
	if got := collect(&c, g, s).Memory; got.State != "unsupported" {
		t.Fatalf("unsupported reporting: %+v", got)
	}
	g = guest()
	s.MemoryError = errors.New("memory denied")
	if got := collect(&c, g, s); got.Memory.State != "unavailable" || got.CPU.State == "unavailable" {
		t.Fatalf("optional memory failure hid CPU: %+v", got)
	}
}

func TestNativeCollectorUsesSelectedConnectionAndEmptyResultsAreArrays(t *testing.T) {
	guests, err := domains.List("test:///default")
	if err != nil {
		t.Fatal(err)
	}
	var c Collector
	result := c.Collect("test:///default", guests)
	if len(result) != len(guests) || result[0].UUID != guests[0].UUID || result[0].CPU.State != "collecting" {
		t.Fatalf("native collection failed: %+v", result)
	}
	if empty := c.Collect("test:///default", nil); empty == nil || len(empty) != 0 {
		t.Fatalf("empty result=%+v", empty)
	}
}
