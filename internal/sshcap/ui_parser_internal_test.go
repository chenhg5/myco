package sshcap

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/testsshd"
)

// Verify the template parser against a real SSH response. The rest of the SSH
// authorization tests remain independent of Node and template dependencies.
func TestRealSSHOverviewTemplateParser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node not installed")
	}
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "templates", "node_modules", "typescript", "lib", "typescript.js")); err != nil {
		t.Skip("template dependencies not installed")
	}
	home, config := testsshd.Start(t)
	m := &Manager{Home: home, Config: config}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	p, err := m.Create(ctx, Create{Name: "Template parser fixture", Destination: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Run(ctx, p.ID, p.Binding, Operation{Action: "overview"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("isolated SSH overview exit %d", result.ExitCode)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, node, filepath.Join(root, "templates", "server-console-protocol-runner.mjs"))
	cmd.Stdin = bytes.NewReader(data)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("template parser: %v\n%s", err, output)
	}
}
