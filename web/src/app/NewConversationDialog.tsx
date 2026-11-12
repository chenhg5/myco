import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, Plus, Search } from "lucide-react";
import { appApi, type Hub, type SpawnCli } from "@/appapi";
import type { Machine, Project, RuntimeAccess } from "@/api";
import type { AgentRow } from "@/app/types";
import {
  agentKind, aliasSummary, cliName, cloudLabel, isCloud, machineLabel, machinesOfHub, nodeIcon, nodeName, osInfo, stateLabel,
} from "@/app/nodes";
import { ACCESS, CLIS } from "@/app/AddNodeDialog";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { Elapsed, Mushroom } from "@/app/Working";
import { IconCloud, IconMachine, IconNodes, type IconType } from "@/components/icons";
import { stateMeta } from "@/components/status";
import { ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export interface NewConversationTarget {
  hubId?: string;
  agent?: string;
  /** Puts the conversation in this project (of hubId). */
  project?: string;
}

/** Where a node lives: a machine, a cloud, or somewhere not reported. */
interface Place {
  key: string;
  hubId: string;
  label: string;
  sub?: string;
  icon: IconType;
  machine?: Machine;
  rows: AgentRow[];
  offline?: boolean;
}

const PLACE_KEY = "myco-newconv-place";
const ALL = "all";

function readPlace(): string {
  try { return localStorage.getItem(PLACE_KEY) || ALL; } catch { return ALL; }
}
function writePlace(key: string) {
  try { localStorage.setItem(PLACE_KEY, key); } catch { /* not kept */ }
}

/** Where a node's place key is: its machine, its cloud, or "other". */
function placeOf(r: AgentRow): string {
  const a = r.st.agent;
  if (isCloud(a)) return `${r.hub.id}\u0000cloud\u0000${(a.runtime?.cli || a.binding.harness || "").replace(/-cloud$/, "")}`;
  if (a.machine_id && machinesOfHub(r.hub).some((m) => m.id === a.machine_id)) return `${r.hub.id}\u0000m\u0000${a.machine_id}`;
  return `${r.hub.id}\u0000other`;
}

// The CLIs a conversation can be started with (a shell node needs senders,
// so it is added from the Nodes page).
const AGENT_CLIS = CLIS.filter((c) => c.value !== "exec");

/**
 * Starts a conversation the way a chat app does: pick a node and its chat
 * opens; the first message is written there. Nodes are browsed by where
 * they run (each machine, each cloud), and on a machine a new session can
 * be started right here, which then opens.
 */
export function NewConversationDialog({ target, hubs, rows, projects, scopeHubId, onClose, onPick, onSpawned }: {
  target: NewConversationTarget | null;
  hubs: Hub[];
  rows: AgentRow[];
  /** Projects of every hub: in a project, its members come first. */
  projects?: { hubId: string; p: Project }[] | null;
  /** The Hub chosen in the switcher, when it is one: only its nodes. */
  scopeHubId?: string;
  onClose: () => void;
  onPick: (hubId: string, agent: string, project?: string) => void;
  /** A node was started here (refresh, mark it in the list). */
  onSpawned?: (hubId: string, agent: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [placeKey, setPlaceKey] = useState(ALL);
  const [creating, setCreating] = useState(false);
  const [cli, setCli] = useState<SpawnCli>("claude");
  const [dir, setDir] = useState("");
  const [access, setAccess] = useState<RuntimeAccess>("workspace");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!target) return;
    setQuery("");
    setPlaceKey(readPlace());
    setCreating(false);
    setError(null);
    setBusy(null);
  }, [target]);

  const project = target?.project ? (projects || []).find((x) => x.hubId === target.hubId && x.p.name === target.project)?.p : undefined;
  const members = new Set(project?.members.map((m) => m.name) || []);
  // A project's conversation stays on its hub; otherwise the hub chosen in
  // the switcher, or all of them.
  const scope = target?.project && target.hubId ? target.hubId : scopeHubId;
  const scopedHubs = hubs.filter((h) => !scope || h.id === scope);

  const usable = useMemo(() => rows.filter((r) => {
    const k = agentKind(r.st.agent);
    return (k === "agent" || k === "shell") && (!scope || r.hub.id === scope);
  }), [rows, scope]);

  // The places, by hub: its machines (those online first), its clouds, and
  // nodes on no reported machine.
  const places = useMemo(() => {
    const out: { hub: Hub; places: Place[] }[] = [];
    for (const h of scopedHubs) {
      const hubRows = usable.filter((r) => r.hub.id === h.id);
      const ov = rows.find((r) => r.hub.id === h.id)?.hub;
      const machines = ov ? machinesOfHub(ov) : [];
      const list: Place[] = [];
      for (const m of [...machines].sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name))) {
        const on = hubRows.filter((r) => placeOf(r) === `${h.id}\u0000m\u0000${m.id}`);
        if (!on.length && !m.can_spawn) continue;
        list.push({ key: `${h.id}\u0000m\u0000${m.id}`, hubId: h.id, label: m.name, sub: osInfo(m.os).label, icon: osInfo(m.os).icon, machine: m, rows: on, offline: !m.online });
      }
      const clouds = new Map<string, AgentRow[]>();
      for (const r of hubRows) {
        const k = placeOf(r);
        if (k.includes("\u0000cloud\u0000")) clouds.set(k, [...(clouds.get(k) || []), r]);
      }
      for (const [k, rs] of clouds) list.push({ key: k, hubId: h.id, label: cloudLabel(rs[0].st.agent), icon: IconCloud, rows: rs });
      const other = hubRows.filter((r) => placeOf(r) === `${h.id}\u0000other`);
      if (other.length) list.push({ key: `${h.id}\u0000other`, hubId: h.id, label: L("其他", "Other"), sub: L("没有上报机器", "No machine reported"), icon: IconMachine, rows: other });
      if (list.length) out.push({ hub: h, places: list });
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [usable, rows, scopedHubs.map((h) => h.id).join(",")]);

  const place = places.flatMap((g) => g.places).find((p) => p.key === placeKey);
  // The chosen place stays in sight (a row of chips in a narrow window).
  const nav = useRef<HTMLElement>(null);
  useEffect(() => {
    const show = () => nav.current?.querySelector(".app-convpick-place.active")?.scrollIntoView({ block: "nearest", inline: "nearest" });
    show();
    window.addEventListener("resize", show);
    return () => window.removeEventListener("resize", show);
  }, [placeKey, target, places.length]);
  const choose = (key: string) => {
    setPlaceKey(key);
    writePlace(key);
    setCreating(false);
    setError(null);
  };

  const q = query.trim().toLowerCase();
  const matches = (r: AgentRow) => {
    if (!q) return true;
    const a = r.st.agent;
    return [nodeName(a, r.hub), a.name, a.description, cliName(a), machineLabel(r) || "", ...(a.aliases || [])].some((s) => (s || "").toLowerCase().includes(q));
  };
  const order = (a: AgentRow, b: AgentRow) =>
    Number(members.has(b.st.agent.name)) - Number(members.has(a.st.agent.name))
    || Number(a.st.state === "offline") - Number(b.st.state === "offline")
    || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub));
  const groups = (place ? [{ hub: scopedHubs.find((h) => h.id === place.hubId)!, rows: place.rows }] : scopedHubs.map((h) => ({ hub: h, rows: usable.filter((r) => r.hub.id === h.id) })))
    .map((g) => ({ hub: g.hub, rows: g.rows.filter(matches).sort(order) }))
    .filter((g) => g.rows.length > 0);

  const pick = (r: AgentRow) => onPick(r.hub.id, r.st.agent.name, target?.project);
  const first = groups[0]?.rows[0];

  // Folders this machine's nodes already work in: one click to start
  // another session there.
  const folders = place?.machine
    ? [...new Set(place.rows.map((r) => r.st.agent.binding.workdir).filter((d): d is string => !!d))].slice(0, 6)
    : [];
  const openCreate = () => {
    setCreating(true);
    setError(null);
    setDir((d) => d || folders[0] || "");
  };

  const start = async (e: React.FormEvent) => {
    e.preventDefault();
    const m = place?.machine;
    if (!m || !dir.trim() || busy) return;
    setBusy(new Date().toISOString());
    setError(null);
    try {
      const r = await appApi.spawn(place.hubId, m.id, { cli, dir: dir.trim(), access });
      onSpawned?.(place.hubId, r.agent);
      onPick(place.hubId, r.agent, target?.project);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(null);
    }
  };

  const total = usable.length;
  const cliLabel = AGENT_CLIS.find((c) => c.value === cli)?.label || cli;

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && !busy && onClose()}
      title={L("新对话", "New conversation")}
      description={project
        ? L(`选一个节点，或在机器上新建一个；第一条消息会放进项目「${project.title || project.name}」。`, `Pick a node, or start one on a machine; the first message goes into ${project.title || project.name}.`)
        : L("选一个节点，或在机器上新建一个，打开和它的对话。", "Pick a node, or start one on a machine, to open your conversation with it.")}
      className="app-modal-convpick"
    >
      <div className="app-convpick-search">
        <Search aria-hidden />
        <input
          className="app-convpick-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && first && !e.nativeEvent.isComposing) { e.preventDefault(); pick(first); } }}
          placeholder={place ? L(`在「${place.label}」里搜索`, `Search in ${place.label}`) : L("搜索节点：名字、CLI、机器", "Search nodes: name, CLI, machine")}
          aria-label={L("搜索节点", "Search nodes")}
          autoFocus
          disabled={!!busy}
        />
      </div>

      <div className="app-convpick-body">
        {/* Where nodes run: everything, each machine, each cloud. */}
        <nav ref={nav} className="app-convpick-places" aria-label={L("按位置浏览", "Browse by place")}>
          <button type="button" className={cn("app-convpick-place", !place && "active")} onClick={() => choose(ALL)} disabled={!!busy}>
            <IconNodes />
            <span className="app-convpick-place-name">{L("全部节点", "All nodes")}</span>
            <span className="app-convpick-count">{total}</span>
          </button>
          {places.map((g) => (
            <div key={g.hub.id} className="app-convpick-placegroup">
              {places.length > 1 && <div className="app-convpick-hub">{g.hub.name}</div>}
              {g.places.map((p) => {
                const Icon = p.icon;
                return (
                  <button key={p.key} type="button" className={cn("app-convpick-place", place?.key === p.key && "active", p.offline && "is-offline")}
                    onClick={() => choose(p.key)} disabled={!!busy} title={p.offline ? L(`${p.label} · 离线`, `${p.label} · offline`) : p.label}>
                    <Icon />
                    <span className="app-convpick-place-name">{p.label}</span>
                    <span className="app-convpick-count">{p.rows.length || ""}</span>
                  </button>
                );
              })}
            </div>
          ))}
        </nav>

        <div className="app-convpick-main">
          {place?.machine && (
            <div className="app-convpick-machine">
              <div className="app-convpick-machine-text">
                <span className="app-convpick-machine-name">{place.label}</span>
                <span className="app-convpick-machine-sub">
                  <span className={cn("dot", place.offline ? "offline" : "working")} />
                  {place.offline ? L("离线", "Offline") : L("在线", "Online")} · {place.sub}
                </span>
              </div>
              {place.machine.can_spawn && !creating && (
                <button type="button" className="app-btn app-btn-small" onClick={openCreate} disabled={!!busy}>
                  <Plus />{L("在这台机器上新建", "New on this machine")}
                </button>
              )}
            </div>
          )}

          {creating && place?.machine && (
            <form className="app-convpick-create" onSubmit={start}>
              <div className="app-convpick-create-row">
                <label className="app-field">
                  <span className="app-field-label">{L("类型", "Type")}</span>
                  <select className="app-select-field" value={cli} onChange={(e) => setCli(e.target.value as SpawnCli)} disabled={!!busy}>
                    {AGENT_CLIS.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
                  </select>
                </label>
                <div className="app-field">
                  <span className="app-field-label">{L("权限", "Access")}</span>
                  <div className="seg seg-fill" role="tablist" aria-label={L("权限", "Access")}>
                    {ACCESS.map((o) => (
                      <button key={o.value} type="button" role="tab" aria-selected={access === o.value}
                        className={cn("seg-btn", access === o.value && "active")} onClick={() => setAccess(o.value)} disabled={!!busy}>
                        {o.label}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              <label className="app-field">
                <span className="app-field-label">{L("目录", "Folder")}</span>
                <input className="app-input mono" value={dir} onChange={(e) => setDir(e.target.value)}
                  placeholder={L("~/code/项目名", "~/code/project")} disabled={!!busy} required autoFocus />
              </label>
              {folders.length > 0 && (
                <div className="app-convpick-folders" aria-label={L("这台机器上用过的目录", "Folders used on this machine")}>
                  {folders.map((d) => (
                    <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy}>{d}</button>
                  ))}
                </div>
              )}
              {access === "full" && <LocalShellAccess machine={place.machine} what="full" />}
              {place.offline && (
                <div className="app-notice"><AlertTriangle /><span>{L("这台机器现在离线，会话可能启动不了。", "This machine is offline right now, so the session may not start.")}</span></div>
              )}
              <ErrorText error={error} />
              {busy ? (
                <div className="app-convpick-starting">
                  <Mushroom />
                  <span>
                    {L(`正在 ${place.label} 上启动 ${cliLabel}`, `Starting ${cliLabel} on ${place.label}`)}
                    {" · "}<Elapsed since={busy} />
                    <small>{L("它要先跑一轮简短的对话来建好会话，通常半分钟到一分钟。好了会直接打开对话。", "It runs one short turn to create the session, usually 30–60 seconds. The conversation opens when it is ready.")}</small>
                  </span>
                </div>
              ) : (
                <div className="app-modal-actions">
                  <button type="button" className="app-btn" onClick={() => { setCreating(false); setError(null); }}>{L("取消", "Cancel")}</button>
                  <button type="submit" className="app-btn app-btn-primary" disabled={!dir.trim()}>{L("开始对话", "Start conversation")}</button>
                </div>
              )}
            </form>
          )}

          {!creating && (
            <div className="app-convpick-list">
              {groups.length === 0 && (
                <div className="app-empty-note">
                  {total === 0
                    ? L("还没有可以发消息的节点。先在「节点」页添加一个。", "There are no nodes to message yet. Add one on the Nodes page first.")
                    : q
                      ? L("没有匹配的节点。", "No matching nodes.")
                      : place?.machine?.can_spawn
                        ? L("这台机器上还没有节点。可以在上面新建一个。", "No nodes on this machine yet. Start one above.")
                        : L("这里还没有节点。", "No nodes here yet.")}
                </div>
              )}
              {groups.map((g) => (
                <div key={g.hub.id}>
                  {!place && groups.length > 1 && <div className="app-convpick-group">{g.hub.name}</div>}
                  <div className="app-list">
                    {g.rows.map((r) => <NodeRow key={r.st.agent.name} r={r} member={members.has(r.st.agent.name)} showPlace={!place} onPick={() => pick(r)} />)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}

function NodeRow({ r, member, showPlace, onPick }: { r: AgentRow; member: boolean; showPlace: boolean; onPick: () => void }) {
  const a = r.st.agent;
  const Icon = nodeIcon(a);
  const st = stateLabel(r.st, stateMeta[r.st.state].label);
  const machine = machineLabel(r);
  const aliases = aliasSummary(a);
  const dir = a.binding.workdir;
  return (
    <button type="button" className={cn("app-row", r.st.state === "offline" && "offline")} onClick={onPick} title={a.name}>
      <span className="app-row-icon"><Icon /></span>
      <span className="app-row-main">
        <span className="app-row-titleline">
          <span className="app-row-title">{nodeName(a, r.hub)}</span>
          {member && <span className="app-label group">{L("项目成员", "Member")}</span>}
        </span>
        <span className="app-row-sub">
          <span className={cn("dot", st.dot)} />
          <span>{st.label}</span>
          {aliases && (
            <>
              <span className="sep">·</span>
              <span className="txt mono alias" title={(a.aliases || []).map((x) => "@" + x).join(", ")}>{aliases}</span>
            </>
          )}
          <span className="sep">·</span>
          <span className="txt">{cliName(a)}</span>
          {/* On a machine's page its name goes without saying; the folder does not. */}
          {showPlace
            ? machine && <><span className="sep">·</span><span className="txt">{machine}</span></>
            : dir && <><span className="sep">·</span><span className="txt mono">{dir}</span></>}
        </span>
      </span>
      <span className="app-row-trail"><span className="app-chev">›</span></span>
    </button>
  );
}
