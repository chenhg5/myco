import { fitMenu } from "@/app/fitMenu";
import { useEffect, useState, useSyncExternalStore } from "react";
import { appApi, isOldHub } from "@/appapi";
import type { RunSettings, Runtime, RuntimeAccess, RuntimeModel } from "@/api";
import type { AgentRow } from "@/app/types";
import { accessLabel, agentKind, canManage, isCloud, isInbox, machineOfRow, machinesOfHub, modelSourceLabel, nodeName } from "@/app/nodes";
import { ChevronDown, ChevronRight, Shield, ShieldOff, TriangleAlert } from "lucide-react";
import { hubAfter0194 } from "@/app/hubVersion";
import { ConfirmDialog, ErrorText } from "@/app/dialogs";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { quotaHasNumbers, quotaSourceText, quotaSpent, quotaStaleNote, quotaStatusText, quotaTight, quotaWindowLabel, quotaWindowText } from "@/app/quota";
import { cn, timeAgo } from "@/lib/utils";
import { L, lang, E } from "@/lib/i18n";

/** The models a node may be told to use when its computer does not say
 *  (Claude Code's aliases; Codex on runners before v0.21.1, which do not
 *  read the list Codex keeps): modelChoices prefers what it says. */
export const MODELS: Record<string, string[]> = {
  claude: ["fable", "opus", "sonnet", "haiku"],
  codex: ["gpt-6-sol", "gpt-6", "gpt-5-codex", "o3"],
};
export const EFFORTS: Record<string, string[]> = {
  claude: ["low", "medium", "high", "xhigh", "max"],
  codex: ["minimal", "low", "medium", "high", "xhigh"],
};
const ACCESS: RuntimeAccess[] = ["readonly", "workspace", "full"];

const EFFORT_LABELS: Record<string, [string, string]> = {
  minimal: ["最低", "Minimal"],
  low: ["低", "Low"],
  medium: ["中", "Medium"],
  high: ["高", "High"],
  xhigh: ["很高", "Very high"],
  max: ["最高", "Max"],
};

/** The models to offer for an agent or a computer's CLI: the list its
 *  CLI offers there when it said (Codex's own, always current), else the
 *  known names; the ones in use or chosen are added when missing. */
// What was just picked, shown at once: the save and the App's refresh take
// a moment (a refresh reads every workspace), and the picker should not wait
// for them. Dropped once the agent's own run settings say the same, or put
// back when the save fails.
const picked = new Map<string, RunSettings>();
const pickedSubs = new Set<() => void>();
let pickedVersion = 0;
const pickedKey = (row: AgentRow) => `${row.hub.id}\u0000${row.st.agent.name}`;
const same = (a?: RunSettings, b?: RunSettings) =>
  (a?.model || "") === (b?.model || "") && (a?.effort || "") === (b?.effort || "") && (a?.access || "") === (b?.access || "") && !!a?.mcp_tools === !!b?.mcp_tools;

/** An agent's run settings as last picked here, else as the hub says. */
function useRunSettings(row: AgentRow): RunSettings | undefined {
  useSyncExternalStore((f) => { pickedSubs.add(f); return () => { pickedSubs.delete(f); }; }, () => pickedVersion);
  const key = pickedKey(row);
  const own = row.st.agent.run_settings;
  const mine = picked.get(key);
  useEffect(() => {
    if (mine && same(mine, own)) { picked.delete(key); pickedVersion += 1; }
  }, [key, mine, own]);
  return mine || own;
}

/** Saves an agent's run settings; the pick shows at once, the App's
 *  refresh follows in the background. */
async function saveRunSettings(row: AgentRow, next: RunSettings, onRefresh: () => Promise<void>): Promise<void> {
  const key = pickedKey(row);
  const before = picked.get(key);
  next = { ...(before || row.st.agent.run_settings), ...next };
  picked.set(key, next);
  pickedVersion += 1; pickedSubs.forEach((f) => f());
  try {
    await appApi.setRunSettings(row.hub.id, row.st.agent.name, { model: next.model || "", effort: next.effort || "", access: next.access || "", ...(next.mcp_tools !== undefined ? { mcp_tools: next.mcp_tools } : {}) });
    void onRefresh().catch(() => undefined);
  } catch (e) {
    if (before) picked.set(key, before); else picked.delete(key);
    pickedVersion += 1; pickedSubs.forEach((f) => f());
    throw e;
  }
}

export function modelChoices(cli: string, offered?: RuntimeModel[], ...keep: (string | undefined)[]): string[] {
  const out = offered && offered.length ? offered.map((m) => m.id) : [...(MODELS[cli] || [])];
  for (const v of keep) if (v && !out.includes(v)) out.unshift(v);
  return out;
}

