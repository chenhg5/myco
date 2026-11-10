package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
)

// createLink creates a join link (admin) and returns its token, enrollment
// id and join URL.
func (h *hub) createLink(extra map[string]any) (tok, id, joinURL string) {
	h.t.Helper()
	body := map[string]any{"kind": "link"}
	for k, v := range extra {
		body[k] = v
	}
	r := h.do("POST", "/v1/enrollments", adminToken, body).want(h.t, http.StatusCreated)
	tok, id, joinURL = str(r.body, "token"), str(r.body, "enrollment", "id"), str(r.body, "join_url")
	if !strings.HasPrefix(tok, app.EnrollTokenPrefix) || id == "" || str(r.body, "enrollment", "kind") != "link" {
		h.t.Fatalf("create link: %s", r.raw)
	}
	return tok, id, joinURL
}

func TestCreateLinkReturnsJoinURL(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	tok, _, joinURL := h.createLink(nil)
	// No --public-url: the URL is derived from the request.
	if joinURL != h.url+"/j/"+tok {
		t.Fatalf("join_url %q, want %q", joinURL, h.url+"/j/"+tok)
	}
	// Behind a TLS proxy the scheme follows X-Forwarded-Proto.
	r := h.doReq("POST", "/v1/enrollments", adminToken, map[string]any{"kind": "link"}, map[string]string{"X-Forwarded-Proto": "https"}).want(t, 201)
	if u := str(r.body, "join_url"); !strings.HasPrefix(u, "https://") {
		t.Fatalf("join_url behind TLS proxy: %q", u)
	}
	// A plain enrollment has no join URL.
	r = h.do("POST", "/v1/enrollments", adminToken, map[string]any{"name_prefix": "ci-"}).want(t, 201)
	if _, ok := r.body["join_url"]; ok {
		t.Fatalf("plain enrollment has join_url: %s", r.raw)
	}
	// Machine enrollments cannot be created directly.
	h.do("POST", "/v1/enrollments", adminToken, map[string]any{"kind": "machine", "name_prefix": "m-"}).wantErr(t, 400, "invalid_argument")
}

