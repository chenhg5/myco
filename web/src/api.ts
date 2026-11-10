// Types mirror internal/api (the hub's JSON contract).
export interface Binding { harness?: string; session_id?: string; environment?: string; workdir?: string }
export interface Delivery { mode: string; url?: string; has_secret?: boolean; params?: Record<string, string> }

/** What a node is. `machine` is a machine node (`xxx-machine`), not shown as a node. */
export type AgentKind = "agent" | "shell" | "human" | "machine";
export type RuntimeAccess = "readonly" | "workspace" | "full";
export type ModelSource = "configured" | "cli-config" | "observed" | "default" | "provider";
/** pinned: chosen for this node; machine: the machine's current setting; env_file: entered by hand; missing: the pinned one is gone. */
export type ProviderSource = "pinned" | "machine" | "env_file" | "missing";
/** What the node actually runs; absent on nodes reported by old runners. */
export interface Runtime {
  cli: string;
  cli_version?: string;
  model?: string;
  model_source?: ModelSource;
  access?: RuntimeAccess;
  /** model provider's name (never its key) */
  provider?: string;
  provider_source?: ProviderSource;
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
}
export interface ProjectLink { title?: string; url: string }
export interface ProjectMember {
  name: string; role: "member" | "coordinator"; note?: string; added_by?: string; added_at: string;
  /** its card, when the viewer may see the node */
  agent?: Agent;
}
export interface ProjectDoc {
  name: string; size: number; updated_by?: string; updated_at: string; body?: string;
  /** set for uploaded files (from v0.18.29) */
  content_type?: string;
  /** not text: download it (files/{name}) */
  binary?: boolean;
}
export interface Agent {
  id: string; name: string; display_name?: string; aliases?: string[]; description: string;
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
  /** the person who added it (member name) */
  owner?: string; owner_id?: string;
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
export interface Attachment { file_id: string; name: string; size: number; sha256: string; content_type?: string }
export interface Message {
  id: string; from: string; to: string; thread_id: string; reply_to?: string; type: string; body: string;
  status: string; attempts: number; error?: string; broadcast_id?: string; project?: string; attachments?: Attachment[];
  /** the person behind the sender (member name) */
  from_owner?: string;
  /** end-to-end encrypted (decrypted by the app when it may read it) */
  e2e?: boolean;
  /** why the app could not decrypt it */
  unreadable?: string;
  created_at: string; updated_at: string;
}
/** Another node a node exchanged messages with. */
export interface Peer { agent: string; messages: number; last?: Message }
export type State = "working" | "pending" | "idle" | "offline";
export interface AgentStatus { agent: Agent; state: State; queued: number; in_progress: number; held: number; current?: Message[];
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
export interface Whoami { member?: Member; admin?: boolean; can_manage: boolean; /** this app on this device */ device?: Device }
export interface AuditEntry { id: string; at: string; actor: string; actor_kind: "member" | "admin" | "agent"; action: string; target?: string; detail?: string }