/** The efforts to offer: those the chosen model supports when its CLI
 *  said, else the CLI's usual ones. */
export function effortChoices(cli: string, offered: RuntimeModel[] | undefined, model: string | undefined, ...keep: (string | undefined)[]): string[] {
  const m = offered?.find((x) => x.id === model) || (offered?.length && !model ? offered[0] : undefined);
  const out = m?.efforts?.length ? [...m.efforts] : [...(EFFORTS[cli] || [])];
  for (const v of keep) if (v && !out.includes(v)) out.unshift(v);
  return out;
}

/** How a model is shown: the name its CLI gave it, else modelShort. */
export function modelName(model: string | undefined, rt?: Pick<Runtime, "models">): string {
  if (!model) return "";
  return rt?.models?.find((m) => m.id === model)?.name || modelShort(model);
}

export function effortLabel(effort: string): string {
  const pair = EFFORT_LABELS[effort];
  return pair ? L(pair[0], pair[1]) : effort;
}

/** One value with where it came from, as a row shows it. */
function SourceValue({ value, source, sourceLabel }: { value?: string; source?: Parameters<typeof modelSourceLabel>[0]; sourceLabel?: string }) {
  const label = sourceLabel || modelSourceLabel(source);
  return (
    <>
      {value || "—"}
      {value && label && (
        <span className="app-kv-note">{lang() === "zh" ? <>（{label}）</> : <> ({label})</>}</span>
      )}
    </>
  );
}

/**
 * A runner agent's run block: the model and effort it is woken with, and its
 * CLI's own quota windows. Those who may manage it pick a model or effort (or
 * follow the computer's setting); an agent that receives in its own session
 * is left to that session.
 */
