package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// The built whiteboard runs in Chromium, while the bridge's data port talks to
// a real Hub with an ordinary agent token. This checks published collection
// schemas, CAS versions, freshness and the documented CLI extraction recipe.
func TestWhiteboardSharedExport(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is needed for whiteboard browser acceptance")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "node_modules", "typescript", "package.json")); err != nil {
		t.Skip("install template dependencies for whiteboard browser acceptance")
	}
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	token := h.register(api.RegisterRequest{Name: "whiteboard-export-agent", Binding: api.Binding{Harness: "codex", SessionID: "whiteboard-export"}})
	used, err := h.client(token).UseTemplate(c, "myco/whiteboard", api.TemplateUse{Name: "Agent export acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("WHITEBOARD_EXPORT_SHOTS")
	if output == "" {
		output = t.TempDir()
	}
	fixture := struct {
		URL    string `json:"url"`
		Token  string `json:"token"`
		Page   string `json:"page"`
		Output string `json:"output"`
	}{h.url, token, used.Page.ID, output}
	input, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(c, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, node, filepath.Join(root, "templates", "whiteboard-export.integration.mjs"))
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(input) // Test credentials never enter argv, env or logs.
	result, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("whiteboard browser acceptance: %v\n%s", err, result)
	}
	if strings.HasPrefix(string(result), "SKIP ") {
		t.Skip(strings.TrimSpace(string(result)))
	}
	t.Log(string(result))
}
