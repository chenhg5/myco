package service

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Launchd manages a per-user LaunchAgent on macOS.
type Launchd struct {
	Home string
	UID  int
	R    Runner
	// ID names the service (default: the runner's).
	ID ID
}

func (l *Launchd) label() string { return l.ID.orDefault().Label }

func (l *Launchd) Name() string { return "launchd" }

func (l *Launchd) file() string {
	return filepath.Join(l.Home, "Library", "LaunchAgents", l.label()+".plist")
}
func (l *Launchd) domain() string  { return fmt.Sprintf("gui/%d", l.UID) }
func (l *Launchd) service() string { return l.domain() + "/" + l.label() }

// Plist renders the LaunchAgent definition.
func (l *Launchd) Plist(s Spec) string {
	x := html.EscapeString
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + l.label() + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range append([]string{s.Binary}, s.Args...) {
		b.WriteString("    <string>" + x(a) + "</string>\n")
	}
	b.WriteString("  </array>\n  <key>EnvironmentVariables</key>\n  <dict>\n")
	for _, k := range sortedKeys(s.Env) {
		b.WriteString("    <key>" + x(k) + "</key><string>" + x(s.Env[k]) + "</string>\n")
	}
	b.WriteString(`  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ExitTimeOut</key><integer>600</integer>
  <key>StandardOutPath</key><string>` + x(s.LogPath) + `</string>
  <key>StandardErrorPath</key><string>` + x(s.LogPath) + `</string>
</dict>
</plist>
`)
	return b.String()
}

func (l *Launchd) PlanInstall(s Spec) (Plan, error) {
	return Plan{
		Manager: l.Name(), File: l.file(), Content: l.Plist(s),
		Commands: [][]string{
			{"-launchctl", "bootout", l.service()},
			{"launchctl", "bootstrap", l.domain(), l.file()},
		},
		Notes: []string{"starts now and at every login; restarts automatically if it exits"},
	}, nil
}

func (l *Launchd) PlanUninstall() Plan {
	return Plan{Manager: l.Name(), File: l.file(), Commands: [][]string{{"-launchctl", "bootout", l.service()}}}
}

var (
	launchdState = regexp.MustCompile(`(?m)^\s*state = (\S+)`)
	launchdPID   = regexp.MustCompile(`(?m)^\s*pid = (\d+)`)
)

func (l *Launchd) Status() (Status, error) {
	st := Status{Manager: l.Name(), File: l.file(), Installed: exists(l.file())}
	out, err := l.R.Run("launchctl", "print", l.service())
	if err != nil {
		st.Detail = "not loaded"
		return st, nil
	}
	if m := launchdState.FindStringSubmatch(out); m != nil {
		st.Detail = "state " + m[1]
		st.Running = m[1] == "running"
	}
	if m := launchdPID.FindStringSubmatch(out); m != nil {
		st.PID, _ = strconv.Atoi(m[1])
	}
	return st, nil
}

func (l *Launchd) Restart() error {
	_, err := l.R.Run("launchctl", "kickstart", "-k", l.service())
	return err
}
