package console

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/remotedesktop"
	"github.com/myco-ai/myco/internal/sshcap"
)

// The App hosts agent actions on this computer (api/actions.go): for each
// workspace where this computer is a machine, once the optional Remote
// Desktop component offers agent actions, it registers as the computer's
// remote.desktop host, takes the jobs agents run on grants its owner
// approved, checks each grant again, runs the job through the component and
// reports. It stops a job as soon as the Hub says to (cancelled, revoked,
// expired).

// desktopActions are the component's agent calls (a fake in tests).
type desktopActions interface {
	Observe(ctx context.Context, maxWidth int, format string) (*remotedesktop.Observation, error)
	Input(ctx context.Context, frameID string, events json.RawMessage) (int, error)
}

type actionHosts struct {
	once    sync.Once
	mu      sync.Mutex
	running map[string]context.CancelFunc // hub id -> its host loop
	epochs  map[string]uint64
	retry   map[string]time.Time // hub id -> not before (no machine here, old hub)
	// The component was started for agents (not by Remote Desktop's
	// sharing), with control or not; it stops when idle.
	agentOwned, control bool
	lastJob             time.Time
	frames              frameBook
	lease               controlLease
}

// controlLease is the one grant that may control this computer at a time
// (whichever workspace it is in): it holds until it ends or expires, and
// another grant's input meanwhile is refused as computer_busy.
type controlLease struct {
	mu    sync.Mutex
	grant string
	until time.Time
	// active asks the holder's Hub whether its grant still holds.
	active func(context.Context) bool
}

func (l *controlLease) take(ctx context.Context, grant string, until time.Time, active func(context.Context) bool) bool {
	l.mu.Lock()
	holder, holderUntil, holderActive := l.grant, l.until, l.active
	l.mu.Unlock()
	if holder != "" && holder != grant && time.Now().Before(holderUntil) && (holderActive == nil || holderActive(ctx)) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.grant != holder { // taken meanwhile
		return l.grant == grant
	}
	if active != nil || l.grant != grant {
		l.active = active
	}
	l.grant, l.until = grant, until
	return true
}

// frameBook remembers which grant observed each frame: input may only use a
// frame its own grant observed, within frameMaxAge (the component checks age
// and display geometry again).
type frameBook struct {
	mu sync.Mutex
	m  map[string]frameSeen
}
type frameSeen struct {
	grant string
	at    time.Time
}

const frameMaxAge = 30 * time.Second

func (f *frameBook) add(id, grant string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]frameSeen{}
	}
	for k, v := range f.m {
		if time.Since(v.at) > frameMaxAge {
			delete(f.m, k)
		}
	}
	f.m[id] = frameSeen{grant: grant, at: time.Now()}
}

func (f *frameBook) ok(id, grant string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, found := f.m[id]
	return found && v.grant == grant && time.Since(v.at) <= frameMaxAge
}

const (
	actionHostPoll = 25 * time.Second // under the Hub's 45 s host staleness
	// actionHostRetake: how long a host the Hub no longer knows waits before
	// it registers again.
	actionHostRetake = 30 * time.Second
	agentIdleStop    = 2 * time.Minute
	artifactMaxSize  = 4 << 20 // the Hub's artifact limit
)

var desktopActionList = []api.ActionDescriptor{
	{Name: "remote.desktop.observe", Version: 1, Title: "See the screen", Permission: "remote.desktop.view"},
	{Name: "remote.desktop.input", Version: 1, Title: "Click and type", Permission: "remote.desktop.control"},
}

// startActionHosts starts the loop that keeps a host per workspace (the
// desktop App only).
func (b *Backend) startActionHosts() {
	if b.Local == nil || b.Store == nil {
		return
	}
	b.actionHosts.once.Do(func() {
		go func() {
			time.Sleep(15 * time.Second) // after the App has started up
			for {
				b.reconcileActionHosts()
				time.Sleep(30 * time.Second)
			}
		}()
	})
}

