package oscap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestForgeRepositoryHistoryProviders(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	sha := strings.Repeat("a", 40)
	parent := strings.Repeat("b", 40)
	for _, provider := range []string{"github", "gitlab", "gitea"} {
		t.Run(provider, func(t *testing.T) {
			paths := []string{}
			forgeTransport = forgeTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("mutation")
				}
				paths = append(paths, r.URL.EscapedPath())
				if r.URL.Query().Get("page") != "2" {
					t.Fatal("lost history page")
				}
				body := ""
				if strings.HasSuffix(r.URL.Path, "/branches") {
					key := "id"
					if provider == "github" {
						key = "sha"
					}
					body = `[{"name":"feature/demo","commit":{"` + key + `":"` + sha + `"}}]`
				} else {
					key := "sha"
					if provider == "gitlab" {
						key = "ref_name"
					}
					if r.URL.Query().Get(key) != "feature/demo" {
						t.Fatal("ref must be a query value")
					}
					if provider == "gitlab" {
						body = `[{"id":"` + sha + `","parent_ids":["` + parent + `"],"message":"Subject\n\nBody","author_name":"Demo","committed_date":"2026-11-01T12:00:00Z","web_url":"https://git.example.com/acme/widget/-/commit/` + sha + `"}]`
					} else {
						body = `[{"sha":"` + sha + `","parents":[{"sha":"` + parent + `"}],"commit":{"message":"Subject\n\nBody","author":{"name":"Demo","email":"private@example.com"},"committer":{"date":"2026-11-01T12:00:00Z"}},"html_url":"https://git.example.com/acme/widget/commit/` + sha + `"}]`
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Link": {`<https://example.com/next>; rel="next"`}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			for _, action := range []string{"branches", "commits"} {
				v, err := forgeHistory(context.Background(), forgeAccount{Provider: provider, Host: "https://git.example.com"}, "fixture-secret", forgeRequest{Action: action, Repo: "acme/widget", Ref: "feature/demo", Page: 2})
				if err != nil {
					t.Fatal(err)
				}
				data := v.(map[string]any)
				if data["has_more"] != true {
					t.Fatal("lost more flag")
				}
				if action == "branches" {
					if data["items"].([]api.CodeHostBranch)[0].SHA != sha {
						t.Fatal("wrong branch head")
					}
				} else {
					c := data["items"].([]api.CodeHostCommit)[0]
					if c.SHA != sha || len(c.Parents) != 1 || c.Parents[0] != parent || c.Subject != "Subject" || c.Message != "Subject\n\nBody" || c.Author != "Demo" {
						t.Fatalf("bad commit %+v", c)
					}
					raw, _ := json.Marshal(c)
					if strings.Contains(string(raw), "private@example.com") {
						t.Fatal("unneeded email leaked")
					}
				}
			}
			prefix := "/api/v1/repos/acme/widget/"
			if provider == "github" {
				prefix = "/api/v3/repos/acme/widget/"
			}
			if provider == "gitlab" {
				prefix = "/api/v4/projects/acme%2Fwidget/repository/"
			}
			if paths[0] != prefix+"branches" || paths[1] != prefix+"commits" {
				t.Fatalf("unexpected paths: %v", paths)
			}
		})
	}
}
func TestForgeHistoryRefValidation(t *testing.T) {
	old := forgeTransport
	t.Cleanup(func() { forgeTransport = old })
	forgeTransport = forgeTestTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid ref reached network"); return nil, nil })
	for _, ref := range []string{"main\nsecret", "https://example.com/x", "../main", "main~1", strings.Repeat("a", 256)} {
		_, err := forgeHistory(context.Background(), forgeAccount{Provider: "github"}, "fixture-secret", forgeRequest{Repo: "acme/widget", Action: "commits", Ref: ref, Page: 1})
		if err == nil {
			t.Fatalf("accepted ref %q", ref)
		}
	}
}
