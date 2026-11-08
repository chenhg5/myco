import { useEffect, useState } from "react";
import { Check, MessageSquare, X } from "lucide-react";
import { appApi, type AgentProvider, type ProviderList } from "@/appapi";
import type { AgentRow } from "@/app/types";
import type { Member } from "@/api";
import {
  accessLabel, agentKind, cliName, cliVersion, cloudLabel, isCloud, kindLabel, machineLabel,
  canManage, cleanLabel, groupsOf, isMe, machineOfRow, modelSourceLabel, nodeIcon, personLabel, tagsOf,
} from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { NodePicker } from "@/app/NodePicker";
import { summarize } from "@/app/summary";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import { L, lang, locale } from "@/lib/i18n";

type Action = null | "profile" | "allow" | "kick" | "delete" | "owner" | "copy";

const pillClass: Record<string, string> = { working: "green", pending: "amber", idle: "", offline: "" };

const isConsole = (name: string) => name.startsWith("@console:");

// MessageRows lists the node's recent threads, one row per thread: either
// the ones you (the console) take part in, or its exchanges with other
// nodes, which you can only read.
function MessageRows({ row, now, mine, onThread }: {
  row: AgentRow;
  now: number;
  mine: boolean;
  onThread: (hubId: string, threadId: string, agent: string) => void;
}) {
  const name = row.st.agent.name;
  const seen = new Set<string>();
  const msgs = (row.hub.overview?.recent || [])
    .filter((m) => m.from === name || m.to === name)
    .filter((m) => (isMe(m.from, row.hub) || isMe(m.to, row.hub)) === mine)
    .filter((m) => (seen.has(m.thread_id) ? false : (seen.add(m.thread_id), true)))
    .slice(0, 6);
  if (!msgs.length) return <div className="app-empty-note">{mine ? L("你还没有和它对话", "No conversation with it yet") : L("最近没有和其他节点的往来", "No recent exchanges with other nodes")}</div>;
  const label = (n: string) => (isConsole(n) ? personLabel(n, row.hub) : n === name ? (row.st.agent.display_name || n) : n);
  return (
    <>
      {msgs.map((m) => (
        <button key={m.id} type="button" className="app-msg" onClick={() => onThread(row.hub.id, m.thread_id, name)}>
          <span className="meta">
            <span className="who">{label(m.from)}</span>→<span className="who">{label(m.to)}</span>
            <span className="time">{timeAgo(m.updated_at, now)}</span>
          </span>
          <span className="body">{summarize(m)}</span>
        </button>
      ))}
    </>
  );
}

