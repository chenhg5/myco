import type { Agent, AuditEntry, Device, Machine, Member, MemberRole, Message, Overview, Peer, RuntimeAccess, Whoami } from "@/api";

// Types mirror the App backend (cmd/myco-app) which proxies to enrolled hubs.
export interface Hub {
  id: string;
  name: string;
  url: string;
  ok: boolean;
  error?: string;
  hub_version?: string;
  update_available?: boolean;
}

export interface HubOverview {
  id: string;
  name: string;
  ok: boolean;
  error?: string;
  overview?: Overview;
  /** who this app is on the hub (absent on hubs without members) */
  me?: Whoami;
  /** the hub has end-to-end encryption on */
  e2e?: boolean;
}

export interface AppOverview {
  generated_at: string;
  hubs: HubOverview[];
}

export interface NewHub { name: string; url: string; token: string }
/** A console link (`https://host/c/myco_console_…`) added to the App. */
export interface NewHubLink { name?: string; link: string }
export interface PendingLink { link: string }
export interface ProfileInput { display_name?: string; description?: string }

/** CLIs that a machine can be asked to start a new session with. */
/** An agent CLI to start a session with, or "exec": a shell node that runs the commands it is sent. */
export type SpawnCli = "claude" | "codex" | "pi" | "kiro" | "cursor" | "gemini" | "opencode" | "exec";
export interface SpawnInput {
  cli: SpawnCli;
  /** Directory on that machine; `~/...` is that machine's home directory. */
  dir: string;
  name?: string;
  description?: string;
  model?: string;
  access?: RuntimeAccess;
  /** A shell node's allowed senders (cli "exec"): agents that may send it commands. */
  allow_from?: string[];
  /** copy this session (same CLI, on that machine) instead of starting an empty one */
  fork_from?: string;
  display_name?: string;
}
export interface SpawnResult {
  agent: Agent;
  session_id: string;
  cli: string;
  workdir: string;
  access: RuntimeAccess;
  /** The new session's first reply. */
  reply: string;
}
export interface JoinLinkInput { uses?: number; expires_hours?: number }
export interface JoinLink { join_url: string; command: string }

/** A Claude Code or Codex session on this computer, found from its transcript. */
export interface LocalSession {
  cli: "claude" | "codex";
  session_id: string;
  workdir: string;
  last_active: string;
  /** The session's first request, shortened; may be empty. */
  title?: string;
  /** Set when this session is already an agent somewhere. */
  joined?: { agent: string; hub: string };
}
export interface LocalJoinInput {
  cli: string;
  session_id: string;
  workdir: string;
  display_name?: string;
  /** join a copy of the session (default in the app): the original stays the user's */
  fork?: boolean;
}
export interface LocalJoinResult {
  agent: string;
  created: boolean;
  /** "running" when this computer's runner serves the new agent. */
  runner: string;
}

/** One console conversation: a thread with an agent, newest activity first. */
export interface Conversation {
  thread_id: string;
  agent?: string;
  updated_at: string;
  first: Message;
  last: Message;
  messages: number;
}
export interface ConversationQuery { agent?: string; q?: string; before?: number }
/** One row of the recent list: a thread, or an agent's exchange with a shell node. */
export interface RecentItem {
  kind: "thread" | "pair";
  thread_id: string;
  /** pair: the agent (or console) and the shell node */
  other?: string;
  node?: string;
  participants: string[];
  updated_at: string;
  messages: number;
  first: Message;
  last: Message;
  task?: Message;
  answer?: Message;
  /** queued | working | done | failed, or "" without a task */
  state?: "queued" | "working" | "done" | "failed" | "";
}
export interface RecentQuery { scope?: "mine"; label?: string; q?: string; before?: number }
/** fallback: the hub predates the recent list; use its conversations. */
export interface RecentList { items: RecentItem[]; more: boolean; fallback?: boolean }
export interface SendMessageInput {
  to: string;
  body: string;
  /** Omit to start a new conversation. */
  thread_id?: string;
  type?: string;
}

/** How the App window is hosted: native macOS shell or a plain browser. */
export type Shell = "mac" | "browser";
export interface AppInfo { version: string; shell: Shell }
/** Everything the boot probe needs to decide the mode and lay out the shell. */
export interface AppBootstrap { hubs: Hub[]; info: AppInfo }

