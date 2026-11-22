//go:build darwin || windows

package main

import (
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/chenhg5/myco/internal/console/notices"
)

// systemNotices is notices.Presenter with the system's own notifications
// (macOS's Notification Centre, Windows' toasts) and, on the Mac, the
// number on the Dock icon (badge_*.go). A click on a banner opens the App
// where it leads.
type systemNotices struct {
	ns    *notifications.NotificationService
	badge appBadge

	mu      sync.Mutex
	allowed bool
	asked   bool
	shown   int
	badgeMu sync.Mutex // one Dock update at a time
}

// setUpNotices adds the services to app and, once it has started, follows
// the workspaces' notices; open is called with where a clicked banner
// leads.
func setUpNotices(app *application.App, open func(hub, target, actor string)) {
	if startNotices == nil {
		return
	}
	p := &systemNotices{ns: notifications.New(), badge: newAppBadge(app), shown: -1}
	app.RegisterService(application.NewService(p.ns))
	p.ns.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		hub, _ := r.Response.UserInfo["hub"].(string)
		target, _ := r.Response.UserInfo["target"].(string)
		actor, _ := r.Response.UserInfo["actor"].(string)
		if hub != "" {
			application.InvokeAsync(func() { open(hub, target, actor) })
		}
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		startNotices(p)
	})
}

// ready asks the system for permission the first time something is to be
// shown (not at launch: the question then comes with a reason).
func (p *systemNotices) ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.allowed {
		return true
	}
	if ok, err := p.ns.CheckNotificationAuthorization(); err == nil && ok {
		p.allowed = true
		return true
	}
	if p.asked {
		return false
	}
	p.asked = true
	ok, err := p.ns.RequestNotificationAuthorization()
	if err != nil {
		log.Printf("notifications: %v", err)
	}
	p.allowed = ok
	return ok
}

func (p *systemNotices) Show(t notices.Toast) error {
	if !p.ready() {
		return nil
	}
	o := notifications.NotificationOptions{
		ID: t.ID, Title: t.Title, Subtitle: t.Subtitle, Body: t.Body, ThreadID: t.ID,
		Data: map[string]interface{}{"hub": t.Hub, "target": t.Target, "actor": t.Actor},
	}
	if !t.Sound {
		o.Sound = &notifications.NotificationSound{Silent: true}
	}
	// One banner per conversation: the newest replaces the one before.
	_ = p.ns.RemoveDeliveredNotification(t.ID)
	return p.ns.SendNotification(o)
}

func (p *systemNotices) Withdraw(ids ...string) {
	for _, id := range ids {
		_ = p.ns.RemoveDeliveredNotification(id)
	}
}

func (p *systemNotices) Badge(n int) {
	p.mu.Lock()
	same := p.shown == n
	p.shown = n
	p.mu.Unlock()
	if !same && p.badge != nil {
		// Not on the main thread: the Dock service dispatch_syncs onto
		// it itself, and doing that from the main thread traps (the App
		// crashed at launch). The newest number wins.
		go func() {
			p.badgeMu.Lock()
			defer p.badgeMu.Unlock()
			p.mu.Lock()
			n := p.shown
			p.mu.Unlock()
			p.badge(n)
		}()
	}
}
