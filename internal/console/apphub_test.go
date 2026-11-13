package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

// Without a service manager (Windows), starting the local hub installs the
// CLI when missing, prepares the hub with --no-service, runs the printed
// command in the background, and remembers it; stopping forgets it.
func TestLocalHubWithoutServiceManager(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	mycoPath := ""
	var calls []string
	var started [][]string
	run := func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		c := strings.Join(args, " ")
		calls = append(calls, c)
		switch {
		case strings.HasPrefix(c, "hub up --tunnel none"):
			dir := filepath.Join(home, ".myco", "hub")
			addr := "0.0.0.0:7777"
			if strings.Contains(c, "--name work") {
				dir = filepath.Join(home, ".myco", "hubs", "work")
				addr = "0.0.0.0:7778"
			}
			os.MkdirAll(dir, 0o700)
			os.WriteFile(filepath.Join(dir, "hub.env"), []byte("MYCO_ADMIN_TOKEN="+adminToken+"\nMYCO_HUB="+r.hubURL+"\n"), 0o600)
			return json.Marshal(map[string]any{"dir": dir, "command": []string{"C:/myco.exe", "hub", "serve", "--addr", addr}})
		case strings.HasPrefix(c, "console token create"):
			res, err := r.admin.CreateConsoleToken(context.Background(), "pc-app")
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{"token": res.Token})
		case strings.HasPrefix(c, "version"):
			return []byte(`{"version":"v1"}`), nil
		}
		return nil, errors.New("unexpected: " + c)
	}
	l := &console.Local{Home: home, Device: "pc", Run: run, NoServiceManager: true, Dirs: []string{filepath.Join(home, ".local", "bin")},
		LookPath: func(name string) (string, error) {
			if name == "myco" && mycoPath != "" {
				return mycoPath, nil
			}
			return "", errors.New("no")
		},
		Install: func(_ context.Context, dir string) (string, error) {
			mycoPath = filepath.Join(dir, "myco.exe")
			return mycoPath, nil
		},
		StartProcess: func(command []string, logPath string, _ []string) (int, error) {
			started = append(started, command)
			return 4242, nil
		}}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: l}
	r.app.Config.Handler = b.Handler()

	code, m, raw := r.do("POST", "/app/local/hub", map[string]string{"tunnel": "none"})
	if code != 201 {
		t.Fatalf("start: %d %s", code, raw)
	}
	if mycoPath == "" {
		t.Fatal("the CLI must be installed first")
	}
	if len(started) != 1 || started[0][1] != "hub" || started[0][2] != "serve" {
		t.Fatalf("hub not started from the app: %v (calls %v)", started, calls)
	}
	if hub, _ := m["hub"].(map[string]any); hub == nil || hub["ok"] != true {
		t.Fatalf("hub not added: %s", raw)
	}
	b2, err := os.ReadFile(filepath.Join(home, ".myco", "hub", "app-hub.json"))
	if err != nil || !strings.Contains(string(b2), `"pid":4242`) {
		t.Fatalf("app-hub.json: %v %s", err, b2)
	}
	if code, _, raw := r.do("POST", "/app/local/hub/stop", nil); code != 200 {
		t.Fatalf("stop: %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(home, ".myco", "hub", "app-hub.json")); !os.IsNotExist(err) {
		t.Fatal("a stopped hub must not be started again with the app")
	}
	if code, _, raw := r.do("POST", "/app/local/hub", map[string]string{"name": "work", "tunnel": "none"}); code != 200 {
		// The fake uses the same hub URL as the default, so it is already in the App.
		t.Fatalf("named start: %d %s", code, raw)
	}
	if len(started) != 2 || started[1][4] != "0.0.0.0:7778" {
		t.Fatalf("named app hub command: %v", started)
	}
	if b, err := os.ReadFile(filepath.Join(home, ".myco", "hubs", "work", "app-hub.json")); err != nil || !strings.Contains(string(b), `"pid":4242`) {
		t.Fatalf("named app-hub.json: %v %s", err, b)
	}
	if code, _, raw := r.do("POST", "/app/local/hub/stop", map[string]string{"name": "work"}); code != 200 {
		t.Fatalf("named stop: %d %s", code, raw)
	}
}
