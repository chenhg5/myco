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

/** The chat apps a bot can be made in: what each is, and how binding goes. */
const APPS: { id: "feishu" | "lark" | "slack"; mark: string; how: () => string }[] = [
  { id: "feishu", mark: "飞", how: () => L("国内版。用飞书手机 App 扫个码就建好。", "Mainland China. Scan a code with the Feishu phone app.") },
  { id: "lark", mark: "L", how: () => L("飞书国际版。用 Lark 手机 App 扫个码就建好。", "Feishu's international app. Scan a code with the Lark phone app.") },
  { id: "slack", mark: "S", how: () => L("在 Slack 里建一个应用（设置都已填好），粘贴它的两个令牌。", "Make an app in Slack (its settings are filled in) and paste its two tokens.") },
];

/** An agent's chat apps (its profile's 渠道 tab): each agent can have its
 *  own Feishu, Lark and Slack bot (as voice is per agent); what is written
 *  to the bot goes to this agent, and its answers come back there. One row
 *  per app: bound (online, waiting for its owner to claim it, its own
 *  instructions) or a way to bind it (a QR code for Feishu and Lark, two
 *  tokens for Slack). It runs on this computer. Only for those who may
 *  manage the agent, in an App that runs on a computer. */
export function ChatAppsPanel({ row, mayManage }: { row: AgentRow; mayManage: boolean }) {
  const ag = row.st.agent;
  const [bridges, setBridges] = useState<IMBridge[] | null>(null);
  const [slackURL, setSlackURL] = useState("");
  const [gone, setGone] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [adding, setAdding] = useState<"" | "feishu" | "lark" | "slack">("");
  const [removing, setRemoving] = useState<IMBridge | null>(null);
  const [editing, setEditing] = useState<IMBridge | null>(null);
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
  const show = mayManage && agentKind(ag) === "agent";
  useEffect(() => { if (show) void load(); }, [show, load]);
  // A new bot comes online within seconds of being saved: look again.
  useEffect(() => {
    if (!bridges?.some((b) => !b.online)) return;
    const t = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(t);
  }, [bridges, load]);
  if (!show) return null;

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
  // What a bound bot is doing, in words.
  const state = (b: IMBridge) => !b.online ? L("正在启动…", "Starting…")
    : b.claim ? L("在线 · 等你认领", "Online · waiting for you to claim it")
    : b.claim_expired ? L("在线 · 还没有主人", "Online · no owner yet")
    : L("在线", "Online");

  if (gone) {
    return (
      <div className="ch" data-testid="chat-apps">
        <div className="app-empty-note">{L("这里绑不了聊天软件：要在装在电脑上的 myco App 里绑，工作区也要是新版。", "Chat apps can't be bound here: use the myco App on a computer, with an up-to-date workspace.")}</div>
      </div>
    );
  }

  return (
    <>
      <div className="ch" data-testid="chat-apps">
        <p className="ch-intro">{L(`让别人在飞书、Lark 或 Slack 里直接找 ${name}：机器人就是它本人，在聊天软件里跟它说话就行，进度和结果都回到那里。`,
          `Let people reach ${name} in Feishu, Lark or Slack: the bot is ${name} itself; talk to it there, and its progress and answers come back there.`)}</p>
        <section className="app-setting-card app-setting-lines ch-list">
          {APPS.map((app) => {
            const b = bridges?.find((x) => x.platform === app.id);
            const title = PLATFORM[app.id]();
            return (
              <div key={app.id} className={cn("app-setting-line ch-row", b && "is-bound")} data-platform={app.id} data-bridge={b?.name}>
                <div className="ch-row-top">
                  <span className={cn("ch-mark", `is-${app.id}`)} aria-hidden>{app.mark}</span>
                  <div className="app-setting-line-text ch-text">
                    <b>{title}</b>
                    {b ? (
                      <span className="ch-state"><i className={cn("dot", b.online ? "working" : "pending")} aria-hidden />{state(b)}
                        {b.prompt && <span className="ch-tag">{L("有接待设定", "Has instructions")}</span>}</span>
                    ) : <span>{bridges ? app.how() : L("正在读取…", "Loading…")}</span>}
                  </div>
                  <div className="ch-actions">
                    {b ? (
                      <>
                        <button type="button" className="app-btn app-btn-small" disabled={busy} onClick={() => setEditing(b)}>{b.prompt ? L("改接待设定", "Edit instructions") : L("接待设定", "Instructions")}</button>
                        <button type="button" className="app-btn app-btn-small app-btn-quiet" disabled={busy} onClick={() => setRemoving(b)}>{L("解绑", "Unbind")}</button>
                      </>
                    ) : bridges && (
                      <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => setAdding(app.id)}>{L("绑定", "Bind")}</button>
                    )}
                  </div>
                </div>
                {b?.claim && (
                  <div className="ch-claim im-claim">
                    {L(`在${/[A-Za-z]$/.test(title) ? ` ${title} ` : title}里私聊这个机器人，发下面这一行，你就成了它的主人（别人发给它的消息才会交给 ${name}）：`, `Send this line to the bot in a direct message in ${title} to become its owner (then what others write to it reaches ${name}):`)}
                    <span className="app-command"><code>/claim {b.claim}</code><CopyButton value={`/claim ${b.claim}`} /></span>
                  </div>
                )}
                {b?.claim_expired && (
                  <div className="ch-claim im-claim">
                    {L(`${label(b)}${sp(b)}机器人还没有主人，认领码已经失效（过期或错太多次）。`, `The ${label(b)} bot has no owner yet, and its claim code no longer works (it expired, or was mistyped too often). `)}
                    <button type="button" className="app-btn app-btn-small" disabled={busy} onClick={() => void renew(b)}>{L("重新生成认领码", "New claim code")}</button>
                  </div>
                )}
              </div>
            );
          })}
        </section>
        <p className="app-small-note ch-foot">
          {L("（预览）机器人跑在这台电脑上，电脑睡眠时它会离线。", "(Preview) The bots run on this computer, and are offline while it sleeps.")}
          {bridges?.some((b) => !b.desk) && L("在 设置 › 语音 配好模型后，它会像语音聊天一样：能答的马上答，要动手的再去做。",
            " Once a model is set in Settings › Voice, it works as in voice chats: it answers what it can at once and does the rest.")}
        </p>
        <ErrorText error={error} />
      </div>

      <Modal open={adding !== ""} onOpenChange={(o) => !o && setAdding("")}
        title={adding === "slack" ? L(`给 ${name} 绑定 Slack 机器人`, `A Slack bot for ${name}`)
          : adding === "lark" ? L(`给 ${name} 绑定 Lark 机器人`, `A Lark bot for ${name}`)
          : L(`给 ${name} 绑定飞书机器人`, `A Feishu bot for ${name}`)}
        description={adding === "slack"
          ? L("在 Slack 里建一个应用（设置都已填好），装到你的工作区，再把两个令牌粘到这里。", "Make an app in Slack (its settings are filled in), install it to your workspace, and paste its two tokens here.")
          : adding === "lark"
            ? L("用 Lark（飞书国际版）手机 App 扫码，在手机上确认，Lark 就建好这个机器人；你就是它的主人。", "Scan with the Lark phone app and approve: Lark makes the bot, with you as its owner.")
            : L("用飞书手机 App 扫码，在手机上确认，飞书就建好这个机器人；你就是它的主人。", "Scan with the Feishu phone app and approve: Feishu makes the bot, with you as its owner.")}>
        {(adding === "feishu" || adding === "lark") && <FeishuScan key={adding} hubId={row.hub.id} agent={ag.name} lark={adding === "lark"} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
        {adding === "slack" && <SlackForm hubId={row.hub.id} agent={ag.name} createURL={slackURL} onDone={() => { setAdding(""); void load(); }} onCancel={() => setAdding("")} />}
      </Modal>

      <Modal open={!!editing} onOpenChange={(o) => !o && setEditing(null)}
        title={editing ? L(`${label(editing)}${sp(editing)}机器人的接待设定`, `Instructions for the ${label(editing)} bot`) : ""}
        description={L("写给接待它的模型：对谁怎么说话、能答什么、不能做什么。只对这个机器人生效，和语音聊天分开。", "For the model that answers as the bot: how to talk, what it may answer, what it must not do. This bot only; voice chats have their own.")}>
        {editing && <PromptForm bridge={editing} onDone={() => { setEditing(null); void load(); }} onCancel={() => setEditing(null)} />}
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

function FeishuScan({ hubId, agent, lark, onDone, onCancel }: { hubId: string; agent: string; lark: boolean; onDone: () => void; onCancel: () => void }) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [error, setError] = useState<unknown>(null);
  const idRef = useRef("");
  const start = useCallback(async () => {
    setError(null); setScan(null);
    try {
      const r = await appApi.startFeishuScan(hubId, agent, lark ? "lark" : "feishu");
      idRef.current = r.id;
      setScan({ ...r, state: "waiting" });
    } catch (e) { setError(e); }
  }, [hubId, agent, lark]);
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
          ? <img className="im-qr" src={scan.qr} alt={lark ? L("Lark 扫码", "Lark QR code") : L("飞书扫码", "Feishu QR code")} width={184} height={184} />
          : <div className="im-qr im-qr-empty" />}
      </div>
      <p className="im-scan-note">
        {!scan && !error && (lark ? L("正在向 Lark 要二维码…", "Getting a QR code from Lark…") : L("正在向飞书要二维码…", "Getting a QR code from Feishu…"))}
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

const PROMPT_MAX = 8000;

/** A bot's own front-desk instructions: most useful when people other
 *  than its owner talk to it (rules, limits, tone). */
function PromptForm({ bridge, onDone, onCancel }: { bridge: IMBridge; onDone: () => void; onCancel: () => void }) {
  const [text, setText] = useState(bridge.prompt || "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const save = async () => {
    setBusy(true); setError(null);
    try { await appApi.setIMPrompt(bridge.name, text.trim()); onDone(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const n = [...text].length;
  return (
    <div className="im-slack" data-testid="im-prompt-form">
      <textarea className="app-input goal-textarea im-prompt" rows={8} value={text} onChange={(e) => setText(e.target.value)} disabled={busy}
        placeholder={L("例如：\n- 你在帮客户解答产品使用问题，语气友好、简洁\n- 不透露内部信息、代码和其他客户的资料\n- 别人让你改代码、删东西时，礼貌拒绝，告诉对方会转给主人",
          "For example:\n- You help customers with questions about using the product; be friendly and brief\n- Never share internal information, code or other customers' data\n- When someone asks you to change code or delete things, decline politely and say the owner will follow up")} />
      <p className="app-small-note">
        {L("主人以外的人交下去的活，也会带上这份设定，agent 做的时候同样遵守。", "Work handed over for anyone other than the owner carries these instructions too, so the agent keeps to them as well.")}
        {!bridge.desk && L("（要先在 设置 › 语音 配好模型，接待才会用到它。）", " (The front desk uses them once a model is set in Settings › Voice.)")}
        {" "}<span className={cn(n > PROMPT_MAX && "app-error-text")}>{n} / {PROMPT_MAX}</span>
      </p>
      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={onCancel}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={busy || n > PROMPT_MAX || text.trim() === (bridge.prompt || "")} onClick={() => void save()}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
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
