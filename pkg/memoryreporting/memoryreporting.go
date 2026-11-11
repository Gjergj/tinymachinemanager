// Package memoryreporting explicitly enables per-guest balloon statistics.
package memoryreporting

import (
	"encoding/xml"
	"errors"
	"fmt"

	"libvirt.org/go/libvirt"
)

// PeriodSeconds is the per-guest balloon statistics collection interval.
const PeriodSeconds = 5

const defaultURI = "qemu:///system"

type memoryDomain interface {
	GetXMLDesc(libvirt.DomainXMLFlags) (string, error)
	IsActive() (bool, error)
	IsPersistent() (bool, error)
	SetMemoryStatsPeriod(int, libvirt.DomainMemoryModFlags) error
	Free() error
}

type memoryConnection interface {
	LookupDomainByUUIDString(string) (memoryDomain, error)
	Close() (int, error)
}

type nativeConnection struct {
	*libvirt.Connect
}

func (c nativeConnection) LookupDomainByUUIDString(uuid string) (memoryDomain, error) {
	return c.Connect.LookupDomainByUUIDString(uuid)
}

// Enable explicitly opts a single guest into five-second balloon statistics
// collection. An empty URI selects qemu:///system. Active guests are updated
// live; persistent guests also retain the setting for subsequent starts.
//
// This is a write operation and must not be called as part of enumeration.
// It requires a supported virtio balloon and never changes guest memory size.
// Native errors remain available through errors.As, including cleanup errors.
func Enable(uri, uuid string) (retErr error) {
	return enable(uri, uuid, func(uri string) (memoryConnection, error) {
		conn, err := libvirt.NewConnect(uri)
		if err != nil {
			return nil, err
		}
		return nativeConnection{conn}, nil
	})
}

func enable(uri, uuid string, open func(string) (memoryConnection, error)) (retErr error) {
	if uri == "" {
		uri = defaultURI
	}
	conn, err := open(uri)
	if err != nil {
		return fmt.Errorf("connect to libvirt %q for memory reporting: %w", uri, err)
	}
	defer func() {
		remaining, err := conn.Close()
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %w", uri, err))
		} else if remaining != 0 {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %d references remain", uri, remaining))
		}
	}()
	dom, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return fmt.Errorf("look up libvirt domain %q for memory reporting: %w", uuid, err)
	}
	// Registered after connection cleanup so the domain is always freed first.
	defer func() {
		if err := dom.Free(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("free libvirt domain %q: %w", uuid, err))
		}
	}()
	// Zero flags reads nonsecure live XML for active guests, otherwise their
	// saved definition. No secrets or guest-agent access are required.
	data, err := dom.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("get XML for libvirt domain %q: %w", uuid, err)
	}
	var definition struct {
		XMLName xml.Name `xml:"domain"`
		Devices struct {
			MemBalloon *struct {
				Model string `xml:"model,attr"`
			} `xml:"memballoon"`
		} `xml:"devices"`
	}
	if err := xml.Unmarshal([]byte(data), &definition); err != nil {
		return fmt.Errorf("parse XML for libvirt domain %q: %w", uuid, err)
	}
	balloon := definition.Devices.MemBalloon
	if balloon == nil {
		return fmt.Errorf("libvirt domain %q has no memory balloon device", uuid)
	}
	switch balloon.Model {
	case "virtio", "virtio-transitional", "virtio-non-transitional":
	default:
		return fmt.Errorf("libvirt domain %q has unsupported memory balloon model %q", uuid, balloon.Model)
	}
	active, err := dom.IsActive()
	if err != nil {
		return fmt.Errorf("get activity for libvirt domain %q: %w", uuid, err)
	}
	persistent, err := dom.IsPersistent()
	if err != nil {
		return fmt.Errorf("get persistence for libvirt domain %q: %w", uuid, err)
	}
	var flags libvirt.DomainMemoryModFlags
	if active {
		flags |= libvirt.DOMAIN_MEM_LIVE
	}
	if persistent {
		flags |= libvirt.DOMAIN_MEM_CONFIG
	}
	if flags == 0 {
		return fmt.Errorf("cannot enable memory reporting for inactive transient libvirt domain %q", uuid)
	}
	if err := dom.SetMemoryStatsPeriod(PeriodSeconds, flags); err != nil {
		return fmt.Errorf("enable %d-second memory reporting for libvirt domain %q: %w", PeriodSeconds, uuid, err)
	}
	return nil
}
