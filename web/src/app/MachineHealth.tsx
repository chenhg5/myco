import { useEffect, useRef, useState } from "react";
import type { Machine, MachineProcess } from "@/api";
import { appApi } from "@/appapi";
import { ConfirmDialog, ErrorText } from "@/app/dialogs";
import { L } from "@/lib/i18n";

export function healthState(machine: Machine, now = Date.now()): "unknown" | "healthy" | "alert" {
  const h = machine.machine_health;
  if (!machine.online || !h?.complete || !Number.isFinite(Date.parse(h.at)) || now - Date.parse(h.at) > 120_000 || Date.parse(h.at) > now + 30_000) return "unknown";
  return h.alert ? "alert" : "healthy";
}

/** Only a fresh server-authorized process reference enables a destructive action. */
export function MachineHealthPanel({ hubId, machine, onChanged }: { hubId: string; machine: Machine; onChanged: () => void }) {
  const viewKey = `${hubId}/${machine.id}`;
  const currentKey = useRef(viewKey); currentKey.current = viewKey;
  const generation = useRef(0);
  const [now, setNow] = useState(Date.now);
  const [selected, setSelected] = useState<MachineProcess | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [ended, setEnded] = useState<string[]>([]);
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 10_000); return () => clearInterval(timer); }, []);
  useEffect(() => { generation.current++; setSelected(null); setError(null); setEnded([]); setBusy(false); return () => { generation.current++; }; }, [hubId, machine.id]);
  const h = machine.machine_health;
  const state = healthState(machine, now);
  const canEnd = (p: MachineProcess) => !!(state !== "unknown" && machine.can_stop_process && p.agent && p.suspicious && p.ref && !ended.includes(p.ref)
    && h?.processes?.some((current) => current.ref === p.ref && current.pid === p.pid && current.created === p.created && current.agent === p.agent && current.suspicious));
  useEffect(() => {
    if (selected && !busy && !canEnd(selected)) { setSelected(null); setError(new Error(L("观测已变化或过期，请查看最新列表。", "The observation changed or expired. Check the latest list."))); }
  }, [now, machine.machine_health, machine.can_stop_process, selected, busy]);
  const stop = async () => {
    if (!selected || !canEnd(selected) || !selected.ref || busy) return;
    const started = generation.current;
    const stillHere = () => generation.current === started && currentKey.current === viewKey;
    setBusy(true); setError(null);
    try { await appApi.stopMachineProcess(hubId, machine.id, selected.ref); if (stillHere()) { setEnded((old) => [...old, selected.ref!]); setSelected(null); onChanged(); } }
    catch (e) { if (stillHere()) setError(e); }
    finally { if (stillHere()) setBusy(false); }
  };
  if (!h) return null;
  return <section className="app-list machine-health" data-testid="machine-health">
    <div className="app-group-title"><span className={`dot ${state === "alert" ? "pending" : state === "unknown" ? "offline" : "idle"}`} /> {L("机器健康", "Machine health")} · {state === "unknown" ? L("采样不完整或已过期", "Sample incomplete or stale") : state === "alert" ? L("需要关注", "Needs attention") : L("正常", "Healthy")}</div>
    <div className="app-kv"><span>{h.load_metric === "load1_per_core" ? L("负载 / 核数", "Load / cores") : "CPU"}</span><span>{state === "unknown" ? "—" : `${(h.load_ratio * 100).toFixed(0)}%`} · {h.cores} {L("核", "cores")}</span></div>
    <div className="app-kv"><span>{L("可用内存", "Available memory")}</span><span>{state === "unknown" || !h.memory_total ? "—" : `${(100 * h.memory_available / h.memory_total).toFixed(0)}%`}</span></div>
    {(h.processes || []).map((p) => <div className="app-row" key={`${p.pid}/${p.created}`}>
      <div className="machine-health-process"><b>{p.name}</b> <span className="mono">PID {p.pid}</span><div>{p.agent || L("来源未知", "Unknown source")} · CPU {state === "unknown" ? "—" : `${p.cpu.toFixed(0)}%`}{p.suspicious ? ` · ${L("可疑孤儿进程", "Suspected orphan")}` : ""}</div></div>
      {canEnd(p) ? <button type="button" className="app-btn app-btn-small app-btn-danger" onClick={() => { setSelected(p); setError(null); }}>{L("结束", "End")}</button> : null}
    </div>)}
    <p className="app-note">{L("只告警，不自动结束进程。来源未知的进程计入整机负载，但不能在这里结束。", "Alerts never stop processes automatically. Unknown processes count toward machine load but cannot be ended here.")}</p>
    {error && !selected ? <ErrorText error={error} /> : null}
    <ConfirmDialog open={!!selected} onOpenChange={(open) => { if (!open && !busy) setSelected(null); }} title={L("结束这个进程？", "End this process?")} subject={selected && <span>{selected.name} · PID {selected.pid}</span>}
      description={L("会再次核对进程归属；这个操作可能中断后台工作。", "Process ownership is checked again. This may interrupt background work.")}
      confirmLabel={L("结束进程", "End process")} onConfirm={() => void stop()} busy={busy} error={error ? <ErrorText error={error} /> : undefined} />
  </section>;
}
