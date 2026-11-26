import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, Pause, Play, Target, Timer, X } from "lucide-react";
import { appApi, errorMessage } from "@/appapi";
import type { Goal } from "@/api";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ConfirmDialog, Modal } from "@/app/dialogs";
import { useContextMenu } from "@/app/ContextMenu";

/*
 * 目标 (myco goal): a goal set on a chat. The hub wakes the agent on a
 * schedule, while it is idle, to push the goal forward, until it is done,
 * given up, or out of time or rounds. Its chat shows it at the top, in the
 * watch bar's style: "目标：… · 第 3/16 轮 · 下次唤醒 14:30（还有 12 分钟）·
 * 已进行 1 小时 20 分", with 暂停/继续, 改间隔 and 停止; opened, what done
 * means, the last round's summary and next step, and every round. When it
 * ends a grey line stays where it ended.
 */

const POLL_MS = 5000;
const LIVE = new Set(["active", "paused"]);

/** One agent's goals (at work and ended), read every few seconds while its
 *  chat is open; the one at work with its rounds. Older hubs: nothing. */
export function useGoals(hubId: string, agent: string) {
  const [list, setList] = useState<Goal[]>([]);
  const [detail, setDetail] = useState<Goal | null>(null);
  const [none, setNone] = useState(false);
  const live = useRef(true);
  const load = useCallback(async () => {
    try {
      const r = await appApi.goals(hubId, { agent, state: "all" });
      if (!live.current) return;
      const goals = r.goals || [];
      setList(goals);
      const cur = goals.find((g) => LIVE.has(g.state));
      if (cur) {
        const d = await appApi.goal(hubId, cur.id).catch(() => null);
        if (live.current) setDetail(d || cur);
      } else setDetail(null);
    } catch (e) {
      if ((e as { code?: string })?.code === "not_found" || (e as { status?: number })?.status === 404) setNone(true);
    }
  }, [hubId, agent]);
  useEffect(() => {
    live.current = true;
    setList([]); setDetail(null); setNone(false);
    void load();
    return () => { live.current = false; };
  }, [load]);
  useEffect(() => {
    if (none) return;
    const t = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(t);
  }, [load, none]);
  /** one changed here: shown at once, read again behind */
  const put = (g: Goal) => {
    setList((xs) => (xs.some((x) => x.id === g.id) ? xs.map((x) => (x.id === g.id ? { ...x, ...g } : x)) : [...xs, g]));
    setDetail((d) => (LIVE.has(g.state) ? { ...(d && d.id === g.id ? d : {}), ...g } as Goal : null));
    void load();
  };
  return { supported: !none, current: detail, ended: list.filter((g) => !LIVE.has(g.state) && g.ended_at), put };
}

function useNow(on: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!on) return;
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [on]);
  return now;
}

