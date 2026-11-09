package networks

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseNetworkConfiguration(t *testing.T) {
	config, err := parseConfiguration(`<network>
  <name>office</name><forward mode="nat" dev="eno1"><interface dev="eno2"/></forward>
  <bridge name="virbr10"/><virtualport type="openvswitch"/><port isolated="yes"/>
  <vlan trunk="yes"><tag id="10" nativeMode="untagged"/><tag id="20"/></vlan>
  <portgroup name="staff" default="yes"><vlan><tag id="30"/></vlan><virtualport type="openvswitch"/></portgroup>
  <ip address="192.168.10.1" netmask="255.255.255.0"><dhcp>
    <range start="192.168.10.10" end="192.168.10.100"/>
    <host mac="52:54:00:00:00:01" name="desk" ip="192.168.10.5"/>
  </dhcp></ip>
  <ip family="ipv6" address="fd00:10::1" prefix="64"><dhcp>
    <range start="fd00:10::10" end="fd00:10::100"/>
    <host id="00:01:02" name="desk6" ip="fd00:10::5"/>
  </dhcp></ip>
</network>`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Bridge != "virbr10" || config.ForwardMode != "nat" || config.ForwardDevice != "eno1" || !reflect.DeepEqual(config.ForwardInterfaces, []string{"eno2"}) || config.PortIsolated != "yes" || config.VirtualPortType != "openvswitch" {
		t.Fatalf("configuration lost topology: %+v", config)
	}
	wantVLAN := &VLAN{Trunk: true, Tags: []VLANTag{{ID: 10, NativeMode: "untagged"}, {ID: 20}}}
	if !reflect.DeepEqual(config.VLAN, wantVLAN) {
		t.Fatalf("VLAN = %+v", config.VLAN)
	}
	if len(config.PortGroups) != 1 || config.PortGroups[0].Name != "staff" || !config.PortGroups[0].Default || config.PortGroups[0].VirtualPortType != "openvswitch" || !reflect.DeepEqual(config.PortGroups[0].VLAN, &VLAN{Tags: []VLANTag{{ID: 30}}}) {
		t.Fatalf("portgroups = %+v", config.PortGroups)
	}
	if len(config.IPs) != 2 {
		t.Fatalf("IPs = %+v", config.IPs)
	}
	v4, v6 := config.IPs[0], config.IPs[1]
	if v4.Family != "ipv4" || v4.Address != "192.168.10.1" || v4.Netmask != "255.255.255.0" || v4.Prefix != nil || v4.DHCP == nil || !reflect.DeepEqual(v4.DHCP.Ranges, []DHCPRange{{Start: "192.168.10.10", End: "192.168.10.100"}}) || !reflect.DeepEqual(v4.DHCP.Hosts, []DHCPHost{{MAC: "52:54:00:00:00:01", Name: "desk", IP: "192.168.10.5"}}) {
		t.Fatalf("IPv4 = %+v", v4)
	}
	if v6.Family != "ipv6" || v6.Prefix == nil || *v6.Prefix != 64 || v6.Address != "fd00:10::1" || v6.DHCP == nil || v6.DHCP.Hosts[0].ID != "00:01:02" {
		t.Fatalf("IPv6 = %+v", v6)
	}
}

func TestNetworkDefaultsAndOptionalConfiguration(t *testing.T) {
	for _, tt := range []struct{ xml, mode string }{
		{`<network/>`, "isolated"},
		{`<network><forward/></network>`, "nat"},
		{`<network><forward mode="bridge"/><bridge name="br0"/></network>`, "bridge"},
		{`<network><forward mode="future-mode"/></network>`, "future-mode"},
	} {
		config, err := parseConfiguration(tt.xml)
		if err != nil || config.ForwardMode != tt.mode || config.VLAN != nil || config.PortIsolated != "" {
			t.Fatalf("config=%+v err=%v", config, err)
		}
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"ips", "portGroups", "forwardInterfaces"} {
			if _, ok := value[key].([]any); !ok {
				t.Fatalf("%s must be [], JSON=%s", key, data)
			}
		}
	}
}

func TestMalformedNetworkXML(t *testing.T) {
	for _, input := range []string{`<network>`, `<domain/>`, `<network><vlan><tag id="bad"/></vlan></network>`} {
		if _, err := parseConfiguration(input); err == nil {
			t.Errorf("accepted malformed XML: %s", input)
		}
	}
}
