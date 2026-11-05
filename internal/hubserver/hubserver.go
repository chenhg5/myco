// Package hubserver is the composition root of the hub: it wires storage,
// notifier, use cases and the HTTP adapter together and runs the server.
package hubserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
	"github.com/chenhg5/myco/internal/transport/httpapi"
)

// Options configures the hub.
type Options struct {
	Addr         string
	DBPath       string
	AdminToken   string
	Policy       app.MessagePolicy
	OnlineWindow time.Duration
	Log          *slog.Logger
	// Ready, if set, receives the bound address once listening.
	Ready func(addr string)
}

// Build assembles the HTTP handler and returns a cleanup func.
func Build(o Options) (http.Handler, func() error, error) {
	db, err := sqlite.Open(o.DBPath)
	if err != nil {
		return nil, nil, err
	}
	agentsRepo := sqlite.NewAgentRepo(db)
	msgRepo := sqlite.NewMessageRepo(db)
	ids, clock := security.IDs{}, app.SystemClock{}
	agents := app.NewAgentService(agentsRepo, security.Tokens{}, ids, clock, o.OnlineWindow)
	messages := app.NewMessageService(agentsRepo, msgRepo, notify.NewMemory(), ids, clock, o.Policy)
	return httpapi.New(agents, messages, o.AdminToken, o.Log), db.Close, nil
}

// Run serves until ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	if o.AdminToken == "" {
		return errors.New("admin token is required")
	}
	h, closeDB, err := Build(o)
	if err != nil {
		return err
	}
	defer closeDB()
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
