package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/go-rod/rod/lib/proto"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type runEvent struct{ Step *runStep }
type playRun struct {
	// outputCut: cleanOutput left something out (read and reset by its callers).
	outputCut       bool
	applyDialog     func(context.Context, node, observation) error
	after           uint64
	anchorTarget    proto.TargetTargetID
	anchorFrame     proto.PageFrameID
	tabOrder        []proto.TargetTargetID
	preparedDialogs map[string]*preparedDialog
	tabs            map[string]proto.TargetTargetID
	id, scope       string
	session         *session
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	update          runUpdate
	history         []runEvent
	wake            chan struct{}
	paused          bool
	step            bool
	choice          string
	waitingConfirm  bool
	breakpoints     map[string]bool
	values          map[string]string
	secrets         []string
	uploads         []string
	action          func(context.Context, node) (any, error)
}

var variablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]{0,39})\}`)

func arguments(options runOptions) (map[string]string, []string, error) {
	values := map[string]string{}
	secrets := []string{}
	declared := map[string]string{}
	for _, p := range options.Script.Params {
		if !variableName.MatchString(p.Name) || declared[p.Name] != "" {
			return nil, nil, errors.New("invalid_param")
		}
		if !map[string]bool{"string": true, "text": true, "number": true, "bool": true, "secret": true}[p.Type] {
			return nil, nil, errors.New("invalid_param")
		}
		declared[p.Name] = p.Type
		value := options.Params[p.Name]
		if value == "" {
			value = p.Default
		}
		if p.Type == "secret" {
			if p.Default != "" {
				return nil, nil, errors.New("sensitive_literal")
			}
			value = options.Secrets[p.Name]
			if value != "" {
				secrets = append(secrets, value)
			}
		}
		if p.Required && value == "" {
			return nil, nil, errors.New("secret_required")
		}
		if p.Type == "number" && value != "" {
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				return nil, nil, errors.New("invalid_param")
			}
		}
		if p.Type == "bool" && value != "" && value != "true" && value != "false" {
			return nil, nil, errors.New("invalid_param")
		}
		values[p.Name] = value
	}
	for name := range options.Secrets {
		if declared[name] != "secret" {
			return nil, nil, errors.New("invalid_param")
		}
	}
	for name := range options.Params {
		if declared[name] == "" || declared[name] == "secret" {
			return nil, nil, errors.New("invalid_param")
		}
	}
	return values, secrets, nil
}
func (r *playRun) changedLocked(step *runStep) {
	r.history = append(r.history, runEvent{step})
	close(r.wake)
	r.wake = make(chan struct{})
}
func (r *playRun) mask(s string) string {
	for _, secret := range r.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	return truncate(s, 2000)
}
func (r *playRun) cleanOutput(value any, depth int) any {
	if depth > 4 {
		r.outputCut = true
		return nil
	}
	switch v := value.(type) {
	case string:
		// Scripts often return JSON.stringify(list): keep its items, not
		// its first characters.
		if t := strings.TrimSpace(v); len(t) > 1 && len(t) <= 4<<20 && (t[0] == '[' || t[0] == '{') {
			var decoded any
			if json.Unmarshal([]byte(t), &decoded) == nil {
				return r.cleanOutput(decoded, depth)
			}
		}
		if utf8.RuneCountInString(v) > 2000 {
			r.outputCut = true
		}
		return r.mask(v)
	case []any:
		out := []any{}
		for i, item := range v {
			if i >= 200 {
				r.outputCut = true
				break
			}
			out = append(out, r.cleanOutput(item, depth+1))
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			if len(out) >= 100 {
				r.outputCut = true
				break
			}
			out[r.mask(key)] = r.cleanOutput(item, depth+1)
		}
		return out
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var decoded any
		if json.Unmarshal(raw, &decoded) != nil {
			return nil
		}
		switch decoded.(type) {
		case string, []any, map[string]any:
			return r.cleanOutput(decoded, depth+1)
		}
		return decoded
	}
}

func (r *playRun) expand(raw string, allowSecret bool) (string, error) {
	var failure error
	result := variablePattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := match[2 : len(match)-1]
		value, ok := r.values[name]
		if !ok {
			failure = errors.New("invalid_param")
			return ""
		}
		if !allowSecret {
			for _, secret := range r.secrets {
				if secret != "" && strings.Contains(value, secret) {
					failure = errors.New("sensitive_literal")
					return ""
				}
			}
		}
		return value
	})
	return result, failure
}
func (r *playRun) prepare(n node) (node, error) {
	clone := n
	for _, pair := range []struct {
		dst    *string
		src    string
		secret bool
	}{{&clone.URL, n.URL, false}, {&clone.Value, n.Value, n.Type == "type" || n.Type == "select" || n.Type == "dialog"}, {&clone.Key, n.Key, false}, {&clone.Code, n.Code, false}, {&clone.Message, n.Message, false}} {
		value, err := r.expand(pair.src, pair.secret)
		if err != nil {
			return node{}, err
		}
		*pair.dst = value
	}
	return clone, nil
}
func (r *playRun) gate(n node) error {
	r.mu.Lock()
	if r.breakpoints[n.ID] {
		r.paused = true
	}
	if n.Type == "confirm" {
		r.paused = true
		r.waitingConfirm = true
		r.update.Question = r.mask(n.Message)
	}
	r.update.Node = n.ID
	for {
		if !r.paused {
			r.update.State = "running"
			r.update.Question = ""
			r.changedLocked(nil)
			r.mu.Unlock()
			return nil
		}
		r.update.State = "paused"
		r.changedLocked(nil)
		wake := r.wake
		r.mu.Unlock()
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-wake:
		}
		r.mu.Lock()
	}
}
func (r *playRun) control(action, choice string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if finishedRun(r.update.State) {
		return errors.New("not_found")
	}
	switch action {
	case "cancel":
		r.cancel()
	case "pause":
		r.paused = true
	case "resume", "step":
		if r.waitingConfirm {
			if choice != "ok" && choice != "cancel" {
				return errors.New("invalid_argument")
			}
			if choice == "cancel" {
				r.cancel()
				return nil
			}
			r.choice = choice
			r.waitingConfirm = false
		}
		r.paused = false
		r.step = action == "step"
	default:
		return errors.New("invalid_argument")
	}
	r.changedLocked(nil)
	return nil
}
func finishedRun(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}
func (r *playRun) play(seq []node, onError string) {
	defer r.cancel()
	if r.session != nil {
		defer r.session.release(r.id)
	}
	defer func() { r.mu.Lock(); clear(r.values); r.secrets = nil; r.uploads = nil; r.mu.Unlock() }()
	for index, original := range seq {
		if err := r.ctx.Err(); err != nil {
			r.end("cancelled", original.ID, "")
			return
		}
		preparedDialog := r.preparedDialogs[original.ID]
		if preparedDialog == nil || !preparedDialog.gated {
			if err := r.gate(original); err != nil {
				r.end("cancelled", original.ID, "")
				return
			}
		}
		n, err := r.prepare(original)
		if err != nil {
			r.end("failed", original.ID, publicError(err))
			return
		}
		r.mu.Lock()
		r.changedLocked(&runStep{Node: n.ID, State: "running"})
		r.mu.Unlock()
		timeout := 20 * time.Second
		if n.TimeoutMS > 0 {
			timeout = time.Duration(min(n.TimeoutMS, 600000)) * time.Millisecond
		}
		ctx, stop := context.WithTimeout(r.ctx, timeout)
		started := time.Now()
		var value any
		policy := n.OnError
		if policy == "" {
			policy = onError
		}
		attempts := 1
		if strings.HasPrefix(policy, "retry:") {
			count, _ := strconv.Atoi(strings.TrimPrefix(policy, "retry:"))
			attempts += min(5, max(0, count))
		}
		for i := 0; i < attempts; i++ {
			var next *node
			if index+1 < len(seq) {
				next = &seq[index+1]
			}
			value, err = r.perform(ctx, n, next, timeout)
			if err == nil || ctx.Err() != nil {
				break
			}
		}
		stop()
		state, code := "ok", ""
		if err != nil {
			state, code = "failed", publicError(err)
			if policy == "skip" {
				state = "skipped"
			}
		}
		shot := ""
		// Secret-bearing runs deliberately omit snapshots. A plain text secret
		// can be rendered anywhere by a site, not only inside a password input.
		if n.Clip != nil {
			if err == nil && len(r.secrets) == 0 {
				shot, _ = value.(string)
			}
		} else if r.session != nil && r.session.e != nil && len(r.secrets) == 0 {
			if p, e := r.session.page(); e == nil {
				shot, _ = r.session.e.snapshot(r.scope, p, n.Target)
			}
		}
		step := &runStep{Node: n.ID, State: state, Error: code, MS: time.Since(started).Milliseconds(), Shot: shot}
		r.mu.Lock()
		if err == nil && n.Into != "" && variableName.MatchString(n.Into) {
			r.outputCut = false
			safe := r.cleanOutput(value, 0)
			candidate := map[string]any{}
			for key, value := range r.update.Output {
				candidate[key] = value
			}
			candidate[n.Into] = safe
			if r.outputCut {
				candidate["truncated"] = true
			}
			if raw, e := json.Marshal(candidate); e == nil && len(raw) <= 16<<10 {
				r.update.Output = candidate
				if text, ok := safe.(string); ok {
					r.values[n.Into] = text
				} else {
					encoded, _ := json.Marshal(safe)
					r.values[n.Into] = string(encoded)
				}
			} else {
				r.update.Output["truncated"] = true
			}
		}
		r.changedLocked(step)
		if r.step || (preparedDialog != nil && preparedDialog.stepAfter) {
			r.paused = true
			r.step = false
		}
		r.mu.Unlock()
		if r.ctx.Err() != nil {
			r.end("cancelled", n.ID, "")
			return
		}
		if state == "failed" {
			r.end("failed", n.ID, code)
			return
		}
	}
	r.end("succeeded", "", "")
}
func (r *playRun) end(state, id, code string) {
	if r.session != nil {
		r.session.release(r.id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.update.State = state
	r.update.Node = id
	r.update.ErrorNode = id
	r.update.Error = code
	r.update.Question = ""
	r.changedLocked(nil)
}
func (r *playRun) events(ctx context.Context, after int) (runUpdate, error) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		r.mu.Lock()
		if after < 0 || after > len(r.history) {
			r.mu.Unlock()
			return runUpdate{}, errors.New("invalid_argument")
		}
		if len(r.history) > after || finishedRun(r.update.State) {
			out := r.update
			out.Steps = []runStep{}
			for _, evt := range r.history[after:] {
				if evt.Step != nil {
					out.Steps = append(out.Steps, *evt.Step)
				}
			}
			out.Next = len(r.history)
			out.Output = map[string]any{}
			for k, v := range r.update.Output {
				out.Output[k] = v
			}
			r.mu.Unlock()
			return out, nil
		}
		wake := r.wake
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return runUpdate{}, ctx.Err()
		case <-wake:
		case <-timer.C:
			r.mu.Lock()
			out := r.update
			out.Steps = []runStep{}
			out.Next = after
			out.Output = map[string]any{}
			r.mu.Unlock()
			return out, nil
		}
	}
}
func (e *engine) runRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /runs", func(w http.ResponseWriter, req *http.Request) {
		var options runOptions
		if !readJSON(w, req, &options) {
			return
		}
		seq, err := linearOrder(options.Script, options.StartAt)
		if err != nil {
			fail(w, publicError(err), 400)
			return
		}
		values, secrets, err := arguments(options)
		if err != nil {
			fail(w, publicError(err), 400)
			return
		}
		if options.StartAt == "" && options.Script.StartURL != "" && checkedURL(options.Script.StartURL) != nil {
			fail(w, "invalid_argument", 400)
			return
		}
		s, err := e.openSession(req.Context(), scope(req), options.Profile, options.Headless)
		if err != nil {
			failProfileProcess(w, err)
			return
		}
		ctx, cancel := context.WithCancel(s.ctx)
		r := &playRun{id: "run_" + rand.Text(), scope: scope(req), session: s, ctx: ctx, cancel: cancel, wake: make(chan struct{}), values: values, secrets: secrets, breakpoints: map[string]bool{}, update: runUpdate{State: "queued", Steps: []runStep{}, Output: map[string]any{}}}
		if s.claim(r.id) != nil {
			cancel()
			fail(w, "busy", 409)
			return
		}
		for _, id := range options.Breakpoints {
			r.breakpoints[id] = true
		}
		r.uploads = append([]string(nil), options.Uploads...)
		r.action = r.act
		e.mu.Lock()
		if len(e.runs) >= 256 {
			e.mu.Unlock()
			s.release(r.id)
			cancel()
			fail(w, "run_limit", 409)
			return
		}
		e.runs[r.id] = r
		e.mu.Unlock()
		go func() {
			if options.StartAt == "" && options.Script.StartURL != "" {
				opening, finish := context.WithTimeout(r.ctx, 30*time.Second)
				p, err := s.newPage(opening, options.Script.StartURL)
				finish()
				if err != nil {
					r.end("failed", "", publicError(err))
					cancel()
					return
				}
				r.tabOrder = []proto.TargetTargetID{p.TargetID}
			} else {
				if p, err := s.page(); err == nil {
					r.tabOrder = []proto.TargetTargetID{p.TargetID}
				}
			}
			r.after = s.cursor()
			r.play(seq, options.Script.OnError)
		}()
		jsonReply(w, map[string]string{"id": r.id})
	})
	m.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, req *http.Request) {
		after, err := strconv.Atoi(req.URL.Query().Get("after"))
		if err != nil {
			fail(w, "invalid_argument", 400)
			return
		}
		e.mu.Lock()
		r := e.runs[req.PathValue("id")]
		e.mu.Unlock()
		if r == nil || r.scope != scope(req) {
			fail(w, "not_found", 404)
			return
		}
		out, err := r.events(req.Context(), after)
		if err != nil {
			failProfileProcess(w, err)
			return
		}
		jsonReply(w, out)
	})
	m.HandleFunc("POST /runs/{id}/{action}", func(w http.ResponseWriter, req *http.Request) {
		var in struct {
			Choice string `json:"choice"`
		}
		if !readJSON(w, req, &in) {
			return
		}
		e.mu.Lock()
		r := e.runs[req.PathValue("id")]
		e.mu.Unlock()
		if r == nil || r.scope != scope(req) {
			fail(w, "not_found", 404)
			return
		}
		if err := r.control(req.PathValue("action"), in.Choice); err != nil {
			failProfileProcess(w, err)
			return
		}
		jsonReply(w, true)
	})
}
