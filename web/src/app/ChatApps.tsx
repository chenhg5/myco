import { useCallback, useEffect, useRef, useState } from "react";
import { MessageSquare, RotateCcw } from "lucide-react";
import { appApi, type HubOverview, type IMBridge } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText } from "@/app/dialogs";
import { nodeName } from "@/app/nodes";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const PLATFORM: Record<string, () => string> = {
  feishu: () => L("飞书", "Feishu"),
  lark: () => "Lark",
  slack: () => "Slack",
};

type Scan = { id: string; qr: string; url: string; state: string; error?: string };

/** Settings › Chat apps: bots in Feishu/Lark or Slack that people talk to
 *  like an agent. Feishu is a QR code scanned with the phone app; Slack is
 *  an app made from a manifest, and its two tokens. The bots run on this
 *  computer (its runner service) and speak as a node of yours. */
export function ChatAppsSection({ hubsOv }: { hubsOv?: HubOverview[] }) {
  const hub = (hubsOv || []).find((h) => h.ok) || hubsOv?.[0];
  const agents = (hub?.overview?.agents || [])
    .map((s) => s.agent)
    .filter((a) => (!a.kind || a.kind === "agent") && !a.name.includes("im-"));
  const [bridges, setBridges] = useState<IMBridge[] | null>(null);
  const [slackURL, setSlackURL] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [agent, setAgent] = useState("");
  const [adding, setAdding] = useState<"" | "feishu" | "slack">("");

  const load = useCallback(async () => {
    if (!hub) return;
    try {
      const r = await appApi.imBridges(hub.id);
      setBridges(r.bridges);
      setSlackURL(r.slack_create_url);
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [hub?.id]);

  useEffect(() => { void load(); }, [load]);
  // A new bot comes online within seconds of being saved: look again.
  useEffect(() => {
    if (!bridges?.some((b) => !b.online)) return;
    const t = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(t);
  }, [bridges, load]);
  useEffect(() => {
    if (!agent && agents.length) setAgent(agents[0].name);
  }, [agents.length]);

  if (!hub) {
    return <div className="app-empty-note">{L("先连上一个工作区。", "Connect a workspace first.")}</div>;
  }
  const agentPicker = (
    <select className="app-select-field im-agent" value={agent} onChange={(e) => setAgent(e.target.value)} aria-label={L("交给哪个 agent", "Which agent")}>
      {agents.length === 0 && <option value="">{L("（还没有 agent）", "(no agents yet)")}</option>}
      {agents.map((a) => <option key={a.name} value={a.name}>{nodeName(a, hub)}</option>)}
    </select>
  );

  return (
    <>
      <section className="app-setting-card app-setting-lines">
        <div data-setting="chat-apps" className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("在聊天软件里找 agent", "Talk to agents from a chat app")}</b>
            <span>{L("连上后，在飞书或 Slack 里给机器人发消息，就是交给下面选的 agent 做事：进度在原消息上更新，它有问题会在聊天里问你，做完把结果发回来。机器人跑在这台电脑上，不需要公网地址。",
              "Once connected, what you send the bot in Feishu or Slack goes to the agent chosen below: progress updates in place, it asks you there when it has a question, and its answer comes back in the chat. The bot runs on this computer; no public address is needed.")}</span>
          </div>
        </div>
        <div className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("交给哪个 agent", "Which agent gets the messages")}</b>
            <span>{L("在聊天里也能随时换：发 /agent 名字，或者 /to 名字 要做的事。", "It can be changed in the chat too: /agent name, or /to name what to do.")}</span>
          </div>
          {agentPicker}
        </div>
      </section>

      <h4 className="app-setting-group">{L("已连接", "Connected")}</h4>
      <section className="app-setting-card app-setting-lines" data-testid="im-bridges">
        {bridges === null && !error && <div className="app-setting-line"><span className="app-empty-note">{L("正在读取…", "Loading…")}</span></div>}
        {bridges?.length === 0 && (
          <div className="app-setting-line"><span className="app-empty-note">{L("还没有连接聊天软件。", "No chat apps connected yet.")}</span></div>
        )}
        {bridges?.map((b) => <BridgeRow key={b.name} b={b} agents={agents.map((a) => ({ name: a.name, label: nodeName(a, hub) }))} onChanged={load} />)}
        {error ? <div className="app-setting-line"><ErrorText error={error} onRetry={() => void load()} /></div> : null}
      </section>

      <h4 className="app-setting-group">{L("添加", "Add")}</h4>
      <section className="app-setting-card app-setting-lines">
        <div className="app-setting-line">
          <span className="im-icon"><MessageSquare className="h-4 w-4" /></span>
          <div className="app-setting-line-text">
            <b>{L("飞书 / Lark", "Feishu / Lark")}</b>
            <span>{L("用飞书手机 App 扫一下码，就建好一个你自己的机器人。", "Scan a code with the Feishu or Lark phone app and your own bot is made.")}</span>
          </div>
          {adding !== "feishu" && <button type="button" className="app-btn app-btn-quiet" disabled={!agent} onClick={() => setAdding("feishu")}>{L("扫码连接", "Connect")}</button>}
        </div>
        {adding === "feishu" && <FeishuScan hubId={hub.id} agent={agent} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
        <div className="app-setting-line">
          <span className="im-icon"><MessageSquare className="h-4 w-4" /></span>
          <div className="app-setting-line-text">
            <b>Slack</b>
            <span>{L("在 Slack 里建一个应用（设置都已填好），装到你的工作区，再把两个令牌粘到这里。", "Make an app in Slack (its settings are filled in), install it to your workspace, and paste its two tokens here.")}</span>
          </div>
          {adding !== "slack" && <button type="button" className="app-btn app-btn-quiet" disabled={!agent} onClick={() => setAdding("slack")}>{L("连接", "Connect")}</button>}
        </div>
        {adding === "slack" && <SlackForm hubId={hub.id} agent={agent} createURL={slackURL} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
      </section>
    </>
  );
}

function BridgeRow({ b, agents, onChanged }: { b: IMBridge; agents: { name: string; label: string }[]; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirm, setConfirm] = useState(false);
  const act = async (f: () => Promise<unknown>) => {
    setBusy(true); setError(null);
    try { await f(); await onChanged(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const status = b.online
    ? { dot: "working", text: L("在线", "Online") }
    : { dot: "pending", text: L("正在启动…（这台电脑的 runner 几秒内会接上）", "Starting… (this computer's runner picks it up in a few seconds)") };
  return (
    <div className="app-setting-line im-bridge" data-bridge={b.name}>
      <span className="im-icon"><MessageSquare className="h-4 w-4" /></span>
      <div className="app-setting-line-text">
        <b>{PLATFORM[b.platform]?.() || b.platform}{b.name !== b.platform && <span className="im-name"> · {b.name}</span>}</b>
        <span className="im-status"><i className={cn("dot", status.dot)} aria-hidden />{status.text}</span>
        {b.claim && (
          <span className="im-claim">
            {L("还没有主人：在飞书或 Slack 里私聊机器人，发这一行就成为它的主人：", "No owner yet: send the bot this line in a direct message to become its owner:")}
            <span className="app-command"><code>/claim {b.claim}</code><CopyButton value={`/claim ${b.claim}`} /></span>
          </span>
        )}
        {error ? <ErrorText error={error} /> : null}
      </div>
      <select className="app-select-field im-agent" value={b.agent || ""} disabled={busy} aria-label={L("交给哪个 agent", "Which agent")}
        onChange={(e) => void act(() => appApi.setIMBridge(b.name, e.target.value))}>
        {!b.agent && <option value="">{L("（未设置）", "(not set)")}</option>}
        {b.agent && !agents.some((a) => a.name === b.agent) && <option value={b.agent}>{b.agent}</option>}
        {agents.map((a) => <option key={a.name} value={a.name}>{a.label}</option>)}
      </select>
      {confirm
        ? <button type="button" className="app-btn app-btn-danger" disabled={busy} onClick={() => void act(() => appApi.removeIMBridge(b.name))}>{L("确定断开", "Disconnect")}</button>
        : <button type="button" className="app-btn app-btn-quiet" disabled={busy} onClick={() => setConfirm(true)}>{L("断开", "Disconnect")}</button>}
    </div>
  );
}

function FeishuScan({ hubId, agent, onDone, onCancel }: { hubId: string; agent: string; onDone: () => void; onCancel: () => void }) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [error, setError] = useState<unknown>(null);
  const idRef = useRef("");
  const start = useCallback(async () => {
    setError(null); setScan(null);
    try {
      const r = await appApi.startFeishuScan(hubId, agent);
      idRef.current = r.id;
      setScan({ ...r, state: "waiting" });
    } catch (e) { setError(e); }
  }, [hubId, agent]);
  useEffect(() => {
    void start();
    return () => { if (idRef.current) void appApi.stopFeishuScan(idRef.current).catch(() => {}); };
  }, [start]);
  useEffect(() => {
    if (!scan || scan.state === "done" || scan.state === "failed") return;
    const t = window.setInterval(async () => {
      try {
        const r = await appApi.feishuScan(scan.id);
        setScan((s) => s && { ...s, state: r.state, error: r.error });
        if (r.state === "done") { idRef.current = ""; onDone(); }
      } catch (e) { setError(e); }
    }, 2000);
    return () => window.clearInterval(t);
  }, [scan?.id, scan?.state]);

  return (
    <div className="app-setting-line im-scan" data-testid="feishu-scan">
      {scan?.qr && scan.state === "waiting" && <img className="im-qr" src={scan.qr} alt={L("飞书扫码", "Feishu QR code")} width={168} height={168} />}
      <div className="app-setting-line-text">
        {!scan && !error && <span>{L("正在向飞书要二维码…", "Getting a QR code from Feishu…")}</span>}
        {scan?.state === "waiting" && <>
          <b>{L("用飞书或 Lark 手机 App 扫码", "Scan with the Feishu or Lark phone app")}</b>
          <span>{L("扫码后在手机上确认，飞书会建好机器人；你就是它的主人。二维码 10 分钟内有效。", "Approve on the phone and Feishu makes the bot, with you as its owner. The code works for 10 minutes.")}</span>
        </>}
        {scan?.state === "connecting" && <span>{L("扫好了，正在连接…", "Scanned; connecting…")}</span>}
        {scan?.state === "failed" && <span className="app-error-text">{scan.error || L("没有连上。", "It did not connect.")}</span>}
        {error ? <ErrorText error={error} /> : null}
      </div>
      {(scan?.state === "failed" || !!error) && (
        <button type="button" className="app-btn app-btn-quiet" onClick={() => void start()}><RotateCcw className="h-3.5 w-3.5" />{L("再来一次", "Try again")}</button>
      )}
      <button type="button" className="app-btn app-btn-quiet" onClick={onCancel}>{L("取消", "Cancel")}</button>
    </div>
  );
}

function SlackForm({ hubId, agent, createURL, onDone, onCancel }: { hubId: string; agent: string; createURL: string; onDone: () => void; onCancel: () => void }) {
  const [bot, setBot] = useState("");
  const [app, setApp] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const save = async () => {
    setBusy(true); setError(null);
    try { await appApi.connectSlack(hubId, agent, bot.trim(), app.trim()); onDone(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const ok = bot.trim().startsWith("xoxb-") && app.trim().startsWith("xapp-");
  return (
    <div className="app-setting-line im-slack" data-testid="slack-form">
      <ol className="im-steps">
        <li>{L("打开 Slack 的建应用页（设置都已填好），选你的工作区，创建：", "Open Slack's create-app page (the settings are filled in), pick your workspace and create it:")}{" "}
          <a href={createURL} target="_blank" rel="noreferrer">{L("在 Slack 里创建", "Create in Slack")} ↗</a></li>
        <li>{L("「Install App」装到工作区，复制 Bot User OAuth Token（xoxb- 开头）：", "Install it to the workspace and copy the Bot User OAuth Token (starts xoxb-):")}
          <input className="app-input mono" type="password" value={bot} onChange={(e) => setBot(e.target.value)} placeholder="xoxb-…" autoComplete="off" spellCheck={false} /></li>
        <li>{L("「Basic Information」里生成一个 App-Level Token，权限选 connections:write（xapp- 开头）：", "Under Basic Information, generate an app-level token with connections:write (starts xapp-):")}
          <input className="app-input mono" type="password" value={app} onChange={(e) => setApp(e.target.value)} placeholder="xapp-…" autoComplete="off" spellCheck={false} /></li>
      </ol>
      <span className="app-small-note">{L("令牌只存在这台电脑上。连好后在 Slack 里私聊机器人，发这里给出的 /claim 码，就成为它的主人。", "The tokens stay on this computer. Once connected, send the bot the /claim code shown here to become its owner.")}</span>
      {error ? <ErrorText error={error} /> : null}
      <div className="im-actions">
        <button type="button" className="app-btn app-btn-quiet" onClick={onCancel}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={!ok || busy} onClick={() => void save()}>{busy ? L("连接中…", "Connecting…") : L("连接", "Connect")}</button>
      </div>
    </div>
  );
}
