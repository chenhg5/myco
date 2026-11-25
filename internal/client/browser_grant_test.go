package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestScopedBrowserGrantFailsClosedWithOldHub(t *testing.T) {
	var writes atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dry_run") == "" {
			writes.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"grant":{"target":{"kind":"machine","id":"machine"},"permissions":["browser.control"]}}`)
	}))
	defer s.Close()
	c, err := New(s.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.RequestActionGrant(context.Background(), api.ActionGrantRequest{
		Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: &api.ActionResource{
			Provider: "browser", ID: "p_personal", Binding: "binding",
		}},
		Permissions: []string{api.BrowserControlPermission},
		Reason:      "Read documentation", Hosts: []string{"example.com"},
	}, false)
	if ce, ok := AsError(err); !ok || ce.Code != "unsupported" || writes.Load() != 0 {
		t.Fatalf("an old Hub accepted or was sent an unscoped request: %v, %d writes", err, writes.Load())
	}
}
