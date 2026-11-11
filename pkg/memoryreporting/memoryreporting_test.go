package memoryreporting

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"libvirt.org/go/libvirt"
)

// Only the synthetic libvirt driver is used here. These tests must never write
// settings on a real hypervisor.
const testURI = "test:///default"

type fakeDomain struct {
	xml                   string
	active, persistent    bool
	xmlErr, activeErr     error
	persistentErr, setErr error
	freeErr               error
	events                *[]string
	xmlFlags              []libvirt.DomainXMLFlags
	periods               []int
	flags                 []libvirt.DomainMemoryModFlags
}

func (d *fakeDomain) GetXMLDesc(flags libvirt.DomainXMLFlags) (string, error) {
	*d.events = append(*d.events, "xml")
	d.xmlFlags = append(d.xmlFlags, flags)
	return d.xml, d.xmlErr
}

func (d *fakeDomain) IsActive() (bool, error) {
	*d.events = append(*d.events, "active")
	return d.active, d.activeErr
}

func (d *fakeDomain) IsPersistent() (bool, error) {
	*d.events = append(*d.events, "persistent")
	return d.persistent, d.persistentErr
}

func (d *fakeDomain) SetMemoryStatsPeriod(period int, flags libvirt.DomainMemoryModFlags) error {
	*d.events = append(*d.events, "set")
	d.periods = append(d.periods, period)
	d.flags = append(d.flags, flags)
	return d.setErr
}

func (d *fakeDomain) Free() error {
	*d.events = append(*d.events, "free")
	return d.freeErr
}

type fakeConnection struct {
	domain     memoryDomain
	lookupErr  error
	closeErr   error
	remaining  int
	events     *[]string
	lookupUUID string
}

func (c *fakeConnection) LookupDomainByUUIDString(uuid string) (memoryDomain, error) {
	*c.events = append(*c.events, "lookup")
	c.lookupUUID = uuid
	return c.domain, c.lookupErr
}

func (c *fakeConnection) Close() (int, error) {
	*c.events = append(*c.events, "close")
	return c.remaining, c.closeErr
}

func writerFixture() (*fakeDomain, *fakeConnection, *[]string) {
	events := []string{}
	dom := &fakeDomain{
		xml:    `<domain><devices><memballoon model="virtio"/></devices></domain>`,
		active: true, persistent: true, events: &events,
	}
	conn := &fakeConnection{domain: dom, events: &events}
	return dom, conn, &events
}

// A wrong period or lifecycle flag could alter only a running copy or fail to
// persist the explicit opt-in, so verify the actual native write arguments.
func TestEnableSelectsFiveSecondPeriodAndEligibleFlags(t *testing.T) {
	for _, tt := range []struct {
		name               string
		active, persistent bool
		wantFlags          libvirt.DomainMemoryModFlags
	}{
		{"running persistent", true, true, libvirt.DOMAIN_MEM_LIVE | libvirt.DOMAIN_MEM_CONFIG},
		{"stopped persistent", false, true, libvirt.DOMAIN_MEM_CONFIG},
		{"running transient", true, false, libvirt.DOMAIN_MEM_LIVE},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dom, conn, events := writerFixture()
			dom.active, dom.persistent = tt.active, tt.persistent
			err := enable(testURI, "selected-uuid", func(uri string) (memoryConnection, error) {
				if uri != testURI {
					t.Fatalf("opened URI %q", uri)
				}
				return conn, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if conn.lookupUUID != "selected-uuid" || !reflect.DeepEqual(dom.periods, []int{5}) || !reflect.DeepEqual(dom.flags, []libvirt.DomainMemoryModFlags{tt.wantFlags}) {
				t.Fatalf("write = uuid %q, periods %v, flags %v", conn.lookupUUID, dom.periods, dom.flags)
			}
			if !reflect.DeepEqual(dom.xmlFlags, []libvirt.DomainXMLFlags{0}) {
				t.Fatalf("XML flags = %v; only nonsecure current XML is needed", dom.xmlFlags)
			}
			if !reflect.DeepEqual(*events, []string{"lookup", "xml", "active", "persistent", "set", "free", "close"}) {
				t.Fatalf("operation/cleanup order = %v", *events)
			}
		})
	}
}