func TestJoinPageValidLink(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	tok, _, _ := h.createLink(map[string]any{"max_uses": 3})
	path := "/j/" + tok

	tests := []struct {
		name     string
		query    string
		hdr      map[string]string
		wantType string
		contains []string
	}{
		{"curl user agent gets the script", "", map[string]string{"User-Agent": "curl/8.5.0"}, "text/x-shellscript",
			[]string{"#!/usr/bin/env bash", "set -euo pipefail", "export MYCO_JOIN_TOKEN='" + tok + "'", "export MYCO_HUB='" + h.url + "'", "machine join"}},
		{"wget user agent gets the script", "", map[string]string{"User-Agent": "Wget/1.21"}, "text/x-shellscript", []string{"MYCO_JOIN_TOKEN="}},
		{"format=sh", "?format=sh", nil, "text/x-shellscript", []string{"MYCO_JOIN_TOKEN="}},
		{"format=md", "?format=md", nil, "text/markdown", []string{"# Join the myco network at " + h.url, "curl -fsSL " + h.url + path + " | bash", "myco join"}},
		{"Accept text/markdown", "", map[string]string{"Accept": "text/markdown"}, "text/markdown", []string{"# Join the myco network"}},
		{"browser gets HTML", "", map[string]string{"Accept": "text/html,application/xhtml+xml", "User-Agent": "Mozilla/5.0"}, "text/html",
			[]string{"<!doctype html>", "还可接入 3 台机器", "curl -fsSL " + h.url + path + " | bash"}},
		{"default is markdown", "", map[string]string{"User-Agent": "python-requests/2"}, "text/markdown", []string{"# Join the myco network"}},
		{"PowerShell gets the PowerShell script", "", map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Microsoft Windows 10.0.26100; zh-CN) PowerShell/7.5.0"}, "text/plain",
			[]string{"$env:MYCO_JOIN_TOKEN = '" + tok + "'", "$env:MYCO_HUB = '" + h.url + "'", "install-client.ps1", "machine join", "Remove-Item Env:MYCO_JOIN_TOKEN"}},
		{"Windows PowerShell 5.1", "", map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT; Windows NT 10.0; zh-CN) WindowsPowerShell/5.1.26100"}, "text/plain", []string{"install-client.ps1"}},
		{"format=ps1", "?format=ps1", nil, "text/plain", []string{"install-client.ps1"}},
		{"guide has the Windows command", "?format=md", nil, "text/markdown", []string{"irm '" + h.url + path + "?format=ps1' | iex"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h.t = t
			r := h.doReq("GET", path+tc.query, "", nil, tc.hdr).want(t, 200)
			if tc.wantType == "text/plain" {
				for i, c := range string(r.raw) {
					if c > 127 {
						t.Fatalf("PowerShell script has non-ASCII %q at %d (Windows PowerShell 5.1 may misread it)", c, i)
					}
				}
			}
			if ct := r.header.Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
				t.Errorf("Content-Type %q, want %s", ct, tc.wantType)
			}
			body := string(r.raw)
			for _, s := range tc.contains {
				if !strings.Contains(body, s) {
					t.Errorf("body lacks %q:\n%s", s, body)
				}
			}
			if strings.Contains(body, adminToken) {
				t.Fatal("join page leaks the admin token")
			}
			// The link is a credential: never cached, never sent as referrer.
			if r.header.Get("Cache-Control") != "no-store" || r.header.Get("Referrer-Policy") != "no-referrer" || r.header.Get("X-Robots-Tag") != "noindex" {
				t.Errorf("credential headers: %v", r.header)
			}
		})
	}
	h.t = t
	// Viewing the page does not use up the link.
	r := h.do("GET", "/v1/enrollments", adminToken, nil).want(t, 200)
	for _, e := range r.body["enrollments"].([]any) {
		if e.(map[string]any)["uses"] != float64(0) {
			t.Fatalf("page view consumed a use: %s", r.raw)
		}
	}
}

func TestJoinPageUnusableLinks(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	revokedTok, revokedID, _ := h.createLink(nil)
	h.do("DELETE", "/v1/enrollments/"+revokedID, adminToken, nil).want(t, 200)
	usedTok, _, _ := h.createLink(map[string]any{"max_uses": 1})
	h.do("POST", "/v1/join", usedTok, map[string]any{"machine": "box"}).want(t, 201)
	r := h.do("POST", "/v1/enrollments", adminToken, map[string]any{"name_prefix": "ci-"}).want(t, 201)
	plainTok := str(r.body, "token")

	cases := []struct {
		name, tok, wantMsg string
	}{
		{"unknown token", "myco_enroll_doesnotexist", "不存在"},
		{"not even an enrollment token", "garbage", "不存在"},
		{"revoked link", revokedTok, "不可用"},
		{"used-up link", usedTok, "不可用"},
		{"plain enrollment token is not a link", plainTok, "不可用"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.t = t
			// Markdown and HTML: 404 with an explanation.
			for _, f := range []string{"md", "html"} {
				r := h.do("GET", "/j/"+tc.tok+"?format="+f, "", nil).want(t, 404)
				if !strings.Contains(string(r.raw), tc.wantMsg) {
					t.Errorf("%s: body lacks %q: %s", f, tc.wantMsg, r.raw)
				}
				if strings.Contains(string(r.raw), "curl -fsSL") {
					t.Errorf("%s: unusable link still shows the install command", f)
				}
			}
			// Script: served with 200 so `curl -fsSL ... | bash` prints the
			// reason, but it only reports and exits 1; it never exports the
			// token or runs the installer.
			r := h.doReq("GET", "/j/"+tc.tok, "", nil, map[string]string{"User-Agent": "curl/8"}).want(t, 200)
			body := string(r.raw)
			if !strings.Contains(body, "exit 1") || !strings.Contains(body, tc.wantMsg) {
				t.Errorf("script does not fail with the reason:\n%s", body)
			}
			if strings.Contains(body, "MYCO_JOIN_TOKEN=") || strings.Contains(body, "install-client.sh") || strings.Contains(body, adminToken) {
				t.Errorf("unusable-link script still installs or leaks:\n%s", body)
			}
		})
	}
}

