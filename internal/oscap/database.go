package oscap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/dbstudio"
)

func init() {
	register(
		&Cap{Name: "db.profile", About: "Save this page's private local database connections; credentials stay in the credential store", Fn: databaseProfile},
		&Cap{Name: "db.catalog", About: "List databases, tables, collections and columns", Fn: databaseCall},
		&Cap{Name: "db.query", About: "Run bounded SQL with read-only queries or explicit writes", Fn: databaseCallQuery},
		&Cap{Name: "db.rows", About: "Browse or change keyed rows with original-snapshot checks", Fn: databaseCallRows},
		&Cap{Name: "db.mongo", About: "Find, aggregate or change MongoDB Extended JSON documents", Fn: databaseCallMongo},
		&Cap{Name: "db.cancel", About: "Cancel a query started by this page", Fn: databaseCancel},
	)
}
func databaseStore(e *Env) (dbstudio.Store, error) {
	if e == nil || e.Home == "" || !strings.Contains(e.Scope, "\x00") || strings.HasSuffix(e.Scope, "\x00") || strings.HasPrefix(e.Scope, "\x00") {
		return dbstudio.Store{}, errf(CodeInvalid, "database calls require a workspace and page")
	}
	return dbstudio.Store{Home: e.Home, Scope: e.Scope, Secrets: e.Secrets}, nil
}
func databaseError(err error) error {
	if err == nil {
		return nil
	}
	var e *dbstudio.Error
	if errors.As(err, &e) {
		return &Error{Code: e.Code, Message: e.Message}
	}
	return AsError(err)
}
func databaseProfile(ctx context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	s, err := databaseStore(e)
	if err != nil {
		return nil, err
	}
	in := struct {
		Action  string                `json:"action"`
		ID      string                `json:"id"`
		Version int                   `json:"version"`
		Profile dbstudio.ProfileInput `json:"profile"`
	}{}
	in.Profile.ReadOnly = true
	if err = decode(args, &in); err != nil {
		return nil, err
	}
	switch in.Action {
	case "list":
		v, err := s.List()
		return v, databaseError(err)
	case "save":
		v, err := s.Save(in.Profile)
		return v, databaseError(err)
	case "forget":
		err := s.Forget(in.ID, in.Version)
		return err == nil, databaseError(err)
	case "test":
		if in.Profile.ID == "" {
			in.Profile.ID = "unsaved"
		}
		c, err := s.Prepare(in.Profile)
		if err != nil {
			return nil, databaseError(err)
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		err = c.Ping(ctx)
		if ctx.Err() != nil {
			return nil, AsError(ctx.Err())
		}
		return map[string]bool{"connected": err == nil}, databaseError(err)
	}
	return nil, errf(CodeInvalid, "choose list, save, forget or test")
}

var databaseRuns = struct {
	sync.Mutex
	active        map[string]context.CancelFunc
	pendingCancel map[string]time.Time
}{active: map[string]context.CancelFunc{}, pendingCancel: map[string]time.Time{}}

func databaseCancel(_ context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	if _, err := databaseStore(e); err != nil {
		return nil, err
	}
	var in struct {
		RequestID string `json:"request_id"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if in.RequestID == "" || len(in.RequestID) > 80 {
		return nil, errf(CodeInvalid, "a request ID is required")
	}
	databaseRuns.Lock()
	key := e.Scope + "\x00" + in.RequestID
	cancel := databaseRuns.active[key]
	for id, at := range databaseRuns.pendingCancel {
		if time.Since(at) > 2*time.Minute {
			delete(databaseRuns.pendingCancel, id)
		}
	}
	if cancel == nil && len(databaseRuns.pendingCancel) < 4096 {
		databaseRuns.pendingCancel[key] = time.Now()
	}
	databaseRuns.Unlock()
	if cancel != nil {
		cancel()
	}
	return map[string]bool{"cancel_requested": true}, nil
}
func databaseOperation(ctx context.Context, e *Env, args json.RawMessage, kind string) (any, error) {
	s, err := databaseStore(e)
	if err != nil {
		return nil, err
	}
	var r dbstudio.Request
	if err = decode(args, &r); err != nil {
		return nil, err
	}
	ms := r.TimeoutMS
	if ms <= 0 {
		ms = 30000
	}
	if ms > 120000 {
		ms = 120000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
	defer cancel()
	if len(r.RequestID) > 80 {
		return nil, errf(CodeInvalid, "request ID is too long")
	}
	key := e.Scope + "\x00" + r.RequestID
	if r.RequestID != "" {
		databaseRuns.Lock()
		if at, ok := databaseRuns.pendingCancel[key]; ok {
			delete(databaseRuns.pendingCancel, key)
			if time.Since(at) < 2*time.Minute {
				databaseRuns.Unlock()
				return nil, errf(CodeCancelled, "cancelled before execution")
			}
		}
		if _, exists := databaseRuns.active[key]; exists {
			databaseRuns.Unlock()
			return nil, errf("conflict", "request ID already running")
		}
		count := 0
		for k := range databaseRuns.active {
			if strings.HasPrefix(k, e.Scope+"\x00") {
				count++
			}
		}
		if count >= 4 {
			databaseRuns.Unlock()
			return nil, errf(CodeFailed, "this page already has four database requests running")
		}
		databaseRuns.active[key] = cancel
		databaseRuns.Unlock()
		defer func() { databaseRuns.Lock(); delete(databaseRuns.active, key); databaseRuns.Unlock() }()
	}
	c, err := s.Get(r.Connection)
	if err != nil {
		return nil, databaseError(err)
	}
	if ctx.Err() != nil {
		return nil, AsError(ctx.Err())
	}
	var v any
	switch kind {
	case "catalog":
		switch r.Action {
		case "databases":
			v, err = c.Databases(ctx)
		case "tables":
			v, err = c.Tables(ctx, r)
		case "columns":
			v, err = c.Columns(ctx, r)
		default:
			err = errf(CodeInvalid, "unknown catalog operation")
		}
	case "query":
		v, err = c.SQL(ctx, r)
	case "rows":
		if r.Action == "browse" {
			v, err = c.Browse(ctx, r)
		} else {
			v, err = c.Mutate(ctx, r)
		}
	case "mongo":
		v, err = c.Mongo(ctx, r)
	}
	var de *dbstudio.Error
	if err != nil && ctx.Err() != nil && !(errors.As(err, &de) && de.Code == "outcome_unknown") {
		return nil, AsError(ctx.Err())
	}
	return v, databaseError(err)
}
func databaseCall(ctx context.Context, e *Env, a json.RawMessage, _ Emit) (any, error) {
	return databaseOperation(ctx, e, a, "catalog")
}
func databaseCallQuery(ctx context.Context, e *Env, a json.RawMessage, _ Emit) (any, error) {
	return databaseOperation(ctx, e, a, "query")
}
func databaseCallRows(ctx context.Context, e *Env, a json.RawMessage, _ Emit) (any, error) {
	return databaseOperation(ctx, e, a, "rows")
}
func databaseCallMongo(ctx context.Context, e *Env, a json.RawMessage, _ Emit) (any, error) {
	return databaseOperation(ctx, e, a, "mongo")
}
