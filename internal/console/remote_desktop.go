package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"crypto/rand"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/remotedesktop"
)

type remoteDesktop struct {
	mu          sync.Mutex
	component   *remotedesktop.Component
	host        *remoteDesktopHost
	revoked     map[string]bool
	generations map[string]uint64
	sessions    map[string]string // session id -> hub/page: an app cannot close another app's session
}
type remoteDesktopHost struct {
	scope, machine, key string
	cancel              context.CancelFunc
	done                chan struct{}
}

func (b *Backend) remoteComponent() *remotedesktop.Component {
	b.remoteDesktop.mu.Lock()
	defer b.remoteDesktop.mu.Unlock()
	if b.remoteDesktop.component == nil {
		home, _ := os.UserHomeDir()
		if b.Local != nil && b.Local.Home != "" {
			home = b.Local.Home
		}
		b.remoteDesktop.component = &remotedesktop.Component{Home: home}
	}
	return b.remoteDesktop.component
}
func (b *Backend) remoteDesktopRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /app/hubs/{id}/remote", b.withHubAndHubTimeout(10*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		var in struct {
			Action   string          `json:"action"`
			Page     string          `json:"page"`
			Instance string          `json:"instance"`
			Args     json.RawMessage `json:"args"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 80<<10)
		if !decode(w, r, &in) {
			return
		}
		if in.Page == "" || len(in.Page) > 100 || in.Instance == "" || len(in.Instance) > 80 {
			writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "remote desktop calls need a page"})
			return
		}
		scope := h.ID + "/" + in.Page + "/" + in.Instance
		comp := b.remoteComponent()
		result, err := b.remoteDesktopCall(r.Context(), c, h, scope, in.Action, in.Args, comp)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
}
func (b *Backend) remoteDesktopCall(ctx context.Context, c *client.Client, h Hub, scope, action string, args json.RawMessage, comp *remotedesktop.Component) (any, error) {
	b.remoteDesktop.mu.Lock()
	if b.remoteDesktop.revoked == nil {
		b.remoteDesktop.revoked = map[string]bool{}
	}
	if b.remoteDesktop.revoked[scope] && action != "close" {
		b.remoteDesktop.mu.Unlock()
		return nil, context.Canceled
	}
	if b.remoteDesktop.generations == nil {
		b.remoteDesktop.generations = map[string]uint64{}
	}
	generation := b.remoteDesktop.generations[scope]
	if action == "close" {
		b.remoteDesktop.revoked[scope] = true
		b.remoteDesktop.generations[scope]++
	}
	b.remoteDesktop.mu.Unlock()
	switch action {
	case "status":
		out := comp.Status()
		b.remoteDesktop.mu.Lock()
		h := b.remoteDesktop.host
		b.remoteDesktop.mu.Unlock()
		if h == nil || h.scope != scope {
			out.PairingCode = ""
		}
		return out, nil
	case "install":
		if err := comp.Install(ctx); err != nil {
			return nil, err
		}
		return comp.Status(), nil
	case "remove":
		b.stopRemoteHost(scope, true)
		return true, comp.Remove()
	case "devices":
		return c.RemoteDevices(ctx)
	case "enable":
		var in struct {
			Control bool `json:"control"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
		if b.Local == nil {
			return nil, errors.New("remote hosting requires the desktop App")
		}
		who, err := c.Whoami(ctx)
		if err != nil {
			return nil, err
		}
		localID := "app:" + rand.Text()
		if who.Device != nil {
			localID = "app:" + who.Device.ID
		}
		b.remoteDesktop.mu.Lock()
		busy := b.remoteDesktop.host != nil
		b.remoteDesktop.mu.Unlock()
		if busy {
			return nil, remotedesktop.ErrBusy
		}
		status, err := comp.Start(ctx, in.Control, nil)
		if err != nil {
			return nil, err
		}
		if in.Control {
			var capabilities struct {
				Control bool `json:"control"`
			}
			if e := comp.Call(ctx, "GET", "/status", nil, &capabilities); e != nil || !capabilities.Control {
				comp.Stop()
				return nil, errors.New("control_not_allowed")
			}
		}
		hostname, _ := os.Hostname()
		var reg struct {
			Key string `json:"host_key"`
		}
		err = c.RemoteRequest(ctx, "PUT", "machines/"+url.PathEscape(localID), "", remotedesktop.Device{Machine: localID, Name: hostname, OS: runtime.GOOS, Control: in.Control}, &reg)
		if err != nil {
			comp.Stop()
			return nil, err
		}
		run, cancel := context.WithCancel(context.Background())
		host := &remoteDesktopHost{scope: scope, machine: localID, key: reg.Key, cancel: cancel, done: make(chan struct{})}
		b.remoteDesktop.mu.Lock()
		if b.remoteDesktop.host != nil || b.remoteDesktop.generations[scope] != generation {
			b.remoteDesktop.mu.Unlock()
			cancel()
			comp.Stop()
			end, endCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.RemoteRequest(end, "DELETE", "machines/"+url.PathEscape(localID), reg.Key, nil, nil)
			endCancel()
			return nil, remotedesktop.ErrBusy
		}
		b.remoteDesktop.host = host
		b.remoteDesktop.mu.Unlock()
		go b.serveRemoteHost(run, c, host, comp)
		return status, nil
	case "disable", "close":
		b.stopRemoteHost(scope, false)
		if action == "close" {
			b.remoteDesktop.mu.Lock()
			ids := []string{}
			for id, owner := range b.remoteDesktop.sessions {
				if owner == scope {
					ids = append(ids, id)
					delete(b.remoteDesktop.sessions, id)
				}
			}
			b.remoteDesktop.mu.Unlock()
			for _, id := range ids {
				_ = c.RemoteRequest(ctx, "DELETE", "sessions/"+id, "", nil, nil)
			}
		}
		return true, nil
	case "connect":
		var in struct {
			Machine string              `json:"machine"`
			Offer   remotedesktop.Offer `json:"offer"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
		var out remotedesktop.Session
		if err := c.RemoteRequest(ctx, "POST", "sessions", "", in, &out); err != nil {
			return nil, err
		}
		b.remoteDesktop.mu.Lock()
		if b.remoteDesktop.sessions == nil {
			b.remoteDesktop.sessions = map[string]string{}
		}
		if b.remoteDesktop.generations[scope] != generation {
			b.remoteDesktop.mu.Unlock()
			end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.RemoteRequest(end, "DELETE", "sessions/"+out.ID, "", nil, nil)
			return nil, context.Canceled
		}
		b.remoteDesktop.sessions[out.ID] = scope
		b.remoteDesktop.mu.Unlock()
		return out, nil
	case "session", "disconnect":
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
		b.remoteDesktop.mu.Lock()
		owner := b.remoteDesktop.sessions[in.ID]
		b.remoteDesktop.mu.Unlock()
		if owner != scope {
			return nil, &client.Error{Status: 403, Code: "permission_denied", Message: "this session belongs to another app"}
		}
		method := "GET"
		if action == "disconnect" {
			method = "DELETE"
		}
		var out remotedesktop.Session
		err := c.RemoteRequest(ctx, method, "sessions/"+url.PathEscape(in.ID), "", nil, &out)
		if action == "disconnect" {
			b.remoteDesktop.mu.Lock()
			delete(b.remoteDesktop.sessions, in.ID)
			b.remoteDesktop.mu.Unlock()
		}
		return out, err
	}
	return nil, &client.Error{Status: 400, Code: "invalid_argument", Message: "unknown remote desktop operation"}
}
func (b *Backend) stopRemoteHost(scope string, all bool) {
	b.remoteDesktop.mu.Lock()
	h := b.remoteDesktop.host
	if h != nil && (all || h.scope == scope) {
		h.cancel()
	} else {
		h = nil
	}
	b.remoteDesktop.mu.Unlock()
	if h != nil {
		<-h.done
	}
}
func (b *Backend) serveRemoteHost(ctx context.Context, c *client.Client, h *remoteDesktopHost, comp *remotedesktop.Component) {
	defer close(h.done)
	defer func() {
		comp.Stop()
		b.remoteDesktop.mu.Lock()
		if b.remoteDesktop.host == h {
			b.remoteDesktop.host = nil
		}
		b.remoteDesktop.mu.Unlock()
		end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.RemoteRequest(end, "DELETE", "machines/"+url.PathEscape(h.machine), h.key, nil, nil)
	}()
	answers := map[string]map[string]any{}
	last := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !comp.Status().Running {
				return
			}
			var out struct {
				Sessions []remotedesktop.Session `json:"sessions"`
			}
			call, cancel := context.WithTimeout(ctx, 6*time.Second)
			err := c.RemoteRequest(call, "GET", "machines/"+url.PathEscape(h.machine)+"/requests", h.key, nil, &out)
			cancel()
			if err != nil {
				if time.Since(last) > 25*time.Second {
					return
				}
				continue
			}
			last = time.Now()
			var componentState struct {
				Sessions map[string]string `json:"sessions"`
			}
			if e := comp.Call(ctx, "GET", "/status", nil, &componentState); e != nil {
				return
			}
			live := map[string]bool{}
			for _, s := range out.Sessions {
				live[s.ID] = true
				if s.State == "closed" || s.State == "failed" {
					_ = comp.Call(ctx, "DELETE", "/sessions/"+s.ID, nil, nil)
					delete(answers, s.ID)
					continue
				}
				if s.State == "connected" {
					message, exists := componentState.Sessions[s.ID]
					if !exists {
						message = "connection_closed"
					}
					if message != "" {
						_ = c.RemoteRequest(ctx, "PUT", "machines/"+url.PathEscape(h.machine)+"/sessions/"+s.ID, h.key, map[string]any{"error": message}, nil)
					}
					continue
				}
				if s.State != "pending" || s.Offer == nil {
					continue
				}
				answer := answers[s.ID]
				if answer == nil {
					var a remotedesktop.Description
					e := comp.Call(ctx, "POST", "/sessions/"+s.ID, s.Offer, &a)
					answer = map[string]any{"answer": a}
					if e != nil {
						answer = map[string]any{"error": e.Error()}
					}
					answers[s.ID] = answer
				}
				_ = c.RemoteRequest(ctx, "PUT", "machines/"+url.PathEscape(h.machine)+"/sessions/"+s.ID, h.key, answer, nil)
			}
			for id := range answers {
				if !live[id] {
					_ = comp.Call(ctx, "DELETE", "/sessions/"+id, nil, nil)
					delete(answers, id)
				}
			}
		}
	}
}
