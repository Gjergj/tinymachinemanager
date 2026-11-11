package main

import (
	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"github.com/Gjergj/tinymachinemanager/pkg/hostinfo"
	"github.com/Gjergj/tinymachinemanager/pkg/memoryreporting"
	"github.com/Gjergj/tinymachinemanager/pkg/usage"
)

// MachineInventory carries guest assignments and capacity from the same host.
type MachineInventory struct {
	Machines []domains.Domain   `json:"machines"`
	Host     hostinfo.Resources `json:"host"`
	Usage    []usage.GuestUsage `json:"usage"`
}

// MachinesService exposes inventory and usage plus an explicit per-VM action
// for enabling memory statistics. Inventory reads never configure guests.
// The zero value uses the system QEMU/KVM connection.
type MachinesService struct {
	uri       string
	collector usage.Collector
}

func (s *MachinesService) ListMachines() ([]domains.Domain, error) {
	return domains.List(s.uri)
}

func (s *MachinesService) GetInventory() (*MachineInventory, error) {
	machines, err := s.ListMachines()
	if err != nil {
		return nil, err
	}
	host, err := hostinfo.Read(s.uri)
	if err != nil {
		return nil, err
	}
	return &MachineInventory{Machines: machines, Host: host, Usage: s.collector.Collect(s.uri, machines)}, nil
}

func (s *MachinesService) EnableMemoryReporting(uuid string) error {
	return memoryreporting.Enable(s.uri, uuid)
}