export function RunRows({ row, now, mayManage, onRefresh, part = "all" }: {
  row: AgentRow;
  now: number;
  mayManage: boolean;
  onRefresh: () => Promise<void>;
  /** "settings": model, effort, access; "quota": the plan's windows;
   *  "all": both (the inspector). The agent card shows them apart. */
  part?: "all" | "settings" | "quota";
}) {
  const ag = row.st.agent;
  const rt = ag.runtime;
  const cli = rt?.cli || ag.binding.harness || "";
  const tunable = agentKind(ag) === "agent" && !isCloud(ag) && (cli === "claude" || cli === "codex");
  // Set when the hub answered 404/405: it has no run settings, so the block
  // stays read-only (older hubs, or one released without the feature).
  const [old, setOld] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirmAccess, setConfirmAccess] = useState(false);
  const [custom, setCustom] = useState(false);
  const [draft, setDraft] = useState("");
  const editable = mayManage && tunable && !old && hubAfter0194(row.hub.hub_version);

  const member = row.hub.me?.member;
  const owns = !!member && (ag.owner_id ? ag.owner_id === member.id
    : ag.owner ? ag.owner === member.name : member.role === "owner");
  const runner = agentKind(ag) === "agent" && !isCloud(ag) && !isInbox(ag);
  const accessEditable = owns && runner && !!rt?.max_access && !old && hubAfter0194(row.hub.hub_version);
  const mcpAvailable = owns && runner && !!rt?.features?.includes("mcp_tools");
  const cap = ACCESS[Math.max(ACCESS.indexOf(rt?.max_access as RuntimeAccess), ACCESS.indexOf(rt?.access as RuntimeAccess), 0)];
  const rs = useRunSettings(row);
  const access = rs?.access || "";
  const model = rs?.model || "";
  const effort = rs?.effort || "";
  const save = async (next: { model: string; effort: string; access?: RuntimeAccess | ""; mcp_tools?: boolean }) => {
    setBusy(true);
    setError(null);
    try {
      await saveRunSettings(row, next, onRefresh);
      return true;
    } catch (e) {
      if (isOldHub(e)) setOld(true);
      else setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  };
  const pickModel = (v: string) => {
    if (v === "__custom__") {
      setDraft(model || rt?.model || "");
      setCustom(true);
      return;
    }
    void save({ model: v, effort });
  };
  const commitCustom = () => {
    const v = draft.trim();
    setCustom(false);
    if (v && v !== model) void save({ model: v, effort });
  };

  const pickAccess = (next: RuntimeAccess | "") => {
    if (!accessEditable || (next && ACCESS.indexOf(next) > ACCESS.indexOf(cap))) return;
    setError(null);
    if (next === "full" && access !== "full") setConfirmAccess(true);
    else void save({ model, effort, access: next });
  };
  const accessRow = (
    <div className={cn("app-kv", runner && "has-help")}>
      <span className="app-kv-label">{L("权限", "Access")}</span>
      <span className="app-kv-value">
        {accessEditable ? (
          <select className="app-select-field app-kv-select" aria-label={L("权限", "Access")} value={access} disabled={busy}
            onChange={(e) => pickAccess(e.target.value as RuntimeAccess | "")}>
            <option value="">{L("跟随电脑设置", "Follow this computer")}</option>
            {ACCESS.map((level) => <option key={level} value={level} disabled={ACCESS.indexOf(level) > ACCESS.indexOf(cap)}>{accessLabel(level)}</option>)}
          </select>
        ) : <SourceValue value={accessLabel(rt?.access)} sourceLabel={access ? "App" : undefined} />}
      </span>
      {runner && (
        <div className="app-kv-help">
          {accessEditable && <div>{L("现在用的：", "In use: ")}<SourceValue value={accessLabel(rt?.access)} sourceLabel={access ? "App" : undefined} /></div>}
          {!rt?.max_access && L("升级那台电脑上的 myco 后才能在这里改。", "Upgrade myco on that computer to change this here.")}
          {/* (how to allow more: LocalShellAccess under it, "Allow" on this
              computer, or where to change it on another) */}
          {rt?.max_access && cap !== "full" && <div>{L(
            E`这台电脑最多允许「${accessLabel(cap)}」。`,
            E`This computer allows at most ${accessLabel(cap)}.`,
          )}</div>}
          {accessEditable && cap !== "full" && <LocalShellAccess machine={machineOfRow(row)} hub={row.hub} what="full" onlyLocal onChange={onRefresh}
            agent={{ name: nodeName(ag, row.hub), live: !!rt?.features?.includes("live_max_access") }} />}
        </div>
      )}
    </div>
  );
  const mcpFull = (access || rt?.access) === "full";
  const mcpRow = mcpAvailable && (
    <div className="app-kv has-help">
      <span className="app-kv-label">{L("允许 MCP 工具", "Allow MCP tools")}</span>
      <span className="app-kv-value">
        <button type="button" role="switch" aria-checked={mcpFull || !!rs?.mcp_tools} aria-label={L("允许 MCP 工具", "Allow MCP tools")}
          disabled={busy || old || mcpFull} className={cn("app-switch", (mcpFull || !!rs?.mcp_tools) && "is-on")}
          onClick={() => void save({ model, effort, access, mcp_tools: !rs?.mcp_tools })}>
          <span className="app-switch-knob" />
        </button>
      </span>
      <span className="app-kv-help">{mcpFull
        ? L("完全权限下本来就全部放行。", "Full access already allows all tools.")
        : L("打开后，这个 agent 在工作区权限下也能用它电脑上配置的 MCP 工具（它们能做的事不受文件夹限制）。默认关。", "When on, this agent may use MCP tools configured on its computer even with workspace access. Their actions are not limited to its folder. Off by default.")}</span>
    </div>
  );

  const models = modelChoices(cli, rt?.models, rt?.model, model);
  const efforts = effortChoices(cli, rt?.models, model || rt?.model, rt?.effort, effort);

  // An agent that receives in its own session runs its CLI itself: myco can
  // neither read nor set its model; its quota is there once its session
  // reports it (myco agent runtime quota, e.g. a Claude cloud session).
  if (isInbox(ag)) {
    if (part === "quota") return rt?.quota ? <QuotaRows quota={rt.quota} row={row} now={now} /> : null;
    return (
      <><div className="app-kv has-help">
        <span className="app-kv-label">{L("模型", "Model")}</span>
        <span className="app-kv-value">
          {rt?.model_source === "self"
            ? <SourceValue value={rt.model} source="self" />
            : "—"}
        </span>
        <span className="app-kv-help">
          {rt?.quota
            ? L("模型由它自己的会话管理，myco 改不了；额度是它的会话上报的。", "Its own session manages the model, which myco can't change; its session reports the quota.")
            : L("模型和额度由它自己的会话管理；会话上报额度后，这里就能看到。", "Its own session manages the model and quota; the quota shows here once its session reports it.")}
        </span>
      </div>{accessRow}{mcpRow}</>
    );
  }

  const quota = rt?.quota;
  const quotaRows = quota ? <QuotaRows quota={quota} row={row} now={now} /> : null;
  if (part === "quota") return quotaRows;

  return (
    <>
      <div className={cn("app-kv", editable && "has-help")}>
        <span className="app-kv-label">{L("模型", "Model")}</span>
        <span className="app-kv-value">
          {editable ? (
            custom ? (
              <span className="app-run-custom">
                <input
                  className="app-input app-run-input"
                  value={draft}
                  autoFocus
                  aria-label={L("自定义模型", "Custom model")}
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") commitCustom();
                    if (e.key === "Escape") setCustom(false);
                  }}
                />
                <button type="button" className="app-btn app-btn-small" onClick={commitCustom}>{L("保存", "Save")}</button>
              </span>
            ) : (
              <select
                className="app-select-field app-kv-select"
                value={model}
                disabled={busy}
                aria-label={L("模型", "Model")}
                onChange={(e) => pickModel(e.target.value)}
              >
                <option value="">{L("跟随电脑设置", "Follow this computer")}</option>
                {models.map((m) => <option key={m} value={m}>{modelName(m, rt) === m ? m : `${modelName(m, rt)} (${m})`}</option>)}
                <option value="__custom__">{L("自定义…", "Custom…")}</option>
              </select>
            )
          ) : <SourceValue value={rt?.model} source={rt?.model_source} />}
        </span>
        {editable && (
          <span className="app-kv-help">
            {L("现在用的：", "In use: ")}<SourceValue value={rt?.model} source={rt?.model_source} />
          </span>
        )}
        <ModelsSource row={row} now={now} mayRefresh={editable} onRefresh={onRefresh} />
      </div>

      {tunable && (
        <div className={cn("app-kv", editable && "has-help")}>
          <span className="app-kv-label">{L("推理强度", "Effort")}</span>
          <span className="app-kv-value">
            {editable ? (
              <select
                className="app-select-field app-kv-select"
                value={effort}
                disabled={busy}
                aria-label={L("推理强度", "Effort")}
                onChange={(e) => void save({ model, effort: e.target.value })}
              >
                <option value="">{L("跟随电脑设置", "Follow this computer")}</option>
                {efforts.map((x) => <option key={x} value={x}>{effortLabel(x)}</option>)}
              </select>
            ) : rt?.effort ? <SourceValue value={effortLabel(rt.effort)} source={rt.effort_source} /> : "—"}
          </span>
          {editable && rt?.effort && (
            <span className="app-kv-help">
              {L("现在用的：", "In use: ")}{effortLabel(rt.effort)}
              {rt?.effort && modelSourceLabel(rt.effort_source) ? L(E`（${modelSourceLabel(rt.effort_source)}）`, E` (${modelSourceLabel(rt.effort_source)})`) : ""}
            </span>
          )}
        </div>
      )}

      {accessRow}
      {mcpRow}

      {part === "all" && quotaRows}
      <ErrorText error={confirmAccess ? null : error} />
      <ConfirmDialog open={confirmAccess} onOpenChange={(open) => { if (!busy) setConfirmAccess(open); }}
        title={L("使用完全权限？", "Use full access?")}
        description={L("完全权限下，它运行时没有沙箱，可以读写这台电脑上的任何文件。确定吗？", "With full access, it runs without a sandbox and can read and write any file on this computer. Continue?")}
        confirmLabel={L("使用完全权限", "Use full access")} busy={busy} error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => {
          if (accessEditable && cap === "full") void save({ model, effort, access: "full" }).then((saved) => { if (saved) setConfirmAccess(false); });
        }} />
    </>
  );
}

