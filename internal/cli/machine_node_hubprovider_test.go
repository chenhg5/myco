package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness/machinenode"
)

// L4 (i119 review): a spawn request that the hub did not send is refused
// before its grant is used: the grant is not spent for nothing.
func TestAHubProviderGrantIsUsedOnlyForARequestTheHubSent(t *testing.T) {
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/provider-material" {
			asked.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"permission_denied","message":"no"}`))
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "node-token")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, sender := range []string{"@console:mallory", "planter", ""} {
		if _, _, err := hubProvider(ctx, c, "claude", sender, "pgr_x"); err == nil {
			t.Fatalf("a request from %q was taken", sender)
		}
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("the grant was used %d times for requests the hub did not send", n)
	}
	if _, _, err := hubProvider(ctx, c, "claude", machinenode.AdminSender, "pgr_x"); err == nil {
		t.Fatal("a refused grant was taken")
	}
	if asked.Load() != 1 {
		t.Fatal("the hub's own request did not fetch")
	}
}
