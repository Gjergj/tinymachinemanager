// Package hostnet inventories interfaces and kernel bridges in the local Linux
// network namespace. It never changes interface or network configuration.
package hostnet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Interface describes an OS link, including links created outside libvirt.
// IsBridge identifies a Linux kernel bridge, not an Open vSwitch bridge.
// Master and Members apply to other master devices (such as bonds) as well.
// Addresses are IP/prefix strings assigned to this host link, not guest IPs.
type Interface struct {
	Index     int      `json:"index"`
	Name      string   `json:"name"`
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
	Running   bool     `json:"running"`
	Loopback  bool     `json:"loopback"`
	OperState string   `json:"operState"`
	IsBridge  bool     `json:"isBridge"`
	Master    string   `json:"master,omitempty"`
	Members   []string `json:"members"`
	Addresses []string `json:"addresses"`
}

// List returns all local interfaces, including down, loopback, bridge and tap
// links, sorted by name. It requires Linux/sysfs but no root privileges. This is
// a snapshot of the app's network namespace, never a remote libvirt host.
func List() ([]Interface, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("host interface enumeration requires Linux, running on %s", runtime.GOOS)
	}
	links, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list local host interfaces: %w", err)
	}
	return list(links, func(link net.Interface) ([]net.Addr, error) { return link.Addrs() }, "/sys/class/net")
}

func list(links []net.Interface, addresses func(net.Interface) ([]net.Addr, error), sysfsRoot string) ([]Interface, error) {
	result := make([]Interface, 0, len(links))
	for _, link := range links {
		addrs, err := addresses(link)
		if err != nil {
			return nil, fmt.Errorf("get addresses for host interface %q: %w", link.Name, err)
		}
		path := filepath.Join(sysfsRoot, link.Name)
		// A missing interface directory is an inventory race/error; a missing
		// master or bridge entry within an existing directory is normal.
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("inspect host interface %q: %w", link.Name, err)
		}
		state, err := os.ReadFile(filepath.Join(path, "operstate"))
		if err != nil {
			return nil, fmt.Errorf("get state for host interface %q: %w", link.Name, err)
		}
		bridge, err := os.Stat(filepath.Join(path, "bridge"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect bridge for host interface %q: %w", link.Name, err)
		}
		iface := Interface{
			Index: link.Index, Name: link.Name, MAC: link.HardwareAddr.String(), MTU: link.MTU,
			Up: link.Flags&net.FlagUp != 0, Running: link.Flags&net.FlagRunning != 0,
			Loopback: link.Flags&net.FlagLoopback != 0, OperState: strings.TrimSpace(string(state)),
			IsBridge: bridge != nil && bridge.IsDir(), Members: []string{}, Addresses: []string{},
		}
		master, err := os.Readlink(filepath.Join(path, "master"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("get master for host interface %q: %w", link.Name, err)
		}
		if err == nil {
			iface.Master = filepath.Base(master)
		}
		for _, address := range addrs {
			iface.Addresses = append(iface.Addresses, address.String())
		}
		sort.Strings(iface.Addresses)
		result = append(result, iface)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	byName := make(map[string]int, len(result))
	for i := range result {
		byName[result[i].Name] = i
	}
	for _, link := range result {
		if parent, ok := byName[link.Master]; ok {
			result[parent].Members = append(result[parent].Members, link.Name)
		}
	}
	return result, nil
}
