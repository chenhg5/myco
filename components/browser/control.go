package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const controlStepLimit = 200
const controlByteLimit = 192 << 10
const controlIdle = 5 * time.Minute

type controlActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type controlInput struct {
	Graph             json.RawMessage `json:"graph,omitempty"`
	Hosts             []string        `json:"hosts"`
	Reason            string          `json:"reason"`
	RequestID         string          `json:"request_id"`
	Session           string          `json:"session"`
	Profile           string          `json:"profile"`
	URL               string          `json:"url"`
	GrantID           string          `json:"grant_id"`
	Actor             controlActor    `json:"actor"`
	ExpiresAt         time.Time       `json:"expires_at"`
	Tab               string          `json:"tab"`
	Mode              string          `json:"mode"`
	MaxNodes          int             `json:"max_nodes"`
	IncludeScreenshot bool            `json:"include_screenshot"`
	Op                string          `json:"op"`
	Ref               string          `json:"ref"`
	Value             string          `json:"value"`
	Clear             *bool           `json:"clear"`
	Key               string          `json:"key"`
	DY                float64         `json:"dy"`
	Wait              string          `json:"wait"`
	Choice            string          `json:"choice"`
	TimeoutMS         int             `json:"timeout_ms"`
	What              string          `json:"what"`
	Attr              string          `json:"attr"`
	Clip              *screenshotClip `json:"clip"`
	Full              bool            `json:"full"`
}
type controlError struct {
	ErrorNode string `json:"error_node,omitempty"`
	Host      string `json:"host,omitempty"`
	Message   string `json:"message,omitempty"`
	Code      string `json:"code"`
	Error     string `json:"error"`
	Reason    string `json:"reason,omitempty"`
	Applied   *int   `json:"applied,omitempty"`
}

func (x *controlError) ErrorMessage() string { return x.Code }
func controlFailure(code, reason string) *controlError {
	return &controlError{Code: code, Error: code, Reason: reason}
}

type controlRequest struct {
	retainUntil time.Time
	terminal    *controlError
	created     time.Time
	digest      [32]byte
	done        chan struct{}
	raw         []byte
	status      int
	session     string
	method      string
}
type controlRef struct {
	sid        proto.TargetSessionID
	frame      proto.PageFrameID
	tab        proto.TargetTargetID
	backend    proto.DOMBackendNodeID
	epoch      uint64
	generation uint64
	bound      bool
}
type controlEvent struct {
	Code   string    `json:"code,omitempty"`
	Host   string    `json:"host,omitempty"`
	Action string    `json:"action,omitempty"`
	Time   time.Time `json:"time"`
	Done   bool      `json:"done"`
	Reason string    `json:"reason,omitempty"`
}
type controlSession struct {
	barriers         map[string]chan struct{}
	allowedURLs      map[proto.TargetTargetID]string
	hosts            []string
	hostMu           sync.Mutex
	violation        *controlError
	initialTab       proto.TargetTargetID
	e                *engine
	browser          *session
	id, scope, grant string
	actor            controlActor
	startURL         string
	ctx              context.Context
	cancel           context.CancelFunc
	opMu             sync.Mutex
	replaying        bool
	replayAbort      context.CancelFunc
	mu               sync.Mutex
	expires, last    time.Time
	closed           bool
	closedAt         time.Time
	code, reason     string
	steps            []node
	variable         int
	refs             map[string]controlRef
	refKeys          map[controlRef]string
	refNext          uint64
	epochs           map[proto.PageFrameID]uint64
	frameNames       map[proto.PageFrameID]string
	tabNames         map[proto.TargetTargetID]string
	tabOrder         []proto.TargetTargetID
	events           []controlEvent
	wake             chan struct{}
	lastAction       string
	lastHost         string
	hostRecovered    map[proto.TargetTargetID]bool
	hostScheduled    bool
	hostDirty        bool
	// A trusted input may cause one top-level navigation and one popup. Reserve
	// both before dispatch, coalesce redirects, and never record iframe traffic.
	navigationUntil           time.Time
	navigationPending         bool
	navigationLoads           map[proto.TargetTargetID]bool
	navigationTracked         map[proto.TargetTargetID]bool
	navigationRequests        map[proto.NetworkRequestID]proto.TargetTargetID
	pendingPopupURL           string
	navigationTab             proto.TargetTargetID
	navigationNode, popupNode string
	pending                   bool
	pendingURL                string
	pendingPopup              bool
	reservedWait              bool
	reservedPopup             bool
}

