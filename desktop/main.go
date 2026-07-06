package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if runWintunRepairCommandIfRequested(os.Args[1:]) {
		return
	}
	if runMacOSHelperAdminCommandIfRequested(os.Args[1:]) {
		return
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "NexTunnel",
		Width:     1240,
		Height:    820,
		MinWidth:  1080,
		MinHeight: 720,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}
