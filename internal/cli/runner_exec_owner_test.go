package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/execnode"
)

func TestExecOwnerConsoleAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, me, from string
		denied         bool
	}{
		{"owner", `{"allow_from":["lead"],"owner":"alice"}`, "@console:alice", false},
		{"other member", `{"allow_from":["lead"],"owner":"alice"}`, "@console:bob", true},
		{"administrator is not owner", `{"allow_from":["lead"],"owner":"alice"}`, "@console:admin", true},
		{"prefix is not identity", `{"allow_from":["lead"],"owner":"alice"}`, "@console:alice-extra", true},
		{"empty list", `{"allow_from":[],"owner":"alice"}`, "@console:alice", true},
		{"old hub", `{"allow_from":["lead"]}`, "@console:alice", true},
		{"empty owner", `{"allow_from":["lead"],"owner":""}`, "@console:", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/me" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.me))
			}))
			defer srv.Close()
			c, err := client.New(srv.URL, "test-token")
			if err != nil {
				t.Fatal(err)
			}
			audit := filepath.Join(t.TempDir(), "exec.jsonl")
			n, err := execnode.New(harness.Options{Exec: &harness.ExecSpec{
				Senders: []string{"lead"}, CurrentSenders: cachedExecSenders(context.Background(), c, time.Second), AuditLog: audit,
			}})
			if err != nil {
				t.Fatal(err)
			}
			out, err := n.Run(context.Background(), harness.Request{From: tc.from, Body: "echo ok", Workdir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			var res api.ExecResult
			if err := json.Unmarshal([]byte(out.Output), &res); err != nil {
				t.Fatal(err)
			}
			if res.Denied != tc.denied || !tc.denied && (res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "ok") {
				t.Fatalf("authorization: %+v", res)
			}
			b, err := os.ReadFile(audit)
			if err != nil {
				t.Fatal(err)
			}
			var entry struct {
				From string `json:"from"`
			}
			if err := json.Unmarshal(b, &entry); err != nil || entry.From != tc.from {
				t.Fatalf("audit lost sender identity: %s, %v", b, err)
			}
		})
	}
}

func TestExecOwnerConsoleRefreshAndStaticFallback(t *testing.T) {
	// Unbuffered exchanges keep response changes deterministic under -race.
	responses := make(chan string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := <-responses
		if body == "unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	current := cachedExecSenders(context.Background(), c, 0)
	n, err := execnode.New(harness.Options{Exec: &harness.ExecSpec{Senders: []string{"static-lead"}, CurrentSenders: current}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		me, from string
		denied   bool
	}{
		{`{"allow_from":["lead"],"owner":"alice"}`, "@console:alice", false},
		{`{"allow_from":["lead"],"owner":"bob"}`, "@console:alice", true},
		{`{"allow_from":["lead"],"owner":"bob"}`, "@console:bob", false},
		{`{"allow_from":["lead"]}`, "@console:bob", true},
		{`{"allow_from":["lead"],"owner":"bob"}`, "@console:bob", false},
		{"unavailable", "@console:bob", true},
		{"unavailable", "static-lead", false},
		{"unavailable", "lead", true},
	} {
		go func() { responses <- tc.me }()
		out, err := n.Run(context.Background(), harness.Request{From: tc.from, Body: "echo ok", Workdir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		var res api.ExecResult
		if err := json.Unmarshal([]byte(out.Output), &res); err != nil || res.Denied != tc.denied || !tc.denied && res.ExitCode != 0 {
			t.Fatalf("refresh/fallback from %s: %+v %v", tc.from, res, err)
		}
	}
	// This helper also feeds machine nodes; their policy must not gain the
	// exec-only exception. A missing owner must remain compatible as well.
	go func() { responses <- `{"allow_from":["lead"],"owner":"bob"}` }()
	got, err := cachedAllowFrom(context.Background(), c, time.Second)()
	if err != nil || !reflect.DeepEqual(got, []string{"lead"}) {
		t.Fatalf("machine-node senders changed: %v %v", got, err)
	}
}