/** The myco CLI installed on this machine. */
export interface MycoInstall { path: string; version: string }
/** A Hub running on this machine (started from the App). */
export interface LocalHub {
  running: boolean;
  local_url: string;
  public_url?: string;
  version?: string;
  /** Whether it is already in the App's hub list. */
  added: boolean;
}
export interface LocalTool { path: string }
export interface LocalTools { cloudflared: LocalTool | null; tailscale: LocalTool | null }
export interface LocalState {
  myco: MycoInstall | null;
  hub: LocalHub | null;
  tools: LocalTools;
  device: string;
  /** This computer's host name, as its runners report it for their machine. */
  hostname?: string;
  /** This computer's machine names, one per hub it joined. */
  machines?: string[];
  /** This computer allows shell nodes and full-access sessions (spawn.max_access: full). */
  shell_allowed?: boolean;
}
export type LocalTunnel = "cloudflare" | "tailscale" | "none";
export interface StartLocalHubInput { tunnel: LocalTunnel; hostname?: string }
export interface StartLocalHubResult {
  hub: Hub;
  public_url: string;
  join_url?: string;
  warnings: string[];
}

/** An error returned by the App backend or a hub (same shape as the hub API). */
export class ApiError extends Error {
  status: number;
  code?: string;
  suggestion?: string;
  constructor(message: string, status: number, code?: string, suggestion?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.suggestion = suggestion;
  }
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : String(e);
}

