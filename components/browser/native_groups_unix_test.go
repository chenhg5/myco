//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNativeChromeCrashGroupRecovery(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	if err := adoptBrowserChildren(); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	secret := strings.Repeat("fixture", 8)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "--test-component")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	for _, key := range []string{"HOME", "PATH", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL"} {
		if v := os.Getenv(key); v != "" {
			cmd.Env = append(cmd.Env, key+"="+v)
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { stdin.Close(); _ = cmd.Process.Kill(); <-done; stdout.Close() })
	if err = json.NewEncoder(stdin).Encode(settings{Secret: secret, DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	var hello struct {
		Address string `json:"address"`
	}
	ready := make(chan error, 1)
	go func() { ready <- json.NewDecoder(stdout).Decode(&hello) }()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("component did not start")
	}
	invoke := func(method, path string, in, out any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(ctx, method, "http://"+hello.Address+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("X-Myco-Scope", "native/recovery")
		client := &http.Client{Transport: &http.Transport{Proxy: nil}}
		defer client.CloseIdleConnections()
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		if out != nil && json.Unmarshal(body, out) != nil {
			t.Fatal("invalid component response")
		}
	}
	var p profile
	invoke("POST", "/profiles", map[string]string{"name": "Crash recovery fixture"}, &p)
	invoke("POST", "/sessions", map[string]any{"profile": p.ID, "headless": true}, nil)
	dir := filepath.Join(dataDir, "profiles", p.ID)
	record, err := readBrowserGroup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if record.PGID == cmd.Process.Pid {
		t.Fatal("browser inherited component group")
	}
	// Crash only the component. Its browser is a separately recorded group.
	_ = cmd.Process.Kill()
	<-done
	if gone, err := groupGone(record.PGID); err != nil || gone {
		t.Fatalf("browser fixture did not survive component crash: %v %v", gone, err)
	}
	// Next startup must clean the old group before a profile can be used again.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	e, err := newEngine(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	if gone, err := groupGone(record.PGID); err != nil || !gone {
		t.Fatalf("orphan group remains: %v %v", gone, err)
	}
	if _, err = os.Stat(filepath.Join(dir, groupRecordName)); !os.IsNotExist(err) {
		t.Fatal("stale group record remains", err)
	}
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal("profile cannot be reclaimed", err)
	}
	lease.Close()
	s, err := e.openSession(ctx, "native/recovered", p.ID, true)
	if err != nil {
		t.Fatal("profile failed to reopen", err)
	}
	active, err := readBrowserGroup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.close(); err != nil {
		t.Fatal(err)
	}
	if gone, err := groupGone(active.PGID); err != nil || !gone {
		t.Fatalf("normal close left processes: %v %v", gone, err)
	}
}
