// Package domains provides read-only enumeration of guests on a libvirt connection.
package domains

import (
	"errors"
	"fmt"
	"sort"

	"libvirt.org/go/libvirt"
)

// DefaultURI selects the system-wide QEMU/KVM inventory.
const DefaultURI = "qemu:///system"

// State is a domain's lifecycle state. Unknown also covers future libvirt states.
type State string

const (
	StateUnknown     State = "unknown"
	StateRunning     State = "running"
	StateBlocked     State = "blocked"
	StatePaused      State = "paused"
	StateShutdown    State = "shutdown"
	StateShutoff     State = "shutoff"
	StateCrashed     State = "crashed"
	StatePMSuspended State = "pmsuspended"
)

// Domain is a snapshot of a guest
// Memory fields are in KiB and describe libvirt's allocation/configuration,
// no the memory consumption inside the guest.
type Domain struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	State        State  `json:"state"`
	Active       bool   `json:"active"`
	Persistent   bool   `json:"persistent"`
	VCPUs        uint   `json:"vcpus"`
	MemoryKiB    uint64 `json:"memoryKiB"`
	MaxMemoryKiB uint64 `json:"maxMemoryKiB"`
}

// List enumerates active and inactive domains visible through uri using a
// read-only connection. An empty uri selects DefaultURI. The QEMU driver returns
// both KVM and emulated QEMU guests. Results are sorted by name, then UUID.
//
// All native resources are released before returning.
// Underlying libvirt errors are preserved errors.As.
func List(uri string) (guests []Domain, retErr error) {
	if uri == "" {
		uri = DefaultURI
	}
	conn, err := libvirt.NewConnectReadOnly(uri)
	if err != nil {
		return nil, fmt.Errorf("connect to libvirt %q read-only: %w", uri, err)
	}
	defer func() {
		remaining, err := conn.Close()
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %w", uri, err))
		} else if remaining != 0 {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %d references remain", uri, remaining))
		}
		if retErr != nil {
			guests = nil
		}
	}()
	return list(conn)
}

func list(conn *libvirt.Connect) (guests []Domain, retErr error) {
	// Zero flags includes active/inactive and persistent/transient domains.
	handles, err := conn.ListAllDomains(0)
	if err != nil {
		return nil, fmt.Errorf("list libvirt domains: %w", err)
	}
	// Register cleanup for the entire returned set before reading any metadata,
	// so an early error also frees domains that have not yet been inspected.
	defer func() {
		for i := range handles {
			if err := handles[i].Free(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("free libvirt domain reference %d: %w", i, err))
			}
		}
		if retErr != nil {
			guests = nil
		}
	}()
	guests = make([]Domain, 0, len(handles))
	for i := range handles {
		guest, err := describe(&handles[i])
		if err != nil {
			return nil, err
		}
		guests = append(guests, guest)
	}
	sort.Slice(guests, func(i, j int) bool {
		if guests[i].Name == guests[j].Name {
			return guests[i].UUID < guests[j].UUID
		}
		return guests[i].Name < guests[j].Name
	})
	return guests, nil
}

func describe(dom *libvirt.Domain) (Domain, error) {
	name, err := dom.GetName()
	if err != nil {
		return Domain{}, fmt.Errorf("get libvirt domain name: %w", err)
	}
	uuid, err := dom.GetUUIDString()
	if err != nil {
		return Domain{}, fmt.Errorf("get UUID for domain %q: %w", name, err)
	}
	info, err := dom.GetInfo()
	if err != nil {
		return Domain{}, fmt.Errorf("get info for domain %q (%s): %w", name, uuid, err)
	}
	active, err := dom.IsActive()
	if err != nil {
		return Domain{}, fmt.Errorf("get activity for domain %q (%s): %w", name, uuid, err)
	}
	persistent, err := dom.IsPersistent()
	if err != nil {
		return Domain{}, fmt.Errorf("get persistence for domain %q (%s): %w", name, uuid, err)
	}
	return Domain{
		UUID: uuid, Name: name, State: stateFromLibvirt(info.State),
		Active: active, Persistent: persistent, VCPUs: info.NrVirtCpu,
		MemoryKiB: info.Memory, MaxMemoryKiB: info.MaxMem,
	}, nil
}

func stateFromLibvirt(state libvirt.DomainState) State {
	switch state {
	case libvirt.DOMAIN_RUNNING:
		return StateRunning
	case libvirt.DOMAIN_BLOCKED:
		return StateBlocked
	case libvirt.DOMAIN_PAUSED:
		return StatePaused
	case libvirt.DOMAIN_SHUTDOWN:
		return StateShutdown
	case libvirt.DOMAIN_SHUTOFF:
		return StateShutoff
	case libvirt.DOMAIN_CRASHED:
		return StateCrashed
	case libvirt.DOMAIN_PMSUSPENDED:
		return StatePMSuspended
	default:
		return StateUnknown
	}
}
