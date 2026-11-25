//go:build darwin || windows

package main

import (
	"net/url"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// pageWindows are pages (apps) opened in windows of their own, as an
// installed app opens: one per page, brought forward when it is open
// already. The window loads the App's own page in its page-window mode
// (web/src/app/PageWindow.tsx: ?pagewin=<hub>/<page>), which shows that one
// page with the same bridge, consent and origin as the panel.
type pageWindows struct {
	app  *application.App
	mu   sync.Mutex
	open map[string]*application.WebviewWindow
}

func newPageWindows(app *application.App) *pageWindows {
	return &pageWindows{app: app, open: map[string]*application.WebviewWindow{}}
}

// show opens the page's window, or brings it forward.
func (p *pageWindows) show(hub, page, title string) error {
	if title == "" {
		title = "myco"
	}
	key := hub + "/" + page
	application.InvokeSync(func() {
		p.mu.Lock()
		w := p.open[key]
		p.mu.Unlock()
		if w != nil {
			w.Show()
			w.Focus()
			return
		}
		prefs := application.MacWebviewPreferences{}
		prefs.FullscreenEnabled.Set(true) // (video in pages)
		w = p.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:      "page:" + key,
			Title:     title,
			Width:     1100,
			Height:    760,
			MinWidth:  360,
			MinHeight: 280,
			URL:       "/?pagewin=" + url.QueryEscape(key),
			Mac:       application.MacWindow{WebviewPreferences: prefs},
			Windows:   application.WindowsWindow{Theme: application.SystemDefault},
		})
		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			p.mu.Lock()
			if p.open[key] == w {
				delete(p.open, key)
			}
			p.mu.Unlock()
		})
		p.mu.Lock()
		p.open[key] = w
		p.mu.Unlock()
		w.Show()
		w.Focus()
	})
	return nil
}
