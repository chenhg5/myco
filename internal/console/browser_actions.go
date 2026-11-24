package console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type browserHostPolicy struct {
	Scope  browserpolicy.Scope
	Policy browserpolicy.Policy
}

var browserActionList = func() []api.ActionDescriptor {
	var out []api.ActionDescriptor
	for _, name := range []string{"browser.profiles.list", "browser.script.run", "browser.run.status", "browser.run.resume", "browser.run.cancel"} {
		schema := `{"type":"object","properties":{},"additionalProperties":false}`
		if name == "browser.script.run" {
			schema = `{"type":"object","properties":{"script_id":{"type":"string"},"version":{"type":"integer","minimum":1},"page_id":{"type":"string"},"params":{"type":"object","additionalProperties":{"type":"string"}},"headless":{"type":"boolean"}},"required":["script_id","version"],"additionalProperties":false}`
		} else if strings.HasPrefix(name, "browser.run.") {
			schema = `{"type":"object","properties":{"run_id":{"type":"string"}},"required":["run_id"],"additionalProperties":false}`
			if name == "browser.run.resume" {
				schema = `{"type":"object","properties":{"run_id":{"type":"string"},"choice":{"type":"string","enum":["ok","cancel"]}},"required":["run_id"],"additionalProperties":false}`
			}
		}
		out = append(out, api.ActionDescriptor{Name: name, Version: 1, Title: name, Permission: api.BrowserActionPermission(name), InputSchema: json.RawMessage(schema)})
	}
	return out
}()

// Discovery reads native persisted opt-ins without keeping the component alive.
func (b *Backend) browserHostPolicies(h Hub) []browserHostPolicy {
	st := b.browserComponent().Status()
	if !st.Installed || !st.Supported || st.Protocol != 1 || !browserautomation.CompatibleComponentVersion(st.Version) {
		return nil
	}
	scope, ok := b.cachedBrowserOwner(h, "")
	if !ok {
		c, e := b.client(h)
		if e != nil {
			return nil
		}
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		scope, e = b.browserOwner(ctx, c, h, "")
		if e != nil {
			return nil
		}
	}
	store, e := b.browserPolicies()
	if e != nil {
		return nil
	}
	ps, e := store.Profiles(scope)
	if e != nil {
		return nil
	}
	var out []browserHostPolicy
	for _, p := range ps {
		if p.Enabled && p.Binding != "" && !b.browserAgentsBlocked(h.ID, p.Profile) {
			out = append(out, browserHostPolicy{scope, p})
		}
	}
	return out
}
func (b *Backend) browserPolicyCurrent(h Hub, p browserHostPolicy) bool {
	scope, ok := b.cachedBrowserOwner(h, "")
	if !ok || scope != p.Scope || b.browserAgentsBlocked(h.ID, p.Policy.Profile) {
		return false
	}
	s, e := b.browserPolicies()
	if e != nil {
		return false
	}
	ps, e := s.Profiles(scope)
	if e != nil {
		return false
	}
	for _, v := range ps {
		if v.Profile == p.Policy.Profile {
			return v.Enabled && v.Binding == p.Policy.Binding
		}
	}
	return false
}
func (b *Backend) browserRuns() *browserRunBook {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	if b.browser.runs == nil {
		b.browser.runs = &browserRunBook{runs: map[string]*browserAgentRun{}}
	}
	if b.browser.cancelAgents == nil {
		b.browser.cancelAgents = b.stopBrowserProfile
	}
	return b.browser.runs
}
func (b *Backend) stopBrowserProfile(hub, profile string) {
	book := b.browserRuns()
	book.mu.Lock()
	var runs []*browserAgentRun
	for _, r := range book.runs {
		if r.hub == hub && r.profile == profile {
			runs = append(runs, r)
		}
	}
	book.mu.Unlock()
	for _, r := range runs {
		b.cancelBrowserRun(r)
	}
	b.actionHosts.mu.Lock()
	for key, stop := range b.actionHosts.running {
		if strings.HasPrefix(key, "browser:"+hub+":") && strings.Contains(key, ":"+profile+":") {
			stop()
		}
	}
	b.actionHosts.mu.Unlock()
}
func (b *Backend) hostBrowser(ctx context.Context, h Hub, p browserHostPolicy) time.Duration {
	b.browserRuns()
	store, e := b.browserPolicies()
	if e != nil {
		return time.Minute
	}
	release, e := store.HostLease(p.Scope, p.Policy.Profile)
	if e != nil {
		return actionHostRetake
	}
	defer release()
	c, e := b.client(h)
	if e != nil {
		return time.Minute
	}
	check, stop := context.WithTimeout(ctx, 10*time.Second)
	settings, e := c.HubSettings(check)
	stop()
	if e != nil || settings.E2E {
		return actionHostRetake
	}
	check, stop = context.WithTimeout(ctx, 10*time.Second)
	machine, e := b.thisMachine(check, c)
	stop()
	if e != nil {
		return time.Minute
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
				if !b.browserPolicyCurrent(h, p) {
					cancel()
					return
				}
			}
		}
	}()
	var host *api.ActionHost
	defer func() {
		if run.Err() != nil {
			book := b.browserRuns()
			book.mu.Lock()
			var owned []*browserAgentRun
			for _, r := range book.runs {
				if r.hub == h.ID && r.profile == p.Policy.Profile && r.binding == p.Policy.Binding {
					owned = append(owned, r)
				}
			}
			book.mu.Unlock()
			for _, r := range owned {
				b.cancelBrowserRun(r)
			}
			if host != nil {
				end, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				_ = c.UnregisterActionHost(end, host.ID, host.Generation)
			}
		}
	}()
	for run.Err() == nil {
		if !b.browserPolicyCurrent(h, p) {
			cancel()
			break
		}
		check, stop = context.WithTimeout(run, 10*time.Second)
		host, e = c.RegisterActionHost(check, api.ActionHostRegister{MachineID: machine, Provider: "browser", Resource: &api.ActionResource{Provider: "browser", ID: p.Policy.Profile, Binding: p.Policy.Binding, Name: p.Policy.Name, Summary: "Owner-approved fixed script versions"}, Generation: "browser-" + rand.Text(), Actions: browserActionList})
		stop()
		if e == nil {
			for run.Err() == nil {
				jobs, err := c.ActionHostJobs(run, host.ID, actionHostPoll)
				if err != nil {
					break
				}
				for _, j := range jobs {
					done := b.runBrowserJob(run, c, host, j, h, p)
					done.HostID, done.Generation = host.ID, host.Generation
					end, finish := context.WithTimeout(context.Background(), 15*time.Second)
					_, _ = c.CompleteActionJob(end, j.Job.ID, done)
					finish()
				}
			}
		}
		// A transient poll failure retakes without revoking valid paused grants.
		select {
		case <-run.Done():
			return 0
		case <-time.After(actionHostRetake):
		}
	}
	return 0
}

