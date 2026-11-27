// Managed-copy contracts used by coordinator clients. Lifecycle authority is
// held by the Hub, independently of editable agent labels.
export interface AgentForkRequest {
  from: string;
  count?: number;
  worktree?: boolean;
  name_prefix?: string;
  for_task?: string;
  force?: boolean;
  request_id: string;
}
export interface AgentRetireRequest {
  name?: string;
  fork_of?: string;
  now?: boolean;
}
export interface AgentFork {
  id: string;
  batch: string;
  name_prefix?: string;
  source_id: string;
  source_visibility?: "team" | "private";
  source_allow_from?: string[];
  fork_of: string;
  cli: string;
  source_session: string;
  source_dir: string;
  node_id: string;
  node: string;
  machine_id: string;
  owner_id: string;
  name: string;
  agent_id?: string;
  forked_by: string;
  for_task?: string;
  worktree: boolean;
  force?: boolean;
  access?: string;
  model?: string;
  state: "planned" | "ready" | "retiring" | "retired";
  message_id?: string;
  retire_message_id?: string;
  retire_attempt?: number;
  finishing?: string[];
  workdir?: string;
  repository?: string;
  branch?: string;
  now?: boolean;
  created_at: string;
  error?: string;
}
export interface AgentForkResult {
  forks: AgentFork[];
  dry_run?: boolean;
}
export interface AgentForkCompletion {
  workdir: string;
  repository?: string;
  branch?: string;
  retired?: boolean;
  error?: string;
}