/** Where an agent's model list came from, when it is its provider's (or
 *  the built-in list for it), with a way to list again on runners that
 *  can: the request goes through the hub, and the runner reads it within
 *  a minute. */
function ModelsSource({ row, now, mayRefresh, onRefresh }: { row: AgentRow; now: number; mayRefresh: boolean; onRefresh: () => Promise<void> }) {
  const rt = row.st.agent.runtime;
  const from = rt?.models_from;
  const at = rt?.models_at || "";
  const [asked, setAsked] = useState<{ at: string } | null>(null);
  const [error, setError] = useState<unknown>(null);
  // Done once a new list is there; given up after a while (the runner may
  // be busy or offline): the line says what it has.
  useEffect(() => { if (asked && at !== asked.at) setAsked(null); }, [at, asked]);
  useEffect(() => {
    if (!asked) return;
    const pulls = [15_000, 40_000, 70_000].map((ms) => setTimeout(() => void onRefresh().catch(() => undefined), ms));
    const end = setTimeout(() => setAsked(null), 90_000);
    return () => { pulls.forEach(clearTimeout); clearTimeout(end); };
  }, [asked, onRefresh]);
  if (from !== "provider" && from !== "catalog") return null;
  const where = rt?.provider || rt?.provider_host || L("服务商", "the provider");
  const canRefresh = mayRefresh && !!rt?.features?.includes("models_refresh");
  const refresh = async () => {
    setError(null);
    setAsked({ at });
    try {
      await appApi.setRunSettings(row.hub.id, row.st.agent.name, { models_refresh: new Date().toISOString() });
      void onRefresh().catch(() => undefined);
    } catch (e) {
      setAsked(null);
      setError(e);
    }
  };
  return (
    <span className="app-kv-help" data-testid="run-models-source">
      {from === "provider"
        ? L(E`模型列表从 ${where} 读取`, E`Models as ${where} lists them`) + (at ? ` · ${timeAgo(at, now)}` : "")
        : L(E`内置列表：还没能从 ${where} 读取`, E`Built-in list: ${where} could not be asked yet`)}
      {canRefresh && (
        <>
          {" · "}
          <button type="button" className="app-kv-link" disabled={!!asked} onClick={() => void refresh()}>
            {asked ? L("正在读取…", "Listing…") : L("重新读取", "List again")}
          </button>
        </>
      )}
      <ErrorText error={error} />
    </span>
  );
}