type browserRunBook struct {
	mu   sync.Mutex
	runs map[string]*browserAgentRun
}
type browserAgentRun struct {
	mu                                             sync.Mutex
	id, hub, profile, binding, agent, grant, scope string
	expires                                        time.Time
	update                                         browserNativeUpdate
	shot                                           string
	cancel                                         context.CancelFunc
	timer                                          *time.Timer
	closed                                         bool
}
type browserNativeUpdate struct {
	State     string          `json:"state"`
	Node      string          `json:"node,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorNode string          `json:"error_node,omitempty"`
	Question  string          `json:"question,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Steps     []struct {
		Shot string `json:"shot"`
	} `json:"steps,omitempty"`
	Next int `json:"next"`
}

func browserNativeScope(p browserHostPolicy, g api.ActionGrant) string {
	sum := sha256.Sum256([]byte(p.Scope.Workspace + "\x00" + p.Scope.Owner + "\x00" + p.Policy.Profile + "\x00" + g.ID))
	return "agent-" + hex.EncodeToString(sum[:])
}
func (b *Backend) cancelBrowserRun(r *browserAgentRun) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.cancel != nil {
		r.cancel()
	}
	if r.timer != nil {
		r.timer.Stop()
	}
	r.update = browserNativeUpdate{State: "cancelled"}
	r.shot = ""
	r.mu.Unlock()
	b.closeBrowserNative(r)
}
func (b *Backend) closeBrowserNative(r *browserAgentRun) {
	r.mu.Lock()
	id, scope := r.id, r.scope
	r.mu.Unlock()
	end, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	comp := b.browserComponent()
	var discard any
	if id != "" {
		_ = comp.Call(end, scope, "POST", "/runs/"+url.PathEscape(id)+"/cancel", map[string]any{}, &discard)
	}
	_ = comp.Call(end, scope, "DELETE", "/sessions", nil, &discard)
}
func (b *Backend) browserOwnedRun(id string, g api.ActionGrant, p browserHostPolicy) (*browserAgentRun, bool) {
	book := b.browserRuns()
	book.mu.Lock()
	r := book.runs[id]
	book.mu.Unlock()
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r, r.scope == browserNativeScope(p, g) && r.grant == g.ID && r.agent == g.Agent && r.profile == p.Policy.Profile && r.binding == p.Policy.Binding && time.Now().Before(r.expires)
}
func (b *Backend) runBrowserJob(ctx context.Context, c *client.Client, host *api.ActionHost, hj api.ActionHostJob, h Hub, p browserHostPolicy) api.ActionJobComplete {
	permission := api.BrowserActionPermission(hj.Job.Action)
	g := hj.Grant
	if permission == "" || hj.Job.Version != 1 {
		return actionFailure("unsupported", "this browser host does not offer that action version", false, nil)
	}
	r := g.Target.Resource
	if !b.browserPolicyCurrent(h, p) || g.State != api.GrantActive || g.Agent == "" || g.Target.Kind != "machine" || g.Target.ID != host.MachineID || host.Resource == nil || r == nil || r.Provider != "browser" || r.ID != p.Policy.Profile || r.Binding != p.Policy.Binding || *r != *host.Resource || g.ExpiresAt == nil || !time.Now().Before(*g.ExpiresAt) || !slices.Contains(g.Permissions, permission) {
		return actionFailure("permission_denied", "the grant does not allow this operation on this browser profile", false, nil)
	}
	raw := hj.Args
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var args struct {
		ScriptID string            `json:"script_id"`
		Version  int64             `json:"version"`
		Page     string            `json:"page_id"`
		Params   map[string]string `json:"params"`
		Headless bool              `json:"headless"`
	}
	var id struct {
		RunID string `json:"run_id"`
	}
	var resume struct {
		RunID  string `json:"run_id"`
		Choice string `json:"choice"`
	}
	switch hj.Job.Action {
	case "browser.script.run":
		if browserDecode(raw, &args) != nil || args.ScriptID == "" || args.Version < 1 {
			return actionFailure("invalid_argument", "supply script_id and version of an approved snapshot; code, nodes, start_at and breakpoints are not accepted", false, nil)
		}
	case "browser.run.resume":
		if browserDecode(raw, &resume) != nil || resume.RunID == "" || resume.Choice != "" && resume.Choice != "ok" && resume.Choice != "cancel" {
			return actionFailure("invalid_argument", "supply run_id and choice ok or cancel", false, nil)
		}
		id.RunID = resume.RunID
	case "browser.profiles.list":
		if browserDecode(raw, &struct{}{}) != nil {
			return actionFailure("invalid_argument", "profiles.list accepts no arguments", false, nil)
		}
	default:
		if browserDecode(raw, &id) != nil || id.RunID == "" {
			return actionFailure("invalid_argument", "supply run_id", false, nil)
		}
	}
	run, stop, _, early := watchHostJob(ctx, c, host, hj.Job.ID)
	if early != nil {
		return *early
	}
	defer stop()
	if hj.Job.Action == "browser.profiles.list" {
		data, _ := json.Marshal(map[string]any{"profiles": []map[string]any{{"id": p.Policy.Profile, "name": p.Policy.Name, "agents": true}}})
		return api.ActionJobComplete{State: api.JobSucceeded, Result: data}
	}
	var record *browserAgentRun
	cursor := 0
	if hj.Job.Action == "browser.script.run" {
		store, e := b.browserPolicies()
		if e != nil {
			return actionFailure("permission_denied", "native policy unavailable", false, nil)
		}
		scope := p.Scope
		scope.Page = args.Page
		snap, e := store.Approved(scope, p.Policy.Profile, p.Policy.Binding, args.ScriptID, args.Version)
		if e != nil {
			return actionFailure("permission_denied", "ask the owner to approve this exact script version; include page_id when ambiguous", false, nil)
		}
		if e = browserpolicy.CheckFixedCode(snap.Script); e != nil {
			return actionFailure("invalid_graph", e.Error(), false, nil)
		}
		for _, child := range snap.Scripts {
			if e = browserpolicy.CheckFixedCode(child); e != nil {
				return actionFailure("invalid_graph", e.Error(), false, nil)
			}
		}
		if !browserParamsAllowed(snap.Script, args.Params) {
			return actionFailure("invalid_argument", "params must be declared non-secret fields in the approved snapshot", false, nil)
		}
		book := b.browserRuns()
		book.mu.Lock()
		busy := false
		now := time.Now()
		for key, v := range book.runs {
			v.mu.Lock()
			if now.After(v.expires) && (v.closed || v.update.State == "succeeded" || v.update.State == "failed") {
				delete(book.runs, key)
			} else if v.profile == p.Policy.Profile && !v.closed && (v.update.State == "running" || v.update.State == "paused") {
				busy = true
			}
			v.mu.Unlock()
		}
		if busy || len(book.runs) >= 128 {
			book.mu.Unlock()
			return actionFailure("profile_busy", "a browser run owns this profile or the local run registry is full", false, nil)
		}
		record = &browserAgentRun{cancel: stop, hub: h.ID, profile: p.Policy.Profile, binding: p.Policy.Binding, agent: g.Agent, grant: g.ID, scope: browserNativeScope(p, g), expires: *g.ExpiresAt, update: browserNativeUpdate{State: "running"}}
		reservation := "pending-" + rand.Text()
		book.runs[reservation] = record
		book.mu.Unlock()
		comp := b.browserComponent()
		e = comp.EnsureRunning(run)
		var started struct {
			ID string `json:"id"`
		}
		if e == nil {
			e = comp.Call(run, record.scope, "POST", "/runs", map[string]any{"script": snap.Script, "script_id": args.ScriptID, "script_version": args.Version, "profile": p.Policy.Profile, "params": args.Params, "headless": args.Headless}, &started)
		}
		book.mu.Lock()
		delete(book.runs, reservation)
		record.mu.Lock()
		if e == nil && validBrowserRunID(started.ID) {
			record.id = started.ID
			book.runs[started.ID] = record
		}
		record.mu.Unlock()
		book.mu.Unlock()
		record.mu.Lock()
		closed := record.closed
		record.mu.Unlock()
		if e != nil || record.id == "" || closed {
			b.cancelBrowserRun(record)
			b.closeBrowserNative(record)
			return actionFailure("outcome_unknown", "native startup was not confirmed; its session was closed, check before retrying", false, nil)
		}
	} else {
		var ok bool
		record, ok = b.browserOwnedRun(id.RunID, g, p)
		if !ok {
			return actionFailure("permission_denied", "run_id must belong to this same grant, agent and profile", false, nil)
		}
		if hj.Job.Action == "browser.run.status" {
			return b.browserRunResult(run, c, host, hj.Job.ID, record)
		}
		if hj.Job.Action == "browser.run.cancel" {
			b.cancelBrowserRun(record)
			return b.browserRunResult(run, c, host, hj.Job.ID, record)
		}
		record.mu.Lock()
		paused := !record.closed && record.update.State == "paused"
		cursor = record.update.Next
		if paused {
			if record.timer != nil {
				record.timer.Stop()
			}
			record.update.State = "running"
		}
		record.mu.Unlock()
		if !paused {
			return actionFailure("conflict", "only a paused run can resume; cancel its running action job to stop it", false, nil)
		}
		choice := resume.Choice
		if choice == "" {
			choice = "ok"
		}
		// Protocol 1 appends a control acknowledgement before gate() changes
		// state. Skip that event: its state can still be the previous pause.
		if choice == "ok" {
			cursor++
		}
		var discard any
		if e := b.browserComponent().Call(run, record.scope, "POST", "/runs/"+url.PathEscape(record.id)+"/resume", map[string]string{"choice": choice}, &discard); e != nil {
			b.cancelBrowserRun(record)
			return actionFailure("outcome_unknown", "resume was not confirmed; the native session was closed, do not repeat blindly", false, nil)
		}
	}
	return b.waitBrowserRun(run, c, host, hj.Job.ID, record, cursor)
}
func validBrowserRunID(id string) bool {
	if !strings.HasPrefix(id, "run_") || len(id) > 100 {
		return false
	}
	for _, v := range id {
		if !(v >= 'a' && v <= 'z' || v >= 'A' && v <= 'Z' || v >= '0' && v <= '9' || v == '_' || v == '-') {
			return false
		}
	}
	return true
}
func browserParamsAllowed(raw json.RawMessage, params map[string]string) bool {
	var s struct {
		Params []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"params"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	allowed := map[string]bool{}
	for _, p := range s.Params {
		allowed[p.Name] = p.Type != "secret"
	}
	if len(params) > 128 {
		return false
	}
	for k, v := range params {
		if !allowed[k] || len(v) > 16<<10 {
			return false
		}
	}
	return true
}
func (b *Backend) waitBrowserRun(ctx context.Context, c *client.Client, host *api.ActionHost, job string, r *browserAgentRun, after int) api.ActionJobComplete {
	native := b.browserComponent()
	run, stop := context.WithCancel(ctx)
	defer stop()
	r.mu.Lock()
	r.cancel = stop
	closed := r.closed
	r.mu.Unlock()
	if closed {
		stop()
	}
	for run.Err() == nil {
		var update browserNativeUpdate
		e := native.Call(run, r.scope, "GET", "/runs/"+url.PathEscape(r.id)+"?after="+strconv.Itoa(after), nil, &update)
		if e != nil {
			break
		}
		if update.State != "queued" && update.State != "running" && update.State != "paused" && update.State != "succeeded" && update.State != "failed" && update.State != "cancelled" {
			break
		}
		// A long-poll heartbeat has no new event. In particular it cannot
		// confirm a fresh pause after a resume acknowledgement.
		if update.State == "paused" && update.Next <= after {
			continue
		}
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			break
		}
		for _, step := range update.Steps {
			if step.Shot != "" {
				r.shot = step.Shot
			}
		}
		update.Steps = nil
		r.update = update
		after = update.Next
		if update.State == "paused" {
			until := time.Now().Add(15 * time.Minute)
			if r.expires.Before(until) {
				until = r.expires
			}
			r.timer = time.AfterFunc(time.Until(until), func() { b.cancelBrowserRun(r) })
			r.cancel = nil
			r.mu.Unlock()
			return b.browserRunResult(run, c, host, job, r)
		}
		terminal := update.State != "running" && update.State != "queued"
		r.mu.Unlock()
		if terminal {
			r.mu.Lock()
			r.closed = true
			r.cancel = nil
			r.mu.Unlock()
			result := b.browserRunResult(run, c, host, job, r)
			end, done := context.WithTimeout(context.Background(), 5*time.Second)
			var discard any
			_ = native.Call(end, r.scope, "DELETE", "/sessions", nil, &discard)
			done()
			return result
		}
	}
	b.cancelBrowserRun(r)
	return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "outcome_unknown", Message: "browser stopped or disconnected; earlier website actions may already have applied"}}
}
func (b *Backend) browserRunResult(ctx context.Context, c *client.Client, host *api.ActionHost, job string, r *browserAgentRun) api.ActionJobComplete {
	r.mu.Lock()
	update, shot, id, scope := r.update, r.shot, r.id, r.scope
	r.mu.Unlock()
	value := map[string]any{"run_id": id, "state": update.State, "node": update.Node, "output": update.Output}
	if update.Error != "" {
		value["error"] = update.Error
		value["error_node"] = update.ErrorNode
	}
	if update.Question != "" {
		value["question"] = update.Question
	}
	result := api.ActionJobComplete{State: api.JobSucceeded}
	if shot != "" {
		data, kind, e := b.browserComponent().Artifact(ctx, scope, shot)
		if e == nil && len(data) <= artifactMaxSize {
			art, e := c.UploadActionArtifact(ctx, job, host.ID, host.Generation, kind, data)
			if e == nil {
				value["screenshot"] = art
				result.ArtifactIDs = []string{art.ID}
			}
		}
	}
	result.Result, _ = json.Marshal(value)
	if update.State == "failed" {
		result.State = api.JobFailed
		result.Error = &api.ActionError{Code: "failed", Message: "browser script failed; earlier website actions may already have applied"}
	}
	if update.State == "cancelled" {
		result.State = api.JobCancelled
		result.Error = &api.ActionError{Code: "cancelled", Message: "browser stopped; earlier website actions may already have applied"}
	}
	return result
}
