// Package hubserver is the composition root of the hub: it wires storage,
// notifier, use cases and the HTTP adapter together and runs the server.
package hubserver

import (
	"bufio"
	"context"
	"errors"
	"github.com/chenhg5/myco/internal/templates"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/blob"
	"github.com/chenhg5/myco/internal/infra/jsonschema"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
	"github.com/chenhg5/myco/internal/infra/tunnel"
	"github.com/chenhg5/myco/internal/infra/wake"
	"github.com/chenhg5/myco/internal/release"
	"github.com/chenhg5/myco/internal/transport/httpapi"
	"github.com/chenhg5/myco/internal/transport/ingress"
	// Automations run in their own time zone, also where the system has
	// no zone database (Windows).
	_ "time/tzdata"
)

// Options configures the hub.
type Options struct {
	Addr         string
	DBPath       string
	AdminToken   string
	Policy       app.MessagePolicy
	OnlineWindow time.Duration
	Dispatch     app.DispatchPolicy
	// FilesDir stores attachment contents (default: <db dir>/files).
	FilesDir string
	// Files bounds uploads (zero value: DefaultFilePolicy).
	Files app.FilePolicy
	// GitHubWebhookSecret enables POST /v1/ingress/github.
	GitHubWebhookSecret string
	// GitHubReplyFrom restricts which GitHub logins can answer messages.
	GitHubReplyFrom []string
	Log             *slog.Logger
	// Ready, if set, receives the bound address once listening.
	Ready func(addr string)
	// Tunnel, if set, publishes the hub (listening on localhost) at a
	// public URL, which then becomes its public URL.
	Tunnel tunnel.Tunnel
	// Version is this hub's myco build, reported at /v1/health.
	Version string
	// UpdateCheck, if set, looks for newer releases in the background
	// (Run starts it); the result is reported at /v1/health.
	UpdateCheck *release.Checker
	// Apps, if set, are the official apps every workspace offers, fetched
	// in the background as they are published (Run starts it); else the
	// ones the binary carries.
	Apps *templates.Catalog
	// PathPrefix is where this hub is served under another's address
	// ("/w/<name>": one workspace of several); "" for the hub itself.
	PathPrefix string
	// Upgrade, if set, brings the hub's myco to a release and restarts it
	// (POST /v1/admin/upgrade).
	Upgrade func(ctx context.Context, version string) (api.HubUpgrade, error)
}

// AutomationTick is how often each workspace looks for automations due.
const AutomationTick = 20 * time.Second

// WorkflowTick is how often each workspace checks its workflow runs when
// no answer kicked it sooner (timeouts, decisions that expire).
const WorkflowTick = app.WorkflowTick

// Hub is an assembled hub: the HTTP handler plus background workers.
type Hub struct {
	Handler http.Handler
	// Server is the HTTP adapter (Handler), for the workspace router.
	Server     *httpapi.Server
	Dispatcher *app.Dispatcher
	Files      *app.FileService
	Traces     *app.TraceService
	// Automations runs the workspace's automations (Run).
	Automations *app.AutomationService
	// Workflows walks the workspace's workflow runs (Run).
	Workflows *app.WorkflowService
	Close     func() error
	// SetPublicURL changes the URL used in join links (e.g. a tunnel's).
	SetPublicURL func(string)
	// SetLAN makes links created from this machine use its LAN address.
	SetLAN func(bool)
}

// DefaultWakers are the push delivery modes shipped with myco.
func DefaultWakers() app.Wakers {
	return app.NewWakers(wake.Webhook{}, wake.ClaudeRoutine{}, wake.GitHubDispatch{}, wake.GitHubIssue{})
}

