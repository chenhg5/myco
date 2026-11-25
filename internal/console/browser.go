package console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

// browserComponentAPI is the local optional-process boundary; tests supply
// the same HTTP protocol without launching a browser or depending on an OS.
type browserComponentAPI interface {
	Status() api.BrowserComponentStatus
	EnsureRunning(context.Context) error
	Install(context.Context) error
	Call(context.Context, string, string, string, any, any) error
	Artifact(context.Context, string, string) ([]byte, string, error)
	Stop()
}

type browserState struct {
	mu            sync.Mutex
	component     browserComponentAPI
	confirmations map[string]browserConfirmation
	policies      *browserpolicy.Store
	identities    map[string]browserOwnerIdentity
	blocked       map[string]bool
	runs          *browserRunBook
	cancelAgents  func(hub, profile string)
}
type browserConfirmation struct {
	Scope, Action, Profile string
	Expires                time.Time
}
type browserCall struct {
	Page         string          `json:"page"`
	Instance     string          `json:"instance"`
	Action       string          `json:"action"`
	Args         json.RawMessage `json:"args"`
	Confirmation string          `json:"confirmation,omitempty"`
}

func (b *Backend) browserComponent() browserComponentAPI {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	if b.browser.component == nil {
		home, _ := os.UserHomeDir()
		if b.Local != nil && b.Local.Home != "" {
			home = b.Local.Home
		}
		b.browser.component = &browserautomation.Component{Home: home}
	}
	return b.browser.component
}
func browserScope(hub, page, instance string) string {
	sum := sha256.Sum256([]byte(hub + "\x00" + page))
	return hex.EncodeToString(sum[:]) + "/" + instance
}
func browserFailure(code string, status int) error {
	return &client.Error{Code: code, Status: status, Message: code}
}
func (b *Backend) confirmBrowser(scope, action, profile string) string {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	if b.browser.confirmations == nil {
		b.browser.confirmations = map[string]browserConfirmation{}
	}
	now := time.Now()
	for key, item := range b.browser.confirmations {
		if now.After(item.Expires) {
			delete(b.browser.confirmations, key)
		}
	}
	if len(b.browser.confirmations) >= 32 {
		return ""
	}
	nonce := rand.Text() + rand.Text()
	b.browser.confirmations[nonce] = browserConfirmation{Scope: scope, Action: action, Profile: profile, Expires: now.Add(time.Minute)}
	return nonce
}
func (b *Backend) consumeBrowserConfirmation(nonce, scope, action, profile string) bool {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	record, ok := b.browser.confirmations[nonce]
	delete(b.browser.confirmations, nonce)
	return ok && time.Now().Before(record.Expires) && record.Scope == scope && record.Action == action && record.Profile == profile
}
func (b *Backend) browserRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /app/hubs/{id}/browser/confirm", b.withHubAndHubTimeout(30*time.Second, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		// A page bridge never exposes this route/header or receives this nonce.
		// The native host calls it only after its own modal was accepted.
		if r.Header.Get("X-Myco-Browser-Owner") != "1" {
			writeErr(w, browserFailure("permission_denied", 403))
			return
		}
		var in browserCall
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if !decode(w, r, &in) {
			return
		}
		if !validBrowserCall(in) || (in.Action != "install" && in.Action != "profiles.allowAgents" && in.Action != "profiles.configure" && in.Action != "approvals.request") {
			writeErr(w, browserFailure("invalid_argument", 400))
			return
		}
		if _, err := c.Page(r.Context(), in.Page); err != nil {
			writeErr(w, err)
			return
		}
		who, err := c.Whoami(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if who.Device == nil {
			writeErr(w, browserFailure("permission_denied", 403))
			return
		}
		var args struct {
			ID string `json:"id"`
		}
		json.Unmarshal(in.Args, &args)
		bound := args.ID
		if in.Action == "profiles.allowAgents" {
			owner, err := b.browserOwner(r.Context(), c, h, in.Page)
			if err != nil {
				writeErr(w, err)
				return
			}
			bound += "\x00" + owner.Owner + "\x00" + owner.Workspace
		}
		if in.Action == "profiles.configure" {
			_, key, err := browserConfigureBound(in.Args)
			if err != nil {
				writeErr(w, err)
				return
			}
			bound = key
		}
		if in.Action == "approvals.request" {
			scope, err := b.browserOwner(r.Context(), c, h, in.Page)
			if err != nil {
				writeErr(w, err)
				return
			}
			var req browserpolicy.Request
			if err := browserDecode(in.Args, &req); err != nil {
				writeErr(w, err)
				return
			}
			review, err := browserpolicy.Prepare(scope, req)
			if err != nil {
				writeErr(w, browserPolicyError(err))
				return
			}
			bound = req.Profile + "\x00" + review.Digest + "\x00" + scope.Owner + "\x00" + scope.Workspace
		}
		nonce := b.confirmBrowser(browserScope(h.ID, in.Page, in.Instance), in.Action, bound)
		if nonce == "" {
			writeErr(w, browserFailure("busy", 409))
			return
		}
		writeJSON(w, 200, map[string]string{"confirmation": nonce})
	}))
	m.HandleFunc("POST /app/hubs/{id}/browser", b.withHubAndHubTimeout(10*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		var in browserCall
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if !decode(w, r, &in) {
			return
		}
		if !validBrowserCall(in) {
			writeErr(w, browserFailure("invalid_argument", 400))
			return
		}
		result, err := b.browserCall(r.Context(), c, h, in)
		if err != nil {
			var consent *browserNeedsConsent
			if errors.As(err, &consent) {
				writeJSON(w, 403, consent.value)
			} else {
				writeErr(w, err)
			}
			return
		}
		writeJSON(w, 200, result)
	}))
}
func validBrowserCall(in browserCall) bool {
	return strings.HasPrefix(in.Page, "pg_") && len(in.Page) <= 100 && in.Instance != "" && len(in.Instance) <= 80 && !strings.ContainsAny(in.Instance, "/\\\x00\r\n")
}
func (b *Backend) browserCall(ctx context.Context, c *client.Client, h Hub, in browserCall) (any, error) {
	comp := b.browserComponent()
	scope := browserScope(h.ID, in.Page, in.Instance)
	if in.Action == "close" {
		if !comp.Status().Running {
			return true, nil
		}
		var out bool
		err := comp.Call(ctx, scope, "DELETE", "/sessions", nil, &out)
		return out, err
	}
	if value, handled, err := b.browserLocalRevocation(h, in); handled {
		return value, err
	}
	page, err := c.Page(ctx, in.Page)
	if err != nil {
		return nil, err
	}
	if isBrowserApproval(in.Action) {
		check := osCallIn{Name: "browser.call", Page: osPage{Hub: h.ID, ID: in.Page, Name: page.Name}}
		if consent, err := b.osCheck(ctx, check); err != nil {
			return nil, browserFailure("permission_denied", 403)
		} else if consent != nil {
			return nil, &browserNeedsConsent{consent}
		}
		return b.browserApprovalCall(ctx, c, h, in)
	}
	status := comp.Status()
	if in.Action == "status" && !status.Installed {
		return api.BrowserStatus{Protocol: 1, Version: "", Supported: status.Supported, Reason: status.Reason, Browsers: []string{}, Machine: b.deviceName(), Profiles: []api.BrowserProfile{}}, nil
	}
	if in.Action == "install" {
		who, err := c.Whoami(ctx)
		if err != nil {
			return nil, err
		}
		if who.Device == nil || !b.consumeBrowserConfirmation(in.Confirmation, scope, "install", "") {
			return nil, browserFailure("permission_denied", 403)
		}
		if err = comp.Install(ctx); err != nil {
			return nil, browserFailure("install_failed", 409)
		}
		status = comp.Status()
		return api.BrowserStatus{Protocol: 1, Version: status.Version, Installed: status.Installed, Supported: status.Supported, Browsers: []string{}, Machine: b.deviceName(), Profiles: []api.BrowserProfile{}}, nil
	}
	if !status.Supported {
		return nil, browserFailure("unsupported", 501)
	}
	if !status.Installed {
		return nil, browserFailure("not_installed", 409)
	}
	check := osCallIn{Name: "browser.call", Page: osPage{Hub: h.ID, ID: in.Page, Name: page.Name}}
	if consent, err := b.osCheck(ctx, check); err != nil {
		return nil, browserFailure("permission_denied", 403)
	} else if consent != nil {
		return nil, &browserNeedsConsent{consent}
	}
	if err = comp.EnsureRunning(ctx); err != nil {
		return nil, browserComponentFailure(err)
	}
	method, path, body := "POST", "", any(in.Args)
	var args map[string]json.RawMessage
	if len(in.Args) > 0 && json.Unmarshal(in.Args, &args) != nil {
		return nil, browserFailure("invalid_argument", 400)
	}
	if args == nil {
		args = map[string]json.RawMessage{}
	}
	text := func(key string) string { var value string; _ = json.Unmarshal(args[key], &value); return value }
	switch in.Action {
	case "status":
		var out api.BrowserStatus
		if err = comp.Call(ctx, scope, "GET", "/status", nil, &out); err != nil {
			return nil, browserComponentFailure(err)
		}
		// Human browser use remains available to legacy/admin credentials;
		// only a resolved native console account can expose agent opt-ins.
		owner, known := b.cachedBrowserOwner(h, in.Page)
		if !known {
			owner, err = b.browserOwner(ctx, c, h, in.Page)
			known = err == nil
		}
		if known {
			policies, err := b.browserPolicies()
			if err != nil {
				return nil, err
			}
			for i := range out.Profiles {
				p, err := policies.Policy(owner, out.Profiles[i].ID)
				if err != nil {
					return nil, browserPolicyError(err)
				}
				out.Profiles[i].Agents = p.Enabled && !b.browserAgentsBlocked(h.ID, p.Profile)
			}
		} else {
			for i := range out.Profiles {
				out.Profiles[i].Agents = false
			}
		}

		out.UpdateAvailable = status.UpdateAvailable
		out.Installed = true
		out.Supported = status.Supported
		if len(out.Browsers) == 0 {
			out.Supported = false
			out.Reason = "no_browser"
		}
		return out, nil
	case "profiles.create":
		path = "/profiles"
		body = map[string]string{"name": text("name")}
	case "profiles.configure":
		inSettings, bound, e := browserConfigureBound(in.Args)
		if e != nil {
			return nil, e
		}
		who, e := c.Whoami(ctx)
		if e != nil {
			return nil, e
		}
		if who.Device == nil || !b.consumeBrowserConfirmation(in.Confirmation, scope, in.Action, bound) {
			return nil, browserFailure("permission_denied", 403)
		}
		var profile api.BrowserProfile
		if e = comp.Call(ctx, scope, "PATCH", "/profiles/"+url.PathEscape(inSettings.ID), map[string]any{"settings": inSettings.Settings}, &profile); e != nil {
			return nil, browserComponentFailure(e)
		}
		if profile.ID != inSettings.ID {
			return nil, browserFailure("incompatible_component", 409)
		}
		// Keep the workspace-local agent policy instead of the legacy global flag.
		if owner, known := b.cachedBrowserOwner(h, in.Page); known {
			if store, e := b.browserPolicies(); e == nil {
				if policy, e := store.Policy(owner, profile.ID); e == nil {
					profile.Agents = policy.Enabled && !b.browserAgentsBlocked(h.ID, profile.ID)
				}
			}
		} else {
			profile.Agents = false
		}
		return profile, nil
	case "profiles.open":
		path = "/sessions"
		body = map[string]any{"profile": text("id"), "headless": false}
	case "profiles.allowAgents":
		var allowed bool
		if json.Unmarshal(args["allowed"], &allowed) != nil {
			return nil, browserFailure("invalid_argument", 400)
		}
		who, err := c.Whoami(ctx)
		if err != nil {
			return nil, err
		}
		if who.Device == nil {
			return nil, browserFailure("permission_denied", 403)
		}
		owner, err := b.browserOwner(ctx, c, h, in.Page)
		if err != nil {
			return nil, err
		}
		if allowed {
			settings, err := c.HubSettings(ctx)
			if err != nil {
				return nil, err
			}
			if settings.E2E {
				return nil, browserFailure("unsupported", 501)
			}
		}
		if allowed && !b.consumeBrowserConfirmation(in.Confirmation, scope, in.Action, text("id")+"\x00"+owner.Owner+"\x00"+owner.Workspace) {
			return nil, browserFailure("permission_denied", 403)
		}
		path = "/profiles/" + url.PathEscape(text("id")) + "/agents"
		body = map[string]bool{"allowed": allowed}
	case "record.active":
		var out struct {
			Recordings []api.BrowserRecording `json:"recordings"`
		}
		if err := comp.Call(ctx, scope, "GET", "/status", nil, &out); err != nil {
			return nil, browserComponentFailure(err)
		}
		if out.Recordings == nil {
			out.Recordings = []api.BrowserRecording{}
		}
		return out.Recordings, nil
	case "record.start":
		path = "/record/start"
		body = map[string]string{"profile": text("profile"), "url": text("url")}
	case "record.stop":
		path = "/record/stop"
		body = map[string]string{"id": text("id")}
	case "record.events":
		method = "GET"
		path = "/record/events?id=" + url.QueryEscape(text("id")) + "&after=" + browserAfter(args["after"])
	case "runs.start":
		if err := browserpolicy.CheckFixedCode(args["script"]); err != nil {
			return nil, browserPolicyError(err)
		}
		var children map[string]json.RawMessage
		if len(args["scripts"]) > 0 && json.Unmarshal(args["scripts"], &children) != nil {
			return nil, browserFailure("invalid_argument", 400)
		}
		for _, child := range children {
			if err := browserpolicy.CheckFixedCode(child); err != nil {
				return nil, browserPolicyError(err)
			}
		}
		// The web page cannot approve local file uploads by adding private fields.
		delete(args, "uploads")
		picked := []string{}
		g := b.grants()
		g.mu.Lock()
		picked = append(picked, g.perm(h.ID, in.Page).picked...)
		g.mu.Unlock()
		paths := []string{}
		for _, path := range picked {
			if real, err := filepath.EvalSymlinks(path); err == nil {
				paths = append(paths, real)
			}
		}
		raw, _ := json.Marshal(paths)
		args["uploads"] = raw
		body = args
		path = "/runs"
	case "runs.get":
		method = "GET"
		path = "/runs/" + url.PathEscape(text("id")) + "?after=" + browserAfter(args["after"])
	case "runs.control":
		action := text("action")
		if action != "pause" && action != "resume" && action != "step" && action != "cancel" {
			return nil, browserFailure("invalid_argument", 400)
		}
		path = "/runs/" + url.PathEscape(text("id")) + "/" + action
		body = map[string]string{"choice": text("choice")}
	case "locate":
		path = "/locate"
		body = map[string]any{"profile": text("profile"), "target": args["target"]}
	case "pick":
		path = "/pick"
		body = map[string]string{"profile": text("profile")}
	case "snapshot":
		data, kind, err := comp.Artifact(ctx, scope, text("id"))
		if err != nil {
			return nil, browserComponentFailure(err)
		}
		return map[string]string{"base64": base64.StdEncoding.EncodeToString(data), "type": kind}, nil
	default:
		return nil, browserFailure("unsupported", 400)
	}
	var out any
	if err = comp.Call(ctx, scope, method, path, body, &out); err != nil {
		return nil, browserComponentFailure(err)
	}
	if in.Action == "profiles.allowAgents" {
		owner, err := b.browserOwner(ctx, c, h, in.Page)
		if err != nil {
			return nil, err
		}
		var allowed bool
		_ = json.Unmarshal(args["allowed"], &allowed)
		raw, _ := json.Marshal(out)
		var profile api.BrowserProfile
		if json.Unmarshal(raw, &profile) != nil || profile.ID != text("id") {
			return nil, browserFailure("incompatible_component", 409)
		}
		store, err := b.browserPolicies()
		if err != nil {
			return nil, err
		}
		b.blockBrowserAgents(h.ID, profile.ID)
		if _, err := store.SetEnabled(owner, profile.ID, allowed, profile.Name); err != nil {
			return nil, browserPolicyError(err)
		}
		b.unblockBrowserAgents(h.ID, profile.ID)
		profile.Agents = allowed
		if b.Local != nil && b.Store != nil {
			go b.reconcileActionHosts()
		}
		return profile, nil
	}
	if in.Action == "profiles.open" || in.Action == "record.stop" || in.Action == "runs.control" {
		return true, nil
	}
	return out, nil
}
func browserAfter(raw json.RawMessage) string {
	var n uint64
	if len(raw) == 0 {
		return "0"
	}
	if json.Unmarshal(raw, &n) != nil {
		return "invalid"
	}
	value, _ := json.Marshal(n)
	return string(value)
}
func browserError(err error) string {
	switch err.Error() {
	case "browser_missing":
		return "no_browser"
	case "target_not_found":
		return "not_found"
	case "component_not_installed":
		return "not_installed"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	for _, code := range []string{"sensitive_snapshot", "busy", "not_found", "cancelled", "timeout", "permission_denied", "invalid_argument", "unsupported", "incompatible_component", "profile_locked", "sensitive_literal", "secret_required", "invalid_graph", "unsupported_node", "artifact_limit", "recording_limit"} {
		if err.Error() == code {
			return code
		}
	}
	return "browser_failed"
}

type browserNeedsConsent struct{ value *osConsent }

func (e *browserNeedsConsent) Error() string { return "consent_required" }

// Closing a page or withdrawing its local grant never waits for its Hub.
func (b *Backend) stopBrowserPage(hub, page string) {
	b.browser.mu.Lock()
	comp := b.browser.component
	b.browser.mu.Unlock()
	if comp == nil || !comp.Status().Running {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bool
	_ = comp.Call(ctx, browserScope(hub, page, ""), "DELETE", "/sessions", nil, &out)
}

// The feature marker is kept out of the static script so other consoles and
// unsupported platforms do not claim a usable native browser component.
func (b *Backend) browserPageScript() string {
	enabled := false
	if b.Local != nil {
		status := b.browserComponent().Status()
		enabled = status.Supported
	}
	addition := ""
	if enabled {
		addition = `.concat(["browser"])`
	}
	return strings.Replace(pageHelperJS, "/*__MYCO_BROWSER_FEATURE__*/", addition, 1)
}

type browserReasonError struct {
	cause  *client.Error
	reason string
}

func (e *browserReasonError) Error() string         { return e.cause.Error() }
func (e *browserReasonError) Unwrap() error         { return e.cause }
func (e *browserReasonError) BrowserReason() string { return e.reason }
func browserComponentFailure(err error) error {
	var pe *browserautomation.ProfileError
	if errors.As(err, &pe) {
		reason := pe.Reason
		switch reason {
		case "leftover_unverified", "cleanup_pending", "cleanup_failed":
		default:
			reason = "leftover_unverified"
		}
		return &browserReasonError{cause: &client.Error{Code: "profile_locked", Status: 409, Message: "This profile's previous browser is still running. Close it and retry, or restart your computer."}, reason: reason}
	}
	return browserFailure(browserError(err), 409)
}
