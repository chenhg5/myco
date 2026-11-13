package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/skill"
)

// skillMarker marks a SKILL.md myco installed, so `myco upgrade` may
// replace it with the new version's (and never a file someone wrote).
const skillMarker = "<!-- installed by myco: `myco upgrade` keeps this file current; edits here are replaced -->"

// installedSkill is the skill as myco writes it: the embedded file with the
// marker after its front matter.
func installedSkill() string {
	s := skill.Myco
	if strings.HasPrefix(s, "---\n") {
		if i := strings.Index(s[4:], "\n---\n"); i >= 0 {
			end := 4 + i + len("\n---\n")
			return s[:end] + skillMarker + "\n" + s[end:]
		}
	}
	return skillMarker + "\n" + s
}

// skillTargets are the skill folders of agent CLIs on this machine (only
// those whose own folder exists): Claude Code and Codex read
// <home>/skills/<name>/SKILL.md.
func skillTargets(getenv func(string) string) []string {
	// The home folder from the command's own environment (tests pass
	// theirs), never one the process happens to have.
	home := ""
	if getenv != nil {
		home = getenv("HOME")
		if home == "" && runtime.GOOS == "windows" {
			home = getenv("USERPROFILE")
		}
	}
	var out []string
	for _, c := range []struct{ env, dir string }{{"CLAUDE_CONFIG_DIR", ".claude"}, {"CODEX_HOME", ".codex"}} {
		root := ""
		if getenv != nil {
			root = getenv(c.env)
		}
		if root == "" {
			if home == "" {
				continue
			}
			root = filepath.Join(home, c.dir)
		}
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			out = append(out, filepath.Join(root, "skills", "myco", "SKILL.md"))
		}
	}
	return out
}

// installSkill writes the skill to each target. refreshOnly updates files
// myco wrote before and creates none. A file without the marker (someone's
// own) is left alone.
func installSkill(targets []string, refreshOnly bool) (written, skipped []string, err error) {
	body := installedSkill()
	for _, path := range targets {
		old, rerr := os.ReadFile(path)
		switch {
		case errors.Is(rerr, fs.ErrNotExist):
			if refreshOnly {
				continue
			}
		case rerr != nil:
			return written, skipped, rerr
		case !strings.Contains(string(old), skillMarker):
			skipped = append(skipped, path)
			continue
		case string(old) == body:
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, skipped, err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
			return written, skipped, err
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return written, skipped, err
		}
		written = append(written, path)
	}
	return written, skipped, nil
}

func skillCommands() *Command {
	show := &Command{
		Name:    "show",
		Summary: "Print the myco skill for this version: how to receive, report progress, plan, ask and delegate",
		Long: `The guide an agent should read once after joining. It matches this binary, so it is
current after every myco upgrade. Agent CLIs with a skills folder (Claude Code, Codex) get it
installed there by myco join and myco skill install.`,
		Examples: []string{"myco skill show"},
		Run: handler(func(e *env) error {
			return e.p.Print(map[string]any{"version": Version, "skill": skill.Myco}, func(w io.Writer) {
				io.WriteString(w, skill.Myco)
			})
		}),
	}
	install := &Command{
		Name:     "install",
		Summary:  "Install the myco skill into the skills folder of each agent CLI on this machine",
		Mutating: true,
		Long: `Writes skills/myco/SKILL.md under ~/.claude (CLAUDE_CONFIG_DIR) and ~/.codex (CODEX_HOME), for
each that exists. myco join, myco upgrade and the runner (when it starts) do this too, so the
skill follows the installed version; a SKILL.md you wrote yourself is never replaced. A session that is already running may need a restart to see a new skill.`,
		Examples: []string{"myco skill install", "myco skill install --dir ~/.config/agent/skills/myco"},
		Flags: []Flag{
			{Name: "dir", Kind: String, Help: "Install into this folder instead (the file is <dir>/SKILL.md)"},
			{Name: "refresh", Kind: Bool, Help: "Only update copies myco installed before; create none"},
		},
		Run: handler(func(e *env) error {
			targets := skillTargets(e.in.Getenv)
			if d := strings.TrimSpace(e.in.String("dir")); d != "" {
				targets = []string{filepath.Join(d, "SKILL.md")}
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "skill_install", "targets": targets, "refresh": e.in.Bool("refresh")})
			}
			written, skipped, err := installSkill(targets, e.in.Bool("refresh"))
			if err != nil {
				return errorf("skill_install_failed", ExitError, "check the folder's permissions, or pass --dir", "%v", err)
			}
			return e.p.Print(map[string]any{"version": Version, "written": nonNilStrings(written), "skipped": nonNilStrings(skipped), "targets": nonNilStrings(targets)}, func(w io.Writer) {
				if len(targets) == 0 {
					fmt.Fprintln(w, "no agent CLI skills folder here (~/.claude or ~/.codex); read the skill with: myco skill show")
				}
				for _, p := range written {
					fmt.Fprintf(w, "installed the myco skill (%s): %s\n", Version, p)
				}
				for _, p := range skipped {
					fmt.Fprintf(w, "left alone (not written by myco): %s\n", p)
				}
				if len(targets) > 0 && len(written) == 0 && len(skipped) == 0 {
					fmt.Fprintln(w, "the myco skill is already current")
				}
			})
		}),
	}
	return (&Command{Name: "skill", Summary: "The guide that teaches an agent to use myco (built into this version)"}).Add(show, install)
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
