import type { Page, PageChanges, PageOpen, PageRecord, PageState, Pin, Template, PinCreate, PinList, Automation, AutomationDetail, Handover, HandoverLink, HandoverRequest, HandoverResult, AutomationRequest, AutomationResponse, AutomationRun, Agent, AuditEntry, Project, ProjectDoc, ProjectLink, Skill, Device, Machine, Member, MemberRole, Message, Overview, Peer, RuntimeAccess, Whoami, Trace } from "@/api";
import { L } from "@/lib/i18n";

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

/** Voice calls: what the person chose (no keys) and whether this App can. */
export interface VoiceConfig {
  llm_base_url?: string;
  llm_model?: string;
  /** the speech service the voice speaks with (one of VoiceInfo.tts_providers) */
  tts_provider?: string;
  tts_speaker?: string;
  /** how fast it talks, -50..100 (0 = normal) */
  tts_rate?: number;
}
/** A speech service this App can speak with: the settings page draws these. */
export interface VoiceTTSProvider {
  id: string;
  name: string;
  name_en: string;
  default_speaker?: string;
  help?: string;
  help_en?: string;
  /** the voices to choose from (another may still be typed in, like a cloned one) */
  voices?: TTSVoice[];
  /** this App holds a key for it */
  has_key: boolean;
}
/** One voice on a speech service's list. lang: zh, en, ja, … (mx = Mexican Spanish, tl = Filipino). */
export interface TTSVoice { id: string; name: string; lang: string; gender: "female" | "male" }
/** How one workspace's hub stands with the voice settings kept on it. */
export interface VoiceSyncHub { id: string; name: string; state: "synced" | "unsupported" | "unreachable" }
export type VoiceState = "idle" | "listening" | "thinking" | "speaking";
export interface VoiceInfo {
  /** this App has a microphone and speaker for calls (the Mac app) */
  available: boolean;
  config: VoiceConfig;
  has_llm_key: boolean;
  has_tts_key: boolean;
  session: { on: boolean; state: VoiceState; muted: boolean; hub_id?: string; agent?: string };
  /** a call can start: a microphone here, the model and the voice set up (v0.19.12+) */
  ready?: boolean;
  /** the speech services to choose from (absent before v0.19.15: Doubao alone) */
  tts_providers?: VoiceTTSProvider[];
  /** the settings (keys too) are kept on the workspaces' hubs for the person's other devices */
  share?: boolean;
  /** how each hub stood at the last look */
  sync?: VoiceSyncHub[];
}
/** One line of a call: id stays the line's own; seq orders its changes. */
export interface VoiceLine {
  seq: number;
  id: number;
  kind: "user" | "hearing" | "assistant" | "action" | "notice";
  text: string;
  interrupted?: boolean;
  task?: { hub_id: string; id: string; agent?: string; thread_id?: string };
  at: string;
}
export interface VoiceEvents { state: VoiceState; on: boolean; muted?: boolean; seq: number; events: VoiceLine[] }

/** The workspaces of a hub's server: whether it serves several (v0.19.9+),
 *  and whether this App may make more there (it signs in as the owner of
 *  the server's default workspace). */
export interface WorkspaceState {
  supported: boolean;
  allowed: boolean;
  /** the server's own address */
  server: string;
  workspaces: { name: string; url: string; created_at: string }[];
}

/** A computer already on this hub's server, in another of its workspaces. */
export interface ServerMachine {
  hub_id: string;
  hub_name: string;
  machine_id: string;
  name: string;
  hostname?: string;
  os?: string;
  online: boolean;
  can_spawn: boolean;
}

export interface HubOverview {
  id: string;
  name: string;
  ok: boolean;
  error?: string;
  /** the hub's version, from the App's hub list (not its overview) */
  hub_version?: string;
  overview?: Overview;
  /** who this app is on the hub (absent on hubs without members) */
  me?: Whoami;
  /** the hub has end-to-end encryption on */
  e2e?: boolean;
  /** set by the App: the hub could not be reached, and this is what it
   *  showed last (at: when; error: why it is unreachable now) */
  stale?: { at: string; error?: string };
}

export interface AppOverview {
  generated_at: string;
  hubs: HubOverview[];
}

export interface NewHub { name: string; url: string; token: string }
/** A console link (`https://host/c/myco_console_…`) added to the App. */
export interface NewHubLink { name?: string; link: string }
export interface PendingLink { link: string }
/** What deleting a machine removes: the machine and the nodes on it. */
export interface MachineRemoval {
  machine: Machine;
  /** node names (display names are resolved by the App) */
  removed: string[];
  dry_run: boolean;
}
export interface ProfileInput {
  display_name?: string; description?: string;
  /** "" (the default face), "look:..." or a picture as a data: URL */
  avatar?: string;
}

/** CLIs that a machine can be asked to start a new session with. */
/** An agent CLI to start a session with, or "exec": a shell node that runs the commands it is sent. */
export type SpawnCli = "claude" | "codex" | "pi" | "kiro" | "cursor" | "gemini" | "opencode" | "exec"
  | "qoder" | "qwen" | "kimi" | "goose" | "copilot" | "auggie" | "codebuddy" | "kilo" | "cline" | "grok";
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
/** What a computer allows and has, as its machine node says
 *  (GET /app/hubs/{id}/machines/{mid}/info; "too_old" from an older myco). */