// Build assembles the hub. Call hub.Dispatcher.Run in the background.
func Build(o Options) (*Hub, error) {
	db, err := sqlite.Open(o.DBPath)
	if err != nil {
		return nil, err
	}
	agentsRepo := sqlite.NewAgentRepo(db)
	msgRepo := sqlite.NewMessageRepo(db)
	ids, clock := security.IDs{}, app.SystemClock{}
	wakers := DefaultWakers()
	agents := app.NewAgentService(agentsRepo, security.Tokens{}, ids, clock, o.OnlineWindow, wakers)
	agents.SetAvatarStore(agentsRepo)
	messages := app.NewMessageService(agentsRepo, msgRepo, notify.NewMemory(), ids, clock, o.Policy)
	// A session that waited in the last 30 minutes is most likely still open
	// in front of someone (a desktop app or a terminal) between two waits:
	// the runner keeps away from it, so it is not resumed in the background
	// behind that open window. A closed session is taken over after that.
	messages.SetPresence(app.NewPresence(clock, 30*time.Minute))
	enroll := app.NewEnrollmentService(sqlite.NewEnrollmentRepo(db), agentsRepo, security.Tokens{}, ids, clock)
	aliasRepo := sqlite.NewAliasRepo(db)
	if n, err := aliasRepo.DeleteDangling(context.Background()); err != nil {
		db.Close()
		return nil, err
	} else if n > 0 && o.Log != nil {
		o.Log.Info("removed aliases whose agent no longer exists", "count", n)
	}
	agents.SetAliases(aliasRepo)
	messages.SetAliases(aliasRepo)
	enroll.SetAliases(aliasRepo)
	dispatcher := app.NewDispatcher(agentsRepo, msgRepo, wakers, clock, o.Dispatch, o.Log)
	messages.SetObserver(dispatcher)
	var ing []httpapi.Ingress
	if o.GitHubWebhookSecret != "" {
		ing = append(ing, &ingress.GitHub{Secret: o.GitHubWebhookSecret, AllowedAuthors: o.GitHubReplyFrom})
	}
	filesDir := o.FilesDir
	if filesDir == "" {
		if o.DBPath == ":memory:" {
			if filesDir, err = os.MkdirTemp("", "myco-files-"); err != nil {
				return nil, err
			}
		} else {
			filesDir = filepath.Join(filepath.Dir(o.DBPath), "files")
		}
	}
	policy := o.Files
	if policy.MaxSize <= 0 {
		policy = app.DefaultFilePolicy()
	}
	fileRepo := sqlite.NewFileRepo(db)
	files := app.NewFileService(fileRepo, blob.FS{Dir: filesDir}, ids, clock, policy, o.Log)
	messages.SetFiles(fileRepo)
	srv := httpapi.New(agents, messages, enroll, o.AdminToken, o.Log, ing...)
	srv.SetResume(app.NewResumeService(sqlite.NewResumeRepo(db), agentsRepo, security.Tokens{}, clock))
	srv.SetFiles(files)
	srv.SetPublicURL(o.Dispatch.PublicURL)
	srv.SetPathPrefix(o.PathPrefix)
	srv.SetUpgrade(o.Upgrade)
	var latest func() string
	if o.UpdateCheck != nil {
		latest = o.UpdateCheck.Latest
	}
	srv.SetVersion(o.Version, latest)
	overview := app.NewOverviewService(agentsRepo, msgRepo, clock, o.OnlineWindow)
	overview.SetPresence(messages.Presence())
	srv.SetOverview(overview)
	machineSvc := app.NewMachineService(sqlite.NewMachineRepo(db), agentsRepo, messages, ids, clock, o.OnlineWindow)
	machineSvc.SetRemovalDeps(agents, enroll)
	srv.SetMachines(machineSvc)
	consoleSvc := app.NewConsoleService(sqlite.NewConsoleTokenRepo(db), security.Tokens{}, ids, clock)
	consoleSvc.SetPairings(sqlite.NewConsolePairingRepo(db))
	srv.SetConsole(consoleSvc)
	// Members: the people using this hub. An existing hub gets an owner who
	// owns everything already there.
	audit := app.NewAuditService(sqlite.NewAuditRepo(db), ids, clock, o.Log)
	members := app.NewMemberService(sqlite.NewMemberRepo(db), consoleSvc, audit, ids, clock)
	if _, err := members.Bootstrap(context.Background(), o.Log); err != nil {
		db.Close()
		return nil, err
	}
	srv.SetMembers(members, audit)
	// New nodes of a plain member are private to them; the hub's owner and
	// admins add nodes for the team.
	defaultVisibility := func(ctx context.Context, owner string) domain.Visibility {
		if owner == "" {
			return domain.VisibilityTeam
		}
		if m, err := members.Get(ctx, owner); err == nil && !m.Role.Manages() {
			return domain.VisibilityPrivate
		}
		return domain.VisibilityTeam
	}
	agents.SetDefaultVisibility(defaultVisibility)
	projectRepo := sqlite.NewProjectRepo(db)
	agents.SetProjects(projectRepo)
	messages.SetProjects(projectRepo)
	projectSvc := app.NewProjectService(projectRepo, agentsRepo, ids, clock)
	projectSvc.SetBlobs(blob.FS{Dir: filesDir})
	srv.SetProjects(projectSvc)
	enroll.SetDefaultVisibility(defaultVisibility)
	srv.SetHidden(app.NewHiddenService(sqlite.NewSettingRepo(db)))
	srv.SetPins(app.NewPinService(sqlite.NewSettingRepo(db), ids, clock))
	srv.SetVoiceSettings(app.NewVoiceSettingsService(sqlite.NewSettingRepo(db)))
	e2e := app.NewE2EService(sqlite.NewKeyRepo(db), sqlite.NewSettingRepo(db), agentsRepo, consoleSvc, sqlite.NewMemberRepo(db), clock)
	srv.SetE2E(e2e)
	messages.SetMemberNames(func(ctx context.Context, id string) string {
		if m, err := members.Get(ctx, id); err == nil {
			return m.Name
		}
		return ""
	})
	traces := app.NewTraceService(sqlite.NewTraceRepo(db), msgRepo, agentsRepo, clock, o.Log)
	srv.SetTraces(traces)
	automations := app.NewAutomationService(sqlite.NewAutomationRepo(db), messages, msgRepo, security.Tokens{}, ids, clock, o.Log)
	srv.SetAutomations(automations)
	domain.CompleteContracts = jsonschema.New()
	workflows := app.NewWorkflowService(sqlite.NewWorkflowRepo(db), messages, msgRepo, ids, clock, o.Log)
	workflows.SetTraces(sqlite.NewTraceRepo(db))
	srv.SetWorkflows(workflows)
	automations.SetWorkflows(workflows)
	pages := app.NewPageService(sqlite.NewPageRepo(db), blob.FS{Dir: filesDir}, projectSvc, ids, clock)
	pages.SetEncrypted(e2e.Enabled)
	srv.SetPages(pages)
	pageData := app.NewPageDataService(pages, sqlite.NewPageDataRepo(db), ids, clock)
	srv.SetPageData(pageData)
	templateSvc := app.NewTemplateService(sqlite.NewTemplateRepo(db), blob.FS{Dir: filesDir}, pages, pageData, ids, clock)
	templateSvc.SetHubVersion(o.Version)
	if o.Apps != nil {
		o.Apps.Attach(templateSvc.SetOfficial)
		srv.SetAppSync(func(ctx context.Context) api.AppSync {
			r := o.Apps.Sync(ctx)
			out := api.AppSync{Apps: r.Apps, Updated: r.Updated, Source: r.Source, Error: r.Error}
			if !r.At.IsZero() {
				out.At = &r.At
			}
			return out
		})
	} else if official, err := templates.Official(); err == nil {
		templateSvc.SetOfficial(official)
	} else {
		o.Log.Warn("official templates", "err", err)
	}
	srv.SetTemplates(templateSvc)
	handovers := app.NewHandoverService(sqlite.NewHandoverRepo(db), agentsRepo, msgRepo, security.Tokens{}, ids, clock)
	handovers.SetContext(projectRepo, sqlite.NewMachineRepo(db), messages)
	srv.SetHandovers(handovers)
	messages.SetCredentialCheck(handovers.StillCurrent)
	return &Hub{Handler: srv, Server: srv, Dispatcher: dispatcher, Files: files, Traces: traces, Automations: automations, Workflows: workflows,
		Close: db.Close, SetPublicURL: srv.SetPublicURL, SetLAN: srv.SetLAN}, nil
}

