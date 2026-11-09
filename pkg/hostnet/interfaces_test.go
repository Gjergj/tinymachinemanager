package hostnet

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func sysfsFixture(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "operstate"), []byte("up\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLocalBridgeTopologyAndAddresses(t *testing.T) {
	root := sysfsFixture(t, "vnet1", "br0", "eno1", "lo")
	if err := os.Mkdir(filepath.Join(root, "br0", "bridge"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vnet1", "eno1"} {
		if err := os.Symlink("../br0", filepath.Join(root, name, "master")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "eno1", "operstate"), []byte("down\n"), 0644); err != nil {
		t.Fatal(err)
	}
	links := []net.Interface{
		{Index: 4, Name: "vnet1", MTU: 1500, HardwareAddr: net.HardwareAddr{0x52, 0x54, 0, 0, 0, 1}, Flags: net.FlagUp | net.FlagRunning},
		{Index: 2, Name: "br0", MTU: 1500, Flags: net.FlagUp | net.FlagRunning},
		{Index: 3, Name: "eno1", MTU: 1500},
		{Index: 1, Name: "lo", MTU: 65536, Flags: net.FlagUp | net.FlagLoopback},
	}
	addresses := func(link net.Interface) ([]net.Addr, error) {
		if link.Name != "br0" {
			return nil, nil
		}
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("192.168.10.1"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}
	result, err := list(links, addresses, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 4 {
		t.Fatalf("missing links: %+v", result)
	}
	wantNames := []string{"br0", "eno1", "lo", "vnet1"}
	for i, name := range wantNames {
		if result[i].Name != name {
			t.Fatalf("unsorted links: %+v", result)
		}
	}
	bridge := result[0]
	if !bridge.IsBridge || bridge.Master != "" || !reflect.DeepEqual(bridge.Members, []string{"eno1", "vnet1"}) || !reflect.DeepEqual(bridge.Addresses, []string{"192.168.10.1/24", "fd00::1/64"}) {
		t.Fatalf("bridge=%+v", bridge)
	}
	if result[1].Up || result[1].OperState != "down" || result[1].Master != "br0" {
		t.Fatalf("down link lost: %+v", result[1])
	}
	if !result[2].Loopback || result[2].MTU != 65536 {
		t.Fatalf("loopback lost: %+v", result[2])
	}
	if result[3].Index != 4 || result[3].MAC != "52:54:00:00:00:01" || !result[3].Up || !result[3].Running || result[3].IsBridge || result[3].Master != "br0" {
		t.Fatalf("tap link=%+v", result[3])
	}
	data, err := json.Marshal(result[3])
	if err != nil || !strings.Contains(string(data), `"addresses":[]`) || !strings.Contains(string(data), `"members":[]`) {
		t.Fatalf("empty link arrays: %s %v", data, err)
	}
}

func TestEmptyHostInterfacesAndErrors(t *testing.T) {
	root := sysfsFixture(t, "eth0")
	addresses := func(net.Interface) ([]net.Addr, error) { return nil, nil }
	result, err := list(nil, addresses, root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil || string(data) != "[]" {
		t.Fatalf("empty=%s err=%v", data, err)
	}
	addressError := errors.New("address lookup failed")
	result, err = list([]net.Interface{{Name: "eth0"}}, func(net.Interface) ([]net.Addr, error) { return nil, addressError }, root)
	if result != nil || !errors.Is(err, addressError) || !strings.Contains(err.Error(), "eth0") {
		t.Fatalf("address error lost: %v", err)
	}
	result, err = list([]net.Interface{{Name: "missing"}}, addresses, root)
	if result != nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("sysfs error lost: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "eth0", "operstate")); err != nil {
		t.Fatal(err)
	}
	result, err = list([]net.Interface{{Name: "eth0"}}, addresses, root)
	if result != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state error lost: %v", err)
	}
}

func TestListLocalLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("local sysfs inventory requires Linux")
	}
	result, err := List()
	if errors.Is(err, syscall.EPERM) {
		t.Skipf("environment blocks read-only route netlink enumeration: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("successful inventory should be allocated")
	}
	for i, link := range result {
		if link.Name == "" || link.Index <= 0 || link.Addresses == nil || link.Members == nil {
			t.Fatalf("invalid local link: %+v", link)
		}
		if i > 0 && result[i-1].Name > link.Name {
			t.Fatal("local inventory not sorted")
		}
	}
}
