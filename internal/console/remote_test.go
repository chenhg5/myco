package console_test

import (
	"context"
	"fmt"
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
		case strings.Contains(stdin, "myco-check"):
			for _, l := range []string{"myco-check myco v0.20.17", "myco-check service active", "myco-check hub " + r.hubURL, "myco-check hub_direct " + r.hubURL + " 200", "myco-check end ok"} {
				line(l)
			}
			return "", 0, nil
		case strings.Contains(stdin, "runner service restart"):
			line("restarted")
			return "", 0, nil
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
	mid := j["machine_id"].(string)
	mu.Unlock()
	// The App remembers how it got there: a check-up goes the same way.
	if code, m, _ := r.do("GET", "/app/hubs/"+id+"/machines/"+mid+"/reach", nil); code != 200 || m["route"].(map[string]any)["target"] != "root@vps1" {
		t.Fatalf("reach: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/"+mid+"/diagnose", nil); code != 200 || m["found"].(map[string]any)["service"] != "active" {
		t.Fatalf("diagnose: %d %v", code, m)
	} else if hs := m["hubs"].([]any); len(hs) != 1 || hs[0].(map[string]any)["mine"] != true {
		t.Fatalf("hubs: %v", hs)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/"+mid+"/fix", map[string]string{"action": "rm -rf"}); code != 400 {
		t.Fatalf("an unknown fix must be refused: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/"+mid+"/fix", map[string]string{"action": "restart"}); code != 200 || m["ok"] != true {
		t.Fatalf("restart: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/nope/diagnose", nil); code != 409 {
		t.Fatalf("unknown route: %d %v", code, m)
	}
	mu.Lock()
	if !strings.Contains(scripts[len(scripts)-1], "runner service restart") {
		t.Fatalf("restart script: %s", scripts[len(scripts)-1])
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

// Setting up a workspace on a server over SSH: log in, install the hub,
// make an App link there, and the workspace is in this App, with no link
// in what the App shows. Without root or passwordless sudo it says so.
func TestHubOverSSH(t *testing.T) {
	r := newRig(t)
	tok := r.consoleToken("server")
	var scripts []string
	remote := func(_ context.Context, name string, args []string, stdin string, line func(string)) (string, int, error) {
		scripts = append(scripts, stdin)
		target := args[len(args)-2]
		switch {
		case strings.Contains(stdin, "myco-probe"):
			root := "yes"
			if target == "plain" {
				root = "no"
			}
			for _, l := range []string{"myco-probe os Linux", "myco-probe host srv", "myco-probe root " + root, "myco-probe sudo no",
				"myco-probe curl yes", "myco-probe systemd yes", "myco-probe hub no", "myco-probe end ok"} {
				line(l)
			}
			return "", 0, nil
		case strings.Contains(stdin, "install-hub.sh"):
			line("==> installing myco hub")
			line("Add this hub to the myco app, paste this link: " + r.hubURL + "/c/" + tok)
			line(`myco-applink {"link":"` + r.hubURL + `/c/` + tok + `"}`)
			return "", 0, nil
		}
		return "unexpected", 1, nil
	}
	b := &console.Backend{Store: console.NewStore(filepath.Join(t.TempDir(), "app", "hubs.json")), Secrets: r.secrets, Timeout: 3 * time.Second,
		Local: &console.Local{Home: t.TempDir(), Remote: remote}}
	r.app = httptest.NewServer(b.Handler())
	t.Cleanup(r.app.Close)
	wait := func(id string) map[string]any {
		t.Helper()
		for i := 0; i < 100; i++ {
			_, j, _ := r.do("GET", "/app/connect/"+id, nil)
			if j["done"] == true {
				return j
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("never finished")
		return nil
	}
	_, m, _ := r.do("POST", "/app/local/hub-over-ssh", map[string]any{"target": "plain"})
	if j := wait(m["id"].(string)); j["error"].(map[string]any)["code"] != "needs_sudo" {
		t.Fatalf("no sudo: %v", j)
	}
	_, m, _ = r.do("POST", "/app/local/hub-over-ssh", map[string]any{"target": "root@srv"})
	j := wait(m["id"].(string))
	if j["error"] != nil || j["hub_id"] == nil {
		t.Fatalf("set up: %v", j)
	}
	if strings.Contains(fmt.Sprint(j["log"]), tok) {
		t.Fatalf("the App link showed in the log: %v", j["log"])
	}
	if code, _, raw := r.do("GET", "/app/hubs", nil); code != 200 || !strings.Contains(string(raw), r.hubURL) {
		t.Fatalf("the workspace was not added: %d %s", code, raw)
	}
	if s := scripts[len(scripts)-1]; !strings.Contains(s, "| bash -s -- --domain auto --yes") || strings.Contains(s, "sudo -n bash") {
		t.Fatalf("as root, no sudo: %s", s)
	}
}
