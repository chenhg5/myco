package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// appLinkURL is the myco:// address that hands an app link to the myco
// app on this computer: it opens (or comes forward) and offers to add the
// workspace, so nobody has to copy and paste the link.
func appLinkURL(link string) string { return "myco://add-hub?link=" + url.QueryEscape(link) }

// openAppLink gives an app link to the myco app on this computer. On Linux
// the app registers no myco:// handler; there, or when nothing handles
// myco://, it says so and the link is pasted by hand.
var openAppLink = func(link string) error {
	u := appLinkURL(link)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u) // fails when no app handles myco://
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		return errors.New("the myco app cannot take a link here: paste it into the app (加入已有的工作区)")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New("could not open the myco app (is it installed?): " + msg)
	}
	return nil
}

// loopbackLink reports whether a link names this computer only
// (127.0.0.1, localhost, ::1): other computers cannot use it.
func loopbackLink(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "localhost" || strings.HasPrefix(h, "127.") || h == "::1"
}

// loopbackWarning is what to say about such a link: a hub on this computer
// made it because it found no local-network address to name instead.
const loopbackWarning = "this link names 127.0.0.1, so only this computer can use it: the hub found no local-network address for this computer. " +
	"Run the hub with MYCO_LAN_ADDR=<this computer's address> (for myco hub up: MYCO_LAN_ADDR=<address> myco hub up …, which restarts it), or publish it with a tunnel (myco hub up --tunnel cloudflare|tailscale)"

// warnLoopbackLink adds that warning to a command's output (and says it on
// stderr, where a person reading text output sees it).
func warnLoopbackLink(e *env, out map[string]any, link string) {
	if !loopbackLink(link) {
		return
	}
	out["warnings"] = append(asStrings(out["warnings"]), loopbackWarning)
	fmt.Fprintln(e.in.Stderr, "warning: "+loopbackWarning)
}

func asStrings(v any) []string {
	s, _ := v.([]string)
	return s
}
