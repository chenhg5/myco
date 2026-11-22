package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// Cross-language contract check: the actual TypeScript SDK talks to the real
// HTTP adapter and SQLite-backed Hub, with a simulated App provider. Native
// capture/input still requires separate component and device acceptance.
func TestAgentActionsSDK(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is needed for the optional SDK integration test")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "node_modules", "typescript", "package.json")); err != nil {
		t.Skip("install template dependencies to run the SDK integration test")
	}
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	owner, err := h.admin().CreateConsoleToken(c, "sdk-test-host")
	if err != nil {
		t.Fatal(err)
	}
	machine := h.client(h.register(api.RegisterRequest{Name: "sdk-machine", Binding: api.Binding{Harness: "machine"}}))
	if err := machine.ReportMachine(c, api.MachineReport{Fingerprint: "sdk-fixture-0123456789abcdef", Name: "sdk-studio", Hostname: "sdk-studio", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	fixture := struct {
		URL   string `json:"url"`
		Agent string `json:"agent"`
		Other string `json:"other"`
		Owner string `json:"owner"`
	}{
		URL:   h.url,
		Agent: h.register(api.RegisterRequest{Name: "sdk-agent", Binding: api.Binding{Harness: "codex", SessionID: "sdk-1"}}),
		Other: h.register(api.RegisterRequest{Name: "sdk-other", Binding: api.Binding{Harness: "codex", SessionID: "sdk-2"}}),
		Owner: owner.Token,
	}
	input, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, node, filepath.Join(root, "templates", "actions.integration.mjs"))
	cmd.Dir = root
	// Temporary test credentials stay in memory/stdin, not command arguments,
	// environment, disk fixtures or test output.
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("SDK integration: %v\n%s", err, output)
	}
	t.Log(string(output))
}
