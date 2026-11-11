package main

import (
	"bytes"
	"context"
	"github.com/Gjergj/tinymachinemanager/pkg/serialconsole"
	"github.com/wailsapp/wails/v3/pkg/application"
	"slices"
	"strings"
	"testing"
)

type fakeConsoleWindow struct {
	id      uint
	focused int
}

func (w *fakeConsoleWindow) ID() uint { return w.id }
func (w *fakeConsoleWindow) Focus()   { w.focused++ }
func (w *fakeConsoleWindow) Close()   {}

func TestConsoleServiceReusesWindowAndChecksOwner(t *testing.T) {
	window := &fakeConsoleWindow{id: 7}
	created := 0
	s := &ConsoleService{windows: map[string]consoleWindow{}, bridge: serialconsole.NewBridge([]string{"wails://localhost"}, false), inspect: func(string, string) (serialconsole.Capabilities, error) {
		return serialconsole.Capabilities{UUID: "vm", Name: "balboa", Active: true, Available: true, Revision: "rev", Devices: []serialconsole.Device{{Key: "key", Supported: true}}}, nil
	}}
	defer s.bridge.Close()
	s.createWindow = func(string, string) consoleWindow { created++; return window }
	if err := s.OpenWindow("vm"); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenWindow("vm"); err != nil {
		t.Fatal(err)
	}
	if created != 1 || window.focused != 1 {
		t.Fatal("duplicate window created instead of focused")
	}
	ctx := context.WithValue(context.Background(), application.WindowKey, window)
	if _, err := s.CreateSession(context.Background(), "vm", "key", "rev"); err == nil {
		t.Fatal("unowned binding call accepted")
	}
	c, err := s.CreateSession(ctx, "vm", "key", "rev")
	if err != nil {
		t.Fatal(err)
	}
	other := context.WithValue(context.Background(), application.WindowKey, &fakeConsoleWindow{id: 8})
	if err := s.Disconnect(other, c.ID); err == nil {
		t.Fatal("other window accepted")
	}
	if err := s.Disconnect(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	s.removeWindow("vm", 7)
	if _, err := s.CreateSession(ctx, "vm", "key", "rev"); err == nil {
		t.Fatal("closed window accepted")
	}
}

func TestConsoleLoggerDropsBindingPayloads(t *testing.T) {
	var output bytes.Buffer
	consoleLogger(&output).Info("Binding call complete:", "args", "private-input", "result", "private-token", "id", 42)
	if strings.Contains(output.String(), "private-") {
		t.Fatal("binding credentials logged")
	}
}

func TestConsoleOriginsIncludeWailsDevelopmentPort(t *testing.T) {
	origins, opaque, err := consoleOrigins("http://127.0.0.1:19245")
	if err != nil || opaque || !slices.Contains(origins, "wails://localhost:19245") || !slices.Contains(origins, "http://127.0.0.1:19245") {
		t.Fatalf("native dev origin missing: %v %v", origins, err)
	}
	if _, _, err = consoleOrigins("https://remote.example:19245"); err == nil {
		t.Fatal("remote development origin accepted")
	}
}
