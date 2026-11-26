package browserautomation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestClipRunPreflightRejectsLegacyBeforeAnyPost(t *testing.T) {
	for _, test := range []struct {
		name, features, statusError string
		status, wantPosts           int
		wantError                   string
	}{
		{name: "legacy", status: 200, wantError: "unsupported_node"},
		{name: "current", status: 200, features: `["screenshot_clip"]`, wantPosts: 1},
		{name: "unrelated", status: 200, features: `["another_feature"]`, wantError: "unsupported_node"},
		{name: "auth", status: 403, statusError: "permission_denied", wantError: "permission_denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture" || r.Header.Get("X-Myco-Scope") != "scope" {
					t.Error("component capability check lost authentication/scope")
				}
				if r.URL.Path == "/status" {
					w.WriteHeader(test.status)
					value := map[string]any{}
					if test.features != "" {
						var features []string
						json.Unmarshal([]byte(test.features), &features)
						value["features"] = features
					}
					if test.statusError != "" {
						value["error"] = test.statusError
					}
					json.NewEncoder(w).Encode(value)
					return
				}
				if r.Method == "POST" && r.URL.Path == "/runs" {
					posts++
					w.Write([]byte(`{"id":"fixture"}`))
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			component := &Component{address: strings.TrimPrefix(server.URL, "http://"), secret: "fixture"}
			var response any
			err := component.Call(context.Background(), "scope", "POST", "/runs", json.RawMessage(`{"script":{"graph":{"nodes":[{"type":"screenshot","clip":{"x":1,"y":2,"width":3,"height":4}}]}}}`), &response)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != test.wantError || posts != test.wantPosts {
				t.Fatalf("error=%q posts=%d", got, posts)
			}
		})
	}
}
func TestClipDetectionIncludesChildrenAndMalformedFields(t *testing.T) {
	for _, body := range []string{
		`{"script":{"graph":{"nodes":[{"type":"screenshot","clip":null}]}}}`,
		`{"scripts":{"child":{"graph":{"nodes":[{"type":"screenshot","clip":{}}]}}}}`,
	} {
		if !slices.Contains(runRequiredFeatures([]byte(body)), "screenshot_clip") {
			t.Fatal("clip field ignored", body)
		}
	}
	for _, body := range []string{`{}`, `{"script":{"graph":{"nodes":[{"type":"screenshot","full":true}]}}}`} {
		if slices.Contains(runRequiredFeatures([]byte(body)), "screenshot_clip") {
			t.Fatal("ordinary script requires new feature", body)
		}
	}
}

func TestBrowserDraftRequiresPlaybackSupport(t *testing.T) {
	for _, node := range []string{`{"type":"back"}`, `{"type":"forward"}`, `{"type":"reload"}`, `{"type":"tab","tab":"close"}`, `{"type":"screenshot","full":true}`} {
		body := []byte(`{"scripts":{"child":{"graph":{"nodes":[` + node + `]}}}}`)
		if got := runRequiredFeatures(body); !slices.Equal(got, []string{"agent_control"}) {
			t.Fatalf("%s: %v", node, got)
		}
	}
}

func TestBrowserControlErrorPreservesStableCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"page text must stay private","code":"session_closed","reason":"stopped_by_owner"}`))
	}))
	defer server.Close()
	component := &Component{address: strings.TrimPrefix(server.URL, "http://"), secret: "fixture"}
	err := component.Call(context.Background(), "scope", http.MethodPost, "/control/act", nil, nil)
	var native *ComponentError
	if !errors.As(err, &native) || native.Code != "session_closed" || native.Reason != "stopped_by_owner" || err.Error() != "session_closed" {
		t.Fatalf("unexpected native error: %v", err)
	}
}

func TestReplayPreflightRequiresBothFeaturesAndKeepsNode(t *testing.T) {
	for _, features := range []string{`[]`, `["agent_control"]`, `["agent_replay"]`, `["agent_control","agent_replay"]`} {
		t.Run(features, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture" || r.Header.Get("X-Myco-Scope") != "scope" {
					t.Error("lost native scope")
				}
				if r.URL.Path == "/status" {
					fmt.Fprintf(w, `{"protocol":1,"features":%s}`, features)
					return
				}
				posts++
				w.WriteHeader(400)
				w.Write([]byte(`{"code":"invalid_argument","error_node":"blocked","error":"private input"}`))
			}))
			defer server.Close()
			c := &Component{address: strings.TrimPrefix(server.URL, "http://"), secret: "fixture"}
			err := c.Call(context.Background(), "scope", "POST", "/control/run", json.RawMessage(`{"session":"demo","graph":{"nodes":[],"edges":[]}}`), nil)
			if features == `["agent_control","agent_replay"]` {
				var failure *ComponentError
				if posts != 1 || !errors.As(err, &failure) || failure.Code != "invalid_argument" || failure.ErrorNode != "blocked" || failure.Error() != "invalid_argument" {
					t.Fatal(posts, err)
				}
			} else if posts != 0 || err == nil || err.Error() != "unsupported_node" {
				t.Fatal(posts, err)
			}
		})
	}
}
