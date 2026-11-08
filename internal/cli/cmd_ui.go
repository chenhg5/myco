package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenhg5/myco/internal/release"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/web"
)

func uiCommand() *Command {
	return &Command{
		Name:    "ui",
		Summary: "Open a local, read-only dashboard of the network",
		Long: `Serves the embedded web dashboard on 127.0.0.1 and proxies its requests to the hub with this
node's identity (the token never reaches the browser). With MYCO_ADMIN_TOKEN (or
--admin-token-file) the page can also switch to the admin view, which shows message contents
for every agent. Access requires the one-time key printed on start; requests with a foreign
Host header are rejected (DNS rebinding).`,
		Examples: []string{
			"myco ui --open",
			"myco ui --port 7781",
			"MYCO_ADMIN_TOKEN=... myco ui --open   # adds the admin view",
		},
		Flags: []Flag{
			{Name: "port", Kind: Int, Default: "7780", Help: "Local port"},
			{Name: "open", Kind: Bool, Help: "Open the dashboard in the default browser"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (enables the admin view)"},
		},
		Run: handler(func(e *env) error {
			hub, err := e.hubURL()
			if err != nil {
				return err
			}
			var agentC, adminC *client.Client
			if e.cfg.Token != "" {
				if agentC, err = client.New(hub, e.cfg.Token); err != nil {
					return err
				}
			}
			adminTok := e.in.Getenv(config.EnvAdminToken)
			if f := e.in.String("admin-token-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "", "cannot read --admin-token-file: %v", err)
				}
				adminTok = strings.TrimSpace(string(b))
			}
			if adminTok != "" {
				if adminC, err = client.New(hub, adminTok); err != nil {
					return err
				}
			}
			if agentC == nil && adminC == nil {
				return errorf("not_configured", ExitUsage, "run it where an agent is configured, or set MYCO_ADMIN_TOKEN",
					"no agent token or admin token available")
			}
			keyBytes := make([]byte, 16)
			_, _ = rand.Read(keyBytes)
			key := hex.EncodeToString(keyBytes)
			port := e.in.Int("port")
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return errorf("unavailable", ExitError, "pick another --port", "cannot listen on %s: %v", addr, err)
			}
			ui := &uiServer{hub: hub, agentName: e.cfg.Agent, agent: agentC, admin: adminC, key: key, port: port}
			srv := &http.Server{Handler: ui.routes(), ReadHeaderTimeout: 10 * time.Second}
			go ui.watchVersions(e.in.Ctx)
			url := fmt.Sprintf("http://127.0.0.1:%d/?key=%s", port, key)
			fmt.Fprintf(e.in.Stderr, "myco dashboard: %s\n(read-only; only this machine can open it; Ctrl-C to stop)\n", url)
			if e.in.Bool("open") {
				openBrowser(url)
			}
			go func() {
				<-e.in.Ctx.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
			}()
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		}),
	}
}

type uiServer struct {
	hub, agentName string
	agent, admin   *client.Client
	key            string
	port           int

	vmu        sync.RWMutex // versions, refreshed in the background
	hubVersion string
	hubKnown   bool
	latest     string
}

// watchVersions refreshes the hub's version and the latest release in the
// background (hourly), so /api/session never waits for the network.
func (u *uiServer) watchVersions(ctx context.Context) {
	for {
		if h, err := hubHealth(ctx, u.hub, 5*time.Second); err == nil {
			u.vmu.Lock()
			u.hubVersion, u.hubKnown, u.latest = h.ServerVersion, h.ServerVersion != "", h.LatestVersion
			u.vmu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

const uiCookie = "myco_ui"

func (u *uiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", u.session)
	mux.HandleFunc("GET /api/overview", u.overview)
	mux.HandleFunc("GET /api/threads/{id}", u.thread)
	mux.Handle("/", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DNS-rebinding guard: only our own loopback origin.
		host := r.Host
		if host != fmt.Sprintf("127.0.0.1:%d", u.port) && host != fmt.Sprintf("localhost:%d", u.port) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'")
		if k := r.URL.Query().Get("key"); k != "" {
			if subtle.ConstantTimeCompare([]byte(k), []byte(u.key)) != 1 {
				http.Error(w, "invalid key", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: uiCookie, Value: u.key, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		c, err := r.Cookie(uiCookie)
		if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(u.key)) != 1 {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Open the dashboard with the URL printed by `myco ui` (it contains a one-time key).")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (u *uiServer) pick(r *http.Request) (*client.Client, error) {
	if r.URL.Query().Get("as") == "admin" {
		if u.admin == nil {
			return nil, &client.Error{Code: "permission_denied", Message: "admin view needs MYCO_ADMIN_TOKEN when starting myco ui"}
		}
		return u.admin, nil
	}
	if u.agent != nil {
		return u.agent, nil
	}
	return u.admin, nil
}

func uiJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func uiErr(w http.ResponseWriter, err error) {
	ce := toError(err, "myco ui")
	status := http.StatusBadGateway
	switch ce.Exit {
	case ExitNotFound:
		status = http.StatusNotFound
	case ExitPermission:
		status = http.StatusForbidden
	case ExitUsage:
		status = http.StatusBadRequest
	}
	uiJSON(w, status, ce)
}

func (u *uiServer) session(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"hub": u.hub, "version": Version, "admin": u.admin != nil}
	u.vmu.RLock()
	if u.hubKnown {
		out["hub_version"] = u.hubVersion
	}
	if u.latest != "" {
		out["latest_version"] = u.latest
		out["update_available"] = release.Newer(u.latest, Version) || (u.hubKnown && release.Newer(u.latest, u.hubVersion))
	}
	u.vmu.RUnlock()
	if u.agent != nil {
		out["agent"] = u.agentName
	}
	uiJSON(w, http.StatusOK, out)
}

func (u *uiServer) overview(w http.ResponseWriter, r *http.Request) {
	c, err := u.pick(r)
	if err != nil {
		uiErr(w, err)
		return
	}
	ov, err := c.Overview(r.Context(), 100)
	if err != nil {
		uiErr(w, err)
		return
	}
	uiJSON(w, http.StatusOK, ov)
}

func (u *uiServer) thread(w http.ResponseWriter, r *http.Request) {
	c, err := u.pick(r)
	if err != nil {
		uiErr(w, err)
		return
	}
	msgs, err := c.Thread(r.Context(), r.PathValue("id"))
	if err != nil {
		uiErr(w, err)
		return
	}
	uiJSON(w, http.StatusOK, map[string]any{"messages": nonNilMsgs(msgs)})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