/** A model's name, short, as a person says it: claude-opus-5-5 → Opus 5.5,
 *  gpt-6-sol → GPT-6 Sol, opus → Opus. */
export function modelShort(model?: string): string {
  if (!model) return "";
  let m = model.trim().replace(/\[.*\]$/, "").replace(/^claude-/i, "").replace(/-\d{8}$/, "");
  if (/^gpt-/i.test(m)) {
    m = m.replace(/^gpt-/i, "GPT-");
    return m.replace(/-([a-z])/g, (_, c: string) => ` ${c.toUpperCase()}`);
  }
  // name-5-5 → Name 5.5; name-4 → Name 4
  const parts = m.split("-");
  const words: string[] = [];
  const nums: string[] = [];
  for (const p of parts) (/^\d+$/.test(p) ? nums : words).push(p);
  const name = words.map((w) => (w.length <= 2 ? w.toUpperCase() : w[0].toUpperCase() + w.slice(1))).join(" ");
  return nums.length ? `${name} ${nums.join(".")}` : name;
}

/**
 * The model and reasoning effort beside the message box (as Codex and
 * Claude show theirs at the bottom of their input): small, quiet. For an
 * agent a runner wakes, those who may manage it switch both here (from its
 * next run, as in its profile's run block); an agent in its own session (it
 * receives there, or runs in the cloud) decides itself: shown, and where it
 * is switched said. Nothing known, nothing shown.
 */
