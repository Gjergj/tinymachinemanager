package networks

import (
	"encoding/xml"
	"fmt"
)

// VLAN preserves tags and trunk/native-tag settings. A nil *VLAN means no
// explicit setting at that level; libvirt may inherit one from a network/portgroup.
type VLAN struct {
	Trunk bool      `json:"trunk"`
	Tags  []VLANTag `json:"tags"`
}

type VLANTag struct {
	ID         uint   `json:"id" xml:"id,attr"`
	NativeMode string `json:"nativeMode,omitempty" xml:"nativeMode,attr"`
}

func (v *VLAN) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Trunk string    `xml:"trunk,attr"`
		Tags  []VLANTag `xml:"tag"`
	}
	if err := decoder.DecodeElement(&raw, &start); err != nil {
		return err
	}
	v.Trunk, v.Tags = raw.Trunk == "yes", append([]VLANTag{}, raw.Tags...)
	return nil
}

type PortGroup struct {
	Name            string `json:"name"`
	Default         bool   `json:"default"`
	VLAN            *VLAN  `json:"vlan,omitempty"`
	VirtualPortType string `json:"virtualPortType,omitempty"`
}

// IPConfig describes the virtual network's address/DHCP configuration, not a
// discovered guest address. Prefix is a pointer to distinguish /0 from absent.
type IPConfig struct {
	Family  string `json:"family" xml:"family,attr"`
	Address string `json:"address" xml:"address,attr"`
	Netmask string `json:"netmask,omitempty" xml:"netmask,attr"`
	Prefix  *uint  `json:"prefix,omitempty" xml:"prefix,attr"`
	DHCP    *DHCP  `json:"dhcp,omitempty" xml:"dhcp"`
}

type DHCP struct {
	Ranges []DHCPRange `json:"ranges" xml:"range"`
	Hosts  []DHCPHost  `json:"hosts" xml:"host"`
}

type DHCPRange struct {
	Start string `json:"start" xml:"start,attr"`
	End   string `json:"end" xml:"end,attr"`
}

type DHCPHost struct {
	MAC  string `json:"mac,omitempty" xml:"mac,attr"`
	ID   string `json:"id,omitempty" xml:"id,attr"`
	Name string `json:"name,omitempty" xml:"name,attr"`
	IP   string `json:"ip" xml:"ip,attr"`
}

// Configuration contains topology/configuration from network XML. PortIsolated
// preserves "", "yes" and "no" rather than collapsing unspecified settings.
type Configuration struct {
	Bridge            string      `json:"bridge,omitempty"`
	ForwardMode       string      `json:"forwardMode"`
	ForwardDevice     string      `json:"forwardDevice,omitempty"`
	ForwardInterfaces []string    `json:"forwardInterfaces"`
	VirtualPortType   string      `json:"virtualPortType,omitempty"`
	PortIsolated      string      `json:"portIsolated,omitempty"`
	VLAN              *VLAN       `json:"vlan,omitempty"`
	PortGroups        []PortGroup `json:"portGroups"`
	IPs               []IPConfig  `json:"ips"`
	DNSDomain         string      `json:"dnsDomain,omitempty"`
}

func parseConfiguration(data string) (Configuration, error) {
	var raw struct {
		XMLName xml.Name `xml:"network"`
		Bridge  struct {
			Name string `xml:"name,attr"`
		} `xml:"bridge"`
		Forward *struct {
			Mode       string `xml:"mode,attr"`
			Device     string `xml:"dev,attr"`
			Interfaces []struct {
				Device string `xml:"dev,attr"`
			} `xml:"interface"`
		} `xml:"forward"`
		VirtualPort struct {
			Type string `xml:"type,attr"`
		} `xml:"virtualport"`
		Port struct {
			Isolated string `xml:"isolated,attr"`
		} `xml:"port"`
		VLAN       *VLAN `xml:"vlan"`
		PortGroups []struct {
			Name        string `xml:"name,attr"`
			Default     string `xml:"default,attr"`
			VLAN        *VLAN  `xml:"vlan"`
			VirtualPort struct {
				Type string `xml:"type,attr"`
			} `xml:"virtualport"`
		} `xml:"portgroup"`
		IPs    []IPConfig `xml:"ip"`
		Domain struct {
			Name string `xml:"name,attr"`
		} `xml:"domain"`
	}
	if err := xml.Unmarshal([]byte(data), &raw); err != nil {
		return Configuration{}, fmt.Errorf("parse libvirt network XML: %w", err)
	}
	config := Configuration{
		Bridge: raw.Bridge.Name, ForwardMode: "isolated", ForwardInterfaces: []string{},
		VirtualPortType: raw.VirtualPort.Type, PortIsolated: raw.Port.Isolated,
		VLAN: raw.VLAN, PortGroups: []PortGroup{}, IPs: append([]IPConfig{}, raw.IPs...), DNSDomain: raw.Domain.Name,
	}
	if raw.Forward != nil {
		config.ForwardMode, config.ForwardDevice = raw.Forward.Mode, raw.Forward.Device
		if config.ForwardMode == "" {
			config.ForwardMode = "nat"
		}
		for _, iface := range raw.Forward.Interfaces {
			config.ForwardInterfaces = append(config.ForwardInterfaces, iface.Device)
		}
	}
	for _, group := range raw.PortGroups {
		config.PortGroups = append(config.PortGroups, PortGroup{Name: group.Name, Default: group.Default == "yes", VLAN: group.VLAN, VirtualPortType: group.VirtualPort.Type})
	}
	for i := range config.IPs {
		ip := &config.IPs[i]
		if ip.Family == "" {
			ip.Family = "ipv4"
		}
		if ip.DHCP != nil {
			ip.DHCP.Ranges = append([]DHCPRange{}, ip.DHCP.Ranges...)
			ip.DHCP.Hosts = append([]DHCPHost{}, ip.DHCP.Hosts...)
		}
	}
	return config, nil
}