func TestJoinPageEscapesUntrustedToken(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	evil := "%3Cscript%3Ealert(1)%3C%2Fscript%3E"
	r := h.doReq("GET", "/j/"+evil, "", nil, map[string]string{"Accept": "text/html"}).want(t, 404)
	if strings.Contains(string(r.raw), "<script>alert(1)") {
		t.Fatalf("token reflected unescaped in HTML:\n%s", r.raw)
	}
	if csp := r.header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP: %q", csp)
	}
}

func TestJoinWithLink(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	tok, id, _ := h.createLink(map[string]any{"labels": []string{"team=x"}, "max_uses": 1})

	h.do("POST", "/v1/join", "", map[string]any{"machine": "box"}).wantErr(t, 401, "unauthenticated")
	h.do("POST", "/v1/join", "myco_enroll_nope", map[string]any{"machine": "box"}).wantErr(t, 401, "unauthenticated")
	h.do("POST", "/v1/join", tok, map[string]any{"machine": "!!!"}).wantErr(t, 400, "invalid_argument")

	// Dry run creates nothing.
	r := h.do("POST", "/v1/join?dry_run=true", tok, map[string]any{"machine": "Box.local"}).want(t, 200)
	if r.body["dry_run"] != true || str(r.body, "token") != "" {
		t.Fatalf("dry run: %s", r.raw)
	}

	r = h.do("POST", "/v1/join", tok, map[string]any{"machine": "Box.local"}).want(t, 201)
	first := str(r.body, "token")
	if r.body["machine"] != "box" || r.body["created"] != true || r.body["name_prefix"] != "box-" || !strings.HasPrefix(first, app.EnrollTokenPrefix) {
		t.Fatalf("join: %s", r.raw)
	}
	labels, _ := r.body["labels"].([]any)
	if !(len(labels) == 2 && labels[0] == "machine=box" && labels[1] == "team=x") {
		t.Fatalf("join labels: %v", labels)
	}

	// The same machine again: 200, new token, link not used up (max_uses=1).
	r = h.do("POST", "/v1/join", tok, map[string]any{"machine": "box"}).want(t, 200)
	if r.body["created"] != false || str(r.body, "token") == "" || str(r.body, "token") == first {
		t.Fatalf("re-join: %s", r.raw)
	}
	// Another machine: the single use is gone.
	h.do("POST", "/v1/join", tok, map[string]any{"machine": "other"}).wantErr(t, 403, "permission_denied")

	// The machine token enrolls sessions under its prefix.
	machineTok := str(r.body, "token")
	r = h.do("POST", "/v1/enroll", machineTok, map[string]any{"binding": map[string]any{"harness": "claude", "session_id": "sess-abc12345"}}).want(t, 201)
	if name := str(r.body, "agent", "name"); !strings.HasPrefix(name, "box-") {
		t.Fatalf("enrolled agent name %q", name)
	}
	// A link itself cannot enroll a session.
	h.do("POST", "/v1/enroll", tok, map[string]any{"binding": map[string]any{"harness": "claude", "session_id": "s2"}}).wantErr(t, 400, "invalid_argument")

	// Revoked link: joining is refused.
	h.do("DELETE", "/v1/enrollments/"+id, adminToken, nil).want(t, 200)
	h.do("POST", "/v1/join", tok, map[string]any{"machine": "box"}).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/enrollments/enr_nope", adminToken, nil).wantErr(t, 404, "not_found")
}
