package console_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/console"
)

// A local hub from before v0.18.3 listens on 127.0.0.1 only: opening the app
// moves it onto the LAN once, upgrading the CLI first when it is too old.
// Hubs behind a tunnel, or local-only on purpose, are left alone.
func TestLocalHubMovesOntoTheLAN(t *testing.T) {
	cases := []struct {
		name     string
		service  string
		marker   bool
		oldCLI   bool
		wantCmds []string
	}{
		{"old hub, current CLI", "<string>--addr</string>\n<string>127.0.0.1:7777</string>", false, false, []string{"hub up --help", "hub up --tunnel none"}},
		{"old hub, old CLI", "--addr 127.0.0.1:7777", false, true, []string{"hub up --help", "upgrade", "hub up --help", "hub up --tunnel none"}},
		{"already on the LAN", "--addr 0.0.0.0:7777", false, false, nil},
		{"behind a tunnel", "--addr 127.0.0.1:7777 --tunnel cloudflare", false, false, nil},
		{"local only on purpose", "--addr 127.0.0.1:7777", true, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			hubDir := filepath.Join(home, ".myco", "hub")
			os.MkdirAll(hubDir, 0o700)
			os.WriteFile(filepath.Join(hubDir, "hub.env"), []byte("MYCO_HUB=http://127.0.0.1:1\n"), 0o600)
			if tc.marker {
				os.WriteFile(filepath.Join(hubDir, "local-only"), nil, 0o600)
			}
			svc := filepath.Join(home, "hub.plist")
			os.WriteFile(svc, []byte(tc.service), 0o600)
			var (
				mu       sync.Mutex
				cmds     []string
				upgraded = !tc.oldCLI
				done     = make(chan struct{}, 1)
			)
			run := func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
				mu.Lock()
				defer mu.Unlock()
				c := strings.Join(args, " ")
				switch {
				case strings.HasPrefix(c, "version"):
					return []byte(`{"version":"v0.18.2"}`), nil
				case strings.HasPrefix(c, "hub up --help"):
					cmds = append(cmds, "hub up --help")
					if upgraded {
						return []byte("--local-only  listen on 127.0.0.1 only"), nil
					}
					return []byte("--port"), nil
				case strings.HasPrefix(c, "upgrade"):
					cmds = append(cmds, "upgrade")
					upgraded = true
					return []byte("{}"), nil
				case strings.HasPrefix(c, "hub up --tunnel none"):
					cmds = append(cmds, "hub up --tunnel none")
					done <- struct{}{}
					return []byte("{}"), nil
				}
				return nil, errors.New("unexpected: " + c)
			}
			l := &console.Local{Home: home, Run: run, ServiceFile: func() string { return svc },
				LookPath: func(name string) (string, error) {
					if name == "myco" {
						return "/usr/local/bin/myco", nil
					}
					return "", errors.New("no")
				}}
			b := &console.Backend{Store: console.NewStore(newRig(t).store), Local: l}
			srv := b.Handler()
			for i := 0; i < 2; i++ { // twice: the move happens once
				srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/app/local", nil))
			}
			if tc.wantCmds != nil {
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("hub up never ran")
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
			mu.Lock()
			defer mu.Unlock()
			if strings.Join(cmds, "|") != strings.Join(tc.wantCmds, "|") {
				t.Fatalf("commands %q, want %q", cmds, tc.wantCmds)
			}
		})
	}
}
