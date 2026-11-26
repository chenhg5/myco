import { useCallback, useEffect, useRef, useState } from "react";
import { RotateCcw } from "lucide-react";
import { appApi, isOldHub, type IMBridge } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { CopyButton } from "@/app/copy";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const PLATFORM: Record<string, () => string> = {
  feishu: () => L("飞书", "Feishu"),
  lark: () => "Lark",
  slack: () => "Slack",
};

type Scan = { id: string; qr: string; url: string; state: string; error?: string };

/** An agent's chat-app bots, in its details ("Name and access"): each agent
 *  can have its own Feishu/Lark bot and Slack bot (as voice is per agent);
 *  what is written to the bot goes to this agent. Bind one by scanning a
 *  QR code (Feishu) or pasting a Slack app's two tokens; it runs on this
 *  computer. Only for those who may manage the agent, in an App that runs
 *  on a computer (the route is absent otherwise, and so is the row). */
export function ChatAppsRow({ row, mayManage }: { row: AgentRow; mayManage: boolean }) {
  const ag = row.st.agent;
  const [bridges, setBridges] = useState<IMBridge[] | null>(null);
  const [slackURL, setSlackURL] = useState("");
  const [gone, setGone] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [adding, setAdding] = useState<"" | "feishu" | "slack">("");
  const [removing, setRemoving] = useState<IMBridge | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await appApi.imBridges(row.hub.id, ag.name);
      setBridges(r.bridges);
      setSlackURL(r.slack_create_url);
      setError(null);
    } catch (e) {
      if (isOldHub(e)) setGone(true); else setError(e);
    }
  }, [row.hub.id, ag.name]);
  const show = mayManage && agentKind(ag) === "agent" && !gone;
  useEffect(() => { if (show) void load(); }, [show, load]);
  // A new bot comes online within seconds of being saved: look again.
  useEffect(() => {
    if (!bridges?.some((b) => !b.online)) return;
    const t = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(t);
  }, [bridges, load]);
  if (!show) return null;

  const has = (p: string) => bridges?.some((b) => b.platform === p || (p === "feishu" && b.platform === "lark"));
  const name = nodeName(ag, row.hub);
  const label = (b: IMBridge) => PLATFORM[b.platform]?.() || b.platform;
  // "Slack 机器人", "飞书机器人": a space only after a Latin name.
  const sp = (b: IMBridge) => (/[A-Za-z]$/.test(label(b)) ? " " : "");
  const renew = async (b: IMBridge) => {
    setBusy(true); setError(null);
    try { await appApi.renewIMClaim(b.name); await load(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const remove = async (b: IMBridge) => {
    setBusy(true); setError(null);
    try { await appApi.removeIMBridge(b.name); setRemoving(null); await load(); } catch (e) { setError(e); } finally { setBusy(false); }
  };

  return (
    <>
      <div className="app-kv has-help" data-testid="chat-apps-row">
        <span className="app-kv-label">{L("聊天软件", "Chat apps")}</span>
        <span className="app-kv-value app-resume-actions">
          {bridges?.length ? bridges.map((b) => (
            <span key={b.name} className="im-bot" data-bridge={b.name}>
              <i className={cn("dot", b.online ? "working" : "pending")} aria-hidden />
              {label(b)}
              <span className="app-kv-note">{b.online ? L("在线", "online") : L("启动中…", "starting…")}</span>
              <button type="button" className="app-kv-link" disabled={busy} onClick={() => setRemoving(b)}>{L("解绑", "Unbind")}</button>
            </span>
          )) : <span className="app-kv-note">{bridges ? L("还没有绑定", "None yet") : L("正在读取…", "Loading…")}</span>}
          {bridges && !has("feishu") && <button type="button" className="app-kv-link" onClick={() => setAdding("feishu")}>{L("绑定飞书", "Bind Feishu")}</button>}
          {bridges && !has("slack") && <button type="button" className="app-kv-link" onClick={() => setAdding("slack")}>{L("绑定 Slack", "Bind Slack")}</button>}
        </span>
        <span className="app-kv-help">
          {L(`给它一个飞书或 Slack 机器人：在聊天软件里发消息，就是交给 ${name} 做事，进度和结果都回到聊天里。（预览）机器人跑在这台电脑上，电脑睡眠时它会离线。`,
            `Give it a Feishu or Slack bot: what you send the bot goes to ${name}, and its progress and answer come back in the chat. (Preview) The bot runs on this computer, and is offline while it sleeps.`)}
        </span>
        {bridges?.filter((b) => b.claim).map((b) => (
          <span key={b.name} className="im-claim">
            {L(`${label(b)}${sp(b)}机器人还没有主人：私聊机器人发这一行，就成为它的主人：`, `The ${label(b)} bot has no owner yet: send it this line in a direct message to become its owner:`)}
            <span className="app-command"><code>/claim {b.claim}</code><CopyButton value={`/claim ${b.claim}`} /></span>
          </span>
        ))}
        {bridges?.filter((b) => b.claim_expired).map((b) => (
          <span key={b.name} className="im-claim">
            {L(`${label(b)}${sp(b)}机器人还没有主人，认领码已经失效（过期或错太多次）。`, `The ${label(b)} bot has no owner yet, and its claim code no longer works (it expired, or was mistyped too often). `)}
            <button type="button" className="app-kv-link" disabled={busy} onClick={() => void renew(b)}>{L("重新生成认领码", "New claim code")}</button>
          </span>
        ))}
        <ErrorText error={error} />
      </div>

      <Modal open={adding !== ""} onOpenChange={(o) => !o && setAdding("")}
        title={adding === "slack" ? L(`给 ${name} 绑定 Slack 机器人`, `A Slack bot for ${name}`) : L(`给 ${name} 绑定飞书机器人`, `A Feishu bot for ${name}`)}
        description={adding === "slack"
          ? L("在 Slack 里建一个应用（设置都已填好），装到你的工作区，再把两个令牌粘到这里。", "Make an app in Slack (its settings are filled in), install it to your workspace, and paste its two tokens here.")
          : L("用飞书或 Lark 手机 App 扫码，在手机上确认，飞书就建好这个机器人；你就是它的主人。", "Scan with the Feishu or Lark phone app and approve: Feishu makes the bot, with you as its owner.")}>
        {adding === "feishu" && <FeishuScan hubId={row.hub.id} agent={ag.name} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
        {adding === "slack" && <SlackForm hubId={row.hub.id} agent={ag.name} createURL={slackURL} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={L("解绑这个机器人？", "Unbind this bot?")}
        description={L("这台电脑不再运行它，聊天软件里的消息不会再交给这个 agent。飞书或 Slack 里的应用还在，可以到那边删掉。", "This computer stops running it, and messages in the chat app no longer reach this agent. The app stays in Feishu or Slack; delete it there if you like.")}
        confirmLabel={L("解绑", "Unbind")}
        busy={busy}
        onConfirm={() => { if (removing) void remove(removing); }}
      />
    </>
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
    <div className="im-scan" data-testid="feishu-scan">
      <div className="im-qr-box">
        {scan?.qr && scan.state === "waiting"
          ? <img className="im-qr" src={scan.qr} alt={L("飞书扫码", "Feishu QR code")} width={184} height={184} />
          : <div className="im-qr im-qr-empty" />}
      </div>
      <p className="im-scan-note">
        {!scan && !error && L("正在向飞书要二维码…", "Getting a QR code from Feishu…")}
        {scan?.state === "waiting" && L("二维码 10 分钟内有效。扫好后这里会自动完成。", "The code works for 10 minutes. This finishes by itself once scanned.")}
        {scan?.state === "connecting" && L("扫好了，正在连接…", "Scanned; connecting…")}
        {scan?.state === "failed" && <span className="app-error-text">{scan.error || L("没有连上。", "It did not connect.")}</span>}
      </p>
      <ErrorText error={error} />
      <div className="app-modal-actions">
        {(scan?.state === "failed" || !!error) && (
          <button type="button" className="app-btn app-btn-quiet" onClick={() => void start()}><RotateCcw className="h-3.5 w-3.5" />{L("再来一次", "Try again")}</button>
        )}
        <button type="button" className="app-btn" onClick={onCancel}>{L("取消", "Cancel")}</button>
      </div>
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
    <div className="im-slack" data-testid="slack-form">
      <ol className="im-steps">
        <li>{L("打开 Slack 的建应用页（设置都已填好），选你的工作区，创建：", "Open Slack's create-app page (the settings are filled in), pick your workspace and create it:")}{" "}
          <a href={createURL} target="_blank" rel="noreferrer">{L("在 Slack 里创建", "Create in Slack")} ↗</a></li>
        <li>{L("「Install App」装到工作区，复制 Bot User OAuth Token（xoxb- 开头）：", "Install it to the workspace and copy the Bot User OAuth Token (starts xoxb-):")}
          <input className="app-input mono" type="password" value={bot} onChange={(e) => setBot(e.target.value)} placeholder="xoxb-…" autoComplete="off" spellCheck={false} /></li>
        <li>{L("「Basic Information」里生成一个 App-Level Token，权限选 connections:write（xapp- 开头）：", "Under Basic Information, generate an app-level token with connections:write (starts xapp-):")}
          <input className="app-input mono" type="password" value={app} onChange={(e) => setApp(e.target.value)} placeholder="xapp-…" autoComplete="off" spellCheck={false} /></li>
      </ol>
      <p className="app-small-note">{L("令牌只存在这台电脑上。连好后会给出一行 /claim 码，私聊机器人发一下就成为它的主人。", "The tokens stay on this computer. Once connected you get a /claim line; send it to the bot in a direct message to become its owner.")}</p>
      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={onCancel}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={!ok || busy} onClick={() => void save()}>{busy ? L("连接中…", "Connecting…") : L("连接", "Connect")}</button>
      </div>
    </div>
  );
}