export function RunPill({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const rt = ag.runtime;
  const cli = rt?.cli || ag.binding.harness || "";
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [old, setOld] = useState(false);
  const runner = agentKind(ag) === "agent" && !isCloud(ag) && !isInbox(ag) && (cli === "claude" || cli === "codex");
  const editable = runner && canManage(row.hub, ag.owner_id) && !old && hubAfter0194(row.hub.hub_version);
  const rs = useRunSettings(row);
  const setModel = rs?.model || "";
  const setEffort = rs?.effort || "";
  const model = setModel || rt?.model || "";
  const effort = setEffort || rt?.effort || "";
  // In its own session and not said yet: shown as unknown, and how it comes to be known.
  const unknown = !model && !editable && (isInbox(ag) || isCloud(ag));
  if (agentKind(ag) !== "agent" || (!model && !editable && !unknown)) return null;
  const label = unknown ? L("模型未知", "Model unknown")
    : [modelName(model, rt) || L("默认模型", "Default model"), effort ? effortLabel(effort) : ""].filter(Boolean).join(" · ");
  const save = async (next: { model: string; effort: string }) => {
    setBusy(true); setError(null);
    setOpen(false);
    try {
      await saveRunSettings(row, { ...next, access: rs?.access || "" }, onRefresh);
    } catch (e) {
      if (isOldHub(e)) setOld(true); else setError(e);
    } finally { setBusy(false); }
  };
  const models = modelChoices(cli, rt?.models, rt?.model, setModel);
  const efforts = effortChoices(cli, rt?.models, setModel || rt?.model, rt?.effort, setEffort);
  const pending = editable && ((setModel && setModel !== rt?.model) || (setEffort && setEffort !== rt?.effort));
  const title = editable
    ? L(E`模型和推理强度：${label}（点一下切换，下次唤醒时生效）`, E`Model and effort: ${label} (click to switch; from its next run)`)
    : unknown ? L("它还没说自己用的哪个模型", "It has not said which model it runs on")
      : L(E`模型和推理强度：${label}（它自己的会话决定）`, E`Model and effort: ${label} (its own session decides)`);
  return (
    <span className="run-pill-wrap">
      <button type="button" className={cn("run-pill", editable && "is-editable", pending && "is-pending", unknown && "is-unknown")} title={title} aria-label={title}
        aria-expanded={open} onClick={() => setOpen((o) => !o)} onMouseDown={(e) => e.preventDefault()}>
        <span className="run-pill-text">{label}</span>
        {editable && <ChevronDown className="run-pill-chev" aria-hidden />}
      </button>
      {open && (
        <>
          <span className="run-pill-scrim" onClick={() => setOpen(false)} />
          <div ref={fitMenu} className="run-pill-menu" role="menu" onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}>
            {editable ? (
              <>
                <div className="run-pill-group">{L("模型", "Model")}</div>
                {["", ...models].map((m) => (
                  <button key={`m:${m}`} type="button" role="menuitemradio" aria-checked={setModel === m} className={cn("run-pill-item", setModel === m && "is-on")}
                    disabled={busy} onClick={() => void save({ model: m, effort: setEffort })}>
                    <span>{m ? modelName(m, rt) : L("跟随电脑设置", "Follow this computer")}</span>
                    {m && m !== modelName(m, rt) && <span className="run-pill-sub">{m}</span>}
                  </button>
                ))}
                <div className="run-pill-group">{L("推理强度", "Effort")}</div>
                <div className="run-pill-efforts">
                  {["", ...efforts].map((x) => (
                    <button key={`e:${x}`} type="button" role="menuitemradio" aria-checked={setEffort === x} className={cn("run-pill-chip", setEffort === x && "is-on")}
                      disabled={busy} onClick={() => void save({ model: setModel, effort: x })}
                      title={x ? undefined : L("跟随电脑设置", "Follow this computer")}>
                      {x ? effortLabel(x) : L("默认", "Default")}
                    </button>
                  ))}
                </div>
                <p className="run-pill-note">
                  {L("现在是：", "In use: ")}{modelName(rt?.model, rt) || "—"}{rt?.effort ? ` · ${effortLabel(rt.effort)}` : ""}
                  {L("。切换后下次唤醒时生效。", ". A switch applies from its next run.")}
                </p>
              </>
            ) : unknown ? (
              <p className="run-pill-note">
                {L("它在自己的会话里收消息，还没说用的哪个模型。它的 myco 升级到新版后，会在等消息时自动报上来；也可以让它运行 ",
                  "It receives in its own session and has not said which model it runs on. With a newer myco it says so itself while it waits for messages; or ask it to run ")}
                <code>myco agent runtime set</code>{L("。", ".")}
              </p>
            ) : (
              <p className="run-pill-note">
                {model && <><b>{model}</b>{effort ? ` · ${effortLabel(effort)}` : ""}<br /></>}
                {isInbox(ag) || isCloud(ag)
                  ? cli === "codex" ? L("这是它自己的会话在用的设置，在它的会话里用 /model 切换。", "Its own session's setting: switch it there with /model.")
                    : L("这是它自己的会话在用的设置，在它的会话里用 /model 切换。", "Its own session's setting: switch it there with /model.")
                  : L("只有管理它的人能在这里切换。", "Only those who manage it can switch it here.")}
              </p>
            )}
            <ErrorText error={error} />
          </div>
        </>
      )}
    </span>
  );
}

const ACCESS_SAYS: Record<RuntimeAccess, [string, string]> = {
  readonly: ["只能看文件，不改动", "Reads files, changes nothing"],
  workspace: ["改它文件夹里的文件，能联网；别处不碰", "Changes files in its folder, uses the network; nothing else"],
  full: ["没有沙箱：这台电脑上的什么都能动", "No sandbox: anything on that computer"],
};

/**
 * Its access beside the model (as Codex shows its sandbox under the input):
 * what it may touch when it runs, switched here by its owner from its next
 * run, up to what its computer allows (this computer can allow more right
 * there). An agent in its own session or in the cloud shows nothing. One
 * whose computer sets no level (a session added from this computer) runs
 * with its CLI's own default, shown as such, and can be switched too: for
 * Codex that default is a sandbox without network, where myco itself fails.
 */
