package domains

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"libvirt.org/go/libvirt"
)

// The test driver keeps synthetic guests in memory. Never use a real QEMU URI
// here: these helpers deliberately define, start, stop and undefine test guests.
func emptyTestConnection(t *testing.T) *libvirt.Connect {
	t.Helper()
	conn, err := libvirt.NewConnect("test:///default")
	if err != nil {
		t.Fatalf("open synthetic libvirt connection: %v", err)
	}
	t.Cleanup(func() {
		remaining, err := conn.Close()
		if err != nil || remaining != 0 {
			t.Errorf("close connection: remaining references=%d, err=%v", remaining, err)
		}
	})
	handles, err := conn.ListAllDomains(0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range handles {
		dom := &handles[i]
		active, err := dom.IsActive()
		if err != nil {
			t.Fatal(err)
		}
		if active {
			if err := dom.Destroy(); err != nil {
				t.Fatal(err)
			}
		}
		persistent, err := dom.IsPersistent()
		if err != nil {
			t.Fatal(err)
		}
		if persistent {
			if err := dom.Undefine(); err != nil {
				t.Fatal(err)
			}
		}
		if err := dom.Free(); err != nil {
			t.Fatal(err)
		}
	}
	return conn
}

func defineTestDomain(t *testing.T, conn *libvirt.Connect, name, uuid string, running, paused, persistent bool) {
	t.Helper()
	xml := fmt.Sprintf(`<domain type="test">
  <name>%s</name><uuid>%s</uuid>
  <memory unit="KiB">524288</memory>
  <currentMemory unit="KiB">262144</currentMemory>
  <vcpu>2</vcpu><os><type arch="i686" machine="pc">hvm</type></os><devices><emulator>/usr/bin/test-hv</emulator></devices>
</domain>`, name, uuid)
	var dom *libvirt.Domain
	var err error
	if persistent {
		dom, err = conn.DomainDefineXML(xml)
	} else {
		dom, err = conn.DomainCreateXML(xml, 0)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := dom.Free(); err != nil {
			t.Error(err)
		}
	}()
	if persistent && running {
		if err := dom.Create(); err != nil {
			t.Fatal(err)
		}
	}
	if paused {
		if err := dom.Suspend(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListIncludesAllDomainsAndCopiesMetadata(t *testing.T) {
	conn := emptyTestConnection(t)
	defineTestDomain(t, conn, "zeta", "00000000-0000-4000-8000-000000000003", false, false, true)
	defineTestDomain(t, conn, "alpha", "00000000-0000-4000-8000-000000000001", true, false, true)
	defineTestDomain(t, conn, "middle", "00000000-0000-4000-8000-000000000002", true, true, true)
	defineTestDomain(t, conn, "volatile", "00000000-0000-4000-8000-000000000004", true, false, false)
	got, err := list(conn)
	if err != nil {
		t.Fatal(err)
	}
	want := []Domain{
		{UUID: "00000000-0000-4000-8000-000000000001", Name: "alpha", State: StateRunning, Active: true, Persistent: true, VCPUs: 2, MemoryKiB: 262144, MaxMemoryKiB: 524288},
		{UUID: "00000000-0000-4000-8000-000000000002", Name: "middle", State: StatePaused, Active: true, Persistent: true, VCPUs: 2, MemoryKiB: 262144, MaxMemoryKiB: 524288},
		{UUID: "00000000-0000-4000-8000-000000000004", Name: "volatile", State: StateRunning, Active: true, Persistent: false, VCPUs: 2, MemoryKiB: 262144, MaxMemoryKiB: 524288},
		{UUID: "00000000-0000-4000-8000-000000000003", Name: "zeta", State: StateShutoff, Active: false, Persistent: true, VCPUs: 2, MemoryKiB: 262144, MaxMemoryKiB: 524288},
	}
	for i := range want {
		want[i].Type = "test"
		want[i].OS = OS{Type: OSType{Value: "hvm", Arch: "i686", Machine: "pc"}}
		want[i].Devices = Devices{Emulator: "/usr/bin/test-hv", Interfaces: []Interface{}, Serials: []CharacterDevice{}, Consoles: []CharacterDevice{}}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains:\n got %+v\nwant %+v", got, want)
	}
	// Repeated reads should leave the synthetic guests unchanged.
	again, err := list(conn)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("second enumeration: %+v, %v", again, err)
	}
}

func TestEmptyInventoryIsJSONArray(t *testing.T) {
	conn := emptyTestConnection(t)
	domains, err := list(conn)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(domains)
	if err != nil || string(data) != "[]" {
		t.Fatalf("empty inventory = %s, %v", data, err)
	}
}

func TestListOpensReadOnlyTestConnection(t *testing.T) {
	domains, err := List("test:///default")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0].Name != "test" || domains[0].UUID == "" || domains[0].State != StateRunning || !domains[0].Active || !domains[0].Persistent {
		t.Fatalf("unexpected default test inventory: %+v", domains)
	}
}

func TestListReportsConnectionFailure(t *testing.T) {
	domains, err := List("test://" + filepath.Join(t.TempDir(), "missing.xml"))
	var nativeError libvirt.Error
	if err == nil || domains != nil || !errors.As(err, &nativeError) || !strings.Contains(err.Error(), "connect") {
		t.Fatalf("connection error lost: domains=%+v, err=%v", domains, err)
	}
}

func TestDescribeReportsDisappearedDomain(t *testing.T) {
	conn := emptyTestConnection(t)
	defineTestDomain(t, conn, "removed", "00000000-0000-4000-8000-000000000005", false, false, true)
	dom, err := conn.LookupDomainByName("removed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := dom.Free(); err != nil {
			t.Error(err)
		}
	}()
	if err := dom.Undefine(); err != nil {
		t.Fatal(err)
	}
	guest, err := describe(dom)
	var nativeError libvirt.Error
	if err == nil || !reflect.DeepEqual(guest, Domain{}) || !errors.As(err, &nativeError) || nativeError.Code != libvirt.ERR_NO_DOMAIN || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("metadata failure lost: guest=%+v, err=%v", guest, err)
	}
}

func TestStateMapping(t *testing.T) {
	for _, tt := range []struct {
		native libvirt.DomainState
		want   State
	}{
		{libvirt.DOMAIN_NOSTATE, StateUnknown},
		{libvirt.DOMAIN_RUNNING, StateRunning},
		{libvirt.DOMAIN_BLOCKED, StateBlocked},
		{libvirt.DOMAIN_PAUSED, StatePaused},
		{libvirt.DOMAIN_SHUTDOWN, StateShutdown},
		{libvirt.DOMAIN_SHUTOFF, StateShutoff},
		{libvirt.DOMAIN_CRASHED, StateCrashed},
		{libvirt.DOMAIN_PMSUSPENDED, StatePMSuspended},
		{libvirt.DomainState(999), StateUnknown},
	} {
		if got := stateFromLibvirt(tt.native); got != tt.want {
			t.Errorf("state %d = %q, want %q", tt.native, got, tt.want)
		}
	}
}
