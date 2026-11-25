package console

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/console/notices"
)

// Notices on this computer (package notices): the desktop app makes a
// watcher with its system notifications (StartNotices); the pages say what
// is on screen and read its state. In a browser (no watcher) the routes
// answer that this computer shows none.

// StartNotices follows every workspace's notices and shows them with p
// until the app quits. It returns the watcher (the app stops it).
func (b *Backend) StartNotices(p notices.Presenter) *notices.Watcher {
	state := ""
	if b.Store != nil {
		state = filepath.Join(filepath.Dir(b.Store.path), "console-notices.json")
	}
	w := notices.New(notices.Options{
		Hubs: b.noticeHubs, Present: p, StatePath: state,
		Lang: func() string {
			if l := b.UILang(); l != "" {
				return l
			}
			return "en"
		},
	})
	b.notices.Store(w)
	w.Start()
	return w
}

// noticeHubs: every workspace this App has, with a client opening its
// sealed messages like the pages' requests do.
func (b *Backend) noticeHubs() []notices.Hub {
	if b.Store == nil {
		return nil
	}
	hubs, err := b.Store.List()
	if err != nil {
		return nil
	}
	out := make([]notices.Hub, 0, len(hubs))
	for _, h := range hubs {
		out = append(out, notices.Hub{ID: h.ID, Name: h.Name, Feed: func(ctx context.Context) (notices.Feed, error) {
			c, err := b.client(h)
			if err != nil {
				return nil, err
			}
			b.withSealer(ctx, h.ID, c)
			return c, nil
		}})
	}
	return out
}

// noticeState is GET /app/notices: whether this computer shows notices,
// each workspace's state, and the badge.
type noticeState struct {
	// Available: this App shows system notifications (the desktop app).
	Available bool `json:"available"`
	notices.State
}

func (b *Backend) noticeRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/notices", func(w http.ResponseWriter, r *http.Request) {
		out := noticeState{State: notices.State{Hubs: map[string]notices.HubState{}}}
		if n := b.notices.Load(); n != nil {
			out.Available, out.State = true, n.State()
		}
		writeJSON(w, http.StatusOK, out)
	})
	// What is on screen: the window in front, and the conversation in it.
	m.HandleFunc("PUT /app/notices/focus", func(w http.ResponseWriter, r *http.Request) {
		var f notices.Focus
		if !decode(w, r, &f) {
			return
		}
		if n := b.notices.Load(); n != nil {
			n.SetFocus(r.Context(), f)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.HandleFunc("GET /app/hubs/{id}/notice-prefs", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		p, err := c.NoticePrefs(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/notice-prefs", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.NoticePrefs
		if !decode(w, r, &in) {
			return
		}
		p, err := c.SetNoticePrefs(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	// What still waits for the person on this workspace: the App's 待处理
	// list (the Dock badge counts the same).
	m.HandleFunc("GET /app/hubs/{id}/notices/open", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		open, badge, err := openNotices(r.Context(), c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.NoticeFeed{Notices: open, Badge: badge})
	}))
	m.HandleFunc("POST /app/hubs/{id}/notices/read", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.NoticeReadRequest
		if !decode(w, r, &in) {
			return
		}
		n, err := c.ReadNotices(r.Context(), in.Target, in.Through)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.NoticeReadResponse{Changed: n})
	}))
}

// noticeLister reads a person's notices (the hub client).
type noticeLister interface {
	Notices(ctx context.Context, after int64, limit int, wait time.Duration) (*api.NoticeFeed, error)
}

// openNoticePages bounds one read through the feed (notices are kept 30
// days; 50 pages of 200 is far more than a person gets).
const (
	openNoticePage  = 200
	openNoticePages = 50
)

// openNotices reads a person's whole notice feed and keeps what still needs
// them (open needs_you notices), newest first, with the hub's badge. The
// hub lists only forward from a seq, so the feed is read through.
func openNotices(ctx context.Context, l noticeLister) ([]api.Notice, int, error) {
	var open []api.Notice
	var after int64
	badge := 0
	for i := 0; i < openNoticePages; i++ {
		f, err := l.Notices(ctx, after, openNoticePage, 0)
		if err != nil {
			return nil, 0, err
		}
		badge = f.Badge
		for _, n := range f.Notices {
			if n.Tier == "needs_you" && n.ResolvedAt == nil {
				open = append(open, n)
			}
			if n.Seq > after {
				after = n.Seq
			}
		}
		if len(f.Notices) < openNoticePage {
			break
		}
	}
	slices.Reverse(open)
	return open, badge, nil
}
