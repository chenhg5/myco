package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// caddySitesDir is where install-hub.sh keeps one Caddy site per hub.
const caddySitesDir = "/etc/caddy/myco.d"

// oldCaddySite is a site file exactly as install-hub.sh wrote it before the
// hub's waits (a new session's first turn: up to 5 minutes) outgrew its
// 120s proxy timeout. Only such files are changed: one edited by hand is
// someone's own and is left alone.
var oldCaddySite = regexp.MustCompile(`\A[^\s{}]+ \{\n\treverse_proxy 127\.0\.0\.1:[0-9]+ \{\n\t\ttransport http \{\n\t\t\tread_timeout 120s\n\t\t\}\n\t\}\n\}\n\z`)

// raiseCaddyTimeouts rewrites install-hub.sh's own old site files in dir to
// the 360s timeout it writes now. It returns the files it changed and their
// previous contents, so a caller can put them back.
func raiseCaddyTimeouts(dir string) (changed []string, before map[string][]byte, err error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.caddy"))
	before = map[string][]byte{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil || !oldCaddySite.Match(b) {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		next := strings.Replace(string(b), "read_timeout 120s", "read_timeout 360s", 1)
		if err := os.WriteFile(p, []byte(next), st.Mode().Perm()); err != nil {
			restoreFiles(before)
			return nil, nil, err
		}
		changed = append(changed, p)
		before[p] = b
	}
	return changed, before, nil
}

func restoreFiles(before map[string][]byte) {
	for p, b := range before {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// upgradeCaddySites, on a server set up by install-hub.sh (as root), raises
// the proxy timeout of the hub's own Caddy sites and reloads Caddy; when
// Caddy does not accept the result, the files are put back.
func upgradeCaddySites() (restarted, notes []string) {
	changed, before, err := raiseCaddyTimeouts(caddySitesDir)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not update the Caddy sites in %s: %v", caddySitesDir, err)}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	if b, err := exec.Command("caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile").CombinedOutput(); err != nil {
		restoreFiles(before)
		return nil, []string{fmt.Sprintf("Caddy did not accept the longer proxy timeout, so its sites were left as they were: %s", strings.TrimSpace(string(b)))}
	}
	if b, err := exec.Command("systemctl", "reload", "caddy").CombinedOutput(); err != nil {
		return nil, []string{fmt.Sprintf("raised the proxy timeout to 360s in %s, but reloading Caddy failed: %v %s", strings.Join(changed, ", "), err, strings.TrimSpace(string(b)))}
	}
	return []string{"caddy (proxy timeout 360s)"}, nil
}
