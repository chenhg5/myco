package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/release"
	"github.com/chenhg5/myco/internal/service"
)

// EnvUpgradeTarget replaces a different file than the running binary
// (packagers, tests).
const EnvUpgradeTarget = "MYCO_UPGRADE_TARGET"

func upgradeCommand() *Command {
	return &Command{
		Name:     "upgrade",
		Summary:  "Upgrade this machine's myco to the latest release and restart its services",
		Mutating: true,
		Long: `Downloads the release for this OS and architecture, checks it against the release's SHA256SUMS,
replaces this myco binary atomically and restarts the services on this machine that run it: the
runner service, a hub started with 'myco hub up', and (when run with sudo on a server) the system
myco-* services such as myco-hub from install-hub.sh. The hub keeps its database and tokens; its
schema migrates on start. Nothing is upgraded automatically: run this when 'myco version',
'myco doctor' or a notice says a new release is out.`,
		Examples: []string{
			"myco upgrade --check              # is there a newer release?",
			"myco upgrade                      # upgrade and restart this machine's runner / hub",
			"sudo myco upgrade                 # on a server installed with install-hub.sh",
			"myco upgrade --version v0.16.5    # install a specific release (also to roll back)",
		},
		Flags: []Flag{
			{Name: "check", Kind: Bool, Help: "Only report whether a newer release exists"},
			{Name: "version", Kind: String, Help: "Release to install, e.g. v0.16.5 (default: latest)"},
			{Name: "no-restart", Kind: Bool, Help: "Replace the binary but do not restart services"},
		},
		Run: handler(func(e *env) error {
			hc := &http.Client{Timeout: 30 * time.Minute} // slow links: 15 MB at 40 KB/s is ~7 minutes
			target := e.in.String("version")
			if target != "" {
				if _, ok := release.Parse(target); !ok {
					return errorf("invalid_argument", ExitUsage, "use a release tag such as v0.16.5", "not a release version: %q", target)
				}
				if !strings.HasPrefix(target, "v") {
					target = "v" + target
				}
			} else {
				ctx, cancel := context.WithTimeout(e.in.Ctx, 20*time.Second)
				latest, err := release.Latest(ctx, &http.Client{Timeout: 20 * time.Second}, release.APIURL(e.in.Getenv))
				cancel()
				if err != nil {
					return errorf("unavailable", ExitUnavailable, "check the network, or pass --version vX.Y.Z", "cannot find the latest release: %v", err)
				}
				target = latest
			}
			_, known := release.Parse(Version)
			newer := release.Newer(target, Version)
			out := map[string]any{"current": Version, "target": target, "update_available": newer}
			if e.in.Bool("check") {
				return e.p.Print(out, func(w io.Writer) {
					switch {
					case !known:
						fmt.Fprintf(w, "this is a development build (%s); the latest release is %s\n", Version, target)
					case newer:
						fmt.Fprintf(w, "myco %s is available (this machine has %s): run `myco upgrade`\n", target, Version)
					default:
						fmt.Fprintf(w, "myco %s is up to date\n", Version)
					}
				})
			}
			if e.in.String("version") == "" && !newer {
				if !known {
					return errorf("invalid_argument", ExitUsage, "pass --version "+target+" to install that release",
						"this is a development build (%s); not replacing it automatically", Version)
				}
				return e.p.Print(out, func(w io.Writer) { fmt.Fprintf(w, "myco %s is up to date\n", Version) })
			}
			path, err := upgradeTarget(e.in.Getenv)
			if err != nil {
				return err
			}
			release.CleanupOld(path)
			out["path"] = path
			if e.in.Bool("dry-run") {
				return e.dryRun(out)
			}
			fmt.Fprintf(e.in.Stderr, "==> downloading myco %s for %s/%s (checked against SHA256SUMS; on a slow link this takes a few minutes)\n", target, runtime.GOOS, runtime.GOARCH)
			bin, err := release.Fetch(e.in.Ctx, hc, release.DownloadURL(e.in.Getenv), target, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return errorf("upgrade_failed", ExitError, "nothing was changed; try again, or pass --version", "%v", err)
			}
			if err := release.Replace(path, bin); err != nil {
				if errors.Is(err, release.ErrNotWritable) {
					return errorf("permission_denied", ExitPermission, "run it with sudo: sudo myco upgrade", "cannot replace %s: %v", path, err)
				}
				return err
			}
			var restarted, notes []string
			switch {
			case e.in.Bool("no-restart"):
				notes = append(notes, "services not restarted (--no-restart): restart them to use the new version")
			case e.in.Getenv("MYCO_MESSAGE_ID") != "":
				// Running inside a runner-driven agent turn: restarting the
				// runner would kill this very turn.
				notes = append(notes, "running inside an agent turn, so services were not restarted: run `myco runner service restart` (and restart the hub) when this turn is done")
			default:
				restarted, notes = restartMycoServices()
			}
			out["restarted"], out["notes"] = restarted, notes
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "upgraded %s: %s -> %s\n", path, Version, target)
				for _, r := range restarted {
					fmt.Fprintf(w, "restarted %s\n", r)
				}
				for _, n := range notes {
					fmt.Fprintln(w, n)
				}
			})
		}),
	}
}

