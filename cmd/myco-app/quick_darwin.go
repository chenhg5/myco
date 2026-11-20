//go:build darwin

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// quickKey calls up the quick panel from anywhere:
// ⌥Space is already hold-to-talk, ⌘Space Spotlight.
const quickKey = "Cmd+Shift+Space"

// quickW is the panel's width in points; its page reports the height.
const quickW = 640

// quickRadius is its corners' radius, as the page's card.
const quickRadius = 16

// quick is the small panel the shortcut calls up, Spotlight-like: one line
// to hand the workspace's superagent (Shroom) a thing, its latest answer
// under it, and a way into the full chat. A floating panel that does not
// bring the app forward; it goes away on Esc or when you click elsewhere.
type quick struct {
	mu    sync.Mutex
	win   *application.WebviewWindow
	open  func(hub, agent string, setup bool)
	h     int
	watch sync.Once
}

// qlog notes what the panel does on stderr (a release build's app.Logger
// discards everything): seen when the app is started with its output kept
// (a preview started with its log kept).
var qlog = log.New(os.Stderr, "myco quick: ", log.LstdFlags|log.Lmicroseconds)

func newQuick(app *application.App, open func(hub, agent string, setup bool)) *quick {
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "quick",
		Title:     "myco quick",
		Width:     quickW,
		Height:    120,
		Frameless: true,
		Hidden:    true,
		// (fixed size: the page sets the height)
		DisableResize:  true,
		BackgroundType: application.BackgroundTypeTransparent,
		URL:            "/?quick=1",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTransparent,
			// AppKit rounds the window itself: the page fills it edge to edge
			// (the webview is not transparent without private APIs, so a
			// card with a margin round it showed a white frame).
			CornerRadius: quickRadius,
			WindowClass:  application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true,
				FloatingPanel: true,
			},
			WindowLevel: application.MacWindowLevelFloating,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
		},
	})
	q := &quick{win: win, open: open, h: 120}
	// Clicking elsewhere puts it away, as Spotlight does.
	win.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { q.hide() })
	if err := app.GlobalShortcut.Register(quickKey, q.toggle); err != nil {
		app.Logger.Warn("quick panel shortcut not registered", "key", quickKey, "error", err)
	}
	return q
}

// toggle shows the panel (the page refreshes and takes the typing) or, when
// it is up, puts it away.
func (q *quick) toggle() {
	application.InvokeAsync(func() {
		if q.win.IsVisible() {
			q.hide()
			return
		}
		q.watch.Do(q.watchClicks)
		q.win.Center()
		q.win.Show()
		q.win.Focus()
		q.win.ExecJS(`window.dispatchEvent(new CustomEvent("quick-show"))`)
	})
}

func (q *quick) hide() {
	application.InvokeAsync(func() {
		if q.win.IsVisible() {
			q.win.Hide()
			q.win.ExecJS(`window.dispatchEvent(new CustomEvent("quick-hide"))`)
		}
	})
}

// handler serves the panel page's requests, everything else as before:
//
//	POST /app/quick/size {h}: its height (the width is fixed)
//	POST /app/quick/hide: put it away (Esc)
//	POST /app/quick/open {hub, agent, setup}: the full chat in the app, or
//	(setup) the dialog that sets a superagent up
func (q *quick) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/quick") {
			qlog.Printf("%s %s %s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/app/quick/") {
			next.ServeHTTP(w, r)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/app/quick/") {
		case "size":
			var in struct{ H int }
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.H < 40 || in.H > 900 {
				http.Error(w, "bad size", http.StatusBadRequest)
				return
			}
			q.mu.Lock()
			changed := in.H != q.h
			q.h = in.H
			q.mu.Unlock()
			if changed {
				application.InvokeAsync(func() { q.win.SetSize(quickW, in.H) })
			}
		case "hide":
			q.hide()
		case "open":
			var in struct {
				Hub, Agent string
				Setup      bool
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				qlog.Printf("open: bad body: %v", err)
				http.Error(w, "bad target", http.StatusBadRequest)
				return
			}
			qlog.Printf("open the chat: hub=%s agent=%s setup=%v", in.Hub, in.Agent, in.Setup)
			q.hide()
			if q.open != nil {
				q.open(in.Hub, in.Agent, in.Setup)
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}
