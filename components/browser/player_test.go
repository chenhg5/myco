package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"strings"
	"testing"
	"time"
)

func fixtureRun(t *testing.T) *playRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return &playRun{id: "run_test", ctx: ctx, cancel: cancel, wake: make(chan struct{}), breakpoints: map[string]bool{}, values: map[string]string{}, update: runUpdate{State: "queued", Output: map[string]any{}}}
}
func awaitRun(t *testing.T, r *playRun, after *int, predicate func(runUpdate) bool) runUpdate {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		out, err := r.events(ctx, *after)
		if err != nil {
			t.Fatal(err)
		}
		*after = out.Next
		if predicate(out) {
			return out
		}
	}
}
func TestSingleStepBreakpointAndConfirmation(t *testing.T) {
	r := fixtureRun(t)
	r.breakpoints["a"] = true
	r.breakpoints["c"] = true
	actions := make(chan string, 10)
	r.action = func(ctx context.Context, n node) (any, error) { actions <- n.ID; return nil, nil }
	go r.play([]node{{ID: "a", Type: "click"}, {ID: "b", Type: "click"}, {ID: "c", Type: "confirm", Message: "Stop before submit"}, {ID: "d", Type: "click"}}, "stop")
	cursor := 0
	awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "paused" && out.Node == "a" })
	if err := r.control("step", ""); err != nil {
		t.Fatal(err)
	}
	out := awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "paused" && out.Node == "b" })
	if len(actions) != 1 || out.Question != "" {
		t.Fatal("single step executed more than one action")
	}
	if err := r.control("resume", ""); err != nil {
		t.Fatal(err)
	}
	out = awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "paused" && out.Node == "c" })
	if out.Question != "Stop before submit" || len(actions) != 2 {
		t.Fatal("breakpoint or confirm")
	}
	if err := r.control("resume", ""); err == nil {
		t.Fatal("confirm resumed without explicit choice")
	}
	if err := r.control("resume", "cancel"); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "cancelled" })
	if len(actions) != 2 {
		t.Fatal("submitted despite cancelling confirmation")
	}
}
func TestOutputRedactionIncludesNestedAndEscapedSecrets(t *testing.T) {
	r := fixtureRun(t)
	secret := "private\"\\\nvalue"
	r.secrets = []string{secret}
	r.values["password"] = secret
	r.action = func(ctx context.Context, n node) (any, error) {
		return map[string]any{"nested": []any{"Prefix " + secret + " suffix", map[string]any{"quoted": secret}}}, nil
	}
	go r.play([]node{{ID: "a", Type: "js", Into: "result"}}, "stop")
	cursor := 0
	out := awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "succeeded" })
	raw, _ := json.Marshal(out.Output)
	if strings.Contains(string(raw), "private") || !strings.Contains(string(raw), "redacted") {
		t.Fatal("secret was not redacted", string(raw))
	}
}
func TestCancelInterruptsAnActionAndErrorDetailsStayPrivate(t *testing.T) {
	r := fixtureRun(t)
	started := make(chan struct{})
	r.action = func(ctx context.Context, n node) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, errors.New("provider says private password")
	}
	go r.play([]node{{ID: "a", Type: "click"}, {ID: "b", Type: "click"}}, "stop")
	<-started
	r.control("cancel", "")
	cursor := 0
	out := awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "cancelled" })
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "password") {
		t.Fatal("raw engine error leaked")
	}
}
func TestRecordingRejectsExecutableOrSensitiveBindingPayloads(t *testing.T) {
	valid := node{Type: "type", Target: &target{Locators: []locator{{Kind: "css", Value: "#password"}}}, Sensitive: true, Value: "${password}"}
	if !recordable(valid) {
		t.Fatal("password reference rejected")
	}
	valid.Value = "private"
	if recordable(valid) {
		t.Fatal("literal password accepted")
	}
	valid.Type = "js"
	valid.Sensitive = false
	if recordable(valid) {
		t.Fatal("website forged executable script")
	}
	valid.Type = "upload"
	valid.Value = "/etc/passwd"
	if recordable(valid) {
		t.Fatal("website forged local path")
	}
}

func TestExpectedDialogIsArmedBeforeClickAndHonorsSingleStep(t *testing.T) {
	r := fixtureRun(t)
	s := &session{ctx: r.ctx, pages: map[proto.TargetTargetID]*rod.Page{"root": {TargetID: "root", FrameID: "root"}}, current: "root", controller: r.id}
	r.session = s
	r.breakpoints["dialog"] = true
	handled := make(chan struct{})
	actions := make(chan string, 4)
	r.applyDialog = func(ctx context.Context, n node, evt observation) error {
		if n.Dialog != "accept" || evt.Session != "primary" {
			return errors.New("wrong dialog")
		}
		close(handled)
		return nil
	}
	r.action = func(ctx context.Context, n node) (any, error) {
		if n.Type == "dialog" {
			return nil, r.dialog(ctx, n)
		}
		actions <- n.ID
		if n.ID == "click" {
			s.observe(observation{Kind: "dialog", Target: "root", Session: "primary"})
			select {
			case <-handled:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return nil, nil
	}
	go r.play([]node{{ID: "click", Type: "click", TimeoutMS: 200}, {ID: "dialog", Type: "dialog", Dialog: "accept"}, {ID: "after", Type: "click"}}, "stop")
	cursor := 0
	awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "paused" && out.Node == "dialog" })
	// A debugger pause must outlive the preceding click's normal timeout.
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	if err := r.control("step", ""); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "paused" && out.Node == "after" })
	if len(actions) != 1 {
		t.Fatal("dialog single-step ran the following action")
	}
	r.control("resume", "")
	awaitRun(t, r, &cursor, func(out runUpdate) bool { return out.State == "succeeded" })
}

func TestStepClickIntoDialogThenResumeReachesConfirmation(t *testing.T) {
	r := fixtureRun(t)
	s := &session{ctx: r.ctx, pages: map[proto.TargetTargetID]*rod.Page{"root": {TargetID: "root", FrameID: "root"}}, current: "root", controller: r.id}
	r.session = s
	r.breakpoints["trigger"] = true
	handled := make(chan struct{})
	r.applyDialog = func(context.Context, node, observation) error { close(handled); return nil }
	r.action = func(ctx context.Context, n node) (any, error) {
		if n.Type == "dialog" {
			return nil, r.dialog(ctx, n)
		}
		if n.ID == "trigger" {
			s.observe(observation{Kind: "dialog", Target: "root", Session: "primary"})
			select {
			case <-handled:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return nil, nil
	}
	go r.play([]node{{ID: "trigger", Type: "click"}, {ID: "dialog", Type: "dialog", Dialog: "accept"}, {ID: "after", Type: "click"}, {ID: "confirm", Type: "confirm", Message: "Stop"}, {ID: "submit", Type: "click"}}, "stop")
	cursor := 0
	awaitRun(t, r, &cursor, func(o runUpdate) bool { return o.State == "paused" && o.Node == "trigger" })
	r.control("step", "")
	awaitRun(t, r, &cursor, func(o runUpdate) bool { return o.State == "paused" && o.Node == "dialog" })
	r.control("resume", "")
	awaitRun(t, r, &cursor, func(o runUpdate) bool { return o.State == "paused" && o.Node == "confirm" })
	r.control("cancel", "")
	awaitRun(t, r, &cursor, func(o runUpdate) bool { return o.State == "cancelled" })
}
