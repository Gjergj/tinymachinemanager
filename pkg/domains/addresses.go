package domains

import (
	"fmt"
	"net"
	"strings"

	"libvirt.org/go/libvirt"
)

// DHCPInfo is a runtime observation from libvirt's DHCP lease address source,
// separate from OS/Devices XML. An optional lookup error never hides the guest.
type DHCPInfo struct {
	Interfaces []DHCPInterface `json:"interfaces"`
	Error      string          `json:"error,omitempty"`
}

// Index identifies the NIC's zero-based position in Devices.Interfaces. Name is
// the observed host target or alias; MAC is canonicalized for safe association.
type DHCPInterface struct {
	Index     int         `json:"index"`
	MAC       string      `json:"mac"`
	Name      string      `json:"name,omitempty"`
	Addresses []IPAddress `json:"addresses"`
	Error     string      `json:"error,omitempty"`
}

type IPAddress struct {
	Family  string `json:"family"`
	Address string `json:"address"`
	Prefix  uint   `json:"prefix"`
}

func canonicalMAC(value string) string {
	mac, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return mac.String()
}

func readDHCPAddresses(active bool, nics []Interface, query func() ([]libvirt.DomainInterface, error)) *DHCPInfo {
	if !active {
		return nil
	}
	result := &DHCPInfo{Interfaces: []DHCPInterface{}}
	macCounts := make(map[string]int)
	for index, nic := range nics {
		mac := canonicalMAC(nic.MAC)
		if nic.Type != "network" || nic.Source.Network == "" || mac == "" {
			continue
		}
		result.Interfaces = append(result.Interfaces, DHCPInterface{Index: index, MAC: mac, Addresses: []IPAddress{}})
		macCounts[mac]++
	}
	if len(result.Interfaces) == 0 {
		return nil
	}
	observations, err := query()
	if err != nil {
		result.Error = fmt.Errorf("query DHCP lease addresses: %w", err).Error()
		return result
	}
	for index := range result.Interfaces {
		entry := &result.Interfaces[index]
		nic := nics[entry.Index]
		var candidates []libvirt.DomainInterface
		for _, observation := range observations {
			if canonicalMAC(observation.Hwaddr) == entry.MAC {
				candidates = append(candidates, observation)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		// Prefer a host target, then an alias. MAC-only association is safe only
		// when both the configured NIC and returned observation are unique.
		var named []libvirt.DomainInterface
		for _, name := range []string{nic.Target, nic.Alias} {
			if name == "" {
				continue
			}
			for _, candidate := range candidates {
				if candidate.Name == name {
					named = append(named, candidate)
				}
			}
			if len(named) > 0 {
				break
			}
		}
		if len(named) == 0 && len(candidates) == 1 && macCounts[entry.MAC] == 1 {
			named = candidates
		}
		if len(named) != 1 {
			entry.Error = "DHCP lease interface could not be matched unambiguously."
			continue
		}
		entry.Name = named[0].Name
		for _, address := range named[0].Addrs {
			family := "unknown"
			switch address.Type {
			case libvirt.IP_ADDR_TYPE_IPV4:
				family = "ipv4"
			case libvirt.IP_ADDR_TYPE_IPV6:
				family = "ipv6"
			}
			entry.Addresses = append(entry.Addresses, IPAddress{Family: family, Address: address.Addr, Prefix: address.Prefix})
		}
	}
	return result
}
