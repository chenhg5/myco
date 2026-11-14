//go:build darwin

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

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
	// The notch's island (made once the app exists) answers a few requests
	// of its own page; everything else goes to the app's backend.
	var isl *island
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isl != nil {
			isl.handler(h).ServeHTTP(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
	app := application.New(application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: assets},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// A face is chosen with the system's own panel, pictures only: the web
	// view's panel for <input type=file> shows every file.
	pickImage = func() (string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(true).
			AddFilter("Pictures", "*.png;*.jpg;*.jpeg;*.webp;*.gif").
			PromptForSingleSelection()
	}

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:  "main",
		Title: "myco",
		// Opens filling the screen, as Codex and Claude do; 1200×800 is the
		// size it takes when un-maximised.
		Width:      1200,
		Height:     800,
		MinWidth:   720,
		MinHeight:  480,
		StartState: application.WindowStateMaximised,
		URL:        "/",
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
	// While agents work, the notch grows to show them; a name in it opens
	// the app at that member.
	isl = newIsland(app, func(hub, agent string) {
		show()
		activateApp()
		detail, _ := json.Marshal(map[string]string{"hubId": hub, "agent": agent})
		window.ExecJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent("myco-open-member",{detail:%s}))`, detail))
	})
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

	// Clicking the menu-bar icon opens its menu (the Mac way): the agents'
	// status at a glance, then open / restart the runner / quit.
	menu := newTrayMenu(app, h, show, app.Quit)
	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("myco")
	tray.SetMenu(menu.menu)
	menu.keepFresh(30 * time.Second)

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
