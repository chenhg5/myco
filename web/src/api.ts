// Types mirror internal/api (the hub's JSON contract).
export interface Binding { harness?: string; session_id?: string; environment?: string; workdir?: string }
export interface Delivery { mode: string; url?: string; has_secret?: boolean; params?: Record<string, string> }

/** What a node is. `machine` is a machine node (`xxx-machine`), not shown as a node. */
export type AgentKind = "agent" | "shell" | "human" | "machine";
export type RuntimeAccess = "readonly" | "workspace" | "full";
/** app: chosen for this node in the App; self: reported by the session itself. */
export type ModelSource = "app" | "self" | "configured" | "cli-config" | "observed" | "default" | "provider";
/** pinned: chosen for this node; machine: the machine's current setting; env_file: entered by hand; missing: the pinned one is gone. */
export type ProviderSource = "pinned" | "machine" | "env_file" | "missing";
/** One usage window of a plan (5 hours, 7 days, ...). */
export interface QuotaWindow {
  name: string;
  label?: string;
  /** 0..1 */
  used: number;
  resets_at: string;
  /** "allowed" while there is room; anything else means the window is spent. */
  status?: string;
}
/** A CLI's own usage report (Claude or Codex), as its local files hold it. */
export interface Quota {
  source: "claude" | "codex";
  at: string;
  windows: QuotaWindow[];
}
/** What the node runs unless told otherwise; absent on nodes reported by old runners. */
export interface RunSettings {
  model?: string;
  effort?: string;
  /** Agent owner's choice; omitted on old hubs means disabled. */
  mcp_tools?: boolean;
  /** permission level chosen by the node's owner ("" follows the machine); only its owner may change it */
  access?: RuntimeAccess | "";
}
/** What the node actually runs; absent on nodes reported by old runners. */
/** A model a CLI offers: the id --model takes, a display name, the efforts it supports. */
export interface RuntimeModel { id: string; name?: string; efforts?: string[] }
export interface MCPServer { name: string; transport: string; host?: string; scope: "user" | "project" | "plugin"; enabled?: boolean }
export interface ToolSkill { name: string; description?: string; scope: "user" | "project"; managed_by?: string }
export interface ToolPlugin { name: string; marketplace?: string; enabled: boolean }
export interface Runtime {
  cli: string;
  cli_version?: string;
  model?: string;
  model_source?: ModelSource;
  /** reasoning effort (low/medium/high/... depends on the CLI) */
  effort?: string;
  effort_source?: ModelSource;
  access?: RuntimeAccess;
  /** the highest access its machine lets the owner choose (spawn.max_access); absent from older runners */
  max_access?: RuntimeAccess;
  /** model provider's name (never its key) */
  provider?: string;
  provider_source?: ProviderSource;
  /** plan usage, when the CLI writes it on this computer */
  quota?: Quota;
  /** what its runner supports beyond the basics; absent from older runners */
  features?: string[];
  /** why its runner went offline, said as it stopped: "stopped" (absent from older runners and while it runs) */
  away?: "stopped" | string;
  /** where its runner runs when that explains going offline: "wsl" */
  env?: "wsl" | string;
  /** the models its CLI offers on that computer, in the CLI's order (Codex; runners from v0.21.1) */
  models?: RuntimeModel[];
  mcp?: MCPServer[];
  skills?: ToolSkill[];
  plugins?: ToolPlugin[];
  tool_warnings?: string[];
}
export interface Skill { name: string; summary?: string; examples?: string[] }

