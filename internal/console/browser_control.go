package console

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

const (
	browserControlIdle       = 5 * time.Minute
	browserControlPoll       = 2 * time.Second
	browserControlGrantGrace = 30 * time.Second
)

type browserControlSession struct {
	mu sync.Mutex

	// Immutable after reservation.
	profile, hub, binding, agent, grant, scope string
	expires                                    time.Time

	ctx    context.Context
	cancel context.CancelFunc

	// Protected by mu. A closed entry remains until its grant expires.
	id, reason, host string
	closeRequestID   string
	lastActive       time.Time
	replaying        bool
}

func (b *Backend) registerBrowserHost(ctx context.Context, c *client.Client, machine string, p browserHostPolicy) (*api.ActionHost, error) {
	req := api.ActionHostRegister{
		MachineID: machine, Provider: "browser", Generation: "browser-" + rand.Text(),
		Resource: &api.ActionResource{Provider: "browser", ID: p.Policy.Profile, Binding: p.Policy.Binding, Name: p.Policy.Name, Kind: p.Policy.Kind, Summary: "Owner-approved fixed script versions"},
		Actions:  browserActionList,
	}
	if b.browserControlAvailable(ctx, "control-discovery") {
		descriptors := api.BrowserControlDescriptors()
		if !b.browserControlAvailable(ctx, "control-discovery", api.BrowserControlFeature, api.BrowserReplayFeature) {
			descriptors = slices.DeleteFunc(descriptors, func(d api.ActionDescriptor) bool { return d.Name == "browser.page.run" })
		}
		req.Actions = append(slices.Clone(req.Actions), descriptors...)
		req.BrowserHostScope = true
		host, err := c.RegisterActionHost(ctx, req)
		ce, ok := client.AsError(err)
		if ok && ce.Code == "invalid_argument" && strings.Contains(ce.Message, "browser.page.run") {
			req.Actions = slices.DeleteFunc(req.Actions, func(d api.ActionDescriptor) bool { return d.Name == "browser.page.run" })
			host, err = c.RegisterActionHost(ctx, req)
			ce, ok = client.AsError(err)
		}
		if !ok || ce.Code != "invalid_argument" || !strings.Contains(ce.Message, "browser.page.") {
			return host, err
		}
		req.Actions = browserActionList
		req.BrowserHostScope = false
	}
	return c.RegisterActionHost(ctx, req)
}

func (b *Backend) provisionBrowserAgentProfile(h Hub, scope browserpolicy.Scope, store *browserpolicy.Store) (browserpolicy.Policy, bool) {
	b.browser.mu.Lock()
	if time.Now().Before(b.browser.nextAgentProfileProbe[scope]) {
		b.browser.mu.Unlock()
		return browserpolicy.Policy{}, false
	}
	if b.browser.nextAgentProfileProbe == nil {
		b.browser.nextAgentProfileProbe = make(map[browserpolicy.Scope]time.Time)
	}
	b.browser.nextAgentProfileProbe[scope] = time.Now().Add(2 * time.Minute)
	b.browser.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c, err := b.client(h)
	if err != nil {
		return browserpolicy.Policy{}, false
	}
	settings, err := c.HubSettings(ctx)
	if err != nil || settings.E2E {
		return browserpolicy.Policy{}, false
	}
	if _, err = b.thisMachine(ctx, c); err != nil || !b.browserControlAvailable(ctx, "control-discovery") {
		return browserpolicy.Policy{}, false
	}
	native := b.browserComponent()
	allowCreate, err := store.AgentProfileCreationAllowed()
	if err != nil {
		return browserpolicy.Policy{}, false
	}
	profile, dedicated, err := b.resolveBrowserAgentProfile(ctx, native, allowCreate)
	if err != nil {
		return browserpolicy.Policy{}, false
	}
	p, err := store.EnsureAgentProfile(scope, func() (string, string, error) { return profile.ID, profile.Name, nil }, func(id string) error {
		return native.Call(ctx, "control-discovery", http.MethodPost, "/profiles/"+url.PathEscape(id)+"/agents", map[string]bool{"allowed": true}, nil)
	}, dedicated...)
	return p, err == nil
}

