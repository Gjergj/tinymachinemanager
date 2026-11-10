package domains

import (
	"errors"
	"strings"
	"testing"

	"libvirt.org/go/libvirt"
)

func TestDHCPQuerySkipsStoppedAndIneligibleInterfaces(t *testing.T) {
	eligible := Interface{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "default"}}, MAC: "52:54:00:00:00:01"}
	for _, fixture := range []struct {
		active bool
		nics   []Interface
	}{
		{false, []Interface{eligible}}, {true, nil},
		{true, []Interface{{Attachment: Attachment{Type: "bridge", Source: InterfaceSource{Bridge: "br0"}}, MAC: eligible.MAC}}},
		{true, []Interface{{Attachment: eligible.Attachment}}},
		{true, []Interface{{Attachment: eligible.Attachment, MAC: "invalid"}}},
	} {
		called := false
		got := readDHCPAddresses(fixture.active, fixture.nics, func() ([]libvirt.DomainInterface, error) {
			called = true
			return nil, errors.New("must not query")
		})
		if called || got != nil {
			t.Fatalf("queried an ineligible domain: %+v", fixture)
		}
	}
}

func TestDHCPAddressesMatchMACAndTargetWithMultipleFamilies(t *testing.T) {
	nics := []Interface{
		{Attachment: Attachment{Type: "bridge", Source: InterfaceSource{Bridge: "external"}}},
		{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "default"}}, MAC: "52:54:00:AA:BB:01", Target: "vnet1"},
		{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "lab"}}, MAC: "52:54:00:aa:bb:02", Target: "vnet2"},
	}
	queries := 0
	got := readDHCPAddresses(true, nics, func() ([]libvirt.DomainInterface, error) {
		queries++
		return []libvirt.DomainInterface{
			{Name: "vnet2", Hwaddr: "52:54:00:aa:bb:02", Addrs: []libvirt.DomainIPAddress{{Type: libvirt.IP_ADDR_TYPE_IPV4, Addr: "10.0.0.8", Prefix: 24}}},
			{Name: "vnet1", Hwaddr: "52:54:00:aa:bb:01", Addrs: []libvirt.DomainIPAddress{
				{Type: libvirt.IP_ADDR_TYPE_IPV4, Addr: "192.168.122.41", Prefix: 24},
				{Type: libvirt.IP_ADDR_TYPE_IPV6, Addr: "2001:db8::41", Prefix: 64},
			}},
		}, nil
	})
	if queries != 1 || got == nil || got.Error != "" || len(got.Interfaces) != 2 {
		t.Fatalf("incorrect DHCP query result: %+v queries=%d", got, queries)
	}
	first, second := got.Interfaces[0], got.Interfaces[1]
	if first.Index != 1 || first.MAC != "52:54:00:aa:bb:01" || first.Name != "vnet1" || len(first.Addresses) != 2 || first.Addresses[0].Address != "192.168.122.41" || first.Addresses[0].Family != "ipv4" || first.Addresses[0].Prefix != 24 || first.Addresses[1].Family != "ipv6" || first.Addresses[1].Prefix != 64 || second.Index != 2 || len(second.Addresses) != 1 || second.Addresses[0].Address != "10.0.0.8" {
		t.Fatalf("incorrect NIC address association: %+v", got)
	}
}

func TestDHCPAddressesAvoidDuplicateMACGuesses(t *testing.T) {
	mac := "52:54:00:00:00:01"
	nics := []Interface{
		{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "default"}}, MAC: mac, Target: "vnet1"},
		{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "lab"}}, MAC: mac, Alias: "net2"},
	}
	observations := []libvirt.DomainInterface{
		{Name: "vnet1", Hwaddr: mac, Addrs: []libvirt.DomainIPAddress{{Type: libvirt.IP_ADDR_TYPE_IPV4, Addr: "192.168.122.41", Prefix: 24}}},
		{Name: "net2", Hwaddr: mac, Addrs: []libvirt.DomainIPAddress{{Type: libvirt.IP_ADDR_TYPE_IPV4, Addr: "10.0.0.41", Prefix: 24}}},
	}
	query := func() ([]libvirt.DomainInterface, error) { return observations, nil }
	got := readDHCPAddresses(true, nics, query)
	if got.Interfaces[0].Addresses[0].Address != "192.168.122.41" || got.Interfaces[1].Addresses[0].Address != "10.0.0.41" {
		t.Fatalf("named observations crossed NICs: %+v", got)
	}
	nics[0].Target, nics[1].Alias = "", ""
	got = readDHCPAddresses(true, nics, query)
	for _, nic := range got.Interfaces {
		if len(nic.Addresses) != 0 || nic.Error == "" {
			t.Fatalf("ambiguous MAC was guessed: %+v", nic)
		}
	}
	// Even one returned lease cannot be assigned to both duplicate-MAC NICs.
	got = readDHCPAddresses(true, nics, func() ([]libvirt.DomainInterface, error) { return observations[:1], nil })
	for _, nic := range got.Interfaces {
		if len(nic.Addresses) != 0 || nic.Error == "" {
			t.Fatalf("single observation was guessed across networks: %+v", nic)
		}
	}
}

func TestDHCPAddressesDistinguishEmptyAndFailedLookup(t *testing.T) {
	nics := []Interface{{Attachment: Attachment{Type: "network", Source: InterfaceSource{Network: "default"}}, MAC: "52:54:00:00:00:01"}}
	got := readDHCPAddresses(true, nics, func() ([]libvirt.DomainInterface, error) { return nil, nil })
	if got == nil || got.Error != "" || len(got.Interfaces) != 1 || got.Interfaces[0].Addresses == nil || len(got.Interfaces[0].Addresses) != 0 || got.Interfaces[0].Error != "" {
		t.Fatalf("empty lease inventory not represented: %+v", got)
	}
	got = readDHCPAddresses(true, nics, func() ([]libvirt.DomainInterface, error) { return nil, errors.New("lease permission denied") })
	if got == nil || !strings.Contains(got.Error, "lease permission denied") || len(got.Interfaces[0].Addresses) != 0 {
		t.Fatalf("optional lease error lost: %+v", got)
	}
}
