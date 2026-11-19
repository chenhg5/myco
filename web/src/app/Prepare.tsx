import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Check } from "lucide-react";
import type { Agent } from "@/api";
import { appApi, type LocalSession } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { LocalAgentLines } from "@/app/LocalAgents";
import { Avatar } from "@/app/persona";
import { Logo } from "@/components/brand";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * 准备这台电脑: right after a workspace starts on a new install (and from
 * Settings › About later). What this computer has: the agent CLIs, ready
 * or not, with a button that installs or signs in; its recent Claude Code
 * and Codex sessions, the recently active ones ticked, which join as
 * agents (copies: the originals stay yours). Then in: the chat with the
 * first one that joined. Everything can be skipped. The picture on the
 * right is the sidebar as it will be.
 */

const PREPARE_KEY = "myco-prepare";
/** The workspace waiting for this page (a new install), if any. */
export function preparePending(): string | null {
  try { return localStorage.getItem(PREPARE_KEY); } catch { return null; }
}
export function setPreparePending(hubId: string | null) {
  try {
    if (hubId) localStorage.setItem(PREPARE_KEY, hubId);
    else localStorage.removeItem(PREPARE_KEY);
  } catch { /* not kept */ }
}

const DAY = 86_400_000;
const base = (p: string) => p.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || p;
const cliName = (cli: string) => (cli === "codex" ? "Codex" : "Claude Code");
/** What it will be called (the hub has the last word). */
const nameOf = (s: LocalSession) => base(s.workdir);

