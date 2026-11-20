package console

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// Looking at a computer this App can reach (over SSH or into WSL), and the
// two fixes that cover most trouble: restart its runner, upgrade its myco.
// Only names of what is found are sent back, never a credential: proxy
// passwords, tokens and keys are masked before anything leaves.

// diagScript reads, without changing anything: myco and its version, the
// runner service, linger, the hubs it belongs to, the proxies the service
// runs with, whether each hub answers through that proxy and directly, and
// the runner log's last lines. Each finding is "myco-check <key> <value>".
const diagScript = `cd "$HOME" 2>/dev/null
c() { echo "myco-check $1 $2"; }
c os "$(uname -s 2>/dev/null)"
M="$(command -v myco 2>/dev/null)"; [ -z "$M" ] && [ -x "$HOME/.local/bin/myco" ] && M="$HOME/.local/bin/myco"
if [ -n "$M" ]; then c myco "$("$M" --version 2>/dev/null | head -n1)"; c myco_path "$M"; else c myco none; fi
case "$(uname -s)" in
Linux)
  if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
    c service "$(systemctl --user is-active myco-runner.service 2>/dev/null)"
    c since "$(systemctl --user show myco-runner.service -p ActiveEnterTimestamp --value 2>/dev/null)"
    c linger "$(loginctl show-user "$(id -un)" -p Linger --value 2>/dev/null)"
    ENV="$(systemctl --user show myco-runner.service -p Environment --value 2>/dev/null)"
  else
    c service no_systemd
  fi ;;
Darwin)
  if launchctl print "gui/$(id -u)/ai.myco.runner" >/dev/null 2>&1; then
    c service "$(launchctl print "gui/$(id -u)/ai.myco.runner" 2>/dev/null | sed -n 's/^[[:space:]]*state = //p' | head -n1)"
  else c service missing; fi
  P="$HOME/Library/LaunchAgents/ai.myco.runner.plist"
  [ -f "$P" ] && ENV="$(tr -d '\n\t ' < "$P" | grep -o '<key>[A-Za-z_]*[Pp][Rr][Oo][Xx][Yy]</key><string>[^<]*' | sed 's#<key>\(.*\)</key><string>#\1=#')" ;;
esac
c running "$(pgrep -f 'myco runner start' >/dev/null 2>&1 && echo yes || echo no)"
PROXY=""
for kv in $ENV; do
  case "$kv" in
    [Hh][Tt][Tt][Pp][Ss]_[Pp][Rr][Oo][Xx][Yy]=*) PROXY="${kv#*=}"; c proxy "$PROXY" ;;
    [Aa][Ll][Ll]_[Pp][Rr][Oo][Xx][Yy]=*) [ -z "$PROXY" ] && c proxy_all "${kv#*=}" ;;
    [Nn][Oo]_[Pp][Rr][Oo][Xx][Yy]=*) c no_proxy "${kv#*=}" ;;
  esac
done
for f in "$HOME/.myco/machine.yaml"; do
  [ -f "$f" ] || continue
  for h in $(sed -n 's/^ *-\{0,1\} *hub: *//p' "$f" | tr -d '"'"'"); do
    c hub "$h"
    if command -v curl >/dev/null 2>&1; then
      c hub_direct "$h $(curl -s -o /dev/null -w '%{http_code}' --max-time 8 --noproxy '*' "$h/v1/health" 2>/dev/null)"
      [ -n "$PROXY" ] && c hub_proxy "$h $(curl -s -o /dev/null -w '%{http_code}' --max-time 8 -x "$PROXY" "$h/v1/health" 2>/dev/null)"
    fi
  done
done
if [ -f "$HOME/.myco/runner.log" ]; then
  tail -n 20 "$HOME/.myco/runner.log" | cut -c1-300 | sed 's/^/myco-log /'
fi
c end ok
`

