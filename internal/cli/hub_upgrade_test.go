package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/domain"
)

// On a server with install-hub.sh's path unit, the hub leaves a request
// in its state directory (the root unit upgrades and restarts it); a hub
// under no service manager says how to do it instead.
func TestHubUpgrader(t *testing.T) {
	dir := t.TempDir()
	unit := filepath.Join(dir, "myco-upgrade.path")
	was := upgradePathUnit
	upgradePathUnit = unit
	defer func() { upgradePathUnit = was }()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	stopped := false
	up := hubUpgrader(getenv, filepath.Join(dir, "hub.db"), func() { stopped = true })

	if _, err := up(context.Background(), "v9.9.9"); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("no unit, no service manager: %v", err)
	}
	if err := os.WriteFile(unit, []byte(upgradePathContent), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := up(context.Background(), "9.9.9")
	if err != nil || res.Via != "service" || res.To != "v9.9.9" || res.From != Version {
		t.Fatalf("service: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, upgradeRequestFile)); strings.TrimSpace(string(b)) != "v9.9.9" {
		t.Fatalf("request file: %q", b)
	}
	if stopped {
		t.Fatal("the root service restarts it: the hub must not stop itself")
	}
}

// install-hub.sh writes the same units `sudo myco upgrade` keeps (so it
// does not rewrite them on every upgrade).
func TestInstallHubWritesTheUpgradeUnits(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "deploy", "install-hub.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows checkouts may turn the script's line ends into CRLF.
	script := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, want := range []string{upgradePathContent, upgradeServiceContent} {
		if !strings.Contains(script, want) {
			t.Fatalf("install-hub.sh does not write:\n%s", want)
		}
	}
}
