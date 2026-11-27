package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/machinenode"
)

type managedForkKey struct{}
type forkManifest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SourceID   string `json:"source_id"`
	AgentID    string `json:"agent_id,omitempty"`
	SourceDir  string `json:"source_dir"`
	Workdir    string `json:"workdir"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Worktree   bool   `json:"worktree"`
	Cleaned    bool   `json:"cleaned,omitempty"`
}

func (e *env) forkManifestPath(id string) (string, error) {
	if len(id) > 100 || id == "" || strings.ContainsAny(id, "/\\.\x00") {
		return "", errors.New("invalid fork ID")
	}
	return filepath.Join(config.Dir(e.defaultPath), "forks", id+".json"), nil
}
func saveForkManifest(path string, m *forkManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".fork-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(b)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func loadForkManifest(path string) (*forkManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m forkManifest
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
func forkGit(ctx context.Context, dir string, args ...string) (string, error) {
	hooks, err := os.MkdirTemp("", "myco-fork-hooks-")
	if err != nil {
		return "", err
	}
	defer os.Remove(hooks)
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=" + hooks, "-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	// Session shells may export GIT_DIR or GIT_INDEX_FILE; they must not
	// redirect provisioning or cleanup to another repository.
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git operation refused (%s); inspect the repository on the machine", args[0])
	}
	return strings.TrimSpace(string(b)), nil
}
func canonicalForkDir(dir string) (string, error) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(real)
}
func verifyForkTree(ctx context.Context, m *forkManifest) error {
	if !m.Worktree {
		return nil
	}
	real, err := canonicalForkDir(m.Workdir)
	if err != nil {
		return err
	}
	if real != m.Workdir || real == m.Repository || real == m.SourceDir {
		return errors.New("worktree path changed or is the source directory")
	}
	root, err := forkGit(ctx, m.Workdir, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root, err = canonicalForkDir(root)
	if err != nil || root != m.Workdir {
		return errors.New("copy is not the recorded worktree root")
	}
	branch, err := forkGit(ctx, m.Workdir, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch != m.Branch {
		return errors.New("copy branch changed; retirement refused")
	}
	list, err := forkGit(ctx, m.Repository, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	for _, entry := range strings.Split(list, "\x00\x00") {
		path, branch := "", ""
		for _, line := range strings.Split(entry, "\x00") {
			if v, ok := strings.CutPrefix(line, "worktree "); ok {
				path = v
			}
			if v, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
				branch = v
			}
		}
		if path != "" {
			real, e := canonicalForkDir(filepath.FromSlash(path))
			if e == nil && real == m.Workdir && branch == m.Branch {
				return nil
			}
		}
	}
	return errors.New("worktree is not registered with the recorded source repository")
}
func (e *env) managedMachineFork(ctx context.Context, node *machinenode.Node, c *client.Client, hub string, sh runnerShared, outer harness.Request, id string, retire bool) (res api.SpawnResult, retErr error) {
	defer func() {
		if retErr != nil || res.Error != "" || res.Denied {
			failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = c.CompleteAgentFork(failureCtx, id, api.AgentForkCompletion{Error: "machine operation failed"})
		}
	}()
	f, err := c.MachineForkPlan(ctx, id)
	if err != nil {
		return res, err
	}
	if !retire && outer.From != machinenode.AdminSender && outer.From != f.ForkedBy {
		return res, errors.New("fork requester does not match recorded creator")
	}
	path, err := e.forkManifestPath(id)
	if err != nil {
		return res, err
	}
	if retire {
		return e.retireMachineFork(ctx, c, hub, sh, f, path)
	}
	if f.State == "retired" || f.State == "retiring" {
		return res, errors.New("copy is retiring or retired")
	}
	if f.State == "ready" {
		m, readyErr := loadForkManifest(path)
		if readyErr != nil {
			return res, readyErr
		}
		cfg, readyErr := config.Load(profilePathFor(e.defaultPath, hub, f.Name))
		if readyErr != nil {
			return res, readyErr
		}
		a, readyErr := c.GetAgent(ctx, f.Name)
		if readyErr != nil {
			return res, readyErr
		}
		if m.ID != id || m.Name != f.Name || m.SourceID != f.SourceID || m.Cleaned || cfg.ForkID != id || cfg.Agent != f.Name || !config.SameHub(cfg.Hub, hub) || a.ID != f.AgentID || cfg.Runner.Workdir != f.Workdir {
			return res, errors.New("registered copy no longer matches its local profile")
		}
		m.AgentID = f.AgentID
		if readyErr = saveForkManifest(path, m); readyErr != nil {
			return res, readyErr
		}
		if node.Deps.Spawned != nil {
			node.Deps.Spawned()
		}
		return api.SpawnResult{Agent: f.Name, CLI: f.CLI, Workdir: f.Workdir, Access: f.Access}, nil
	}
	sourcePath := profilePathFor(e.defaultPath, hub, f.ForkOf)
	source, err := config.Load(sourcePath)
	if err != nil {
		return res, err
	}
	if source.Agent != f.ForkOf || !config.SameHub(source.Hub, hub) || source.SessionID != f.SourceSession || source.Harness != f.CLI || source.Runner.Workdir != f.SourceDir {
		return res, errors.New("source session does not match a local runner profile")
	}
	if !f.Force {
		if sh.slots == nil {
			return res, errors.New("runner capacity is unavailable")
		}
		slots := sh.slots.Snapshot("")
		if slots.InUse >= slots.MaxConcurrent {
			return res, errors.New("runner has no free slots; retry or pass --force")
		}
	}
	sr := api.SpawnRequest{CLI: f.CLI, Dir: f.SourceDir, Name: f.Name, ForkFrom: f.SourceSession, Access: f.Access, Model: f.Model, Labels: []string{"fork_of=" + f.ForkOf, "forked_by=" + f.ForkedBy}}
	if f.ForTask != "" {
		sr.Labels = append(sr.Labels, "for_task="+f.ForTask)
	}
	plan, err := node.Check(sr)
	if err != nil {
		return res, err
	}
	if plan.Name != f.Name {
		return res, errors.New("machine prefix changed")
	}
	m, err := loadForkManifest(path)
	if errors.Is(err, os.ErrNotExist) {
		m = &forkManifest{ID: id, Name: f.Name, SourceID: f.SourceID, SourceDir: plan.Workdir, Workdir: plan.Workdir, Worktree: f.Worktree}
		if f.Worktree {
			if f.CLI == "claude" {
				return res, errors.New("unsupported: Claude cannot fork a directory-scoped session into --worktree")
			}
			repo, err := forkGit(ctx, plan.Workdir, "rev-parse", "--show-toplevel")
			if err != nil {
				return res, err
			}
			repo, err = canonicalForkDir(repo)
			if err != nil {
				return res, err
			}
			// A sibling is allowed only when its parent is inside spawn.dirs.
			check := sr
			check.Dir = filepath.Dir(repo)
			if _, err = node.Check(check); err != nil {
				return res, fmt.Errorf("worktree parent is outside allowed spawn.dirs: %w", err)
			}
			m.Repository = repo
			m.Workdir = filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-fork-"+id)
			m.Branch = "fork/" + f.Name
			if _, err := os.Lstat(m.Workdir); !errors.Is(err, os.ErrNotExist) {
				return res, errors.New("worktree destination already exists; refusing to reuse it")
			}
		}
		if err = saveForkManifest(path, m); err != nil {
			return res, err
		}
		if m.Worktree {
			if _, err = forkGit(ctx, m.Repository, "worktree", "add", "-b", m.Branch, "--", m.Workdir, "HEAD"); err != nil {
				return res, err
			}
		}
	} else if err != nil {
		return res, err
	}
	if m.ID != id || m.Name != f.Name || m.SourceID != f.SourceID || m.Cleaned {
		return res, errors.New("local fork record does not match the Hub record")
	}
	if m.Worktree {
		if err = verifyForkTree(ctx, m); err != nil {
			return res, err
		}
	}
	sr.Dir = m.Workdir
	profile := profilePathFor(e.defaultPath, hub, f.Name)
	cfg, err := config.Load(profile)
	if err != nil {
		return res, err
	}
	if cfg.Agent != "" && (cfg.ForkID != id || cfg.Agent != f.Name || !config.SameHub(cfg.Hub, hub)) {
		return res, errors.New("destination profile belongs to another identity")
	}
	if cfg.Agent == "" {
		body, _ := json.Marshal(sr)
		request := outer
		request.Body = string(body)
		inner := *node
		inner.Deps = node.Deps
		inner.Deps.Spawned = nil
		result, err := inner.Run(context.WithValue(ctx, managedForkKey{}, id), request)
		if err != nil {
			return res, err
		}
		if err = json.Unmarshal([]byte(result.Output), &res); err != nil {
			return res, err
		}
		if res.Denied || res.Error != "" {
			return res, nil
		}
	}
	active, err := c.CompleteAgentFork(ctx, id, api.AgentForkCompletion{Workdir: m.Workdir, Repository: m.Repository, Branch: m.Branch})
	if err != nil {
		return res, err
	}
	m.AgentID = active.AgentID
	if err = saveForkManifest(path, m); err != nil {
		return res, err
	}
	if node.Deps.Spawned != nil {
		node.Deps.Spawned()
	}
	return api.SpawnResult{Agent: f.Name, CLI: f.CLI, Workdir: m.Workdir, Access: f.Access}, nil
}
func (e *env) retireMachineFork(ctx context.Context, c *client.Client, hub string, sh runnerShared, f *api.AgentFork, path string) (api.SpawnResult, error) {
	res := api.SpawnResult{Agent: f.Name, CLI: machinenode.ManagedRetireCLI}
	if f.State == "retired" {
		return res, nil
	}
	if f.State != "retiring" {
		return res, errors.New("Hub has not authorized retirement")
	}
	m, err := loadForkManifest(path)
	if errors.Is(err, os.ErrNotExist) && f.AgentID == "" {
		if _, err = c.GetAgent(ctx, f.Name); err != nil {
			if ae, ok := client.AsError(err); ok && ae.Status == 404 {
				_, err = c.CompleteAgentFork(ctx, f.ID, api.AgentForkCompletion{Retired: true})
				return res, err
			}
		}
		return res, errors.New("registration exists but local copy provenance is missing")
	}
	if err != nil {
		return res, fmt.Errorf("local fork provenance is unavailable: %w", err)
	}
	if m.ID != f.ID || m.Name != f.Name || m.SourceID != f.SourceID || (f.AgentID != "" && m.AgentID != "" && m.AgentID != f.AgentID) || (f.Workdir != "" && m.Workdir != f.Workdir) {
		return res, errors.New("local copy identity does not match the Hub")
	}
	if !m.Cleaned {
		target, targetErr := c.GetAgent(ctx, f.Name)
		absent := false
		if ae, ok := client.AsError(targetErr); ok && ae.Status == 404 {
			absent = true
		} else if targetErr != nil {
			return res, targetErr
		}
		if !absent && f.AgentID == "" {
			f, err = c.CompleteAgentFork(ctx, f.ID, api.AgentForkCompletion{Workdir: m.Workdir, Repository: m.Repository, Branch: m.Branch})
			if err != nil {
				return res, err
			}
			m.AgentID = f.AgentID
			if err = saveForkManifest(path, m); err != nil {
				return res, err
			}
			target, targetErr = c.GetAgent(ctx, f.Name)
			if targetErr != nil {
				return res, targetErr
			}
		}
		if !absent {
			if target.ID != f.AgentID || target.MachineID != f.MachineID || target.Binding.Workdir != m.Workdir {
				return res, errors.New("copy identity or directory changed")
			}
			if target.Runtime.RunnerInstance != "" && target.Runtime.RunnerInstance != runnerInstance {
				return res, errors.New("copy is served by another runner instance")
			}
		}
		profile := profilePathFor(e.defaultPath, hub, f.Name)
		cfg, err := config.Load(profile)
		if err != nil {
			return res, err
		}
		if cfg.Agent != "" && (cfg.ForkID != f.ID || cfg.Agent != f.Name || cfg.Runner.Workdir != m.Workdir || !config.SameHub(cfg.Hub, hub)) {
			return res, errors.New("copy profile changed; retirement refused")
		}
		if sh.forkWorkers == nil {
			return res, errors.New("unsupported: managed retirement requires runner --all")
		}
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err = sh.forkWorkers.stopPath(stopCtx, profile); err != nil {
			return res, fmt.Errorf("copy worker has not stopped: %w", err)
		}
		if m.Worktree {
			if _, err = os.Lstat(m.Workdir); !errors.Is(err, os.ErrNotExist) {
				if err = verifyForkTree(ctx, m); err != nil {
					return res, err
				}
				if _, err = forkGit(ctx, m.Repository, "worktree", "remove", "--", m.Workdir); err != nil {
					return res, fmt.Errorf("worktree has uncommitted changes or cannot be removed: %w", err)
				}
			}
		}
		current, err := config.Load(profile)
		if err != nil {
			return res, err
		}
		if current.Token != cfg.Token || current.ForkID != cfg.ForkID {
			return res, errors.New("profile identity changed during retirement")
		}
		if cfg.Agent != "" {
			if err = os.Remove(profile); err != nil && !errors.Is(err, os.ErrNotExist) {
				return res, err
			}
		}
		m.Cleaned = true
		if err = saveForkManifest(path, m); err != nil {
			return res, err
		}
	}
	_, err = c.CompleteAgentFork(ctx, f.ID, api.AgentForkCompletion{Retired: true})
	return res, err
}
