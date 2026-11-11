package domains

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseDomainPlatformAndNestedDevices(t *testing.T) {
	guest, err := parseDomainXML(`<domain type="kvm"><os firmware="efi">
  <type arch="x86_64" machine="pc-q35-10.2">hvm</type>
  <firmware><feature enabled="yes" name="enrolled-keys"/><feature enabled="yes" name="secure-boot"/></firmware>
  <loader readonly="yes" secure="yes" type="pflash" format="raw">/usr/share/OVMF/OVMF_CODE_4M.ms.fd</loader>
  <nvram template="/usr/share/OVMF/OVMF_VARS_4M.ms.fd" templateFormat="raw" format="raw">/var/lib/libvirt/qemu/nvram/balboa_VARS.fd</nvram>
</os><devices><emulator>/usr/bin/qemu-system-x86_64</emulator>
  <interface type="network"><source network="default"/><model type="virtio"/></interface>
</devices></domain>`)
	if err != nil {
		t.Fatal(err)
	}
	if guest.Type != "kvm" || guest.OS.Type.Value != "hvm" || guest.OS.Type.Arch != "x86_64" || guest.OS.Type.Machine != "pc-q35-10.2" || guest.OS.Firmware != "efi" {
		t.Fatalf("platform metadata lost: %+v", guest)
	}
	if guest.OS.FirmwareInfo == nil || len(guest.OS.FirmwareInfo.Features) != 2 || guest.OS.FirmwareInfo.Features[0].Name != "enrolled-keys" || guest.OS.FirmwareInfo.Features[1].Enabled != "yes" {
		t.Fatalf("firmware features lost: %+v", guest.OS.FirmwareInfo)
	}
	loader := guest.OS.Loader
	if loader == nil || loader.Path != "/usr/share/OVMF/OVMF_CODE_4M.ms.fd" || loader.Type != "pflash" || loader.ReadOnly != "yes" || loader.Secure != "yes" || loader.Format != "raw" {
		t.Fatalf("loader metadata lost: %+v", loader)
	}
	nvram := guest.OS.NVRAM
	if nvram == nil || nvram.Path != "/var/lib/libvirt/qemu/nvram/balboa_VARS.fd" || nvram.Template != "/usr/share/OVMF/OVMF_VARS_4M.ms.fd" || nvram.TemplateFormat != "raw" || nvram.Format != "raw" {
		t.Fatalf("NVRAM metadata lost: %+v", nvram)
	}
	if guest.Devices.Emulator != "/usr/bin/qemu-system-x86_64" || len(guest.Devices.Interfaces) != 1 || guest.Devices.Interfaces[0].Source.Network != "default" {
		t.Fatalf("device metadata lost: %+v", guest.Devices)
	}
	data, err := json.Marshal(guest)
	if err != nil || !strings.Contains(string(data), `"devices":{"emulator":"/usr/bin/qemu-system-x86_64","interfaces":[`) {
		t.Fatalf("device structure was flattened: %s err=%v", data, err)
	}
}

func TestDomainPlatformOptionalAndUnknownValues(t *testing.T) {
	for _, input := range []string{`<domain/>`, `<domain><os><type>hvm</type></os><devices/></domain>`} {
		guest, err := parseDomainXML(input)
		if err != nil {
			t.Fatal(err)
		}
		if guest.Type != "" || guest.OS.Firmware != "" || guest.OS.Loader != nil || guest.OS.NVRAM != nil || guest.OS.FirmwareInfo != nil || guest.Devices.Interfaces == nil || guest.Devices.Emulator != "" {
			t.Fatalf("invented optional data: %+v", guest)
		}
	}
	guest, err := parseDomainXML(`<domain type="future-hv"><os firmware="future-fw"><type arch="future-arch">future-os</type>
  <firmware><feature name="future-feature" enabled="future-value"/></firmware><loader type="rom" secure="no"> /firmware.fd </loader>
</os><devices><emulator> /custom/emulator </emulator><future-device/></devices></domain>`)
	if err != nil || guest.Type != "future-hv" || guest.OS.Firmware != "future-fw" || guest.OS.Type.Arch != "future-arch" || guest.OS.Type.Value != "future-os" || guest.OS.FirmwareInfo.Features[0].Enabled != "future-value" || guest.OS.Loader.Path != "/firmware.fd" || guest.OS.Loader.Secure != "no" || guest.OS.Loader.ReadOnly != "" || guest.Devices.Emulator != "/custom/emulator" {
		t.Fatalf("unknown values or text handling lost: %+v err=%v", guest, err)
	}
}

func TestNativeDomainPlatformForStoppedAndRunningGuests(t *testing.T) {
	conn := emptyTestConnection(t)
	dom, err := conn.DomainDefineXML(`<domain type="test"><name>platform</name><memory unit="KiB">262144</memory>
  <os><type arch="i686" machine="pc">hvm</type></os><devices><emulator>/usr/bin/test-hv</emulator></devices></domain>`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := dom.Free(); err != nil {
			t.Error(err)
		}
	}()
	for _, active := range []bool{false, true} {
		if active {
			if err := dom.Create(); err != nil {
				t.Fatal(err)
			}
		}
		guest, err := describe(dom)
		if err != nil || guest.Type != "test" || guest.OS.Type.Arch != "i686" || guest.OS.Type.Machine != "pc" || guest.OS.Type.Value != "hvm" || guest.Devices.Emulator != "/usr/bin/test-hv" || guest.Active != active || guest.Devices.Interfaces == nil {
			t.Fatalf("native platform metadata lost: %+v err=%v", guest, err)
		}
	}
}

func TestParseMemoryBalloonStatisticsConfiguration(t *testing.T) {
	guest, err := parseDomainXML(`<domain><devices><memballoon model="virtio"><stats period="5"/></memballoon></devices></domain>`)
	if err != nil || guest.Devices.MemBalloon == nil || guest.Devices.MemBalloon.Model != "virtio" || guest.Devices.MemBalloon.Stats == nil || guest.Devices.MemBalloon.Stats.Period != 5 {
		t.Fatalf("balloon statistics configuration lost: %+v err=%v", guest.Devices, err)
	}
	guest, err = parseDomainXML(`<domain><devices><memballoon model="future"/></devices></domain>`)
	if err != nil || guest.Devices.MemBalloon == nil || guest.Devices.MemBalloon.Model != "future" || guest.Devices.MemBalloon.Stats != nil {
		t.Fatalf("unspecified balloon statistics were invented: %+v err=%v", guest.Devices, err)
	}
}
