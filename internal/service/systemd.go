package service

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Systemd manages a systemd --user unit on Linux.
type Systemd struct {
	Home string
	R    Runner
	// ID names the service (default: the runner's).
	ID ID
	// User is the account the unit belongs to (default: the current one).
	User string
}

func (s *Systemd) user() string {
	if s.User != "" {
		return s.User
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// Linger tells whether systemd keeps this user's services running while
// they are logged out. Without it the runner stops a while after the last
// session ends: closing the last SSH or WSL window.
func (s *Systemd) Linger() bool {
	out, err := s.R.Run("loginctl", "show-user", s.user(), "--property=Linger")
	return err == nil && strings.Contains(out, "Linger=yes")
}

// LingerNote is what to run when Linger could not be turned on.
func (s *Systemd) LingerNote() string {
	return "the runner stops when you log out (closing the last SSH or WSL window): run sudo loginctl enable-linger " + s.user()
}

func (s *Systemd) unit() string { return s.ID.orDefault().Unit }

func (s *Systemd) Name() string { return "systemd" }

func (s *Systemd) file() string {
	return filepath.Join(s.Home, ".config", "systemd", "user", s.unit())
}

// quote escapes a value for an ExecStart/Environment line.
func quote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"\\'$%") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`)
	return `"` + r.Replace(v) + `"`
}

// Unit renders the service unit.
func (s *Systemd) Unit(sp Spec) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=" + s.ID.orDefault().Description + "\nAfter=network-online.target\n\n[Service]\n")
	parts := []string{quote(sp.Binary)}
	for _, a := range sp.Args {
		parts = append(parts, quote(a))
	}
	b.WriteString("ExecStart=" + strings.Join(parts, " ") + "\n")
	for _, k := range sortedKeys(sp.Env) {
		b.WriteString("Environment=" + quote(k+"="+sp.Env[k]) + "\n")
	}
	stop := "600"
	if s.ID.orDefault() == RunnerID {
		stop = "30"
	}
	b.WriteString("Restart=always\nRestartSec=5\nTimeoutStopSec=" + stop + "\nKillMode=mixed\n")
	b.WriteString("StandardOutput=append:" + sp.LogPath + "\nStandardError=append:" + sp.LogPath + "\n")
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func (s *Systemd) PlanInstall(sp Spec) (Plan, error) {
	cmds := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", s.unit()},
		// Queue the restart before returning; the runner cancels unfinished
		// work and its next process recovers the persisted tasks.
		{"systemctl", "--user", "restart", "--no-block", s.unit()},
	}
	// Keep it running after logout. Turning linger on for yourself needs no
	// password on most systems; when it does, the install says what to run.
	if !s.Linger() {
		cmds = append([][]string{{"-loginctl", "enable-linger", s.user()}}, cmds...)
	}
	return Plan{Manager: s.Name(), File: s.file(), Content: s.Unit(sp), Commands: cmds}, nil
}

func (s *Systemd) PlanUninstall() Plan {
	return Plan{Manager: s.Name(), File: s.file(), Commands: [][]string{
		{"-systemctl", "--user", "disable", "--now", s.unit()},
		{"-systemctl", "--user", "daemon-reload"},
	}}
}

func (s *Systemd) Status() (Status, error) {
	st := Status{Manager: s.Name(), File: s.file(), Installed: exists(s.file())}
	out, err := s.R.Run("systemctl", "--user", "show", s.unit(), "--property=ActiveState,SubState,MainPID")
	if err != nil {
		st.Detail = strings.TrimSpace(out)
		return st, nil
	}
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	st.Running = props["ActiveState"] == "active"
	st.PID, _ = strconv.Atoi(props["MainPID"])
	st.Detail = strings.Trim(props["ActiveState"]+"/"+props["SubState"], "/")
	return st, nil
}

func (s *Systemd) Restart() error {
	// Apply the bound to already-installed units too, before stopping an old
	// binary that still drains tasks indefinitely. Preserve args/environment.
	if s.ID.orDefault() == RunnerID {
		b, err := os.ReadFile(s.file())
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			re := regexp.MustCompile(`(?m)^TimeoutStopSec=.*$`)
			updated := re.ReplaceAllString(string(b), "TimeoutStopSec=30")
			if !re.Match(b) {
				updated = strings.Replace(updated, "[Service]\n", "[Service]\nTimeoutStopSec=30\n", 1)
			}
			if updated != string(b) {
				if err := os.WriteFile(s.file(), []byte(updated), 0o644); err != nil {
					return err
				}
				if _, err := s.R.Run("systemctl", "--user", "daemon-reload"); err != nil {
					return err
				}
			}
		}
	}
	_, err := s.R.Run("systemctl", "--user", "restart", s.unit())
	return err
}
