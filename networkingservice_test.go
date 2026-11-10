package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gjergj/tinymachinemanager/pkg/hostnet"
)

func TestNetworkingInventoryCombinesIndependentSources(t *testing.T) {
	result := readNetworkingInventory("test:///default", func() ([]hostnet.Interface, error) {
		return []hostnet.Interface{{Name: "outside-libvirt", IsBridge: true, Members: []string{}, Addresses: []string{}}}, nil
	})
	if len(result.Issues) != 0 || len(result.Networks) != 1 || result.Networks[0].Name != "default" || len(result.Machines) != 1 || result.Machines[0].Name != "test" || len(result.Interfaces) != 1 || result.Interfaces[0].Name != "outside-libvirt" {
		t.Fatalf("incorrect networking inventory: %+v", result)
	}
}

func TestNetworkingInventoryRetainsHostWhenLibvirtUnavailable(t *testing.T) {
	result := readNetworkingInventory("test://"+filepath.Join(t.TempDir(), "missing.xml"), func() ([]hostnet.Interface, error) {
		return []hostnet.Interface{{Name: "br0"}}, nil
	})
	if len(result.Interfaces) != 1 || len(result.Issues) != 2 || result.Issues[0].Source != "networks" || result.Issues[1].Source != "machines" {
		t.Fatalf("sources were lost: %+v", result)
	}
	if result.Networks == nil || result.Machines == nil || !strings.Contains(result.Issues[0].Message, "missing.xml") {
		t.Fatalf("unavailable sources must have arrays and contextual errors: %+v", result)
	}
}

func TestNetworkingInventoryRetainsLibvirtWhenHostUnavailable(t *testing.T) {
	result := readNetworkingInventory("test:///default", func() ([]hostnet.Interface, error) {
		return nil, errors.New("sysfs unavailable")
	})
	if len(result.Networks) != 1 || len(result.Machines) != 1 || result.Interfaces == nil || len(result.Issues) != 1 || result.Issues[0].Source != "interfaces" || result.Issues[0].Message != "sysfs unavailable" {
		t.Fatalf("sources were lost: %+v", result)
	}
}

func TestNetworkingInventoryEmptyArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xml")
	if err := os.WriteFile(path, []byte(`<node><memory>1048576</memory></node>`), 0600); err != nil {
		t.Fatal(err)
	}
	result := readNetworkingInventory("test://"+path, func() ([]hostnet.Interface, error) { return []hostnet.Interface{}, nil })
	data, err := json.Marshal(result)
	if err != nil || string(data) != `{"networks":[],"interfaces":[],"machines":[],"issues":[]}` {
		t.Fatalf("empty inventory=%s err=%v", data, err)
	}
}
