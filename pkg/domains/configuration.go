package domains

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// OS follows <os>: Firmware is its selection attribute, while FirmwareInfo
// describes its optional <firmware> child. Unspecified values stay unspecified.
type OS struct {
	Type         OSType    `json:"type" xml:"type"`
	Firmware     string    `json:"firmware,omitempty" xml:"firmware,attr"`
	FirmwareInfo *Firmware `json:"firmwareInfo,omitempty" xml:"firmware"`
	Loader       *Loader   `json:"loader,omitempty" xml:"loader"`
	NVRAM        *NVRAM    `json:"nvram,omitempty" xml:"nvram"`
}

// OSType contains the text and attributes of <os><type>, not the hypervisor.
type OSType struct {
	Value   string `json:"value,omitempty" xml:",chardata"`
	Arch    string `json:"arch,omitempty" xml:"arch,attr"`
	Machine string `json:"machine,omitempty" xml:"machine,attr"`
}

type Firmware struct {
	Features []FirmwareFeature `json:"features" xml:"feature"`
}

// Enabled preserves yes/no, unspecified and future libvirt values.
type FirmwareFeature struct {
	Name    string `json:"name" xml:"name,attr"`
	Enabled string `json:"enabled,omitempty" xml:"enabled,attr"`
}

type Loader struct {
	Path     string `json:"path,omitempty" xml:",chardata"`
	Type     string `json:"type,omitempty" xml:"type,attr"`
	ReadOnly string `json:"readOnly,omitempty" xml:"readonly,attr"`
	Secure   string `json:"secure,omitempty" xml:"secure,attr"`
	Format   string `json:"format,omitempty" xml:"format,attr"`
}

type NVRAM struct {
	Path           string `json:"path,omitempty" xml:",chardata"`
	Template       string `json:"template,omitempty" xml:"template,attr"`
	TemplateFormat string `json:"templateFormat,omitempty" xml:"templateFormat,attr"`
	Format         string `json:"format,omitempty" xml:"format,attr"`
}

// Devices groups the supported <devices> entries; it is not an exhaustive XML
// model. Interfaces remain in device order and serialize as [] when absent.
type Devices struct {
	Emulator   string            `json:"emulator,omitempty"`
	Interfaces []Interface       `json:"interfaces"`
	MemBalloon *MemBalloon       `json:"memballoon,omitempty"`
	Serials    []CharacterDevice `json:"serials"`
	Consoles   []CharacterDevice `json:"consoles"`
}

// MemBalloon preserves the configured balloon model and optional statistics
// collection interval. Reading it does not enable or change guest reporting.
type MemBalloon struct {
	Model string        `json:"model" xml:"model,attr"`
	Stats *BalloonStats `json:"stats,omitempty" xml:"stats"`
}

type BalloonStats struct {
	Period uint `json:"period" xml:"period,attr"`
}

// parseDomainXML reads the configuration projection in one pass. Runtime state
// and normalized resource quantities are populated separately from native APIs.
func parseDomainXML(data string) (Domain, error) {
	return ParseConfigurationXML(data)
}

// ParseConfigurationXML projects non-secret device/OS configuration. Runtime
// state and identity must be read separately; this does not open a connection.
func ParseConfigurationXML(data string) (Domain, error) {
	var raw struct {
		XMLName xml.Name `xml:"domain"`
		Type    string   `xml:"type,attr"`
		OS      OS       `xml:"os"`
		Devices struct {
			Emulator   string            `xml:"emulator"`
			Interfaces []interfaceXML    `xml:"interface"`
			MemBalloon *MemBalloon       `xml:"memballoon"`
			Serials    []CharacterDevice `xml:"serial"`
			Consoles   []CharacterDevice `xml:"console"`
		} `xml:"devices"`
	}
	if err := xml.Unmarshal([]byte(data), &raw); err != nil {
		return Domain{}, fmt.Errorf("parse libvirt domain XML: %w", err)
	}
	raw.OS.Type.Value = strings.TrimSpace(raw.OS.Type.Value)
	if raw.OS.Loader != nil {
		raw.OS.Loader.Path = strings.TrimSpace(raw.OS.Loader.Path)
	}
	if raw.OS.NVRAM != nil {
		raw.OS.NVRAM.Path = strings.TrimSpace(raw.OS.NVRAM.Path)
	}
	if raw.OS.FirmwareInfo != nil {
		raw.OS.FirmwareInfo.Features = append([]FirmwareFeature{}, raw.OS.FirmwareInfo.Features...)
	}
	return Domain{Type: raw.Type, OS: raw.OS, Devices: Devices{
		Emulator: strings.TrimSpace(raw.Devices.Emulator), Interfaces: interfacesFromXML(raw.Devices.Interfaces), MemBalloon: raw.Devices.MemBalloon,
		Serials:  append([]CharacterDevice{}, raw.Devices.Serials...),
		Consoles: append([]CharacterDevice{}, raw.Devices.Consoles...),
	}}, nil
}
