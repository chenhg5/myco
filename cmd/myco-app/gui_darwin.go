//go:build darwin

package main

import (
	_ "embed"
	"net/http"
	"os/exec"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// trayIcon is the myco symbol, black on transparent: macOS tints template
// images to match light and dark menu bars.
//
//go:embed build/darwin/tray-template.png
var trayIcon []byte

// guiShell tells the page it runs in the Mac window (hidden title bar).
const guiShell = "mac"

// runGUI opens the console as a regular Mac app (Dock icon, menu bar with
// Edit so copy and paste work) plus a menu-bar icon; closing the window
// hides it, and the app keeps running until quit. setRelaunch receives how
// to quit and reopen the app (after an update put a new bundle in place).
func runGUI(h http.Handler, onLink func(string), setRelaunch func(func())) error {
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

	setRelaunch(func() {
		if bundle := appBundle(); bundle != "" {
			// A detached shell reopens the (new) bundle once this one is gone.
			cmd := exec.Command("/bin/sh", "-c", "sleep 1; exec /usr/bin/open -n \"$0\"", bundle)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			_ = cmd.Start()
		}
		app.Quit()
	})

	return app.Run()
}
