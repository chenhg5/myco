// Package ingress turns callbacks from external systems (GitHub, ...) into
// myco replies. Each source implements httpapi.Ingress.
package ingress

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/wake"
	"github.com/chenhg5/myco/internal/transport/httpapi"
)

var msgIDPattern = regexp.MustCompile(`msg_[a-z0-9]{16}`)

// GitHub handles GitHub webhooks for agents using the github_issue delivery:
// a comment on a "[myco] msg_..." issue, or a PR mentioning a message ID,
// becomes the reply to that message.
type GitHub struct {
	// Secret is the webhook secret configured on the GitHub repo/app.
	Secret string
	// AllowedAuthors, if set, only accepts replies from these logins
	// (e.g. "chatgpt-codex-connector[bot]").
	AllowedAuthors []string
}

var _ httpapi.Ingress = (*GitHub)(nil)

func (*GitHub) Kind() string { return "github" }

type ghUser struct {
	Login string `json:"login"`
}

type ghEvent struct {
	Action string `json:"action"`
	Issue  *struct {
		Title string `json:"title"`
	} `json:"issue"`
	Comment *struct {
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		User    ghUser `json:"user"`
	} `json:"comment"`
	PullRequest *struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		User    ghUser `json:"user"`
	} `json:"pull_request"`
}

func (g *GitHub) Parse(r *http.Request, body []byte) ([]httpapi.Inbound, error) {
	if g.Secret == "" {
		return nil, domain.Errf(domain.CodePermissionDenied, "github ingress is not configured on this hub").
			WithSuggestion("start the hub with --github-webhook-secret-file")
	}
	mac := hmac.New(sha256.New, []byte(g.Secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		return nil, domain.Errf(domain.CodeUnauthenticated, "invalid GitHub webhook signature")
	}
	var ev ghEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "invalid GitHub payload: %v", err)
	}
	switch r.Header.Get("X-GitHub-Event") {
	case "issue_comment":
		if ev.Action != "created" || ev.Issue == nil || ev.Comment == nil {
			return nil, nil
		}
		m := wake.TitleMessageID.FindStringSubmatch(ev.Issue.Title)
		c := ev.Comment
		if m == nil || strings.Contains(c.Body, wake.IssueMarker) || !g.allowed(c.User.Login) {
			return nil, nil
		}
		return []httpapi.Inbound{{MessageID: m[1], Body: c.Body + "\n\n(" + c.HTMLURL + ")", Source: "github:" + c.User.Login}}, nil
	case "pull_request":
		pr := ev.PullRequest
		if (ev.Action != "opened" && ev.Action != "ready_for_review") || pr == nil || !g.allowed(pr.User.Login) {
			return nil, nil
		}
		var out []httpapi.Inbound
		for _, id := range uniq(msgIDPattern.FindAllString(pr.Title+"\n"+pr.Body, -1)) {
			out = append(out, httpapi.Inbound{MessageID: id, Source: "github:" + pr.User.Login,
				Body: fmt.Sprintf("Opened pull request: %s\n\n%s\n\n%s", pr.HTMLURL, pr.Title, pr.Body)})
		}
		return out, nil
	}
	return nil, nil // ping and other events are acknowledged and ignored
}

func (g *GitHub) allowed(login string) bool {
	return len(g.AllowedAuthors) == 0 || slices.Contains(g.AllowedAuthors, login)
}

func uniq(in []string) []string {
	slices.Sort(in)
	return slices.Compact(in)
}
