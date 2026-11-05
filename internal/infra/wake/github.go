package wake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

const githubAPI = "https://api.github.com"

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func githubHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":        "Bearer " + token,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
}

func requireRepo(a *domain.Agent) error {
	if !repoPattern.MatchString(a.Delivery.Params["repo"]) {
		return domain.Errf(domain.CodeInvalidArgument, "delivery %s needs repo=owner/name", a.Delivery.Mode).
			WithSuggestion("pass --wake-param repo=owner/name")
	}
	return requireSecret(a, "a GitHub token")
}

func apiBase(a *domain.Agent) string { return paramOr(a.Delivery.Params, "api_url", githubAPI) }

// GitHubDispatch sends a repository_dispatch event; a workflow in the repo
// (e.g. running `myco runner start --once` with codex/claude) handles it.
// The agent replies with the myco CLI, so no ingress is needed.
type GitHubDispatch struct{ HTTP Doer }

var _ app.Waker = GitHubDispatch{}

func (GitHubDispatch) Mode() string                   { return "github_dispatch" }
func (GitHubDispatch) Consumes() bool                 { return false }
func (GitHubDispatch) Validate(a *domain.Agent) error { return requireRepo(a) }

func (g GitHubDispatch) Wake(ctx context.Context, req app.WakeRequest) error {
	a := req.Agent
	_, err := postJSON(ctx, httpOr(g.HTTP), apiBase(a)+"/repos/"+a.Delivery.Params["repo"]+"/dispatches",
		githubHeaders(a.Delivery.Secret), map[string]any{
			"event_type": paramOr(a.Delivery.Params, "event_type", "myco_wake"),
			"client_payload": map[string]any{
				"agent": a.Name, "session_id": a.Binding.SessionID,
				"message_ids": ids(req.Pending), "text": clipText(req.Text, 60000),
			},
		})
	return err
}

// GitHubIssue hands each pending message to an agent that lives on GitHub
// and cannot run myco (e.g. Codex cloud): it opens an issue per message and
// comments a mention (default "@codex"). Replies come back through the
// GitHub ingress (/v1/ingress/github), correlated by the message ID in the
// issue title.
type GitHubIssue struct{ HTTP Doer }

var _ app.Waker = GitHubIssue{}

func (GitHubIssue) Mode() string                   { return "github_issue" }
func (GitHubIssue) Consumes() bool                 { return true }
func (GitHubIssue) Validate(a *domain.Agent) error { return requireRepo(a) }

// IssueMarker tags hub-authored text so the ingress ignores its own comments.
const IssueMarker = "<!-- myco:hub -->"

// TitleMessageID extracts the myco message ID from an issue title.
var TitleMessageID = regexp.MustCompile(`\[myco\] (msg_[a-z0-9]+)`)

func (g GitHubIssue) Wake(ctx context.Context, req app.WakeRequest) error {
	a := req.Agent
	h, repo := httpOr(g.HTTP), a.Delivery.Params["repo"]
	headers := githubHeaders(a.Delivery.Secret)
	mention := paramOr(a.Delivery.Params, "mention", "@codex")
	for _, m := range req.Pending {
		body := fmt.Sprintf("%s\n\n%s\n\n---\nFrom myco agent `%s` (message `%s`, thread `%s`). "+
			"Post your final answer as a comment on this issue (or open a PR that mentions `%s`).",
			IssueMarker, m.Body, m.From, m.ID, m.ThreadID, m.ID)
		number, err := findOrCreateIssue(ctx, h, apiBase(a), repo, headers, m, body)
		if err != nil {
			return err
		}
		comment := fmt.Sprintf("%s\n%s please handle myco message `%s`:\n\n%s", IssueMarker, mention, m.ID, m.Body)
		if _, err := postJSON(ctx, h, fmt.Sprintf("%s/repos/%s/issues/%s/comments", apiBase(a), repo, number),
			headers, map[string]string{"body": comment}); err != nil {
			return err
		}
	}
	return nil
}

func findOrCreateIssue(ctx context.Context, h Doer, base, repo string, headers map[string]string, m *domain.Message, body string) (string, error) {
	title := fmt.Sprintf("[myco] %s from %s", m.ID, m.From)
	q := url.QueryEscape(fmt.Sprintf(`repo:%s is:issue in:title "%s"`, repo, m.ID))
	data, err := do(ctx, h, "GET", base+"/search/issues?q="+q, headers, nil)
	if err == nil {
		var res struct {
			Items []struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
			} `json:"items"`
		}
		if json.Unmarshal(data, &res) == nil {
			for _, it := range res.Items {
				if strings.Contains(it.Title, m.ID) {
					return fmt.Sprint(it.Number), nil
				}
			}
		}
	}
	data, err = postJSON(ctx, h, base+"/repos/"+repo+"/issues", headers, map[string]any{"title": title, "body": body, "labels": []string{"myco"}})
	if err != nil {
		return "", err
	}
	var created struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(data, &created); err != nil || created.Number == 0 {
		return "", fmt.Errorf("unexpected GitHub response creating issue: %s", clipText(string(data), 300))
	}
	return fmt.Sprint(created.Number), nil
}

func clipText(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
