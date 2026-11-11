package serialconsole

import (
	"errors"
	"libvirt.org/go/libvirt"
	"testing"
)

type fakeDomain struct {
	active          bool
	runtime         uint
	xml             string
	flags           libvirt.DomainConsoleFlags
	alias           string
	attached, freed int
	attachErr       error
}

func (d *fakeDomain) GetUUIDString() (string, error) { return "vm", nil }
func (d *fakeDomain) GetName() (string, error)       { return "balboa", nil }
func (d *fakeDomain) IsActive() (bool, error)        { return d.active, nil }
func (d *fakeDomain) GetID() (uint, error) {
	if !d.active {
		return 0, errors.New("inactive domain has no runtime ID")
	}
	return d.runtime, nil
}
func (d *fakeDomain) GetInfo() (*libvirt.DomainInfo, error) {
	return &libvirt.DomainInfo{State: libvirt.DOMAIN_RUNNING}, nil
}
func (d *fakeDomain) GetXMLDesc(libvirt.DomainXMLFlags) (string, error) { return d.xml, nil }
func (d *fakeDomain) Free() error                                       { d.freed++; return nil }
func (d *fakeDomain) OpenConsole(alias string, _ nativeStream, flags libvirt.DomainConsoleFlags) error {
	d.alias = alias
	d.flags = flags
	d.attached++
	return d.attachErr
}

type fakeNativeConnection struct {
	dom             *fakeDomain
	raw             *fakeStream
	closed, streams int
	flags           libvirt.StreamFlags
}

func TestInspectInactiveDomainDoesNotRequestRuntimeID(t *testing.T) {
	c, err := inspectDomain(&fakeDomain{active: false, xml: serialXML})
	if err != nil || c.Available || c.Code != "inactive" {
		t.Fatalf("inactive guidance lost: %+v %v", c, err)
	}
}

func (c *fakeNativeConnection) lookup(string) (consoleDomain, error) { return c.dom, nil }
func (c *fakeNativeConnection) newStream(flags libvirt.StreamFlags) (nativeStream, error) {
	c.streams++
	c.flags = flags
	return c.raw, nil
}
func (c *fakeNativeConnection) Close() (int, error) { c.closed++; return 0, nil }

func TestNativeAttachmentRequiresSafeFlagAndReleasesEveryResource(t *testing.T) {
	d := &fakeDomain{active: true, runtime: 2, xml: serialXML}
	c := &fakeNativeConnection{dom: d, raw: &fakeStream{}}
	caps, err := inspectDomain(d)
	if err != nil {
		t.Fatal(err)
	}
	s, err := openWith("", "vm", caps.Devices[0].Key, caps.Revision, func(string) (consoleConnection, error) { return c, nil })
	if err != nil {
		t.Fatal(err)
	}
	if d.alias != "serial0" || d.flags != libvirt.DOMAIN_CONSOLE_SAFE || c.flags != libvirt.STREAM_NONBLOCK {
		t.Fatal("unsafe console attachment")
	}
	s.Close()
	s.Close()
	if d.freed != 1 || c.closed != 1 || c.raw.freed != 1 {
		t.Fatal("native resources leaked or released twice")
	}
}
func TestNativeRejectsStaleSelectionBeforeCreatingStream(t *testing.T) {
	d := &fakeDomain{active: true, runtime: 2, xml: serialXML}
	caps, _ := inspectDomain(d)
	d.runtime = 3
	c := &fakeNativeConnection{dom: d, raw: &fakeStream{}}
	_, err := openWith("", "vm", caps.Devices[0].Key, caps.Revision, func(string) (consoleConnection, error) { return c, nil })
	if err == nil || d.attached != 0 || c.streams != 0 || c.closed != 1 || d.freed != 1 {
		t.Fatal("stale endpoint was opened or leaked")
	}
}
func TestNativeBusyPermissionAndUnsupportedSafeErrorsNeverRetryUnsafe(t *testing.T) {
	for _, message := range []string{"console is in use", "permission denied", "safe console handling is not supported"} {
		t.Run(message, func(t *testing.T) {
			d := &fakeDomain{active: true, runtime: 2, xml: serialXML, attachErr: errors.New(message)}
			caps, _ := inspectDomain(d)
			c := &fakeNativeConnection{dom: d, raw: &fakeStream{}}
			_, err := openWith("", "vm", caps.Devices[0].Key, caps.Revision, func(string) (consoleConnection, error) { return c, nil })
			if !errors.Is(err, d.attachErr) || d.attached != 1 || d.flags != libvirt.DOMAIN_CONSOLE_SAFE || d.freed != 1 || c.closed != 1 || c.raw.freed != 1 {
				t.Fatalf("unsafe fallback or lost failure: %v", err)
			}
		})
	}
}