export interface MachineInfo {
  /** folders sessions may start in, as written there ("~/code/x"); ["~"]: anywhere in its home */
  dirs: string[];
  /** where a new conversation's folder goes when none is given ("~/myco") */
  new_dir: string;
  /** the highest access a session there may get */
  max_access: RuntimeAccess;
  ais: MachineAI[];
  sessions: MachineSession[];
  /** its home directory (what "~" stands for there) */
  home?: string;
}
export interface MachineAI {
  cli: string;
  name: string;
  /** missing | signed_out | ready | unknown */
  state: string;
  version?: string;
  /** commands to run on that computer */
  install?: string;
  login?: string;
}
export interface MachineSession {
  cli: "claude" | "codex";
  session_id: string;
  workdir: string;
  last_active: string;
  title?: string;
  /** the agent it already is in this workspace */
  joined?: string;
  joined_elsewhere?: boolean;
  /** a program ran it (claude -p, codex exec, myco), not a person */
  auto?: boolean;
}
export interface SpawnResult {
  /** the new node's name */
  agent: string;
  session_id: string;
  cli: string;
  workdir: string;
  access: RuntimeAccess;
  /** The new session's first reply. */
  reply: string;
}
export interface JoinLinkInput {
  uses?: number; expires_hours?: number;
  /** the computer that runs the command also lets agents open terminals on it
   *  (myco machine join --allow-shell): running it there is that computer's consent */
  allow_shell?: boolean;
  /** a shell-node link: the folder the command is run in becomes a computer
   *  (exec node) that these agents may send commands to */
  shell_node?: boolean;
  allow_from?: string[];
}
export interface JoinLink {
  join_url: string;
  command: string;
  /** the same for Windows PowerShell (newer App backends) */
  ps_command?: string;
  /** the command also lets that computer run terminals (?shell=1) */
  allow_shell?: boolean;
  /** the hub's join script ends with the agent's last step, and
   *  join_url + "/" + project joins that project too (older hubs: absent) */
  agent_guide?: boolean;
}

/** A Claude Code or Codex session on this computer, found from its transcript. */
export interface LocalSession {
  cli: "claude" | "codex";
  session_id: string;
  workdir: string;
  last_active: string;
  /** The session's first request, shortened; may be empty. */
  title?: string;
  /** Set when this session is already an agent somewhere (the first one). */
  joined?: { agent: string; hub: string; hub_name?: string };
  /** Every workspace it joined (one session can be an agent of several). */
  joined_all?: { agent: string; hub: string; hub_name?: string }[];
  /** a program ran it (claude -p, codex exec, myco), not a person */
  auto?: boolean;
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
  /** the newest task not finished yet: can be older than task (one stuck
   *  behind a newer task that finished). Hubs before v0.18.40 leave it out. */
  open?: Message;
  /** queued | working | done | failed, or "" without a task */
  state?: "queued" | "working" | "done" | "failed" | "";
  /** the project the thread belongs to */
  project?: string;
}
/** How the App reaches out: the environment's proxy, none, or one set here. */
export interface ProxySetting {
  mode: "system" | "none" | "manual";
  type?: "http" | "https" | "socks5";
  host?: string;
  port?: number;
  user?: string;
  /** a password is kept in the keychain (never sent back) */
  has_password?: boolean;
}
export type ProxyInput = Omit<ProxySetting, "has_password"> & { password?: string; clear_password?: boolean };
export interface RecentQuery {
  scope?: "mine"; label?: string; q?: string; before?: number; project?: string;
  /** "pair": one row per pair of parties (hubs before v0.18.28 list threads) */
  group?: "pair";
  /** with group "pair": "me" keeps the pairs you are in, "others" the rest */
  side?: "me" | "others";
}
/** fallback: the hub predates the recent list; use its conversations. */
export interface RecentList { items: RecentItem[]; more: boolean; fallback?: boolean }
export interface SendMessageInput {
  to: string;
  body: string;
  /** Omit to start a new conversation. */
  thread_id?: string;
  /** Deliver after this task ends (v0.19.2). */
  after?: string;
  type?: string;
  /** Puts a new conversation in a project. */
  project?: string;
  /** files uploaded with uploadAttachment, sent with the message */
  attachments?: string[];
}

/** How the App window is hosted: native macOS shell or a plain browser. */
export type Shell = "mac" | "browser";
export interface AppInfo {
  version: string; shell: Shell;
  /** the desktop app does speech to text (POST app/dictation) */
  dictation?: boolean;
  /** the desktop app asks for a face's picture with its own panel (macOS) */
  pick_image?: boolean;
  /** the desktop app opens links in the default browser (POST /app/open-url) */
  open_url?: boolean;
}
/** Everything the boot probe needs to decide the mode and lay out the shell. */
export interface AppBootstrap { hubs: Hub[]; info: AppInfo }

/** The myco CLI installed on this machine. */
export interface MycoInstall { path: string; version: string }
/** A Hub running on this machine (started from the App). */
export interface LocalHub {
  /** The name `myco hub up --name` gave it; "" is the default one. Absent on
   *  older backends, which know a single local hub. */
  name?: string;
  running: boolean;
  local_url: string;
  public_url?: string;
  version?: string;
  /** Whether it is already in the App's hub list. */
  added: boolean;
}
export interface LocalTool { path: string }
export interface LocalTools { cloudflared: LocalTool | null; tailscale: LocalTool | null }
/** An agent CLI on this computer (GET /app/local/agents). */
export interface LocalAgent {
  cli: string;
  name: string;
  /** missing, signed_out, ready, or unknown (installed; sign-in not visible without spending quota) */
  state: "missing" | "signed_out" | "ready" | "unknown";
  path?: string;
  version?: string;
  install: string;
  install_mirror?: string;
  login: string;
  /** a program the install command needs ("npm") */
  needs?: string;
}

