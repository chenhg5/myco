package console_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/console"
)

// Connecting a computer over SSH: the App logs in, looks around, runs the
// join command there with the link on curl's stdin, then waits for the
// computer on the workspace and remembers how it got there.
func TestConnectOverSSH(t *testing.T) {
	r := newRig(t)
	var mu sync.Mutex
	var calls []string
	var scripts []string
	remote := func(ctx context.Context, name string, args []string, stdin string, line func(string)) (string, int, error) {
		mu.Lock()
		calls = append(calls, name+" "+strings.Join(args, " "))
		scripts = append(scripts, stdin)
		mu.Unlock()
		target := args[len(args)-2]
		switch {
		case target == "locked":
			return "locked: Permission denied (publickey).", 255, nil
		case strings.Contains(stdin, "myco-probe"):
			for _, l := range []string{"myco-probe os Linux", "myco-probe arch x86_64", "myco-probe host vps1", "myco-probe bash yes", "myco-probe curl yes", "myco-probe end ok"} {
				line(l)
			}
			return "", 0, nil
		case strings.Contains(stdin, "curl -fsSL"):
			// What the join command does there: a machine node reports its computer.
			reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "vps1-machine", Binding: api.Binding{Harness: "machine", Environment: "vps1"}}, false)
			if err != nil {
				return err.Error(), 1, nil
			}
			wc, _ := client.New(r.hubURL, reg.Token)
			if err := wc.ReportMachine(ctx, api.MachineReport{Fingerprint: strings.Repeat("ab", 16), Name: "vps1", OS: "linux"}); err != nil {
				return err.Error(), 1, nil
			}
			line("==> installing myco")
			line("myco-node vps1-machine")
			return "", 0, nil
		}
		return "unexpected", 1, nil
	}
	store := filepath.Join(t.TempDir(), "app", "hubs.json")
	b := &console.Backend{Store: console.NewStore(store), Secrets: r.secrets, Timeout: 3 * time.Second,
		Local: &console.Local{Home: t.TempDir(), Remote: remote}}
	r.app = httptest.NewServer(b.Handler())
	t.Cleanup(r.app.Close)
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, m)
	}
	id := m["id"].(string)

	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/connect", map[string]any{"via": "ssh", "target": "-oProxyCommand=x"}); code != 400 {
		t.Fatalf("an option as a host must be refused: %d %v", code, m)
	}
	wait := func(job string) map[string]any {
		t.Helper()
		for i := 0; i < 100; i++ {
			_, j, _ := r.do("GET", "/app/connect/"+job, nil)
			if j["done"] == true {
				return j
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("connecting never finished")
		return nil
	}

	// A key the App cannot use: told in words, with the command to try.
	_, m, _ = r.do("POST", "/app/hubs/"+id+"/machines/connect", map[string]any{"via": "ssh", "target": "locked"})
	j := wait(m["id"].(string))
	if e, _ := j["error"].(map[string]any); e["code"] != "auth" || e["command"] != "ssh locked" {
		t.Fatalf("auth failure: %v", j)
	}

	code, m, _ = r.do("POST", "/app/hubs/"+id+"/machines/connect", map[string]any{"via": "ssh", "target": "root@vps1", "port": 2222})
	if code != 200 {
		t.Fatalf("connect: %d %v", code, m)
	}
	j = wait(m["id"].(string))
	if j["error"] != nil || j["machine_id"] == "" || j["machine_id"] == nil {
		t.Fatalf("connected: %v", j)
	}
	steps := j["steps"].([]any)
	for _, s := range steps {
		if s.(map[string]any)["state"] != "done" {
			t.Fatalf("steps: %v", steps)
		}
	}
	if !strings.Contains(strings.Join(toStrings(j["log"]), "\n"), "installing myco") {
		t.Fatalf("log: %v", j["log"])
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range calls {
		if strings.Contains(c, "/join/") || strings.Contains(c, "myco_") {
			t.Fatalf("the join link was on a command line: %s", c)
		}
		if strings.Contains(c, "root@vps1") && !strings.Contains(c, "-p 2222") {
			t.Fatalf("port not passed: %s", c)
		}
	}
	if !strings.Contains(scripts[len(scripts)-1], "| curl -fsSL --retry 3 -K - 2>&1)") {
		t.Fatalf("install script: %s", scripts[len(scripts)-1])
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