// Native profiles are machine-wide; approval policies are deliberately not.
// Serialize discovery/create across workspaces, including older components
// whose POST /profiles is not itself idempotent.
func (b *Backend) resolveBrowserAgentProfile(ctx context.Context, native browserComponentAPI, allowCreate ...bool) (api.BrowserProfile, []string, error) {
	b.browser.profileMu.Lock()
	defer b.browser.profileMu.Unlock()
	var status api.BrowserStatus
	if err := native.Call(ctx, "control-discovery", http.MethodGet, "/status", nil, &status); err != nil {
		return api.BrowserProfile{}, nil, err
	}
	var canonical api.BrowserProfile
	ids := []string{}
	for _, p := range status.Profiles {
		if p.Kind != api.BrowserAgentProfile {
			continue
		}
		if !validBrowserID(p.ID, "p_", 80) {
			return api.BrowserProfile{}, nil, errors.New("invalid dedicated profile")
		}
		ids = append(ids, p.ID)
		if canonical.ID == "" || p.ID < canonical.ID {
			canonical = p
		}
	}
	if canonical.ID == "" {
		if len(allowCreate) > 0 && !allowCreate[0] {
			return api.BrowserProfile{}, nil, errors.New("dedicated profile creation disabled")
		}
		if err := native.Call(ctx, "control-discovery", http.MethodPost, "/profiles", map[string]string{"name": "agent 专用", "kind": api.BrowserAgentProfile}, &canonical); err != nil {
			return api.BrowserProfile{}, nil, err
		}
		if canonical.Kind != api.BrowserAgentProfile || !validBrowserID(canonical.ID, "p_", 80) {
			return api.BrowserProfile{}, nil, errors.New("invalid dedicated profile")
		}
		ids = append(ids, canonical.ID)
	}
	return canonical, ids, nil
}

func (b *Backend) browserControlAvailable(ctx context.Context, scope string, required ...string) bool {
	comp := b.browserComponent()
	if comp.EnsureRunning(ctx) != nil {
		return false
	}
	var status api.BrowserStatus
	if len(required) == 0 {
		required = []string{api.BrowserControlFeature}
	}
	if comp.Call(ctx, scope, http.MethodGet, "/status", nil, &status) != nil || status.Protocol != 1 {
		return false
	}
	for _, feature := range required {
		if !slices.Contains(status.Features, feature) {
			return false
		}
	}
	return true
}

// controlBusy and reserveBrowserControl share the run-book lock with M2.
func (book *browserRunBook) controlBusy(profile string) bool {
	for key, s := range book.controls {
		s.mu.Lock()
		closed, expires := s.reason != "", s.expires
		s.mu.Unlock()
		if closed && !time.Now().Before(expires) {
			delete(book.controls, key)
		} else if !closed && s.profile == profile {
			return true
		}
	}
	return false
}

func (b *Backend) reserveBrowserControl(ctx context.Context, h Hub, p browserHostPolicy, g api.ActionGrant, key string) *browserControlSession {
	book := b.browserRuns()
	book.mu.Lock()
	defer book.mu.Unlock()
	if book.controlBusy(p.Policy.Profile) {
		return nil
	}
	for _, r := range book.runs {
		r.mu.Lock()
		busy := r.profile == p.Policy.Profile && !r.closed &&
			(r.id == "" || r.update.State == "running" || r.update.State == "paused")
		r.mu.Unlock()
		if busy {
			return nil
		}
	}
	if len(book.controls) >= 512 {
		return nil
	}
	run, cancel := context.WithDeadline(ctx, *g.ExpiresAt)
	s := &browserControlSession{
		profile: p.Policy.Profile, hub: h.ID, binding: p.Policy.Binding, agent: g.Agent, grant: g.ID,
		scope: browserNativeScope(p, g), expires: *g.ExpiresAt, ctx: run, cancel: cancel, lastActive: time.Now(),
	}
	book.controls[key] = s
	return s
}