export interface LocalState {
  myco: MycoInstall | null;
  /** The default hub on this computer (older backends have only this one). */
  hub: LocalHub | null;
  /** Every hub on this computer (v0.19.4+); fall back to `hub` when absent. */
  hubs?: LocalHub[];
  tools: LocalTools;
  device: string;
  /** This computer's host name, as its runners report it for their machine. */
  hostname?: string;
  /** This computer's machine names, one per hub it joined. */
  machines?: string[];
  /** this computer's machine nodes, one per workspace it joined: name exactly which computer is this one */
  machine_nodes?: string[];
  /** This computer allows shell nodes and full-access sessions (spawn.max_access: full). */
  shell_allowed?: boolean;
  /** this computer's own IP addresses (LAN, Tailscale…), loopback left out */
  addrs?: string[];
}
export type LocalTunnel = "cloudflare" | "tailscale" | "none";
export interface StartLocalHubInput { tunnel: LocalTunnel; hostname?: string; /** Another hub on this computer (`myco hub up --name`); omit for the default one. */ name?: string }
export interface StartLocalHubResult {
  hub: Hub;
  public_url: string;
  join_url?: string;
  warnings: string[];
}

/** An error returned by the App backend or a hub (same shape as the hub API). */
export class ApiError extends Error {
  status: number;
  /** what a conflict answers with (a page's latest state) */
  latest?: unknown;
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

/** The hub could not be reached or answered too slowly: a later try may
 *  well work, and the raw text ("context deadline exceeded") means nothing
 *  to a person. */
export function isTransient(e: unknown): boolean {
  if (e instanceof ApiError) {
    if (e.code === "unavailable" || e.code === "timeout") return true;
    // A timeout or a dropped connection, whatever code the console put on
    // it ("context deadline exceeded" under "internal", say).
    if (/deadline exceeded|timed? ?out|connection (refused|reset)|no such host|\bEOF\b/i.test(e.message)) return true;
    // An error with a code of its own (fork_failed, say) is that error,
    // whatever the status: not a hub that is down or slow.
    if (e.code) return false;
    if (e.status === 502 || e.status === 503 || e.status === 504) return true;
    return /deadline exceeded|timed? ?out|connection (refused|reset)|no such host|EOF/i.test(e.message);
  }
  return e instanceof TypeError; // fetch itself failed
}

/** The hub is older than the app and lacks this feature: it answers an
 *  unknown path with 404 ("no route for …", or Go's plain "404 page not
 *  found") or 405. A real "agent x not found" is not this. */
export function isOldHub(e: unknown): boolean {
  if (!(e instanceof ApiError)) return false;
  if (e.status === 405) return true;
  return e.status === 404 && /no route for|404 page not found|^HTTP 404$/i.test(e.message);
}

/** A hub's status error (raw text from the App backend) for people. The
 *  raw text stays available as a tooltip. */
export function hubErrorText(raw?: string): string {
  if (!raw) return L("App 无法连接这个 Hub。", "The app can’t connect to this Hub.");
  if (/keychain|token|unauthori|permission|revoked/i.test(raw)) {
    return L("这台设备在这个 Hub 上的凭据失效了，到「设置 → Hub」重新添加。", "This device's credentials for this hub no longer work. Add the hub again in Settings → Hubs.");
  }
  return L("暂时连不上这个 Hub，或者它响应太慢。会自动重试。", "Can't reach this hub right now, or it is answering too slowly. It retries by itself.");
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : String(e);
}

export function errorSuggestion(e: unknown): string | undefined {
  return e instanceof ApiError ? e.suggestion : undefined;
}

/** A host in the SSH config, as ssh -G resolves it. */
export interface SSHHost { name: string; user?: string; host: string; port?: number; jump?: string; connected?: string }
export interface WSLDistro { name: string; default?: boolean; running?: boolean; version?: number; connected?: string }
export interface RemoteRoute { via: "ssh" | "wsl"; target: string; port?: number }
/** Why reaching or setting up a computer failed, and what to try in a terminal. */
export interface RemoteError { code: string; message: string; command?: string }
/** What a check-up found on a computer (found: myco, myco_path, service, since, linger, running, proxy, no_proxy, os). */
export interface Diagnosis {
  route: RemoteRoute;
  found: Record<string, string>;
  hubs: { url: string; mine?: boolean; direct?: string; proxy?: string }[];
  log?: string[];
}
/** How Claude Code signs in for agents this computer starts (names and hosts only, never keys). */
export interface ClaudeAuth {
  account: "ready" | "signed_out" | "unknown" | "missing";
  cc_switch: { ref: string; id: string; name: string; host?: string; current?: boolean }[];
  shell?: string[];
  shell_host?: string;
  using: string;
  using_name?: string;
}
export interface ConnectStep { key: "login" | "install" | "online" | "add"; state: "pending" | "running" | "done" | "failed"; detail?: string }
export interface ConnectJob {
  id: string;
  hub: string;
  route: RemoteRoute;
  steps: ConnectStep[];
  found?: Record<string, string>;
  log?: string[];
  error?: RemoteError;
  machine_id?: string;
  /** setting up a workspace: its id in this App once added */
  hub_id?: string;
  done: boolean;
}

async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const raw = body instanceof Blob;
  if (body !== undefined) headers["Content-Type"] = raw ? (body.type || "application/octet-stream") : "application/json";
  const r = await fetch(path, {
    method,
    credentials: "same-origin",
    headers,
    body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
    signal,
  });
  const text = await r.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!r.ok) {
    const e = (data ?? {}) as { error?: string; message?: string; suggestion?: string; latest?: unknown };
    // A gateway's own page (no JSON) still says why: keep its first words for 详细信息.
    const page = data === null && r.status >= 500 ? text.replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim().slice(0, 300) : "";
    throw Object.assign(new ApiError(e.message || page || `HTTP ${r.status}`, r.status, typeof e.error === "string" ? e.error : undefined, e.suggestion), e.latest ? { latest: e.latest } : {});
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
import { hydrate } from "@/lib/memo";

export async function probeAppMode(): Promise<AppBootstrap | null> {
  try {
    const r = await fetch("app/hubs", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!r.ok) return null;
    const data: unknown = await r.json();
    if (!Array.isArray(data)) return null;
    // The App: what the pages showed last time, before the first one draws.
    const [info] = await Promise.all([probeAppInfo(), hydrate()]);
    return { hubs: data as Hub[], info };
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

/** GET /app/cache/stats: the saved page data. */
export interface CacheStats { enabled: boolean; entries: number; bytes: number }

export const appApi = {
  cacheStats: () => request<CacheStats>("GET", "app/cache/stats"),
  info: () => request<AppInfo>("GET", "app/info"),
  /** Speech to text in lang (desktop app): "system" types into the focused
   *  field itself; "listen" has text to ask for (dictationState). */
  startDictation: (lang: string) => request<{ mode: "system" | "listen"; id?: string }>("POST", "app/dictation", { lang }),
  /** What a listening heard after seq (waits a few seconds for more). */
  dictationState: (id: string, seq: number) =>
    request<{ seq: number; text: string; final?: boolean; done?: boolean; error?: string }>("GET", `app/dictation/${enc(id)}?seq=${seq}`),
  stopDictation: (id: string) => request<void>("DELETE", `app/dictation/${enc(id)}`),
  // Translating messages (console translate.go): keys stay in the keychain.
  /** Messages taken out of your own view (删除), kept on the workspace. */
  hiddenMessages: (hubId: string) => request<{ ids: string[] }>("GET", `app/hubs/${enc(hubId)}/hidden`),
  updateHidden: (hubId: string, u: { hide?: string[]; show?: string[] }) => request<{ ids: string[] }>("POST", `app/hubs/${enc(hubId)}/hidden`, u),
  pickImage: () => request<{ cancelled?: boolean; name?: string; type?: string; data?: string }>("POST", "app/pick-image"),
  openUrl: (url: string) => request<{ ok: boolean }>("POST", "app/open-url", { url }),
  /** the App's network proxy (Settings › 代理) */
  proxy: () => request<ProxySetting>("GET", "app/proxy"),
  putProxy: (p: ProxyInput) => request<ProxySetting>("PUT", "app/proxy", p),
  testProxy: (p: ProxyInput & { url?: string }) => request<{ ok: boolean; ms?: number; reason?: string; error?: string }>("POST", "app/proxy/test", p),
  translate: (text: string, to: "zh" | "en") => request<{ text: string; provider: string }>("POST", "app/translate", { text, to }),
  translateConfig: () => request<TranslateConfig>("GET", "app/translate/config"),
  putTranslateConfig: (c: { provider: string; base_url?: string; model?: string; key?: string; clear_key?: boolean }) =>
    request<TranslateConfig>("PUT", "app/translate/config", c),
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
  /** A thread; with since (unix ms), only what changed after it. since in
   *  the answer says the hub did that (hubs from v0.18.35); without it the
   *  messages are the whole thread. */
  thread: (hubId: string, threadId: string, since?: number) =>
    request<{ messages: Message[]; since?: number }>("GET", `app/hubs/${enc(hubId)}/threads/${enc(threadId)}${since ? `?updated_since=${since}` : ""}`),
  /** The hub's settings (end-to-end encryption); the owner may change them. */
  hubSettings: (hubId: string) => request<{ e2e: boolean }>("GET", `app/hubs/${enc(hubId)}/settings`),
  setHubSettings: (hubId: string, input: { e2e: boolean }) => request<{ e2e: boolean }>("PUT", `app/hubs/${enc(hubId)}/settings`, input),
  /** The nodes a node exchanged messages with (not the console), most recent first. */
  peers: (hubId: string, name: string) =>
    request<{ peers: Peer[] }>("GET", `app/hubs/${enc(hubId)}/agents/${enc(name)}/peers`),
  /** Messages between two nodes, newest first; before (unix ms) pages back. */
  /** Messages between a and b, newest first (before pages back); with since
   *  (unix ms), only what changed after it, oldest change first, when the
   *  answer's since says so (hubs from v0.18.35). */
  exchanges: (hubId: string, a: string, b: string, before?: number, since?: number) =>
    request<{ messages: Message[]; more: boolean; since?: number }>("GET", `app/hubs/${enc(hubId)}/exchanges?a=${enc(a)}&b=${enc(b)}${before ? `&before=${before}` : ""}${since ? `&updated_since=${since}` : ""}`),
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
    if (query.project) q.set("project", query.project);
    if (query.group) q.set("group", query.group);
    if (query.side) q.set("side", query.side);
    const qs = q.toString();
    return request<RecentList>("GET", `app/hubs/${enc(hubId)}/recent${qs ? `?${qs}` : ""}`);
  },
  /** Sends a task from this console; without thread_id it starts a conversation. */
  sendMessage: (hubId: string, input: SendMessageInput) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages`, input),
  /** Uploads a file to attach to the next message (as yours; sealed when the
   *  workspace encrypts). Older workspaces answer not_found. */
  uploadAttachment: (hubId: string, file: File, name = file.name) =>
    request<{ id: string; name: string; size: number; content_type?: string }>("POST", `app/hubs/${enc(hubId)}/files?name=${encodeURIComponent(name)}`, file),
  /** Where a message's attachment is read from (a picture is shown in
   *  place; anything else comes as a download). */
  attachmentUrl: (hubId: string, fileId: string) => `app/hubs/${enc(hubId)}/files/${enc(fileId)}`,
  /** Saves a message's attachment to Downloads and shows it in Finder /
   *  Explorer. Older workspaces answer not_found. */
  saveAttachment: (hubId: string, fileId: string) =>
    request<{ path: string }>("POST", `app/hubs/${enc(hubId)}/files/${enc(fileId)}/save`),
  /** The steps of a task's run after seq (what the agent says, thinks and does). */
  trace: (hubId: string, messageId: string, after = 0) =>
    request<Trace>("GET", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/trace${after ? `?after=${after}` : ""}`),
  /** Stops a running task the console sent (hubs from v0.18.32). */
  stop: (hubId: string, messageId: string) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/stop`),
  /** Corrects an unfinished task the console sent. Files go with it on hubs
   *  from v0.19.2; older hubs take the text only. */
  steer: (hubId: string, messageId: string, body: string, attachments?: string[]) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/steer`, { body, attachments }),
  /** Closes an open message for its recipient without replying (owner or admin). */
  dismissMessage: (hubId: string, messageId: string) =>
    request<Message>("POST", `app/hubs/${enc(hubId)}/messages/${enc(messageId)}/dismiss`),
  /** An agent's picture face; v (its avatar word) changes with the picture. */
  avatarUrl: (hubId: string, name: string, v: string) =>
    `app/hubs/${enc(hubId)}/agents/${enc(name)}/avatar?v=${encodeURIComponent(v)}`,
  updateProfile: (hubId: string, name: string, input: ProfileInput) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/profile`, input),
  /** Makes the agent's resume line (the old one stops working): the command
   *  to run in a new sandbox to bring the same agent back. Hubs before
   *  v0.19.5 answer not_found. */
  createAgentResume: (hubId: string, name: string) =>
    request<{ url: string; command: string }>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/resume`),
  /** Revokes it: the command made before no longer works. */
  revokeAgentResume: (hubId: string, name: string) =>
    request<{ ok: boolean }>("DELETE", `app/hubs/${enc(hubId)}/agents/${enc(name)}/resume`),
  /** The model and effort this node runs with; "" follows the computer's
   *  setting. Hubs before the feature answer not_found. */
  setRunSettings: (hubId: string, name: string, input: { model: string; effort: string; access?: RuntimeAccess | "" }) =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/run-settings`, input),
  // Projects
  projects: (hubId: string) => request<{ projects: Project[] }>("GET", `app/hubs/${enc(hubId)}/projects`),
  project: (hubId: string, name: string, bodies = false) =>
    request<Project>("GET", `app/hubs/${enc(hubId)}/projects/${enc(name)}${bodies ? "?docs=body" : ""}`),
  createProject: (hubId: string, input: { name: string; title?: string; goal?: string; visibility?: "members" | "team" }) =>
    request<Project>("POST", `app/hubs/${enc(hubId)}/projects`, input),
  updateProject: (hubId: string, name: string, input: { title?: string; goal?: string; brief?: string; links?: ProjectLink[]; visibility?: "members" | "team"; archived?: boolean }) =>
    request<Project>("PATCH", `app/hubs/${enc(hubId)}/projects/${enc(name)}`, input),
  deleteProject: (hubId: string, name: string) => request<void>("DELETE", `app/hubs/${enc(hubId)}/projects/${enc(name)}`),
  putProjectMember: (hubId: string, name: string, agent: string, input: { role?: "member" | "coordinator"; note?: string } = {}) =>
    request<Project>("PUT", `app/hubs/${enc(hubId)}/projects/${enc(name)}/members/${enc(agent)}`, input),
  removeProjectMember: (hubId: string, name: string, agent: string) =>
    request<Project>("DELETE", `app/hubs/${enc(hubId)}/projects/${enc(name)}/members/${enc(agent)}`),
  /** Uploads a file into a project (any kind, up to 20 MB). */
  uploadProjectFile: (hubId: string, name: string, file: File, as?: string) =>
    request<ProjectDoc>("PUT", `app/hubs/${enc(hubId)}/projects/${enc(name)}/files/${enc(as || file.name)}`, file),
  /** A project file's URL (for a browser download or an image preview). */
  projectFileURL: (hubId: string, name: string, file: string) => `app/hubs/${enc(hubId)}/projects/${enc(name)}/files/${enc(file)}`,
  /** Saves a project file to this computer's Downloads folder and shows it there. */
  saveProjectFile: (hubId: string, name: string, file: string) =>
    request<{ path: string }>("POST", `app/hubs/${enc(hubId)}/projects/${enc(name)}/files/${enc(file)}/save`),
  projectDoc: (hubId: string, name: string, doc: string) =>
    request<ProjectDoc>("GET", `app/hubs/${enc(hubId)}/projects/${enc(name)}/docs/${enc(doc)}`),
  putProjectDoc: (hubId: string, name: string, doc: string, body: string) =>
    request<ProjectDoc>("PUT", `app/hubs/${enc(hubId)}/projects/${enc(name)}/docs/${enc(doc)}`, { body }),
  deleteProjectDoc: (hubId: string, name: string, doc: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/projects/${enc(name)}/docs/${enc(doc)}`),
  /** Makes a node visible to the team or private to its owner. */
  setVisibility: (hubId: string, name: string, visibility: "team" | "private") =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/visibility`, { visibility }),
  /** Replaces a node's skills. */
  setSkills: (hubId: string, name: string, skills: Skill[]) =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/skills`, { skills }),
  /** Adds and removes labels (group=… and tags) of a node. */
  setLabels: (hubId: string, name: string, change: { add?: string[]; remove?: string[] }) =>
    request<Agent>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/labels`, change),
  /** Points an alias at a node: messages to it reach that node. */
  setAlias: (hubId: string, alias: string, target: string) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/aliases/${enc(alias)}`, { target }),
  removeAlias: (hubId: string, alias: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/aliases/${enc(alias)}`),
  updateAllowFrom: (hubId: string, name: string, allowFrom: string[]) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/allow_from`, { allow_from: allowFrom }),
  kick: (hubId: string, name: string) =>
    request<void>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/kick`),
  removeAgent: (hubId: string, name: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/agents/${enc(name)}`),

  /** Another session takes a node over (「换一个来接着做」); older hubs 404. */
  handovers: (hubId: string, name: string) =>
    request<{ handovers: Handover[] }>("GET", `app/hubs/${enc(hubId)}/agents/${enc(name)}/handovers`),
  /** A one-time command (24 hours) that whoever runs it takes the node over with. */
  handoverLink: (hubId: string, name: string, input: { note?: string; back?: string } = {}) =>
    request<HandoverLink>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/handover-link`, input),
  /** A new session on a computer takes the node over; waits for its first turn. */
  handOver: (hubId: string, name: string, input: HandoverRequest) =>
    request<HandoverResult>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/handover`, input),
  /** Starts a session on a machine; the machine may take up to two minutes. */
  spawn: (hubId: string, machineId: string, input: SpawnInput) =>
    request<SpawnResult>("POST", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/spawn`, input),
  machineInfo: (hubId: string, machineId: string) =>
    request<MachineInfo>("GET", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/info`),
  renameMachine: (hubId: string, machineId: string, name: string) =>
    request<Machine>("PATCH", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}`, { name }),
  /** Removes a machine and every node on it. dryRun lists them first. */
  removeMachine: (hubId: string, machineId: string, dryRun = false) =>
    request<MachineRemoval>("DELETE", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}${dryRun ? "?dry_run=true" : ""}`),
  /** Hosts in this computer's SSH config; connected marks the ones this App connected to hubId. */
  sshHosts: (hubId = "") => request<{ hosts: SSHHost[]; error?: RemoteError }>("GET", `app/local/ssh-hosts${hubId ? `?hub=${enc(hubId)}` : ""}`),
  /** This Windows computer's WSL distributions (supported: false elsewhere). */
  wslDistros: (hubId = "") => request<{ supported: boolean; distros: WSLDistro[] }>("GET", `app/local/wsl${hubId ? `?hub=${enc(hubId)}` : ""}`),
  /** Connects a computer over SSH or into WSL from this App: answers the job at once. */
  connectMachine: (hubId: string, route: RemoteRoute) =>
    request<ConnectJob>("POST", `app/hubs/${enc(hubId)}/machines/connect`, route),
  connectJob: (id: string) => request<ConnectJob>("GET", `app/connect/${enc(id)}`),
  /** Sets up a workspace on a server over SSH (or adds the one there) and adds it to this App: answers the job at once. */
  installHubOver: (route: RemoteRoute) => request<ConnectJob>("POST", "app/local/hub-over-ssh", route),
  /** How this App reaches a computer (null: it does not). */
  machineReach: (hubId: string, machineId: string) =>
    request<{ route: RemoteRoute | null }>("GET", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/reach`),
  setMachineReach: (hubId: string, machineId: string, route: RemoteRoute) =>
    request<{ route: RemoteRoute }>("PUT", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/reach`, route),
  /** Looks at a computer over SSH or WSL: its myco, runner, linger, proxies, hubs, log. */
  diagnoseMachine: (hubId: string, machineId: string) =>
    request<Diagnosis>("POST", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/diagnose`),
  fixMachine: (hubId: string, machineId: string, action: "restart" | "upgrade") =>
    request<{ ok: boolean; log: string[] }>("POST", `app/hubs/${enc(hubId)}/machines/${enc(machineId)}/fix`, { action }),
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
  /** Changes to a node (its card, visibility, owner…), newest first. */
  agentHistory: (hubId: string, name: string, limit = 30) =>
    request<{ entries: AuditEntry[] }>("GET", `app/hubs/${enc(hubId)}/agents/${enc(name)}/history?limit=${limit}`),
  audit: (hubId: string, q: { actor?: string; target?: string; before?: number; limit?: number } = {}) => {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v) p.set(k, String(v));
    const qs = p.toString();
    return request<{ entries: AuditEntry[] }>("GET", `app/hubs/${enc(hubId)}/audit${qs ? `?${qs}` : ""}`);
  },

  local: () => request<LocalState>("GET", "app/local"),
  /** The agent CLIs on this computer: installed, signed in (from v0.20.12). */
  localAgents: () => request<{ agents: LocalAgent[] }>("GET", "app/local/agents"),
  /** Opens a terminal that installs or signs in to an agent CLI; opened:false → show command. */
  agentTerminal: (cli: string, action: "install" | "install_mirror" | "login") =>
    request<{ opened: boolean; command: string }>("POST", `app/local/agents/${enc(cli)}/terminal`, { action }),
  /** How Claude Code signs in for agents started here, and the ways this computer has. */
  claudeAuth: () => request<ClaudeAuth>("GET", "app/local/claude-auth"),
  /** use: "cc-switch:<id>", "shell" (the shell's API settings) or "account". */
  setClaudeAuth: (use: string) => request<ClaudeAuth>("POST", "app/local/claude-auth", { use }),
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
  /** Brings this computer's myco to the latest release and restarts its runner. */
  upgradeMyco: () => request<{ from: string; version: string; restarted: string[] }>("POST", "app/local/myco/upgrade"),
  /** Starts a Hub on this machine (and adds it to the App); can take up to 90s. */
  // Voice chat with an agent (the Mac app; voice.go)
  voice: () => request<VoiceInfo>("GET", "app/voice"),
  putVoiceConfig: (input: Partial<VoiceConfig> & { llm_key?: string; tts_key?: string; clear_llm_key?: boolean; clear_tts_key?: boolean; share?: boolean }) =>
    request<VoiceInfo>("PUT", "app/voice/config", input),
  // The template library (hubs from v0.20.5; older ones answer 404).
  templates: (hubId: string) => request<{ templates: Template[] }>("GET", `app/hubs/${enc(hubId)}/templates`),
  template: (hubId: string, id: string) => request<Template>("GET", `app/hubs/${enc(hubId)}/templates/${enc(id)}`),
  // Fetch myco's official apps now; give a page its app's newest version (hubs from v0.20.7).
  syncApps: (hubId: string) => request<{ apps: number; updated?: string[]; source: string; error?: string }>("POST", `app/hubs/${enc(hubId)}/templates/sync`),
  upgradePage: (hubId: string, id: string) => request<Page>("POST", `app/hubs/${enc(hubId)}/pages/${enc(id)}/upgrade`),
  useTemplate: (hubId: string, id: string, input: { name?: string; project?: string; sample?: boolean }) =>
    request<{ page: Page; template: Template }>("POST", `app/hubs/${enc(hubId)}/templates/${enc(id)}/use`, input),
  // A page's records and change feed (hubs from v0.20.5), for the bridge.
  pageRecords: (hubId: string, id: string, collection?: string) =>
    request<{ records: PageRecord[]; seq: number }>("GET", `app/hubs/${enc(hubId)}/pages/${enc(id)}/data${collection ? `?collection=${enc(collection)}` : ""}`),
  pageRecord: (hubId: string, id: string, collection: string, rid: string) =>
    request<PageRecord>("GET", `app/hubs/${enc(hubId)}/pages/${enc(id)}/data/${enc(collection)}/${enc(rid)}`),
  /** A record written; on a conflict the error carries the latest. */
  putPageRecord: (hubId: string, id: string, collection: string, rid: string, data: unknown, ifVersion: number | null) =>
    request<PageRecord>(rid ? "PUT" : "POST", `app/hubs/${enc(hubId)}/pages/${enc(id)}/data/${enc(collection)}${rid ? `/${enc(rid)}` : ""}`,
      { data, ...(ifVersion === null ? {} : { if_version: ifVersion }) }),
  deletePageRecord: (hubId: string, id: string, collection: string, rid: string, ifVersion = 0) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/pages/${enc(id)}/data/${enc(collection)}/${enc(rid)}${ifVersion ? `?if_version=${ifVersion}` : ""}`),
  pageChanges: (hubId: string, id: string, after: number) =>
    request<PageChanges>("GET", `app/hubs/${enc(hubId)}/pages/${enc(id)}/changes?after=${after}`),
  /** A file a page made, saved to Downloads and shown. */
  pageDownload: (name: string, bytes: Uint8Array | Blob, type: string) =>
    request<{ path: string }>("POST", `app/page-download?name=${encodeURIComponent(name)}`,
      bytes instanceof Blob ? bytes : new Blob([bytes as BlobPart], { type: type || "application/octet-stream" })),
  // Pins (hubs from v0.20.3; older ones answer 404): what a conversation
  // keeps at hand, for both its parties. a: the first party (default you).
  pins: (hubId: string, withWhom: string, a?: string) =>
    request<PinList>("GET", `app/hubs/${enc(hubId)}/pins?with=${enc(withWhom)}${a ? `&a=${enc(a)}` : ""}`),
  addPin: (hubId: string, p: PinCreate) => request<Pin>("POST", `app/hubs/${enc(hubId)}/pins`, p),
  renamePin: (hubId: string, id: string, withWhom: string, title: string, a?: string) =>
    request<Pin>("PATCH", `app/hubs/${enc(hubId)}/pins/${enc(id)}`, { with: withWhom, title, ...(a ? { a } : {}) }),
  removePin: (hubId: string, id: string, withWhom: string, a?: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/pins/${enc(id)}?with=${enc(withWhom)}${a ? `&a=${enc(a)}` : ""}`),
  orderPins: (hubId: string, withWhom: string, ids: string[], a?: string) =>
    request<PinList>("PUT", `app/hubs/${enc(hubId)}/pins/order`, { with: withWhom, ids, ...(a ? { a } : {}) }),
  // Pages (hubs from v0.19.24; older ones answer 404): agents' web pages
  pages: (hubId: string, project?: string) => request<{ pages: Page[] }>("GET", `app/hubs/${enc(hubId)}/pages${project ? `?project=${enc(project)}` : ""}`),
  page: (hubId: string, id: string) => request<Page>("GET", `app/hubs/${enc(hubId)}/pages/${enc(id)}`),
  openPage: (hubId: string, id: string, version?: number) => request<PageOpen>("POST", `app/hubs/${enc(hubId)}/pages/${enc(id)}/open`, version ? { version } : {}),
  deletePage: (hubId: string, id: string) => request<void>("DELETE", `app/hubs/${enc(hubId)}/pages/${enc(id)}`),
  pageState: (hubId: string, id: string) => request<PageState>("GET", `app/hubs/${enc(hubId)}/pages/${enc(id)}/state`),
  /** Writes a page's shared state based on version; when someone wrote
   *  first, ok is false and the latest comes back. */
  putPageState: async (hubId: string, id: string, state: unknown, version: number): Promise<PageState & { ok: boolean }> => {
    try {
      return { ok: true, ...(await request<PageState>("PUT", `app/hubs/${enc(hubId)}/pages/${enc(id)}/state`, { state, if_version: version })) };
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.latest) return { ok: false, ...(e.latest as PageState) };
      throw e;
    }
  },
  // Automations (hubs from v0.19.22; older ones answer 404)
  automations: (hubId: string) => request<{ automations: Automation[] }>("GET", `app/hubs/${enc(hubId)}/automations`),
  automation: (hubId: string, id: string) => request<AutomationDetail>("GET", `app/hubs/${enc(hubId)}/automations/${enc(id)}`),
  createAutomation: (hubId: string, input: AutomationRequest) => request<AutomationResponse>("POST", `app/hubs/${enc(hubId)}/automations`, input),
  updateAutomation: (hubId: string, id: string, input: AutomationRequest) => request<AutomationResponse>("PATCH", `app/hubs/${enc(hubId)}/automations/${enc(id)}`, input),
  deleteAutomation: (hubId: string, id: string) => request<void>("DELETE", `app/hubs/${enc(hubId)}/automations/${enc(id)}`),
  automationRun: (hubId: string, id: string, run: string) => request<AutomationRun>("GET", `app/hubs/${enc(hubId)}/automations/${enc(id)}/runs/${enc(run)}`),
  runAutomation: (hubId: string, id: string) => request<AutomationRun>("POST", `app/hubs/${enc(hubId)}/automations/${enc(id)}/run`, {}),
  /** Saves a conversation the app rendered (.md or .jsonl) to Downloads and shows it. */
  saveExport: (name: string, content: string) => request<{ path: string }>("POST", "app/export", { name, content }),
  /** One line in that voice, as WAV audio, to try it before saving (the key typed, else the one kept). */
  previewVoice: async (input: { tts_provider: string; tts_speaker: string; tts_rate: number; tts_key?: string; lang: string }): Promise<Blob> => {
    const r = await fetch("app/voice/preview", {
      method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json", Accept: "audio/wav, application/json" }, body: JSON.stringify(input),
    });
    if (!r.ok) {
      let e: { error?: string; message?: string; suggestion?: string } = {};
      try { e = await r.json(); } catch { /* non-JSON */ }
      throw new ApiError(e.message || `HTTP ${r.status}`, r.status, e.error, e.suggestion);
    }
    return r.blob();
  },
  /** Looks at the hubs for newer settings (set on another device), then answers as voice(). */
  voiceSync: () => request<VoiceInfo>("POST", "app/voice/sync", {}),
  /** A call with one agent, in the person's chat with it (peer: their address there). */
  voiceStart: (hubId: string, agent: string, peer: string | undefined, lang: string) =>
    request<VoiceInfo>("POST", "app/voice/start", { hub_id: hubId, agent, peer, lang }),
  voiceStop: () => request<VoiceInfo>("POST", "app/voice/stop", {}),
  voiceMute: (muted: boolean) => request<VoiceInfo>("POST", "app/voice/mute", { muted }),
  voiceEvents: (after: number) => request<VoiceEvents>("GET", `app/voice/events?after=${after}`),
  /** The language these pages show, for what the desktop app draws itself (its tray menu). */
  putUILang: (lang: "zh" | "en") => request<{ lang: string }>("PUT", "app/ui-lang", { lang }),
  // Workspaces of a hub's server (hubs from v0.19.9 serve several at /w/<name>)
  workspaces: (hubId: string) =>
    request<WorkspaceState>("GET", `app/hubs/${enc(hubId)}/workspaces`),
  createWorkspace: (hubId: string, name: string, displayName: string) =>
    request<Hub>("POST", `app/hubs/${enc(hubId)}/workspaces`, { name, display_name: displayName }),
  serverMachines: (hubId: string) =>
    request<{ machines: ServerMachine[] }>("GET", `app/hubs/${enc(hubId)}/server-machines`),
  addMachineFrom: (hubId: string, fromHub: string, machineId: string) =>
    request<{ joined: string; machine_node?: string }>("POST", `app/hubs/${enc(hubId)}/machines/add-from`, { from_hub: fromHub, machine_id: machineId }),
  /** signal: cancelling stops it on this computer too (the request ends). */
  startLocalHub: (input: StartLocalHubInput, signal?: AbortSignal) =>
    request<StartLocalHubResult>("POST", "app/local/hub", input, signal),
  stopLocalHub: (name?: string) =>
    request<{ stopped: true }>("POST", "app/local/hub/stop", name ? { name } : undefined),
};

/** A cc-switch provider (keys are never sent to the page). */
export interface ProviderInfo { ref: string; id: string; name: string; app: string; current?: boolean; host?: string; model?: string }
export interface ProviderList { cc_switch: boolean; providers: ProviderInfo[]; error?: string }
/** provider: "" follows this computer; "cc-switch:<id>" is pinned; "env_file" was entered by hand. */
export interface AgentProvider { local: boolean; provider: string; harness?: string; env_host?: string }
export interface ManualProvider { base_url: string; api_key: string; model?: string }

/** How messages are translated: the service, and which ones have a key. */
export type TranslateConfig = { provider: "deepl" | "google" | "openai"; base_url?: string; model?: string; keys: Record<string, boolean> };
