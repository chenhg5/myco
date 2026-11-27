package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestAllowFromOldHubNeverReplaces(t *testing.T) {
	for _, tc := range []struct {
		code, message string
		status        int
		upgrade       bool
	}{
		{"invalid_argument", `json: unknown field "allow_add"`, 400, true},
		{"invalid_argument", `json: unknown field "allow_remove"`, 400, true},
		{"permission_denied", `only the owner may change allow_add`, 403, false},
		{"invalid_argument", `unknown field "something_else"`, 400, false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "PUT" {
					t.Error("unexpected fallback read")
				}
				var body map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&body)
				if _, ok := body["allow_from"]; ok {
					t.Error("replacement fallback")
				}
				w.WriteHeader(tc.status)
				json.NewEncoder(w).Encode(api.Error{Code: tc.code, Message: tc.message})
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "test")
			add := []string{"new"}
			_, err := c.UpdateProfile(context.Background(), "box", api.ProfileRequest{AllowAdd: &add})
			var ce *Error
			if !errors.As(err, &ce) || calls != 1 {
				t.Fatalf("%d calls, %v", calls, err)
			}
			if tc.upgrade {
				if ce.Code != "upgrade_required" || !strings.Contains(ce.Suggestion, "upgrade") {
					t.Fatal(ce)
				}
			} else if ce.Code != tc.code {
				t.Fatal(ce)
			}
		})
	}
}
