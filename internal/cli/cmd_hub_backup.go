package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/backup"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
)

// hubBackupCommand makes a backup: on the server, of the whole server
// from its data (--db); from anywhere, as the hub's owner, a new one on
// the hub, downloaded (--download).
func hubBackupCommand() *Command {
	return &Command{
		Name:    "backup",
		Summary: "Back up a hub's data: on the server (--db), or download a new backup as its owner (--download)",
		Long: `A backup holds the hub's database (a consistent snapshot, taken while it runs) and its files
(attachments, pages, apps), checked before it is written. Hubs also make one a day by themselves and
keep the last 7 next to their database (myco hub serve --backups).

On the server, --db backs up the whole server: the hub and every workspace in it. Run it as root on a
server installed with install-hub.sh (its data is /var/lib/myco). --download makes a new backup of
the hub at --hub (or of one workspace, at its /w/<name> address) and saves it here: only the hub's
owner can (the admin token, MYCO_ADMIN_TOKEN or --admin-token-file). Keep backups on another computer:
a backup on the server's own disk does not survive that disk. Put one back with: myco hub restore.`,
		Examples: []string{
			"sudo myco hub backup --db /var/lib/myco/hub.db --out /root/backups/",
			"myco hub backup --download --out ~/myco-backups/ --admin-token-file hub.env",
		},
		Mutating: true,
		Flags: []Flag{
			{Name: "db", Kind: String, Help: "The hub's database, on the server: back up the whole server from it"},
			{Name: "download", Kind: Bool, Help: "Make a new backup on the hub and download it (its owner)"},
			{Name: "out", Kind: String, Default: ".", Help: "Where to write it: a directory, or a file name ending in .tar.gz"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (with --download)"},
		},
		Run: handler(func(e *env) error {
			db, download := e.in.String("db"), e.in.Bool("download")
			if (db == "") == !download {
				return errorf("invalid_argument", ExitUsage, "on the server: --db /var/lib/myco/hub.db; elsewhere, as the hub's owner: --download",
					"say which hub: --db (on the server) or --download")
			}
			out := e.in.String("out")
			if download {
				if e.in.Bool("dry-run") {
					return e.dryRun(map[string]any{"action": "hub_backup_download", "out": out})
				}
				c, err := e.adminClient()
				if err != nil {
					return err
				}
				b, err := c.CreateBackup(e.in.Ctx)
				if err != nil {
					return err
				}
				dir := out
				if strings.HasSuffix(out, backup.Suffix) {
					dir = filepath.Dir(out)
				}
				path, err := c.DownloadBackup(e.in.Ctx, b.Name, dir)
				if err != nil {
					return err
				}
				if strings.HasSuffix(out, backup.Suffix) && path != out {
					if err := os.Rename(path, out); err != nil {
						return err
					}
					path = out
				}
				res := map[string]any{"path": path, "name": b.Name, "bytes": b.Size}
				return e.p.Print(res, func(w io.Writer) { fmt.Fprintf(w, "saved %s (%s)\n", path, humanBytes(b.Size)) })
			}
			if _, err := os.Stat(db); err != nil {
				return errorf("not_found", ExitNotFound, "give the hub's database, e.g. /var/lib/myco/hub.db", "no database at %s", db)
			}
			srcs := backup.Sources(db, "", true)
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "hub_backup", "db": db, "hubs": len(srcs), "out": out})
			}
			now := time.Now()
			dir, path := out, ""
			if strings.HasSuffix(out, backup.Suffix) {
				dir, path = filepath.Dir(out), out
			}
			info, err := backup.Write(e.in.Ctx, dir, srcs, "server", Version, now)
			if err != nil {
				return errorf("backup_failed", ExitError, "check the disk space and that you can read the hub's data (sudo)", "backup failed: %v", err)
			}
			got := filepath.Join(dir, info.Name)
			if path != "" && path != got {
				if err := os.Rename(got, path); err != nil {
					return err
				}
				got = path
			}
			res := map[string]any{"path": got, "hubs": len(srcs), "bytes": info.Size}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "saved %s (%s; the hub and %d workspace(s))\n", got, humanBytes(info.Size), len(srcs)-1)
				fmt.Fprintln(w, "copy it to another computer: a backup on this disk does not survive the disk")
			})
		}),
	}
}

