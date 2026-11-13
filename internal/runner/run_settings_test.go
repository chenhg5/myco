package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// settingsHub answers GET /v1/me with run settings, and records traces.
type settingsHub struct {
	*traceHub
	settings *api.RunSettings
	fail     bool
}

func (h *settingsHub) Me(context.Context) (*api.Agent, error) {
	if h.fail {
		return nil, errors.New("hub unavailable")
	}
	return &api.Agent{Name: "worker", RunSettings: h.settings}, nil
}

func TestRunUsesTheAgentsRunSettingsAndReportsQuota(t *testing.T) {
	m := api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}
	hub := &settingsHub{traceHub: &traceHub{fakeHub: newFakeHub(m)}, settings: &api.RunSettings{Model: "app-model", Effort: "high"}}
	var gotModel, gotEffort, toldModel, toldEffort string
	var quotas []harness.Quota
	h := fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		gotModel, gotEffort = req.Model, req.Effort
		req.OnEvent(harness.Event{Kind: "text", Text: "working"})
		req.OnEvent(harness.Event{Kind: "quota", Quota: &harness.Quota{Source: "claude", Windows: []harness.QuotaWindow{{Name: "five_hour", Used: 0.4}}}})
		return harness.Result{Output: "done"}, nil
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{
		OnRunSettings: func(model, effort string) { toldModel, toldEffort = model, effort },
		OnQuota:       func(q harness.Quota) { quotas = append(quotas, q) },
	}, quiet())
	r.Handle(context.Background(), m)
	if gotModel != "app-model" || gotEffort != "high" || toldModel != "app-model" || toldEffort != "high" {
		t.Fatalf("settings: run %q/%q, reported %q/%q", gotModel, gotEffort, toldModel, toldEffort)
	}
	if len(quotas) != 1 || quotas[0].Windows[0].Used != 0.4 {
		t.Fatalf("quota: %+v", quotas)
	}
	hub.mu.Lock()
	traced := append([]api.TraceEvent(nil), hub.events...)
	hub.mu.Unlock()
	for _, ev := range traced {
		if ev.Kind == "quota" {
			t.Fatalf("quota is not a step of the run: %+v", traced)
		}
	}

	// The hub cannot answer: the last settings known still apply.
	hub.fail = true
	m2 := api.Message{ID: "t2", Type: "task", From: "alice", ThreadID: "th"}
	hub.fakeHub.status[m2.ID] = "delivered"
	gotModel = ""
	r.Handle(context.Background(), m2)
	if gotModel != "app-model" {
		t.Fatalf("a failed read should keep the last settings, got %q", gotModel)
	}
}
