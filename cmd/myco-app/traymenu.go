//go:build darwin || windows

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The menu-bar (macOS) or tray (Windows) menu: how the agents are doing at
// a glance, then open, restart this computer's runner, quit. It follows the
// system language and refreshes in the background.

// trayMenu is the menu and the items it updates.
type trayMenu struct {
	menu    *application.Menu
	summary *application.MenuItem
	down    *application.MenuItem
	zh      trayText
	h       http.Handler
	mu      sync.Mutex
}

// newTrayMenu builds the menu. show opens the window; quit quits.
func newTrayMenu(app *application.App, h http.Handler, show, quit func()) *trayMenu {
	zh := trayText(systemPrefersChinese())
	t := &trayMenu{menu: app.NewMenu(), zh: zh, h: h}
	t.summary = t.menu.Add(zh.s("正在读取…", "Loading…")).SetEnabled(false)
	t.down = t.menu.Add("").SetEnabled(false).SetHidden(true)
	t.menu.AddSeparator()
	t.menu.Add(zh.s("打开 myco", "Open myco")).OnClick(func(*application.Context) { show() })
	t.menu.Add(zh.s("重启本机 runner（让本机节点重新上线）", "Restart this computer's runner (bring its nodes back online)")).OnClick(func(*application.Context) {
		go func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/local/runner", nil))
			time.Sleep(3 * time.Second)
			t.refresh()
		}()
	})
	t.menu.AddSeparator()
	t.menu.Add(zh.s("退出 myco", "Quit myco")).OnClick(func(*application.Context) { quit() })
	return t
}

// refresh reads the status and updates the menu.
func (t *trayMenu) refresh() {
	st := readTrayStatus(t.h)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.summary.SetLabel(t.zh.summary(st))
	if len(st.Down) > 0 {
		t.down.SetLabel(t.zh.s("连不上：", "Can't reach: ") + strings.Join(st.Down, t.zh.s("、", ", "))).SetHidden(false)
	} else {
		t.down.SetHidden(true)
	}
	t.menu.Update()
}

// keepFresh refreshes now and then every interval, for as long as the app runs.
func (t *trayMenu) keepFresh(interval time.Duration) {
	go func() {
		for {
			t.refresh()
			time.Sleep(interval)
		}
	}()
}