func (b *Backend) ownedBrowserControl(id, hub, agent, grant, profile, binding string) *browserControlSession {
	book := b.browserRuns()
	book.mu.Lock()
	defer book.mu.Unlock()
	s := book.controls[id]
	if s == nil || s.hub != hub || s.agent != agent || s.grant != grant || s.profile != profile || s.binding != binding {
		return nil
	}
	return s
}

// markClosed wins the race with other teardown paths and cancels in-flight calls.
func (s *browserControlSession) markClosed(reason string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason != "" {
		return "", false
	}
	s.reason = reason
	s.cancel()
	return s.id, true
}

func (b *Backend) closeBrowserControl(s *browserControlSession, reason string, nativeAlreadyClosed bool) {
	id, changed := s.markClosed(reason)
	if changed && id != "" && !nativeAlreadyClosed {
		b.closeBrowserControlNative(s.scope, id, reason)
	}
}

func (b *Backend) closeBrowserControlNative(scope, id, reason string) {
	switch reason {
	case "grant_expired":
		reason = "grant_ended"
	case "grant_ended", "profile_disabled", "idle_timeout":
	default:
		reason = "host_closed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = b.browserComponent().Call(ctx, scope, http.MethodPost, "/control/close", map[string]string{
		"session": id, "request_id": "close-" + rand.Text(), "reason": reason,
	}, nil)
}

func (b *Backend) stopBrowserControls(hub, profile, binding, reason string) {
	book := b.browserRuns()
	book.mu.Lock()
	var sessions []*browserControlSession
	for _, s := range book.controls {
		if s.hub == hub && s.profile == profile && (binding == "" || s.binding == binding) {
			sessions = append(sessions, s)
		}
	}
	book.mu.Unlock()
	for _, s := range sessions {
		b.closeBrowserControl(s, reason, false)
	}
}

func browserControlClosed(reason string) api.ActionJobComplete {
	code := "session_closed"
	if reason == "idle_timeout" || reason == "grant_expired" || reason == "expired" {
		code = "session_expired"
	}
	out := actionFailure(code, "the browser control session ended", false, nil)
	out.Error.Reason = reason
	return out
}

func browserControlFailure(err error, effectful bool) api.ActionJobComplete {
	var native *browserautomation.ComponentError
	if errors.As(err, &native) {
		switch native.Code {
		case "invalid_argument", "unsupported", "permission_denied", "not_found", "busy", "conflict",
			"assertion_failed", "stale_ref", "target_not_found", "target_not_interactable", "sensitive_field", "sensitive_snapshot",
			"session_closed", "session_expired", "cancelled", "timeout", "recording_limit", "artifact_too_large",
			"artifact_limit", "component_unavailable", "storage_failed", "outcome_unknown", "host_not_allowed":
			out := actionFailure(native.Code, "browser control: "+strings.ReplaceAll(native.Code, "_", " "), false, native.Applied)
			out.Error.Reason = browserControlReason(native.Reason)
			if validBrowserID(native.ErrorNode, "", 80) {
				out.Error.ErrorNode = native.ErrorNode
			}
			if out.Error.Reason == "owner_window_open" {
				out.Error.Message = "Close this profile's browser window before opening agent control."
			}
			if api.ValidBrowserAuditHost(native.Host) {
				out.BrowserHost = native.Host
			}
			return out
		}
	}
	if effectful && err.Error() != "component_not_running" {
		return actionFailure("outcome_unknown", "the component did not confirm the outcome", false, nil)
	}
	return actionFailure("component_unavailable", "the browser component did not respond", true, nil)
}

func browserControlReason(reason string) string {
	switch reason {
	case "closed", api.BrowserControlOwnerStopped, "idle_timeout", "grant_expired", "grant_ended", "grant_check_failed",
		"expired", "profile_disabled", "owner_window_open", "profile_in_use", "profile_starting", "host_closed", "app_exit", "browser_closed", "cleanup_pending", "component_unavailable", "cancelled":
		return reason
	default:
		return ""
	}
}

