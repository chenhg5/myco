package runner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// What a task's run changed in its working directory, shown under the
// turn in the app (docs/design/client-plan.md 5.5). Before the run the
// working tree is recorded with `git stash create`, which writes a commit
// object and changes no file, index or ref; after it, the tree is compared
// with that record. New untracked files count as added. Directories that
// are not git work trees are not looked at.

const (
	changesMaxFiles = 50
	changesMaxDiff  = 12000 // per file, under the hub's limit for one step
	changesMaxNew   = 64 << 10
	changesTimeout  = 10 * time.Second
)

type changeSnapshot struct {
	dir       string
	base      string          // the recorded tree (a commit)
	untracked map[string]bool // untracked files before the run
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.quotepath=off"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	return cmd.Output()
}

// snapshotChanges records dir's working tree before a run; nil when dir is
// not a git work tree with a commit, or git is missing.
func snapshotChanges(ctx context.Context, dir string) *changeSnapshot {
	if dir == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, changesTimeout)
	defer cancel()
	if out, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}
	base := ""
	if out, err := git(ctx, dir, "stash", "create"); err == nil {
		base = strings.TrimSpace(string(out))
	}
	if base == "" { // nothing uncommitted: the commit itself
		out, err := git(ctx, dir, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return nil
		}
		base = strings.TrimSpace(string(out))
	}
	return &changeSnapshot{dir: dir, base: base, untracked: untrackedFiles(ctx, dir)}
}

func untrackedFiles(ctx context.Context, dir string) map[string]bool {
	out, err := git(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil
	}
	m := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			m[p] = true
		}
	}
	return m
}

// changes are the run's changes as trace steps, one per file (kind
// "change": Tool the path, Input the status A/M/D/R, Added and Removed the
// line counts, Output the diff, cut to a size), then, when there were more
// files than are listed, one "change" step with Text "+N" and no Tool.
func (s *changeSnapshot) changes(ctx context.Context) []harness.Event {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, changesTimeout)
	defer cancel()
	var out []harness.Event
	status := map[string]string{}
	if b, err := git(ctx, s.dir, "diff", "--no-color", "--no-ext-diff", "--name-status", "-z", s.base, "--"); err == nil {
		f := strings.Split(string(b), "\x00")
		for i := 0; i+1 < len(f); {
			st := f[i]
			if st == "" {
				break
			}
			if st[0] == 'R' || st[0] == 'C' { // old path, new path
				if i+2 < len(f) {
					status[f[i+2]] = st[:1]
				}
				i += 3
				continue
			}
			status[f[i+1]] = st[:1]
			i += 2
		}
	}
	counts := map[string][2]int{}
	if b, err := git(ctx, s.dir, "diff", "--no-color", "--no-ext-diff", "--numstat", s.base, "--"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 {
				a, _ := strconv.Atoi(parts[0]) // "-" for binary files
				r, _ := strconv.Atoi(parts[1])
				path := parts[2]
				if i := strings.Index(path, " => "); i >= 0 { // a rename: the new path
					path = renamedPath(path)
				}
				counts[path] = [2]int{a, r}
			}
		}
	}
	paths := make([]string, 0, len(status))
	for p := range status {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var fresh []string
	for p := range untrackedFiles(ctx, s.dir) {
		if !s.untracked[p] {
			fresh = append(fresh, p)
		}
	}
	sort.Strings(fresh)
	more := 0
	for _, p := range paths {
		if len(out) >= changesMaxFiles {
			more++
			continue
		}
		ev := harness.Event{Kind: "change", Tool: p, Input: status[p], Added: counts[p][0], Removed: counts[p][1]}
		if d, err := git(ctx, s.dir, "diff", "--no-color", "--no-ext-diff", s.base, "--", p); err == nil {
			ev.Output = cutDiff(string(d))
		}
		out = append(out, ev)
	}
	for _, p := range fresh {
		if len(out) >= changesMaxFiles {
			more++
			continue
		}
		out = append(out, newFileChange(s.dir, p))
	}
	if more > 0 {
		out = append(out, harness.Event{Kind: "change", Text: "+" + strconv.Itoa(more)})
	}
	return out
}

// renamedPath is the new path of a numstat rename ("a/{x => y}/b", "x => y").
func renamedPath(p string) string {
	if i, j := strings.Index(p, "{"), strings.Index(p, "}"); i >= 0 && j > i {
		mid := p[i+1 : j]
		if k := strings.Index(mid, " => "); k >= 0 {
			return strings.ReplaceAll(p[:i]+mid[k+4:]+p[j+1:], "//", "/")
		}
	}
	if k := strings.Index(p, " => "); k >= 0 {
		return p[k+4:]
	}
	return p
}

// newFileChange is an untracked file the run made, as an added diff when it
// is small text.
func newFileChange(dir, p string) harness.Event {
	ev := harness.Event{Kind: "change", Tool: p, Input: "A"}
	b, err := os.ReadFile(filepath.Join(dir, p))
	if err != nil || len(b) > changesMaxNew || bytes.IndexByte(b, 0) >= 0 {
		return ev
	}
	text := strings.TrimSuffix(string(b), "\n")
	lines := strings.Split(text, "\n")
	if text == "" {
		lines = nil
	}
	var d strings.Builder
	d.WriteString("--- /dev/null\n+++ b/" + p + "\n@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
	for _, l := range lines {
		d.WriteString("+" + l + "\n")
	}
	ev.Added, ev.Output = len(lines), cutDiff(d.String())
	return ev
}

// cutDiff keeps a diff under the size one step may have, at a line end.
func cutDiff(d string) string {
	if len(d) <= changesMaxDiff {
		return d
	}
	cut := d[:changesMaxDiff]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i+1]
	}
	return cut + "…\n"
}