/** A project: nodes working together on a goal (hubs from v0.18.25). */
export interface Project {
  name: string; title: string; goal?: string; brief?: string;
  links: ProjectLink[];
  visibility: "members" | "team";
  owner?: string; owner_id?: string; archived?: boolean;
  created_at: string; updated_at: string;
  members: ProjectMember[];
  docs: ProjectDoc[];
  /** edit the content (owner, admins, coordinators); manage it (people) */
  can_edit: boolean; can_manage: boolean;
  /** the computers (ids) its coordinators may start agents on (hubs from
   *  v0.21.2; absent before) */
  machines?: string[];
  /** the project's memory document, with its body (hubs from v0.21.10;
   *  absent before): what members learned and all keep in mind */
  memory?: ProjectDoc;
}
export interface ProjectLink { title?: string; url: string }
export interface ProjectMember {
  name: string; role: "member" | "coordinator"; note?: string; added_by?: string; added_at: string;
  /** its card, when the viewer may see the node */
  agent?: Agent;
}
/** One step of a task's run (GET …/messages/{id}/trace). */
export interface TraceStep {
  kind: "text" | "thinking" | "tool" | "tool_result" | "usage" | "error" | "output" | "delta" | string;
  /** output: stdout/stderr; delta: text/thinking (pieces share a block id). */
  stream?: string;
  text?: string; tool?: string; id?: string; input?: string; output?: string; failed?: boolean;
  input_tokens?: number; output_tokens?: number; cost_usd?: number; duration_ms?: number;
}
export interface TraceEvent { seq: number; kind: string; data: TraceStep; at?: string }
export interface Trace { message: string; status: string; events: TraceEvent[] }
export interface ProjectDoc {
  name: string; size: number; updated_by?: string; updated_at: string; body?: string;
  /** set for uploaded files (from v0.18.29) */
  content_type?: string;
  /** not text: download it (files/{name}) */
  binary?: boolean;
}
export interface Agent {
  id: string; name: string; display_name?: string; aliases?: string[]; description: string;
  /** the face people chose: "look:c3-p1-f2-s1" (a drawn one) or "pic:<hash>"
   *  (a picture: GET /v1/agents/{name}/avatar); absent = drawn from the id */
  avatar?: string;
  status_text?: string; status_at?: string;
  /** names of skills (hubs before skills: the only list) */
  capabilities: string[];
  /** what others can pick it for; absent on hubs before v0.18.21 */
  skills?: Skill[];
  /** team (everyone on the hub) or private (only its owner and their agents); absent = team */
  visibility?: "team" | "private";
  labels: string[];
  binding: Binding; delivery: Delivery; allow_from: string[]; enrolled_by?: string;
  online: boolean; created_at: string; last_seen_at?: string;
  kind?: AgentKind; machine_id?: string; runtime?: Runtime;
  /** the model and effort chosen for this node in the App ("" = follow the
   *  computer's setting); from v0.19.5, and only those who may manage it */
  run_settings?: RunSettings;
  /** the person who added it (member name) */
  owner?: string; owner_id?: string;
  /** its resume line's state, for those who may manage it (hubs from
   *  v0.19.5; absent on older hubs and before a line is made) */
  resume?: AgentResume;
}
/** An agent's resume line: running its command in a new sandbox brings the
 *  same agent (same name, id and history) back. */
export interface AgentResume {
  created_at: string;
  last_used_at?: string;
  /** the sandbox it was last run in */
  last_used_env?: string;
}
/** A real computer (or server) that runs nodes. Cloud sessions are not machines. */
export interface Machine {
  id: string; name: string; hostname: string;
  os: "darwin" | "linux" | "windows" | "cloud";
  arch?: string; myco_version?: string;
  online: boolean; last_seen_at?: string;
  /** False when the runner is too old or not running; such a machine cannot spawn. */
  can_spawn: boolean;
  /** Names of the nodes on it (machine nodes excluded). */
  agents: string[];
  /** Its runners predate machine reporting: grouped by host name, cannot be renamed. */
  legacy?: boolean;
  /** whose machine it is (member name) */
  owner?: string; owner_id?: string;
}
export interface MachineRemoveResult { machine: Machine; removed: string[]; dry_run: boolean }
/** Automations (hubs from v0.19.22): a task sent, as its owner, to one
 *  agent on a schedule or when its webhook is called. */
export interface AutomationSchedule { cron: string; tz?: string }
export interface AutomationRun {
  id: string; automation_id: string; at: string;
  trigger: "schedule" | "webhook" | "manual"; outcome: "sent" | "skipped" | "failed";
  message_id?: string; error?: string;
  /** what a webhook call carried (hubs from v0.20.18); in a list of runs
   *  without its body: appApi.automationRun has it */
  request?: WebhookRequest;
  /** the workflow run it started (a workflow automation) */
  workflow_run?: string;
}
export interface WebhookRequest { content_type?: string; headers?: Record<string, string>; body?: string; size: number }
/** A web page published to a hub (hubs from v0.19.24): a folder of static
 *  files, in versions, opened in the App's page panel. */
