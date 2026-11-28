package oscap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Test-only transports replace this with a local fixture. Production always
// verifies TLS, follows the App's network proxy and refuses all redirects.
var forgeTransport http.RoundTripper // nil in production; replaced only by tests

func forgeNetworkTransport() http.RoundTripper {
	// The App installs its dynamic proxy selector on the default transport.
	// Preserve it, including explicit direct mode and manual authenticated proxies.
	return http.DefaultTransport.(*http.Transport).Clone()
}
func forgeBase(p forgeAccount) string {
	if p.Provider == "github" {
		if p.Host == "https://github.com" {
			return "https://api.github.com"
		}
		return p.Host + "/api/v3"
	}
	if p.Provider == "gitlab" {
		return p.Host + "/api/v4"
	}
	return p.Host + "/api/v1"
}
func forgeGet(ctx context.Context, p forgeAccount, token, path string, q url.Values, out any) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := forgeBase(p) + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, errf(CodeInvalid, "invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if p.Provider == "gitea" {
		req.Header.Set("Authorization", "token "+token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "myco-code-host")
	if p.Provider == "github" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	transport := forgeTransport
	if transport == nil {
		transport, err = forgePinnedTransport(ctx, p)
		if err != nil {
			return false, err
		}
		defer transport.(*http.Transport).CloseIdleConnections()
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, AsError(ctx.Err())
		}
		return false, errf("network_error", "could not reach the code host; check the server address and network")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return false, errf(CodeNoCredentials, "the account is no longer signed in; reconnect it")
	}
	if res.StatusCode == 403 || res.StatusCode == 429 {
		if res.StatusCode == 429 || res.Header.Get("X-RateLimit-Remaining") == "0" {
			return false, errf("rate_limited", "the provider rate limit was reached; wait before refreshing")
		}
		return false, errf(CodePermission, "the account cannot read this resource; check repository access, SSO and token permissions")
	}
	if res.StatusCode == 404 {
		return false, errf(CodeNotFound, "the repository or item is missing, or this account cannot access it")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return false, errf("provider_error", "the provider returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil {
		return false, errf("network_error", "the provider response could not be read")
	}
	if len(raw) > 4<<20 {
		return false, errf(CodeTooLarge, "the provider response exceeded 4 MiB; narrow the selection")
	}
	if strings.Contains(string(raw), token) {
		return false, errf("invalid_response", "the provider response contained a credential and was discarded")
	}
	if json.Unmarshal(raw, out) != nil {
		return false, errf("invalid_response", "the provider did not return valid JSON")
	}
	return strings.Contains(res.Header.Get("Link"), `rel="next"`) || res.Header.Get("X-Next-Page") != "", nil
}

type forgeRepo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Private bool   `json:"private"`
}
type forgeItem struct {
	Number   int      `json:"number"`
	Repo     string   `json:"repo"`
	Project  string   `json:"project"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	URL      string   `json:"url"`
	State    string   `json:"state"`
	Author   string   `json:"author"`
	Updated  string   `json:"updated"`
	Created  string   `json:"created,omitempty"`
	Labels   []string `json:"labels"`
	Comments int      `json:"comments"`
	Draft    bool     `json:"draft"`
	Branch   string   `json:"branch,omitempty"`
	Base     string   `json:"base,omitempty"`
}
type forgeComment struct {
	Author string `json:"author"`
	Body   string `json:"body"`
	At     string `json:"at"`
	URL    string `json:"url"`
}
type forgeFileDiff struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Patch  string `json:"patch"`
}
type forgeRequest struct {
	Account string        `json:"account"`
	Action  string        `json:"action"`
	Kind    string        `json:"kind"`
	Repo    string        `json:"repo"`
	Project string        `json:"project"`
	Scope   string        `json:"scope"`
	State   string        `json:"state"`
	Search  string        `json:"search"`
	Page    int           `json:"page"`
	Number  int           `json:"number"`
	Filters *forgeFilters `json:"filters,omitempty"`
	Ref     string        `json:"ref,omitempty"`
}

var forgeRepoPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)+$`)

