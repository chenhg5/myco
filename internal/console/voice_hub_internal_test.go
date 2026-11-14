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

// The voice assistant acts as the person on a real hub: hands out a task,
// reads how it stands, and hears when the agent has answered.
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
	ha := &hubActions{hubID: "h1", hubName: "Studio", c: cc}

	m, err := ha.Members(ctx)
	if err != nil || !strings.Contains(m, "backend（后端）：修 bug") {
		t.Fatalf("members: %v %q", err, m)
	}
	ref, err := ha.Send(ctx, "backend", "修好登录页\n\n（经语音转述）")
	if err != nil || ref.ID == "" || ref.Agent != "backend" {
		t.Fatalf("send: %v %+v", err, ref)
	}
	if s, err := ha.Status(ctx, ref.ID); err != nil || !strings.Contains(s, "给 backend") {
		t.Fatalf("status: %v %q", err, s)
	}
	if n, err := ha.TaskNews(ctx, ref.ID); err != nil || n.Finished {
		t.Fatalf("news before an answer: %v %+v", err, n)
	}
	bot, _ := client.New(hs.URL, reg.Token)
	if _, err := bot.Pull(ctx, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Reply(ctx, ref.ID, "修好了，登录测试全过", false); err != nil {
		t.Fatal(err)
	}
	if n, err := ha.TaskNews(ctx, ref.ID); err != nil || !n.Finished || !strings.Contains(n.Summary, "登录测试全过") {
		t.Fatalf("news after the answer: %v %+v", err, n)
	}
}
