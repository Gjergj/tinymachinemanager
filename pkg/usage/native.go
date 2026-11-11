package usage

import (
	"errors"
	"fmt"
	"time"

	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"libvirt.org/go/libvirt"
)

// Collect reads current counters on a read-only connection and derives runtime
// usage. Optional per-guest failures are represented in that guest's states.
// Native calls are synchronous and cannot be cancelled through Go contexts.
func (c *Collector) Collect(uri string, guests []domains.Domain) (result []GuestUsage) {
	active := false
	for _, guest := range guests {
		active = active || guest.Active
	}
	if !active {
		return c.collect(guests, nil)
	}
	if uri == "" {
		uri = domains.DefaultURI
	}
	failed := func(err error) []GuestUsage {
		return c.collect(guests, func(domains.Domain) sample { return sample{Err: err} })
	}
	conn, err := libvirt.NewConnectReadOnly(uri)
	if err != nil {
		return failed(fmt.Errorf("connect to usage source %q read-only: %w", uri, err))
	}
	defer func() {
		remaining, closeErr := conn.Close()
		if closeErr != nil {
			result = failed(fmt.Errorf("close usage connection: %w", closeErr))
		} else if remaining != 0 {
			result = failed(fmt.Errorf("close usage connection: %d references remain", remaining))
		}
	}()
	return c.collect(guests, func(guest domains.Domain) sample { return readNativeSample(conn, guest.UUID) })
}

func readNativeSample(conn *libvirt.Connect, uuid string) (out sample) {
	dom, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return sample{Err: fmt.Errorf("find domain %q for usage: %w", uuid, err)}
	}
	defer func() {
		if err := dom.Free(); err != nil {
			out.Err = errors.Join(out.Err, fmt.Errorf("release usage domain %q: %w", uuid, err))
		}
	}()
	active, err := dom.IsActive()
	if err != nil {
		return sample{Err: fmt.Errorf("read usage domain activity: %w", err)}
	}
	if !active {
		return sample{Inactive: true}
	}
	id, err := dom.GetID()
	if err != nil {
		return sample{Err: fmt.Errorf("read usage domain runtime ID: %w", err)}
	}
	info, err := dom.GetInfo()
	if err != nil {
		return sample{Err: fmt.Errorf("read domain CPU counter: %w", err)}
	}
	out.ID, out.CPUTime, out.VCPUs, out.At = id, info.CpuTime, info.NrVirtCpu, time.Now()
	out.Memory, out.MemoryError = dom.MemoryStats(uint32(libvirt.DOMAIN_MEMORY_STAT_NR), 0)
	out.MemoryAt = time.Now()
	return out
}