export interface PageVersion { n: number; entry: string; files: number; size: number; sha256: string; by: string; at: string }
export interface Page {
  id: string; name: string; project?: string; publisher: string; version: number; entry: string; url: string;
  versions: PageVersion[]; created_at: string; updated_at: string; can_manage: boolean;
  /** a server on its publisher's computer (port there), forwarded while
   *  `myco page serve` runs; online: forwarded now */
  live?: boolean; port?: number; online?: boolean;
  /** the template it was made from (hubs from v0.20.5) */
  /** the app it was made from; latest: the app's newer version (hubs from v0.20.7) */
  template?: { id: string; slug: string; name: string; version: string; permissions?: string[]; latest?: string };
}
/** A template in the workspace's library (hubs from v0.20.5): official
 *  ones (myco's own) first. */
export interface Template {
  locales?: Record<string, { name?: string; description?: string }>;
  id: string; slug: string; name: string; description?: string; kind: string; version: string; publisher: string;
  /** its shelf (hubs from v0.20.6): office, dev, tools, life, creative, game, other */
  category?: string;
  /** its icon, SVG markup (hubs from v0.20.7): shown as a mask, tinted */
  icon?: string;
  official?: boolean; can_manage: boolean; guide?: string; permissions?: string[]; export?: string[];
  /** its screenshots (hubs from v0.21.5): paths in its folder, a dark one
   *  beside when there is one; a workspace's own list them when got alone */
  shots?: { src: string; dark?: string }[];
  data?: Record<string, { fields: Record<string, string>; single?: boolean }>;
  versions: { version: string; files: number; size: number; by: string; at: string }[];
  created_at: string; updated_at: string;
}
/** What opening a page gives: where its files are served (a loopback origin
 *  of the App's own), for which version. */
export interface PageOpen { url: string; version: number; latest: number; name: string; entry: string; live?: boolean; online?: boolean; port?: number }
/** A page's shared state (the bridge's myco.update): null before anyone wrote. */
export interface PageState { state: unknown; version: number; updated_by?: string; updated_at?: string }

/** What a conversation keeps at hand (置顶; hubs from v0.20.3): a web
 *  address, a file, or one of its messages. Both parties see the same. */
export interface Pin {
  id: string;
  kind: "link" | "file" | "message";
  title: string;
  /** the address (link), the message's id (message) */
  target?: string;
  file?: Attachment;
  added_by: string;
  at: string;
}
export interface PinList { a: string; with: string; pins: Pin[]; max: number }
/** A page's records (hubs from v0.20.5). */
export interface PageRecord { collection: string; id: string; data: Record<string, unknown>; version: number; updated_by?: string; updated_at: string }
export interface PageChange { seq: number; collection: string; record: string; op: "put" | "delete"; data?: Record<string, unknown>; version: number; by?: string; at: string }
export interface PageChanges { changes: PageChange[]; seq: number; reset?: boolean }
export interface PinCreate { a?: string; with: string; kind: Pin["kind"]; title?: string; target?: string; file?: Attachment }

export interface Automation {
  id: string; name: string; owner: string; from: string; to: string; project?: string; body: string;
  schedule?: AutomationSchedule; webhook: boolean; webhook_url?: string; skip_if_busy: boolean; enabled: boolean;
  created_at: string; updated_at: string; next_run_at?: string; last_run?: AutomationRun;
  /** It starts this workflow instead of sending a task (hubs from v0.21). */
  workflow?: string; workflow_name?: string; inputs?: Record<string, string>;
}
export interface AutomationRequest {
  name?: string; to?: string; project?: string; body?: string;
  schedule?: AutomationSchedule; clear_schedule?: boolean;
  webhook?: boolean; rotate_secret?: boolean; skip_if_busy?: boolean; enabled?: boolean;
  workflow?: string; inputs?: Record<string, string>;
}
export interface AutomationResponse { automation: Automation; webhook_secret?: string }

/** Workflows (hubs from v0.21): steps the workspace hands to agents,
 *  computers and people, in order. */
