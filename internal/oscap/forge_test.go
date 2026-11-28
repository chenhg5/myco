package oscap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type forgeTestSecrets map[string]string

func (s forgeTestSecrets) Get(k string) (string, error) { return s[k], nil }
func (s forgeTestSecrets) Set(k, v string) error        { s[k] = v; return nil }
func (s forgeTestSecrets) Delete(k string) error        { delete(s, k); return nil }

type forgeTestTransport func(*http.Request) (*http.Response, error)

func (f forgeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func forgeTestCall(t *testing.T, e *Env, name string, a any) (any, *Error) {
	t.Helper()
	raw, _ := json.Marshal(a)
	return Call(context.Background(), e, name, raw)
}
func TestForgeReadOnlyAccountsAndQueries(t *testing.T) {
	secret := "private-fixture-credential"
	e := &Env{Home: t.TempDir(), Scope: "workspace\x00viewer\x00page", Secrets: forgeTestSecrets{}}
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	paths := []string{}
	forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatalf("unexpected mutation %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer "+secret || r.URL.Host != "api.github.com" {
			t.Fatalf("credential destination mismatch: %s", r.URL.Host)
		}
		paths = append(paths, r.URL.EscapedPath())
		body := `{"login":"demo"}`
		switch r.URL.Path {
		case "/search/issues":
			if !strings.Contains(r.URL.Query().Get("q"), "author:demo") {
				t.Error("missing identity filter")
			}
			body = `{"items":[{"number":7,"title":"Read me","body":"Details","repository_url":"https://api.github.com/repos/acme/widget","html_url":"https://github.com/acme/widget/pull/7","state":"open","user":{"login":"demo"},"labels":[{"name":"bug"}]}]}`
		case "/repos/acme/widget/pulls/7/files":
			body = `[{"filename":"main.go","status":"modified","patch":"+fixed"}]`
		case "/repos/acme/widget/issues/7/comments":
			body = `[{"user":{"login":"reviewer"},"body":"Looks good"}]`
		case "/user/repos":
			body = `[{"id":3,"full_name":"acme/widget","private":true}]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Link": {`<https://evil.example/next>; rel="next"`}}}, nil
	})
	v, err := forgeTestCall(t, e, "forge.accounts", forgeConnect{Action: "connect", Provider: "github", Source: "token", Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	p := v.(forgeAccount)
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), secret) {
		t.Fatal("credential exposed")
	}
	list, err := forgeTestCall(t, e, "forge.query", forgeRequest{Account: p.ID, Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	items := list.(map[string]any)["items"].([]forgeItem)
	if len(items) != 1 || items[0].Repo != "acme/widget" {
		t.Fatalf("bad normalized list: %#v", items)
	}
	for _, action := range []string{"comments", "files", "detail", "repos"} {
		if _, err = forgeTestCall(t, e, "forge.query", forgeRequest{Account: p.ID, Action: action, Repo: "acme/widget", Number: 7}); err != nil {
			t.Fatal(action, err)
		}
	}
	other := *e
	other.Scope = "workspace\x00other-viewer\x00page"
	if _, err = forgeTestCall(t, &other, "forge.query", forgeRequest{Account: p.ID, Action: "repos"}); err == nil || err.Code != CodeNotFound {
		t.Fatal("account crossed viewer boundary", err)
	}
	for _, in := range []forgeRequest{{Account: p.ID, Action: "delete"}, {Account: p.ID, Action: "detail", Repo: "acme/../admin", Number: 1}, {Account: p.ID, Action: "files", Repo: "acme/widget", Number: 1, Kind: "issue"}} {
		if _, err = forgeTestCall(t, e, "forge.query", in); err == nil {
			t.Fatal("unexpected request allowed")
		}
	}
	// Reconnect updates one account, then disconnect deletes its credential.
	v, err = forgeTestCall(t, e, "forge.accounts", forgeConnect{Action: "connect", Provider: "github", Source: "token", Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	p = v.(forgeAccount)
	v, err = forgeTestCall(t, e, "forge.accounts", forgeConnect{Action: "list"})
	if err != nil || len(v.([]forgeAccount)) != 1 {
		t.Fatal("reconnect duplicated account")
	}
	if _, err = forgeTestCall(t, e, "forge.accounts", forgeConnect{Action: "remove", ID: p.ID}); err != nil {
		t.Fatal(err)
	}
	if len(e.Secrets.(forgeTestSecrets)) != 0 {
		t.Fatal("credential retained")
	}
	if len(paths) < 6 {
		t.Fatal("not all routes read")
	}
}
func TestForgeGitLabNestedProjectsAndSafeFailures(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	p := forgeAccount{Provider: "gitlab", Host: "https://gitlab.example.com", Login: "demo"}
	token := "private-fixture-credential"
	forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fgroup%2Fwidget/merge_requests/12/diffs" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("wrong GitLab request %s", r.URL.EscapedPath())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"new_path":"main.go","diff":"+hello","new_file":true}]`)), Header: http.Header{}}, nil
	})
	v, err := forgeDetail(context.Background(), p, token, forgeRequest{Action: "files", Kind: "pull", Repo: "acme/group/widget", Number: 12, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["items"].([]forgeFileDiff)[0].Status != "added" {
		t.Fatal(v)
	}
	for _, status := range []int{301, 401, 403, 429, 500} {
		forgeTransport = forgeTestTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(token))}, nil
		})
		var out any
		if _, err = forgeGet(context.Background(), p, token, "/user", nil, &out); err == nil || strings.Contains(err.Error(), token) {
			t.Fatal("unsafe error", err)
		}
	}
	forgeTransport = forgeTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"username":"` + token + `"}`))}, nil
	})
	var out any
	if _, err = forgeGet(context.Background(), p, token, "/user", nil, &out); err == nil {
		t.Fatal("credential echo allowed")
	}
	for _, host := range []string{"http://gitlab.example.com", "https://user:secret@gitlab.example.com", "https://gitlab.example.com/api", "https://gitlab.example.com?token=x"} {
		if _, err = forgeHost("gitlab", host); err == nil {
			t.Fatal("unsafe origin", host)
		}
	}
}

func TestForgeGitLabCLIOutput(t *testing.T) {
	for _, text := range []string{"✓ Token found: glpat-fixture-token", "  ✓ Token: glpat-fixture-token\n"} {
		if forgeGitLabToken(text) != "glpat-fixture-token" {
			t.Fatal("could not parse known CLI output")
		}
	}
	for _, text := range []string{"Token: ********", "no credential", "Token: first\nToken: second"} {
		if forgeGitLabToken(text) != "" {
			t.Fatal("ambiguous credential accepted")
		}
	}
}

func TestForgeSecretOriginAndPrivateNetworkBinding(t *testing.T) {
	e := &Env{Home: t.TempDir(), Scope: "workspace\x00viewer\x00page", Secrets: forgeTestSecrets{}}
	key, _ := forgeKey(e)
	p := forgeAccount{ID: "a_fixture", Provider: "gitlab", Host: "https://gitlab.example.com", Login: "demo"}
	if err := forgeSave(e, key, []forgeAccount{p}); err != nil {
		t.Fatal(err)
	}
	_ = e.Secrets.Set(key+"-"+p.ID, forgeSecretValue(p, "private-fixture-token"))
	if _, _, err := forgeCredential(e, p.ID); err != nil {
		t.Fatal(err)
	}
	p.Host = "https://other.example.com"
	_ = forgeSave(e, key, []forgeAccount{p})
	if _, _, err := forgeCredential(e, p.ID); err == nil {
		t.Fatal("stored origin changed without reconnect")
	}
	old := forgeLookup
	t.Cleanup(func() { forgeLookup = old })
	forgeLookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	origin, private, err := ForgePreview(context.Background(), "gitlab", "https://gitlab.example.com")
	if err != nil || !private || origin != "https://gitlab.example.com" {
		t.Fatal("missing private preview", err)
	}
	if _, err = forgePinnedTransport(context.Background(), forgeAccount{Provider: "gitlab", Host: origin}); err == nil {
		t.Fatal("private rebinding accepted without confirmation")
	}
	if _, err = forgePinnedTransport(context.Background(), forgeAccount{Provider: "gitlab", Host: origin, AllowPrivate: true}); err != nil {
		t.Fatal("confirmed private server rejected", err)
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1"} {
		if !forgePrivate(netip.MustParseAddr(address)) {
			t.Fatal("private address not classified", address)
		}
	}
	if forgePrivate(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public address classified private")
	}
}

func TestForgeRepositoryPathsAreFixed(t *testing.T) {
	for _, provider := range []string{"github", "gitea", "gitlab"} {
		for _, repo := range []string{"../user/emails", "../../user/emails", "acme/.", "acme/..", "./widget", "acme/./widget", "acme/../widget"} {
			if forgeValidRepo(provider, repo) {
				t.Fatal(provider, "accepted traversal", repo)
			}
		}
	}
	for _, provider := range []string{"github", "gitea"} {
		if forgeValidRepo(provider, "acme/group/widget") {
			t.Fatal("non-GitLab accepted nested repository", provider)
		}
	}
	if !forgeValidRepo("gitlab", "acme/group/widget") || !forgeValidRepo("github", "acme/a..b") {
		t.Fatal("legitimate repository rejected")
	}
}

func TestForgePreservesAppProxyAndCanonicalHTTPS(t *testing.T) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	used := false
	base.Proxy = func(*http.Request) (*url.URL, error) { used = true; return url.Parse("http://proxy.example.com:8080") }
	http.DefaultTransport = base
	transport := forgeNetworkTransport().(*http.Transport)
	defer transport.CloseIdleConnections()
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	proxy, err := transport.Proxy(req)
	if err != nil || !used || proxy.Host != "proxy.example.com:8080" {
		t.Fatal("App proxy ignored")
	}
	host, err := forgeHost("github", "https://GitHub.com:443/")
	if err != nil || host != "https://github.com" || forgeBase(forgeAccount{Provider: "github", Host: host}) != "https://api.github.com" {
		t.Fatal("default HTTPS port misclassified as enterprise", host, err)
	}
}
func TestForgeGiteaRepositoryFilter(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	p := forgeAccount{Provider: "gitea", Host: "https://git.example.com", Login: "demo"}
	forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/repos/acme/widget/issues" || r.URL.Query().Get("created_by") != "demo" || r.Header.Get("Authorization") != "token fixture-secret" {
			t.Fatalf("invalid Gitea request path or scope %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"number":12,"title":"Gitea issue","state":"open","user":{"login":"demo"},"html_url":"https://git.example.com/acme/widget/issues/12"}]`))}, nil
	})
	result, err := forgeItems(context.Background(), p, "fixture-secret", forgeRequest{Kind: "issue", Repo: "acme/widget", Scope: "created", State: "open", Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows := result.(map[string]any)["items"].([]forgeItem)
	if len(rows) != 1 || rows[0].Repo != "acme/widget" || rows[0].Title != "Gitea issue" {
		t.Fatal("Gitea response not normalized", rows)
	}
}
