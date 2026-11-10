package main

import (
	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"github.com/Gjergj/tinymachinemanager/pkg/hostinfo"
)

// MachineInventory carries guest assignments and capacity from the same host.
type MachineInventory struct {
	Machines []domains.Domain   `json:"machines"`
	Host     hostinfo.Resources `json:"host"`
}

// MachinesService exposes the read-only domain inventory to the Wails frontend.
// The zero value uses the system QEMU/KVM connection.
type MachinesService struct {
	uri string
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
	return &MachineInventory{Machines: machines, Host: host}, nil
}
