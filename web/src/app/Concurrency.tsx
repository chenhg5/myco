import { useCallback, useEffect, useState } from "react";
import { Minus, Plus } from "lucide-react";
import type { Machine, MachineConcurrency, RunnerSlots } from "@/api";
import { ApiError, appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { ErrorText, Modal } from "@/app/dialogs";
import { thisComputerId } from "@/app/nodes";
import { L, E } from "@/lib/i18n";

/*
 * How many agent CLIs a computer runs at once (its runner, shared between
 * all its workspaces): more tasks wait their turn. This computer's is
 * written here (machine.yaml, read by its runner within seconds); another
 * computer's is asked of, and changed through, its machine node, by its
 * owner only. Raising it starts waiting tasks at once; lowering it lets
 * running ones finish.
 */

/** You own this computer (its owner may change its limit). Without a member
 *  (the workspace's own token) you are its owner. */
export function ownsMachine(hub: AgentRow["hub"], m: Machine): boolean {
  const me = hub.me?.member;
  if (!me) return true;
  return m.owner_id ? m.owner_id === me.id : m.owner ? m.owner === me.name : me.role === "owner";
}

/** The limit in force (runners up to v0.23.9 say only max_concurrent). */
export const slotLimit = (s: RunnerSlots) => s.limit || s.max_concurrent;

type State = {
  view: MachineConcurrency | null;
  /** this computer: its runner does not follow the setting yet (none
   *  running, or one older than it) */
  notLive?: boolean;
  setting?: number;
  ceiling: number;
  /** that computer's myco cannot say or change it */
  tooOld?: boolean;
};

function useConcurrency(hubId: string, machineId: string, here: boolean) {
  const [state, setState] = useState<State | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(async () => {
    try {
      if (here) {
        const c = await appApi.localConcurrency();
        setState({ view: c.runner, notLive: !c.runner, setting: c.setting, ceiling: c.ceiling });
      } else {
        setState({ view: await appApi.machineConcurrency(hubId, machineId), ceiling: 64 });
      }
      setError(null);
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.code === "too_old") setState({ view: null, tooOld: true, ceiling: 64 });
      else setError(e);
      return false;
    }
  }, [hubId, machineId, here]);
  useEffect(() => { void load(); }, [load]);
  return { state, setState, error, setError, load };
}

