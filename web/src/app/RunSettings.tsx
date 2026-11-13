import { useState } from "react";
import { appApi, isOldHub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, isCloud, isInbox, modelSourceLabel } from "@/app/nodes";
import { hubAfter0194 } from "@/app/hubVersion";
import { ErrorText } from "@/app/dialogs";
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
function SourceValue({ value, source }: { value?: string; source?: Parameters<typeof modelSourceLabel>[0] }) {
  const label = modelSourceLabel(source);
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
  const [custom, setCustom] = useState(false);
  const [draft, setDraft] = useState("");
  const editable = mayManage && tunable && !old && hubAfter0194(row.hub.hub_version);

  const model = ag.run_settings?.model || "";
  const effort = ag.run_settings?.effort || "";
  const save = async (next: { model: string; effort: string }) => {
    setBusy(true);
    setError(null);
    try {
      await appApi.setRunSettings(row.hub.id, ag.name, next);
      await onRefresh();
    } catch (e) {
      if (isOldHub(e)) setOld(true);
      else setError(e);
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

  const models = [...(MODELS[cli] || [])];
  for (const v of [rt?.model, model]) if (v && !models.includes(v)) models.unshift(v);
  const efforts = [...(EFFORTS[cli] || [])];
  for (const v of [rt?.effort, effort]) if (v && !efforts.includes(v)) efforts.unshift(v);

  // An agent that receives in its own session runs its CLI itself: myco can
  // neither read nor set its model and quota.
  if (isInbox(ag)) {
    return (
      <div className="app-kv has-help">
        <span className="app-kv-label">{L("模型", "Model")}</span>
        <span className="app-kv-value">
          {rt?.model_source === "self"
            ? <SourceValue value={rt.model} source="self" />
            : "—"}
        </span>
        <span className="app-kv-help">
          {L("模型和额度由它自己的会话管理，myco 拿不到也改不了。", "Its own session manages the model and quota; myco can't see or change them.")}
        </span>
      </div>
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

      {quota?.windows?.map((w) => (
        <div key={w.name} className={cn("app-kv app-quota", quotaSpent(w) ? "is-over" : quotaTight(w) && "is-tight")}>
          <span className="app-kv-label">{quotaWindowLabel(w)}</span>
          <span className="app-kv-value">{quotaWindowText(w, now)}</span>
        </div>
      ))}
      {stale && <div className="app-quota-stale">{stale}</div>}
      <ErrorText error={error} />
    </>
  );
}