/** A JSON Schema (draft 2020-12): all of it is kept and checked by the
 *  hub; the App reads the common keywords. */
export interface WfSchema {
  type?: string | string[]; title?: string; description?: string; required?: string[];
  properties?: Record<string, WfSchema>; items?: WfSchema; enum?: unknown[]; default?: unknown; format?: string;
  [keyword: string]: unknown;
}
export interface WfStepDef {
  id: string; title?: string; agent?: string; role?: string; prompt?: string; computer?: string; run?: string; parse?: string;
  human?: string; options?: string[]; output?: WfSchema; outputSchema?: WfSchema; inputSchema?: WfSchema;
  timeout?: string; retry?: number; dispatch?: "first" | "all"; join?: "all" | { from: string[] };
}
export interface WfEdgeDef { from: string; to: string; when?: string; maxIterations?: number; else?: string; onError?: boolean; map?: Record<string, unknown> }
export interface WorkflowDef {
  name: string; title?: string; description?: string; tags?: string[]; group?: string; project?: string;
  inputs?: WfSchema; steps: WfStepDef[]; edges?: WfEdgeDef[]; entry?: string; allowUnboundedCycles?: boolean;
}
export interface Workflow {
  id: string; name: string; title?: string; tags?: string[]; group?: string; description?: string; project?: string; owner?: string; version: number; enabled: boolean;
  steps: number; active: number; last_run?: WorkflowRun; definition?: WorkflowDef; created_at: string; updated_at: string;
}
export type RunStatus = "running" | "waiting" | "succeeded" | "failed" | "cancelled";
export type StepStatus = "running" | "waiting" | "done" | "failed" | "cancelled";
export interface RunStep {
  id: string; step: string; title?: string; kind: "agent" | "computer" | "human"; attempt: number; status: StepStatus;
  target?: string; message_id?: string; input?: string; payload?: unknown; output?: Record<string, unknown>; summary?: string; error?: string;
  retries?: number; bounced?: boolean; options?: string[]; started_at: string; ended_at?: string; due_at?: string;
  /** hubs from v0.21.1: send-backs, tasks sent, time taken (to now while open), cost */
  bounces?: number; attempts?: number; tasks?: string[]; duration_ms?: number; usage?: Usage;
}
export interface RunEvent { at: string; kind: string; step?: string; text: string }
/** What agents' runs cost, as their CLIs report it. */
export interface Usage { input_tokens?: number; output_tokens?: number; cost_usd?: number }
export interface RunMetrics {
  duration_ms: number; active_ms: number; waiting_ms: number; steps: number; done: number; failed: number;
  attempts: number; retries: number; bounces: number; usage?: Usage;
}
export interface RunArtifact {
  step: string; step_run: string; try: number; kind: "reply" | "progress"; message: string; from: string; at: string;
  file_id: string; name: string; size: number; content_type?: string;
}
export interface RunEvaluation {
  id: number; run: string; step?: string; name: string; value: number; unit?: string; direction?: "higher" | "lower" | "";
  note?: string; by?: string; at: string;
}
export interface WorkflowRun {
  id: string; workflow_id: string; name: string; version: number; project?: string; status: RunStatus;
  inputs?: Record<string, unknown>; owner?: string; by?: string; trigger?: string; error?: string; current?: string[];
  steps?: RunStep[]; events?: RunEvent[]; definition?: WorkflowDef; created_at: string; updated_at: string; ended_at?: string;
  /** hubs from v0.21.1 */
  duration_ms?: number; metrics?: RunMetrics; artifacts?: RunArtifact[]; evaluations?: RunEvaluation[];
}
export interface StepLogEntry {
  at: string; kind: "task" | "progress" | "reply" | "failure" | "event" | "stdout" | "stderr" | "trace" | string;
  sub?: string; try?: number; from?: string; text?: string; attachments?: Attachment[];
}
export interface StepLog { run: string; step: RunStep; entries: StepLogEntry[]; clipped?: boolean }
export interface Dist { n: number; avg: number; min: number; p50: number; p90: number; p95: number; max: number }
export interface WorkflowStepStats {
  step: string; title?: string; kind: string; runs: number; done: number; failed: number; retries: number; bounces: number;
  duration_ms: Dist; usage?: Usage; last_error?: string; last_error_run?: string; last_error_at?: string;
}
export interface WorkflowRunPoint {
  id: string; status: RunStatus; version: number; created_at: string; duration_ms: number; error?: string; failed_step?: string; cost_usd?: number;
}
export interface WorkflowStats {
  workflow: string; name: string; total: number; by_status: Partial<Record<RunStatus, number>>; success_rate: number; sample: number;
  version?: number; duration_ms: Dist; active_ms: Dist; waiting_ms: Dist; usage?: Usage; cost_per_run?: number;
  steps: WorkflowStepStats[];
  versions: { version: number; runs: number; succeeded: number; failed: number; cancelled: number; duration_ms: Dist }[];
  evaluations: { name: string; unit?: string; direction?: string; values: Dist; last: number; last_run: string }[];
  recent: WorkflowRunPoint[]; failures: WorkflowRunPoint[];
}
export interface RunCompareStep { status: StepStatus; runs: number; attempts: number; duration_ms: number; usage?: Usage; summary?: string; error?: string }
export interface RunComparison {
  a: { run: WorkflowRun; metrics: RunMetrics }; b: { run: WorkflowRun; metrics: RunMetrics }; same_version: boolean;
  steps: { step: string; title?: string; a?: RunCompareStep; b?: RunCompareStep; faster_in?: "a" | "b"; delta_ms: number; delta_cost_usd?: number }[];
  evaluations: { name: string; unit?: string; direction?: string; a?: number; b?: number; delta: number; better?: "a" | "b" }[];
}
export interface WorkflowApplyResponse { workflow: Workflow; created: boolean; changed: boolean; warnings?: string[] }
export interface WorkflowCheck { valid: boolean; problems?: string[]; warnings?: string[] }
export interface WorkflowDecision { run: WorkflowRun; step: RunStep }
export interface AutomationDetail { automation: Automation; runs: AutomationRun[] }
/** Handing a node over to another session (「换一个来接着做」, hubs after
 *  v0.19.22): the name and conversations stay, the session changes. */
