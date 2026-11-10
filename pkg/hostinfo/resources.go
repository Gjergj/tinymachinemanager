// Package hostinfo reads resource capacity from a libvirt host.
package hostinfo

import (
	"errors"
	"fmt"

	"libvirt.org/go/libvirt"
)

const DefaultURI = "qemu:///system"

// Resources is physical host capacity, not free memory or reserved VM capacity.
// LogicalCPUs counts online logical processors, including hardware threads.
type Resources struct {
	MemoryKiB   uint64 `json:"memoryKiB"`
	LogicalCPUs uint   `json:"logicalCPUs"`
}

// Read uses a read-only connection to the specified host. Empty uri selects
// DefaultURI, so it matches domain enumeration rather than a local OS query.
func Read(uri string) (resources Resources, retErr error) {
	if uri == "" {
		uri = DefaultURI
	}
	conn, err := libvirt.NewConnectReadOnly(uri)
	if err != nil {
		return Resources{}, fmt.Errorf("connect to libvirt host %q read-only: %w", uri, err)
	}
	defer func() {
		remaining, err := conn.Close()
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt host connection %q: %w", uri, err))
		} else if remaining != 0 {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt host connection %q: %d references remain", uri, remaining))
		}
		if retErr != nil {
			resources = Resources{}
		}
	}()
	info, err := conn.GetNodeInfo()
	if err != nil {
		return Resources{}, fmt.Errorf("read memory capacity for libvirt host %q: %w", uri, err)
	}
	_, online, err := conn.GetCPUMap(0)
	if err != nil {
		return Resources{}, fmt.Errorf("read online CPUs for libvirt host %q: %w", uri, err)
	}
	return Resources{MemoryKiB: info.Memory, LogicalCPUs: online}, nil
}
