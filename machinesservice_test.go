package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"libvirt.org/go/libvirt"
)

func TestMachinesServiceReturnsDomainInventory(t *testing.T) {
	service := &MachinesService{uri: "test:///default"}
	guests, err := service.ListMachines()
	if err != nil {
		t.Fatal(err)
	}
	if len(guests) != 1 || guests[0].Name != "test" || guests[0].UUID == "" || !guests[0].Active || guests[0].Interfaces == nil {
		t.Fatalf("unexpected machine inventory: %+v", guests)
	}
}

func TestMachineInventoryIncludesHostCapacityFromSameURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.xml")
	xml := `<node><cpu><model>i686</model><active>12</active><nodes>1</nodes><sockets>1</sockets><cores>8</cores><threads>2</threads></cpu><memory>25165824</memory></node>`
	if err := os.WriteFile(path, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	service := &MachinesService{uri: "test://" + path}
	inventory, err := service.GetInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Machines == nil || len(inventory.Machines) != 0 || inventory.Host.MemoryKiB != 25165824 || inventory.Host.LogicalCPUs != 3 {
		t.Fatalf("incorrect inventory: %+v", inventory)
	}
}

func TestMachineInventoryPreservesConnectionError(t *testing.T) {
	service := &MachinesService{uri: "test://" + filepath.Join(t.TempDir(), "missing.xml")}
	inventory, err := service.GetInventory()
	var nativeError libvirt.Error
	if err == nil || inventory != nil || !errors.As(err, &nativeError) {
		t.Fatalf("lost inventory error: %+v %v", inventory, err)
	}
}

func TestMachinesServicePreservesConnectionError(t *testing.T) {
	service := &MachinesService{uri: "test://" + filepath.Join(t.TempDir(), "missing.xml")}
	guests, err := service.ListMachines()
	var nativeError libvirt.Error
	if err == nil || guests != nil || !errors.As(err, &nativeError) {
		t.Fatalf("lost native connection failure: %+v %v", guests, err)
	}
}
