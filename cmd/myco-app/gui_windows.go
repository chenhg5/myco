//go:build windows

package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

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
// app keeps running until quit from that icon. setRelaunch receives how the
// app quits after an update put a new exe in place and started it (the
// updater starts it and waits for it to report in, so a new version that
// cannot start leaves this one running). myco:// links are not registered.
func runGUI(h http.Handler, _ func(string), setRelaunch func(func())) error {
	// Whether the window's page asked for anything yet: when WebView2 cannot
	// start (its process crashes at once on some machines), nothing ever
	// comes, and the window would stay black (pageWatch).
	var seen atomic.Bool
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(true)
		h.ServeHTTP(w, r)
	})
	opts := application.Options{
		Name:        "myco",
		Description: "Manage your myco hubs and agents",
		Icon:        appIcon,
		// Served in-process to the webview: no network port is opened.
		Assets: application.AssetOptions{Handler: page},
	}
	compat := compatMode()
	if compat {
		// Chosen after WebView2 failed to start here: its sandbox off, for
		// this app only (not the user-wide WEBVIEW2_* variable, which every
		// app on the computer would take).
		opts.Windows.AdditionalBrowserArgs = []string{"--no-sandbox"}
	}
	app := application.New(opts)

	// Links from the pages open in the default browser.
	openURL = func(u string) error { return app.Browser.OpenURL(u) }

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
	// StartState alone is not always kept: Windows takes the first
	// ShowWindow as the launcher says (a shortcut's "normal window"), so it
	// is maximised once more when it first shows. After that it stays as
	// the person leaves it.
	var maximised sync.Once
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		maximised.Do(func() {
			if !window.IsMaximised() {
				window.Maximise()
			}
		})
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	show := func() {
		window.Show()
		window.Focus()
	}
	setFullscreen = func(on bool) {
		application.InvokeAsync(func() {
			if on && !window.IsFullscreen() {
				window.Fullscreen()
			} else if !on && window.IsFullscreen() {
				window.UnFullscreen()
			}
		})
	}
	// Notices: a toast for what needs the person; a click opens the App
	// where it leads.
	setUpNotices(app, func(hub, target, actor string) {
		show()
		detail, _ := json.Marshal(map[string]string{"hubId": hub, "target": target, "actor": actor})
		window.ExecJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent("myco-open-notice",{detail:%s}))`, detail))
	})

	// Clicking the tray icon opens the window, right-clicking shows the menu
	// (the Windows way): the agents' status, open / restart the runner / quit.
	menu := newTrayMenu(app, h, show, app.Quit)
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("myco")
	tray.SetMenu(menu.menu)
	tray.OnClick(show)
	menu.keepFresh(30 * time.Second)

	// The new version is already running (console.Updater.handOver): quit.
	setRelaunch(app.Quit)

	// The path as started, for a restart in compatibility mode.
	exe, exeErr := os.Executable()

	go pageWatch(&seen, compat, h, window, func(u string) error { return app.Browser.OpenURL(u) }, func() {
		if exeErr == nil {
			cmd := exec.Command(exe)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008}
			_ = cmd.Start()
		}
		app.Quit()
	})

	return app.Run()
}

// pageWait is how long the window's page may take to ask for anything.
const pageWait = 25 * time.Second

// pageWatch notices a window whose page never loaded (WebView2 failing to
// start, as with its 157 runtime on some computers) and, instead of leaving
// it black, offers: open myco in the browser, or restart in compatibility
// mode (WebView2's sandbox off for this app; offered once).
func pageWatch(seen *atomic.Bool, compat bool, h http.Handler, window *application.WebviewWindow, open func(string) error, relaunch func()) {
	time.Sleep(pageWait)
	if seen.Load() {
		return
	}
	zh := systemPrefersChinese()
	text := "myco 的窗口没能显示出来：系统的 WebView2 组件（显示窗口内容的部分）启动失败。\n\n" +
		"「是」：在浏览器里打开 myco（推荐）。\n「否」：用兼容模式重启（只对 myco 关掉 WebView2 的沙箱）。\n「取消」：再等等。"
	flags := uintptr(0x3) // yes / no / cancel
	if compat {
		text = "myco 的窗口在兼容模式下也没能显示出来：系统的 WebView2 组件启动失败。\n\n「确定」：在浏览器里打开 myco。"
		flags = 0x1 // ok / cancel
	}
	title := "myco"
	if !zh {
		text = "The myco window could not show: Windows' WebView2 component (which draws it) failed to start.\n\n" +
			"Yes: open myco in your browser (recommended).\nNo: restart in compatibility mode (WebView2's sandbox off, for myco only).\nCancel: keep waiting."
		if compat {
			text = "The myco window could not show in compatibility mode either: Windows' WebView2 component failed to start.\n\nOK: open myco in your browser."
		}
	}
	switch messageBox(title, text, flags|0x30|0x10000|0x40000) { // warning, foreground, topmost
	case 6, 1: // yes, ok
		u, err := serveLoopback(h)
		if err != nil {
			messageBox(title, err.Error(), 0x10)
			return
		}
		window.Hide()
		_ = open(u)
	case 7: // no
		if err := setCompatMode(true); err == nil {
			relaunch()
		}
	}
}

// serveLoopback serves the console on a loopback port with a one-time key
// (as myco-app --serve and the Linux app do) and returns its address.
func serveLoopback(h http.Handler) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	k := hex.EncodeToString(b)
	go func() { _ = http.Serve(ln, guarded(h, port, k)) }()
	return fmt.Sprintf("http://127.0.0.1:%d/?key=%s", port, k), nil
}

// compatPath marks compatibility mode (%APPDATA%\myco\app-compat).
func compatPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "myco", "app-compat")
}

func compatMode() bool {
	if os.Getenv("MYCO_APP_COMPAT") == "1" {
		return true
	}
	p := compatPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func setCompatMode(on bool) error {
	p := compatPath()
	if p == "" {
		return errors.New("no config directory")
	}
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("WebView2 sandbox off for myco (chosen after the window failed to show); delete this file to turn it back on\n"), 0o600)
}

var messageBoxW = user32.NewProc("MessageBoxW")

// messageBox shows a system message box (no window needed) and returns the
// button pressed.
func messageBox(title, text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := messageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

// quitRunning is for Linux, where the app has no tray menu to quit from.
func quitRunning() error { return errors.New("quit the app from its menu") }
