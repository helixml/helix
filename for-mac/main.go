package main

import (
	"embed"
	"log"

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
	// Create an instance of the app structure
	app := NewApp()

	// Create the application menu
	appMenu := createMenu(app)

	err := wails.Run(&options.App{
		Title:             "Helix",
		Width:             1200,
		Height:            800,
		MinWidth:          800,
		MinHeight:         600,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: true, // Keep running when window is closed
		BackgroundColour:  &options.RGBA{R: 18, G: 18, B: 20, A: 255}, // #121214
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
				HideTitle:                  false,
				HideTitleBar:               false,
				FullSizeContent:            true,
				UseToolbar:                 true,
				HideToolbarSeparator:       true,
			},
			About: &mac.AboutInfo{
				Title:   "Helix",
				Message: "GPU-accelerated AI development environment\n\nVersion 0.1.0\n\n\u00a9 2026 Helix ML",
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

	// App Menu (macOS)
	appMenu.Append(menu.AppMenu())

	// File Menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("New Session...", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) {
		app.OpenHelixUI()
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Open Helix UI", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) {
		app.OpenHelixUI()
	})

	// VM Menu
	vmMenu := appMenu.AddSubmenu("VM")
	vmMenu.AddText("Start VM", keys.CmdOrCtrl("r"), func(_ *menu.CallbackData) {
		if err := app.StartVM(); err != nil {
			log.Printf("Failed to start VM: %v", err)
		}
	})
	vmMenu.AddText("Stop VM", keys.Shift("r"), func(_ *menu.CallbackData) {
		if err := app.StopVM(); err != nil {
			log.Printf("Failed to stop VM: %v", err)
		}
	})
	vmMenu.AddSeparator()
	vmMenu.AddText("VM Settings...", keys.CmdOrCtrl(","), nil)
	vmMenu.AddSeparator()
	vmMenu.AddText("SSH to VM", nil, func(_ *menu.CallbackData) {
		log.Printf("SSH command: %s", app.GetSSHCommand())
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
