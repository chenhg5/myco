//go:build windows

package main

import (
	_ "embed"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

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
// app keeps running until quit from that icon. setRelaunch receives how to
// reopen the app after an update put a new exe in place. myco:// links are
// not registered.
func runGUI(h http.Handler, _ func(string), setRelaunch func(func())) error {
	app := application.New(application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		Icon:        appIcon,
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: h},
	})

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
		Windows:    application.WindowsWindow{Theme: application.SystemDefault},
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	show := func() {
		window.Show()
		window.Focus()
	}

	// Clicking the tray icon opens the window, right-clicking shows the menu
	// (the Windows way): the agents' status, open / restart the runner / quit.
	menu := newTrayMenu(app, h, show, app.Quit)
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("myco")
	tray.SetMenu(menu.menu)
	tray.OnClick(show)
	menu.keepFresh(30 * time.Second)

	// The path as started: an update renames this exe and puts the new one here.
	exe, exeErr := os.Executable()
	setRelaunch(func() {
		// Start the new exe, then quit this one.
		if exeErr == nil {
			cmd := exec.Command(exe)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008} // new process group, detached
			_ = cmd.Start()
		}
		app.Quit()
	})

	return app.Run()
}