func (b *Backend) watchBrowserControl(c *client.Client, s *browserControlSession, id string) {
	ctx := s.ctx
	grantCheck := browserControlGrantCheck{}
	tick := time.NewTicker(browserControlPoll)
	defer tick.Stop()
	after := 0
	for {
		select {
		case <-ctx.Done():
			reason := "host_closed"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				reason = "grant_expired"
			}
			b.closeBrowserControl(s, reason, false)
			return
		case <-tick.C:
		}

		// The component's event wait is bounded locally so remote revocation
		// is checked even when no actions are running.
		check, cancel := context.WithTimeout(ctx, browserControlPoll)
		var events api.BrowserControlEvents
		query := url.Values{"session": {id}, "after": {strconv.Itoa(after)}}
		err := b.browserComponent().Call(check, s.scope, http.MethodGet, "/control/events?"+query.Encode(), nil, &events)
		timedOut := errors.Is(check.Err(), context.DeadlineExceeded)
		cancel()
		if err == nil {
			after = max(after, events.Next)
			for _, event := range events.Events {
				if event.Code != "host_not_allowed" && event.Host != "" && api.ValidBrowserAuditHost(event.Host) {
					s.mu.Lock()
					s.host = event.Host
					s.mu.Unlock()
				}
			}
			if events.Done {
				reason := browserControlReason(events.Reason)
				if reason == "" {
					reason = "closed"
				}
				b.closeBrowserControl(s, reason, true)
				if reason == api.BrowserControlOwnerStopped {
					b.revokeStoppedBrowserControl(c, s.grant, s.expires)
				}
				return
			}
		} else if !timedOut && ctx.Err() == nil {
			b.closeBrowserControl(s, "component_unavailable", false)
			return
		}
		check, cancel = context.WithTimeout(ctx, 5*time.Second)
		// A session outlives host generations and completed jobs. Existing
		// Hubs expose owner grants only as a list; inspect this grant's ID,
		// including terminal states, independently of any host/job identity.
		grants, err := c.OwnerActionGrants(check, "")
		cancel()
		if reason := grantCheck.reason(time.Now(), s.grant, s.expires, grants, err); reason != "" {
			b.closeBrowserControl(s, reason, false)
			return
		}
		s.mu.Lock()
		idle := !s.replaying && time.Since(s.lastActive) >= browserControlIdle
		s.mu.Unlock()
		if idle {
			b.closeBrowserControl(s, "idle_timeout", false)
			return
		}
	}
}

type browserControlGrantCheck struct {
	failedSince time.Time
}

func (g *browserControlGrantCheck) reason(now time.Time, id string, expires time.Time, grants []api.ActionGrant, err error) string {
	// The local grant deadline always wins, even during a Hub outage.
	if !now.Before(expires) {
		return "grant_expired"
	}
	if err == nil {
		for _, grant := range grants {
			if grant.ID != id {
				continue
			}
			switch grant.State {
			case api.GrantExpired:
				return "grant_expired"
			case api.GrantRevoked, api.GrantEnded, api.GrantDenied:
				return "grant_ended"
			case api.GrantActive:
				if grant.ExpiresAt != nil {
					if !now.Before(*grant.ExpiresAt) {
						return "grant_expired"
					}
					g.failedSince = time.Time{}
					return ""
				}
			}
			break
		}
	}
	// Absence, malformed data and HTTP failures are not proof of revocation.
	// Bound consecutive uncertainty; a successful check resets this window.
	if g.failedSince.IsZero() {
		g.failedSince = now
	}
	if now.Sub(g.failedSince) >= browserControlGrantGrace {
		return "grant_check_failed"
	}
	return ""
}

