// Package networks enumerates libvirt virtual networks using read-only connections.
package networks

import (
	"errors"
	"fmt"
	"sort"

	"libvirt.org/go/libvirt"
)

const DefaultURI = "qemu:///system"

// Network is a snapshot of a virtual network known to the selected libvirt
// connection. It does not include Linux interfaces/bridges outside libvirt.
type Network struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	Persistent bool   `json:"persistent"`
	Autostart  bool   `json:"autostart"`
	Configuration
}

// List returns every visible active/inactive network sorted by name then UUID.
// Empty uri selects DefaultURI. Native calls are synchronous and resources are
// released before return. Errors return no partial inventory and preserve native
// causes. Successful empty inventories serialize as [].
func List(uri string) (result []Network, retErr error) {
	if uri == "" {
		uri = DefaultURI
	}
	conn, err := libvirt.NewConnectReadOnly(uri)
	if err != nil {
		return nil, fmt.Errorf("connect to libvirt %q read-only: %w", uri, err)
	}
	defer func() {
		remaining, err := conn.Close()
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %w", uri, err))
		} else if remaining != 0 {
			retErr = errors.Join(retErr, fmt.Errorf("close libvirt connection %q: %d references remain", uri, remaining))
		}
		if retErr != nil {
			result = nil
		}
	}()
	return list(conn)
}

func list(conn *libvirt.Connect) (result []Network, retErr error) {
	handles, err := conn.ListAllNetworks(0)
	if err != nil {
		return nil, fmt.Errorf("list libvirt networks: %w", err)
	}
	defer func() {
		for i := range handles {
			if err := handles[i].Free(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("free libvirt network reference %d: %w", i, err))
			}
		}
		if retErr != nil {
			result = nil
		}
	}()
	result = make([]Network, 0, len(handles))
	for i := range handles {
		network, err := describe(&handles[i])
		if err != nil {
			return nil, err
		}
		result = append(result, network)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].UUID < result[j].UUID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func describe(network *libvirt.Network) (Network, error) {
	name, err := network.GetName()
	if err != nil {
		return Network{}, fmt.Errorf("get libvirt network name: %w", err)
	}
	uuid, err := network.GetUUIDString()
	if err != nil {
		return Network{}, fmt.Errorf("get UUID for network %q: %w", name, err)
	}
	active, err := network.IsActive()
	if err != nil {
		return Network{}, fmt.Errorf("get activity for network %q: %w", name, err)
	}
	persistent, err := network.IsPersistent()
	if err != nil {
		return Network{}, fmt.Errorf("get persistence for network %q: %w", name, err)
	}
	autostart := false
	if persistent {
		autostart, err = network.GetAutostart()
		if err != nil {
			return Network{}, fmt.Errorf("get autostart for network %q: %w", name, err)
		}
	}
	xml, err := network.GetXMLDesc(0)
	if err != nil {
		return Network{}, fmt.Errorf("get XML for network %q: %w", name, err)
	}
	config, err := parseConfiguration(xml)
	if err != nil {
		return Network{}, fmt.Errorf("read configuration for network %q: %w", name, err)
	}
	return Network{UUID: uuid, Name: name, Active: active, Persistent: persistent, Autostart: autostart, Configuration: config}, nil
}
