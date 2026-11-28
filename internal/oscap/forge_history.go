package oscap

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/myco-ai/myco/internal/api"
)

func forgeHistoryCapabilities(_ context.Context, _ *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct{}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	return map[string]any{"version": 1, "actions": []string{"branches", "commits"}}, nil
}

var forgeSHA = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

func forgeHistory(ctx context.Context, p forgeAccount, token string, in forgeRequest) (any, error) {
	if in.Repo == "" {
		return nil, errf(CodeInvalid, "select a repository")
	}
	if len(in.Ref) > 255 || strings.ContainsAny(in.Ref, "~^:?*[\\") || strings.Contains(in.Ref, "..") || strings.Contains(in.Ref, "@{") || strings.HasPrefix(in.Ref, "-") {
		return nil, errf(CodeInvalid, "enter a branch name or commit SHA")
	}
	for _, r := range in.Ref {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, errf(CodeInvalid, "invalid branch name")
		}
	}
	q := forgePage(in)
	base := "/repos/" + forgeEscapedRepo(in.Repo)
	if p.Provider == "gitlab" {
		base = "/projects/" + url.PathEscape(in.Repo) + "/repository"
	}
	path := base + "/" + in.Action
	if p.Provider == "gitea" {
		q.Set("limit", "50")
	}
	if in.Action == "commits" && in.Ref != "" {
		key := "sha"
		if p.Provider == "gitlab" {
			key = "ref_name"
		}
		q.Set(key, in.Ref)
	}
	if in.Action == "commits" && p.Provider == "gitlab" {
		q.Set("order", "topo")
	}
	var raw []map[string]any
	more, err := forgeGet(ctx, p, token, path, q, &raw)
	if err != nil {
		return nil, err
	}
	if len(raw) > 50 {
		return nil, errf(CodeTooLarge, "the provider exceeded the requested history page size")
	}
	if in.Action == "branches" {
		rows := []api.CodeHostBranch{}
		for _, r := range raw {
			commit := obj(r, "commit")
			sha := str(commit, "sha")
			if p.Provider != "github" {
				sha = str(commit, "id")
			}
			if !forgeSHA.MatchString(sha) {
				return nil, errf("invalid_response", "invalid branch head")
			}
			rows = append(rows, api.CodeHostBranch{Name: str(r, "name"), SHA: sha})
		}
		return map[string]any{"items": rows, "has_more": more, "page": in.Page}, nil
	}
	rows := []api.CodeHostCommit{}
	for _, r := range raw {
		c := api.CodeHostCommit{SHA: str(r, "sha"), URL: str(r, "html_url"), Parents: []string{}}
		if p.Provider == "gitlab" {
			c.SHA = str(r, "id")
			c.Message = str(r, "message")
			c.Author = str(r, "author_name")
			c.Date = str(r, "committed_date")
			c.URL = str(r, "web_url")
			if parents, ok := r["parent_ids"].([]any); ok {
				for _, v := range parents {
					if s, ok := v.(string); ok {
						c.Parents = append(c.Parents, s)
					}
				}
			}
		} else {
			commit := obj(r, "commit")
			c.Message = str(commit, "message")
			c.Author = str(obj(commit, "author"), "name")
			c.Date = str(obj(commit, "committer"), "date")
			if parents, ok := r["parents"].([]any); ok {
				for _, v := range parents {
					if m, ok := v.(map[string]any); ok {
						c.Parents = append(c.Parents, str(m, "sha"))
					}
				}
			}
		}
		if len(c.Parents) > 64 {
			return nil, errf(CodeTooLarge, "the commit parent list exceeded the history limit")
		}
		if !forgeSHA.MatchString(c.SHA) {
			return nil, errf("invalid_response", "invalid commit identity")
		}
		for _, sha := range c.Parents {
			if !forgeSHA.MatchString(sha) {
				return nil, errf("invalid_response", "invalid parent identity")
			}
		}
		c.Subject = strings.SplitN(c.Message, "\n", 2)[0]
		rows = append(rows, c)
	}
	return map[string]any{"items": rows, "has_more": more, "page": in.Page}, nil
}
