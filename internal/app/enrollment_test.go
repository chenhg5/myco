package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

func setupEnroll(t *testing.T) (*app.EnrollmentService, *app.AgentService) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar := sqlite.NewAgentRepo(db)
	clock := app.SystemClock{}
	return app.NewEnrollmentService(sqlite.NewEnrollmentRepo(db), ar, security.Tokens{}, security.IDs{}, clock),
		app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, nil)
}

func cloud(session string) domain.Binding {
	return domain.Binding{Harness: "claude-cloud", SessionID: session, Environment: "claude-cloud_default"}
}

func TestEnrollCreatesAndRecoversSameSession(t *testing.T) {
	ctx := context.Background()
	enr, agents := setupEnroll(t)
	e, tok, err := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "cloud-", Harness: "claude-cloud",
		AllowFrom: []string{"glenn"}, Capabilities: []string{"code"}, MaxUses: 1})
	if err != nil || tok == "" {
		t.Fatalf("create: %v", err)
	}

	first, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("session_01CewNoAL2FdwDRtyJNyiuG5")})
	if err != nil || !first.Created {
		t.Fatalf("enroll: %+v %v", first, err)
	}
	a := first.Agent
	if a.Name != "cloud-yjnyiug5" || a.EnrolledBy != e.ID || a.Delivery.Mode != domain.DeliveryPull ||
		len(a.AllowFrom) != 1 || a.Capabilities[0] != "code" {
		t.Fatalf("agent: %+v", a)
	}
	// Same session again (e.g. container restarted): same agent, new token,
	// no use consumed (max-uses is 1 and already spent).
	again, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("session_01CewNoAL2FdwDRtyJNyiuG5")})
	if err != nil || again.Created || again.Agent.Name != a.Name || again.Token == first.Token {
		t.Fatalf("recover: %+v %v", again, err)
	}
	if _, err := agents.Authenticate(ctx, first.Token); !domain.IsCode(err, domain.CodeUnauthenticated) {
		t.Fatal("old token must stop working after recovery")
	}
	if _, err := agents.Authenticate(ctx, again.Token); err != nil {
		t.Fatalf("new token: %v", err)
	}
	// A different session needs a free use.
	if _, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("session_other")}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("max uses: %v", err)
	}
}

func TestEnrollGuards(t *testing.T) {
	ctx := context.Background()
	enr, agents := setupEnroll(t)
	_, tok, _ := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "cloud-", Harness: "claude-cloud"})

	cases := map[string]struct {
		in   app.EnrollInput
		code domain.ErrorCode
	}{
		"no session":     {app.EnrollInput{Binding: domain.Binding{Harness: "claude-cloud"}}, domain.CodeInvalidArgument},
		"wrong harness":  {app.EnrollInput{Binding: domain.Binding{Harness: "codex", SessionID: "s"}}, domain.CodePermissionDenied},
		"name w/o prefx": {app.EnrollInput{Name: "admin", Binding: cloud("s1")}, domain.CodePermissionDenied},
	}
	for name, c := range cases {
		if _, err := enr.Enroll(ctx, tok, c.in); !domain.IsCode(err, c.code) {
			t.Errorf("%s: want %s, got %v", name, c.code, err)
		}
	}
	if _, err := enr.Enroll(ctx, "myco_agenttoken", app.EnrollInput{Binding: cloud("s1")}); !domain.IsCode(err, domain.CodeUnauthenticated) {
		t.Errorf("agent token must not work as enrollment token: %v", err)
	}
	// Cannot take over an admin-registered agent bound to the same session.
	if _, err := agents.Register(ctx, app.RegisterInput{Name: "boss", Binding: cloud("s-admin")}); err != nil {
		t.Fatal(err)
	}
	if _, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("s-admin")}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Errorf("hijack of admin agent: %v", err)
	}
}

func TestEnrollmentExpiryAndRevoke(t *testing.T) {
	ctx := context.Background()
	enr, _ := setupEnroll(t)
	_, short, _ := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "tmp-", TTL: time.Millisecond})
	time.Sleep(5 * time.Millisecond)
	if _, err := enr.Enroll(ctx, short, app.EnrollInput{Binding: cloud("s1")}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("expired: %v", err)
	}
	e, tok, _ := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "x-"})
	if _, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("s2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := enr.Revoke(ctx, e.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := enr.Enroll(ctx, tok, app.EnrollInput{Binding: cloud("s2")}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("revoked enrollment must not even recover: %v", err)
	}
	// Dry-run creates nothing.
	if _, tok, err := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "d-", DryRun: true}); err != nil || tok != "" {
		t.Fatalf("dry-run create: %q %v", tok, err)
	}
}

func TestEnrollRefusesAliasNames(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar, aliases := sqlite.NewAgentRepo(db), sqlite.NewAliasRepo(db)
	clock := app.SystemClock{}
	agents := app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, nil)
	agents.SetAliases(aliases)
	enr := app.NewEnrollmentService(sqlite.NewEnrollmentRepo(db), ar, security.Tokens{}, security.IDs{}, clock)
	enr.SetAliases(aliases)

	if _, err := agents.Register(ctx, app.RegisterInput{Name: "cloud-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.SetAlias(ctx, "cloud-reviewer", "cloud-a", false); err != nil {
		t.Fatal(err)
	}
	_, tok, _ := enr.Create(ctx, app.CreateEnrollmentInput{NamePrefix: "cloud-"})
	if _, err := enr.Enroll(ctx, tok, app.EnrollInput{Name: "cloud-reviewer", Binding: cloud("s9")}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("enrolling under an alias name: %v", err)
	}
	if got, err := agents.Get(ctx, "cloud-reviewer"); err != nil || got.Name != "cloud-a" {
		t.Fatalf("alias get: %v %v", got, err)
	}
}
