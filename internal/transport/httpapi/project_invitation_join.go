package httpapi

import (
	"fmt"
	htmltemplate "html/template"
	"net/http"
	"strings"

	"github.com/myco-ai/myco/internal/domain"
)

// A project invitation admits a session, never a computer or a reusable
// machine enrollment. Fetching this page does not consume the invitation.
func (s *Server) projectInvitationPage(w http.ResponseWriter, r *http.Request, e *domain.Enrollment, token string) {
	base := s.publicBase(r)
	link := base + "/j/" + token
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
MYCO_INSTALLED_FILE="$(mktemp)"
export MYCO_INSTALLED_FILE
curl -fsSL --retry 5 https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.sh | MYCO_NO_DOCTOR=1 MYCO_ADD_PATH=1 bash
MYCO="$(cat "$MYCO_INSTALLED_FILE" 2>/dev/null || true)"
rm -f "$MYCO_INSTALLED_FILE"
[ -n "$MYCO" ] && [ -x "$MYCO" ] || MYCO="$(command -v myco)"
export MYCO_ENROLL_TOKEN=%s
"$MYCO" join --invitation --hub %s
unset MYCO_ENROLL_TOKEN
`, shq(token), shq(base))
	switch joinFormat(r) {
	case "sh":
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		fmt.Fprint(w, script)
	case "ps1":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "$ErrorActionPreference = 'Stop'\nirm https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.ps1 | iex\n$env:MYCO_ENROLL_TOKEN = %s\ntry { myco join --invitation --hub %s; if ($LASTEXITCODE -ne 0) { throw 'Joining failed' } } finally { Remove-Item Env:MYCO_ENROLL_TOKEN -ErrorAction SilentlyContinue }\n", psq(token), psq(base))
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		fmt.Fprintf(w, "# Join this myco project\n\nThis invitation admits **one ordinary agent session**, owned by the project owner and automatically added as a member. It expires at %s. It does not connect a computer or grant admin powers. The link is a credential; keep it private.\n\nRun in the invited session (macOS/Linux/WSL):\n\n```sh\ncurl -fsSL %s | bash\n```\n\nOn Windows PowerShell: `irm '%s?format=ps1' | iex`. If an old CLI rejects --invitation, upgrade myco first.\n\nThen keep `myco inbox wait` running in the background and handle its messages. Save the resume command printed by join for a temporary sandbox; this invitation cannot be reused.\n", e.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"), link, link)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		// Only the validated server URL and generated token are interpolated.
		v := joinView{Link: link, Command: "curl -fsSL " + link + " | bash", Expires: e.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC")}
		_ = projectInvitationHTML.Execute(w, v)
	}
}

var projectInvitationHTML = htmltemplate.Must(htmltemplate.New("project-invitation").Parse(strings.TrimSpace(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Join a myco project</title><body><h1>Join a myco project · 加入项目</h1><p>One ordinary agent, automatically added to the project. Only use this in the invited session.</p><p>邀请一个普通 agent，自动加入项目。请只在受邀会话内使用。</p><pre>{{.Command}}</pre><p>Expires · 有效期：{{.Expires}}</p><a href="{{.Link}}?format=md">Agent instructions</a></body></html>`)))
