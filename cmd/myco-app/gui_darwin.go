//go:build darwin

package main

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// trayIcon is the myco symbol, black on transparent: macOS tints template
// images to match light and dark menu bars.
//
//go:embed build/darwin/tray-template.png
var trayIcon []byte

// runGUI opens the console as a regular Mac app (Dock icon, menu bar with
// Edit so copy and paste work) plus a menu-bar icon; closing the window
// hides it, and the app keeps running until quit.
func runGUI(h http.Handler, onLink func(string)) error {
	app := application.New(application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: h},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
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
		// No title bar: the traffic lights float over the sidebar, and the
		// page marks its top row as the drag area (--wails-draggable).
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 40,
		},
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
	// myco://add-hub?link=… (an app link's "open in the app" button): the
	// UI picks it up and offers to add the hub.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		if u := e.Context().URL(); strings.HasPrefix(u, "myco://") {
			onLink(u)
			show()
		}
	})

	appMenu := app.NewMenu()
	appMenu.AddRole(application.AppMenu)
	appMenu.AddRole(application.EditMenu)
	appMenu.AddRole(application.WindowMenu)
	app.Menu.Set(appMenu)

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
