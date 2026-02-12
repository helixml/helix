package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	appMenu := createMenu(app)

	title := "Helix Desktop (Beta)"
	if runtime.GOOS == "darwin" {
		title = "Helix for Mac (Beta)"
	}

	err := wails.Run(&options.App{
		Title:             title,
		Width:             1200,
		Height:            800,
		MinWidth:          800,
		MinHeight:         600,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 18, G: 18, B: 20, A: 255},
		Menu:              appMenu,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               false,
				FullSizeContent:            true,
				UseToolbar:                 true,
				HideToolbarSeparator:       true,
			},
			About: &mac.AboutInfo{
				Title:   "Helix Desktop",
				Message: "GPU-accelerated AI development environment\n\nVersion 0.1.0-beta\n\n\u00a9 2026 Helix ML",
			},
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}

func createMenu(app *App) *menu.Menu {
	appMenu := menu.NewMenu()

	// App Menu (macOS — ignored on Windows)
	appMenu.Append(menu.AppMenu())

	// Edit Menu (required for Cmd+C/V/X/A to work in webview)
	appMenu.Append(menu.EditMenu())

	// File Menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("New Session...", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) {
		app.OpenHelixUI()
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Open Helix UI", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) {
		app.OpenHelixUI()
	})

	// Environment Menu
	envMenu := appMenu.AddSubmenu("Environment")
	envMenu.AddText("Start", keys.CmdOrCtrl("r"), func(_ *menu.CallbackData) {
		if err := app.StartVM(); err != nil {
			log.Printf("Failed to start: %v", err)
		}
	})
	envMenu.AddText("Stop", keys.Shift("r"), func(_ *menu.CallbackData) {
		if err := app.StopVM(); err != nil {
			log.Printf("Failed to stop: %v", err)
		}
	})
	envMenu.AddSeparator()
	envMenu.AddText("Settings...", keys.CmdOrCtrl(","), nil)
	envMenu.AddSeparator()
	envMenu.AddText("SSH Access", nil, func(_ *menu.CallbackData) {
		log.Printf("Access: %s", app.GetSSHCommand())
	})

	// View Menu
	viewMenu := appMenu.AddSubmenu("View")
	viewMenu.AddText("Reload", keys.CmdOrCtrl("shift+r"), nil)
	viewMenu.AddSeparator()
	viewMenu.AddText("Toggle Fullscreen", keys.Key("f11"), nil)

	// Help Menu
	helpMenu := appMenu.AddSubmenu("Help")
	helpMenu.AddText("Documentation", nil, func(_ *menu.CallbackData) {
		openBrowser("https://docs.helix.ml")
	})
	helpMenu.AddText("Report Issue", nil, func(_ *menu.CallbackData) {
		openBrowser("https://github.com/helixml/helix/issues")
	})
	helpMenu.AddSeparator()
	helpMenu.AddText("Check for Updates...", nil, nil)

	return appMenu
}