func (b *Backend) revokeStoppedBrowserControl(c *client.Client, grant string, expires time.Time) {
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	defer cancel()
	for ctx.Err() == nil {
		check, stop := context.WithTimeout(ctx, 5*time.Second)
		_, err := c.RevokeActionGrantWithReason(check, grant, api.BrowserControlOwnerStopped)
		stop()
		if err == nil {
			return
		}
		if ce, ok := client.AsError(err); ok && ce.Status >= 400 && ce.Status < 500 {
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(browserControlPoll):
		}
	}
}

func (b *Backend) runBrowserControlJob(hostCtx context.Context, c *client.Client, host *api.ActionHost, hj api.ActionHostJob, h Hub, p browserHostPolicy) api.ActionJobComplete {
	g := hj.Grant
	args, err := api.DecodeBrowserControlArgs(hj.Job.Action, hj.Args)
	if err != nil {
		out := actionFailure("invalid_argument", err.Error(), false, nil)
		var invalid *api.BrowserReplayValidationError
		if errors.As(err, &invalid) {
			out.Error.ErrorNode = invalid.Node
		}
		return out
	}
	op := strings.TrimPrefix(hj.Job.Action, "browser.page.")
	if op == "open" && args.String("profile") != p.Policy.Profile {
		return actionFailure("permission_denied", "profile is outside the grant", false, nil)
	}
	jobCtx, stop, _, early := watchHostJob(hostCtx, c, host, hj.Job.ID)
	if early != nil {
		return *early
	}
	defer stop()

	if op == "open" {
		return b.openBrowserControl(hostCtx, jobCtx, c, hj, h, p, args)
	}
	s := b.ownedBrowserControl(args.String("session"), h.ID, g.Agent, g.ID, p.Policy.Profile, p.Policy.Binding)
	if s == nil {
		return actionFailure("not_found", "no such browser control session for this grant", false, nil)
	}
	s.mu.Lock()
	reason, auditHost := s.reason, s.host
	if reason == "" {
		s.lastActive = time.Now()
	}
	requestID := hj.Job.ID
	if op == "close" {
		if s.closeRequestID == "" {
			s.closeRequestID = requestID
		}
		requestID = s.closeRequestID
	}
	s.mu.Unlock()
	if reason != "" && !(op == "close" && reason == "closed") {
		return browserControlClosed(reason)
	}
	ctx, cancel := context.WithDeadline(jobCtx, s.expires)
	defer cancel()
	// A voluntary close ends the session before its response necessarily arrives.
	// Keep receipt retrieval bound to the job/grant, not that ended session.
	if op != "close" {
		unwatch := context.AfterFunc(s.ctx, cancel)
		defer unwatch()
	}
	args["request_id"], _ = json.Marshal(requestID)

	if op == "run" {
		if !b.browserControlAvailable(ctx, s.scope, api.BrowserControlFeature, api.BrowserReplayFeature) {
			return actionFailure("unsupported", "the browser component does not offer agent replay", false, nil)
		}
		s.mu.Lock()
		s.replaying = true
		s.lastActive = time.Now()
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.replaying = false; s.lastActive = time.Now(); s.mu.Unlock() }()
	}

	var result json.RawMessage
	err = b.browserComponent().Call(ctx, s.scope, http.MethodPost, "/control/"+op, args, &result)
	if err != nil {
		if op == "run" {
			s.mu.Lock()
			reason := s.reason
			s.mu.Unlock()
			if reason != "" {
				return browserControlClosed(reason)
			}
			if !time.Now().Before(s.expires) {
				return browserControlClosed("grant_expired")
			}
		}
		var native *browserautomation.ComponentError
		if op == "close" && errors.As(err, &native) && native.Code != "outcome_unknown" {
			// A definite native error (for example busy or cleanup_pending)
			// may need a fresh close attempt. Ambiguous transport outcomes reuse
			// the original ID to retrieve its retained response safely.
			s.mu.Lock()
			if s.closeRequestID == requestID {
				s.closeRequestID = ""
			}
			s.mu.Unlock()
		}
		done := browserControlFailure(err, op == "act" || op == "close" || op == "run")
		if done.BrowserHost == "" {
			done.BrowserHost = auditHost
		}
		if done.Error.Code == "session_closed" || done.Error.Code == "session_expired" {
			reason := done.Error.Reason
			if reason == "" {
				reason = "closed"
			}
			b.closeBrowserControl(s, reason, true)
			if reason == api.BrowserControlOwnerStopped {
				go b.revokeStoppedBrowserControl(c, s.grant, s.expires)
			}
		}
		return done
	}
	if op == "close" {
		b.closeBrowserControl(s, "closed", true)
	} else {
		s.mu.Lock()
		s.lastActive = time.Now()
		s.mu.Unlock()
	}
	done := b.browserControlResult(ctx, c, host, hj.Job.ID, s.scope, op, result, auditHost)
	if done.BrowserHost != "" {
		s.mu.Lock()
		s.host = done.BrowserHost
		s.mu.Unlock()
	}
	return done
}