func (e *engine) controlRoutes(m *http.ServeMux) {
	for _, method := range []string{"open", "snapshot", "act", "extract", "screenshot", "close", "run"} {
		m.HandleFunc("POST /control/"+method, func(w http.ResponseWriter, r *http.Request) { e.controlCall(w, r, method) })
	}
	m.HandleFunc("GET /control/events", e.controlEvents)
}
func controlDecode(r *http.Request, method string) (controlInput, []byte, error) {
	var in controlInput
	raw, err := io.ReadAll(io.LimitReader(r.Body, 256<<10+1))
	if err != nil || len(raw) > 256<<10 {
		return in, nil, errors.New("invalid_argument")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return in, nil, errors.New("invalid_argument")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, nil, errors.New("invalid_argument")
	}
	allowed := "request_id session"
	switch method {
	case "open":
		allowed = "request_id profile url grant_id actor expires_at hosts"
	case "close":
		allowed += " reason"
	case "snapshot":
		allowed += " tab mode max_nodes include_screenshot"
	case "extract":
		allowed += " tab ref what attr"
	case "screenshot":
		allowed += " tab ref clip full"
	case "run":
		allowed += " graph tab timeout_ms"
	case "act":
		allowed += " tab op timeout_ms"
		switch in.Op {
		case "navigate":
			allowed += " url"
		case "click", "dblclick", "hover":
			allowed += " ref"
		case "type":
			allowed += " ref value clear"
		case "select":
			allowed += " ref value"
		case "press":
			allowed += " ref key"
		case "scroll":
			allowed += " ref dy"
		case "wait":
			allowed += " wait"
			if in.Wait == "element" {
				allowed += " ref"
			} else if in.Wait == "url" {
				allowed += " value"
			} else if in.Wait != "network" {
				return in, nil, errors.New("invalid_argument")
			}
		case "dialog":
			allowed += " choice"
		case "back", "forward", "reload", "switch_tab", "close_tab":
		default:
			return in, nil, errors.New("invalid_argument")
		}
	}
	set := map[string]bool{}
	for _, k := range strings.Fields(allowed) {
		set[k] = true
	}
	for k, v := range fields {
		if !set[k] || string(v) == "null" {
			return in, nil, errors.New("invalid_argument")
		}
	}
	if method == "screenshot" {
		choices := 0
		for _, name := range []string{"ref", "clip", "full"} {
			if _, exists := fields[name]; exists {
				choices++
			}
		}
		if choices > 1 {
			return in, nil, errors.New("invalid_argument")
		}
		if _, exists := fields["ref"]; exists && in.Ref == "" {
			return in, nil, errors.New("invalid_argument")
		}
	}
	if in.Reason != "" && !map[string]bool{"grant_ended": true, "host_closed": true, "profile_disabled": true, "idle_timeout": true}[in.Reason] {
		return in, nil, errors.New("invalid_argument")
	}
	if strings.Contains(in.Value, "${") || strings.Contains(in.URL, "${") || strings.Contains(in.Key, "${") {
		return in, nil, errors.New("literal_required")
	}
	maxTimeout := 30000
	if method == "run" {
		maxTimeout = 300000
	}
	if method == "run" && fields["timeout_ms"] != nil && in.TimeoutMS == 0 {
		return in, nil, errors.New("invalid_argument")
	}
	if in.RequestID == "" || len(in.RequestID) > 160 || in.TimeoutMS < 0 || in.TimeoutMS > maxTimeout || len(in.Value) > 16000 || len(in.Key) > 160 || len(in.Ref) > 80 || len(in.Tab) > 80 {
		return in, nil, errors.New("invalid_argument")
	}
	if method != "open" && in.Session == "" {
		return in, nil, errors.New("invalid_argument")
	}
	if method == "open" {
		if _, present := fields["hosts"]; present && len(in.Hosts) == 0 {
			return in, nil, errors.New("invalid_argument")
		}
	}
	if method == "snapshot" && (in.MaxNodes < 0 || in.MaxNodes > 400 || (in.Mode != "" && in.Mode != "a11y" && in.Mode != "text")) {
		return in, nil, errors.New("invalid_argument")
	}
	// Normalize nested graph objects as well as the request envelope. A retry
	// from another JSON encoder must not change the request digest.
	if len(in.Graph) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(in.Graph))
		decoder.UseNumber()
		var graph any
		if decoder.Decode(&graph) != nil {
			return in, nil, errors.New("invalid_argument")
		}
		in.Graph, _ = json.Marshal(graph)
	}
	// Normalize key ordering for retries from different JSON encoders.
	canonical, _ := json.Marshal(in)
	return in, canonical, nil
}
func (e *engine) controlCall(w http.ResponseWriter, r *http.Request, method string) {
	in, raw, err := controlDecode(r, method)
	if err != nil {
		if err.Error() == "literal_required" {
			x := controlFailure("invalid_argument", "")
			x.Message = "Live control does not support ${…}. Enter an example value, then replace it with ${parameter} in the draft."
			jsonControlError(w, x)
		} else {
			fail(w, "invalid_argument", 400)
		}
		return
	}
	owner := scope(r)
	if owner == "" {
		fail(w, "permission_denied", 403)
		return
	}
	key := owner + "\x00" + in.RequestID
	digest := sha256.Sum256(append([]byte(method+"\x00"), raw...))
	e.pruneControls(time.Now())
	e.mu.Lock()
	if e.controls == nil {
		e.controls = map[string]*controlSession{}
		e.controlRequests = map[string]*controlRequest{}
	}
	c := e.controls[in.Session]
	if method != "open" && (c != nil && c.scope != owner || c == nil && e.controlRequests[key] == nil) {
		e.mu.Unlock()
		fail(w, "not_found", 404)
		return
	}
	e.mu.Unlock()
	// Termination takes precedence over cached private observations. The voluntary
	// close response alone remains retryable after the browser has been cleaned up.
	if c != nil {
		if x := c.check(method == "close"); x != nil {
			jsonControlError(w, x)
			return
		}
	}
	e.mu.Lock()
	if old := e.controlRequests[key]; old != nil {
		e.mu.Unlock()
		if old.digest != digest {
			fail(w, "conflict", 409)
			return
		}
		select {
		case <-old.done:
		case <-r.Context().Done():
			return
		}
		e.mu.Lock()
		terminal, status, outcome := old.terminal, old.status, old.raw
		e.mu.Unlock()
		if terminal != nil {
			jsonControlError(w, terminal)
			return
		}
		if old.session != "" {
			e.mu.Lock()
			previous := e.controls[old.session]
			e.mu.Unlock()
			if previous != nil {
				if x := previous.check(old.method == "close"); x != nil {
					jsonControlError(w, x)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(outcome)
		return
	}
	if method != "open" && c == nil {
		e.mu.Unlock()
		fail(w, "not_found", 404)
		return
	}
	if len(e.controlRequests) >= 4096 || e.controlCacheBytes+(256<<10) > 64<<20 {
		e.mu.Unlock()
		fail(w, "busy", 409)
		return
	}
	request := &controlRequest{created: time.Now(), digest: digest, done: make(chan struct{}), session: in.Session, method: method}
	e.controlRequests[key] = request
	e.controlCacheBytes += 256 << 10
	e.mu.Unlock()
	result, x := e.controlExecute(r.Context(), owner, method, in, c)
	status := http.StatusOK
	var data []byte
	if x != nil {
		status = controlHTTPStatus(x.Code)
		data, _ = json.Marshal(x)
	} else {
		data, _ = json.Marshal(result)
	}
	// The HTTP outcome is immutable before waking a retry, including failures
	// after dispatch. A dropped response can never repeat a click or extraction.
	e.mu.Lock()
	e.controlCacheBytes += len(data) - (256 << 10)
	request.raw, request.status = data, status
	if method == "open" && x == nil {
		request.session = result["session"].(string)
		request.retainUntil = in.ExpiresAt
	}
	if method == "close" {
		request.retainUntil = c.expires
	}
	e.mu.Unlock()
	close(request.done)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
func controlHTTPStatus(code string) int {
	switch code {
	case "invalid_argument":
		return 400
	case "host_not_allowed", "permission_denied", "sensitive_field", "sensitive_snapshot":
		return 403
	case "not_found", "target_not_found":
		return 404
	case "conflict", "busy", "stale_ref", "session_closed", "session_expired", "recording_limit":
		return 409
	case "timeout":
		return 408
	default:
		return 500
	}
}
func jsonControlError(w http.ResponseWriter, x *controlError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(controlHTTPStatus(x.Code))
	_ = json.NewEncoder(w).Encode(x)
}
func (c *controlSession) check(closing bool) *controlError {
	c.mu.Lock()
	closed, code, reason := c.closed, c.code, c.reason
	expired := !time.Now().Before(c.expires)
	idle := !c.replaying && time.Since(c.last) >= controlIdle
	c.mu.Unlock()
	if closed {
		if closing && reason == "closed" {
			return nil
		}
		return controlFailure(code, reason)
	}
	if expired || idle {
		reason = "grant_expired"
		if idle {
			reason = "idle_timeout"
		}
		c.terminate("session_expired", reason)
		go c.browser.close()
		return controlFailure("session_expired", reason)
	}
	return nil
}
func (c *controlSession) terminate(code, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.terminateLocked(code, reason)
}
func (c *controlSession) terminateLocked(code, reason string) {
	if c.closed {
		return
	}
	c.closed = true
	c.closedAt = time.Now()
	c.code = code
	c.reason = reason
	c.cancel()
	if reason != "closed" {
		c.steps = nil
	}
	c.refs = nil
	c.refKeys = nil
	c.events = append(c.events, controlEvent{Time: time.Now().UTC(), Done: true, Reason: reason})
	close(c.wake)
	c.wake = make(chan struct{})
}
func (c *controlSession) watch() {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.browser.ctx.Done():
			c.terminate("session_closed", "browser_closed")
			_ = c.browser.close()
			return
		case <-timer.C:
			if c.check(false) != nil {
				return
			}
			c.e.mu.Lock()
			p, ok := c.e.profiles[c.browser.id]
			c.e.mu.Unlock()
			if !ok || !p.Agents {
				c.terminate("session_closed", "profile_disabled")
				_ = c.browser.close()
				return
			}
			// Reconcile browser-level URL changes even when a renderer event
			// is absent during a cross-process history restoration.
			c.enforceHostsSoon()
		}
	}
}
func (e *engine) controlExecute(ctx context.Context, owner, method string, in controlInput, c *controlSession) (map[string]any, *controlError) {
	if method == "open" {
		return e.controlOpen(ctx, owner, in)
	}
	if method == "close" && in.Reason != "" {
		code := "session_closed"
		if in.Reason == "idle_timeout" {
			code = "session_expired"
		}
		c.terminate(code, in.Reason)
		if err := c.browser.close(); err != nil {
			return nil, controlFailure("storage_failed", "cleanup_pending")
		}
		return map[string]any{"session": c.id, "done": true, "reason": in.Reason}, nil
	}
	if !c.opMu.TryLock() {
		return nil, controlFailure("busy", "")
	}
	defer c.opMu.Unlock()
	if x := c.check(method == "close"); x != nil {
		return nil, x
	}
	if method == "close" {
		c.mu.Lock()
		if c.closed && c.reason != "closed" {
			x := controlFailure(c.code, c.reason)
			c.mu.Unlock()
			return nil, x
		}
		steps := append([]node{}, c.steps...)
		c.terminateLocked("session_closed", "closed")
		c.mu.Unlock()
		if err := c.browser.close(); err != nil {
			return nil, controlFailure("storage_failed", "cleanup_pending")
		}
		return map[string]any{"host": c.lastHost, "session": c.id, "profile": c.browser.id, "start_url": c.startURL, "steps": steps, "removed": []string{}, "next": len(steps), "done": true, "reason": "closed"}, nil
	}
	limit := 10 * time.Second
	if method == "run" {
		if err := ValidateBrowserReplayGraph(in.Graph); err != nil {
			return nil, controlNativeError(err)
		}
		limit = 5 * time.Minute
	}
	if in.TimeoutMS != 0 {
		limit = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	deadline := time.Now().Add(limit)
	if method == "run" && c.expires.Before(deadline) {
		deadline = c.expires
	}
	call, stop := context.WithDeadline(c.ctx, deadline)
	defer stop()
	stopCaller := context.AfterFunc(ctx, stop)
	defer stopCaller()
	restore := c.browser.commands(call)
	defer restore()
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
	p, err := c.page(call, in.Tab)
	if err != nil {
		return nil, controlFailure("target_not_found", "")
	}
	c.enforceHosts(call)
	if denied := c.takeHostViolation(); denied != nil {
		return nil, denied
	}
	if method == "act" && in.Op == "navigate" && !controlHostAllowed(c.hosts, in.URL) {
		x := c.hostViolation(in.URL)
		c.recordHostViolation(x)
		c.takeHostViolation()
		return nil, x
	}
	if c.dialogOpen() && method != "run" && (method != "act" || in.Op != "dialog") {
		return nil, controlFailure("target_not_interactable", "dialog_open")
	}
	var out map[string]any
	switch method {
	case "run":
		out, err = c.replay(call, p, in)
	case "snapshot":
		out, err = c.snapshot(call, p, in.Mode, in.MaxNodes, (64<<10)-512)
		if err == nil && in.IncludeScreenshot {
			var shot map[string]any
			shot, err = c.captureImage(call, p, controlInput{})
			if err == nil {
				out["screenshot"] = shot["screenshot"]
			}
		}
	case "act":
		out, err = c.act(call, p, in)
	case "extract":
		out, err = c.extract(call, p, in)
	case "screenshot":
		out, err = c.screenshot(call, p, in)
	}
	if x := c.check(false); x != nil {
		return nil, x
	}
	c.enforceHosts(call)
	// A completed logical action stays successful if a later background
	// navigation crosses the scope while its brief snapshot is being built.
	// The pending violation belongs to the next call in that case.
	if method != "act" && method != "run" || err != nil {
		if denied := c.takeHostViolation(); denied != nil {
			return nil, denied
		}
	}
	if err != nil {
		return nil, controlNativeError(err)
	}
	host := ""
	if info, e := p.Context(call).Info(); e == nil {
		if u, e := url.Parse(info.URL); e == nil {
			host = u.Hostname()
		}
	}
	if method == "run" {
		if denied, _ := out["host"].(string); denied != "" {
			host = denied
		} else if raw, _ := out["url"].(string); raw != "" {
			if final, err := url.Parse(raw); err == nil {
				host = final.Hostname()
			}
		}
	}
	out["session"] = c.id
	out["host"] = host
	c.mu.Lock()
	c.lastHost = host
	c.lastAction = method
	if in.Op != "" {
		c.lastAction = in.Op
	}
	c.events = append(c.events, controlEvent{Host: host, Action: c.lastAction, Time: time.Now().UTC()})
	close(c.wake)
	c.wake = make(chan struct{})
	c.mu.Unlock()
	go c.browser.updatePanel("")
	return out, nil
}
func controlNativeError(err error) *controlError {
	var invalid *BrowserReplayValidationError
	if errors.As(err, &invalid) {
		out := controlFailure("invalid_argument", "")
		out.ErrorNode = invalid.Node
		return out
	}
	code := err.Error()
	switch code {
	case "assertion_failed", "host_not_allowed", "invalid_argument", "unsupported", "permission_denied", "not_found", "busy", "conflict", "stale_ref", "target_not_found", "target_not_interactable", "sensitive_field", "sensitive_snapshot", "session_closed", "session_expired", "cancelled", "timeout", "recording_limit", "artifact_too_large", "artifact_limit", "component_unavailable", "storage_failed", "outcome_unknown":
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			code = "timeout"
		} else if errors.Is(err, context.Canceled) {
			code = "cancelled"
		} else {
			controlDiagnostic("control_error_fallback", err)
			code = "component_unavailable"
		}
	}
	return controlFailure(code, "")
}
func (e *engine) controlOpen(ctx context.Context, owner string, in controlInput) (map[string]any, *controlError) {
	if in.Profile == "" || in.GrantID == "" || in.Actor.ID == "" || len(in.Actor.ID) > 160 || len(in.Actor.Name) > 160 || !in.ExpiresAt.After(time.Now()) || (!in.ExpiresAt.Before(time.Now().Add(30 * time.Minute))) || in.URL != "" && checkedURL(in.URL) != nil {
		return nil, controlFailure("invalid_argument", "")
	}
	hosts, err := normalizeControlHosts(in.Hosts)
	if err != nil {
		return nil, controlNativeError(err)
	}
	if in.URL != "" && !controlHostAllowed(hosts, in.URL) {
		x := controlFailure("host_not_allowed", "")
		if u, e := url.Parse(in.URL); e == nil {
			x.Host = u.Hostname()
		}
		return nil, x
	}
	e.mu.Lock()
	p, ok := e.profiles[in.Profile]
	old := e.sessions[in.Profile]
	if e.controlOpening == nil {
		e.controlOpening = map[string]bool{}
	}
	opening := e.controlOpening[in.Profile]
	if old == nil && !opening {
		e.controlOpening[in.Profile] = true
	}
	e.mu.Unlock()
	if old == nil && !opening {
		defer func() { e.mu.Lock(); delete(e.controlOpening, in.Profile); e.mu.Unlock() }()
	}
	if !ok {
		return nil, controlFailure("not_found", "")
	}
	if !p.Agents {
		return nil, controlFailure("permission_denied", "")
	}
	if old != nil {
		old.mu.Lock()
		ownerWindow := !old.headless && old.controller == ""
		old.mu.Unlock()
		reason := "profile_in_use"
		if ownerWindow {
			reason = "owner_window_open"
		}
		return nil, controlFailure("busy", reason)
	}
	if opening {
		return nil, controlFailure("busy", "profile_starting")
	}
	s, err := e.openSession(ctx, owner, in.Profile, false)
	if err != nil {
		return nil, controlNativeError(err)
	}
	firstPage, err := s.page()
	if err != nil {
		_ = s.close()
		return nil, controlNativeError(err)
	}
	life, cancel := context.WithCancel(e.ctx)
	c := &controlSession{allowedURLs: map[proto.TargetTargetID]string{firstPage.TargetID: in.URL}, barriers: map[string]chan struct{}{}, initialTab: firstPage.TargetID, hosts: hosts, e: e, browser: s, id: "ctl_" + rand.Text(), scope: owner, grant: in.GrantID, actor: in.Actor, startURL: in.URL, ctx: life, cancel: cancel, expires: in.ExpiresAt, last: time.Now(), steps: []node{}, refs: map[string]controlRef{}, refKeys: map[controlRef]string{}, epochs: map[proto.PageFrameID]uint64{}, frameNames: map[proto.PageFrameID]string{}, tabNames: map[proto.TargetTargetID]string{}, wake: make(chan struct{})}
	if err = s.claim(c.id); err != nil {
		cancel()
		_ = s.close()
		return nil, controlNativeError(err)
	}
	s.mu.Lock()
	s.control = c
	s.mu.Unlock()
	e.mu.Lock()
	e.controls[c.id] = c
	e.mu.Unlock()
	go c.watch()
	call, stop := context.WithTimeout(life, 30*time.Second)
	defer stop()
	stopCaller := context.AfterFunc(ctx, stop)
	defer stopCaller()
	restore := s.commands(call)
	defer restore()
	page, err := s.page()
	if page != nil {
		c.tabName(page.TargetID)
	}
	if err == nil && in.URL != "" {
		err = page.Context(call).Navigate(in.URL)
		if err == nil {
			err = c.waitReady(call, page)
		}
	}
	if err != nil {
		c.terminate("session_closed", "open_failed")
		_ = s.close()
		return nil, controlNativeError(err)
	}
	if denied := c.enforceHosts(call); denied != nil {
		c.terminate("session_closed", "host_not_allowed")
		_ = s.close()
		return nil, denied
	}
	snap, err := c.snapshot(call, page, "a11y", 400, 64<<10)
	if err != nil {
		c.terminate("session_closed", "open_failed")
		_ = s.close()
		return nil, controlNativeError(err)
	}
	host := ""
	if u, err := url.Parse(snap["url"].(string)); err == nil {
		host = u.Hostname()
	}
	c.mu.Lock()
	c.lastAction = "open"
	c.lastHost = host
	c.events = append(c.events, controlEvent{Host: host, Action: "open", Time: time.Now().UTC()})
	close(c.wake)
	c.wake = make(chan struct{})
	c.mu.Unlock()
	_ = s.updatePanel("")
	return map[string]any{"host": host, "session": c.id, "profile": in.Profile, "recording_id": c.id, "start_url": in.URL, "active_tab": c.tabName(page.TargetID), "tabs": c.tabs(call), "snapshot": snap, "expires_at": c.expires, "idle_expires_at": c.last.Add(controlIdle)}, nil
}
func (c *controlSession) tabName(id proto.TargetTargetID) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if name := c.tabNames[id]; name != "" {
		return name
	}
	name := "t" + strconv.Itoa(len(c.tabOrder)+1)
	c.tabNames[id] = name
	c.tabOrder = append(c.tabOrder, id)
	return name
}
func (c *controlSession) page(ctx context.Context, name string) (*rod.Page, error) {
	if name == "" {
		return c.browser.page()
	}
	c.mu.Lock()
	var id proto.TargetTargetID
	for target, n := range c.tabNames {
		if n == name {
			id = target
			break
		}
	}
	c.mu.Unlock()
	if id == "" {
		return nil, errors.New("target_not_found")
	}
	c.browser.mu.Lock()
	p := c.browser.pages[id]
	c.browser.mu.Unlock()
	if p == nil {
		return nil, errors.New("target_not_found")
	}
	return p.Context(ctx), nil
}
func (c *controlSession) tabs(ctx context.Context) []map[string]any {
	c.browser.mu.Lock()
	pages := []*rod.Page{}
	for _, p := range c.browser.pages {
		pages = append(pages, p)
	}
	active := c.browser.current
	c.browser.mu.Unlock()
	out := []map[string]any{}
	for _, p := range pages {
		info, err := p.Context(ctx).Info()
		if err == nil {
			out = append(out, map[string]any{"id": c.tabName(p.TargetID), "url": truncate(info.URL, 2048), "title": truncate(info.Title, 512), "active": p.TargetID == active})
		}
	}
	return out
}
func (c *controlSession) budget(nodes ...node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.budgetLocked(nodes...)
}
func (c *controlSession) budgetLocked(nodes ...node) error {
	if c.closed {
		return errors.New("session_closed")
	}
	if c.replaying {
		return nil
	}
	all := append(append([]node{}, c.steps...), nodes...)
	if time.Now().Before(c.navigationUntil) {
		if c.reservedWait {
			all = append(all, node{ID: "n_reserved", Type: "wait", Wait: "url", Value: reservedNavigationValue()})
		}
		if c.reservedPopup {
			all = append(all, node{ID: "n_reserved", Type: "tab", Tab: "popup"})
		}
	}
	raw, err := json.Marshal(all)
	if err != nil {
		return err
	}
	if len(all) > controlStepLimit || len(raw) > controlByteLimit {
		return errors.New("recording_limit")
	}
	return nil
}
func (c *controlSession) appendStep(n node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.budgetLocked(n); err != nil {
		return err
	}
	if !c.replaying {
		c.steps = append(c.steps, n)
	}
	return nil
}
func (e *engine) controlEvents(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.Atoi(r.URL.Query().Get("after"))
	if err != nil || after < 0 {
		fail(w, "invalid_argument", 400)
		return
	}
	e.mu.Lock()
	c := e.controls[r.URL.Query().Get("session")]
	e.mu.Unlock()
	if c == nil || c.scope != scope(r) {
		fail(w, "not_found", 404)
		return
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if after > len(c.events) {
			c.mu.Unlock()
			fail(w, "invalid_argument", 400)
			return
		}
		if c.closed || after < len(c.events) {
			events := append([]controlEvent{}, c.events[after:]...)
			result := map[string]any{"session": c.id, "events": events, "next": len(c.events), "done": c.closed, "reason": c.reason}
			c.mu.Unlock()
			jsonReply(w, result)
			return
		}
		wake := c.wake
		c.mu.Unlock()
		select {
		case <-wake:
		case <-r.Context().Done():
			return
		case <-timer.C:
			jsonReply(w, map[string]any{"session": c.id, "events": []controlEvent{}, "next": after, "done": false})
			return
		}
	}
}

func (c *controlSession) activeTab() string {
	c.browser.mu.Lock()
	id := c.browser.current
	c.browser.mu.Unlock()
	if id == "" {
		return ""
	}
	return c.tabName(id)
}
