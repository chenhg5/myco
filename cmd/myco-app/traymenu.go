//go:build darwin || windows

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The menu-bar (macOS) or tray (Windows) menu: how the nodes are doing at a
// glance (who is at work and on what), then open, wake this computer's
// nodes, quit. It follows the system language and refreshes in the
// background.

// trayBusyLines is how many busy nodes the menu lists by name.
const trayBusyLines = 5

// trayMenu is the menu and the items it updates.
type trayMenu struct {
	menu    *application.Menu
	summary *application.MenuItem
	busy    [trayBusyLines]*application.MenuItem
	more    *application.MenuItem
	down    *application.MenuItem
	wake    *application.MenuItem
	update  *application.MenuItem
	// checked is when the latest release was last looked up.
	checked time.Time
	zh      trayText
	h       http.Handler
	mu      sync.Mutex
	waking  bool
}

// newTrayMenu builds the menu. show opens the window; quit quits.
func newTrayMenu(app *application.App, h http.Handler, show, quit func()) *trayMenu {
	zh := trayText(systemPrefersChinese())
	t := &trayMenu{menu: app.NewMenu(), zh: zh, h: h}
	t.summary = t.menu.Add(zh.s("正在读取…", "Loading…")).SetEnabled(false)
	// Busy nodes: clicking one opens the app.
	for i := range t.busy {
		t.busy[i] = t.menu.Add("").SetHidden(true).OnClick(func(*application.Context) { show() })
	}
	t.more = t.menu.Add("").SetEnabled(false).SetHidden(true)
	t.down = t.menu.Add("").SetEnabled(false).SetHidden(true)
	t.menu.AddSeparator()
	// A newer release: opening the window shows the update (sidebar).
	t.update = t.menu.Add("").SetHidden(true).OnClick(func(*application.Context) { show() })
	t.menu.Add(zh.s("打开 myco", "Open myco")).OnClick(func(*application.Context) { show() })
	t.wake = t.menu.Add(zh.wakeLabel()).OnClick(func(*application.Context) { go t.wakeNodes() })
	t.menu.AddSeparator()
	t.menu.Add(zh.s("退出 myco", "Quit myco")).OnClick(func(*application.Context) { quit() })
	return t
}

// wakeNodes restarts this computer's runner, which brings its nodes back
// online, and says how it went in the menu for a moment.
func (t *trayMenu) wakeNodes() {
	t.mu.Lock()
	if t.waking {
		t.mu.Unlock()
		return
	}
	t.waking = true
	t.wake.SetLabel(t.zh.s("正在唤醒…", "Waking…")).SetEnabled(false)
	t.menu.Update()
	t.mu.Unlock()

	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/local/runner", nil))
	ok := rec.Code < 300
	time.Sleep(3 * time.Second)
	t.refresh()

	t.mu.Lock()
	if ok {
		t.wake.SetLabel(t.zh.s("已唤醒 ✓", "Awake ✓"))
	} else {
		t.wake.SetLabel(t.zh.s("没能唤醒，打开 myco 看看", "Couldn't wake them; open myco to see why"))
	}
	t.menu.Update()
	t.mu.Unlock()
	time.Sleep(4 * time.Second)
	t.mu.Lock()
	t.wake.SetLabel(t.zh.wakeLabel()).SetEnabled(true)
	t.waking = false
	t.menu.Update()
	t.mu.Unlock()
}

// trayUpdateEvery is how often the menu looks for a newer release.
const trayUpdateEvery = 6 * time.Hour

// refresh reads the status (and, now and then, the latest release) and
// updates the menu.
func (t *trayMenu) refresh() {
	st := readTrayStatus(t.h)
	var latest string
	check := time.Since(t.checked) >= trayUpdateEvery
	if check {
		latest = readTrayUpdate(t.h)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if check {
		t.checked = time.Now()
		if latest != "" {
			t.update.SetLabel(t.zh.updateLine(latest)).SetHidden(false)
		} else {
			t.update.SetHidden(true)
		}
	}
	t.summary.SetLabel(t.zh.summary(st))
	for i, it := range t.busy {
		if i < len(st.Busy) {
			it.SetLabel(t.zh.nodeLine(st.Busy[i])).SetHidden(false)
		} else {
			it.SetHidden(true)
		}
	}
	if extra := len(st.Busy) - trayBusyLines; extra > 0 {
		t.more.SetLabel(fmt.Sprintf(t.zh.s("   还有 %d 个在忙", "   and %d more busy"), extra)).SetHidden(false)
	} else {
		t.more.SetHidden(true)
	}
	if len(st.Down) > 0 {
		t.down.SetLabel(t.zh.downLine(st.Down)).SetHidden(false)
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
