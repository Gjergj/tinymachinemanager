//go:build console_smoke

// Run explicitly (not with -tags):
// TMM_NATIVE_SMOKE_UUID=<uuid> go run console_smoke_main.go consoleservice.go machinesservice.go networkingservice.go
// This opens the real console UI and attempts SAFE attachment. It sends no keys
// or guest commands. Close other serial viewers manually to test a free console.
// Set TMM_NATIVE_SMOKE_CLOSE=1 to close the console before quitting and check
// for late-window binding errors while the inventory window remains open.
package main

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/Gjergj/tinymachinemanager/pkg/serialconsole"
	"github.com/wailsapp/wails/v3/pkg/application"
	"libvirt.org/go/libvirt"
)

//go:embed all:frontend/dist
var smokeAssets embed.FS

type smokeDriver struct {
	console         *ConsoleService
	report          chan string
	eventDispatched atomic.Bool
}

func (d *smokeDriver) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if _, err := libvirt.EventAddTimeout(100, func(timer int) { d.eventDispatched.Store(true); _ = libvirt.EventRemoveTimeout(timer) }); err != nil {
		return err
	}
	go func() {
		time.Sleep(time.Second)
		uuid := os.Getenv("TMM_NATIVE_SMOKE_UUID")
		if err := d.console.OpenWindow(uuid); err != nil {
			d.report <- "open failed: " + err.Error()
			return
		}
		d.console.mu.Lock()
		win := d.console.windows[uuid].(*application.WebviewWindow)
		d.console.mu.Unlock()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				win.ExecJS(`(()=>{const state=document.querySelector('.console-heading .machine-state')?.textContent;const canvas=document.querySelector('.console-terminal canvas');if(!canvas||!['Connected','Connection unavailable','Disconnected'].includes(state)||window.__serialSmokeReported)return;window.__serialSmokeReported=true;const r=canvas.getBoundingClientRect();fetch('/console-smoke-report?result='+encodeURIComponent(JSON.stringify({state,origin:location.origin,message:document.querySelector('.console-notice')?.textContent,canvasWidth:r.width,canvasHeight:r.height})));})()`)
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func main() {
	if os.Getenv("TMM_NATIVE_SMOKE_UUID") == "" {
		fmt.Fprintln(os.Stderr, "Set TMM_NATIVE_SMOKE_UUID to a local guest UUID.")
		os.Exit(2)
	}
	if err := serialconsole.RegisterEventLoop(); err != nil {
		panic(err)
	}
	console := &ConsoleService{}
	report := make(chan string, 1)
	driver := &smokeDriver{console: console, report: report}
	app := application.New(application.Options{Name: "Serial console native smoke", Logger: consoleLogger(os.Stderr), Services: []application.Service{
		application.NewService(&MachinesService{}), application.NewService(&NetworkingService{}), application.NewService(console), application.NewService(driver),
	}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(smokeAssets), Middleware: func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/console-smoke-report" {
				select {
				case report <- r.URL.Query().Get("result"):
				default:
				}
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	}}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Serial console smoke — inventory", Width: 1160, Height: 760, URL: "/"})
	go func() {
		select {
		case result := <-report:
			fmt.Println("NATIVE_CONSOLE_RESULT", result)
			fmt.Println("LIBVIRT_GLIB_EVENT_DISPATCHED", driver.eventDispatched.Load())
			if os.Getenv("TMM_NATIVE_SMOKE_CLOSE") == "1" {
				console.mu.Lock()
				window := console.windows[os.Getenv("TMM_NATIVE_SMOKE_UUID")]
				console.mu.Unlock()
				if window != nil {
					window.Close()
				}
				time.Sleep(time.Second)
			}
		case <-time.After(25 * time.Second):
			fmt.Println("NATIVE_CONSOLE_TIMEOUT")
		}
		app.Quit()
	}()
	if err := app.Run(); err != nil {
		panic(err)
	}
}
