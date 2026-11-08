//go:build darwin

package main

import (
	_ "embed"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// trayIcon is the myco symbol, black on transparent: macOS tints template
// images to match light and dark menu bars.
//
//go:embed build/darwin/tray-template.png
var trayIcon []byte

// runGUI opens the console as a menu-bar app: no Dock icon, a tray menu,
// and a window that hides instead of closing (quit from the menu).
func runGUI(h http.Handler) error {
	app := application.New(application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: h},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "myco",
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	show := func() {
		window.Show()
		window.Focus()
	}
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { show() })

	menu := app.NewMenu()
	menu.Add("Open myco").OnClick(func(*application.Context) { show() })
	menu.AddSeparator()
	menu.Add("Quit myco").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("myco")
	tray.SetMenu(menu)
	tray.OnClick(show)

	return app.Run()
}
