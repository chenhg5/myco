package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestUsageShowCLIJSONRawAndTextGrouping(t *testing.T) {
	hub := newHub(t)
	cli := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
	code, out, stderr := cli.run("usage", "show", "--by", "model")
	if code != 0 {
		t.Fatalf("usage command: %s", stderr)
	}
	var response api.UsageList
	if err := json.Unmarshal([]byte(out), &response); err != nil || response.From == "" || response.To == "" || response.Rows == nil || response.Agents == nil {
		t.Fatalf("JSON not raw usage response %s %v", out, err)
	}
	data := &api.UsageList{Rows: []api.UsageRow{{Agent: "small", Project: "p", InputTokens: 1}, {Agent: "big", Project: "p", InputTokens: 10, CacheReadTokens: 20}, {Agent: "big", Project: "q", OutputTokens: 5}}}
	var text bytes.Buffer
	printUsage(&text, data, "agent")
	if strings.Index(text.String(), "big") > strings.Index(text.String(), "small") || !strings.Contains(text.String(), "20") {
		t.Fatal("usage table not sorted by consumption")
	}
	text.Reset()
	printUsage(&text, data, "project")
	if !strings.Contains(text.String(), "p") || strings.Contains(text.String(), "big") {
		t.Fatal("project grouping was not applied")
	}
}
