package main

import (
	"context"
	"embed"

	"log"
	"time"

	"github.com/Gjergj/tinymachinemanager/pkg/kvmcheck"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCheck()
	go func() {
		defer cancelCheck()
		report := kvmcheck.Check(checkCtx)
		log.Printf("KVM startup check: ready=%t", report.Ready)
		for _, result := range report.Checks {
			if result.Status == kvmcheck.Pass {
				continue
			}
			log.Printf("KVM startup check [%s] %s: %s", result.Status, result.ID, result.Message)
			if result.Output != "" {
				log.Printf("KVM startup check %s output: %s", result.ID, result.Output)
			}
			if result.Remediation != "" {
				log.Printf("KVM startup check %s suggested fix: %s", result.ID, result.Remediation)
			}
		}
	}()

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app := application.New(application.Options{
		Name:        "tinymachinemanager",
		Description: "Manage local Linux virtual machines with libvirt",
		Services: []application.Service{
			application.NewService(&MachinesService{}),
			application.NewService(&NetworkingService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Tiny Machine Manager",
		Width:     1160,
		Height:    760,
		MinWidth:  800,
		MinHeight: 560,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(24, 27, 31),
		URL:              "/",
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