func TestEnableAcceptsSupportedVirtioBalloonVariants(t *testing.T) {
	for _, model := range []string{"virtio", "virtio-transitional", "virtio-non-transitional"} {
		t.Run(model, func(t *testing.T) {
			dom, conn, _ := writerFixture()
			dom.xml = `<domain><devices><memballoon model="` + model + `"/></devices></domain>`
			if err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) { return conn, nil }); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(dom.periods, []int{5}) {
				t.Fatalf("periods = %v", dom.periods)
			}
		})
	}
}

func TestEnableRejectsUnsupportedOrInvalidBalloonWithoutWriting(t *testing.T) {
	for _, tt := range []struct{ name, xml string }{
		{"missing", `<domain><devices/></domain>`},
		{"disabled", `<domain><devices><memballoon model="none"/></devices></domain>`},
		{"unsupported", `<domain><devices><memballoon model="xen"/></devices></domain>`},
		{"future", `<domain><devices><memballoon model="future-model"/></devices></domain>`},
		{"unspecified", `<domain><devices><memballoon/></devices></domain>`},
		{"wrong root", `<network><devices><memballoon model="virtio"/></devices></network>`},
		{"malformed", `<domain>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dom, conn, events := writerFixture()
			dom.xml = tt.xml
			err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) { return conn, nil })
			if err == nil || len(dom.periods) != 0 {
				t.Fatalf("invalid balloon: err=%v, periods=%v", err, dom.periods)
			}
			if !reflect.DeepEqual(*events, []string{"lookup", "xml", "free", "close"}) {
				t.Fatalf("invalid balloon cleanup = %v", *events)
			}
		})
	}
}

func TestEnableRejectsInactiveTransientWithoutWriting(t *testing.T) {
	dom, conn, events := writerFixture()
	dom.active, dom.persistent = false, false
	err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) { return conn, nil })
	if err == nil || len(dom.periods) != 0 || !strings.Contains(err.Error(), "inactive transient") {
		t.Fatalf("inactive transient: err=%v, periods=%v", err, dom.periods)
	}
	if !reflect.DeepEqual(*events, []string{"lookup", "xml", "active", "persistent", "free", "close"}) {
		t.Fatalf("cleanup = %v", *events)
	}
}

func TestEnableDefaultsURIWithoutTouchingRealHost(t *testing.T) {
	dom, conn, _ := writerFixture()
	var openedURI string
	err := enable("", "selected-uuid", func(uri string) (memoryConnection, error) {
		openedURI = uri
		return conn, nil // Fake only: never open the real default URI in tests.
	})
	if err != nil || openedURI != "qemu:///system" || len(dom.periods) != 1 {
		t.Fatalf("default URI = %q, err=%v", openedURI, err)
	}
}

func TestEnablePreservesErrorsAndReleasesResources(t *testing.T) {
	for _, stage := range []string{"open", "lookup", "xml", "active", "persistent", "set", "free", "close"} {
		t.Run(stage, func(t *testing.T) {
			dom, conn, events := writerFixture()
			failure := errors.New("native failure")
			switch stage {
			case "lookup":
				conn.lookupErr = failure
			case "xml":
				dom.xmlErr = failure
			case "active":
				dom.activeErr = failure
			case "persistent":
				dom.persistentErr = failure
			case "set":
				dom.setErr = failure
			case "free":
				dom.freeErr = failure
			case "close":
				conn.closeErr = failure
			}
			err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) {
				if stage == "open" {
					return nil, failure
				}
				return conn, nil
			})
			if !errors.Is(err, failure) || err == failure {
				t.Fatalf("%s error lost context/cause: %v", stage, err)
			}
			if stage == "open" {
				if len(*events) != 0 {
					t.Fatalf("open failure events = %v", *events)
				}
				return
			}
			if stage == "lookup" {
				if !reflect.DeepEqual(*events, []string{"lookup", "close"}) {
					t.Fatalf("lookup failure events = %v", *events)
				}
				return
			}
			if !reflect.DeepEqual((*events)[len(*events)-2:], []string{"free", "close"}) {
				t.Fatalf("domain must be freed before connection closes: %v", *events)
			}
			if (stage == "xml" || stage == "active" || stage == "persistent") && len(dom.periods) != 0 {
				t.Fatalf("wrote despite failed precondition: %v", dom.periods)
			}
		})
	}
}

func TestEnableJoinsOperationAndCleanupErrors(t *testing.T) {
	dom, conn, _ := writerFixture()
	dom.setErr, dom.freeErr, conn.closeErr = errors.New("set failed"), errors.New("free failed"), errors.New("close failed")
	err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) { return conn, nil })
	for _, cause := range []error{dom.setErr, dom.freeErr, conn.closeErr} {
		if !errors.Is(err, cause) {
			t.Errorf("lost %v in %v", cause, err)
		}
	}
}

func TestEnableReportsRemainingConnectionReferences(t *testing.T) {
	_, conn, _ := writerFixture()
	conn.remaining = 1
	err := enable(testURI, "selected-uuid", func(string) (memoryConnection, error) { return conn, nil })
	if err == nil || !strings.Contains(err.Error(), "1 references remain") {
		t.Fatalf("remaining references error = %v", err)
	}
}

func TestEnablePreservesNativeConnectionAndLookupErrors(t *testing.T) {
	for _, tt := range []struct{ name, uri, uuid string }{
		{"missing connection", "test://" + filepath.Join(t.TempDir(), "missing.xml"), "00000000-0000-4000-8000-000000000001"},
		{"missing domain", testURI, "00000000-0000-4000-8000-000000000001"},
		{"invalid uuid", testURI, "invalid-uuid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Enable(tt.uri, tt.uuid)
			var nativeError libvirt.Error
			if err == nil || !errors.As(err, &nativeError) {
				t.Fatalf("native error lost: %v", err)
			}
		})
	}
}

type closeTrackingConnection struct {
	memoryConnection
	closed    bool
	remaining int
	closeErr  error
}

func (c *closeTrackingConnection) Close() (int, error) {
	c.closed = true
	c.remaining, c.closeErr = c.memoryConnection.Close()
	return c.remaining, c.closeErr
}

// A leaked real domain reference causes virConnectClose to return a nonzero
// count, which the synthetic driver can verify without touching a real guest.
func TestEnableReleasesNativeDomainBeforeClosingConnection(t *testing.T) {
	conn, err := libvirt.NewConnect(testURI)
	if err != nil {
		t.Fatal(err)
	}
	tracked := &closeTrackingConnection{memoryConnection: nativeConnection{conn}}
	t.Cleanup(func() {
		if !tracked.closed {
			if remaining, err := tracked.Close(); err != nil || remaining != 0 {
				t.Errorf("cleanup connection: remaining=%d, err=%v", remaining, err)
			}
		}
	})
	dom, err := conn.LookupDomainByName("test")
	if err != nil {
		t.Fatal(err)
	}
	uuid, uuidErr := dom.GetUUIDString()
	if err := dom.Free(); err != nil {
		t.Fatal(err)
	}
	if uuidErr != nil {
		t.Fatal(uuidErr)
	}
	// The native test driver may reject statistics-period changes. Both success
	// and an unsupported-operation error must release the native handle.
	_ = enable(testURI, uuid, func(string) (memoryConnection, error) { return tracked, nil })
	if !tracked.closed || tracked.closeErr != nil || tracked.remaining != 0 {
		t.Fatalf("native connection cleanup: closed=%t, remaining=%d, err=%v", tracked.closed, tracked.remaining, tracked.closeErr)
	}
}