// LabelsEditor shows the node's group (a group=… label) and tags (bare
// labels), and edits them in a dialog. Other key=value labels, such as
// machine=…, are left alone.
function LabelsEditor({ row, onRefresh, readOnly }: { row: AgentRow; onRefresh: () => Promise<void>; readOnly?: boolean }) {
  const ag = row.st.agent;
  const groups = groupsOf(ag);
  const tags = tagsOf(ag);
  const [open, setOpen] = useState(false);
  const [group, setGroup] = useState("");
  const [draftTags, setDraftTags] = useState<string[]>([]);
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const others = (row.hub.overview?.agents || []).map((x) => x.agent);
  const knownGroups = [...new Set(others.flatMap(groupsOf))].sort((a, b) => a.localeCompare(b, locale()));
  const knownTags = [...new Set(others.flatMap(tagsOf))].sort((a, b) => a.localeCompare(b, locale()));

  const start = () => {
    setGroup(groups[0] || "");
    setDraftTags(tags);
    setTag("");
    setError(null);
    setOpen(true);
  };
  const addTag = (t: string) => {
    const v = cleanLabel(t);
    if (v && !draftTags.includes(v)) setDraftTags([...draftTags, v]);
    setTag("");
  };
  const save = async () => {
    const g = cleanLabel(group);
    const finalTags = cleanLabel(tag) ? [...new Set([...draftTags, cleanLabel(tag)])] : draftTags;
    const add = [...(g && !groups.includes(g) ? ["group=" + g] : []), ...finalTags.filter((t) => !tags.includes(t))];
    const remove = [...groups.filter((x) => x !== g).map((x) => "group=" + x), ...tags.filter((t) => !finalTags.includes(t))];
    if (!add.length && !remove.length) { setOpen(false); return; }
    setBusy(true);
    setError(null);
    try {
      await appApi.setLabels(row.hub.id, ag.name, { add, remove });
      await onRefresh();
      setOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <button type="button" className="app-labels-summary" onClick={start} disabled={readOnly}>
        <span className="app-labels-chips">
          {groups.length === 0 && tags.length === 0 && <span className="app-labels-empty">{L("未分组，没有标签", "No group, no tags")}</span>}
          {groups.map((g) => <span key={"g" + g} className="app-label group">{g}</span>)}
          {tags.map((t) => <span key={"t" + t} className="app-label">#{t}</span>)}
        </span>
        <span className="app-labels-edit">{L("编辑", "Edit")}<span className="app-chev">›</span></span>
      </button>

      <Modal open={open} onOpenChange={(o) => !busy && setOpen(o)} title={L("分组和标签", "Group and tags")} description={L("分组和标签只用来在 App 里整理和筛选节点。", "Groups and tags are only used to organize and filter nodes in the app.")}>
        <label className="app-field">
          <span className="app-field-label">{L("分组", "Group")}</span>
          <input className="app-input" value={group} onChange={(e) => setGroup(e.target.value)} placeholder={L("不分组", "No group")} disabled={busy} />
        </label>
        {knownGroups.length > 0 && (
          <div className="app-label-picks">
            {knownGroups.map((g) => (
              <button key={g} type="button" className={cn("app-label group", cleanLabel(group) === g && "picked")} onClick={() => setGroup(g)}>{g}</button>
            ))}
          </div>
        )}
        <div className="app-field">
          <span className="app-field-label">{L("标签", "Tags")}</span>
          <div className="app-tag-box">
            {draftTags.map((t) => (
              <span key={t} className="app-label editable">
                #{t}
                <button type="button" aria-label={L(`去掉标签 ${t}`, `Remove tag ${t}`)} onClick={() => setDraftTags(draftTags.filter((x) => x !== t))}>×</button>
              </span>
            ))}
            <input
              value={tag}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return;
                if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addTag(tag); }
                if (e.key === "Backspace" && !tag && draftTags.length) setDraftTags(draftTags.slice(0, -1));
              }}
              placeholder={draftTags.length ? "" : L("输入后按回车", "Type and press Return")}
              aria-label={L("添加标签", "Add tag")}
              disabled={busy}
            />
          </div>
        </div>
        {knownTags.filter((t) => !draftTags.includes(t)).length > 0 && (
          <div className="app-label-picks">
            {knownTags.filter((t) => !draftTags.includes(t)).map((t) => (
              <button key={t} type="button" className="app-label" onClick={() => addTag(t)}>+ #{t}</button>
            ))}
          </div>
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setOpen(false)} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy}>
            {busy ? L("正在保存…", "Saving…") : L("保存", "Save")}
          </button>
        </div>
      </Modal>
    </>
  );
}

// ProviderRow: which model provider (endpoint + key) the node's CLI uses.
// A node on this computer can be pinned to a cc-switch provider, given an
// endpoint by hand, or follow the computer's own setting.
function providerSourceNote(src?: string) {
  if (src === "pinned") return L("为它指定", "Chosen for it");
  if (src === "env_file") return L("手动填写", "Entered by hand");
  if (src === "missing") return L("指定的服务商已不存在，暂用本机设置", "The chosen provider is gone; using the machine's setting");
  if (src === "machine") return L("跟随本机设置", "Follows the machine");
  return "";
}

