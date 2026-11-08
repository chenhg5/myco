//go:build windows

package main

import (
	_ "embed"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed build/windows/icon-256.png
var appIcon []byte

//go:embed build/windows/tray.png
var trayIcon []byte

// guiShell: Windows keeps its own title bar, so the page lays out as in a browser.
const guiShell = "browser"

// runGUI opens the console as a Windows app: a window with the system title
// bar, and a notification-area icon; closing the window hides it, and the
// app keeps running until quit from that icon. The app does not update
// itself on Windows (setRelaunch is not called), and myco:// links are not
// registered.
func runGUI(h http.Handler, _ func(string), _ func(func())) error {
	app := application.New(application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		Icon:        appIcon,
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: h},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "myco",
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		Windows:   application.WindowsWindow{Theme: application.SystemDefault},
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	show := func() {
		window.Show()
		window.Focus()
	}

	menu := app.NewMenu()
	menu.Add("Open myco").OnClick(func(*application.Context) { show() })
	menu.AddSeparator()
	menu.Add("Quit myco").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("myco")
	tray.SetMenu(menu)
	tray.OnClick(show)

	return app.Run()
}
