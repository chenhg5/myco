package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("MYCO_QUOTA_NATIVE_TEST"); mode != "" && len(os.Args) > 1 && os.Args[1] == "app-server" {
		if mode == "hang" {
			time.Sleep(time.Minute)
			os.Exit(0)
		}
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			var msg struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(scan.Bytes(), &msg) != nil {
				os.Exit(2)
			}
			switch msg.Method {
			case "initialize":
				fmt.Fprintln(os.Stdout, `{"id":1,"result":{}}`)
			case "initialized":
			case "account/rateLimits/read":
				if mode == "switchback" {
					path := filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")
					original, _ := os.ReadFile(path)
					_ = os.WriteFile(path, []byte(`{"tokens":{"account_id":"different-fixture","access_token":"fixture"}}`), 0600)
					_ = os.WriteFile(path, original, 0600)
					later := time.Now().Add(time.Second)
					_ = os.Chtimes(path, later, later)
				}
				fmt.Fprintln(os.Stdout, `{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":40,"windowDurationMins":10080,"resetsAt":1767225600}}}}`)
			default:
				fmt.Fprintln(os.Stdout, `{"id":2,"error":{"code":-32601}}`)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestNativeQuotaUsesOnlyAccountProtocolAndCancels(t *testing.T) {
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	src := Source{Provider: "codex", Plan: "chatgpt_subscription", Kind: "native", Status: "ok", binary: binary, dir: t.TempDir(), env: map[string]string{"MYCO_QUOTA_NATIVE_TEST": "read"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := Collect(ctx, src)
	if q.Status != "ok" || len(q.Windows) != 1 || q.Windows[0].Used != .4 || q.Windows[0].DurationSec != 604800 {
		t.Fatalf("native query failed: %s", q.Status)
	}
	src.env["MYCO_QUOTA_NATIVE_TEST"] = "hang"
	ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	q = Collect(ctx, src)
	if q.Status == "ok" || time.Since(start) > 4*time.Second {
		t.Fatal("native process outlived bounded collection")
	}
	data, _ := json.Marshal(q)
	if strings.Contains(string(data), binary) {
		t.Fatal("native binary path leaked")
	}
}

func TestNativeLoginSwitchBackCannotPoisonThePreviousAccountCache(t *testing.T) {
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".codex")
	writeFixture(t, filepath.Join(dir, "auth.json"), `{"tokens":{"account_id":"initial-fixture","access_token":"fixture"}}`)
	store, e := NewStore(filepath.Join(home, "cache"))
	if e != nil {
		t.Fatal(e)
	}
	in := Input{CLI: "codex", Binary: binary, Home: home, Env: map[string]string{"CODEX_HOME": dir, "MYCO_QUOTA_NATIVE_TEST": "switchback"}}
	before := store.Discover(in)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := store.Get(ctx, before)
	after := store.Discover(in)
	if before.Ref != after.Ref || before.SameLogin(after) {
		t.Fatal("fixture did not produce an A -> B -> A generation change")
	}
	if q.Status == "ok" || len(q.Windows) != 0 || q.ErrorCode != "binding_changed" {
		t.Fatal("native result was attributed across a login switch")
	}
	if _, e = os.Stat(store.path(before)); !os.IsNotExist(e) {
		t.Fatal("mismatched result was written to the previous account cache")
	}
	in.Env["MYCO_QUOTA_NATIVE_TEST"] = "read"
	q = store.Get(ctx, store.Discover(in))
	if q.Status != "ok" || len(q.Windows) != 1 {
		t.Fatal("stable login failed to recover")
	}
}
