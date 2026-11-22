package console

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/hubserver"
	"github.com/chenhg5/myco/internal/remotedesktop"
)

type fakeDesktop struct {
	inputStarted chan struct{}
	inputEnded   chan error
}

func (f *fakeDesktop) Observe(ctx context.Context, maxWidth int, format string) (*remotedesktop.Observation, error) {
	return &remotedesktop.Observation{FrameID: "frm_1", CapturedAt: time.Now(), DisplayID: "main", Width: 1280, Height: 800, MediaType: "image/jpeg", Image: []byte("jpeg")}, nil
}

// Input blocks until it is stopped (a long batch while its grant is revoked).
func (f *fakeDesktop) Input(ctx context.Context, frameID string, events json.RawMessage) (int, error) {
	close(f.inputStarted)
	<-ctx.Done()
	f.inputEnded <- ctx.Err()
	return 0, ctx.Err()
}

// The App's host: an observe goes through the component to a screenshot the
// agent reads; an input still running when the owner revokes is stopped at
// once and reported as maybe-happened.
func TestDesktopJobsObserveAndStopOnRevoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: "admin-token-123456",
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h.Handler)
	defer func() { hs.Close(); h.Close() }()
	admin, _ := client.New(hs.URL, "admin-token-123456")
	ct, err := admin.CreateConsoleToken(ctx, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(hs.URL, ct.Token)
	reg := func(name, harness string) *client.Client {
		r, err := admin.Register(ctx, api.RegisterRequest{Name: name, Binding: api.Binding{Harness: harness, SessionID: name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := client.New(hs.URL, r.Token)
		return c
	}
	node := reg("mac-machine", "machine")
	if err := node.ReportMachine(ctx, api.MachineReport{Fingerprint: "fp-host-0123456789abcdef", Name: "studio", Hostname: "studio", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	agent := reg("cloud-claude", "claude")
	ms, _ := owner.Machines(ctx)
	machine := ms[0].ID
	g, err := agent.RequestActionGrant(ctx, api.ActionGrantRequest{Target: api.ActionTarget{Kind: "machine", ID: machine}, Permissions: []string{"remote.desktop.control"}, Reason: "fill the form"}, false)
	if err != nil {
		t.Fatal(err)
	}
	aps, _ := owner.Approvals(ctx, "pending")
	if _, err := owner.DecideApproval(ctx, aps[0].ID, true); err != nil {
		t.Fatal(err)
	}
	host, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: machine, Provider: "remote.desktop", Generation: "app-1", Actions: desktopActionList})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeDesktop{inputStarted: make(chan struct{}), inputEnded: make(chan error, 1)}
	var askedControl []bool
	desktop := func(_ context.Context, control bool) (desktopActions, error) {
		askedControl = append(askedControl, control)
		return fake, nil
	}
	b := &Backend{}

	obs, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "o1"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := owner.ActionHostJobs(ctx, host.ID, 2*time.Second)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	b.runDesktopJob(ctx, owner, host, jobs[0], desktop)
	done, err := agent.ActionJob(ctx, obs.ID, time.Second)
	if err != nil || done.State != api.JobSucceeded || len(done.Artifacts) != 1 {
		t.Fatalf("observe: %+v %v", done, err)
	}
	var res struct {
		FrameID string `json:"frame_id"`
		Width   int    `json:"width"`
	}
	if json.Unmarshal(done.Result, &res) != nil || res.FrameID != "frm_1" || res.Width != 1280 {
		t.Fatalf("result: %s", done.Result)
	}
	if data, _, err := agent.ActionArtifact(ctx, done.Artifacts[0].ID); err != nil || string(data) != "jpeg" {
		t.Fatalf("artifact: %q %v", data, err)
	}

	// A frame no observe of this grant made is refused before anything runs.
	bad, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.input", Version: 1, IdempotencyKey: "i0",
		Args: json.RawMessage(`{"frame_id":"frm_other","events":[{"kind":"text","text":"x"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ = owner.ActionHostJobs(ctx, host.ID, 2*time.Second)
	b.runDesktopJob(ctx, owner, host, jobs[0], desktop)
	if j, _ := agent.ActionJob(ctx, bad.ID, time.Second); j.State != api.JobFailed || j.Error == nil || j.Error.Code != "stale_frame" {
		t.Fatalf("foreign frame: %+v", j)
	}
	in, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.input", Version: 1, IdempotencyKey: "i1",
		Args: json.RawMessage(`{"frame_id":"frm_1","events":[{"kind":"text","text":"hello"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err = owner.ActionHostJobs(ctx, host.ID, 2*time.Second)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	ran := make(chan struct{})
	go func() { b.runDesktopJob(ctx, owner, host, jobs[0], desktop); close(ran) }()
	<-fake.inputStarted
	revoked := time.Now()
	if _, err := owner.RevokeActionGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.inputEnded:
		if d := time.Since(revoked); d > 3*time.Second {
			t.Fatalf("input stopped %v after the revoke", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("input kept running after its grant was revoked")
	}
	<-ran
	if j, err := agent.ActionJob(ctx, in.ID, time.Second); err != nil || j.State != api.JobCancelled {
		t.Fatalf("input after revoke: %+v %v", j, err)
	}
	// A control grant starts the component able to control from the first
	// observe (no restart that would lose the frame).
	if len(askedControl) != 3 || !askedControl[0] || !askedControl[1] {
		t.Fatalf("component asked for control: %v", askedControl)
	}
}

func TestActionPermitted(t *testing.T) {
	soon := time.Now().Add(time.Minute)
	g := api.ActionGrant{State: api.GrantActive, Target: api.ActionTarget{Kind: "machine", ID: "mch_1"}, Permissions: []string{"remote.desktop.control"}, ExpiresAt: &soon}
	if !actionPermitted(g, "mch_1", "remote.desktop.view", time.Now()) || !actionPermitted(g, "mch_1", "remote.desktop.control", time.Now()) {
		t.Fatal("control should allow view and control")
	}
	if actionPermitted(g, "mch_2", "remote.desktop.view", time.Now()) || actionPermitted(g, "mch_1", "remote.desktop.view", soon.Add(time.Second)) {
		t.Fatal("another computer or an expired grant was allowed")
	}
	g.Permissions = []string{"remote.desktop.view"}
	if actionPermitted(g, "mch_1", "remote.desktop.control", time.Now()) {
		t.Fatal("view allowed control")
	}
}

func TestControlLeaseOneGrantAtATime(t *testing.T) {
	var l controlLease
	ctx := context.Background()
	soon := time.Now().Add(time.Minute)
	stillActive := true
	if !l.take(ctx, "g1", soon, func(context.Context) bool { return stillActive }) {
		t.Fatal("first grant refused")
	}
	if !l.take(ctx, "g1", soon, nil) {
		t.Fatal("the holder refused its own input")
	}
	if l.take(ctx, "g2", soon, nil) {
		t.Fatal("a second grant took control while the first holds")
	}
	stillActive = false // revoked or ended
	if !l.take(ctx, "g2", soon, nil) {
		t.Fatal("control was not released when the holder's grant ended")
	}
	l.until = time.Now().Add(-time.Second) // expired
	if !l.take(ctx, "g3", soon, nil) {
		t.Fatal("control was not released when the holder's grant expired")
	}
}
