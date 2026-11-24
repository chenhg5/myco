package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

// Exercise the published schema and the production UI against real Hub data.
// The test agent token stays on stdin and in the Node process, never in the page.
func TestShoppingComparisonAgentToPage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is needed for shopping comparison browser acceptance")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	token := h.register(api.RegisterRequest{Name: "comparison-agent", Binding: api.Binding{Harness: "codex", SessionID: "comparison-test"}})
	used, err := h.client(token).UseTemplate(c, "myco/price-compare", api.TemplateUse{Name: "Shopping acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := struct {
		URL   string `json:"url"`
		Token string `json:"token"`
		Page  string `json:"page"`
	}{h.url, token, used.Page.ID}
	input, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(c, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, node, filepath.Join(root, "templates", "price-compare.integration.mjs"))
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(input)
	result, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shopping browser acceptance: %v\n%s", err, result)
	}
	if strings.HasPrefix(string(result), "SKIP ") {
		t.Skip(strings.TrimSpace(string(result)))
	}
	t.Log(string(result))
}
