package main

import (
	"embed"
	"io/fs"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/agentsafe/agentsafe/internal/applog"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Start app-level file logging before anything else so even early startup
	// failures are recorded. A failure here must not stop the app launching.
	if err := applog.Init(""); err != nil {
		println("applog init failed:", err.Error())
	}

	app := NewApp()
	// Inject the SPA assets so the popout bridge can serve the frontend to
	// detached windows. A failure here only disables popout, not the app.
	if dist, err := fs.Sub(assets, "frontend/dist"); err != nil {
		applog.Warn("popout assets unavailable", "err", err)
	} else {
		app.assets = dist
	}

	err := wails.Run(&options.App{
		Title:     "agentsafe",
		Width:     1180,
		Height:    780,
		MinWidth:  900,
		MinHeight: 600,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
