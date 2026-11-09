package networks

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"libvirt.org/go/libvirt"
)

func emptyTestConnection(t *testing.T) *libvirt.Connect {
	t.Helper()
	conn, err := libvirt.NewConnect("test:///default")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		remaining, err := conn.Close()
		if err != nil || remaining != 0 {
			t.Errorf("close connection: remaining=%d err=%v", remaining, err)
		}
	})
	handles, err := conn.ListAllNetworks(0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range handles {
		network := &handles[i]
		active, err := network.IsActive()
		if err != nil {
			t.Fatal(err)
		}
		if active {
			if err := network.Destroy(); err != nil {
				t.Fatal(err)
			}
		}
		persistent, err := network.IsPersistent()
		if err != nil {
			t.Fatal(err)
		}
		if persistent {
			if err := network.Undefine(); err != nil {
				t.Fatal(err)
			}
		}
		if err := network.Free(); err != nil {
			t.Fatal(err)
		}
	}
	return conn
}

func TestListAllNetworkStates(t *testing.T) {
	conn := emptyTestConnection(t)
	for _, fixture := range []struct {
		name, uuid, bridge            string
		active, persistent, autostart bool
	}{
		{"zeta", "10000000-0000-4000-8000-000000000003", "virbr13", false, true, false},
		{"alpha", "10000000-0000-4000-8000-000000000001", "virbr11", true, true, true},
		{"temporary", "10000000-0000-4000-8000-000000000002", "virbr12", true, false, false},
	} {
		xml := fmt.Sprintf(`<network><name>%s</name><uuid>%s</uuid><bridge name="%s"/><forward mode="nat"/><ip address="192.168.10.1" prefix="24"/></network>`, fixture.name, fixture.uuid, fixture.bridge)
		var network *libvirt.Network
		var err error
		if fixture.persistent {
			network, err = conn.NetworkDefineXML(xml)
		} else {
			network, err = conn.NetworkCreateXML(xml)
		}
		if err != nil {
			t.Fatal(err)
		}
		if fixture.persistent && fixture.active {
			if err := network.Create(); err != nil {
				t.Fatal(err)
			}
		}
		if fixture.autostart {
			if err := network.SetAutostart(true); err != nil {
				t.Fatal(err)
			}
		}
		if err := network.Free(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := list(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("networks=%+v", got)
	}
	for i, name := range []string{"alpha", "temporary", "zeta"} {
		if got[i].Name != name {
			t.Fatalf("networks not complete/sorted: %+v", got)
		}
	}
	if got[0].UUID != "10000000-0000-4000-8000-000000000001" || !got[0].Active || !got[0].Persistent || !got[0].Autostart || got[0].Bridge != "virbr11" || got[0].ForwardMode != "nat" || len(got[0].IPs) != 1 {
		t.Fatalf("metadata missing: %+v", got[0])
	}
	if !got[1].Active || got[1].Persistent || got[1].Autostart || got[2].Active || !got[2].Persistent {
		t.Fatalf("state missing: %+v", got)
	}
	again, err := list(conn)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("enumeration mutated networks: %+v %v", again, err)
	}
}

func TestEmptyNetworksIsJSONArray(t *testing.T) {
	result, err := list(emptyTestConnection(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil || string(data) != "[]" {
		t.Fatalf("empty=%s err=%v", data, err)
	}
}

func TestPublicListAndConnectionFailure(t *testing.T) {
	result, err := List("test:///default")
	if err != nil || len(result) != 1 || result[0].Name != "default" || result[0].UUID == "" || !result[0].Active {
		t.Fatalf("default inventory=%+v err=%v", result, err)
	}
	result, err = List("test://" + filepath.Join(t.TempDir(), "missing.xml"))
	var nativeError libvirt.Error
	if err == nil || result != nil || !errors.As(err, &nativeError) {
		t.Fatalf("lost connection error: %+v %v", result, err)
	}
}

func TestDisappearedNetworkError(t *testing.T) {
	conn := emptyTestConnection(t)
	network, err := conn.NetworkDefineXML(`<network><name>removed</name><bridge name="virbr99"/></network>`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := network.Free(); err != nil {
			t.Error(err)
		}
	}()
	if err := network.Undefine(); err != nil {
		t.Fatal(err)
	}
	_, err = describe(network)
	var nativeError libvirt.Error
	if err == nil || !errors.As(err, &nativeError) || nativeError.Code != libvirt.ERR_NO_NETWORK {
		t.Fatalf("metadata error=%v", err)
	}
}
