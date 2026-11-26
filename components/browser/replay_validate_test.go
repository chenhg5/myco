package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

// Both module boundaries consume these public contract fixtures independently.
func TestBrowserReplaySharedContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/agent-replay-graphs.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name  string
		Valid bool
		Graph json.RawMessage
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			err := ValidateBrowserReplayGraph(fixture.Graph)
			if (err == nil) != fixture.Valid {
				t.Fatalf("valid=%t error=%v", fixture.Valid, err)
			}
		})
	}
}

func TestReplayRequestDigestIgnoresNestedObjectOrdering(t *testing.T) {
	decode := func(body string) []byte {
		t.Helper()
		_, raw, err := controlDecode(httptest.NewRequest("POST", "/control/run", bytes.NewBufferString(body)), "run")
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	a := decode(`{"request_id":"same","session":"demo","graph":{"nodes":[{"id":"a","type":"note","title":"Report","pos":[0,1]}],"edges":[]}}`)
	b := decode(`{"graph":{"edges":[],"nodes":[{"pos":[0,1],"title":"Report","type":"note","id":"a"}]},"session":"demo","request_id":"same"}`)
	if !bytes.Equal(a, b) {
		t.Fatal("same graph produced a different request digest")
	}
}
