package oscap

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestForgeServerSideFilterQueries(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	for _, provider := range []string{"github", "gitlab", "gitea"} {
		t.Run(provider, func(t *testing.T) {
			f := &forgeFilters{Author: "demo", UpdatedAfter: "2026-11-01"}
			state := "open"
			if provider != "gitea" {
				f.Label = `needs review`
				f.Draft = "exclude"
				f.Sort = "created_asc"
				state = "merged"
			}
			forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("filters must not mutate")
				}
				q := r.URL.Query()
				if q.Get("page") != "3" {
					t.Fatal("lost pagination")
				}
				body := `[{"number":7,"iid":7,"created_at":"2026-11-01T01:00:00Z"}]`
				switch provider {
				case "github":
					for _, term := range []string{"is:pr", "is:merged", "repo:acme/widget", "author:demo", `label:"needs review"`, "updated:>=2026-11-01", "draft:false", `"patch"`} {
						if !strings.Contains(q.Get("q"), term) {
							t.Errorf("missing %s in %s", term, q.Get("q"))
						}
					}
					if q.Get("sort") != "created" || q.Get("order") != "asc" {
						t.Fatal("lost sorting")
					}
					body = `{"items":` + body + `}`
				case "gitlab":
					for k, v := range map[string]string{"scope": "all", "state": "merged", "author_username": "demo", "labels": "needs review", "updated_after": "2026-11-01T00:00:00Z", "wip": "no", "order_by": "created_at", "sort": "asc", "search": "patch"} {
						if q.Get(k) != v {
							t.Errorf("%s=%q want %q", k, q.Get(k), v)
						}
					}
					if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fwidget/merge_requests" {
						t.Fatal("lost repository binding")
					}
				case "gitea":
					if q.Get("created_by") != "demo" || q.Get("since") != "2026-11-01T00:00:00Z" || q.Get("type") != "pulls" {
						t.Fatal("lost gitea filters")
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Link": {`<https://example.com/next>; rel="next"`}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			value, err := forgeItems(context.Background(), forgeAccount{Provider: provider, Host: "https://git.example.com", Login: "me"}, "fixture-secret", forgeRequest{Kind: "pull", Repo: "acme/widget", Scope: "repo", State: state, Search: "patch", Page: 3, Filters: f})
			if err != nil {
				t.Fatal(err)
			}
			data := value.(map[string]any)
			if data["has_more"] != true || data["items"].([]forgeItem)[0].Created != "2026-11-01T01:00:00Z" {
				t.Fatal("lost next page or created date")
			}
		})
	}
}
func TestForgeRejectsUnsupportedOrAmbiguousFiltersBeforeRequest(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	forgeTransport = forgeTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid filter reached provider")
		return nil, nil
	})
	for _, tc := range []struct {
		provider, kind, scope, repo, state string
		f                                  forgeFilters
	}{
		{"github", "pull", "created", "", "open", forgeFilters{Author: "other"}},
		{"github", "issue", "repo", "acme/widget", "merged", forgeFilters{}},
		{"github", "issue", "repo", "acme/widget", "open", forgeFilters{Draft: "only"}},
		{"github", "pull", "repo", "acme/widget", "open", forgeFilters{Author: "demo is:closed"}},
		{"github", "pull", "repo", "acme/widget", "open", forgeFilters{Label: "bug,feature"}},
		{"github", "pull", "repo", "acme/widget", "open", forgeFilters{UpdatedAfter: "2026-02-30"}},
		{"github", "pull", "repo", "acme/widget", "open", forgeFilters{Sort: "random"}},
		{"gitlab", "pull", "repo", "acme/widget", "open", forgeFilters{Label: "None"}},
		{"gitea", "pull", "repo", "acme/widget", "merged", forgeFilters{}},
		{"gitea", "pull", "repo", "acme/widget", "open", forgeFilters{Label: "bug"}},
		{"gitea", "pull", "assigned", "", "open", forgeFilters{Author: "demo"}},
	} {
		_, err := forgeItems(context.Background(), forgeAccount{Provider: tc.provider, Login: "me"}, "fixture-secret", forgeRequest{Kind: tc.kind, Repo: tc.repo, Scope: tc.scope, State: tc.state, Page: 1, Filters: &tc.f})
		if err == nil {
			t.Fatalf("accepted invalid %+v", tc)
		}
	}
}

func TestForgeGitLabReviewFilter(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("scope") != "all" || r.URL.Query().Get("reviewer_username") != "demo" {
			t.Fatal("review scope must use the reviewer filter")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("[]"))}, nil
	})
	_, err := forgeItems(context.Background(), forgeAccount{Provider: "gitlab", Host: "https://git.example.com", Login: "demo"}, "fixture-secret", forgeRequest{Kind: "pull", Repo: "acme/widget", Scope: "review", State: "open", Page: 1})
	if err != nil {
		t.Fatal(err)
	}
}
