package browserautomation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileRecoveryErrorPreservesSafeReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`{"error":"profile_locked","reason":"leftover_unverified"}`))
	}))
	defer server.Close()
	c := &Component{address: strings.TrimPrefix(server.URL, "http://"), secret: "private-fixture"}
	err := c.Call(context.Background(), "fixture", "POST", "/sessions", nil, nil)
	var pe *ProfileError
	if !errors.As(err, &pe) || pe.Reason != "leftover_unverified" || err.Error() != "profile_locked" {
		t.Fatal(err)
	}
}
