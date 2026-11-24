package integration_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/claude"
	"github.com/chenhg5/myco/internal/runner"
)

func TestUsageRunnerFakeClaudeAndSessionDedup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake Claude executable")
	}
	t.Setenv("MYCO_NO_USAGE", "")
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	ctx := ctx(t)
	sender := h.client(h.register(api.RegisterRequest{Name: "sender"}))
	worker := h.client(h.register(api.RegisterRequest{Name: "worker", Binding: api.Binding{Harness: "claude"}}))
	if _, err := sender.Send(ctx, api.SendRequest{To: "worker", Body: "numeric accounting fixture", Type: "task"}, false); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "claude")
	body := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"assistant","message":{"id":"fixture","model":"claude-fixture","content":[]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","session_id":"fixture-session","total_cost_usd":0.25,"usage":{"input_tokens":11,"cache_read_input_tokens":40,"cache_creation_input_tokens":7,"output_tokens":9}}'
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cli, err := claude.New(harness.Options{Binary: script, Access: harness.AccessFull})
	if err != nil {
		t.Fatal(err)
	}
	r := runner.New(worker, cli, runner.NewMemorySessions(), nil, runner.Options{Once: true, Trace: runner.TraceOff, Self: "worker", Workdir: t.TempDir(), Timeout: 5 * time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	out, err := h.admin().Usage(ctx, client.UsageFilter{From: day, To: day})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("usage %+v %v", out, err)
	}
	p := out.Rows[0]
	if p.Agent != "worker" || p.Source != "runner" || p.InputTokens != 11 || p.CacheReadTokens != 40 || p.CacheWriteTokens != 7 || p.OutputTokens != 9 || p.CostUSD != 0.25 || p.Model != "claude-fixture" {
		t.Fatalf("runner numeric accounting %+v", p)
	}
	request := api.UsageRequest{Source: "session", Reports: []api.UsageReport{{Key: "same-session-key", Day: day, Model: "opus", Calls: 1, InputTokens: 5}}}
	for _, n := range []int{1, 0} {
		recorded, err := worker.PostUsage(ctx, request)
		if err != nil || recorded == nil || recorded.Recorded != n {
			t.Fatalf("dedup %+v %v", recorded, err)
		}
	}
	// A message reference supplies its project, but cannot expose another
	// agent's unrelated message as a usage attribution oracle.
	if _, err := h.admin().CreateProject(ctx, api.CreateProjectRequest{Name: "billing"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sender", "worker"} {
		if _, err := h.admin().PutProjectMember(ctx, "billing", name, api.ProjectMemberRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	msg, err := sender.Send(ctx, api.SendRequest{To: "worker", Body: "project fixture", Project: "billing", Type: "task"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.PostUsage(ctx, api.UsageRequest{Reports: []api.UsageReport{{Key: "project-from-message", Day: day, MessageID: msg.Message.ID, Calls: 1}}}); err != nil {
		t.Fatal(err)
	}
	projectRows, err := h.admin().Usage(ctx, client.UsageFilter{Project: "billing"})
	if err != nil || len(projectRows.Rows) != 1 || projectRows.Rows[0].Project != "billing" {
		t.Fatalf("message project %+v %v", projectRows, err)
	}
	// Ordinary agents cannot read workspace-wide usage or impersonate another reporter.
	if _, err := worker.Usage(ctx, client.UsageFilter{}); err == nil {
		t.Fatal("agent read console usage")
	}
	for _, bad := range []api.UsageRequest{{Reports: []api.UsageReport{{Day: day, InputTokens: -1}}}, {Reports: []api.UsageReport{{Day: day, Calls: 100000000001}}}, {Reports: []api.UsageReport{{Day: "2026-02-30"}}}, {Source: "trace", Reports: []api.UsageReport{{Day: day}}}} {
		if _, err := worker.PostUsage(ctx, bad); err == nil {
			t.Fatal("invalid usage accepted")
		}
	}
	if _, err := h.admin().Usage(ctx, client.UsageFilter{From: "2025-01-01", To: "2026-11-24"}); err == nil {
		t.Fatal("oversized range accepted")
	}
	// Turning on workspace encryption must not block the independent numeric API.
	if _, err := h.admin().SetHubSettings(ctx, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	recorded, err := worker.PostUsage(ctx, api.UsageRequest{Reports: []api.UsageReport{{Key: "encrypted-numeric", Day: day, Model: "encrypted-numeric", Calls: 1, InputTokens: 1}}})
	if err != nil || recorded == nil || recorded.Recorded != 1 {
		t.Fatalf("encrypted workspace accounting %+v %v", recorded, err)
	}
	encrypted, err := h.admin().Usage(ctx, client.UsageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range encrypted.Rows {
		if p.Model == "encrypted-numeric" && p.InputTokens == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("encrypted workspace lost numeric usage")
	}

}
func TestUsageMemberVisibilityAndProjectFilters(t *testing.T) {
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	c := ctx(t)
	admin := h.admin()
	mine := h.client(h.register(api.RegisterRequest{Name: "owners-agent"}))
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "member"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "member-device")
	if err != nil {
		t.Fatal(err)
	}
	member := h.client(paired.Token)
	en, err := member.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "members-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := h.client(en.Token).Enroll(c, api.EnrollRequest{Name: "members-agent", Binding: api.Binding{Harness: "claude", SessionID: "member-session"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	other := h.client(reg.Token)
	day := time.Now().UTC().Format("2006-01-02")
	for _, cl := range []*client.Client{mine, other} {
		if _, err := cl.PostUsage(c, api.UsageRequest{Reports: []api.UsageReport{{Day: day, Project: "project", Calls: 1, InputTokens: 10}, {Day: day, Calls: 2, InputTokens: 20}}}); err != nil {
			t.Fatal(err)
		}
	}
	own, err := member.Usage(c, client.UsageFilter{})
	if err != nil || len(own.Rows) != 2 {
		t.Fatalf("member usage %+v %v", own, err)
	}
	for _, p := range own.Rows {
		if p.Agent != "members-agent" {
			t.Fatal("other member usage leaked")
		}
	}
	for _, a := range own.Agents {
		if a.Name != "members-agent" {
			t.Fatal("other member metadata leaked")
		}
	}
	all, err := admin.Usage(c, client.UsageFilter{})
	if err != nil || len(all.Rows) != 4 {
		t.Fatalf("admin %+v %v", all, err)
	}
	only, err := member.Usage(c, client.UsageFilter{Project: "project"})
	if err != nil || len(only.Rows) != 1 || only.Rows[0].Project != "project" {
		t.Fatalf("project %+v %v", only, err)
	}
	none, err := member.Usage(c, client.UsageFilter{Project: "-"})
	if err != nil || len(none.Rows) != 1 || none.Rows[0].Project != "" {
		t.Fatalf("unassigned %+v %v", none, err)
	}
}
