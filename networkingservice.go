package main

import (
	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"github.com/Gjergj/tinymachinemanager/pkg/hostnet"
	"github.com/Gjergj/tinymachinemanager/pkg/networks"
)

// NetworkingIssue identifies an unavailable inventory source. Other sources can
// still be displayed; an unavailable source is distinct from an empty one.
type NetworkingIssue struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// NetworkingInventory combines read-only snapshots, not an atomic topology.
// Interfaces belong to the local app's network namespace.
type NetworkingInventory struct {
	Networks   []networks.Network  `json:"networks"`
	Interfaces []hostnet.Interface `json:"interfaces"`
	Machines   []domains.Domain    `json:"machines"`
	Issues     []NetworkingIssue   `json:"issues"`
}

// NetworkingService inventories local networking. The zero value uses the
// system QEMU/KVM connection. It never creates or modifies networks or links.
type NetworkingService struct {
	uri string
}

func (s *NetworkingService) GetInventory() NetworkingInventory {
	return readNetworkingInventory(s.uri, hostnet.List)
}

func readNetworkingInventory(uri string, listHost func() ([]hostnet.Interface, error)) NetworkingInventory {
	result := NetworkingInventory{
		Networks: []networks.Network{}, Interfaces: []hostnet.Interface{},
		Machines: []domains.Domain{}, Issues: []NetworkingIssue{},
	}
	if inventory, err := networks.List(uri); err != nil {
		result.Issues = append(result.Issues, NetworkingIssue{Source: "networks", Message: err.Error()})
	} else {
		result.Networks = inventory
	}
	if inventory, err := domains.List(uri); err != nil {
		result.Issues = append(result.Issues, NetworkingIssue{Source: "machines", Message: err.Error()})
	} else {
		result.Machines = inventory
	}
	if inventory, err := listHost(); err != nil {
		result.Issues = append(result.Issues, NetworkingIssue{Source: "interfaces", Message: err.Error()})
	} else {
		result.Interfaces = inventory
	}
	return result
}
