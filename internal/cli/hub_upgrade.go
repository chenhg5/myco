package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/release"
)

// A hub upgraded from the App (POST /v1/admin/upgrade). A hub that
// install-hub.sh set up runs as an unprivileged systemd user and cannot
// replace /usr/local/bin/myco: it leaves a request in its state directory,
// and a root path unit (installed by install-hub.sh, or by `sudo myco
// upgrade` for hubs set up before) runs `myco upgrade`, which restarts it.
// A hub under the user's own service manager (myco hub up: launchd or
// systemd --user) replaces its own binary and stops; the service manager
// starts it again on the new one.

// (variables for tests)
var (
	upgradePathUnit    = "/etc/systemd/system/myco-upgrade.path"
	upgradeServiceUnit = "/etc/systemd/system/myco-upgrade.service"
)

const upgradeRequestFile = "upgrade.request"

const upgradePathContent = `[Unit]
Description=myco: upgrade when a hub asks for it (from the App)

[Path]
PathExistsGlob=/var/lib/private/myco*/upgrade.request
PathExistsGlob=/var/lib/myco*/upgrade.request

[Install]
WantedBy=multi-user.target
`

const upgradeServiceContent = `[Unit]
Description=myco: upgrade asked for by a hub

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'rm -f /var/lib/private/myco*/upgrade.request /var/lib/myco*/upgrade.request; exec /usr/local/bin/myco upgrade'
`

// hubUpgrader is the hub's POST /v1/admin/upgrade for a hub with its
// database at dbPath.
func hubUpgrader(getenv func(string) string, dbPath string, stop func()) func(ctx context.Context, version string) (api.HubUpgrade, error) {
	return func(ctx context.Context, version string) (api.HubUpgrade, error) {
		hc := &http.Client{Timeout: 30 * time.Minute}
		target := version
		if target == "" {
			lctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			latest, err := release.Latest(lctx, hc, release.APIURL(getenv))
			cancel()
			if err != nil {
				return api.HubUpgrade{}, domain.Errf(domain.CodeInvalidState, "cannot find the latest release: %v", err)
			}
			target = latest
			if !release.Newer(target, Version) {
				return api.HubUpgrade{}, domain.Errf(domain.CodeInvalidState, "the hub already runs %s, the latest release", Version)
			}
		} else if !strings.HasPrefix(target, "v") {
			target = "v" + target
		}
		up := api.HubUpgrade{From: Version, To: target}
		// A root service of the server does it.
		if _, err := os.Stat(upgradePathUnit); err == nil {
			req := filepath.Join(filepath.Dir(dbPath), upgradeRequestFile)
			if err := os.WriteFile(req, []byte(target+"\n"), 0o600); err != nil {
				return api.HubUpgrade{}, domain.Errf(domain.CodeInternal, "cannot ask the server to upgrade: %v", err)
			}
			up.Via = "service"
			return up, nil
		}
		// The hub itself, under a service manager that starts it again.
		if getenv("INVOCATION_ID") == "" && !strings.HasPrefix(getenv("XPC_SERVICE_NAME"), "ai.myco.") {
			return api.HubUpgrade{}, domain.Errf(domain.CodeInvalidState, "this hub does not run as a service, so it cannot restart itself").
				WithSuggestion("run sudo myco upgrade on the server")
		}
		exe, err := os.Executable()
		if err != nil {
			return api.HubUpgrade{}, err
		}
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		bin, err := release.FetchFrom(ctx, hc, release.DownloadURLs(getenv), target, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return api.HubUpgrade{}, domain.Errf(domain.CodeInvalidState, "downloading myco %s: %v", target, err)
		}
		if err := release.Replace(exe, bin); err != nil {
			if errors.Is(err, release.ErrNotWritable) {
				return api.HubUpgrade{}, domain.Errf(domain.CodePermissionDenied, "the hub cannot replace %s", exe).
					WithSuggestion("run sudo myco upgrade on the server once; after that the App can do it")
			}
			return api.HubUpgrade{}, err
		}
		up.Via = "self"
		// Stop once the answer is out; the service manager starts the new one.
		time.AfterFunc(1500*time.Millisecond, stop)
		return up, nil
	}
}

// ensureUpgradeUnits installs the root units that upgrade a hub asked from
// the App, on a server with install-hub.sh's hub (as root, from `myco
// upgrade`). It reports what it did, or why not.
func ensureUpgradeUnits() (done []string, notes []string) {
	hubs, _ := filepath.Glob("/etc/systemd/system/myco-hub*.service")
	if len(hubs) == 0 {
		return nil, nil
	}
	changed := false
	for _, f := range []struct{ path, content string }{{upgradePathUnit, upgradePathContent}, {upgradeServiceUnit, upgradeServiceContent}} {
		if b, err := os.ReadFile(f.path); err == nil && string(b) == f.content {
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return nil, []string{fmt.Sprintf("could not let the App upgrade this hub: %v", err)}
		}
		changed = true
	}
	if !changed {
		return nil, nil
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "myco-upgrade.path"}} {
		if b, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return nil, []string{fmt.Sprintf("could not let the App upgrade this hub: systemctl %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))}
		}
	}
	return []string{"myco-upgrade.path (the App can now upgrade this hub)"}, nil
}
