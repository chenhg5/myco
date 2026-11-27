package api

import "github.com/myco-ai/myco/internal/domain"

// AgentFork is a managed copy's immutable provenance and lifecycle state.
type AgentFork = domain.AgentFork

type AgentForkRequest struct {
	From       string `json:"from"`
	Count      int    `json:"count,omitempty"`
	Worktree   bool   `json:"worktree,omitempty"`
	NamePrefix string `json:"name_prefix,omitempty"`
	ForTask    string `json:"for_task,omitempty"`
	Force      bool   `json:"force,omitempty"`
	RequestID  string `json:"request_id"`
}
type AgentRetireRequest struct {
	Name   string `json:"name,omitempty"`
	ForkOf string `json:"fork_of,omitempty"`
	Now    bool   `json:"now,omitempty"`
}
type AgentForkResult struct {
	Forks  []*AgentFork `json:"forks"`
	DryRun bool         `json:"dry_run,omitempty"`
}
type AgentForkCompletion struct {
	Workdir    string `json:"workdir"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Retired    bool   `json:"retired,omitempty"`
	Error      string `json:"error,omitempty"`
}
