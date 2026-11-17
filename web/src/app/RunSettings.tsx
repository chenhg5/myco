import { useState } from "react";
import { appApi, isOldHub } from "@/appapi";
import type { RuntimeAccess } from "@/api";
import type { AgentRow } from "@/app/types";
import { accessLabel, agentKind, canManage, isCloud, isInbox, machineOfRow, modelSourceLabel } from "@/app/nodes";
import { ChevronDown } from "lucide-react";
import { hubAfter0194 } from "@/app/hubVersion";
import { ConfirmDialog, ErrorText } from "@/app/dialogs";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { quotaSpent, quotaStaleNote, quotaTight, quotaWindowLabel, quotaWindowText } from "@/app/quota";
import { cn } from "@/lib/utils";
import { L, lang } from "@/lib/i18n";

/** The models a node may be told to use: aliases for Claude Code, and the
 *  names Codex is commonly started with. */
const MODELS: Record<string, string[]> = {
  claude: ["fable", "opus", "sonnet", "haiku"],
  codex: ["gpt-6-sol", "gpt-6", "gpt-5-codex", "o3"],
};
const EFFORTS: Record<string, string[]> = {
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

function effortLabel(effort: string): string {
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
export function RunRows({ row, now, mayManage, onRefresh }: {
  row: AgentRow;
  now: number;
  mayManage: boolean;
  onRefresh: () => Promise<void>;
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
  const cap = ACCESS[Math.max(ACCESS.indexOf(rt?.max_access as RuntimeAccess), ACCESS.indexOf(rt?.access as RuntimeAccess), 0)];
  const access = ag.run_settings?.access || "";
  const model = ag.run_settings?.model || "";
  const effort = ag.run_settings?.effort || "";
  const save = async (next: { model: string; effort: string; access?: RuntimeAccess | "" }) => {
    setBusy(true);
    setError(null);
    try {
      await appApi.setRunSettings(row.hub.id, ag.name, next);
      await onRefresh();
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
          {accessEditable && <>{L("现在是：", "In use: ")}<SourceValue value={accessLabel(rt?.access)} sourceLabel={access ? "App" : undefined} />
            {L(" · 下次唤醒时生效。", " · takes effect the next time it wakes.")}</>}
          {!rt?.max_access && L("升级那台电脑上的 myco 后才能在这里改。", "Upgrade myco on that computer to change this here.")}
          {rt?.max_access && cap !== "full" && <div>{L(
            `这台电脑最多允许「${accessLabel(cap)}」。要更高，在那台电脑的 ~/.myco/machine.yaml 里设 spawn.max_access。`,
            `This computer allows at most ${accessLabel(cap)}. For more access, set spawn.max_access in ~/.myco/machine.yaml on that computer.`,
          )}</div>}
          {accessEditable && cap !== "full" && <LocalShellAccess machine={machineOfRow(row)} what="full" onlyLocal onChange={onRefresh} />}
        </div>
      )}
    </div>
  );

  const models = [...(MODELS[cli] || [])];
  for (const v of [rt?.model, model]) if (v && !models.includes(v)) models.unshift(v);
  const efforts = [...(EFFORTS[cli] || [])];
  for (const v of [rt?.effort, effort]) if (v && !efforts.includes(v)) efforts.unshift(v);

  // An agent that receives in its own session runs its CLI itself: myco can
  // neither read nor set its model and quota.
  if (isInbox(ag)) {
    return (
      <><div className="app-kv has-help">
        <span className="app-kv-label">{L("模型", "Model")}</span>
        <span className="app-kv-value">
          {rt?.model_source === "self"
            ? <SourceValue value={rt.model} source="self" />
            : "—"}
        </span>
        <span className="app-kv-help">
          {L("模型和额度由它自己的会话管理，myco 拿不到也改不了。", "Its own session manages the model and quota; myco can't see or change them.")}
        </span>
      </div>{accessRow}</>
    );
  }

  const quota = rt?.quota;
  const stale = quota ? quotaStaleNote(quota, now) : undefined;

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
                {models.map((m) => <option key={m} value={m}>{m}</option>)}
                <option value="__custom__">{L("自定义…", "Custom…")}</option>
              </select>
            )
          ) : <SourceValue value={rt?.model} source={rt?.model_source} />}
        </span>
        {editable && (
          <span className="app-kv-help">
            {L("现在是：", "In use: ")}<SourceValue value={rt?.model} source={rt?.model_source} />
            {L(" · 下次唤醒时生效。", " · takes effect the next time it wakes.")}
          </span>
        )}
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
          {editable && (
            <span className="app-kv-help">
              {L("现在是：", "In use: ")}{rt?.effort ? effortLabel(rt.effort) : "—"}
              {rt?.effort && modelSourceLabel(rt.effort_source) ? L(`（${modelSourceLabel(rt.effort_source)}）`, ` (${modelSourceLabel(rt.effort_source)})`) : ""}
              {L(" · 下次唤醒时生效。", " · takes effect the next time it wakes.")}
            </span>
          )}
        </div>
      )}

      {accessRow}

      {quota?.windows?.map((w) => (
        <div key={w.name} className={cn("app-kv app-quota", quotaSpent(w) ? "is-over" : quotaTight(w) && "is-tight")}>
          <span className="app-kv-label">{quotaWindowLabel(w)}</span>
          <span className="app-kv-value">{quotaWindowText(w, now)}</span>
        </div>
      ))}
      {stale && <div className="app-quota-stale">{stale}</div>}
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
  const setModel = ag.run_settings?.model || "";
  const setEffort = ag.run_settings?.effort || "";
  const model = setModel || rt?.model || "";
  const effort = setEffort || rt?.effort || "";
  // In its own session and not said yet: shown as unknown, and how it comes to be known.
  const unknown = !model && !editable && (isInbox(ag) || isCloud(ag));
  if (agentKind(ag) !== "agent" || (!model && !editable && !unknown)) return null;
  const label = unknown ? L("模型未知", "Model unknown")
    : [modelShort(model) || L("默认模型", "Default model"), effort ? effortLabel(effort) : ""].filter(Boolean).join(" · ");
  const save = async (next: { model: string; effort: string }) => {
    setBusy(true); setError(null);
    try {
      await appApi.setRunSettings(row.hub.id, ag.name, next);
      await onRefresh();
      setOpen(false);
    } catch (e) {
      if (isOldHub(e)) setOld(true); else setError(e);
    } finally { setBusy(false); }
  };
  const models = [...(MODELS[cli] || [])];
  for (const v of [rt?.model, setModel]) if (v && !models.includes(v)) models.unshift(v);
  const efforts = [...(EFFORTS[cli] || [])];
  for (const v of [rt?.effort, setEffort]) if (v && !efforts.includes(v)) efforts.unshift(v);
  const pending = editable && ((setModel && setModel !== rt?.model) || (setEffort && setEffort !== rt?.effort));
  const title = editable
    ? L(`模型和推理强度：${label}（点一下切换，下次唤醒时生效）`, `Model and effort: ${label} (click to switch; from its next run)`)
    : unknown ? L("它还没说自己用的哪个模型", "It has not said which model it runs on")
      : L(`模型和推理强度：${label}（它自己的会话决定）`, `Model and effort: ${label} (its own session decides)`);
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
          <div className="run-pill-menu" role="menu" onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}>
            {editable ? (
              <>
                <div className="run-pill-group">{L("模型", "Model")}</div>
                {["", ...models].map((m) => (
                  <button key={`m:${m}`} type="button" role="menuitemradio" aria-checked={setModel === m} className={cn("run-pill-item", setModel === m && "is-on")}
                    disabled={busy} onClick={() => void save({ model: m, effort: setEffort })}>
                    <span>{m ? modelShort(m) : L("跟随电脑设置", "Follow this computer")}</span>
                    {m && m !== modelShort(m) && <span className="run-pill-sub">{m}</span>}
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
                  {L("现在是：", "In use: ")}{modelShort(rt?.model) || "—"}{rt?.effort ? ` · ${effortLabel(rt.effort)}` : ""}
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
