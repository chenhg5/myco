package service

import (
	"path/filepath"
	"strconv"
	"strings"
)

// Systemd manages a systemd --user unit on Linux.
type Systemd struct {
	Home string
	R    Runner
	// ID names the service (default: the runner's).
	ID ID
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
	b.WriteString("Restart=always\nRestartSec=5\nTimeoutStopSec=600\nKillMode=mixed\n")
	b.WriteString("StandardOutput=append:" + sp.LogPath + "\nStandardError=append:" + sp.LogPath + "\n")
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func (s *Systemd) PlanInstall(sp Spec) (Plan, error) {
	return Plan{
		Manager: s.Name(), File: s.file(), Content: s.Unit(sp),
		Commands: [][]string{
			{"systemctl", "--user", "daemon-reload"},
			{"systemctl", "--user", "enable", s.unit()},
			{"systemctl", "--user", "restart", s.unit()},
		},
		Notes: []string{"to keep it running after you log out: sudo loginctl enable-linger $USER"},
	}, nil
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
	_, err := s.R.Run("systemctl", "--user", "restart", s.unit())
	return err
}
