package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The memory block: a few lines in the agent CLIs' user-level instructions
// (~/.claude/CLAUDE.md, ~/.codex/AGENTS.md), always in context, so an agent
// that joined myco keeps using it well (progress with screenshots, a
// suggested next step, asking, delegating) without first deciding to read
// the skill. Never a project's own files. It is marked, so myco updates its
// own lines and leaves the rest of the file alone; removing it leaves a
// marker so that it is not added again.

const (
	memoryBegin = "<!-- myco:begin — added by myco; `myco upgrade` keeps it current, `myco skill memory --remove` takes it out -->"
	memoryEnd   = "<!-- myco:end -->"
	// memoryOff stays where the block was removed: join and upgrade leave
	// such a file alone (`myco skill memory --add` puts it back).
	memoryOff = "<!-- myco:memory off — `myco skill memory --add` puts the myco block back -->"
)

const memoryBody = `## myco (agents working together)

This applies only when you are a myco agent (you joined a myco workspace: tasks reach you
through ` + "`myco inbox wait`" + ` or the myco runner, with a message id). People follow your work in the
myco app: your chat, your progress, your run as it goes.

- Ack a task (` + "`myco message ack --id <id>`" + `), then keep the sender posted at real milestones:
  ` + "`myco message progress --id <id> --body \"...\"`" + `. When they would want to see it (a page, a
  chart, a draft), attach a screenshot: ` + "`--attach shot.png`" + `; it shows under the update.
- When there is an obvious next step for them, suggest it as you answer:
  ` + "`myco message reply --id <id> --body \"...\" --suggest \"...\"`" + ` (one short line in their language;
  the app puts it in their input, Tab fills it in).
- Need their decision? ` + "`myco message ask --id <id> --question \"...\" --option \"...\"`" + `. Need another agent?
  ` + "`myco message hold`" + `, then ` + "`myco message send --to <agent>`" + `; reply when the answers are in.
- In a session that receives with ` + "`myco inbox wait`" + `: after each task, start it again in the background.
- The whole guide, for your version: ` + "`myco skill show`" + `.`

func memoryBlock() string { return memoryBegin + "\n" + memoryBody + "\n" + memoryEnd + "\n" }

// memoryTargets are the user-level instruction files of the agent CLIs on
// this machine (only those whose own folder exists).
func memoryTargets(getenv func(string) string) []string {
	home := ""
	if getenv != nil {
		home = getenv("HOME")
		if home == "" && runtime.GOOS == "windows" {
			home = getenv("USERPROFILE")
		}
	}
	var out []string
	for _, c := range []struct{ env, dir, file string }{{"CLAUDE_CONFIG_DIR", ".claude", "CLAUDE.md"}, {"CODEX_HOME", ".codex", "AGENTS.md"}} {
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
			out = append(out, filepath.Join(root, c.file))
		}
	}
	return out
}

// memoryMode says what installMemory does.
type memoryMode int

const (
	memoryRefresh memoryMode = iota // update blocks already there; add none
	memoryAdd                       // add where there is none (not where it was turned off)
	memoryForce                     // add even where it was turned off
	memoryRemove                    // take it out, and leave the off marker
)

// installMemory writes (or removes) the block in each target, keeping the
// rest of the file as it is. It returns the files it changed, and those it
// left alone because the block was turned off there.
func installMemory(targets []string, mode memoryMode) (changed, off []string, err error) {
	for _, path := range targets {
		b, rerr := os.ReadFile(path)
		if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return changed, off, rerr
		}
		old := string(b)
		next, ok := withMemory(old, mode)
		if !ok {
			if strings.Contains(old, memoryOff) && mode != memoryRemove {
				off = append(off, path)
			}
			continue
		}
		if next == old {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return changed, off, err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(next), 0o644); err != nil {
			return changed, off, err
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return changed, off, err
		}
		changed = append(changed, path)
	}
	return changed, off, nil
}

// withMemory is the file's text after mode; ok false: leave it as it is.
func withMemory(old string, mode memoryMode) (string, bool) {
	i := strings.Index(old, memoryBegin)
	j := -1
	if i >= 0 {
		if k := strings.Index(old[i:], memoryEnd); k >= 0 {
			j = i + k + len(memoryEnd)
			if j < len(old) && old[j] == '\n' {
				j++
			}
		}
	}
	has := i >= 0 && j > i
	switch mode {
	case memoryRemove:
		if has {
			return old[:i] + memoryOff + "\n" + old[j:], true
		}
		if strings.Contains(old, memoryOff) {
			return old, false
		}
		return appendBlock(old, memoryOff+"\n"), true
	case memoryRefresh:
		if !has {
			return old, false
		}
		return old[:i] + memoryBlock() + old[j:], true
	case memoryAdd, memoryForce:
		if has {
			return old[:i] + memoryBlock() + old[j:], true
		}
		if strings.Contains(old, memoryOff) {
			if mode == memoryAdd {
				return old, false
			}
			return strings.Replace(old, memoryOff, strings.TrimSuffix(memoryBlock(), "\n"), 1), true
		}
		return appendBlock(old, memoryBlock()), true
	}
	return old, false
}

func appendBlock(old, block string) string {
	switch {
	case old == "":
		return block
	case strings.HasSuffix(old, "\n\n"):
		return old + block
	case strings.HasSuffix(old, "\n"):
		return old + "\n" + block
	}
	return old + "\n\n" + block
}