export interface HandoverSide { binding: Binding; machine_id?: string; machine_name?: string; runtime?: Runtime }
export interface Handover {
  id: string; agent: string; at: string; by?: string; note?: string;
  from: HandoverSide; to: HandoverSide;
  /** waiting: a command made, nobody ran it yet; expired: not run in time, or another took over */
  state: "waiting" | "done" | "expired";
  done_at?: string;
  /** tasks that went back to the queue when it was taken */
  open_tasks: string[];
  /** the handover this one undid (a switch back) */
  back?: string;
  expires_at: string;
}
export interface HandoverLink { handover: Handover; url?: string; command?: string; command_ps1?: string }
export interface HandoverRequest { machine: string; cli?: string; dir?: string; model?: string; access?: RuntimeAccess; note?: string; back?: string }
export interface HandoverResult {
  handover: Handover;
  spawn?: { agent: string; session_id: string; cli: string; workdir: string; access: RuntimeAccess; reply: string };
}
export interface Attachment { file_id: string; name: string; size: number; sha256: string; content_type?: string }
export interface Message {
  id: string; from: string; to: string; thread_id: string; reply_to?: string; after?: string; type: string; body: string;
  status: string; attempts: number; error?: string; broadcast_id?: string; project?: string; attachments?: Attachment[];
  /** the automation that sent it (hubs from v0.19.22) */
  automation?: string;
  /** On a task a workflow sent: its run (hubs from v0.21). */
  workflow?: string;
  /** On a reply: the structured result handed back with it. */
  result?: Record<string, unknown>;
  /** why the hub let it past its recipient's allow list ("project:<name>": a
   *  project with a terminal on that computer; hubs from v0.21.2) */
  granted_by?: string;
  /** the person behind the sender (member name) */
  from_owner?: string;
  /** a progress update asking the task's sender to decide (body: the
   *  question, then the choices as a numbered list) */
  ask?: boolean;
  /** on a task: the question it waits on */
  asking?: string;
  /** On a task: what its recipient thinks you may want to say next (v0.19.15+); the input offers it, Tab takes it. */
  suggest?: string;
  /** on a steer: the question it answers */
  answers?: string;
  /** end-to-end encrypted (decrypted by the app when it may read it) */
  e2e?: boolean;
  /** why the app could not decrypt it */
  unreadable?: string;
  /** when an accepted task last had activity: its runner touches it while the
   *  run goes on. Absent on hubs and messages without a runner. */
  active_at?: string;
  created_at: string; updated_at: string;
}
/** Another node a node exchanged messages with. */
export interface Peer { agent: string; messages: number; last?: Message }
export type State = "working" | "pending" | "idle" | "offline";
export interface AgentStatus { agent: Agent; state: State; queued: number; in_progress: number; held: number;
  /** held tasks waiting on a question to their sender (absent on older hubs) */
  asking?: number;
  /** acked tasks that show no life (the agent is away, or none was touched
   *  for 5 min under a runner / 30 in a session): open, not in in_progress,
   *  and not making it "working". Absent on hubs before v0.20.28. */
  stale?: number;
  stale_tasks?: string[];
  current?: Message[];
  /** how it receives right now: a session keeps `inbox wait` running, or a runner wakes the session per message */
  receiving?: "session" | "runner";
  /** the runner recently found the session in use and handed messages back */
  session_in_use?: boolean }