export function AccessPill({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const rt = ag.runtime;
  const cli = rt?.cli || ag.binding.harness || "";
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [old, setOld] = useState(false);
  const [confirmFull, setConfirmFull] = useState(false);
  const runner = agentKind(ag) === "agent" && !isCloud(ag) && !isInbox(ag) && (cli === "claude" || cli === "codex");
  const member = row.hub.me?.member;
  const owns = !!member && (ag.owner_id ? ag.owner_id === member.id
    : ag.owner ? ag.owner === member.name : member.role === "owner");
  const editable = runner && owns && !!rt?.max_access && !old && hubAfter0194(row.hub.hub_version);
  const mcpAvailable = runner && owns && !!rt?.features?.includes("mcp_tools");
  const rs = useRunSettings(row);
  if (!runner || !rt) return null;
  const chosen = rs?.access || "";
  const cap = ACCESS[Math.max(ACCESS.indexOf(rt.max_access as RuntimeAccess), ACCESS.indexOf(rt.access as RuntimeAccess), 0)];
  const shown = (chosen || rt.access || "") as RuntimeAccess | "";
  const pending = !!chosen && chosen !== rt.access;
  // (its CLI's own default: no level from myco)
  const cliDefault = L("CLI 默认", "CLI default");
  const label = shown === "full" ? L("完全权限", "Full access") : shown === "readonly" ? L("只读", "Read-only") : shown === "workspace" ? L("工作区权限", "Workspace access") : L("默认权限", "Default access");
  const inUse = rt.access ? accessLabel(rt.access as RuntimeAccess) : cliDefault;
  const title = editable
    ? L(E`权限：${label}（点一下切换，下次唤醒时生效）`, E`Access: ${label} (click to switch; from its next run)`)
    : L(E`权限：${label}`, E`Access: ${label}`);
  const save = async (access: RuntimeAccess | "") => {
    setBusy(true); setError(null);
    try {
      setOpen(false);
      setConfirmFull(false);
      await saveRunSettings(row, { model: rs?.model || "", effort: rs?.effort || "", access }, onRefresh);
    } catch (e) {
      if (isOldHub(e)) setOld(true); else setError(e);
    } finally { setBusy(false); }
  };
  const saveMCP = async () => {
    setBusy(true); setError(null);
    try {
      await saveRunSettings(row, { model: rs?.model || "", effort: rs?.effort || "", access: chosen, mcp_tools: !rs?.mcp_tools }, onRefresh);
    } catch (e) {
      if (isOldHub(e)) setOld(true); else setError(e);
    } finally { setBusy(false); }
  };
  const pick = (next: RuntimeAccess | "") => {
    if (!editable || (next && ACCESS.indexOf(next) > ACCESS.indexOf(cap))) return;
    if (next === "full" && chosen !== "full") { setOpen(false); setConfirmFull(true); return; }
    void save(next);
  };
  return (
    <span className="run-pill-wrap">
      <button type="button" className={cn("run-pill", editable && "is-editable", pending && "is-pending", shown === "full" && "is-full")} title={title} aria-label={title}
        aria-expanded={open} onClick={() => setOpen((o) => !o)} onMouseDown={(e) => e.preventDefault()} data-testid="access-pill">
        {shown === "full" ? <ShieldOff className="run-pill-icon" aria-hidden /> : <Shield className="run-pill-icon" aria-hidden />}
        <span className="run-pill-text">{label}</span>
        {editable && <ChevronDown className="run-pill-chev" aria-hidden />}
      </button>
      {open && (
        <>
          <span className="run-pill-scrim" onClick={() => setOpen(false)} />
          <div ref={fitMenu} className="run-pill-menu" role="menu" onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}>
            {editable ? (
              <>
                <div className="run-pill-group">{L("它运行时能动什么", "What it may touch when it runs")}</div>
                {(["", ...ACCESS] as (RuntimeAccess | "")[]).map((level) => {
                  const over = !!level && ACCESS.indexOf(level) > ACCESS.indexOf(cap);
                  return (
                    <button key={`a:${level}`} type="button" role="menuitemradio" aria-checked={chosen === level} className={cn("run-pill-item run-pill-access", chosen === level && "is-on")}
                      disabled={busy || over} onClick={() => pick(level)}>
                      <span>{level ? accessLabel(level) : L("跟随电脑设置", "Follow this computer")}</span>
                      <span className="run-pill-sub">{level ? L(ACCESS_SAYS[level][0], ACCESS_SAYS[level][1]) : rt.access ? accessLabel(rt.access as RuntimeAccess) : L("它的 CLI 自己的设置（Codex 默认不能联网，用不了 myco）", "Its CLI's own setting (Codex's default has no network, so myco fails)")}</span>
                    </button>
                  );
                })}
                <p className="run-pill-note">
                  {L("现在是：", "In use: ")}{inUse}{L("。切换后下次唤醒时生效。", ". A switch applies from its next run.")}
                </p>
                {mcpAvailable && <button type="button" role="menuitemcheckbox" aria-checked={shown === "full" || !!rs?.mcp_tools}
                  className={cn("run-pill-item", (shown === "full" || rs?.mcp_tools) && "is-on")}
                  disabled={busy || shown === "full"}
                  onClick={() => void saveMCP()}>
                  <span>{L("允许 MCP 工具", "Allow MCP tools")}</span>
                  <span className="run-pill-sub">{shown === "full"
                    ? L("完全权限下本来就全部放行", "Full access already allows all tools")
                    : L("工作区权限下也能用；MCP 不受文件夹限制。默认关。", "Available with workspace access; MCP is not folder-limited. Off by default.")}</span>
                </button>}
                {cap !== "full" && (
                  <div className="run-pill-note">
                    {L(E`那台电脑最多允许「${accessLabel(cap)}」。`, E`That computer allows at most ${accessLabel(cap)}. `)}
                    <LocalShellAccess machine={machineOfRow(row)} hub={row.hub} what="full" onlyLocal onChange={onRefresh}
                      agent={{ name: nodeName(ag, row.hub), live: !!rt.features?.includes("live_max_access") }} />
                  </div>
                )}
              </>
            ) : (
              <p className="run-pill-note">
                <b>{label}</b><br />
                {shown ? L(ACCESS_SAYS[shown][0], ACCESS_SAYS[shown][1]) : L("用它的 CLI 自己的设置", "Its CLI's own setting")}
                {!owns ? L("。只有它的主人能改。", ". Only its owner can change it.")
                  : !rt.max_access ? L("。升级那台电脑上的 myco 后才能在这里改。", ". Upgrade myco on that computer to change it here.") : "。"}
              </p>
            )}
            <ErrorText error={error} />
          </div>
        </>
      )}
      <ConfirmDialog open={confirmFull} onOpenChange={(o) => { if (!busy) setConfirmFull(o); }}
        title={L("使用完全权限？", "Use full access?")}
        description={L("完全权限下，它运行时没有沙箱，可以读写这台电脑上的任何文件。确定吗？", "With full access, it runs without a sandbox and can read and write any file on this computer. Continue?")}
        confirmLabel={L("使用完全权限", "Use full access")} busy={busy} error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void save("full")} />
    </span>
  );
}