function span(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return L(`${s} 秒`, `${s}s`);
  const m = Math.floor(s / 60);
  if (m < 60) return L(`${m} 分钟`, `${m} min`);
  const h = Math.floor(m / 60);
  return m % 60 ? L(`${h} 小时 ${m % 60} 分`, `${h} h ${m % 60} min`) : L(`${h} 小时`, `${h} h`);
}
const clock = (iso?: string) => {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
};
const when = (iso?: string) => {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString(undefined, { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
};
const short = (s: string, n = 28) => ([...s.trim()].length > n ? `${[...s.trim()].slice(0, n).join("")}…` : s.trim());

const EVERY: [number, string, string][] = [[15 * 60, "15 分钟", "15 minutes"], [30 * 60, "30 分钟", "30 minutes"], [60 * 60, "1 小时", "1 hour"]];

/** The goal at work, at the top of the chat. */
export function GoalBar({ hubId, goal, onChanged }: { hubId: string; goal: Goal | null; onChanged: (g: Goal) => void }) {
  const now = useNow(!!goal);
  const [open, setOpen] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const menu = useContextMenu();
  if (!goal) return null;
  const run = async (f: () => Promise<Goal>) => {
    setBusy(true); setError("");
    try { onChanged(await f()); return true; } catch (e) { setError(errorMessage(e)); return false; } finally { setBusy(false); }
  };
  const paused = goal.state === "paused";
  const backedOff = goal.every_sec > goal.base_every_sec;
  const next = goal.next_wake_at ? Date.parse(goal.next_wake_at) : NaN;
  const parts = [
    L(`第 ${goal.round}/${goal.max_rounds} 轮`, `Round ${goal.round} of ${goal.max_rounds}`),
    paused
      ? L("已暂停", "Paused")
      : !Number.isNaN(next)
        ? next > now
          ? L(`下次唤醒 ${clock(goal.next_wake_at)}（还有 ${span(next - now)}）`, `next ${clock(goal.next_wake_at)} (in ${span(next - now)})`)
          : L("等它空闲就唤醒", "wakes once it is idle")
        : "",
    L(`已进行 ${span(now - Date.parse(goal.started_at))}`, `${span(now - Date.parse(goal.started_at))} so far`),
  ].filter(Boolean);
  return (
    <div className={cn("watch-bar goal-bar", open && "is-open", paused && "is-paused")} data-testid="goal-bar">
      <div className="watch-line">
        <button type="button" className="watch-toggle" onClick={() => setOpen((o) => !o)} aria-expanded={open}
          title={open ? L("收起", "Fold") : L("看详细", "Details")}>
          <Target className="watch-eye goal-icon" />
          <span className="watch-what">{L(`目标：${short(goal.objective)}`, `Goal: ${short(goal.objective)}`)}</span>
          <span className="watch-time">{parts.join(" · ")}</span>
          <ChevronDown className="watch-chev" />
        </button>
        <span className="watch-actions">
          <button type="button" className="watch-btn" disabled={busy}
            title={paused ? L("继续", "Resume") : L("暂停", "Pause")} aria-label={paused ? L("继续", "Resume") : L("暂停", "Pause")}
            onClick={() => void run(() => appApi.goalAction(hubId, goal.id, paused ? "resume" : "pause"))}>
            {paused ? <Play /> : <Pause />}<span className="watch-btn-label">{paused ? L("继续", "Resume") : L("暂停", "Pause")}</span>
          </button>
          <button type="button" className="watch-btn" disabled={busy} title={L("改间隔", "Interval")} aria-label={L("改间隔", "Interval")}
            onClick={(e) => menu.openUnder(e.currentTarget, [{ header: L("多久唤醒一次", "Wake it every") },
              ...EVERY.map(([sec, zh, en]) => ({ label: L(zh, en), checked: goal.base_every_sec === sec, onSelect: () => void run(() => appApi.patchGoal(hubId, goal.id, { every_sec: sec })) }))])}>
            <Timer /><span className="watch-btn-label">{L("改间隔", "Interval")}</span>
          </button>
          <button type="button" className="watch-btn" disabled={busy} onClick={() => setStopping(true)} title={L("停止", "Stop")} aria-label={L("停止", "Stop")}>
            <X /><span className="watch-btn-label">{L("停止", "Stop")}</span>
          </button>
        </span>
      </div>
      {(paused || backedOff) && (
        <div className={cn("goal-note", paused ? "is-paused" : "is-backoff")}>
          {paused
            ? L(`暂停了，等你决定：${goal.outcome || "连续几轮没有进展"}`, `Paused, waiting for you: ${goal.outcome || "several rounds without progress"}`)
            : L(`连续 ${goal.no_progress} 轮没有进展，间隔放宽到 ${span(goal.every_sec * 1000)}`, `${goal.no_progress} rounds without progress: now every ${span(goal.every_sec * 1000)}`)}
        </div>
      )}
      {open && (
        <div className="goal-detail">
          <dl className="goal-facts">
            <dt>{L("目标", "Goal")}</dt><dd>{goal.objective}</dd>
            <dt>{L("算达成", "Done when")}</dt><dd>{goal.done_when}</dd>
            {goal.last_summary && <><dt>{L("上一轮", "Last round")}</dt><dd>{goal.last_summary}</dd></>}
            {goal.next_step && <><dt>{L("下一步", "Next")}</dt><dd>{goal.next_step}</dd></>}
            <dt>{L("期限", "Until")}</dt><dd>{L(`${when(goal.deadline)}，最多 ${goal.max_rounds} 轮，每 ${span(goal.base_every_sec * 1000)}`, `${when(goal.deadline)}, at most ${goal.max_rounds} rounds, every ${span(goal.base_every_sec * 1000)}`)}</dd>
          </dl>
          {!!goal.rounds?.length && (
            <ol className="goal-rounds">
              {[...goal.rounds].reverse().map((r) => (
                <li key={r.round} className={cn("goal-round", !r.progressed && "is-stuck", r.blocked && "is-blocked")}>
                  <span className="goal-round-n">{L(`第 ${r.round} 轮`, `Round ${r.round}`)}</span>
                  <span className="goal-round-time">{clock(r.started_at)}</span>
                  <span className="goal-round-tag">{r.blocked ? L("遇阻", "Blocked") : r.progressed ? L("有进展", "Progress") : L("没进展", "No progress")}</span>
                  <span className="goal-round-text">{r.summary}</span>
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
      {error && <div className="app-error-text watch-error">{error}</div>}
      {menu.element}
      <ConfirmDialog open={stopping} onOpenChange={(o) => { if (!o) { setStopping(false); setError(""); } }}
        title={L("停止这个目标？", "Stop this goal?")}
        description={L(`「${short(goal.objective, 60)}」。停了以后不再按时唤醒它，做到哪算哪，它会收到一条消息。`, `“${short(goal.objective, 60)}”. It is no longer woken on schedule; what is done stays done, and it gets a message.`)}
        confirmLabel={L("停止", "Stop")} busy={busy} error={stopping ? error : undefined}
        onConfirm={() => void run(() => appApi.goalAction(hubId, goal.id, "stop")).then((ok) => { if (ok) setStopping(false); })} />
    </div>
  );
}

/** 设定目标…: what, when it counts as done, how often, how long. */
export function GoalDialog({ open, onOpenChange, hubId, agent, name, project, onStarted }: {
  open: boolean; onOpenChange: (o: boolean) => void;
  hubId: string; agent: string; name: string; project?: string;
  onStarted: (g: Goal) => void;
}) {
  const [objective, setObjective] = useState("");
  const [doneWhen, setDoneWhen] = useState("");
  const [every, setEvery] = useState(30 * 60);
  const [hours, setHours] = useState(8);
  const [rounds, setRounds] = useState(16);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { if (open) { setError(""); } }, [open]);
  const ok = objective.trim() && doneWhen.trim() && hours > 0 && rounds > 0;
  const start = async () => {
    setBusy(true); setError("");
    try {
      const g = await appApi.startGoal(hubId, { agent, objective: objective.trim(), done_when: doneWhen.trim(), every_sec: every, max_rounds: rounds, for_sec: Math.round(hours * 3600), ...(project ? { project } : {}) });
      onStarted(g);
      setObjective(""); setDoneWhen("");
      onOpenChange(false);
    } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  };
  return (
    <Modal open={open} onOpenChange={onOpenChange} title={L("设定目标", "Set a goal")} className="goal-dialog"
      description={L(`${name} 空闲时会被按时唤醒，围绕这个目标一轮一轮往前推，直到做到、做不到或到期。重大动作仍会先问你。`,
        `${name} is woken on schedule while idle to push this goal forward, round by round, until it is done, cannot be done, or time is up. Big steps still ask you first.`)}>
      <label className="app-field">
        <span className="app-field-label">{L("目标", "Goal")}</span>
        <textarea className="app-input goal-textarea" rows={3} value={objective} onChange={(e) => setObjective(e.target.value)}
          placeholder={L("比如：把 iOS 端的单测覆盖率提到 80%", "e.g. Get the iOS unit test coverage to 80%")} />
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("怎样算达成", "Done when")}</span>
        <textarea className="app-input goal-textarea" rows={2} value={doneWhen} onChange={(e) => setDoneWhen(e.target.value)}
          placeholder={L("比如：CI 的覆盖率报告 ≥ 80%，并且全部测试通过", "e.g. The CI coverage report shows ≥ 80% and every test passes")} />
      </label>
      <div className="app-field">
        <span className="app-field-label">{L("多久唤醒一次", "Wake it every")}</span>
        <div className="seg" role="tablist" aria-label={L("多久唤醒一次", "Wake it every")}>
          {EVERY.map(([sec, zh, en]) => (
            <button key={sec} type="button" role="tab" aria-selected={every === sec} className={cn("seg-btn", every === sec && "active")} onClick={() => setEvery(sec)}>{L(zh, en)}</button>
          ))}
        </div>
      </div>
      <div className="goal-limits">
        <label className="app-field">
          <span className="app-field-label">{L("最长（小时，最多 8）", "At most (hours, up to 8)")}</span>
          <input className="app-input" type="number" min={1} max={8} value={hours} onChange={(e) => setHours(Math.min(8, Number(e.target.value) || 0))} />
        </label>
        <label className="app-field">
          <span className="app-field-label">{L("最多几轮", "At most (rounds)")}</span>
          <input className="app-input" type="number" min={1} max={256} value={rounds} onChange={(e) => setRounds(Math.min(256, Number(e.target.value) || 0))} />
        </label>
      </div>
      <p className="goal-hint">{L("连续 2 轮没进展，间隔会自动放宽；连续 4 轮没进展或遇阻，会暂停来问你。", "After 2 rounds without progress it waits longer; after 4, or when blocked, it pauses and asks you.")}</p>
      {error && <div className="app-error-text">{error}</div>}
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => onOpenChange(false)} disabled={busy}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={!ok || busy} onClick={() => void start()}>
          {busy ? <span className="app-spinner" /> : null}{L("开始", "Start")}
        </button>
      </div>
    </Modal>
  );
}

/** How a goal ended, a grey line in the chat (a click shows the summary). */
export function GoalEnded({ g, nameOf }: { g: Goal; nameOf: (n: string) => string }) {
  const [open, setOpen] = useState(false);
  const what = short(g.objective, 40);
  const text = g.state === "achieved" ? L(`目标达成：${what}`, `Goal achieved: ${what}`)
    : g.state === "abandoned" ? L(`目标放弃了：${what}`, `Goal given up: ${what}`)
    : g.outcome === "round_limit" || (g.state === "expired" && g.round >= g.max_rounds) ? L(`目标轮次用完了：${what}`, `Goal out of rounds: ${what}`)
    : g.state === "expired" ? L(`目标到期了：${what}`, `Goal out of time: ${what}`)
    : L(`${g.ended_by ? nameOf(g.ended_by) : "有人"}停止了目标：${what}`, `${g.ended_by ? nameOf(g.ended_by) : "Someone"} stopped the goal: ${what}`);
  return (
    <div className={cn("goal-ended", `is-${g.state}`, open && "is-open")} data-goal={g.id}>
      <button type="button" className="goal-ended-line" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <Target />
        <span>{text}{L(` · ${g.round} 轮`, ` · ${g.round} rounds`)}</span>
        <span className="watch-ended-time">{when(g.ended_at)}</span>
      </button>
      {open && (
        <div className="goal-ended-detail">
          {g.outcome && g.outcome !== "round_limit" && <p>{g.outcome}</p>}
          {g.last_summary && <p className="goal-ended-summary">{L("最后一轮：", "Last round: ")}{g.last_summary}</p>}
        </div>
      )}
    </div>
  );
}
