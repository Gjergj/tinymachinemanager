package domains

import (
	"encoding/xml"
	"fmt"

	"github.com/Gjergj/tinymachinemanager/pkg/networks"
)

// InterfaceSource preserves libvirt's attachment references. For Type=network,
// Network/PortGroup refer to a network in the same libvirt URI. Bridge and Device
// can refer to host interfaces managed outside libvirt.
type InterfaceSource struct {
	Network       string         `json:"network,omitempty" xml:"network,attr"`
	PortGroup     string         `json:"portGroup,omitempty" xml:"portgroup,attr"`
	Bridge        string         `json:"bridge,omitempty" xml:"bridge,attr"`
	Device        string         `json:"device,omitempty" xml:"dev,attr"`
	Mode          string         `json:"mode,omitempty" xml:"mode,attr"`
	Type          string         `json:"type,omitempty" xml:"type,attr"`
	Address       string         `json:"address,omitempty" xml:"address,attr"`
	Port          *uint          `json:"port,omitempty" xml:"port,attr"`
	Path          string         `json:"path,omitempty" xml:"path,attr"`
	DeviceAddress *DeviceAddress `json:"deviceAddress,omitempty" xml:"address"`
}

// DeviceAddress is the device identity for a hostdev interface (usually PCI).
type DeviceAddress struct {
	Type     string `json:"type,omitempty" xml:"type,attr"`
	Domain   string `json:"domain,omitempty" xml:"domain,attr"`
	Bus      string `json:"bus,omitempty" xml:"bus,attr"`
	Slot     string `json:"slot,omitempty" xml:"slot,attr"`
	Function string `json:"function,omitempty" xml:"function,attr"`
	Device   string `json:"device,omitempty" xml:"device,attr"`
}

// Attachment records one attachment layer. A network-backed interface may also
// have an Actual layer resolved to a bridge/direct device in live domain XML.
type Attachment struct {
	Type            string          `json:"type"`
	Source          InterfaceSource `json:"source"`
	VirtualPortType string          `json:"virtualPortType,omitempty"`
	PortIsolated    string          `json:"portIsolated,omitempty"`
	VLAN            *networks.VLAN  `json:"vlan,omitempty"`
}

// Interface describes a guest NIC from domain XML.
type Interface struct {
	Attachment
	MAC       string      `json:"mac,omitempty"`
	Model     string      `json:"model,omitempty"`
	Target    string      `json:"target,omitempty"`
	LinkState string      `json:"linkState,omitempty"`
	Alias     string      `json:"alias,omitempty"`
	Filter    string      `json:"filter,omitempty"`
	Actual    *Attachment `json:"actual,omitempty"`
}

type attachmentXML struct {
	Type        string          `xml:"type,attr"`
	Source      InterfaceSource `xml:"source"`
	VirtualPort struct {
		Type string `xml:"type,attr"`
	} `xml:"virtualport"`
	Port struct {
		Isolated string `xml:"isolated,attr"`
	} `xml:"port"`
	VLAN *networks.VLAN `xml:"vlan"`
}

func (a attachmentXML) summary() Attachment {
	return Attachment{Type: a.Type, Source: a.Source, VirtualPortType: a.VirtualPort.Type, PortIsolated: a.Port.Isolated, VLAN: a.VLAN}
}

func parseInterfaces(data string) ([]Interface, error) {
	var raw struct {
		XMLName xml.Name `xml:"domain"`
		Devices struct {
			Interfaces []struct {
				attachmentXML
				MAC struct {
					Address string `xml:"address,attr"`
				} `xml:"mac"`
				Model struct {
					Type string `xml:"type,attr"`
				} `xml:"model"`
				Target struct {
					Device string `xml:"dev,attr"`
				} `xml:"target"`
				Link struct {
					State string `xml:"state,attr"`
				} `xml:"link"`
				Alias struct {
					Name string `xml:"name,attr"`
				} `xml:"alias"`
				Filter struct {
					Name string `xml:"filter,attr"`
				} `xml:"filterref"`
				Actual *attachmentXML `xml:"actual"`
			} `xml:"interface"`
		} `xml:"devices"`
	}
	if err := xml.Unmarshal([]byte(data), &raw); err != nil {
		return nil, fmt.Errorf("parse libvirt domain interfaces: %w", err)
	}
	result := make([]Interface, 0, len(raw.Devices.Interfaces))
	for _, iface := range raw.Devices.Interfaces {
		nic := Interface{
			Attachment: iface.attachmentXML.summary(), MAC: iface.MAC.Address, Model: iface.Model.Type,
			Target: iface.Target.Device, LinkState: iface.Link.State, Alias: iface.Alias.Name, Filter: iface.Filter.Name,
		}
		if iface.Actual != nil {
			actual := iface.Actual.summary()
			nic.Actual = &actual
		}
		result = append(result, nic)
	}
	return result, nil
}
