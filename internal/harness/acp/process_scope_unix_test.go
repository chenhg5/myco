//go:build linux || darwin

package acp

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/processwatch"
)

// Both transports must end their foreground without cancelling the retained
// process scope. The bounded child closes its inherited pipes deliberately;
// the foreground's own sleeping descendant also exercises pipe drain cleanup.
func TestTransportCleanupRetainsOwnedBackgroundChild(t *testing.T) {
	for _, transport := range []string{"stdio", "streamable-http"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			manager := processwatch.New("acp-scope-test")
			defer manager.Close()
			owner := processwatch.Owner{Workspace: "test", Member: "owner", Agent: "fixture"}
			ctx = processwatch.WithOwner(ctx, manager, owner)
			procCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
			defer stop()
			endpoint := ""
			if transport == "streamable-http" {
				srv := httptest.NewServer(newFakeHTTP())
				defer srv.Close()
				endpoint = srv.URL
			}
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			a := &agent{spec: Spec{Binary: "/bin/sh", Transport: transport, Args: []string{"-c", `sleep 15 >/dev/null 2>&1 & echo $! > "$1"; if [ -n "$2" ]; then echo "HTTP server ready at $2" >&2; fi; sleep 15`, "fixture", pidFile, endpoint}}}
			_, _, cleanup, err := a.startConnection(ctx, procCtx, stop, harness.Request{}, nil)
			if err != nil {
				if strings.Contains(err.Error(), "cannot establish child process creation identity") {
					t.Skip("OS process identity is unavailable in this sandbox")
				}
				t.Fatal(err)
			}
			// The stdio process starts asynchronously; bound only fixture preparation.
			var data []byte
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				data, _ = os.ReadFile(pidFile)
				if len(data) > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				cleanup()
				t.Fatal("fixture child did not start")
			}
			running := func() bool {
				b, e := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
				state := strings.TrimSpace(string(b))
				return e == nil && state != "" && !strings.HasPrefix(state, "Z")
			}
			cleanup()
			stop()
			if !running() {
				t.Fatal("normal transport cleanup killed background work")
			}
			manager.StopAgent("test", "fixture")
			if running() {
				t.Fatal("explicit session cleanup left an owned child running")
			}
		})
	}
}