export function Prepare({ hubId, now, onDone }: {
  hubId: string;
  now: number;
  /** in: with the agents that joined (the first one's chat opens) */
  onDone: (joined: string[]) => void;
}) {
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [all, setAll] = useState(false);
  const [busy, setBusy] = useState<{ at: number; of: number } | null>(null);
  const [failed, setFailed] = useState<{ s: LocalSession; error: unknown }[]>([]);
  const [joinedSoFar, setJoinedSoFar] = useState<string[]>([]);
  useEffect(() => {
    appApi.localSessions().then((r) => {
      // Sessions people had (not ones a program ran), not in a workspace yet.
      const mine = r.sessions.filter((s) => !s.auto && !s.joined)
        .sort((a, b) => (b.last_active > a.last_active ? 1 : -1));
      setSessions(mine);
      // The ones of the last few days, a different folder each, two at most.
      const pick = new Set<string>();
      const dirs = new Set<string>();
      for (const s of mine) {
        if (pick.size >= 2 || now - new Date(s.last_active).getTime() > 3 * DAY) break;
        if (dirs.has(s.workdir)) continue;
        dirs.add(s.workdir);
        pick.add(s.session_id);
      }
      setPicked(pick);
    }, () => setSessions([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const shown = sessions ? (all ? sessions : sessions.slice(0, 5)) : [];
  const chosen = useMemo(() => (sessions || []).filter((s) => picked.has(s.session_id)), [sessions, picked]);
  const toggle = (id: string) => setPicked((p) => { const n = new Set(p); if (n.has(id)) n.delete(id); else n.add(id); return n; });

  const go = async () => {
    if (!chosen.length) { onDone([]); return; }
    const joined: string[] = [];
    const bad: { s: LocalSession; error: unknown }[] = [];
    for (let i = 0; i < chosen.length; i++) {
      setBusy({ at: i + 1, of: chosen.length });
      const s = chosen[i];
      try {
        const r = await appApi.localJoin(hubId, { cli: s.cli, session_id: s.session_id, workdir: s.workdir, fork: true });
        joined.push(r.agent);
      } catch (error) {
        bad.push({ s, error });
      }
    }
    setBusy(null);
    if (bad.length) {
      // What could not join stays ticked, with why; the rest are in.
      setFailed(bad);
      setPicked(new Set(bad.map((b) => b.s.session_id)));
      setSessions((ss) => (ss || []).filter((s) => !chosen.includes(s) || bad.some((b) => b.s === s)));
      if (joined.length) setJoinedSoFar((j) => [...j, ...joined]);
      return;
    }
    onDone([...joinedSoFar, ...joined]);
  };

  const clisPart = (
    <>
          <h2 className="wl-sec">{L("AI 命令行", "Agent CLIs")}</h2>
          <section className="app-setting-card app-setting-lines wl-card"><LocalAgentLines /></section>

    </>
  );
  const sessionsPart = (
    <>
          <h2 className="wl-sec">{L("把已有的会话加进来", "Bring in sessions you have")}
            <small>{L("加进来的是一份副本，原来的会话还是你的。", "Each joins as a copy; the original stays yours.")}</small></h2>
          {sessions === null ? (
            <div className="wl-card wl-sessions"><div className="wl-session is-skel"><span className="skel skel-line" style={{ width: "60%" }} /></div></div>
          ) : sessions.length === 0 ? (
            <div className="wl-card wl-none" data-testid="prepare-no-sessions">{L(
              "这台电脑上还没有 Claude Code 或 Codex 的会话。进去以后，在「添加 agent」里可以新开一个。",
              "No Claude Code or Codex sessions on this computer yet. Inside, Add agent starts a new one.",
            )}</div>
          ) : (
            <div className="wl-card wl-sessions" role="group" aria-label={L("把已有的会话加进来", "Bring in sessions you have")}>
              {shown.map((s) => {
                const on = picked.has(s.session_id);
                const why = failed.find((f) => f.s.session_id === s.session_id);
                return (
                  <label key={s.session_id} className={cn("wl-session", on && "is-on")} data-testid="prepare-session">
                    <input type="checkbox" checked={on} onChange={() => toggle(s.session_id)} disabled={!!busy} />
                    <span className="wl-check" aria-hidden>{on && <Check />}</span>
                    <span className="wl-session-text">
                      <b>{s.title || base(s.workdir)}</b>
                      <small><span className="mono">{s.workdir}</span> · {cliName(s.cli)} · {timeAgo(s.last_active, now)}</small>
                      {why && <ErrorText error={why.error} />}
                    </span>
                  </label>
                );
              })}
              {sessions.length > 5 && (
                <button type="button" className="wl-more" onClick={() => setAll((a) => !a)}>
                  {all ? L("收起", "Fewer") : L(`显示全部 ${sessions.length} 个`, `Show all ${sessions.length}`)}
                </button>
              )}
            </div>
          )}
    </>
  );
  return (
    <div className="wl wl-prep" data-testid="prepare">
      <div className="app-welcome-drag" style={{ "--wails-draggable": "drag" } as React.CSSProperties} />
      <section className="wl-left">
        <header className="wl-head"><Logo className="wl-logo" /></header>
        <div className="wl-body">
          <p className="wl-eyebrow">{L("工作区已经开好了", "Your workspace is running")}</p>
          <h1 className="wl-title">{L("准备这台电脑", "Set up this computer")}</h1>
          <p className="wl-lead">{L(
            "myco 用你自己装好、登录好的 Claude Code、Codex，用的是你已有的订阅。下面是在这台电脑上找到的。",
            "myco runs the Claude Code and Codex you installed and signed in to, on your own subscriptions. Here is what this computer has.",
          )}</p>

          {/* What the button below acts on comes first, when there is any. */}
          {sessions && sessions.length > 0 ? <>{sessionsPart}{clisPart}</> : <>{clisPart}{sessionsPart}</>}
        </div>
        <footer className="wl-foot wl-prep-foot">
          <span>{L("以后在「添加 agent」里随时能加。", "You can add more any time with Add agent.")}</span>
          <div className="wl-prep-actions">
            <button type="button" className="app-btn" onClick={() => onDone(joinedSoFar)} disabled={!!busy}>{L("先跳过", "Skip for now")}</button>
            <button type="button" className="app-btn app-btn-primary wl-start" onClick={() => void go()} disabled={!!busy} data-testid="prepare-go">
              {busy ? <><span className="app-spinner" />{L(`正在加入 ${busy.at}/${busy.of}…`, `Joining ${busy.at}/${busy.of}…`)}</>
                : <>{chosen.length ? L(`加入 ${chosen.length} 个并开始`, `Add ${chosen.length} and start`) : L("开始", "Start")}<ArrowRight /></>}
            </button>
          </div>
        </footer>
      </section>
      <aside className="wl-stage" aria-hidden>
        <PrepStage chosen={chosen} />
      </aside>
    </div>
  );
}

/** The sidebar as it will be: the sessions ticked, as agents. */
function PrepStage({ chosen }: { chosen: LocalSession[] }) {
  const faces = chosen.slice(0, 5).map((s) => ({
    s,
    a: { id: `prep-${s.session_id}`, name: nameOf(s), display_name: nameOf(s), binding: { harness: s.cli }, runtime: { cli: s.cli } } as unknown as Agent,
  }));
  return (
    <div className="wl-scene">
      <div className="wl-win wl-prep-win">
        <div className="wl-win-bar"><i /><i /><i /></div>
        <div className="wl-prep-side">
          <div className="wl-side-title">{L("会话", "Chats")}</div>
          {faces.map(({ s, a }) => (
            <div key={s.session_id} className="wl-member wl-prep-row">
              <Avatar agent={a} size={30} />
              <span><b>{a.display_name}</b><small>{s.title || s.workdir}</small></span>
            </div>
          ))}
          {Array.from({ length: Math.max(0, 3 - faces.length) }, (_, i) => (
            <div key={`ghost-${i}`} className="wl-member wl-ghost"><i /><span><em /><em /></span></div>
          ))}
        </div>
      </div>
      <p className="wl-cap">
        <b>{chosen.length ? L("进去以后，它们就在侧栏里", "Inside, they are in the sidebar") : L("选几个会话，它们就会出现在这里", "Pick a few sessions to see them here")}</b>
        <span>{L("每一个都是一个 agent：直接聊，或者拉进一个项目一起做事", "Each one is an agent: chat with it, or bring it into a project with others")}</span>
      </p>
    </div>
  );
}