export function errorSuggestion(e: unknown): string | undefined {
  return e instanceof ApiError ? e.suggestion : undefined;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const r = await fetch(path, {
    method,
    credentials: "same-origin",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await r.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!r.ok) {
    const e = (data ?? {}) as { error?: string; message?: string; suggestion?: string };
    throw new ApiError(e.message || `HTTP ${r.status}`, r.status, e.error, e.suggestion);
  }
  return data as T;
}

const enc = encodeURIComponent;

async function probeAppInfo(): Promise<AppInfo> {
  try {
    const r = await fetch("app/info", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!r.ok) return { version: "", shell: "browser" };
    const d = (await r.json()) as Partial<AppInfo> | null;
    return { version: typeof d?.version === "string" ? d.version : "", shell: d?.shell === "mac" ? "mac" : "browser" };
  } catch {
    return { version: "", shell: "browser" };
  }
}

/**
 * Probe for App mode. Returns the hub list when the App backend answers 200;
 * otherwise null, which means "plain `myco ui`" and the existing UI is used.
 */
export async function probeAppMode(): Promise<AppBootstrap | null> {
  try {
    const r = await fetch("app/hubs", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!r.ok) return null;
    const data: unknown = await r.json();
    if (!Array.isArray(data)) return null;
    return { hubs: data as Hub[], info: await probeAppInfo() };
  } catch {
    return null;
  }
}

/** GET /app/update: the app's own version against the latest release. */
export interface AppUpdate {
  current: string;
  latest?: string;
  available: boolean;
  can_install: boolean;
  reason?: string;
  error?: string;
}

export const appApi = {
  info: () => request<AppInfo>("GET", "app/info"),
  update: () => request<AppUpdate>("GET", "app/update"),
  /** Downloads and installs the latest app, then the app reopens itself. */
  installUpdate: () => request<{ installed: string; relaunching: boolean }>("POST", "app/update"),
  hubs: () => request<Hub[]>("GET", "app/hubs"),
  addHub: (input: NewHub) => request<Hub>("POST", "app/hubs", input),
  /** Adds a hub from the App link printed by the hub installer. */
  addHubByLink: (input: NewHubLink) => request<Hub>("POST", "app/hubs", input),
  /** A console link the App was opened with, consumed once. */
  pendingLink: () => request<PendingLink>("GET", "app/pending-link"),
  removeHub: (id: string) => request<void>("DELETE", `app/hubs/${enc(id)}`),
  overview: () => request<AppOverview>("GET", "app/overview"),
  thread: (hubId: string, threadId: string) =>
    request<{ messages: Message[] }>("GET", `app/hubs/${enc(hubId)}/threads/${enc(threadId)}`),
  /** The hub's settings (end-to-end encryption); the owner may change them. */
  hubSettings: (hubId: string) => request<{ e2e: boolean }>("GET", `app/hubs/${enc(hubId)}/settings`),
  setHubSettings: (hubId: string, input: { e2e: boolean }) => request<{ e2e: boolean }>("PUT", `app/hubs/${enc(hubId)}/settings`, input),
  /** The nodes a node exchanged messages with (not the console), most recent first. */
  peers: (hubId: string, name: string) =>
    request<{ peers: Peer[] }>("GET", `app/hubs/${enc(hubId)}/agents/${enc(name)}/peers`),
  /** Messages between two nodes, newest first; before (unix ms) pages back. */
  exchanges: (hubId: string, a: string, b: string, before?: number) =>
    request<{ messages: Message[]; more: boolean }>("GET", `app/hubs/${enc(hubId)}/exchanges?a=${enc(a)}&b=${enc(b)}${before ? `&before=${before}` : ""}`),
  /** Removes (or, with dry_run, lists) nodes offline for offline_days or more that you may manage. */
  cleanup: (hubId: string, offlineDays: number, dryRun: boolean) =>
    request<{ agents: Agent[]; removed: boolean; dry_run?: boolean }>("POST", `app/hubs/${enc(hubId)}/agents/cleanup`, { offline_days: offlineDays, dry_run: dryRun }),

  /** Claude Code / Codex sessions on this computer, newest first. */
  localSessions: () => request<{ sessions: LocalSession[] }>("GET", "app/local/sessions"),
  /** Makes one of those sessions an agent of the hub. */
  localJoin: (hubId: string, input: LocalJoinInput) =>
    request<LocalJoinResult>("POST", `app/hubs/${enc(hubId)}/local-join`, input),

  conversations: (hubId: string, query: ConversationQuery = {}) => {
    const q = new URLSearchParams();
    if (query.agent) q.set("agent", query.agent);
    if (query.q) q.set("q", query.q);
    if (query.before) q.set("before", String(query.before));
    const qs = q.toString();
    return request<{ conversations: Conversation[] }>(
      "GET", `app/hubs/${enc(hubId)}/conversations${qs ? `?${qs}` : ""}`,
    );
  },
  /** The recent list: threads and shell-node exchanges the caller may read. */
  recent: (hubId: string, query: RecentQuery = {}) => {
    const q = new URLSearchParams();
    if (query.scope) q.set("scope", query.scope);
    if (query.label && query.label !== "all") q.set("label", query.label);
    if (query.q) q.set("q", query.q);
    if (query.before) q.set("before", String(query.before));
    const qs = q.toString();
    return request<RecentList>("GET", `app/hubs/${enc(hubId)}/recent${qs ? `?${qs}` : ""}`);
  },
  /** Sends a task from this console; without thread_id it starts a conversation. */
  sendMessage: (hubId: string, input: SendMessageInput) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages`, input),
  /** Corrects an unfinished task the console sent. */
  steer: (hubId: string, messageId: string, body: string) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/steer`, { body }),
  /** Closes an open message for its recipient without replying (owner or admin). */
  dismissMessage: (hubId: string, messageId: string) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/dismiss`),
  updateProfile: (hubId: string, name: string, input: ProfileInput) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/profile`, input),
  /** Adds and removes labels (group=… and tags) of a node. */
  setLabels: (hubId: string, name: string, change: { add?: string[]; remove?: string[] }) =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/labels`, change),
  updateAllowFrom: (hubId: string, name: string, allowFrom: string[]) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/allow_from`, { allow_from: allowFrom }),
  kick: (hubId: string, name: string) =>
    request<void>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/kick`),
  removeAgent: (hubId: string, name: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/agents/${enc(name)}`),

  /** Starts a session on a machine; the machine may take up to two minutes. */
  spawn: (hubId: string, machineId: string, input: SpawnInput) =>
    request<SpawnResult>("POST", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/spawn`, input),
  renameMachine: (hubId: string, machineId: string, name: string) =>
    request<Machine>("PATCH", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}`, { name }),
  /** Creates a one-off enrollment link for adding another machine to this hub. */
  createJoinLink: (hubId: string, input: JoinLinkInput = {}) =>
    request<JoinLink>("POST", `app/hubs/${enc(hubId)}/links`, input),

  /** This person's devices on a hub (each has its own credential). */
  devices: (hubId: string) => request<{ devices: Device[] }>("GET", `app/hubs/${enc(hubId)}/devices`),
  removeDevice: (hubId: string, deviceId: string) => request<{ ok: boolean }>("DELETE", `app/hubs/${enc(hubId)}/devices/${enc(deviceId)}`),
  /** A one-time link (24 hours) to sign in on another device as yourself. */
  createPairing: (hubId: string) => request<{ link: string; expires_at: string; command?: string }>("POST", `app/hubs/${enc(hubId)}/pairings`),
  members: (hubId: string, all = false) =>
    request<{ members: Member[] }>("GET", `app/hubs/${enc(hubId)}/members${all ? "?all=1" : ""}`),
  /** Adds a person; the link opens the hub in their app as them (shown once). */
  inviteMember: (hubId: string, input: { name: string; display_name?: string; role?: MemberRole }) =>
    request<{ member: Member; link: string; command?: string }>("POST", `app/hubs/${enc(hubId)}/members`, input),
  updateMember: (hubId: string, member: string, input: { name?: string; display_name?: string; role?: MemberRole }) =>
    request<Member>("PATCH", `app/hubs/${enc(hubId)}/members/${enc(member)}`, input),
  removeMember: (hubId: string, member: string) =>
    request<Member>("DELETE", `app/hubs/${enc(hubId)}/members/${enc(member)}`),
  setAgentOwner: (hubId: string, name: string, owner: string) =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/owner`, { owner }),
  setMachineOwner: (hubId: string, machineId: string, owner: string) =>
    request<{ ok: boolean }>("PUT", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/owner`, { owner }),
  audit: (hubId: string, q: { actor?: string; target?: string; before?: number; limit?: number } = {}) => {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v) p.set(k, String(v));
    const qs = p.toString();
    return request<{ entries: AuditEntry[] }>("GET", `app/hubs/${enc(hubId)}/audit${qs ? `?${qs}` : ""}`);
  },

  local: () => request<LocalState>("GET", "app/local"),
  /** cc-switch's Claude providers on this computer (names and hosts only). */
  providers: (app: "claude" | "codex" = "claude") => request<ProviderList>("GET", `app/local/providers?app=${app}`),
  /** A node's provider setting, if the node runs on this computer. */
  agentProvider: (name: string, hubId: string) =>
    request<AgentProvider>("GET", `app/local/agents/${enc(name)}/provider?hub_id=${enc(hubId)}`),
  setAgentProvider: (name: string, hubId: string, body: { provider?: string; manual?: ManualProvider }) =>
    request<AgentProvider>("PUT", `app/local/agents/${enc(name)}/provider?hub_id=${enc(hubId)}`, body),
  /** Starts this computer's runner service, or restarts it when running. */
  restartRunner: () => request<{ runner: string }>("POST", "app/local/runner"),
  /** Allow (or stop allowing) shell nodes and full-access sessions on this computer. */
  shellAccess: (allow: boolean) => request<{ shell_allowed: boolean }>("POST", "app/local/shell-access", { allow }),
  /** Downloads and verifies the myco CLI; can take up to a minute. */
  installMyco: () => request<MycoInstall>("POST", "app/local/myco"),
  /** Starts a Hub on this machine (and adds it to the App); can take up to 90s. */
  startLocalHub: (input: StartLocalHubInput) =>
    request<StartLocalHubResult>("POST", "app/local/hub", input),
  stopLocalHub: () => request<{ stopped: true }>("POST", "app/local/hub/stop"),
};

/** A cc-switch provider (keys are never sent to the page). */
export interface ProviderInfo { ref: string; id: string; name: string; app: string; current?: boolean; host?: string; model?: string }
export interface ProviderList { cc_switch: boolean; providers: ProviderInfo[]; error?: string }
/** provider: "" follows this computer; "cc-switch:<id>" is pinned; "env_file" was entered by hand. */
export interface AgentProvider { local: boolean; provider: string; harness?: string; env_host?: string }
export interface ManualProvider { base_url: string; api_key: string; model?: string }