// fixScripts are the two fixes. Upgrading reuses the installer (which
// installs into ~/.local/bin when it cannot use sudo) and points the
// service at the new binary.
var fixScripts = map[string]string{
	"restart": `cd "$HOME" 2>/dev/null
M="$(command -v myco 2>/dev/null)"; [ -z "$M" ] && M="$HOME/.local/bin/myco"
"$M" runner service restart 2>&1 || "$M" runner service install 2>&1
`,
	"upgrade": `cd "$HOME" 2>/dev/null
OLD="$(command -v myco 2>/dev/null)"
MYCO_INSTALLED_FILE="$(mktemp)"; export MYCO_INSTALLED_FILE
curl -fsSL --retry 3 https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.sh | MYCO_NO_DOCTOR=1 bash 2>&1 || exit 1
NEW="$(cat "$MYCO_INSTALLED_FILE" 2>/dev/null)"; rm -f "$MYCO_INSTALLED_FILE"
[ -n "$NEW" ] || NEW="$(command -v myco)"
if [ "$NEW" = "$OLD" ]; then "$NEW" runner service restart 2>&1; else "$NEW" runner service install 2>&1; fi
`,
}

var (
	credInURL = regexp.MustCompile(`//[^/@\s]*@`)
	tokenLike = regexp.MustCompile(`(?i)(myco_[a-z]+_|bearer\s+|token[=:]\s*|key[=:]\s*)[A-Za-z0-9._\-]{8,}`)
)

// maskSecrets hides proxy passwords and anything that looks like a token.
func maskSecrets(s string) string {
	s = credInURL.ReplaceAllString(s, "//***@")
	return tokenLike.ReplaceAllString(s, "${1}***")
}

// Diagnosis is what was found on a computer.
type Diagnosis struct {
	Route Route             `json:"route"`
	Found map[string]string `json:"found"`
	// Hubs: each hub it belongs to, with the HTTP status it answered
	// directly and through the service's proxy ("000": no answer).
	Hubs []DiagHub `json:"hubs"`
	Log  []string  `json:"log,omitempty"`
}

type DiagHub struct {
	URL string `json:"url"`
	// Mine: it is the workspace the check was asked from.
	Mine   bool   `json:"mine,omitempty"`
	Direct string `json:"direct,omitempty"`
	Proxy  string `json:"proxy,omitempty"`
}

func parseDiag(lines []string) (map[string]string, []DiagHub, []string) {
	found := map[string]string{}
	var hubs []DiagHub
	var log []string
	hub := func(u string) *DiagHub {
		for i := range hubs {
			if hubs[i].URL == u {
				return &hubs[i]
			}
		}
		hubs = append(hubs, DiagHub{URL: u})
		return &hubs[len(hubs)-1]
	}
	for _, l := range lines {
		if rest, ok := strings.CutPrefix(l, "myco-log "); ok {
			log = append(log, maskSecrets(rest))
			continue
		}
		rest, ok := strings.CutPrefix(l, "myco-check ")
		if !ok {
			continue
		}
		k, v, _ := strings.Cut(rest, " ")
		v = maskSecrets(strings.TrimSpace(v))
		switch k {
		case "hub":
			hub(v)
		case "hub_direct", "hub_proxy":
			u, code, _ := strings.Cut(v, " ")
			if k == "hub_direct" {
				hub(u).Direct = code
			} else {
				hub(u).Proxy = code
			}
		default:
			found[k] = v
		}
	}
	return found, hubs, log
}