export interface Overview {
  generated_at: string; viewer: { kind: "agent" | "admin"; name?: string }; online_window_seconds: number;
  totals: { agents: number; working: number; pending: number; idle: number; offline: number };
  agents: AgentStatus[]; machines?: Machine[]; recent: Message[];
}
export interface Session {
  hub: string; version: string; agent?: string; admin: boolean;
  hub_version?: string; latest_version?: string; update_available?: boolean;
}
export type ViewAs = "agent" | "admin";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" } });
  const text = await r.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!r.ok) {
    const e = data as { message?: string; suggestion?: string } | null;
    throw new Error([e?.message || `HTTP ${r.status}`, e?.suggestion].filter(Boolean).join(" — "));
  }
  return data as T;
}

export const api = {
  session: () => get<Session>("api/session"),
  overview: (as: ViewAs) => get<Overview>(`api/overview?as=${as}`),
  thread: (id: string, as: ViewAs) => get<{ messages: Message[] }>(`api/threads/${encodeURIComponent(id)}?as=${as}`),
};

/** A person on a hub. Members manage what they own; owner and admins manage everything. */
export type MemberRole = "owner" | "admin" | "member";
export interface Member { id: string; name: string; display_name?: string; role: MemberRole; created_at: string; removed?: boolean }
/** Who this app is on a hub (absent on hubs without members). */
/** One device (app) of a person on a hub: its own credential. */
export interface Device { id: string; name: string; active: boolean; created_at: string; last_used_at?: string }
/** A hub's backups (its owner; hubs from v0.21.12). */
export interface Backup { name: string; size: number; created_at: string }
export interface Backups { backups: Backup[]; keep: number; daily: boolean; restore?: string }
export interface Whoami { member?: Member; admin?: boolean; can_manage: boolean; /** this app on this device */ device?: Device }
export interface AuditEntry { id: string; at: string; actor: string; actor_kind: "member" | "admin" | "agent"; action: string; target?: string; detail?: string }

/** Superagents (hubs from v0.21.1): a person's own agent with the App's powers. */
export interface SuperagentState { agent?: Agent; pending?: number }
export interface Approval {
  id: string; agent: string; what: string; reason?: string; method: string; path: string; body?: string;
  status: "pending" | "done" | "denied" | "expired" | "withdrew"; result?: number; result_body?: string; decided_by?: string;
  created_at: string; expires_at: string; decided_at?: string;
}

// Optional remote desktop signaling. No image, audio or input payloads use the Hub API.
export type RemoteDevice = {machine:string; name:string; os:string; control:boolean};
export type RemoteDescription = {type:"offer"|"answer"; sdp:string};
export type RemoteOffer = {description:RemoteDescription; pairing_code?:string; control:boolean};
export type RemoteSession = {id:string; machine:string; state:"pending"|"connected"|"failed"|"closed"; answer?:RemoteDescription; error?:string};
