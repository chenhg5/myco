// Package hubserver is the composition root of the hub: it wires storage,
// notifier, use cases and the HTTP adapter together and runs the server.
package hubserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/infra/blob"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
	"github.com/chenhg5/myco/internal/infra/wake"
	"github.com/chenhg5/myco/internal/transport/httpapi"
	"github.com/chenhg5/myco/internal/transport/ingress"
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
}

// Hub is an assembled hub: the HTTP handler plus background workers.
type Hub struct {
	Handler    http.Handler
	Dispatcher *app.Dispatcher
	Files      *app.FileService
	Close      func() error
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
	messages := app.NewMessageService(agentsRepo, msgRepo, notify.NewMemory(), ids, clock, o.Policy)
	enroll := app.NewEnrollmentService(sqlite.NewEnrollmentRepo(db), agentsRepo, security.Tokens{}, ids, clock)
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
	srv.SetFiles(files)
	srv.SetOverview(app.NewOverviewService(agentsRepo, msgRepo, clock, o.OnlineWindow))
	return &Hub{Handler: srv, Dispatcher: dispatcher, Files: files, Close: db.Close}, nil
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
	go hub.Files.Run(ctx, time.Hour)
	h := hub.Handler
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: logRequests(h, o.Log), ReadHeaderTimeout: 10 * time.Second}
	o.Log.Info("myco hub listening", "addr", ln.Addr().String(), "db", o.DBPath)
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

func logRequests(h http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		log.Debug("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).Round(time.Millisecond))
	})
}