func forgeValidRepo(provider, repo string) bool {
	if len(repo) > 500 || !forgeRepoPattern.MatchString(repo) {
		return false
	}
	parts := strings.Split(repo, "/")
	if provider != "gitlab" && len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}
func forgeQuery(ctx context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	var in forgeRequest
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, t, err := forgeCredential(e, in.Account)
	if err != nil {
		return nil, err
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Page < 1 || in.Page > 100 {
		return nil, errf(CodeInvalid, "page must be between 1 and 100")
	}
	if in.Kind == "" {
		in.Kind = "pull"
	}
	if in.Kind != "pull" && in.Kind != "issue" {
		return nil, errf(CodeInvalid, "choose pull or issue")
	}
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" && in.State != "merged" {
		return nil, errf(CodeInvalid, "choose open, closed, merged or all")
	}
	if in.Scope == "" {
		in.Scope = "created"
	}
	if in.Scope != "created" && in.Scope != "assigned" && in.Scope != "review" && in.Scope != "repo" {
		return nil, errf(CodeInvalid, "invalid scope")
	}
	if in.Repo != "" && !forgeValidRepo(p.Provider, in.Repo) {
		return nil, errf(CodeInvalid, "enter a repository path such as owner/repository")
	}
	if len(in.Search) > 200 {
		return nil, errf(CodeInvalid, "search is too long")
	}
	switch in.Action {
	case "repos":
		return forgeRepos(ctx, p, t, in)
	case "branches", "commits":
		return forgeHistory(ctx, p, t, in)
	case "list":
		return forgeItems(ctx, p, t, in)
	case "detail", "comments", "files":
		if in.Repo == "" || in.Number < 1 || in.Number > 100000000 {
			return nil, errf(CodeInvalid, "a repository and item number are required")
		}
		return forgeDetail(ctx, p, t, in)
	}
	return nil, errf(CodeInvalid, "choose repos, list, detail, comments, files, branches or commits")
}
func forgePage(in forgeRequest) url.Values {
	return url.Values{"page": {strconv.Itoa(in.Page)}, "per_page": {"50"}}
}
func forgeRepos(ctx context.Context, p forgeAccount, t string, in forgeRequest) (any, error) {
	q := forgePage(in)
	path := "/user/repos"
	if p.Provider == "gitlab" {
		path = "/projects"
		q.Set("membership", "true")
		q.Set("order_by", "last_activity_at")
		if in.Search != "" {
			q.Set("search", in.Search)
		}
	} else {
		q.Set("sort", "updated")
	}
	var raw []map[string]any
	more, err := forgeGet(ctx, p, t, path, q, &raw)
	if err != nil {
		return nil, err
	}
	out := []forgeRepo{}
	for _, r := range raw {
		name := str(r, "full_name")
		address := str(r, "html_url")
		private := boolean(r, "private")
		if p.Provider == "gitlab" {
			name = str(r, "path_with_namespace")
			address = str(r, "web_url")
			private = str(r, "visibility") != "public"
		}
		out = append(out, forgeRepo{ID: fmt.Sprint(r["id"]), Name: name, URL: address, Private: private})
	}
	return map[string]any{"items": out, "has_more": more, "page": in.Page}, nil
}
func forgeItems(ctx context.Context, p forgeAccount, t string, in forgeRequest) (any, error) {
	if err := forgeValidateFilters(p, in); err != nil {
		return nil, err
	}
	if in.Scope == "repo" && in.Repo == "" {
		return nil, errf(CodeInvalid, "select a repository")
	}
	if in.Kind == "issue" && in.Scope == "review" {
		return nil, errf(CodeInvalid, "review requests apply to pull requests")
	}
	q := forgePage(in)
	path := ""
	var raw []map[string]any
	more := false
	var err error
	incomplete := false
	if p.Provider == "github" {
		query := []string{"is:issue"}
		if in.Kind == "pull" {
			query = []string{"is:pr"}
		}
		if in.State != "all" {
			query = append(query, "is:"+in.State)
		}
		switch in.Scope {
		case "created":
			query = append(query, "author:"+p.Login)
		case "assigned":
			query = append(query, "assignee:"+p.Login)
		case "review":
			query = append(query, "review-requested:"+p.Login)
		}
		if in.Repo != "" {
			query = append(query, "repo:"+in.Repo)
		}
		if in.Search != "" {
			query = append(query, strconv.Quote(in.Search))
		}
		q.Set("sort", "updated")
		q.Set("order", "desc")
		forgeGitHubFilters(q, &query, in)
		q.Set("q", strings.Join(query, " "))
		var data struct {
			Items      []map[string]any `json:"items"`
			Incomplete bool             `json:"incomplete_results"`
		}
		more, err = forgeGet(ctx, p, t, "/search/issues", q, &data)
		raw = data.Items
		incomplete = data.Incomplete
	} else if p.Provider == "gitlab" {
		path = "/issues"
		if in.Kind == "pull" {
			path = "/merge_requests"
		}
		if in.Repo != "" {
			path = "/projects/" + url.PathEscape(in.Repo) + path
		}
		state := in.State
		if state == "open" {
			state = "opened"
		}
		q.Set("state", state)
		q.Set("order_by", "updated_at")
		q.Set("sort", "desc")
		scope := map[string]string{"created": "created_by_me", "assigned": "assigned_to_me", "review": "reviews_for_me", "repo": "all"}[in.Scope]
		q.Set("scope", scope)
		if in.Scope == "review" {
			q.Set("scope", "all")
			q.Set("reviewer_username", p.Login)
		}
		if in.Search != "" {
			q.Set("search", in.Search)
		}
		forgeGitLabFilters(q, in)
		more, err = forgeGet(ctx, p, t, path, q, &raw)
	} else {
		if in.Repo != "" {
			path = "/repos/" + forgeEscapedRepo(in.Repo) + "/issues"
		} else {
			path = "/repos/issues/search"
		}
		q.Set("type", map[string]string{"pull": "pulls", "issue": "issues"}[in.Kind])
		q.Set("state", in.State)
		q.Set("sort", "updated")
		q.Set("limit", "50")
		if in.Repo != "" {
			if in.Scope == "created" {
				q.Set("created_by", p.Login)
			} else if in.Scope == "assigned" {
				q.Set("assigned_by", p.Login)
			} else if in.Scope == "review" {
				return nil, errf("scope_unavailable", "clear the repository filter to view Gitea review requests")
			}
		} else if in.Scope == "created" {
			q.Set("created", "true")
		} else if in.Scope == "assigned" {
			q.Set("assigned", "true")
		} else if in.Scope == "review" {
			q.Set("review_requested", "true")
		}
		if in.Search != "" {
			q.Set("q", in.Search)
		}
		forgeGiteaFilters(q, in)
		more, err = forgeGet(ctx, p, t, path, q, &raw)
	}
	if err != nil {
		return nil, err
	}
	out := []forgeItem{}
	for _, r := range raw {
		out = append(out, forgeNormalize(p, r, in))
	}
	return map[string]any{"items": out, "has_more": more, "page": in.Page, "incomplete": incomplete}, nil
}
func forgeDetail(ctx context.Context, p forgeAccount, t string, in forgeRequest) (any, error) {
	base := "/repos/" + forgeEscapedRepo(in.Repo)
	kind := "/issues/"
	if in.Kind == "pull" {
		kind = "/pulls/"
	}
	if p.Provider == "gitlab" {
		base = "/projects/" + url.PathEscape(in.Repo)
		kind = "/issues/"
		if in.Kind == "pull" {
			kind = "/merge_requests/"
		}
	}
	path := base + kind + strconv.Itoa(in.Number)
	q := forgePage(in)
	if in.Action == "detail" {
		var r map[string]any
		_, err := forgeGet(ctx, p, t, path, nil, &r)
		if err != nil {
			return nil, err
		}
		return forgeNormalize(p, r, in), nil
	}
	if in.Action == "comments" {
		if p.Provider == "gitlab" {
			path += "/notes"
			q.Set("sort", "asc")
			q.Set("order_by", "created_at")
		} else {
			path = base + "/issues/" + strconv.Itoa(in.Number) + "/comments"
		}
		var raw []map[string]any
		more, err := forgeGet(ctx, p, t, path, q, &raw)
		if err != nil {
			return nil, err
		}
		out := []forgeComment{}
		for _, r := range raw {
			if boolean(r, "system") {
				continue
			}
			user := obj(r, "user")
			if p.Provider == "gitlab" {
				user = obj(r, "author")
			}
			login := str(user, "login")
			if login == "" {
				login = str(user, "username")
			}
			out = append(out, forgeComment{Author: login, Body: str(r, "body"), At: str(r, "created_at"), URL: str(r, "html_url")})
		}
		return map[string]any{"items": out, "has_more": more, "page": in.Page}, nil
	}
	if in.Kind != "pull" {
		return nil, errf(CodeInvalid, "only pull requests have changed files")
	}
	path += "/files"
	if p.Provider == "gitlab" {
		path = base + kind + strconv.Itoa(in.Number) + "/diffs"
	}
	var raw []map[string]any
	more, err := forgeGet(ctx, p, t, path, q, &raw)
	if err != nil {
		return nil, err
	}
	out := []forgeFileDiff{}
	for _, r := range raw {
		name := str(r, "filename")
		patch := str(r, "patch")
		status := str(r, "status")
		if p.Provider == "gitlab" {
			name = str(r, "new_path")
			patch = str(r, "diff")
			status = "modified"
			if boolean(r, "new_file") {
				status = "added"
			}
			if boolean(r, "deleted_file") {
				status = "removed"
			}
		}
		out = append(out, forgeFileDiff{Name: name, Patch: patch, Status: status})
	}
	return map[string]any{"items": out, "has_more": more, "page": in.Page}, nil
}
func str(r map[string]any, key string) string         { v, _ := r[key].(string); return v }
func boolean(r map[string]any, key string) bool       { v, _ := r[key].(bool); return v }
func obj(r map[string]any, key string) map[string]any { v, _ := r[key].(map[string]any); return v }
func num(r map[string]any, key string) int            { v, _ := r[key].(float64); return int(v) }
func forgeNormalize(p forgeAccount, r map[string]any, in forgeRequest) forgeItem {
	item := forgeItem{Number: num(r, "number"), Repo: in.Repo, Kind: in.Kind, Title: str(r, "title"), Body: str(r, "body"), URL: str(r, "html_url"), State: str(r, "state"), Author: str(obj(r, "user"), "login"), Updated: str(r, "updated_at"), Created: str(r, "created_at"), Comments: num(r, "comments"), Draft: boolean(r, "draft"), Labels: []string{}}
	if repository := obj(r, "repository"); str(repository, "full_name") != "" {
		item.Repo = str(repository, "full_name")
	}
	if item.Repo == "" {
		path := strings.TrimPrefix(str(r, "repository_url"), forgeBase(p)+"/repos/")
		if forgeRepoPattern.MatchString(path) {
			item.Repo = path
		}
	}
	if p.Provider == "gitlab" {
		item.Number = num(r, "iid")
		item.Project = fmt.Sprint(r["project_id"])
		item.Body = str(r, "description")
		item.URL = str(r, "web_url")
		item.Author = str(obj(r, "author"), "username")
		item.Comments = num(r, "user_notes_count")
		item.Branch = str(r, "source_branch")
		item.Base = str(r, "target_branch")
		if item.Repo == "" {
			u, _ := url.Parse(item.URL)
			if u != nil {
				item.Repo = strings.Split(strings.TrimPrefix(u.Path, "/"), "/-/")[0]
			}
		}
	}
	if item.State == "opened" {
		item.State = "open"
	}
	if str(r, "merged_at") != "" || boolean(r, "merged") || str(obj(r, "pull_request"), "merged_at") != "" {
		item.State = "merged"
	}
	if labels, ok := r["labels"].([]any); ok {
		for _, l := range labels {
			if s, ok := l.(string); ok {
				item.Labels = append(item.Labels, s)
			} else if v, ok := l.(map[string]any); ok {
				item.Labels = append(item.Labels, str(v, "name"))
			}
		}
	}
	if item.Branch == "" {
		item.Branch = str(obj(r, "head"), "ref")
		item.Base = str(obj(r, "base"), "ref")
	}
	return item
}

func forgeEscapedRepo(repo string) string {
	parts := strings.Split(repo, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