// hubRestoreCommand puts a backup back, on the server.
func hubRestoreCommand() *Command {
	return &Command{
		Name:    "restore",
		Summary: "Put a backup back in place of a hub's data (on the server)",
		Long: `Checks the whole backup first (every entry against its checksum, every database's integrity), then
replaces the data next to --db with it. What was there is kept in before-restore-<time>/ next to it,
not deleted. A backup of a whole server (myco hub backup --db) restores the hub and its workspaces;
a hub's own backup (from the App, or a daily one) restores that hub; for a workspace, give its
database: --db /var/lib/myco/workspaces/<name>/hub.db.

The hub must not run meanwhile. On a server installed with install-hub.sh, run it as root: it stops the
hub's service and starts it again. Elsewhere stop the hub yourself (myco hub down) and pass --yes.`,
		Examples: []string{
			"sudo myco hub restore --from /root/backups/myco-backup-server-20261121-093000.tar.gz --db /var/lib/myco/hub.db",
			"myco hub restore --from myco-backup-team-20261121-093000.tar.gz --db /var/lib/myco/workspaces/team/hub.db --dry-run",
		},
		Mutating: true,
		Flags: []Flag{
			{Name: "from", Kind: String, Required: true, Help: "The backup (.tar.gz)"},
			{Name: "db", Kind: String, Required: true, Help: "The database of the hub to restore, e.g. /var/lib/myco/hub.db"},
			{Name: "yes", Kind: Bool, Help: "The hub is stopped (needed when it does not run as a system service here)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("from", "db"); err != nil {
				return err
			}
			from, db := e.in.String("from"), e.in.String("db")
			dataDir := filepath.Dir(db)
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				return err
			}
			f, err := os.Open(from)
			if err != nil {
				return errorf("not_found", ExitNotFound, "", "cannot open the backup: %v", err)
			}
			defer f.Close()
			tmp, err := os.MkdirTemp(dataDir, ".restore-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			m, err := backup.Extract(e.in.Ctx, f, tmp)
			if err != nil {
				return errorf("invalid_backup", ExitError, "use another backup", "%v", err)
			}
			unit := hubUnitFor(dataDir)
			plan := map[string]any{"backup": filepath.Base(from), "made": m.CreatedAt, "scope": m.Scope, "hubs": len(m.Hubs), "files": m.Files, "into": dataDir, "service": unit}
			if e.in.Bool("dry-run") {
				plan["action"] = "hub_restore"
				return e.dryRun(plan)
			}
			if unit == "" && !e.in.Bool("yes") {
				return errorf("confirm", ExitUsage, "stop the hub (myco hub down, or its service), then run this again with --yes",
					"the hub's service was not found here, so it cannot be stopped for you")
			}
			if unit != "" {
				if out, err := exec.Command("systemctl", "stop", unit).CombinedOutput(); err != nil {
					return errorf("service", ExitError, "run it as root (sudo)", "cannot stop %s: %v: %s", unit, err, strings.TrimSpace(string(out)))
				}
			}
			aside, rerr := backup.Restore(tmp, dataDir, m, time.Now())
			if unit != "" {
				if out, err := exec.Command("systemctl", "start", unit).CombinedOutput(); err != nil && rerr == nil {
					rerr = fmt.Errorf("restored, but %s did not start again: %v: %s", unit, err, strings.TrimSpace(string(out)))
				}
			}
			if rerr != nil {
				return errorf("restore_failed", ExitError, "what was there is in before-restore-*/ next to the database", "%v", rerr)
			}
			plan["kept_aside"] = aside
			return e.p.Print(plan, func(w io.Writer) {
				fmt.Fprintf(w, "restored %s (made %s) into %s\n", filepath.Base(from), m.CreatedAt.Local().Format("2006-01-02 15:04"), dataDir)
				for _, a := range aside {
					fmt.Fprintf(w, "what was there: %s\n", a)
				}
				if unit != "" {
					fmt.Fprintf(w, "%s started again\n", unit)
				}
			})
		}),
	}
}

var unitDB = regexp.MustCompile(`--db\s+(\S+)`)

// hubUnitFor is the system service (install-hub.sh's myco-hub*) whose hub
// keeps its data in dataDir or has it as a workspace; "" if none.
func hubUnitFor(dataDir string) string {
	want := realDir(dataDir)
	units, _ := filepath.Glob("/etc/systemd/system/myco-hub*.service")
	for _, u := range units {
		fh, err := os.Open(u)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "ExecStart=") {
				continue
			}
			if m := unitDB.FindStringSubmatch(line); m != nil {
				dir := realDir(filepath.Dir(m[1]))
				if want == dir || strings.HasPrefix(want, dir+string(filepath.Separator)) {
					fh.Close()
					return filepath.Base(u)
				}
			}
		}
		fh.Close()
	}
	return ""
}

func realDir(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
