package oscap

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// Fields are explicit rather than a provider search expression. They can only
// narrow fixed GET endpoints; they never select an account or credential.
type forgeFilters = api.CodeHostQueryFilters

func forgeFilterCapabilities(_ context.Context, _ *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct{}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	return map[string]any{"version": 1, "providers": map[string][]string{
		"github": {"author", "label", "updated_after", "draft", "sort", "merged"},
		"gitlab": {"author", "label", "updated_after", "draft", "sort", "merged"},
		"gitea":  {"author_repo", "updated_after"},
	}}, nil
}

var forgeLoginPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\[\]-]{1,100}$`)

func forgeValidateFilters(p forgeAccount, in forgeRequest) error {
	if in.State == "merged" && (in.Kind != "pull" || p.Provider == "gitea") {
		return errf("filter_unavailable", "merged status is supported for GitHub and GitLab pull requests")
	}
	if in.Filters == nil {
		return nil
	}
	f := in.Filters
	if f.Author != "" && !forgeLoginPattern.MatchString(f.Author) {
		return errf(CodeInvalid, "enter a username, not a URL or query expression")
	}
	if f.Author != "" && in.Scope == "created" && !strings.EqualFold(f.Author, p.Login) {
		return errf(CodeInvalid, "the author conflicts with created-by-me scope")
	}
	if p.Provider == "gitlab" && (strings.EqualFold(f.Label, "Any") || strings.EqualFold(f.Label, "None")) {
		return errf("filter_unavailable", "this label name is reserved by the GitLab API")
	}
	if len(f.Label) > 100 || strings.ContainsAny(f.Label, ",\r\n\x00") || strings.TrimSpace(f.Label) != f.Label {
		return errf(CodeInvalid, "enter one label name of at most 100 bytes")
	}
	for _, r := range f.Label {
		if r < 32 || r == 127 {
			return errf(CodeInvalid, "invalid label")
		}
	}
	if f.UpdatedAfter != "" {
		if _, err := time.Parse("2006-01-02", f.UpdatedAfter); err != nil {
			return errf(CodeInvalid, "updated_after must be a UTC date (YYYY-MM-DD)")
		}
	}
	if f.Draft != "" && (in.Kind != "pull" || (f.Draft != "only" && f.Draft != "exclude")) {
		return errf(CodeInvalid, "draft must be only or exclude for pull requests")
	}
	if f.Sort != "" && f.Sort != "updated_desc" && f.Sort != "updated_asc" && f.Sort != "created_desc" && f.Sort != "created_asc" {
		return errf(CodeInvalid, "invalid sort order")
	}
	if p.Provider == "gitea" && (f.Label != "" || f.Draft != "" || f.Sort != "" || (f.Author != "" && in.Repo == "")) {
		return errf("filter_unavailable", "this Gitea query supports updated_after and a repository-scoped author")
	}
	return nil
}

func forgeSort(in forgeRequest) (string, string) {
	if in.Filters != nil && in.Filters.Sort != "" {
		p := strings.Split(in.Filters.Sort, "_")
		return p[0], p[1]
	}
	return "updated", "desc"
}
func forgeGitHubFilters(q url.Values, terms *[]string, in forgeRequest) {
	if in.Filters == nil {
		return
	}
	f := in.Filters
	if f.Author != "" && in.Scope != "created" {
		*terms = append(*terms, "author:"+f.Author)
	}
	if f.Label != "" {
		*terms = append(*terms, "label:"+strconv.Quote(f.Label))
	}
	if f.UpdatedAfter != "" {
		*terms = append(*terms, "updated:>="+f.UpdatedAfter)
	}
	if f.Draft != "" {
		*terms = append(*terms, "draft:"+strconv.FormatBool(f.Draft == "only"))
	}
	sort, order := forgeSort(in)
	q.Set("sort", sort)
	q.Set("order", order)
}
func forgeGitLabFilters(q url.Values, in forgeRequest) {
	if in.Filters == nil {
		return
	}
	f := in.Filters
	if f.Author != "" {
		q.Set("author_username", f.Author)
	}
	if f.Label != "" {
		q.Set("labels", f.Label)
	}
	if f.UpdatedAfter != "" {
		q.Set("updated_after", f.UpdatedAfter+"T00:00:00Z")
	}
	if f.Draft != "" {
		value := "no"
		if f.Draft == "only" {
			value = "yes"
		}
		q.Set("wip", value)
	}
	sort, order := forgeSort(in)
	q.Set("order_by", sort+"_at")
	q.Set("sort", order)
}
func forgeGiteaFilters(q url.Values, in forgeRequest) {
	if in.Filters == nil {
		return
	}
	f := in.Filters
	if f.Author != "" {
		q.Set("created_by", f.Author)
	}
	if f.UpdatedAfter != "" {
		q.Set("since", f.UpdatedAfter+"T00:00:00Z")
	}
}
