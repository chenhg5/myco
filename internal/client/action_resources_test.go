package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

func TestBoundSSHDoesNotDowngradeOldHub(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/v1/me/action-targets" {
			http.NotFound(w, r)
			return
		}
		var old struct {
			Target struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			}
			Permissions []string
			Reason      string
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		err := d.Decode(&old)
		if err == nil || !strings.Contains(err.Error(), `unknown field "resource"`) {
			t.Errorf("request was stripped to an unbound old grant: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_argument", "message": fmt.Sprint(err)})
	}))
	defer server.Close()
	c, _ := New(server.URL, "test")
	if _, err := c.ActionTargets(context.Background()); err == nil {
		t.Fatal("old discovery not unsupported")
	} else {
		var e *Error
		if !errors.As(err, &e) || e.Code != "unsupported" {
			t.Fatal(err)
		}
	}
	_, err := c.RequestActionGrant(context.Background(), api.ActionGrantRequest{Target: api.ActionTarget{Kind: "machine", ID: "mch_1", Resource: &api.ActionResource{Provider: "server.ssh", ID: "ssh_profile", Binding: strings.Repeat("a", 64)}}, Permissions: []string{"server.ssh.read"}, Reason: "test"}, false)
	var e *Error
	if !errors.As(err, &e) || e.Code != "unsupported" {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal("bound request retried without resource")
	}
	denied := &Error{Status: 403, Code: "permission_denied", Message: "not yours"}
	if sshGrantCompatibility(denied, &api.ActionResource{}) != denied {
		t.Fatal("authorization failure hidden")
	}
}