function ProviderRow({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const rt = ag.runtime;
  const [local, setLocal] = useState<AgentProvider | null>(null);
  const [list, setList] = useState<ProviderList | null>(null);
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState("");
  const [manual, setManual] = useState({ base_url: "", api_key: "", model: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState("");

  useEffect(() => {
    let off = false;
    setLocal(null);
    setSaved("");
    appApi.agentProvider(ag.name, row.hub.id).then((p) => { if (!off) setLocal(p); }).catch(() => {});
    return () => { off = true; };
  }, [row.hub.id, ag.name]);

  const codex = (rt?.cli || ag.binding.harness) === "codex";
  const editable = !!local?.local && (local.harness === "claude" || local.harness === "codex");
  const start = async () => {
    setError(null);
    setChoice(local?.provider || "");
    setManual({ base_url: "", api_key: "", model: "" });
    setOpen(true);
    try { setList(await appApi.providers(codex ? "codex" : "claude")); } catch (e) { setError(e); }
  };
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const body = choice === "env_file"
        ? (manual.base_url || manual.api_key ? { manual } : null)
        : { provider: choice };
      if (body) setLocal(await appApi.setAgentProvider(ag.name, row.hub.id, body));
      setSaved(L("已保存，从它收到的下一条消息开始生效。", "Saved. It applies from the node's next message."));
      setOpen(false);
      window.setTimeout(() => void onRefresh(), 1500);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const current = list?.providers.find((p) => p.current);
  const name = rt?.provider || (rt ? L("CLI 自己的设置", "The CLI's own setting") : "—");
  return (
    <div className="app-kv">
      <span className="app-kv-label">{L("服务商", "Provider")}</span>
      <span className="app-kv-value">
        {editable ? (
          <button type="button" className="app-kv-link" onClick={() => void start()}>
            {name}<span className="app-chev">›</span>
          </button>
        ) : name}
        {rt?.provider_source && <span className="app-kv-note app-kv-block">{providerSourceNote(rt.provider_source)}</span>}
        {saved && <span className="app-kv-note app-kv-block app-offline-ok">{saved}</span>}
      </span>

      <Modal open={open} onOpenChange={(o) => !busy && setOpen(o)} title={L("服务商", "Provider")}
        description={L(`后台唤醒这个节点时，它的 ${codex ? "Codex" : "Claude Code"} 用哪个服务商（地址、密钥和模型）。只影响这个节点，密钥只保存在这台电脑上。`, `Which provider (endpoint, key and model) this node's ${codex ? "Codex" : "Claude Code"} uses when it is woken in the background. Only this node is affected; keys stay on this computer.`)}>
        <div className="app-provider-list" role="radiogroup">
          <label className={cn("app-provider-opt", choice === "" && "picked")}>
            <input type="radio" name="provider" checked={choice === ""} onChange={() => setChoice("")} />
            <span>
              <b>{L("跟随本机设置", "Follow this computer")}</b>
              <small>{current ? L(`现在是「${current.name}」，在 cc-switch 里切换时跟着变`, `Now “${current.name}”; changes when you switch in cc-switch`) : L(`用这台电脑上 ${codex ? "Codex" : "Claude Code"} 自己的设置`, `Uses ${codex ? "Codex" : "Claude Code"}'s own settings on this computer`)}</small>
            </span>
          </label>
          {list?.providers.map((p) => (
            <label key={p.ref} className={cn("app-provider-opt", choice === p.ref && "picked")}>
              <input type="radio" name="provider" checked={choice === p.ref} onChange={() => setChoice(p.ref)} />
              <span>
                <b>{p.name}</b>
                <small>{[p.host || L("官方地址", "Official endpoint"), p.model, "cc-switch"].filter(Boolean).join(" · ")}</small>
              </span>
            </label>
          ))}
          <label className={cn("app-provider-opt", choice === "env_file" && "picked")}>
            <input type="radio" name="provider" checked={choice === "env_file"} onChange={() => setChoice("env_file")} />
            <span>
              <b>{L("手动填写", "Enter by hand")}</b>
              <small>{local?.env_host ? L(`现在是 ${local.env_host}`, `Now ${local.env_host}`) : codex ? L("OpenAI 兼容的地址和密钥", "An OpenAI-compatible endpoint and key") : L("Anthropic 兼容的地址和密钥", "An Anthropic-compatible endpoint and key")}</small>
            </span>
          </label>
        </div>
        {choice === "env_file" && (
          <div className="app-provider-manual">
            <input className="app-input" placeholder={codex ? "https://api.example.com/v1" : "https://api.example.com/anthropic"} value={manual.base_url} onChange={(e) => setManual({ ...manual, base_url: e.target.value })} disabled={busy} />
            <input className="app-input" type="password" placeholder={L("密钥", "Key")} value={manual.api_key} onChange={(e) => setManual({ ...manual, api_key: e.target.value })} disabled={busy} autoComplete="off" />
            <input className="app-input" placeholder={L("模型（可选）", "Model (optional)")} value={manual.model} onChange={(e) => setManual({ ...manual, model: e.target.value })} disabled={busy} />
          </div>
        )}
        {list && !list.cc_switch && (
          <p className="app-modal-note">{L("装了 cc-switch 的话，这里会列出它的服务商。", "With cc-switch installed, its providers are listed here.")}</p>
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setOpen(false)} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()}
            disabled={busy || (choice === "env_file" && !local?.env_host && (!manual.base_url || !manual.api_key))}>
            {busy ? L("正在保存…", "Saving…") : L("保存", "Save")}
          </button>
        </div>
      </Modal>
    </div>
  );
}

const hostKey = (h?: string) => (h || "").toLowerCase().replace(/\.local$/, "");

function openStatusLabel(status: string): string {
  if (status === "queued") return L("待处理", "Queued");
  if (status === "held") return L("已挂起", "Held");
  return L("处理中", "In progress");
}

// The command a session keeps running to receive in the window.
const WAIT_CMD = "myco inbox wait --timeout 55m";

function receivingLabel(r?: string) {
  if (r === "session") return L("会话里在线接收", "In the session");
  if (r === "runner") return L("后台唤醒", "Woken in the background");
  return "—";
}

function receivingNote(r?: string, cloud?: boolean) {
  if (r === "session" && cloud) return L("会话自己挂着 inbox wait 收消息；停止等待后就收不到了，回到会话里让它重新开始等待。", "The session keeps inbox wait running and reads its own messages. If it stops waiting it can't receive; go back to the session and have it start waiting again.");
  if (r === "session") return L("会话自己挂着 inbox wait 收消息，runner 暂时让开；会话停止等待约 5 分钟后，runner 自动接手。", "The session keeps inbox wait running and reads its own messages, so the runner stands aside. About 5 minutes after the session stops waiting, the runner takes over.");
  if (r === "runner") return L("有消息时，runner 在后台唤醒这个会话处理，你不用开着它。", "When a message arrives, the runner wakes this session in the background to handle it. You don't need to keep it open.");
  return "";
}

// InUseCard: the runner found the session open elsewhere (a terminal or the
// IDE) and handed messages back instead of interrupting. The fix is to let
// that open session receive by itself.
function InUseCard() {
  const ask = L(
    `请在后台一直运行 \`${WAIT_CMD}\`，结束后马上再启动一次，收到消息时用 myco inbox pull 读取并处理。`,
    `Keep \`${WAIT_CMD}\` running as a background command, start it again whenever it exits, and when it reports messages read them with myco inbox pull and handle them.`,
  );
  return (
    <section className="app-offline app-inuse">
      <div className="app-offline-head">
        <span className="dot pending" />
        <b>{L("会话正在被使用", "Session in use")}</b>
      </div>
      <p>{L("这个会话正开在终端或编辑器里，runner 不会打断你，消息先排着、过一会儿再试。想让它在你开着的窗口里直接收消息，把下面这句话发给那个会话：", "This session is open in a terminal or editor, so the runner won't interrupt you: messages wait and are retried later. To have the open window receive them directly, send this sentence to that session:")}</p>
      <div className="app-command">
        <code>{ask}</code>
        <CopyButton value={ask} />
      </div>
    </section>
  );
}

// OfflineCard says why a node is offline and how to bring it back: on this
// computer, restart the runner; on another one, the command to run there;
// a cloud session comes back when its session continues.
function OfflineCard({ row, now, onRefresh }: { row: AgentRow; now: number; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const machine = machineOfRow(row);
  const [hostname, setHostname] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let off = false;
    setNote("");
    setError(null);
    appApi.local().then((l) => { if (!off) setHostname(l.hostname); }).catch(() => {});
    return () => { off = true; };
  }, [row.hub.id, ag.name]);
  const here = !!machine?.hostname && !!hostname && hostKey(machine.hostname) === hostKey(hostname);

  const restart = async () => {
    setBusy(true);
    setError(null);
    setNote("");
    try {
      const r = await appApi.restartRunner();
      if (r.runner !== "running") throw new Error(r.runner);
      setNote(L("runner 已重启，通常半分钟内会重新上线。", "The runner restarted. The node is usually back online within half a minute."));
      window.setTimeout(() => void onRefresh(), 8000);
      window.setTimeout(() => void onRefresh(), 30000);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const seen = ag.last_seen_at ? timeAgo(ag.last_seen_at, now) : "";
  return (
    <section className="app-offline">
      <div className="app-offline-head">
        <span className="dot offline" />
        <b>{L("离线", "Offline")}</b>
        {seen && <span>{L(`· 最后一次连接 Hub 是 ${seen}`, `· Last connected to the Hub ${seen}`)}</span>}
      </div>
      <p>
        {L("在线是指它最近 90 秒内连过 Hub：节点所在机器上的 runner 会每隔几秒替它连一次。 离线时发给它的任务会先排队，上线后自动送达。", "Online means it reached the Hub in the last 90 seconds: the runner on the node's machine checks in for it every few seconds. Tasks sent while it is offline wait in a queue and are delivered when it comes back.")}
      </p>
      {isCloud(ag) ? (
        <p>{L("这是云端会话，会话结束就会离线。回到那个会话继续对话，它就会重新上线。", "This is a cloud session. It goes offline when the session ends. Continue the conversation in that session to bring it back online.")}</p>
      ) : here ? (
        <>
          <p>{L("它在这台电脑上。重启这里的 runner，它就会重新连上 Hub。", "It runs on this computer. Restart the runner here and it reconnects to the Hub.")}</p>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void restart()} disabled={busy}>
            {busy ? <><span className="app-spinner" />{L("正在重启…", "Restarting…")}</> : L("重启本机的 runner", "Restart the runner on this computer")}
          </button>
        </>
      ) : (
        <>
          <p>
            {machine ? (lang() === "zh" ? <>它在「{machine.name}」上。</> : <>It runs on “{machine.name}”. </>) : null}
            {L("在那台机器的终端里运行下面这条命令（启动 runner；已经在运行就重启它）。机器关机或睡眠时，开机后会自动上线。", "Run this command in a terminal on that machine (it starts the runner, or restarts it if it's already running). If the machine is off or asleep, the node comes back online once it wakes.")}
          </p>
          <div className="app-command">
            <code>myco runner service restart || myco runner service install</code>
            <CopyButton value="myco runner service restart || myco runner service install" />
          </div>
        </>
      )}
      {note && <p className="app-offline-ok">{note}</p>}
      <ErrorText error={error} />
    </section>
  );
}

export function Inspector({ row, now, onClose, onRefresh, onThread, onMessage, onOpenMachine }: {
  row: AgentRow | null;
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onThread: (hubId: string, threadId: string, agent: string) => void;
  onMessage: (row: AgentRow) => void;
  onOpenMachine: (hubId: string, machineId: string) => void;
}) {
  const ag = row?.st.agent;
  const [action, setAction] = useState<Action>(null);
  const [people, setPeople] = useState<Member[]>([]);
  const [newOwner, setNewOwner] = useState("");
  const [copyName, setCopyName] = useState("");
  const [copied, setCopied] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [allowFrom, setAllowFrom] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setAction(null);
    setError(null);
    setBusy(false);
    setDisplayName(ag?.display_name || "");
    setDescription(ag?.description || "");
    setAllowFrom(ag?.allow_from || []);
    // Only reset when a different agent is opened, not on every 5s poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row?.hub.id, ag?.id]);

  useEffect(() => {
    if (!row) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !action) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [row, action, onClose]);

  if (!row || !ag) return null;

  const hubId = row.hub.id;
  const state = stateMeta[row.st.state];
  const kind = agentKind(ag);
  const Icon = nodeIcon(ag);
  const rt = ag.runtime;
  const machine = machineLabel(row);
  const machineObj = machineOfRow(row);
  const mayManage = canManage(row.hub, ag.owner_id);
  const me = row.hub.me;
  // Copy: fork the session into a new node on the same machine and folder.
  const forkable = kind === "agent" && (ag.binding.harness === "claude" || ag.binding.harness === "codex") &&
    !!ag.binding.session_id && !!ag.binding.workdir && !!machineObj?.can_spawn && canManage(row.hub, machineObj?.owner_id);
  const openCopy = () => {
    const names = new Set((row.hub.overview?.agents || []).map((x) => x.agent.name));
    let n = 2;
    while (names.has(`${ag.name}-${n}`)) n++;
    setCopyName(`${ag.name}-${n}`);
    setCopied("");
    openAction("copy");
  };
  const copyNode = () => void run(async () => {
    if (!machineObj) return;
    const r = await appApi.spawn(hubId, machineObj.id, {
      cli: ag.binding.harness as "claude" | "codex", dir: ag.binding.workdir || "", fork_from: ag.binding.session_id,
      name: copyName.trim(), description: ag.description || undefined,
      display_name: ag.display_name ? `${ag.display_name} ${L("副本", "copy")}` : undefined,
      ...(rt?.access ? { access: rt.access } : {}),
    });
    setCopied(r.agent.name || copyName);
  }, "stay");
  const openOwner = () => {
    setNewOwner(ag.owner || "");
    appApi.members(hubId).then((r) => setPeople(r.members)).catch(() => {});
    openAction("owner");
  };

  const openAction = (a: Exclude<Action, null>) => { setError(null); setAction(a); };
  const closeAction = () => { if (!busy) { setAction(null); setError(null); } };

  const run = async (fn: () => Promise<unknown>, after: "close-action" | "close-panel" | "stay") => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await onRefresh();
      if (after === "close-action") { setAction(null); }
      if (after === "close-panel") { setAction(null); onClose(); }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const saveProfile = () => void run(
    () => appApi.updateProfile(hubId, ag.name, { display_name: displayName.trim(), description: description.trim() }),
    "close-action",
  );
  const saveAllowFrom = () => void run(
    () => appApi.updateAllowFrom(hubId, ag.name, allowFrom),
    "close-action",
  );

  const current = row.st.current || [];

  return (
    <>
      <div className="app-inspector-backdrop" onClick={onClose} />
      <aside className="app-inspector" role="dialog" aria-label={L(`${ag.display_name || ag.name} 详情`, `${ag.display_name || ag.name} details`)}>
        <div className="app-inspector-head">
          <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
          <div className="titles">
            <h2>{ag.display_name || ag.name}</h2>
            <div className="sub">
              {ag.display_name ? `${ag.name} · ` : ""}{row.hub.name}{machine ? ` · ${machine}` : ""}
            </div>
          </div>
          <span className={cn("app-pill", pillClass[row.st.state])}>
            <span className={cn("dot", row.st.state)} />{state.label}
          </span>
          <button type="button" className="app-btn app-btn-plain" onClick={onClose} aria-label={L("关闭", "Close")}>
            <X />
          </button>
        </div>

        <div className="app-inspector-body">

          {row.st.state === "offline" && <OfflineCard row={row} now={now} onRefresh={onRefresh} />}
          {row.st.state !== "offline" && row.st.session_in_use && row.st.receiving !== "session" && <InUseCard />}

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("运行", "Runtime")}</span></div>
            <div className="app-list">
              <div className="app-kv">
                <span className="app-kv-label">{L("类型", "Type")}</span>
                <span className="app-kv-value">{kindLabel(kind)}</span>
              </div>
              {ag.owner && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("主人", "Owner")}</span>
                  <span className="app-kv-value">
                    {me?.can_manage ? (
                      <button type="button" className="app-kv-link" onClick={openOwner}>
                        {ag.owner}{me.member?.id === ag.owner_id ? L("（你）", " (you)") : ""}<span className="app-chev">›</span>
                      </button>
                    ) : <>{ag.owner}{me?.member?.id === ag.owner_id ? L("（你）", " (you)") : ""}</>}
                  </span>
                </div>
              )}
              {kind !== "human" && row.st.receiving && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("接收方式", "Receives")}</span>
                  <span className="app-kv-value" title={receivingNote(row.st.receiving, isCloud(ag))}>
                    {receivingLabel(row.st.receiving)}
                    <span className="app-kv-note app-kv-block">{receivingNote(row.st.receiving, isCloud(ag))}</span>
                  </span>
                </div>
              )}
              <div className="app-kv">
                <span className="app-kv-label">CLI</span>
                <span className="app-kv-value">
                  {kind === "human" ? "—" : <>{cliName(ag)}{cliVersion(ag) ? ` ${cliVersion(ag)}` : ""}</>}
                </span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">{L("模型", "Model")}</span>
                <span className="app-kv-value">
                  {rt?.model || "—"}
                  {rt?.model && modelSourceLabel(rt.model_source) && (
                    <span className="app-kv-note">{lang() === "zh" ? <>（{modelSourceLabel(rt.model_source)}）</> : <> ({modelSourceLabel(rt.model_source)})</>}</span>
                  )}
                </span>
              </div>
              {kind === "agent" && !isCloud(ag) && ["claude", "codex"].includes(rt?.cli || ag.binding.harness || "") && <ProviderRow row={row} onRefresh={onRefresh} />}
              <div className="app-kv">
                <span className="app-kv-label">{L("权限", "Access")}</span>
                <span className="app-kv-value">{accessLabel(rt?.access)}</span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">{L("机器", "Machine")}</span>
                <span className="app-kv-value">
                  {machineObj ? (
                    <button
                      type="button"
                      className="app-kv-link"
                      onClick={() => onOpenMachine(hubId, machineObj.id)}
                    >
                      {machineObj.name}<span className="app-chev">›</span>
                    </button>
                  ) : isCloud(ag) ? (
                    cloudLabel(ag)
                  ) : (
                    machine || "—"
                  )}
                </span>
              </div>
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("分组和标签", "Group and tags")}</span></div>
            <LabelsEditor row={row} onRefresh={onRefresh} readOnly={!mayManage} />
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("当前工作", "Current work")}</span></div>
            <div className="app-list">
              <div className="app-counts">
                <span>{L("处理中", "In progress")}<b>{row.st.in_progress}</b></span>
                <span>{L("待处理", "Queued")}<b>{row.st.queued}</b></span>
                <span>{L("已挂起", "Held")}<b>{row.st.held}</b></span>
              </div>
              {current.length === 0 && <div className="app-empty-note">{row.st.queued + row.st.in_progress + row.st.held > 0
                ? L("这些消息不在你能看到的范围里。", "These messages are not visible to you.")
                : L("暂无未完成的任务", "Nothing open")}</div>}
              {current.map((m) => (
                <div key={m.id} className="app-msg-wrap">
                  <button type="button" className="app-msg" onClick={() => onThread(hubId, m.thread_id, ag.name)}>
                    <span className="meta">
                      <span className={cn("app-msg-status", m.status)}>{openStatusLabel(m.status)}</span>
                      <span className="who">{personLabel(m.from, row.hub)}</span>→<span className="who">{personLabel(m.to, row.hub)}</span>
                      <span className="time">{timeAgo(m.updated_at, now)}</span>
                    </span>
                    <span className="body">{m.body}</span>
                  </button>
                  {mayManage && (
                    <button type="button" className="app-msg-dismiss" disabled={busy}
                      title={L("不回复，直接标记为已处理", "Mark it handled without a reply")}
                      onClick={() => void run(() => appApi.dismissMessage(hubId, m.id), "stay")}>
                      {L("忽略", "Dismiss")}
                    </button>
                  )}
                </div>
              ))}
            </div>
            {!action && error != null && <ErrorText error={error} />}
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">{L("和你的对话", "Conversations with you")}</span>
              {(kind === "agent" || kind === "shell") && (
                <button type="button" className="app-group-action" onClick={() => onMessage(row)}>
                  <MessageSquare />{L("发消息", "Message")}
                </button>
              )}
            </div>
            <div className="app-list"><MessageRows row={row} now={now} mine onThread={onThread} /></div>
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">{L("与其他节点的往来", "Exchanges with other nodes")}</span>
              <span className="app-group-note">{L("只读", "Read-only")}</span>
            </div>
            <div className="app-list"><MessageRows row={row} now={now} mine={false} onThread={onThread} /></div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("身份与绑定", "Identity and binding")}</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">Agent ID</span><span className="app-kv-value mono">{ag.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("会话 ID", "Session ID")}</span><span className="app-kv-value mono">{ag.binding.session_id || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("工作目录", "Working directory")}</span><span className="app-kv-value mono">{ag.binding.workdir || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("投递方式", "Delivery")}</span><span className="app-kv-value">{ag.delivery.mode}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("入网令牌", "Join token")}</span><span className="app-kv-value mono">{ag.enrolled_by || L("管理员注册", "Registered by admin")}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("允许来源", "Allowed from")}</span><span className="app-kv-value">{ag.allow_from.length ? ag.allow_from.join(", ") : L("任何人", "Anyone")}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("最近在线", "Last online")}</span><span className="app-kv-value">{timeAgo(ag.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("注册时间", "Registered")}</span><span className="app-kv-value">{new Date(ag.created_at).toLocaleString(locale(), { hour12: false })}</span></div>
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("操作", "Actions")}</span></div>
            <div className="app-list">
              {!mayManage && (
                <div className="app-empty-note">{L(`这是 ${ag.owner || L("别人", "someone else")} 的节点，只有 ${ag.owner || L("他", "they")} 或管理员能修改。`, `This node belongs to ${ag.owner || "someone else"}; only they or an admin can change it.`)}</div>
              )}
              <button type="button" className="app-action-row" onClick={() => openAction("profile")} disabled={!mayManage}>
                {L("修改显示名和描述…", "Edit display name and description…")}<span className="app-chev">›</span>
              </button>
              <button type="button" className="app-action-row" onClick={() => openAction("allow")} disabled={!mayManage}>
                {L("允许谁派任务…", "Who can give tasks…")}<span className="app-chev">›</span>
              </button>
              {forkable && (
                <button type="button" className="app-action-row" onClick={openCopy}>
                  {L("复制一个…", "Make a copy…")}<span className="app-chev">›</span>
                </button>
              )}
              <button type="button" className="app-action-row danger" onClick={() => openAction("kick")} disabled={!mayManage}>
                {L("踢下线", "Disconnect")}<span className="app-chev">›</span>
              </button>
              <button type="button" className="app-action-row danger" onClick={() => openAction("delete")} disabled={!mayManage}>
                {L("删除", "Delete")}<span className="app-chev">›</span>
              </button>
            </div>
          </section>
        </div>
      </aside>

      <Modal
        open={action === "copy"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("复制节点", "Copy this node")}
        description={L("在同一台机器、同一个目录里，从它的会话复制出一个新节点：带着全部上下文，之后两边各干各的。复制要跑一轮对话，可能需要一两分钟。", "Copies its session into a new node on the same machine and folder: it keeps the whole context, then the two work separately. Copying runs one turn, which can take a minute or two.")}
      >
        {copied ? (
          <>
            <div className="app-success"><Check />{L(`已复制为 ${copied}`, `Copied as ${copied}`)}</div>
            <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={closeAction}>{L("完成", "Done")}</button></div>
          </>
        ) : (
          <>
            <label className="app-field">
              <span className="app-field-label">{L("新节点的名字", "Name of the copy")}</span>
              <input className="app-input mono" value={copyName} onChange={(e) => setCopyName(e.target.value)} disabled={busy} />
            </label>
            <ErrorText error={error} />
            {busy && <div className="app-progress"><span className="app-spinner" />{L("正在复制会话…", "Copying the session…")}</div>}
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
              <button type="button" className="app-btn app-btn-primary" onClick={copyNode} disabled={busy || !copyName.trim()}>{busy ? L("正在复制…", "Copying…") : L("复制", "Copy")}</button>
            </div>
          </>
        )}
      </Modal>

      <Modal
        open={action === "owner"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("转交节点", "Give this node to someone")}
        description={L("新主人可以修改、踢下线和删除它；它收到的消息会说明是谁的 agent。", "The new owner can change, disconnect and delete it; messages it sends say whose agent it is.")}
      >
        <label className="app-field">
          <span className="app-field-label">{L("主人", "Owner")}</span>
          <select className="app-select-field" value={newOwner} onChange={(e) => setNewOwner(e.target.value)} disabled={busy}>
            {people.map((p) => <option key={p.id} value={p.name}>{p.display_name ? `${p.display_name} (@${p.name})` : p.name}</option>)}
          </select>
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" disabled={busy || !newOwner || newOwner === ag.owner}
            onClick={() => void run(() => appApi.setAgentOwner(hubId, ag.name, newOwner), "close-action")}>{L("转交", "Give")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "profile"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("修改显示名和描述", "Edit display name and description")}
        description={L("显示名和描述会在所有 Hub 上生效。显示名留空则只显示名字。", "The display name and description apply on every Hub. Leave the display name empty to show just the name.")}
      >
        <label className="app-field">
          <span className="app-field-label">{L("显示名", "Display name")}</span>
          <input className="app-input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={ag.name} />
        </label>
        <label className="app-field">
          <span className="app-field-label">{L("描述", "Description")}</span>
          <textarea className="app-input" value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveProfile} disabled={busy}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "allow"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("允许谁派任务", "Who can give tasks")}
        description={L("只有选中的节点可以给它派任务。一个都不选表示任何节点都可以。你在 App 里发的消息不受这个限制。", "Only the selected nodes can give it tasks. Select none to allow any node. Messages you send from the app are not limited by this.")}
      >
        <NodePicker
          nodes={(row.hub.overview?.agents || []).map((x) => x.agent)
            .filter((x) => x.name !== ag.name && agentKind(x) !== "machine")}
          value={allowFrom}
          onChange={setAllowFrom}
          disabled={busy}
        />
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveAllowFrom} disabled={busy}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </Modal>

      <ConfirmDialog
        open={action === "kick"}
        onOpenChange={(o) => !o && closeAction()}
        title={L(`将 ${ag.name} 踢下线？`, `Disconnect ${ag.name}?`)}
        description={L(`它会立即从 ${row.hub.name} 下线，并收不到新消息。`, `It goes offline from ${row.hub.name} right away and stops receiving new messages.`)}
        consequences={<p>{L("令牌立即作废，正在进行的任务会被中断。节点记录、消息和资料仍保留在 Hub 上，重新接入即可回来。", "Its token is revoked immediately and tasks in progress are interrupted. The node record, messages and profile stay on the Hub; join again to come back.")}</p>}
        confirmLabel={L("踢下线", "Disconnect")}
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.kick(hubId, ag.name), "close-action")}
      />
      <ConfirmDialog
        open={action === "delete"}
        onOpenChange={(o) => !o && closeAction()}
        title={L(`从 ${row.hub.name} 删除 ${ag.name}？`, `Delete ${ag.name} from ${row.hub.name}?`)}
        description={L("这个节点会从 Hub 上永久删除。", "This node will be permanently deleted from the Hub.")}
        consequences={<p>{L("节点记录和令牌都会删除，必须重新接入才能回来。已有线程里的消息仍会保留。", "The node record and token are deleted; it must join again to come back. Messages in existing threads are kept.")}</p>}
        confirmLabel={L("删除", "Delete")}
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.removeAgent(hubId, ag.name), "close-panel")}
      />
    </>
  );
}
