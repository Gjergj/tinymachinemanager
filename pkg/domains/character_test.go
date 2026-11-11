package domains

import "testing"

func TestCharacterDevicesPreserveTargetAndUnknownBackend(t *testing.T) {
	d, err := ParseConfigurationXML(`<domain type="kvm"><devices>
<serial type="pty"><alias name="serial0"/><target type="isa-serial" port="0"/></serial>
<console type="pty"><alias name="serial0"/><target type="serial" port="0"/></console>
<console type="future"><target type="future"/></console>
<channel type="unix"><target name="org.qemu.guest_agent.0"/></channel>
</devices></domain>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Devices.Serials) != 1 || len(d.Devices.Consoles) != 2 {
		t.Fatalf("lost devices: %+v", d.Devices)
	}
	s := d.Devices.Serials[0]
	if s.Type != "pty" || s.Alias != "serial0" || s.Target.Type != "isa-serial" || s.Target.Port == nil || *s.Target.Port != 0 {
		t.Fatalf("lost serial metadata: %+v", s)
	}
	if d.Devices.Consoles[1].Type != "future" || d.Devices.Consoles[1].Target.Port != nil {
		t.Fatal("unknown/unspecified values were changed")
	}
	d, err = ParseConfigurationXML(`<domain/>`)
	if err != nil || d.Devices.Serials == nil || d.Devices.Consoles == nil {
		t.Fatal("missing devices must serialize as empty arrays")
	}
}
