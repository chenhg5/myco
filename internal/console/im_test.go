package console_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/console"
)

// Chat-app bots from the App: listed per workspace, and Slack's tokens go
// to myco im connect in files, never on its command line.
func TestIMBridges(t *testing.T) {
	r := newRig(t)
	code, hub, _ := r.do("POST", "/app/hubs", map[string]string{"name": "w", "url": r.hubURL, "token": r.consoleToken("ann")})
	if code != 200 && code != 201 {
		t.Fatalf("add hub: %d %v", code, hub)
	}
	hubID, _ := hub["id"].(string)
	var connect []string
	var secret string
	run := func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		cmd := strings.Join(args[:len(args)-2], " ") // without --format json
		switch {
		case cmd == "im list":
			return []byte(`{"dir":"/nowhere","bridges":[` +
				`{"name":"feishu","platform":"feishu","agent":"coder","owner":"ou_1","hub":"` + r.hubURL + `","node":"im-feishu","online":true},` +
				`{"name":"slack","platform":"slack","hub":"https://other.example","online":false}]}`), nil
		case strings.HasPrefix(cmd, "im connect slack"):
			connect = args
			i := slices.Index(args, "--bot-token-file")
			b, err := os.ReadFile(args[i+1])
			if err != nil {
				return nil, err
			}
			secret = string(b)
			return []byte(`{"bridge":{"name":"slack-2","platform":"slack","agent":"coder","owner":""}}`), nil
		case strings.HasPrefix(cmd, "im set") || strings.HasPrefix(cmd, "im remove"):
			return []byte(`{}`), nil
		}
		return nil, errors.New("unexpected: " + cmd)
	}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets,
		Local: &console.Local{Home: t.TempDir(), Run: run, LookPath: func(string) (string, error) { return "/usr/local/bin/myco", nil }}}
	r.app.Config.Handler = b.Handler()

	code, m, raw := r.do("GET", "/app/im?hub_id="+hubID, nil)
	if code != 200 {
		t.Fatalf("list: %d %s", code, raw)
	}
	list := m["bridges"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["name"] != "feishu" || list[0].(map[string]any)["has_owner"] != true {
		t.Fatalf("this workspace's bots: %s", raw)
	}
	if !strings.Contains(m["slack_create_url"].(string), "manifest_json") {
		t.Fatalf("create url: %v", m["slack_create_url"])
	}

	if code, m, _ := r.do("POST", "/app/im/slack", map[string]string{"hub_id": hubID, "agent": "coder", "bot_token": "xoxb-1", "app_token": "nope"}); code != 400 {
		t.Fatalf("bad token: %d %v", code, m)
	}
	code, m, raw = r.do("POST", "/app/im/slack", map[string]string{"hub_id": hubID, "agent": "coder", "bot_token": " xoxb-1-2 ", "app_token": "xapp-3"})
	if code != 200 || m["bridge"].(map[string]any)["name"] != "slack-2" {
		t.Fatalf("connect: %d %s", code, raw)
	}
	if secret != "xoxb-1-2" {
		t.Fatalf("token file held %q", secret)
	}
	for _, a := range connect {
		if strings.Contains(a, "xoxb-") || strings.Contains(a, "xapp-") {
			t.Fatalf("a token on the command line: %v", connect)
		}
	}
	if !slices.Contains(connect, "--name") || connect[slices.Index(connect, "--name")+1] != "slack-2" || !slices.Contains(connect, r.hubURL) {
		t.Fatalf("connect args %v", connect)
	}
	if code, _, raw := r.do("PUT", "/app/im/feishu", map[string]string{"agent": "reviewer"}); code != 200 {
		t.Fatalf("set: %d %s", code, raw)
	}
	if code, _, _ := r.do("DELETE", "/app/im/Bad..Name", nil); code != 400 {
		t.Fatalf("bad name: %d", code)
	}
	if code, _, raw := r.do("DELETE", "/app/im/feishu", nil); code != 200 {
		t.Fatalf("remove: %d %s", code, raw)
	}

	// A myco from before chat apps.
	b.Local.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte(`{"error":"invalid_usage","message":"unknown command \"im\" for myco"}`), errors.New("exit status 2")
	}
	if code, m, _ := r.do("GET", "/app/im?hub_id="+hubID, nil); code != 409 || m["error"] != "cli_too_old" {
		t.Fatalf("old cli: %d %v", code, m)
	}
}
