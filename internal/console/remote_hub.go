package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

// Setting up a workspace (a hub) on a server over SSH, from the App: log in
// and look around, run deploy/install-hub.sh there (or keep the hub that is
// already there), make an App link on the server, and add the workspace to
// this App with it. No link or token is ever shown or typed.

// hubProbeScript: the system, whether it can act as root without a
// password, and whether a hub is installed already.
const hubProbeScript = `cd "$HOME" 2>/dev/null
p() { echo "myco-probe $1 $2"; }
p os "$(uname -s 2>/dev/null)"
p host "$( (hostname || uname -n) 2>/dev/null | head -n1)"
if [ "$(id -u)" = 0 ]; then p root yes; else p root no; fi
command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null && p sudo yes || p sudo no
command -v curl >/dev/null 2>&1 && p curl yes || p curl no
command -v systemctl >/dev/null 2>&1 && p systemd yes || p systemd no
[ -f /etc/myco/hub.env ] && p hub yes || p hub no
p end ok
`

// hubInstallScript installs the hub (automatic HTTPS on <ip>.sslip.io)
// unless one is there, then makes an App link for device and prints it on
// a line of its own.
func hubInstallScript(install bool, sudo, device string) string {
	var b strings.Builder
	b.WriteString(`cd "$HOME" 2>/dev/null
set -e
`)
	if install {
		b.WriteString(`curl -fsSL --retry 3 https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-hub.sh | ` + sudo + `bash -s -- --domain auto --yes 2>&1
`)
	}
	b.WriteString(`M=/usr/local/bin/myco; [ -x "$M" ] || M="$(command -v myco)"
L="$(` + sudo + `"$M" console link --name ` + shellQuote(device) + ` --format json)"
printf 'myco-applink %s\n' "$(printf '%s' "$L" | tr -d '\n')"
`)
	return b.String()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// secretInLine hides app links and join links in what the installer prints.
var secretInLine = regexp.MustCompile(`(/c/|/j/)[A-Za-z0-9_\-]+`)

func (b *Backend) hubRoutes(m *http.ServeMux) {
	// POST {target, port}: set up (or reach) a workspace on that server.
	m.HandleFunc("POST /app/local/hub-over-ssh", func(w http.ResponseWriter, r *http.Request) {
		var in Route
		if !decode(w, r, &in) {
			return
		}
		in.Via = "ssh"
		if !validRoute(&in) {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "give the server as an SSH name or user@host"})
			return
		}
		job := &ConnectJob{ID: newJobID(), Route: in, Steps: []ConnectStep{
			{Key: "login", State: "running"}, {Key: "install", State: "pending"}, {Key: "add", State: "pending"}}}
		b.connects.put(job)
		go b.runHubOver(job.ID, in)
		j, _ := b.connects.get(job.ID)
		writeJSON(w, http.StatusOK, j)
	})
}

func (b *Backend) runHubOver(id string, rt Route) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	step := func(i int, state, detail string) {
		b.connects.update(id, func(j *ConnectJob) { j.Steps[i].State, j.Steps[i].Detail = state, detail })
	}
	fail := func(i int, e *RemoteError) {
		b.connects.update(id, func(j *ConnectJob) { j.Steps[i].State, j.Error, j.Done = "failed", e, true })
	}
	asRemote := func(err error) *RemoteError {
		var re *RemoteError
		if errors.As(err, &re) {
			return re
		}
		return &RemoteError{Code: "unreachable", Message: err.Error(), Command: rt.TerminalCommand()}
	}

	// 1. Log in and look around.
	var lines []string
	lctx, lcancel := context.WithTimeout(ctx, 40*time.Second)
	stderr, _, err := b.Local.runScript(lctx, rt, hubProbeScript, func(s string) { lines = append(lines, s) })
	lcancel()
	if err != nil {
		fail(0, asRemote(err))
		return
	}
	found := parseProbe(lines)
	if found["end"] != "ok" {
		fail(0, &RemoteError{Code: "no_shell", Message: orStr(lastLines(stderr, 3), "it did not answer like a Linux shell"), Command: rt.TerminalCommand()})
		return
	}
	b.connects.update(id, func(j *ConnectJob) { j.Found = found })
	existing := found["hub"] == "yes"
	switch {
	case !strings.EqualFold(found["os"], "linux") || found["systemd"] != "yes":
		fail(0, &RemoteError{Code: "unsupported", Message: "a workspace is installed on a Linux server with systemd; it runs " + orStr(found["os"], "an unknown system")})
		return
	case found["curl"] != "yes" && !existing:
		fail(0, &RemoteError{Code: "missing_curl", Message: "curl is not installed there"})
		return
	case found["root"] != "yes" && found["sudo"] != "yes":
		// sudo would ask for a password nobody can type here.
		fail(0, &RemoteError{Code: "needs_sudo", Message: "installing needs root, and sudo asks for a password there",
			Command: "ssh -t " + rt.Target + " 'curl -fsSL https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-hub.sh | sudo bash -s -- --domain auto'"})
		return
	}
	sudo := ""
	if found["root"] != "yes" {
		sudo = "sudo -n "
	}
	detail := found["host"]
	if existing {
		detail = "existing"
	}
	step(0, "done", detail)

	// 2. Install the hub (or keep the one there) and make an App link.
	step(1, "running", "")
	var link string
	_, code, err := b.Local.runScript(ctx, rt, hubInstallScript(!existing, sudo, b.deviceName()), func(s string) {
		if rest, ok := strings.CutPrefix(s, "myco-applink "); ok {
			var out struct {
				Link string `json:"link"`
			}
			if json.Unmarshal([]byte(rest), &out) == nil {
				link = out.Link
			}
			return
		}
		s = secretInLine.ReplaceAllString(s, "$1***")
		b.connects.update(id, func(j *ConnectJob) {
			j.Log = append(j.Log, s)
			if len(j.Log) > 30 {
				j.Log = j.Log[len(j.Log)-30:]
			}
		})
	})
	if err != nil {
		fail(1, asRemote(err))
		return
	}
	if code != 0 || link == "" {
		fail(1, &RemoteError{Code: "install_failed", Message: "the install stopped", Command: rt.TerminalCommand()})
		return
	}
	step(1, "done", "")

	// 3. Add it to this App.
	step(2, "running", "")
	u, tok, err := ParseAppLink(link)
	if err != nil {
		fail(2, &RemoteError{Code: "install_failed", Message: "the server gave no usable App link"})
		return
	}
	actx, acancel := context.WithTimeout(ctx, time.Minute)
	st, _, err := b.addHubWith(actx, "", u, tok)
	acancel()
	if err != nil {
		// Most often: the new address is not reachable from here yet
		// (DNS, a firewall in front of ports 80 and 443).
		fail(2, &RemoteError{Code: "hub_unreachable", Message: err.Error(), Command: "curl -I " + u + "/v1/health"})
		return
	}
	b.connects.update(id, func(j *ConnectJob) {
		j.Steps[2].State, j.Steps[2].Detail, j.HubID, j.Done = "done", u, st.Hub.ID, true
	})
}