func (b *Backend) reconcileActionHosts() {
	a := &b.actionHosts
	st := b.remoteComponent().Status()
	// Keys: a hub's id (the computer's remote.desktop host) and "page:" and
	// its id (its page renderer, while the window runs the renderer).
	want := map[string]Hub{}
	sshWant := map[string]sshcap.Profile{}
	_, profiles := b.sshProfiles()
	hubs, _ := b.Store.List()
	desktop := st.Supported && st.Installed && remotedesktop.AgentActions(st.Version)
	pages := b.pageRender.active()
	for _, h := range hubs {
		for _, p := range profiles {
			if p.AgentEnabled && p.AgentHub == h.ID {
				key := "ssh:" + h.ID + ":" + p.ID + ":" + p.Binding
				want[key] = h
				sshWant[key] = p
			}
		}
		if desktop {
			want[h.ID] = h
		}
		if pages {
			want["page:"+h.ID] = h
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running == nil {
		a.running, a.retry = map[string]context.CancelFunc{}, map[string]time.Time{}
	}
	if a.epochs == nil {
		a.epochs = map[string]uint64{}
	}
	for id, stop := range a.running {
		if _, ok := want[id]; !ok {
			stop()
			a.epochs[id]++ // invalidate that loop's late completion
			delete(a.retry, id)
			delete(a.running, id)
		}
	}
	for key, h := range want {
		if a.running[key] != nil || time.Now().Before(a.retry[key]) {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		a.running[key] = cancel
		a.epochs[key]++
		epoch := a.epochs[key]
		go func() {
			var wait time.Duration
			if p, ok := sshWant[key]; ok {
				wait = b.hostSSH(ctx, h, p)
			} else if strings.HasPrefix(key, "page:") {
				wait = b.hostPages(ctx, h)
			} else {
				wait = b.hostActions(ctx, h)
			}
			a.finish(key, epoch, wait)
		}()
	}
	if a.agentOwned && time.Since(a.lastJob) > agentIdleStop {
		b.remoteComponent().Stop()
		a.agentOwned = false
	}
}

// finish never removes a newer loop when a profile is quickly disabled and
// re-enabled while the cancelled loop is still unwinding its HTTP poll.
func (a *actionHosts) finish(key string, epoch uint64, wait time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.epochs[key] != epoch {
		return
	}
	delete(a.running, key)
	a.retry[key] = time.Now().Add(wait)
}

// thisMachine is this computer's machine at hub: the one its machine node
// there runs on (a machine's own agents list leaves machine nodes out).
func (b *Backend) thisMachine(ctx context.Context, c *client.Client) (string, error) {
	_, nodes, _ := b.Local.localMachine()
	if len(nodes) == 0 {
		return "", errors.New("this computer is not a machine of any workspace")
	}
	agents, err := c.ListAgents(ctx, "", false)
	if err != nil {
		return "", err
	}
	if id := machineOfNodes(agents, nodes); id != "" {
		return id, nil
	}
	return "", errors.New("this computer is not a machine of this workspace")
}

// machineOfNodes is the machine one of nodes (this computer's machine
// nodes) runs on.
func machineOfNodes(agents []api.Agent, nodes []string) string {
	for _, a := range agents {
		if slices.Contains(nodes, a.Name) && strings.HasPrefix(a.MachineID, "mch_") {
			return a.MachineID
		}
	}
	return ""
}

// hostActions is the host for hub h until ctx ends or it cannot go on; it
// returns how long to wait before trying again.
func (b *Backend) hostActions(ctx context.Context, h Hub) time.Duration {
	c, err := b.client(h)
	if err != nil {
		return 10 * time.Minute
	}
	call, cancel := context.WithTimeout(ctx, 15*time.Second)
	machine, err := b.thisMachine(call, c)
	cancel()
	if err != nil {
		return 2 * time.Minute
	}
	gen := "app-" + rand.Text()
	register := func() (*api.ActionHost, error) {
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		return c.RegisterActionHost(call, api.ActionHostRegister{MachineID: machine, Provider: "remote.desktop", Generation: gen, Actions: desktopActionList})
	}
	host, err := register()
	if err != nil {
		var ce *client.Error
		if errors.As(err, &ce) && (ce.Code == "unsupported" || ce.Status == 404 || ce.Code == "permission_denied") {
			return 5 * time.Minute // an older hub, an end-to-end workspace, or not this person's computer
		}
		return time.Minute
	}
	for ctx.Err() == nil {
		jobs, err := c.ActionHostJobs(ctx, host.ID, actionHostPoll)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			var ce *client.Error
			if errors.As(err, &ce) && (ce.Code == "not_found" || ce.Code == "conflict") {
				// Gone quiet (asleep) or replaced, maybe by another App of
				// this person's on this computer: offer again after a pause,
				// never at once (two Apps would take it from each other in a
				// tight loop).
				return actionHostRetake
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			continue
		}
		// One screen: jobs run one after another.
		for _, j := range jobs {
			b.runDesktopJob(ctx, c, host, j, b.desktopFor)
		}
	}
	return 0
}

// desktopFor makes sure the component runs (with control when needed) and
// returns it.
func (b *Backend) desktopFor(ctx context.Context, control bool) (desktopActions, error) {
	a := &b.actionHosts
	comp := b.remoteComponent()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastJob = time.Now()
	if st := comp.Status(); st.Running {
		if !a.agentOwned || a.control || !control {
			// Remote Desktop's own sharing is used as it is (view only
			// there means view only here).
			return comp, nil
		}
		comp.Stop()
		a.agentOwned = false
	}
	if _, err := comp.Start(ctx, control, nil); err != nil {
		return nil, err
	}
	a.agentOwned, a.control = true, control
	if actions, _, err := comp.AgentStatus(ctx); err != nil || !slices.Contains(actions, "remote.desktop.observe") {
		comp.Stop()
		a.agentOwned = false
		return nil, errors.New("component_outdated")
	}
	return comp, nil
}

// releaseAgentComponent stops a component started for agents, so Remote
// Desktop's sharing can start its own.
func (b *Backend) releaseAgentComponent() {
	a := &b.actionHosts
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.agentOwned {
		b.remoteComponent().Stop()
		a.agentOwned = false
	}
}

func (b *Backend) agentOwnsComponent() bool {
	b.actionHosts.mu.Lock()
	defer b.actionHosts.mu.Unlock()
	return b.actionHosts.agentOwned
}

// actionPermitted reports whether grant g (as the host got it) allows
// action on machine now.
func actionPermitted(g api.ActionGrant, machine, permission string, now time.Time) bool {
	if g.State != api.GrantActive || g.Target.Kind != "machine" || g.Target.ID != machine || g.Target.Resource != nil || (g.ExpiresAt != nil && !now.Before(*g.ExpiresAt)) {
		return false
	}
	return slices.Contains(g.Permissions, permission) || (permission == "remote.desktop.view" && slices.Contains(g.Permissions, "remote.desktop.control"))
}

func actionFailure(code, message string, retryable bool, applied *int) api.ActionJobComplete {
	return api.ActionJobComplete{State: api.JobFailed, Error: &api.ActionError{Code: code, Message: message, Retryable: retryable, Applied: applied}}
}

// runDesktopJob runs one job: grant checked again here, stopped as soon as
// the Hub says to, reported either way.
func (b *Backend) runDesktopJob(ctx context.Context, c *client.Client, host *api.ActionHost, hj api.ActionHostJob, desktop func(context.Context, bool) (desktopActions, error)) {
	done := runDesktopJob(ctx, c, host, hj, desktop, &b.actionHosts.frames, &b.actionHosts.lease)
	done.HostID, done.Generation = host.ID, host.Generation
	end, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	_, _ = c.CompleteActionJob(end, hj.Job.ID, done)
}

func runDesktopJob(ctx context.Context, c *client.Client, host *api.ActionHost, hj api.ActionHostJob, desktop func(context.Context, bool) (desktopActions, error), frames *frameBook, lease *controlLease) api.ActionJobComplete {
	j := hj.Job
	var permission string
	for _, d := range desktopActionList {
		if d.Name == j.Action && d.Version == j.Version {
			permission = d.Permission
		}
	}
	if permission == "" {
		return actionFailure("unsupported", "this computer does not offer "+j.Action+" v"+strconv.Itoa(j.Version), false, nil)
	}
	if !actionPermitted(hj.Grant, host.MachineID, permission, time.Now()) {
		return actionFailure("permission_denied", "the grant does not allow this on this computer now", false, nil)
	}
	run, done, wasStopped, early := watchHostJob(ctx, c, host, j.ID)
	if early != nil {
		return *early
	}
	defer done()

	// With control granted, the component starts able to control even for
	// an observe: starting it again for the input that follows would lose
	// the frame just observed.
	d, err := desktop(run, actionPermitted(hj.Grant, host.MachineID, "remote.desktop.control", time.Now()))
	if err != nil {
		return actionFailure("unavailable", "the Remote Desktop component could not start on this computer: "+err.Error(), true, intPtr(0))
	}
	switch j.Action {
	case "remote.desktop.observe":
		var args struct {
			DisplayID string `json:"display_id"`
			MaxWidth  int    `json:"max_width"`
			Format    string `json:"format"`
		}
		if len(hj.Args) > 0 && json.Unmarshal(hj.Args, &args) != nil {
			return actionFailure("invalid_argument", "args are not an observe request", false, nil)
		}
		if args.DisplayID != "" && args.DisplayID != "main" {
			return actionFailure("invalid_argument", "only the main display (display_id \"main\") can be observed", false, nil)
		}
		obs, err := d.Observe(run, args.MaxWidth, args.Format)
		if err != nil {
			if wasStopped() {
				return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "cancelled", Message: "stopped"}}
			}
			var ce *remotedesktop.ComponentError
			if errors.As(err, &ce) && ce.Code == "invalid_argument" {
				return actionFailure("invalid_argument", "max_width is 320 to 2560 and format jpeg or png", false, nil)
			}
			return actionFailure("capture_failed", "could not capture the screen (on a Mac, allow Screen Recording for myco)", true, nil)
		}
		if len(obs.Image) > artifactMaxSize {
			return actionFailure("too_large", "the frame is over 4 MB: ask for jpeg or a smaller max_width", false, nil)
		}
		frames.add(obs.FrameID, hj.Grant.ID)
		up, cancel := context.WithTimeout(run, 30*time.Second)
		art, err := c.UploadActionArtifact(up, j.ID, host.ID, host.Generation, obs.MediaType, obs.Image)
		cancel()
		if err != nil {
			if wasStopped() {
				return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "cancelled", Message: "stopped"}}
			}
			return actionFailure("unavailable", "could not upload the frame", true, nil)
		}
		result, _ := json.Marshal(map[string]any{"frame_id": obs.FrameID, "captured_at": obs.CapturedAt, "display_id": obs.DisplayID,
			"width": obs.Width, "height": obs.Height, "artifact": art})
		return api.ActionJobComplete{State: api.JobSucceeded, Result: result, ArtifactIDs: []string{art.ID}}
	case "remote.desktop.input":
		var args struct {
			FrameID string          `json:"frame_id"`
			Events  json.RawMessage `json:"events"`
		}
		if json.Unmarshal(hj.Args, &args) != nil || args.FrameID == "" || len(args.Events) == 0 {
			return actionFailure("invalid_argument", "input needs frame_id and events", false, intPtr(0))
		}
		if !frames.ok(args.FrameID, hj.Grant.ID) {
			return actionFailure("stale_frame", "that frame is not this grant's or is over 30 seconds old: observe again", false, intPtr(0))
		}
		until := time.Now().Add(30 * time.Minute)
		if hj.Grant.ExpiresAt != nil {
			until = *hj.Grant.ExpiresAt
		}
		jobID := j.ID
		active := func(ctx context.Context) bool {
			check, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			s, err := c.ActionHostJob(check, jobID, host.ID, host.Generation, 0)
			return err == nil && s.GrantState == api.GrantActive
		}
		if !lease.take(run, hj.Grant.ID, until, active) {
			return actionFailure("computer_busy", "another agent controls this computer now: try again when its grant ends", true, intPtr(0))
		}
		applied, err := d.Input(run, args.FrameID, args.Events)
		if err == nil {
			result, _ := json.Marshal(map[string]int{"applied": applied})
			return api.ActionJobComplete{State: api.JobSucceeded, Result: result}
		}
		if wasStopped() {
			return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "outcome_unknown",
				Message: "stopped while it ran: some of it may have happened; observe before going on"}}
		}
		var ce *remotedesktop.ComponentError
		if !errors.As(err, &ce) {
			return actionFailure("outcome_unknown", "the component stopped answering: some of it may have happened; observe before going on", false, nil)
		}
		switch ce.Code {
		case "invalid_argument":
			return actionFailure("invalid_argument", "an event is not valid (all or nothing: none was applied)", false, intPtr(0))
		case "stale_frame":
			return actionFailure("stale_frame", "that frame is over 30 seconds old or the display changed since: observe again", false, intPtr(0))
		case "computer_busy":
			return actionFailure("computer_busy", "a person is controlling this computer remotely: try again later", true, intPtr(0))
		case "control_not_allowed":
			return actionFailure("control_not_allowed", "this computer allows viewing only (on a Mac, allow Accessibility for myco; or Remote Desktop is sharing view only)", false, intPtr(0))
		case "input_failed":
			return actionFailure("input_failed", "the computer refused an event; observe before going on", false, ce.Applied)
		}
		return actionFailure("outcome_unknown", "input failed ("+ce.Code+"): some of it may have happened; observe before going on", false, ce.Applied)
	}
	return actionFailure("unsupported", "unknown action", false, nil)
}

