package serialconsole

import (
	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"testing"
)

const serialXML = `<domain type="kvm"><devices><serial type="pty"><alias name="serial0"/><target type="isa-serial" port="0"/></serial><console type="pty"><alias name="serial0"/><target type="serial" port="0"/></console></devices></domain>`

func configuration(t *testing.T, data string) domains.Domain {
	t.Helper()
	d, err := domains.ParseConfigurationXML(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCapabilitiesDeduplicateAndRejectChangedRuntime(t *testing.T) {
	d := configuration(t, serialXML)
	c := capabilities("vm", "balboa", true, "running", 2, d)
	if !c.Available || len(c.Devices) != 1 || !c.Devices[0].Supported || c.Devices[0].Alias != "serial0" {
		t.Fatalf("bad capabilities: %+v", c)
	}
	if _, err := validate(c, c.Devices[0].Key, c.Revision); err != nil {
		t.Fatal(err)
	}
	restarted := capabilities("vm", "balboa", true, "running", 3, d)
	if _, err := validate(restarted, c.Devices[0].Key, c.Revision); err == nil {
		t.Fatal("stale runtime was accepted")
	}
	d.Devices.Serials[0].Target.Type = "pci-serial"
	changed := capabilities("vm", "balboa", true, "running", 2, d)
	if _, err := validate(changed, c.Devices[0].Key, c.Revision); err == nil {
		t.Fatal("changed endpoint was accepted")
	}
}

func TestCapabilitiesExplainUnavailableDevices(t *testing.T) {
	for _, tc := range []struct {
		name, xml string
		active    bool
		code      string
	}{
		{"none", `<domain type="kvm"/>`, true, "no-device"},
		{"stopped", serialXML, false, "inactive"},
		{"file", `<domain type="kvm"><devices><serial type="file"><alias name="serial0"/></serial></devices></domain>`, true, "unsupported"},
		{"no alias", `<domain type="kvm"><devices><serial type="pty"/></devices></domain>`, true, "unsupported"},
		{"wrong driver", `<domain type="lxc"><devices><console type="pty"><alias name="console0"/></console></devices></domain>`, true, "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := capabilities("vm", "guest", tc.active, "running", 1, configuration(t, tc.xml))
			if c.Available || c.Code != tc.code || c.Reason == "" {
				t.Fatalf("unexplained support state: %+v", c)
			}
		})
	}
}

func TestCapabilitiesKeepDistinctPTYConsolesAndPausedGuests(t *testing.T) {
	d := configuration(t, `<domain type="qemu"><devices><console type="pty"><alias name="console0"/><target type="virtio" port="0"/></console><console type="pty"><alias name="console1"/><target type="virtio" port="1"/></console></devices></domain>`)
	c := capabilities("vm", "guest", true, "paused", 1, d)
	if !c.Available || len(c.Devices) != 2 || c.Devices[0].Key == c.Devices[1].Key {
		t.Fatalf("distinct endpoints lost: %+v", c)
	}
	if _, err := validate(c, "made-up", c.Revision); err == nil {
		t.Fatal("unknown endpoint accepted")
	}
}
