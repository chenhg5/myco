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

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

// Real Hub and agent SDK, real page bridge and built application handlers in
// Chromium. The App's parent renderer is simulated; native desktop acceptance
// remains a separate check. Every export artifact must expire with its grant.
func TestPageExportApplications(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is needed for page export acceptance")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "node_modules", "typescript", "package.json")); err != nil {
		t.Skip("install template dependencies for page export acceptance")
	}
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	owner, err := h.admin().CreateConsoleToken(c, "page-render-host")
	if err != nil {
		t.Fatal(err)
	}
	token := h.register(api.RegisterRequest{Name: "page-export-agent", Binding: api.Binding{Harness: "codex", SessionID: "page-export"}})
	pages := map[string]string{}
	for _, name := range []string{"whiteboard", "design-studio", "slides"} {
		used, err := h.client(token).UseTemplate(c, "myco/"+name, api.TemplateUse{Name: "Export acceptance"})
		if err != nil {
			t.Fatal(err)
		}
		pages[name] = used.Page.ID
	}
	fixture := struct {
		URL    string            `json:"url"`
		Agent  string            `json:"agent"`
		Owner  string            `json:"owner"`
		Pages  map[string]string `json:"pages"`
		Output string            `json:"output"`
	}{h.url, token, owner.Token, pages, os.Getenv("PAGE_EXPORT_SHOTS")}
	input, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(c, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, node, filepath.Join(root, "templates", "page-actions.browser.mjs"), "--hub")
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(input) // Disposable credentials never enter argv or logs.
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("page export acceptance: %v\n%s", err, output)
	}
	if strings.HasPrefix(string(output), "SKIP ") {
		t.Skip(strings.TrimSpace(string(output)))
	}
	t.Log(string(output))
}
