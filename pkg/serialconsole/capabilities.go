// Package serialconsole provides local QEMU/KVM serial-console discovery and
// byte streams. It has no dependency on Wails or a frontend terminal renderer.
package serialconsole

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"libvirt.org/go/libvirt"
)

type Device struct {
	Key       string                  `json:"key"`
	Label     string                  `json:"label"`
	Kind      string                  `json:"kind"`
	Backend   string                  `json:"backend"`
	Alias     string                  `json:"alias"`
	Target    domains.CharacterTarget `json:"target"`
	Supported bool                    `json:"supported"`
	Reason    string                  `json:"reason"`
}

type Capabilities struct {
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	Active    bool     `json:"active"`
	State     string   `json:"state"`
	Available bool     `json:"available"`
	Code      string   `json:"code"`
	Reason    string   `json:"reason"`
	Revision  string   `json:"revision"`
	Devices   []Device `json:"devices"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func capabilities(uuid, name string, active bool, state string, runtimeID uint, d domains.Domain) Capabilities {
	c := Capabilities{UUID: uuid, Name: name, Active: active, State: state, Devices: []Device{}}
	add := func(kind string, index int, dev domains.CharacterDevice) {
		label := fmt.Sprintf("%s %d", kind, index+1)
		if dev.Target.Port != nil {
			label = fmt.Sprintf("%s %d", kind, *dev.Target.Port+1)
		}
		endpoint := Device{Kind: kind, Label: label, Backend: dev.Type, Alias: dev.Alias, Target: dev.Target, Supported: true}
		endpoint.Key = digest(struct {
			Kind   string
			Device domains.CharacterDevice
		}{kind, dev})
		if dev.Type != "pty" {
			endpoint.Supported = false
			endpoint.Reason = fmt.Sprintf("The %q backend is not supported; a PTY is required.", dev.Type)
		}
		if active && dev.Alias == "" {
			endpoint.Supported = false
			endpoint.Reason = "Libvirt did not report a live device alias."
		}
		c.Devices = append(c.Devices, endpoint)
	}
	for i, dev := range d.Devices.Serials {
		add("Serial", i, dev)
	}
	for i, dev := range d.Devices.Consoles {
		duplicate := false
		if dev.Target.Type == "serial" {
			for _, serial := range d.Devices.Serials {
				if (dev.Alias != "" && dev.Alias == serial.Alias) || (dev.Target.Port != nil && serial.Target.Port != nil && *dev.Target.Port == *serial.Target.Port) {
					duplicate = true
					break
				}
			}
		}
		if !duplicate {
			add("Console", i, dev)
		}
	}
	for _, dev := range c.Devices {
		c.Available = c.Available || dev.Supported
	}
	switch {
	case d.Type != "kvm" && d.Type != "qemu":
		c.Code = "unsupported"
		c.Reason = "Serial consoles currently support local QEMU/KVM guests only."
		c.Available = false
	case !active:
		c.Code = "inactive"
		c.Reason = "Start the virtual machine before connecting."
		c.Available = false
	case len(c.Devices) == 0:
		c.Code = "no-device"
		c.Reason = "No serial or text console is configured."
	case !c.Available:
		c.Code = "unsupported"
		c.Reason = "No supported PTY serial console is available."
	default:
		c.Code = "available"
	}
	keys := make([]string, 0, len(c.Devices))
	for _, dev := range c.Devices {
		keys = append(keys, dev.Key)
	}
	sort.Strings(keys)
	c.Revision = digest(struct {
		UUID    string
		Runtime uint
		Active  bool
		Type    string
		Keys    []string
	}{uuid, runtimeID, active, d.Type, keys})
	return c
}

func validate(c Capabilities, key, revision string) (Device, error) {
	if !c.Available {
		return Device{}, fmt.Errorf("%s", c.Reason)
	}
	if c.Revision != revision {
		return Device{}, fmt.Errorf("Console configuration changed. Refresh the console and select the device again.")
	}
	for _, d := range c.Devices {
		if d.Key == key && d.Supported {
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("The selected serial device is no longer available.")
}

// Inspect uses a read-only connection and never claims or changes a console.
func Inspect(uri, uuid string) (Capabilities, error) {
	if uri == "" {
		uri = domains.DefaultURI
	}
	if uri != domains.DefaultURI && uri != "qemu:///session" {
		return Capabilities{}, fmt.Errorf("Only local QEMU connections are supported.")
	}
	conn, err := libvirt.NewConnectReadOnly(uri)
	if err != nil {
		return Capabilities{}, err
	}
	defer conn.Close()
	dom, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return Capabilities{}, err
	}
	defer dom.Free()
	return inspectDomain(dom)
}

type domainMetadata interface {
	GetUUIDString() (string, error)
	GetName() (string, error)
	IsActive() (bool, error)
	GetID() (uint, error)
	GetInfo() (*libvirt.DomainInfo, error)
	GetXMLDesc(libvirt.DomainXMLFlags) (string, error)
}

func inspectDomain(dom domainMetadata) (Capabilities, error) {
	uuid, err := dom.GetUUIDString()
	if err != nil {
		return Capabilities{}, err
	}
	name, err := dom.GetName()
	if err != nil {
		return Capabilities{}, err
	}
	active, err := dom.IsActive()
	if err != nil {
		return Capabilities{}, err
	}
	var id uint
	if active {
		id, err = dom.GetID()
		if err != nil {
			return Capabilities{}, err
		}
	}
	info, err := dom.GetInfo()
	if err != nil {
		return Capabilities{}, err
	}
	x, err := dom.GetXMLDesc(0)
	if err != nil {
		return Capabilities{}, err
	}
	d, err := domains.ParseConfigurationXML(x)
	if err != nil {
		return Capabilities{}, err
	}
	state := fmt.Sprint(info.State)
	if info.State == libvirt.DOMAIN_PAUSED {
		state = "paused"
	} else if active {
		state = "running"
	} else {
		state = "shutoff"
	}
	return capabilities(uuid, name, active, state, id, d), nil
}