// upgradeTarget is the binary to replace: this executable (symlinks
// resolved) unless MYCO_UPGRADE_TARGET says otherwise.
func upgradeTarget(getenv func(string) string) (string, error) {
	if t := strings.TrimSpace(getenv(EnvUpgradeTarget)); t != "" {
		return filepath.Abs(t)
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(self); err == nil {
		self = real
	}
	return self, nil
}

// restartMycoServices restarts what runs myco on this machine: the user's
// runner and `hub up` services, and, as root on systemd, active myco-*
// system units (install-hub.sh's myco-hub, exec nodes, ...).
func restartMycoServices() (restarted, notes []string) {
	if home, err := os.UserHomeDir(); err == nil {
		for _, id := range []service.ID{service.RunnerID, service.HubID} {
			m, err := service.DetectID(home, service.ExecRunner{}, id)
			if err != nil {
				continue
			}
			st, err := m.Status()
			if err != nil || !st.Installed || !st.Running {
				continue
			}
			if err := m.Restart(); err != nil {
				notes = append(notes, fmt.Sprintf("could not restart %s: %v", id.Description, err))
			} else {
				restarted = append(restarted, id.Description)
			}
		}
	}
	if runtime.GOOS == "windows" {
		// No service manager integration on Windows yet: the runner and hub
		// run in a terminal, and keep the old binary until restarted.
		return nil, []string{"restart any running `myco runner start` / `myco hub` processes to use the new version"}
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if _, err := exec.LookPath("systemctl"); err == nil {
			out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=active", "--plain", "--no-legend", "myco*").Output()
			if err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					unit := strings.Fields(line)
					if len(unit) == 0 || !strings.HasSuffix(unit[0], ".service") {
						continue
					}
					if b, err := exec.Command("systemctl", "restart", unit[0]).CombinedOutput(); err != nil {
						notes = append(notes, fmt.Sprintf("could not restart %s: %v %s", unit[0], err, strings.TrimSpace(string(b))))
					} else {
						restarted = append(restarted, unit[0])
					}
				}
			}
		}
	} else if runtime.GOOS == "linux" {
		if _, err := os.Stat("/etc/systemd/system/myco-hub.service"); err == nil {
			notes = append(notes, "a system myco-hub service exists: run `sudo myco upgrade` (or `sudo systemctl restart myco-hub`) to restart it")
		}
	}
	if len(restarted) == 0 && len(notes) == 0 {
		notes = append(notes, "no running myco services found here; restart any myco processes you started by hand")
	}
	return restarted, notes
}