/** The limit, how many run and wait, and (for its owner) a way to change it. */
export function ConcurrencyLimit({ hubId, machine, here, owns, slots, onChanged, titled = true }: {
  hubId: string; machine: Machine; here: boolean; owns: boolean;
  /** with its own title (a dialog that has one says it already: false) */
  titled?: boolean;
  /** what its agents last reported (shown until the computer answers) */
  slots?: RunnerSlots;
  onChanged?: () => void;
}) {
  const { state, setState, error, setError, load } = useConcurrency(hubId, machine.id, here);
  const view = state?.view;
  const limit = view?.limit ?? (slots ? slotLimit(slots) : undefined);
  const [draft, setDraft] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);
  const [waitingFor, setWaitingFor] = useState<number | null>(null);
  const value = draft ?? limit ?? state?.setting ?? 10;
  const ceiling = state?.ceiling || 64;

  // This computer: until its runner says it took the new number.
  useEffect(() => {
    if (waitingFor === null) return;
    let n = 0;
    const t = window.setInterval(async () => {
      n++;
      const c = await appApi.localConcurrency().catch(() => null);
      if (c) setState({ view: c.runner, notLive: !c.runner, setting: c.setting, ceiling: c.ceiling });
      if ((c?.runner && c.runner.limit === waitingFor) || n >= 8) {
        window.clearInterval(t);
        setWaitingFor(null);
      }
    }, 1500);
    return () => window.clearInterval(t);
  }, [waitingFor, setState]);

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      if (here) {
        const c = await appApi.setLocalConcurrency(value);
        setState({ view: c.runner, notLive: !c.runner, setting: c.setting, ceiling: c.ceiling });
        if (c.runner) setWaitingFor(value);
      } else {
        setState({ view: await appApi.setMachineConcurrency(hubId, machine.id, value), ceiling });
      }
      setDraft(null);
      onChanged?.();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  const inUse = view?.in_use ?? slots?.in_use;
  const waiting = view?.waiting ?? slots?.waiting ?? 0;
  const changed = draft !== null && draft !== limit;
  return (
    <div className="cc" data-testid="concurrency">
      <div className="cc-top">
        <div className="cc-text">
          {titled && <b>{L("同时干活", "At once")}</b>}
          <span>{limit === undefined ? L("正在问它…", "Asking it…")
            : L(E`最多 ${limit} 个 agent 同时干活${inUse !== undefined ? `，现在 ${inUse} 个在跑` : ""}${waiting ? `，${waiting} 个在排队` : ""}`,
              E`Up to ${limit} agents at once${inUse !== undefined ? `; ${inUse} running` : ""}${waiting ? `, ${waiting} waiting` : ""}`)}</span>
        </div>
        {owns && !state?.tooOld && limit !== undefined && (
          <div className="cc-edit">
            <div className="cc-step" role="group" aria-label={L("上限", "Limit")}>
              <button type="button" className="app-btn app-btn-small cc-step-btn" aria-label={L("少一个", "One fewer")} disabled={value <= 1 || saving}
                onClick={() => setDraft(Math.max(1, value - 1))}><Minus /></button>
              <input className="app-input cc-step-input" type="number" min={1} max={ceiling} value={value} disabled={saving} data-testid="concurrency-input"
                aria-label={L("同时最多几个", "How many at once")}
                onChange={(e) => { const n = Math.round(Number(e.target.value)); if (Number.isFinite(n)) setDraft(Math.min(ceiling, Math.max(1, n))); }} />
              <button type="button" className="app-btn app-btn-small cc-step-btn" aria-label={L("多一个", "One more")} disabled={value >= ceiling || saving}
                onClick={() => setDraft(Math.min(ceiling, value + 1))}><Plus /></button>
            </div>
            {changed && (
              <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={saving} onClick={() => void save()} data-testid="concurrency-save">
                {saving ? L("正在改…", "Saving…") : L(E`改成 ${value} 个`, E`Set to ${value}`)}
              </button>
            )}
          </div>
        )}
      </div>
      {state?.tooOld && (
        <p className="app-small-note cc-note" data-testid="concurrency-too-old">{L("那台电脑上的 myco 版本较旧，在这里看不到也改不了。在那台电脑上升级 myco 后就可以了。", "That computer's myco is too old to show or change this here. Upgrade myco on it.")}</p>
      )}
      {!owns && limit !== undefined && (
        <p className="app-small-note cc-note">{L("只有这台电脑的主人能改。", "Only this computer's owner can change it.")}</p>
      )}
      {waitingFor !== null && <p className="app-small-note cc-note">{L("几秒内生效，正在跑的任务不受影响…", "Takes effect within seconds; running tasks are not interrupted…")}</p>}
      {here && state?.notLive && (state.setting || 0) > 0 && (
        <p className="app-small-note cc-note" data-testid="concurrency-not-live">{L(E`已经记下最多 ${state.setting} 个。这台电脑的 runner 还没用上它（没在跑，或者版本较旧）：重启 runner 后生效。`,
          E`Saved: up to ${state.setting}. This computer's runner does not use it yet (not running, or older): restart the runner.`)}
          {" "}<button type="button" className="app-btn app-btn-small cc-restart" onClick={() => void appApi.restartRunner().then(() => window.setTimeout(() => void load(), 3000)).catch(setError)}>{L("重启 runner", "Restart the runner")}</button></p>
      )}
      <ErrorText error={error} />
    </div>
  );
}

/** "Raise the limit", where a task waits only because the computer is full:
 *  the same control in a small dialog (this computer's through its own
 *  settings, another's through its machine node). */
export function RaiseLimit({ hub, machine, slots }: { hub: AgentRow["hub"]; machine: Machine; slots?: RunnerSlots }) {
  const [open, setOpen] = useState(false);
  const [here, setHere] = useState<boolean | null>(null);
  useEffect(() => {
    if (!open || here !== null) return;
    appApi.local().then((l) => setHere(thisComputerId(hub, l) === machine.id)).catch(() => setHere(false));
  }, [open, here, hub, machine.id]);
  return (
    <>
      <button type="button" className="app-btn app-btn-small trace-wait-act" onClick={() => setOpen(true)} data-testid="raise-limit">{L("调高上限", "Raise the limit")}</button>
      <Modal open={open} onOpenChange={setOpen} title={L(E`${machine.name} 同时干活的上限`, E`How many run at once on ${machine.name}`)}
        description={L("多出来的任务会排队，轮到了才开始。调高后排队的任务马上开始，正在跑的不受影响。", "Tasks beyond it wait their turn. Raising it starts waiting ones at once; running ones are not affected.")}>
        {here === null ? <p className="app-small-note">{L("正在问它…", "Asking it…")}</p>
          : <ConcurrencyLimit hubId={hub.id} machine={machine} here={here} owns slots={slots} titled={false} />}
      </Modal>
    </>
  );
}