func (b *Backend) openBrowserControl(hostCtx, jobCtx context.Context, c *client.Client, hj api.ActionHostJob, h Hub, p browserHostPolicy, args api.BrowserControlArgs) api.ActionJobComplete {
	g := hj.Grant
	key := "pending:" + hj.Job.ID
	s := b.reserveBrowserControl(hostCtx, h, p, g, key)
	if s == nil {
		return actionFailure("busy", "this browser profile already has a controller", false, nil)
	}
	book := b.browserRuns()
	// The open can race profile shutdown or a grant expiry before the
	// component has returned its session ID.
	var result json.RawMessage
	if !b.browserControlAvailable(jobCtx, s.scope) {
		b.closeBrowserControl(s, "component_unavailable", true)
		book.mu.Lock()
		delete(book.controls, key)
		book.mu.Unlock()
		return actionFailure("unsupported", "the browser component does not offer live control", false, nil)
	}
	args["request_id"], _ = json.Marshal(hj.Job.ID)
	args["grant_id"], _ = json.Marshal(g.ID)
	if len(g.Hosts) != 0 {
		args["hosts"], _ = json.Marshal(g.Hosts)
	}
	actor := api.BrowserControlActor{ID: g.Agent, Name: g.Agent}
	labelCtx, labelCancel := context.WithTimeout(jobCtx, 2*time.Second)
	if agent, err := c.GetAgent(labelCtx, g.Agent); err == nil && agent.DisplayName != "" && len(agent.DisplayName) <= 160 {
		actor.Name = agent.DisplayName
	}
	labelCancel()
	args["actor"], _ = json.Marshal(actor)
	args["expires_at"], _ = json.Marshal(g.ExpiresAt)
	ctx, cancel := context.WithCancel(jobCtx)
	defer cancel()
	unwatch := context.AfterFunc(s.ctx, cancel)
	defer unwatch()
	err := b.browserComponent().Call(ctx, s.scope, http.MethodPost, "/control/open", args, &result)
	var opened api.BrowserControlStatus
	jsonErr := json.Unmarshal(result, &opened)
	book.mu.Lock()
	delete(book.controls, key)
	s.mu.Lock()
	reason := s.reason
	aborted := reason != "" || ctx.Err() != nil
	if err == nil && jsonErr == nil && opened.Session != "" && book.controls[opened.Session] == nil && !aborted {
		s.id = opened.Session
		s.lastActive = time.Now()
		book.controls[s.id] = s
	}
	id := s.id
	s.mu.Unlock()
	book.mu.Unlock()
	if id == "" {
		if opened.Session != "" && aborted {
			b.closeBrowserControlNative(s.scope, opened.Session, reason)
		}
		b.closeBrowserControl(s, "closed", true)
		if err != nil {
			return browserControlFailure(err, true)
		}
		return actionFailure("outcome_unknown", "the component returned no usable control session", false, nil)
	}
	auditHost := browserControlResultHost(result)
	s.mu.Lock()
	s.host = auditHost
	s.mu.Unlock()
	go b.watchBrowserControl(c, s, id)
	return api.ActionJobComplete{State: api.JobSucceeded, Result: result, BrowserHost: auditHost}
}