// Run serves until ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	if o.AdminToken == "" {
		return errors.New("admin token is required")
	}
	hub, err := Build(o)
	if err != nil {
		return err
	}
	defer hub.Close()
	go hub.Dispatcher.Run(ctx)
	if o.UpdateCheck != nil {
		go o.UpdateCheck.Run(ctx) // background only: never in a request path
	}
	if o.Apps != nil {
		go o.Apps.Run(ctx)
	}
	go hub.Files.Run(ctx, time.Hour)
	go hub.Traces.Run(ctx, time.Hour)
	go hub.Automations.Run(ctx, AutomationTick)
	go hub.Workflows.Run(ctx, WorkflowTick)
	// The other workspaces this hub serves (under /w/<name>).
	ws := newWorkspaces(ctx, o, hub)
	if err := ws.load(); err != nil {
		return err
	}
	defer ws.close()
	if n := len(ws.hubs); n > 0 {
		o.Log.Info("serving workspaces", "count", n, "dir", ws.dir)
	}
	setPublicURL := func(u string) { hub.SetPublicURL(u); ws.setPublicURL(u) }
	setLAN := func(on bool) { hub.SetLAN(on); ws.setLAN(on) }
	var h http.Handler = ws
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: logRequests(h, o.Log), ReadHeaderTimeout: 10 * time.Second}
	o.Log.Info("myco hub listening", "addr", ln.Addr().String(), "db", o.DBPath)
	if ta, ok := ln.Addr().(*net.TCPAddr); ok && ta.IP.IsUnspecified() && o.Tunnel == nil {
		// Reachable on the LAN: links made over localhost get the LAN address
		// (unless --public-url names another).
		setLAN(true)
	}
	if o.Tunnel != nil {
		local := "http://" + ln.Addr().String()
		public, err := o.Tunnel.Prepare(ctx, local)
		if err != nil {
			ln.Close()
			return err
		}
		if public != "" {
			setPublicURL(public)
			o.Log.Info("published through tunnel", "tunnel", o.Tunnel.Name(), "url", public)
		}
		go func() {
			if err := o.Tunnel.Run(ctx, local, setPublicURL); err != nil {
				o.Log.Error("tunnel stopped", "tunnel", o.Tunnel.Name(), "err", err)
			}
		}()
	}
	if o.Ready != nil {
		o.Ready(ln.Addr().String())
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// Unwrap lets http.ResponseController reach the connection (a WebSocket, a
// forwarded live page, streaming).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Hijack: for handlers that ask the writer itself (WebSocket libraries).
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(s.ResponseWriter).Hijack()
}

// Flush passes streaming on.
func (s *statusRecorder) Flush() { _ = http.NewResponseController(s.ResponseWriter).Flush() }

func logRequests(h http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		path := r.URL.Path
		if strings.HasPrefix(path, "/r/") {
			path = "/r/<redacted>" // agent recovery links carry a credential
		}
		if strings.HasPrefix(path, "/j/") {
			path = "/j/<redacted>" // join links carry a credential
		}
		if strings.HasPrefix(path, "/h/") {
			path = "/h/<redacted>" // take-over links carry a credential
		}
		if strings.HasPrefix(path, "/c/") {
			path = "/c/<redacted>" // app links carry a console token
		}
		log.Debug("request", "method", r.Method, "path", path, "status", rec.status, "duration", time.Since(start).Round(time.Millisecond))
	})
}
