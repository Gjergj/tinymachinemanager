package domains

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Gjergj/tinymachinemanager/pkg/networks"
)

func TestParseMultipleInterfaceSources(t *testing.T) {
	interfaces, err := parseInterfaces(`<domain><devices>
  <interface type="network">
    <mac address="52:54:00:00:00:01"/><model type="virtio"/>
    <source network="office" portgroup="staff"/><target dev="vnet3"/>
    <link state="down"/><alias name="net0"/><filterref filter="clean-traffic"/>
    <vlan><tag id="10"/></vlan><port isolated="no"/>
    <actual type="bridge"><source bridge="virbr10"/>
      <virtualport type="openvswitch"/><port isolated="yes"/>
      <vlan trunk="yes"><tag id="20"/><tag id="30" nativeMode="tagged"/></vlan>
    </actual>
  </interface>
  <interface type="bridge"><source bridge="br-external"/><virtualport type="openvswitch"/></interface>
  <interface type="direct"><source dev="eno1" mode="bridge"/></interface>
  <interface type="user"/>
  <interface type="mcast"><source address="239.1.2.3" port="5555"/></interface>
  <interface type="vhostuser"><source type="unix" path="/run/guest.sock" mode="client"/></interface>
  <interface type="hostdev"><source><address type="pci" domain="0x0000" bus="0x02" slot="0x00" function="0x1"/></source></interface>
  <interface type="future-kind"><source dev="future0"/></interface>
</devices></domain>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 8 {
		t.Fatalf("interfaces dropped: %+v", interfaces)
	}
	first := interfaces[0]
	if first.Type != "network" || first.MAC != "52:54:00:00:00:01" || first.Model != "virtio" || first.Source.Network != "office" || first.Source.PortGroup != "staff" || first.Source.Bridge != "" || first.Target != "vnet3" || first.LinkState != "down" || first.Alias != "net0" || first.Filter != "clean-traffic" || first.PortIsolated != "no" {
		t.Fatalf("interface=%+v", first)
	}
	if !reflect.DeepEqual(first.VLAN, &networks.VLAN{Tags: []networks.VLANTag{{ID: 10}}}) {
		t.Fatalf("configured VLAN lost: %+v", first.VLAN)
	}
	if first.Actual == nil || first.Actual.Type != "bridge" || first.Actual.Source.Bridge != "virbr10" || first.Actual.PortIsolated != "yes" || first.Actual.VirtualPortType != "openvswitch" || !reflect.DeepEqual(first.Actual.VLAN, &networks.VLAN{Trunk: true, Tags: []networks.VLANTag{{ID: 20}, {ID: 30, NativeMode: "tagged"}}}) {
		t.Fatalf("actual attachment=%+v", first.Actual)
	}
	if interfaces[1].Source.Bridge != "br-external" || interfaces[1].VirtualPortType != "openvswitch" {
		t.Fatalf("bridge interface=%+v", interfaces[1])
	}
	if interfaces[2].Source.Device != "eno1" || interfaces[2].Source.Mode != "bridge" || interfaces[2].VLAN != nil || interfaces[2].PortIsolated != "" {
		t.Fatalf("direct interface=%+v", interfaces[2])
	}
	if interfaces[3].Type != "user" || interfaces[3].Actual != nil || interfaces[3].LinkState != "" {
		t.Fatalf("user interface=%+v", interfaces[3])
	}
	if interfaces[4].Source.Address != "239.1.2.3" || interfaces[4].Source.Port == nil || *interfaces[4].Source.Port != 5555 {
		t.Fatalf("socket source=%+v", interfaces[4].Source)
	}
	if interfaces[5].Source.Type != "unix" || interfaces[5].Source.Path != "/run/guest.sock" || interfaces[5].Source.Mode != "client" {
		t.Fatalf("vhostuser source=%+v", interfaces[5].Source)
	}
	if interfaces[6].Source.DeviceAddress == nil || interfaces[6].Source.DeviceAddress.Bus != "0x02" {
		t.Fatalf("hostdev source=%+v", interfaces[6].Source)
	}
	if interfaces[7].Type != "future-kind" || interfaces[7].Source.Device != "future0" {
		t.Fatalf("unknown interface=%+v", interfaces[7])
	}
}

func TestNoInterfacesAndInvalidDomainXML(t *testing.T) {
	for _, input := range []string{`<domain/>`, `<domain><devices><disk/></devices></domain>`} {
		interfaces, err := parseInterfaces(input)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(interfaces)
		if err != nil || string(data) != "[]" {
			t.Fatalf("interfaces=%s err=%v", data, err)
		}
	}
	for _, input := range []string{`<domain>`, `<network/>`, `<domain><devices><interface><vlan><tag id="bad"/></vlan></interface></devices></domain>`} {
		if _, err := parseInterfaces(input); err == nil {
			t.Errorf("accepted invalid XML: %s", input)
		}
	}
}

func TestNativeDomainNetworkAttachments(t *testing.T) {
	conn := emptyTestConnection(t)
	dom, err := conn.DomainDefineXML(`<domain type="test"><name>networked</name>
  <memory unit="KiB">262144</memory><os><type>hvm</type></os><devices>
    <interface type="network"><mac address="52:54:00:00:00:11"/><source network="default"/><model type="virtio"/></interface>
    <interface type="bridge"><mac address="52:54:00:00:00:12"/><source bridge="br-external"/></interface>
  </devices></domain>`)
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
		result, err := list(conn)
		if err != nil {
			t.Fatal(err)
		}
		if len(result) != 1 || len(result[0].Interfaces) != 2 {
			t.Fatalf("domain interfaces not populated: %+v", result)
		}
		if result[0].Interfaces[0].Source.Network != "default" || result[0].Interfaces[0].MAC != "52:54:00:00:00:11" || result[0].Interfaces[1].Source.Bridge != "br-external" {
			t.Fatalf("attachment info lost: %+v", result[0].Interfaces)
		}
	}
}