func (b *Backend) browserControlResult(ctx context.Context, c *client.Client, host *api.ActionHost, job, scope, op string, raw json.RawMessage, fallbackHost string) api.ActionJobComplete {
	auditHost := browserControlResultHost(raw)
	if auditHost == "" {
		auditHost = fallbackHost
	}
	out := api.ActionJobComplete{State: api.JobSucceeded, Result: raw, BrowserHost: auditHost}
	fail := func(code, message string) api.ActionJobComplete {
		failure := actionFailure(code, message, false, nil)
		failure.BrowserHost = auditHost
		return failure
	}
	if op == "run" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return fail("component_unavailable", "the component returned an invalid replay result")
		}
		var output map[string]json.RawMessage
		if json.Unmarshal(fields["output"], &output) != nil {
			return fail("component_unavailable", "the component returned invalid replay output")
		}
		keys := make([]string, 0, len(output))
		for key := range output {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			var image struct {
				Screenshot *struct {
					ID string `json:"id"`
				} `json:"screenshot"`
			}
			if json.Unmarshal(output[key], &image) != nil || image.Screenshot == nil {
				continue
			}
			if !validBrowserID(image.Screenshot.ID, "a_", 80) {
				return fail("component_unavailable", "the component returned an invalid private image")
			}
			uploaded := b.browserControlResult(ctx, c, host, job, scope, "screenshot", output[key], auditHost)
			if uploaded.State != api.JobSucceeded {
				return uploaded
			}
			var value map[string]json.RawMessage
			if len(uploaded.ArtifactIDs) != 1 || json.Unmarshal(uploaded.Result, &value) != nil {
				return fail("storage_failed", "could not store replay screenshot")
			}
			value["artifact_id"], _ = json.Marshal(uploaded.ArtifactIDs[0])
			output[key], _ = json.Marshal(value)
			out.ArtifactIDs = append(out.ArtifactIDs, uploaded.ArtifactIDs...)
		}
		fields["output"], _ = json.Marshal(output)
		out.Result, _ = json.Marshal(fields)
		return out
	}
	var result struct {
		Screenshot *struct {
			ID string `json:"id"`
		} `json:"screenshot"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return fail("component_unavailable", "the browser component returned an invalid result")
	}
	if result.Screenshot == nil || result.Screenshot.ID == "" {
		if op == "screenshot" {
			return fail("component_unavailable", "the browser component returned no screenshot")
		}
		return out
	}
	data, kind, err := b.browserComponent().Artifact(ctx, scope, result.Screenshot.ID)
	if err != nil {
		return fail("storage_failed", "could not read the private screenshot")
	}
	if len(data) == 0 || len(data) > 4<<20 {
		return fail("artifact_too_large", "the private screenshot exceeded the artifact limit")
	}
	if kind != "image/png" && kind != "image/jpeg" && kind != "image/webp" {
		return fail("storage_failed", "unexpected screenshot media type")
	}
	file, err := c.UploadActionArtifact(ctx, job, host.ID, host.Generation, kind, data)
	if err != nil {
		return fail("storage_failed", "could not store the private screenshot")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "screenshot")
	out.Result, _ = json.Marshal(fields)
	out.ArtifactIDs = []string{file.ID}
	return out
}

func browserControlResultHost(raw json.RawMessage) string {
	var result struct {
		Host     string          `json:"host"`
		URL      string          `json:"url"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return ""
	}
	if result.Host != "" && api.ValidBrowserAuditHost(result.Host) {
		return result.Host
	}
	if parsed, err := url.Parse(result.URL); err == nil && parsed.Hostname() != "" && api.ValidBrowserAuditHost(parsed.Hostname()) {
		return parsed.Hostname()
	}
	if len(result.Snapshot) > 0 {
		return browserControlResultHost(result.Snapshot)
	}
	return ""
}
