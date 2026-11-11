package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/Gjergj/tinymachinemanager/pkg/serialconsole"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type consoleWindow interface {
	ID() uint
	Focus()
	Close()
}

// ConsoleService owns window lifetimes; terminal bytes never cross Wails RPC.
type ConsoleService struct {
	mu           sync.Mutex
	windows      map[string]consoleWindow
	bridge       *serialconsole.Bridge
	inspect      func(string, string) (serialconsole.Capabilities, error)
	createWindow func(string, string) consoleWindow
	stopped      bool
}

func (s *ConsoleService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.windows = make(map[string]consoleWindow)
	s.inspect = serialconsole.Inspect
	origins, opaque, err := consoleOrigins(os.Getenv("FRONTEND_DEVSERVER_URL"))
	if err != nil {
		return err
	}
	s.bridge = serialconsole.NewBridge(origins, opaque)
	s.createWindow = func(uuid, name string) consoleWindow {
		win := application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
			Name: "serial-" + uuid, Title: name + " — Serial console", Width: 960, Height: 640, MinWidth: 640, MinHeight: 400,
			BackgroundColour: application.NewRGB(23, 23, 23), URL: "/?console=" + url.QueryEscape(uuid),
		})
		win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { s.removeWindow(uuid, win.ID()) })
		return win
	}
	return nil
}

func consoleOrigins(devURL string) ([]string, bool, error) {
	origins := []string{"wails://localhost"}
	if devURL == "" {
		return origins, true, nil
	}
	u, err := url.Parse(devURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !(u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, false, errors.New("Console development origin must be an exact loopback HTTP URL.")
	}
	origins = append(origins, u.Scheme+"://"+u.Host)
	// Wails retains its custom scheme in dev, but copies the Vite port into
	// the native page URL (assetserver.GetStartURL). Allow that exact origin.
	if u.Port() != "" {
		origins = append(origins, "wails://localhost:"+u.Port())
	}
	return origins, false, nil
}

func (s *ConsoleService) GetCapabilities(uuid string) (serialconsole.Capabilities, error) {
	return s.inspect("", uuid)
}

func (s *ConsoleService) OpenWindow(uuid string) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("Console service has stopped.")
	}
	if w := s.windows[uuid]; w != nil {
		s.mu.Unlock()
		w.Focus()
		return nil
	}
	s.mu.Unlock()
	c, err := s.GetCapabilities(uuid)
	if err != nil {
		return err
	}
	if !c.Available {
		return fmt.Errorf("%s", c.Reason)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("Console service has stopped.")
	}
	if w := s.windows[uuid]; w != nil {
		w.Focus()
		return nil
	}
	s.windows[uuid] = s.createWindow(uuid, c.Name)
	return nil
}

func callerID(ctx context.Context) (uint, error) {
	w, ok := ctx.Value(application.WindowKey).(interface{ ID() uint })
	if !ok || w.ID() == 0 {
		return 0, errors.New("A registered console window is required.")
	}
	return w.ID(), nil
}
func (s *ConsoleService) ownsLocked(id uint, uuid string) bool {
	w := s.windows[uuid]
	return !s.stopped && w != nil && w.ID() == id
}
func (s *ConsoleService) CreateSession(ctx context.Context, uuid, key, revision string) (serialconsole.Credentials, error) {
	id, err := callerID(ctx)
	if err != nil {
		return serialconsole.Credentials{}, err
	}
	s.mu.Lock()
	owned := s.ownsLocked(id, uuid)
	s.mu.Unlock()
	if !owned {
		return serialconsole.Credentials{}, errors.New("This window does not own the VM console.")
	}
	c, err := s.GetCapabilities(uuid)
	if err != nil {
		return serialconsole.Credentials{}, err
	}
	if !c.Available {
		return serialconsole.Credentials{}, fmt.Errorf("%s", c.Reason)
	}
	if c.Revision != revision {
		return serialconsole.Credentials{}, errors.New("Console configuration changed. Refresh and reconnect.")
	}
	found := false
	for _, device := range c.Devices {
		if device.Key == key && device.Supported {
			found = true
		}
	}
	if !found {
		return serialconsole.Credentials{}, errors.New("The selected serial device is unavailable.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ownsLocked(id, uuid) {
		return serialconsole.Credentials{}, errors.New("Console window closed during connection.")
	}
	return s.bridge.Issue(strconv.FormatUint(uint64(id), 10), uuid, key, revision)
}
func (s *ConsoleService) Disconnect(ctx context.Context, sessionID string) error {
	id, err := callerID(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	registered := false
	for _, w := range s.windows {
		if w.ID() == id {
			registered = true
		}
	}
	s.mu.Unlock()
	if !registered {
		return errors.New("The console window is no longer registered.")
	}
	return s.bridge.Disconnect(strconv.FormatUint(uint64(id), 10), sessionID)
}
func (s *ConsoleService) removeWindow(uuid string, id uint) {
	s.mu.Lock()
	if w := s.windows[uuid]; w != nil && w.ID() == id {
		delete(s.windows, uuid)
	}
	s.mu.Unlock()
	s.bridge.CloseOwner(strconv.FormatUint(uint64(id), 10))
}
func (s *ConsoleService) ServiceShutdown() error {
	s.mu.Lock()
	s.stopped = true
	s.windows = make(map[string]consoleWindow)
	s.mu.Unlock()
	if s.bridge != nil {
		return s.bridge.Close()
	}
	return nil
}

// Wails debug records include full RPC arguments/results by default. Credentials
// are returned by CreateSession, so strip payload attributes at the logger too.
func consoleLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		switch a.Key {
		case "args", "result", "token", "data":
			return slog.Attr{}
		}
		return a
	}}))
}
