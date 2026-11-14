package console

import (
	"context"
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

// A call's myco side on a real hub: what the person says goes to the agent
// as a new task when it is free and as an addition while it works; the
// model reads the conversation; the agent's answer is news.
func TestVoiceHubActionsOnAHub(t *testing.T) {
	const admin = "voice-test-admin-token-0123456789"
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: admin,
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h.Handler)
	t.Cleanup(func() { hs.Close(); h.Close() })
	ctx := context.Background()
	ac, _ := client.New(hs.URL, admin)
	reg, err := ac.Register(ctx, api.RegisterRequest{Name: "backend", DisplayName: "后端", Description: "修 bug"}, false)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := ac.CreateConsoleToken(ctx, "desk")
	if err != nil {
		t.Fatal(err)
	}
	cc, _ := client.New(hs.URL, ct.Token)
	peer, err := consoleAddress(ctx, cc)
	if err != nil {
		t.Fatal(err)
	}
	ha := &hubActions{hubID: "h1", agent: "backend", peer: peer, c: cc}

	if cur, err := ha.Current(ctx); err != nil || cur != nil {
		t.Fatalf("current before anything: %v %+v", err, cur)
	}
	ref, steered, err := ha.Tell(ctx, "修好登录页\n\n（经语音转述）")
	if err != nil || steered || ref.ID == "" || ref.Agent != "backend" || ref.Thread == "" {
		t.Fatalf("tell (new task): %v %v %+v", err, steered, ref)
	}
	if ha.Name() == "" {
		t.Fatal("no name")
	}
	s, err := ha.Context(ctx)
	if err != nil || !strings.Contains(s, "backend（后端），修 bug") || !strings.Contains(s, "修好登录页") || !strings.Contains(s, "它手上正在做你交的") {
		t.Fatalf("context: %v %q", err, s)
	}
	if ha.Name() != "后端" {
		t.Fatalf("name: %q", ha.Name())
	}
	// While it is open, more goes to the same task.
	again, steered, err := ha.Tell(ctx, "顺便看看注册页")
	if err != nil || !steered || again.ID != ref.ID {
		t.Fatalf("tell (addition): %v %v %+v", err, steered, again)
	}
	if n, err := ha.TaskNews(ctx, ref); err != nil || n.Finished {
		t.Fatalf("news before an answer: %v %+v", err, n)
	}
	bot, _ := client.New(hs.URL, reg.Token)
	if _, err := bot.Pull(ctx, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Reply(ctx, ref.ID, "修好了，登录测试全过", false); err != nil {
		t.Fatal(err)
	}
	if n, err := ha.TaskNews(ctx, ref); err != nil || !n.Finished || !strings.Contains(n.Summary, "登录测试全过") {
		t.Fatalf("news after the answer: %v %+v", err, n)
	}
	// Answered: the next thing is a new task.
	if cur, err := ha.Current(ctx); err != nil || cur != nil {
		t.Fatalf("current after the answer: %v %+v", err, cur)
	}
	next, steered, err := ha.Tell(ctx, "再把注册页修了")
	if err != nil || steered || next.ID == ref.ID {
		t.Fatalf("tell after the answer: %v %v %+v", err, steered, next)
	}
	if _, err := ha.StopCurrent(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
}