/** An account's usage: whose it is (provider · plan · computer), what went
 *  wrong when the last read failed, then each window left and when it
 *  resets. A failure keeps the last numbers with their age and never shows
 *  as a full quota; a model's own buckets fold away. */
function QuotaRows({ quota, row, now }: { quota: NonNullable<Runtime["quota"]>; row: AgentRow; now: number }) {
  const [more, setMore] = useState(false);
  const has = quotaHasNumbers(quota);
  const status = quotaStatusText(quota);
  const stale = quotaStaleNote(quota, now);
  const source = quotaSourceText(quota, machinesOfHub(row.hub));
  const windows = has ? quota.windows : [];
  const main = windows.filter((w) => !w.model_scope);
  const scoped = windows.filter((w) => !!w.model_scope);
  const one = (w: (typeof windows)[number]) => (
    <div key={`${w.name}/${w.model_scope || ""}`} className={cn("app-kv app-quota", quotaSpent(w) ? "is-over" : quotaTight(w) && "is-tight", (stale || status) && "is-stale")}>
      <span className="app-kv-label">{quotaWindowLabel(w)}</span>
      <span className="app-kv-value">{quotaWindowText(w, now)}</span>
    </div>
  );
  const action = quota.status === "auth_required" || quota.status === "no_credentials";
  return (
    <>
      {source && (
        <div className="app-kv app-quota-source">
          <span className="app-kv-label">{L("来源", "Source")}</span>
          <span className="app-kv-value">{source}</span>
        </div>
      )}
      {(status || !has) && (
        <div className={cn("app-quota-status", action && "is-action")} role="status">
          <TriangleAlert aria-hidden />
          <span>{status || L("还没有额度数据", "No usage yet")}{has ? L("，下面是上次取到的。", "; below is the last read.") : L("，所以不显示数字。", ", so no numbers are shown.")}</span>
        </div>
      )}
      {(main.length ? main : scoped).map(one)}
      {main.length > 0 && scoped.length > 0 && (
        <>
          <button type="button" className="app-quota-more" aria-expanded={more} onClick={() => setMore((v) => !v)}>
            <ChevronRight aria-hidden className={cn(more && "is-open")} />
            {L(E`按模型的 ${scoped.length} 个窗口`, E`${scoped.length} per-model windows`)}
          </button>
          {more && scoped.map(one)}
        </>
      )}
      {stale && <div className="app-quota-stale">{stale}</div>}
    </>
  );
}

