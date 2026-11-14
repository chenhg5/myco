package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/hubserver"
)

// startServer runs a hub with its workspaces, as `myco hub serve` does.
func startServer(t *testing.T, db string) (url string, stop func()) {
	t.Helper()
	o := hubserver.Options{DBPath: db, AdminToken: adminToken, Policy: app.DefaultMessagePolicy(),
		OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h, err := hubserver.Build(o)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	handler, closeWS, err := hubserver.Serve(ctx, o, h)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	return srv.URL, func() { srv.Close(); closeWS(); cancel(); h.Close() }
}

// One hub serves several workspaces: each has its own members, agents and
// messages; links name the workspace; they outlive a restart; only the
// server's owner manages them; the hub itself carries on as before.
func TestWorkspacesOnOneHub(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	url, stop := startServer(t, db)
	defer func() { stop() }()
	mk := func(base, token string) *client.Client {
		t.Helper()
		cl, err := client.New(base, token)
		if err != nil {
			t.Fatal(err)
		}
		return cl
	}
	admin := mk(url, adminToken)

	// The hub itself (the default workspace) as before.
	if _, err := admin.Register(c, api.RegisterRequest{Name: "home-bot"}, false); err != nil {
		t.Fatal(err)
	}
	if ws, err := admin.Workspaces(c); err != nil || len(ws) != 0 {
		t.Fatalf("no workspaces yet: %v %+v", err, ws)
	}

	// A new workspace, on the same address.
	if _, err := admin.CreateWorkspace(c, "Bad Name", false); err == nil {
		t.Fatal("an invalid name must be refused")
	}
	dry, err := admin.CreateWorkspace(c, "team-b", true)
	if err != nil || dry.URL != url+"/w/team-b" {
		t.Fatalf("dry run: %v %+v", err, dry)
	}
	if ws, _ := admin.Workspaces(c); len(ws) != 0 {
		t.Fatal("a dry run must not create it")
	}
	created, err := admin.CreateWorkspace(c, "team-b", false)
	if err != nil || created.URL != url+"/w/team-b" {
		t.Fatalf("create: %v %+v", err, created)
	}
	if _, err := admin.CreateWorkspace(c, "team-b", false); err == nil {
		t.Fatal("a second workspace of the same name must be refused")
	}

	// Its owner gets in with an app link that names the workspace.
	b := mk(created.URL, adminToken)
	h, err := b.Health(c)
	if err != nil || h.Workspace != "team-b" {
		t.Fatalf("health: %v %+v", err, h)
	}
	link, err := b.CreatePairing(c)
	if err != nil || !strings.HasPrefix(link.Link, url+"/w/team-b/c/") {
		t.Fatalf("app link: %v %+v", err, link)
	}
	paired, err := mk(created.URL, "").Pair(c, link.Link[strings.LastIndex(link.Link, "/c/")+3:], "laptop")
	if err != nil {
		t.Fatal(err)
	}
	owner := mk(created.URL, paired.Token)

	// Isolated: an agent and a message here are not in the hub's default
	// workspace, and the other way round.
	reg, err := b.Register(c, api.RegisterRequest{Name: "b-bot"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ConsoleSend(c, api.SendRequest{To: "b-bot", Body: "hello b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ConsoleSend(c, api.SendRequest{To: "home-bot", Body: "x"}); err == nil {
		t.Fatal("the default workspace's agent must not be reachable from team-b")
	}
	home, err := admin.ListAgents(c, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range home {
		if a.Name == "b-bot" {
			t.Fatal("team-b's agent leaked into the default workspace")
		}
	}
	if _, err := mk(url, paired.Token).Whoami(c); err == nil {
		t.Fatal("team-b's app credential must not work on the default workspace")
	}
	bbot := mk(created.URL, reg.Token)
	if msgs, err := bbot.Pull(c, 10, 0); err != nil || len(msgs) != 1 || msgs[0].Body != "hello b" {
		t.Fatalf("pull in team-b: %v %+v", err, msgs)
	}

	// Only the server's owner manages workspaces.
	var ce *client.Error
	if _, err := mk(url, paired.Token).Workspaces(c); err == nil || !errors.As(err, &ce) || ce.Code != "permission_denied" {
		t.Fatalf("someone else listing workspaces: %v", err)
	}
	if _, err := mk(url+"/w/nope", adminToken).Health(c); err == nil {
		t.Fatal("an unknown workspace must answer not found")
	}

	// A restart brings it back as it was.
	stop()
	url2, stop2 := startServer(t, db)
	stop = stop2
	if ws, err := mk(url2, adminToken).Workspaces(c); err != nil || len(ws) != 1 || ws[0].Name != "team-b" {
		t.Fatalf("after restart: %v %+v", err, ws)
	}
	if me, err := mk(url2+"/w/team-b", reg.Token).Me(c); err != nil || me.Name != "b-bot" {
		t.Fatalf("agent after restart: %v %+v", err, me)
	}

	// Removed: gone from the hub, its data kept aside.
	if err := mk(url2, adminToken).RemoveWorkspace(c, "team-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := mk(url2+"/w/team-b", reg.Token).Me(c); err == nil {
		t.Fatal("a removed workspace must stop answering")
	}
	if matches, _ := filepath.Glob(filepath.Join(hubserver.WorkspacesDir(db), ".removed", "team-b-*", "hub.db")); len(matches) != 1 {
		t.Fatalf("removed workspace data should be kept aside: %v", matches)
	}
}