// watchHostJob asks the Hub once more right before anything happens here,
// then keeps asking while the job runs: run ends when the Hub says to stop
// (cancelled, revoked, expired), cannot confirm the job may go on, or the
// grant runs out. early is the result when it should not start at all.
func watchHostJob(ctx context.Context, c *client.Client, host *api.ActionHost, jobID string) (run context.Context, done func(), wasStopped func() bool, early *api.ActionJobComplete) {
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	st, err := c.ActionHostJob(check, jobID, host.ID, host.Generation, 0)
	cancel()
	if err != nil {
		e := actionFailure("unavailable", "could not check the grant with the workspace", true, nil)
		return nil, nil, nil, &e
	}
	if st.Stop {
		return nil, nil, nil, &api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "cancelled", Message: "stopped before it started", Applied: intPtr(0)}}
	}
	run, stop := context.WithCancel(ctx)
	var timer *time.Timer
	if exp := st.GrantExpiresAt; exp != nil {
		timer = time.AfterFunc(time.Until(*exp), stop)
	}
	var stopped bool
	var stopMu sync.Mutex
	go func() {
		for run.Err() == nil {
			s, err := c.ActionHostJob(run, jobID, host.ID, host.Generation, 30*time.Second)
			if run.Err() != nil {
				return
			}
			// Fail closed: a host replaced or gone, or a Hub that cannot say
			// whether the grant still holds, stops the job.
			if err != nil || s.Stop {
				stopMu.Lock()
				stopped = true
				stopMu.Unlock()
				stop()
				return
			}
		}
	}()
	done = func() {
		stop()
		if timer != nil {
			timer.Stop()
		}
	}
	wasStopped = func() bool {
		stopMu.Lock()
		defer stopMu.Unlock()
		return stopped || run.Err() != nil && ctx.Err() == nil
	}
	return run, done, wasStopped, nil
}

func intPtr(n int) *int { return &n }