func (b *Backend) diagRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /app/hubs/{id}/machines/{mid}/dirs", b.postMachineDirs)
	// GET: how this App reaches that computer (null: it does not).
	m.HandleFunc("GET /app/hubs/{id}/machines/{mid}/reach", func(w http.ResponseWriter, r *http.Request) {
		rt, ok := b.reachOf(r.PathValue("id"), r.PathValue("mid"))
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"route": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"route": rt})
	})
	// PUT {via, target, port}: reach it this way from now on.
	m.HandleFunc("PUT /app/hubs/{id}/machines/{mid}/reach", func(w http.ResponseWriter, r *http.Request) {
		var in Route
		if !decode(w, r, &in) {
			return
		}
		if !validRoute(&in) {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "say how to reach it: via ssh or wsl, and a plain name"})
			return
		}
		b.rememberReach(in, r.PathValue("id"), r.PathValue("mid"))
		writeJSON(w, http.StatusOK, map[string]any{"route": in})
	})
	m.HandleFunc("POST /app/hubs/{id}/machines/{mid}/diagnose", func(w http.ResponseWriter, r *http.Request) {
		rt, ok := b.reachOf(r.PathValue("id"), r.PathValue("mid"))
		if !ok {
			writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this App does not know how to reach that computer", Suggestion: "pick its SSH host first"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		var lines []string
		stderr, _, err := b.Local.runScript(ctx, rt, diagScript, func(s string) { lines = append(lines, s) })
		if err != nil {
			writeRemoteErr(w, err, rt)
			return
		}
		found, hubs, log := parseDiag(lines)
		if found["end"] != "ok" {
			writeRemoteErr(w, &RemoteError{Code: "no_shell", Message: orStr(lastLines(stderr, 3), "it did not answer like a Linux or macOS shell"), Command: rt.TerminalCommand()}, rt)
			return
		}
		delete(found, "end")
		if hubs == nil {
			hubs = []DiagHub{}
		}
		if h, err := b.find(r.PathValue("id")); err == nil {
			for i := range hubs {
				hubs[i].Mine = sameHubURL(hubs[i].URL, h.URL)
			}
		}
		writeJSON(w, http.StatusOK, Diagnosis{Route: rt, Found: found, Hubs: hubs, Log: log})
	})
	// POST {action: restart|upgrade}: the output's last lines and whether it worked.
	m.HandleFunc("POST /app/hubs/{id}/machines/{mid}/fix", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &in) {
			return
		}
		script, ok := fixScripts[in.Action]
		if !ok {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "action is restart or upgrade"})
			return
		}
		rt, ok := b.reachOf(r.PathValue("id"), r.PathValue("mid"))
		if !ok {
			writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this App does not know how to reach that computer"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		var out []string
		_, code, err := b.Local.runScript(ctx, rt, script, func(s string) {
			out = append(out, maskSecrets(s))
			if len(out) > 30 {
				out = out[1:]
			}
		})
		if err != nil {
			writeRemoteErr(w, err, rt)
			return
		}
		if out == nil {
			out = []string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": code == 0, "log": out})
	})
}

func writeRemoteErr(w http.ResponseWriter, err error, rt Route) {
	var re *RemoteError
	if !errors.As(err, &re) {
		re = &RemoteError{Code: "unreachable", Message: err.Error(), Command: rt.TerminalCommand()}
	}
	// The command to try goes where the App looks for a suggestion.
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": re.Code, "message": re.Message, "suggestion": re.Command})
}

// validRoute checks a route from the App: ssh or wsl, and a plain name
// (never an option or anything a shell would read).
func validRoute(in *Route) bool {
	in.Target = strings.TrimSpace(in.Target)
	bad := in.Target == "" || strings.HasPrefix(in.Target, "-") || strings.ContainsAny(in.Target, " \t\r\n'\"`;|&$<>\\")
	return (in.Via == "ssh" || in.Via == "wsl") && !bad && in.Port >= 0 && in.Port <= 65535
}

// sameHubURL: a and b are the same hub (one may be a workspace under the
// other's /w/<name>).
func sameHubURL(a, b string) bool {
	n := func(s string) string { return strings.ToLower(strings.TrimRight(s, "/")) }
	a, b = n(a), n(b)
	return b != "" && (a == b || strings.HasPrefix(a, b+"/w/") || strings.HasPrefix(b, a+"/w/"))
}
