package hostinfo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"libvirt.org/go/libvirt"
)

func TestReadResourcesFromSelectedHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.xml")
	// Memory is KiB. The test driver's CPU map is a fixed fixture with three
	// online CPUs, independent of the node topology configured in this XML.
	xml := `<node><cpu><model>i686</model><active>12</active><nodes>1</nodes><sockets>1</sockets><cores>8</cores><threads>2</threads></cpu><memory>25165824</memory></node>`
	if err := os.WriteFile(path, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	resources, err := Read("test://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if resources.MemoryKiB != 25165824 || resources.LogicalCPUs != 3 {
		t.Fatalf("incorrect host capacity: %+v", resources)
	}
}

func TestReadPreservesNativeConnectionError(t *testing.T) {
	_, err := Read("test://" + filepath.Join(t.TempDir(), "missing.xml"))
	var nativeError libvirt.Error
	if err == nil || !errors.As(err, &nativeError) {
		t.Fatalf("lost libvirt error: %v", err)
	}
}
