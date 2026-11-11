package serialconsole

import (
	"errors"
	"fmt"
	"github.com/Gjergj/tinymachinemanager/pkg/domains"
	"libvirt.org/go/libvirt"
	"sync"
)

// Open revalidates on the same dedicated read-write connection used to claim
// the stream. SAFE is mandatory: another console client is never displaced.
func Open(uri, uuid, key, revision string) (*Session, error) {
	return openWith(uri, uuid, key, revision, func(uri string) (consoleConnection, error) {
		conn, err := libvirt.NewConnect(uri)
		if err != nil {
			return nil, err
		}
		return nativeConsoleConnection{conn}, nil
	})
}

type consoleDomain interface {
	domainMetadata
	Free() error
	OpenConsole(string, nativeStream, libvirt.DomainConsoleFlags) error
}
type consoleConnection interface {
	lookup(string) (consoleDomain, error)
	newStream(libvirt.StreamFlags) (nativeStream, error)
	Close() (int, error)
}
type nativeConsoleConnection struct{ *libvirt.Connect }

func (c nativeConsoleConnection) lookup(uuid string) (consoleDomain, error) {
	d, err := c.LookupDomainByUUIDString(uuid)
	if err != nil {
		return nil, err
	}
	return nativeConsoleDomain{d}, nil
}
func (c nativeConsoleConnection) newStream(flags libvirt.StreamFlags) (nativeStream, error) {
	return c.NewStream(flags)
}

type nativeConsoleDomain struct{ *libvirt.Domain }

func (d nativeConsoleDomain) OpenConsole(alias string, stream nativeStream, flags libvirt.DomainConsoleFlags) error {
	return d.Domain.OpenConsole(alias, stream.(*libvirt.Stream), flags)
}

func openWith(uri, uuid, key, revision string, connect func(string) (consoleConnection, error)) (*Session, error) {
	if uri == "" {
		uri = domains.DefaultURI
	}
	if uri != domains.DefaultURI && uri != "qemu:///session" {
		return nil, errors.New("Only local QEMU connections are supported.")
	}
	conn, err := connect(uri)
	if err != nil {
		return nil, fmt.Errorf("Access to libvirt is required to attach a console: %w", err)
	}
	dom, err := conn.lookup(uuid)
	if err != nil {
		conn.Close()
		return nil, err
	}
	var once sync.Once
	release := func() (err error) {
		once.Do(func() { err = dom.Free(); _, closeErr := conn.Close(); err = errors.Join(err, closeErr) })
		return
	}
	c, err := inspectDomain(dom)
	if err != nil {
		release()
		return nil, err
	}
	d, err := validate(c, key, revision)
	if err != nil {
		release()
		return nil, err
	}
	stream, err := conn.newStream(libvirt.STREAM_NONBLOCK)
	if err != nil {
		release()
		return nil, err
	}
	if err = dom.OpenConsole(d.Alias, stream, libvirt.DOMAIN_CONSOLE_SAFE); err != nil {
		stream.Free()
		release()
		return nil, fmt.Errorf("Could not attach safely. The console may be in use, access may be denied, or safe attachment may be unsupported: %w", err)
	}
	return newSession(stream, release)
}

var eventOnce sync.Once
var eventError error

// RegisterEventLoop must run before any libvirt connection. On Linux, libvirt's
// default implementation installs GLib sources on the default main context.
// The host GTK application dispatches them alongside its UI events. Running
// EventRunDefaultImpl on another thread would contend with GTK for that context.
func RegisterEventLoop() error {
	eventOnce.Do(func() {
		eventError = libvirt.EventRegisterDefaultImpl()
	})
	return eventError
}
