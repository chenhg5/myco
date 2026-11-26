package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCleanOutputParsesJSONStrings(t *testing.T) {
	var items []map[string]any
	for i := 0; i < 30; i++ {
		items = append(items, map[string]any{"rank": i + 1, "title": strings.Repeat("片", 40), "url": fmt.Sprintf("https://example.com/%d", i)})
	}
	raw, _ := json.Marshal(items)
	if len([]rune(string(raw))) <= 2000 {
		t.Fatal("fixture too short")
	}
	r := &playRun{secrets: []string{"tok123"}}
	got, ok := r.cleanOutput(string(raw), 0).([]any)
	if !ok || len(got) != 30 || r.outputCut {
		t.Fatalf("a JSON string was not kept as its items: %T %d cut=%v", got, len(got), r.outputCut)
	}
	if got[29].(map[string]any)["url"] != "https://example.com/29" {
		t.Fatalf("last item: %v", got[29])
	}

	r = &playRun{secrets: []string{"tok123"}}
	text := r.cleanOutput("[not json tok123 "+strings.Repeat("x", 2100), 0).(string)
	if !r.outputCut || len([]rune(text)) != 2000 || strings.Contains(text, "tok123") {
		t.Fatalf("long text: cut=%v len=%d", r.outputCut, len([]rune(text)))
	}

	r = &playRun{secrets: []string{"tok123"}}
	if v := r.cleanOutput(`{"key":"tok123"}`, 0).(map[string]any); v["key"] != "[redacted]" || r.outputCut {
		t.Fatalf("secrets inside parsed JSON: %v", v)
	}
	r = &playRun{}
	list := make([]any, 250)
	if v := r.cleanOutput(list, 0).([]any); len(v) != 200 || !r.outputCut {
		t.Fatal("a long list is not marked cut")
	}
}
