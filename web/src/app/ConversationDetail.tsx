import { Fragment, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowDown, ArrowLeft, ArrowRightLeft, ArrowUp, ArrowUpRight, Check, ChevronRight, Copy, AudioLines, Download, Images, Loader2, Lock, MoreHorizontal, Paintbrush, Pin as PinIcon, PinOff, Search, Settings2, Square, Trash2, WifiOff, X } from "lucide-react";
import { ExportChat } from "@/app/ExportChat";
import { appApi, isTransient } from "@/appapi";
import type { Agent, Message } from "@/api";
import { SHROOM, useIsSuperagent } from "@/app/superagentMark";
import type { AgentRow } from "@/app/types";
import { agentKind, canManage, isInbox, isMe, lastActivityAt, machineOfRow, personLabel, possiblyStuck, quietFor, runnerAway, SEEN_SLACK_MS, seenNow, sessionBusy, nodeName } from "@/app/nodes";
import { MessageContent } from "@/app/Markdown";
import { PromptSign, STOPPED, Terminal } from "@/app/Terminal";
import { TraceView } from "@/app/TraceView";
import { WorkingLine } from "@/app/Working";
import { AccessPill, RunPill } from "@/app/RunSettings";
import { forkAgent } from "@/app/Fork";
import { HandoffCard, handoffsFrom, withStale } from "@/app/Handoff";
import { TurnRail, type Turn } from "@/app/TurnRail";
import { summarize } from "@/app/summary";
import { ErrorText } from "@/app/dialogs";
import { recall, remember as rememberMemo } from "@/lib/memo";
import { useCopy } from "@/app/copy";
import { cn, timeAgo } from "@/lib/utils";
import { L, locale } from "@/lib/i18n";
import { Busy } from "@/app/Skeleton";
import { Avatar, whereOf, isWorkspace } from "@/app/persona";
import { AttachButton, AttachList, dropFiles, pastedFiles, useAttachments } from "@/app/attach";
import { useAutoGrow } from "@/app/autogrow";
import { MessageAttachments } from "@/app/attachments";
import { ChatFiles, type ChatApp, type ChatFile } from "@/app/ChatFiles";
import { ChatBgDialog } from "@/app/ChatBgDialog";
import { chatKey, useChatBg } from "@/app/chatBg";
import { PageCards, PagePinsContext, pageLinksIn, pageTargetOf, type PagePins } from "@/app/pages";
import { VersionsContext, versionsOf } from "@/app/preview";
import { sentTexts, useInputHistory } from "@/app/inputHistory";
import { drafted, Reveal } from "@/app/Reveal";
import { DictateButton } from "@/app/Dictate";
import { startCall, takePendingCall, useVoiceCall, useVoiceReady, VoiceBar, VoiceMark, viaVoice } from "@/app/VoiceBar";
import { ChatSearch, markActivePane, ownsFind, type SearchItem } from "@/app/ChatSearch";
import { TranslatableBody, TranslateToggle } from "@/app/translate";
import { takePendingFind } from "@/app/CommandPalette";
import { setHidden, useHidden } from "@/app/hidden";
import { noteChat } from "@/app/recentChats";
import { PinBar, pinThing, pinsChanged, titleOfMessage, usePins } from "@/app/pins";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { isPicture } from "@/app/attachments";

/** Answers already grown in (once each, even if the view is drawn again),
 *  and when this App first had each message. */
const revealed = new Set<string>();
const firstSeen = new Map<string, number>();
import { chatDraftKey, draftOf, keepDraft } from "@/app/drafts";
import { chatQueueKey, keepQueue, queueOf, queuedId, type Queued } from "@/app/queued";
import { hubAtLeast, hubHasHandover } from "@/app/hubVersion";
import { HandoverDivider, handoverEntry, openHandover, useHandovers } from "@/app/Handover";
import { ApprovalStrip } from "@/app/Superagent";

export interface ConversationTarget {
  hubId: string;
  hubName: string;
  threadId: string;
  agent: string;
  // With peer: the whole history between agent and peer (all threads),
  // read-only, instead of one thread.
  peer?: string;
  /** A new conversation's first message goes into this project. */
  project?: string;
  /** Open at this message (older pages load until it is there), lit up,
   *  instead of at the bottom. */
  focus?: string;
  /** Your conversation with agent, and also agent's with these (read-only,
   *  the newest page of each): everything it was asked, by whom. */
  also?: string[];
  // Where Back goes: the node panel it was opened from.
  returnTo?: { page: string; node?: { hubId: string; name: string }; project?: { hubId: string; name: string } };
}

const CONSOLE_PREFIX = "@console:";
const POLL_MS = 3000;

function isConsole(from: string): boolean {
  return from.startsWith(CONSOLE_PREFIX);
}

function clockTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit", hour12: false });
}

function dayTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(locale(), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
}

// A pause this long between two messages gets a divider with the time.
const GAP_MS = 30 * 60 * 1000;

// A task open longer than this is not being worked on any more.
const OPEN_FOR_MS = 12 * 3600 * 1000;

/** What a node reported while working on a task, folded to one line ("3 条
 * 进展") that opens to the list. */
function ProgressFold({ items }: { items: Message[] }) {
  const [open, setOpen] = useState(false);
  return (
    <div className={cn("conv-steps", open && "is-open")}>
      <button type="button" className="conv-steps-toggle" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        <ChevronRight className="conv-steps-chev" />
        {L(`${items.length} 条进展`, items.length === 1 ? "1 progress update" : `${items.length} progress updates`)}
      </button>
      {open && (
        <ul className="conv-steps-list">
          {items.map((p) => <li key={p.id} title={clockTime(p.created_at)}>{p.body}</li>)}
        </ul>
      )}
    </div>
  );
}

/** text cut with "…" to fit on one line of input el (its font and width):
 *  a long suggestion as the placeholder. Without a width yet, 40 characters. */
let fitCanvas: HTMLCanvasElement | null = null;
function fitOneLine(text: string, el: HTMLTextAreaElement | null): string {
  const st = el ? getComputedStyle(el) : null;
  const width = el ? el.clientWidth - parseFloat(st!.paddingLeft || "0") - parseFloat(st!.paddingRight || "0") - 4 : 0;
  const ctx = width > 0 ? (fitCanvas ||= document.createElement("canvas")).getContext("2d") : null;
  if (!ctx || !st) return [...text].length > 40 ? [...text].slice(0, 39).join("") + "…" : text;
  ctx.font = `${st.fontStyle} ${st.fontWeight} ${st.fontSize} ${st.fontFamily}`;
  if (ctx.measureText(text).width <= width) return text;
  const chars = [...text];
  let lo = 0, hi = chars.length;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (ctx.measureText(chars.slice(0, mid).join("") + "…").width <= width) lo = mid; else hi = mid - 1;
  }
  return chars.slice(0, lo).join("") + "…";
}

function isOpen(status: string): boolean {
  return status !== "done" && status !== "failed";
}

/** ⌘ on a Mac, Ctrl elsewhere (the same key the input listens for). */
export function modKey(): string {
  const p = typeof navigator === "undefined" ? "" : navigator.platform || "";
  return /Mac|iPhone|iPad/.test(p) ? "⌘" : "Ctrl+";
}

// Conversations opened before: reopening one shows it at once while it
// refreshes (the shared page memory, saved across restarts in the app; see
// lib/memo). The hub stays the source of truth.
type SeenConversation = { messages: Message[]; more: boolean };
const cacheKey = (t: ConversationTarget) => ["conv", t.hubId, t.threadId || "", t.agent, t.peer || ""].join("\u0000");
/** Where each conversation was left in this window: how far above its
 *  bottom (0 = at the bottom). Coming back puts you there again. */
const leftAt = new Map<string, number>();
function remember(key: string, messages: Message[], more: boolean) {
  rememberMemo<SeenConversation>(key, { messages, more });
}

// A full refresh every this many polls, whatever changed.
const FULL_EVERY = 20;

/** The newest change among messages, less 2 s (unix ms; 0 for none). */
function changedUntil(ms: Message[]): number {
  let t = 0;
  for (const m of ms) t = Math.max(t, new Date(m.updated_at || m.created_at).getTime() || 0);
  return t > 0 ? t - 2000 : 0;
}

/** Whether a is a later copy of the same message than b. */
function newerThan(a: Message, b: Message): boolean {
  const ta = new Date(a.updated_at || a.created_at).getTime();
  const tb = new Date(b.updated_at || b.created_at).getTime();
  if (ta !== tb) return ta > tb;
  // The same message, its worker seen on it since (a long, quiet command:
  // the work goes on).
  return new Date(a.active_at || 0).getTime() > new Date(b.active_at || 0).getTime();
}

/** Messages with changed ones put in (by id), in the order they were sent.
 *  Also how a message just sent goes in: the poll may have brought it
 *  already, and a plain append would show it twice. */
function mergeById(ms: Message[], changed: Message[]): Message[] {
  const by = new Map(ms.map((m) => [m.id, m]));
  // The newer of two copies stays: the answer to a send can come after a
  // poll that already brought the same message further along (taken,
  // answered); it must not put it back to 还没开始 and hide the reply.
  for (const m of changed) {
    const had = by.get(m.id);
    if (had && newerThan(had, m)) continue;
    by.set(m.id, m);
  }
  const at = (m: Message) => new Date(m.created_at).getTime();
  return [...by.values()].sort((a, b) => at(a) - at(b) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

// What an empty chat asks, one picked at random each time: about the
// project when this is its coordinator, else about the agent.
const ASK_PROJECT: [string, string][] = [
  ["你想在「{p}」里做成什么？", "What do you want done in {p}?"],
  ["「{p}」的下一步是什么？", "What's next for {p}?"],
  ["说说「{p}」接下来要做什么，{n} 会分给大家。", "Say what {p} needs next; {n} will hand it out."],
  ["今天想让「{p}」往前走哪一步？", "Which step should {p} take today?"],
];
const ASK_AGENT: [string, string][] = [
  ["想让 {n} 帮你做点什么？", "What should {n} do for you?"],
  ["有什么活，交给 {n} 吧。", "Got something? Hand it to {n}."],
  ["{n} 准备好了，交代第一件事吧。", "{n} is ready. What's the first thing?"],
  ["说说你想做成什么，{n} 来想办法。", "Say what you want done; {n} will work it out."],
  ["今天想和 {n} 一起做点什么？", "What shall you and {n} do today?"],
];

/** Things to ask first (an empty chat offers three; 换一批 shows others). */
const STARTERS_AGENT: [string, string][] = [
  ["带我看看这个项目：它是做什么的，怎么跑起来", "Show me around this project: what it does and how to run it"],
  ["看下我还没提交的改动，有没有问题", "Review my uncommitted changes for problems"],
  ["给这个仓库写一份 AGENTS.md", "Write an AGENTS.md for this repository"],
  ["跑一遍测试，挂了的告诉我为什么", "Run the tests and tell me why any fail"],
  ["找出这个项目里最值得先修的三个问题", "Find the three things in this project most worth fixing first"],
  ["看看最近一周的提交，写一段进展总结", "Read the last week's commits and sum up the progress"],
  ["把 README 里过时的地方改一下", "Fix what is out of date in the README"],
  ["列出依赖里该升级的，说说风险", "List the dependencies worth upgrading, and the risks"],
];
const STARTERS_PROJECT: [string, string][] = [
  ["把这个项目要做的事拆成几步，分给大家", "Break this project into steps and hand them out"],
  ["让每个成员说说自己现在在做什么", "Ask each member what they are working on"],
  ["定一个这周的目标，排好谁先做什么", "Set a goal for this week and who does what first"],
  ["看看大家的进展，有卡住的就帮一把", "Check everyone's progress and help whoever is stuck"],
  ["整理一份这个项目的现状，发我看", "Write up where this project stands"],
  ["找出成员之间重复或冲突的工作", "Find work members are duplicating or clashing on"],
];
const STARTERS_OFF = "myco-starters-off";
const startersOff = () => { try { return localStorage.getItem(STARTERS_OFF) === "1"; } catch { return false; } };

/** An empty chat: the agent's face, large, a question to start with, and
 *  a few things to ask (a click puts one in the box, to send or change). */
function EmptyChat({ agent, name, project, onStarter }: { agent?: Agent; name: string; project?: string; onStarter?: (text: string) => void }) {
  const [pick] = useState(() => Math.random());
  const list = project ? ASK_PROJECT : ASK_AGENT;
  const [zh, en] = list[Math.floor(pick * list.length) % list.length];
  const fill = (t: string) => t.replace("{n}", name).replace("{p}", project || "");
  const starters = project ? STARTERS_PROJECT : STARTERS_AGENT;
  const [page, setPage] = useState(() => Math.floor(pick * starters.length));
  const [off, setOff] = useState(startersOff);
  const shown = [0, 1, 2].map((i) => starters[(page + i) % starters.length]);
  return (
    <div className="conv-empty">
      {agent && <Avatar agent={agent} size={64} badge={false} />}
      <h3>{fill(L(zh, en))}</h3>
      <p>{L("一句话就行，它会自己想办法；在下面打字，回车发出去。", "A sentence is enough; it works out the rest. Type below and press Enter.")}</p>
      {onStarter && !off && (
        <div className="conv-starters" data-testid="chat-starters">
          {shown.map(([z, e]) => (
            <button key={z} type="button" className="conv-starter" onClick={() => onStarter(L(z, e))}>
              <span>{L(z, e)}</span><ArrowUpRight />
            </button>
          ))}
          <div className="conv-starters-foot">
            <button type="button" onClick={() => setPage((n) => n + 3)}>{L("换一批", "Others")}</button>
            <span aria-hidden>·</span>
            <button type="button" onClick={() => { try { localStorage.setItem(STARTERS_OFF, "1"); } catch { /* not kept */ } setOff(true); }}>{L("不再显示", "Don't show again")}</button>
          </div>
        </div>
      )}
    </div>
  );
}

/** Your superagent's empty chat: what it is and what it does for you,
 *  in place of things to ask. */
function SuperagentIntro({ agent }: { agent?: Agent }) {
  const mac = /Mac/i.test(navigator.platform || navigator.userAgent);
  const items: [string, string][] = [
    ["派活、盯进度：一句话说你要什么，它找合适的 agent 去做，卡住了去催，做完回来告诉你。", "Hands out work and follows it: say what you want, it finds the right agent, nudges it when stuck, and reports back."],
    ["开 agent、建项目：在你允许的电脑上开新的 agent，建项目，把合适的 agent 拉进去。", "Starts agents and projects: new agents on computers you allow, new projects, the right agents in them."],
    ["看全局：谁在忙、哪些任务卡住了、各个项目走到哪一步，问它就行。", "Sees the whole workspace: who is busy, what is stuck, where each project stands. Just ask."],
    ["要紧的事先问你：删东西、改权限这类，会先在这里弹一张卡片，你点了「同意」它才做。", "Asks you first about what matters: deleting, changing access. A card shows here and it waits for your OK."],
  ];
  return (
    <div className="conv-empty sa-intro" data-testid="superagent-intro">
      {agent && <Avatar agent={agent} size={64} badge={false} />}
      <h3>{L(`${SHROOM}，你在这个工作区的 superagent`, `${SHROOM}, your superagent in this workspace`)}</h3>
      <p>{L("它替你打理整个工作区，你只要说要什么。", "It looks after the whole workspace for you: just say what you want.")}</p>
      <ul className="sa-intro-list">
        {items.map(([z, e]) => <li key={z}>{L(z, e)}</li>)}
      </ul>
      <p className="sa-intro-foot">{L(`它总是以自己的名义说话，不会冒充你。${mac ? "在哪儿都可以按 ⌘⇧Space 叫它。" : ""}`, `It always speaks as itself, never as you.${mac ? " Press ⌘⇧Space anywhere to call it." : ""}`)}</p>
    </div>
  );
}

/** While a conversation loads for the first time: its shape, not a word.
 *  Only after a moment: a load that is quick shows nothing in between,
 *  instead of a blink of placeholders. */
function LoadingSkeleton({ shell }: { shell: boolean }) {
  const [late, setLate] = useState(false);
  useEffect(() => {
    const t = window.setTimeout(() => setLate(true), 220);
    return () => window.clearTimeout(t);
  }, []);
  if (!late) return <div className="conv-skel-list" aria-busy="true" aria-label={L("正在加载", "Loading")} />;
  if (shell) {
    return (
      <div className="term conv-skel-term" aria-busy="true" aria-label={L("正在加载", "Loading")}>
        {[62, 38, 74].map((w, i) => (
          <div key={i} className="conv-skel-cmd">
            <span className="conv-skel conv-skel-line" style={{ width: "22%" }} />
            <span className="conv-skel conv-skel-line" style={{ width: `${w}%` }} />
          </div>
        ))}
      </div>
    );
  }
  return (
    <div className="conv-skel-list" aria-busy="true" aria-label={L("正在加载", "Loading")}>
      {[{ me: true, w: 46, h: 1 }, { me: false, w: 72, h: 3 }, { me: true, w: 34, h: 1 }, { me: false, w: 58, h: 2 }].map((b, i) => (
        <div key={i} className={cn("conv-skel-row", b.me && "is-me")}>
          <span className="conv-skel conv-skel-bubble" style={{ width: `${b.w}%`, height: 18 + b.h * 18 }} />
        </div>
      ))}
    </div>
  );
}

/**
 * Why a message failed, in words: short beside its time, long under it.
 * Known hub reasons are explained; anything else is shown as it came.
 */
function failureWords(err: string): { short: string; long: string } {
  if (/delivery attempts exhausted/.test(err)) {
    return {
      short: L("没送到", "Not delivered"),
      long: L("没送到：它取走了几次，都没开始处理。它那边的会话可能没在运行，或者卡住了。可以重新发送，或者先看看它在不在线。",
        "Not delivered: it picked this up several times but never started on it. Its session may not be running, or may be stuck. Send it again, or check that it is online first."),
    };
  }
  if (/lease expired|timed out|timeout/i.test(err)) {
    return { short: L("超时了", "Timed out"), long: L(`超时了：${err}`, `Timed out: ${err}`) };
  }
  return { short: L("失败了", "Failed"), long: L(`失败了：${err}`, `Failed: ${err}`) };
}

/** What each run's trace last said (runActivity), across chats while the
 *  App is open, the newest few hundred. */
const knownActivity = new Map<string, { at: number; ended: boolean }>();
function rememberActivity(taskId: string, a: { at: number; ended: boolean }) {
  knownActivity.delete(taskId);
  knownActivity.set(taskId, a);
  if (knownActivity.size > 400) knownActivity.delete(knownActivity.keys().next().value as string);
}

export function ConversationDetail({ target, rows, now, onBack, onNewConversation, onOpen, onOpenExchange, onRefresh, showBack = true, backLabel, role, inserts, headActions, projectTitle, bare }: {
  target: ConversationTarget;
  /** the project this chat is the home of (its coordinator's): the empty
   *  chat asks about it */
  projectTitle?: string;
  /** buttons at the right end of the header (e.g. full screen, close) */
  headActions?: React.ReactNode;
  /** no header: the page around it already says who this is (the
   *  project's home, with its coordinator) */
  bare?: boolean;
  /** what it is in the project (e.g. coordinator), before its description */
  role?: string;
  /** small things placed in the timeline by time (e.g. a task the agent
   *  handed out), between the messages; shown even before the first */
  inserts?: { id: string; at: string; node: React.ReactNode }[];
  rows: AgentRow[];
  now: number;
  onBack: () => void;
  /** Next to the list (split view) there is nothing to go back to, unless
   *  the conversation was opened from a node. */
  showBack?: boolean;
  /** what the back button says (where it goes) */
  backLabel?: string;
  // Starts a new conversation of yours with the agent (from a read-only thread).
  onNewConversation: (hubId: string, agent: string) => void;
  /** Opens your conversation with another node (a copy just made). */
  onOpen?: (hubId: string, agent: string, peer: string) => void;
  /** a handoff card: what agent and other said to each other, at that
   *  message (read-only) */
  onOpenExchange?: (agent: string, other: string, focus?: string) => void;
  /** The node's work changed outside this conversation (a command on its
   *  computer was stopped): refresh the rows it came from. */
  onRefresh?: () => void;
}) {
  // Seen before in this window: shown from the first frame (no empty
  // flash while the cache is read in an effect).
  const [messages, setMessages] = useState<Message[] | null>(() => recall<SeenConversation>(cacheKey(target))?.messages ?? null);
  // Exporting this conversation (Markdown or JSON Lines, to Downloads).
  const [exporting, setExporting] = useState(false);
  const [filesOpen, setFilesOpen] = useState(false);
  const [bgOpen, setBgOpen] = useState(false);
  const headMenu = useContextMenu();
  // (a reply done growing in shows its attachments: one more render)
  const [, setRevealedTick] = useState(0);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [sendError, setSendError] = useState<unknown>(null);
  // What was typed and not sent stays with the conversation (drafts.ts).
  const draftKey = chatDraftKey(target.hubId, target.agent);
  const [draft, setDraft] = useState(() => draftOf(draftKey));
  // A voice chat (VoiceBar): its bar is here when it is with this agent;
  // it goes on when you go elsewhere.
  const { call: voiceCall } = useVoiceCall();
  const voiceOn = !!voiceCall && voiceCall.hubId === target.hubId && voiceCall.agent === target.agent;
  const draftKeyRef = useRef(draftKey);
  draftKeyRef.current = draftKey;
  const writeDraft = (v: string) => {
    setDraft(v);
    keepDraft(draftKeyRef.current, v);
  };
  const [sending, setSending] = useState(false);
  // The message on its way: shown at once at the end of the chat, faint,
  // with 发送中, and the box empty again; the hub's copy takes its place.
  // (One put after the running round, ⌘↩, shows in the queue above the box
  // at once instead: it is queued, not sending.)
  const [pending, setPending] = useState<{ body: string; files: { id: string; name: string; size: number; preview?: string }[]; queued?: boolean } | null>(null);
  // Files going with the next message (pasted, dropped or picked).
  const att = useAttachments(target.hubId);
  // Messages put after the running one, while the workspace cannot hold
  // them (older hubs): kept here, like drafts, and sent when the round ends.
  const queueKey = chatQueueKey(target.hubId, target.agent);
  const queueKeyRef = useRef(queueKey);
  queueKeyRef.current = queueKey;
  const [localQueue, setLocalQueue] = useState<Queued[]>(() => queueOf(queueKey));
  const writeQueue = (items: Queued[]) => {
    setLocalQueue(items);
    keepQueue(queueKeyRef.current, items);
  };
  // Which answers came while you were looking (they grow in, Reveal.tsx):
  // not the ones there when the conversation opened, nor ones loaded in its
  // first moments.
  const arrived = useRef<{ key: string; at: number; first: Set<string> | null }>({ key: "", at: 0, first: null });
  /** Queued messages withdrawn here (a stopped one may still be polled back). */
  const [withdrawn, setWithdrawn] = useState<ReadonlySet<string>>(() => new Set());
  const [queueError, setQueueError] = useState<unknown>(null);
  const [queueBusy, setQueueBusy] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  // grow with the text, up to about eight lines, then scroll
  useAutoGrow(inputRef, draft, 200);
  const atBottom = useRef(true);
  // How far above the bottom the view is, and where to put it back when a
  // conversation left higher up opens again.
  const belowNow = useRef(0);
  const restoreBelow = useRef(0);
  // (kept while what it shows loads, a moment: until then the view is put
  // back there each time the conversation grows)
  const restoreUntil = useRef(0);
  // (anything that sends the view to the bottom ends it: atBottom)
  const restoring = () => restoreBelow.current > 0 && !atBottom.current && Date.now() < restoreUntil.current;
  const putBack = (el: HTMLElement) => { el.scrollTop = el.scrollHeight - el.clientHeight - restoreBelow.current; };
  // When you last scrolled yourself (wheel, touch, a click on the
  // scrollbar, a key): only that leaves the bottom. A scroll the browser
  // makes as content loads above (Chromium keeps what is on screen in
  // place) does not.
  const userScrollAt = useRef(0);
  const pointerHeld = useRef(false);
  const markUserScroll = () => { userScrollAt.current = Date.now(); };
  // A jump made on purpose (to a message, to a stuck task) scrolls smoothly
  // for a moment: that too is yours, not content moving the view.
  const jumpUntil = useRef(0);
  const markJump = () => { jumpUntil.current = Date.now() + 1500; };
  useEffect(() => {
    const up = () => { pointerHeld.current = false; };
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    return () => { window.removeEventListener("pointerup", up); window.removeEventListener("pointercancel", up); };
  }, []);
  // When the box below grows (a line, files), the conversation gets
  // shorter: if it was at the bottom, it stays there instead of the last
  // lines sliding under the box.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    let last = el.clientHeight;
    const ro = new ResizeObserver(() => {
      const h = el.clientHeight;
      if (h !== last && atBottom.current) el.scrollTop = el.scrollHeight;
      last = h;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const focusDone = useRef(false);
  const focusTries = useRef(0);

  const row = rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.agent);
  const agent = row?.st.agent;
  const agentName = agent && row ? nodeName(agent, row.hub) : target.agent;
  // Your superagent: no way "back" (it is not in a list you came from),
  // and its empty chat says what it is.
  const isSa = useIsSuperagent({ id: agent?.id || "", name: target.agent });
  const callHim = () => startCall({ hubId: target.hubId, agent: target.agent, peer: target.peer, agentName });
  const callHimRef = useRef(callHim);
  callHimRef.current = callHim;
  // A call asked for from the project's box starts when this opens.
  useEffect(() => { if (takePendingCall(target.hubId, target.agent)) callHimRef.current(); }, [target.hubId, target.agent, target.peer]);

  const exchange = !!target.peer;
  // Exchange view: older pages loaded so far (oldest first) and whether more exist.
  const [older, setOlder] = useState<Message[]>([]);
  const [more, setMore] = useState(() => recall<SeenConversation>(cacheKey(target))?.more ?? false);
  const [loadingOlder, setLoadingOlder] = useState(false);

  // When the hub last answered this conversation's poll (and when it was
  // opened, until it does). On a poor network polls fail or hang without a
  // word: what shows is then older than it looks, and a task that seems
  // quiet may well be at work. Quiet is counted only up to the last answer,
  // and the chat says it is behind instead of calling the task stuck.
  const syncedAt = useRef(0);
  const syncOpened = useRef(Date.now());
  const repoll = useRef<() => Promise<void>>(async () => undefined);
  const [repolling, setRepolling] = useState(false);
  useEffect(() => {
    let cancelled = false;
    syncedAt.current = 0;
    syncOpened.current = Date.now();
    // Seen before in this window: show it at once, refresh behind it.
    const key = cacheKey(target);
    const hit = recall<SeenConversation>(key);
    setMessages(hit ? hit.messages : null);
    setLoadError(null);
    setSendError(null);
    setDraft(draftOf(chatDraftKey(target.hubId, target.agent)));
    setLocalQueue(queueOf(chatQueueKey(target.hubId, target.agent)));
    setQueueError(null);
    setFlushStalled(false);
    setWithdrawn(new Set());
    setOlder([]);
    setMore(hit?.more ?? false);

    focusDone.current = false;
    focusTries.current = 0;
    // Polls ask only for what changed since the newest change they have
    // (hubs from v0.18.35; 2 s back, for changes in the same moment); every
    // FULL_EVERY-th poll, and on older hubs, the whole view again.
    // A new conversation (no thread, no peer yet): nothing to load until the
    // first message, which then opens the whole conversation.
    if (!target.peer && !target.threadId) {
      setMessages([]);
      return () => { cancelled = true; };
    }
    let current: Message[] = hit ? hit.messages : [];
    let more = hit?.more ?? false;
    let since = hit ? changedUntil(hit.messages) : 0;
    let polls = 0;
    let incremental = true;
    // A poll that brings nothing new changes nothing on screen (no new
    // array, no re-render, no scroll).
    const sameAs = (a: Message[], b: Message[]) => a.length === b.length
      && a.every((x, i) => x.id === b[i].id && x.status === b[i].status && x.updated_at === b[i].updated_at && x.body === b[i].body);
    // (Only once something is on screen: the first answer, even an empty
    // one, always replaces the loading state.)
    let onScreen = !!hit;
    const show = (ms: Message[], m: boolean) => {
      const same = onScreen && sameAs(current, ms) && more === m;
      onScreen = true;
      current = ms;
      more = m;
      since = changedUntil(ms);
      setLoadError(null);
      if (same) return;
      setMessages(ms);
      remember(key, ms, m);
    };
    const run = async (silent: boolean) => {
      polls++;
      const delta = silent && incremental && since > 0 && polls % FULL_EVERY !== 0;
      try {
        if (target.peer) {
          // The newest page, polled; older pages stay as loaded. With also,
          // the newest page of each of those is merged in once it comes:
          // yours shows first, the others do not hold it up.
          const extra = target.also?.length
            ? Promise.all(target.also.map((p) => appApi.exchanges(target.hubId, target.agent, p, undefined, delta ? since : undefined)
              .then((x) => x.messages).catch(() => [] as Message[])))
            : null;
          const mergeExtra = () => extra?.then((lists) => {
            const others = lists.flat();
            if (!cancelled && others.length) show(mergeById(current, others), more);
          });
          const r = await appApi.exchanges(target.hubId, target.agent, target.peer, undefined, delta ? since : undefined);
          if (cancelled) return;
          syncedAt.current = Date.now();
          if (delta && r.since) {
            // Many changes at once: the whole page next time.
            if (r.more) polls = FULL_EVERY - 1;
            if (r.messages.length) show(mergeById(current, r.messages), more);
            void mergeExtra();
            return;
          }
          if (delta) incremental = false; // an older hub: whole pages
          // A whole page replaces what was shown, keeping what the others
          // said (merged in again just after).
          const mine = [...r.messages].reverse();
          const keep = target.also?.length ? current.filter((m) => target.also!.includes(m.from) || target.also!.includes(m.to)) : [];
          show(keep.length ? mergeById(mine, keep) : mine, silent ? more : r.more);
          void mergeExtra();
          if (!silent) setMore(r.more);
          return;
        }
        const r = await appApi.thread(target.hubId, target.threadId, delta ? since : undefined);
        if (cancelled) return;
        syncedAt.current = Date.now();
        if (delta && r.since) {
          if (r.messages.length) show(mergeById(current, r.messages), false);
          return;
        }
        if (delta) incremental = false;
        show(r.messages, false);
      } catch (e) {
        if (!cancelled && !silent) setLoadError(e);
      }
    };
    void run(false);
    repoll.current = () => run(true);
    const poll = window.setInterval(() => {
      if (document.visibilityState === "visible") void run(true);
    }, POLL_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") void run(true);
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      cancelled = true;
      window.clearInterval(poll);
      document.removeEventListener("visibilitychange", onVisible);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target.hubId, target.threadId, target.agent, target.peer, (target.also || []).join(",")]);

  const loadOlder = async () => {
    const first = older[0] || messages?.[0];
    if (!target.peer || !first || loadingOlder) return;
    setLoadingOlder(true);
    const el = scrollRef.current;
    const fromBottom = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const r = await appApi.exchanges(target.hubId, target.agent, target.peer, new Date(first.created_at).getTime());
      setOlder((prev) => [...[...r.messages].reverse(), ...prev]);
      setMore(r.more);
      // Keep the view where it was.
      requestAnimationFrame(() => { if (el) el.scrollTop = el.scrollHeight - fromBottom; });
    } catch (e) {
      setLoadError(e);
    } finally {
      setLoadingOlder(false);
    }
  };

  // Before the frame is drawn: opening a chat never shows its top first
  // and then jumps to the newest. Later, only something new (another
  // message, a changed one) brings it down, and only if it was at the
  // bottom: a refresh with nothing new never moves what you are reading.
  const lastSeen = useRef("");
  // Opened at a message: stay there instead of following the bottom; left
  // higher up before: back there; else at the bottom. (Before the effect
  // below, which places the view as soon as the messages are drawn.)
  useLayoutEffect(() => {
    const was = leftAt.get(cacheKey(target)) ?? 0;
    belowNow.current = was;
    atBottom.current = !target.focus && was < 80;
    restoreBelow.current = !target.focus && was >= 80 ? was : 0;
    restoreUntil.current = Date.now() + 1500;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target.hubId, target.threadId, target.agent, target.peer, target.focus]);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    const last = messages?.[messages.length - 1];
    const sig = `${messages?.length ?? -1}\u0000${last?.id ?? ""}\u0000${last?.updated_at ?? ""}`;
    const changed = sig !== lastSeen.current;
    lastSeen.current = sig;
    if (el && restoring() && messages?.length) {
      putBack(el);
      return;
    }
    if (el && changed && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages]);
  // Leaving a conversation: remember where it was.
  useEffect(() => {
    const key = cacheKey(target);
    // (left while still being put back: that place, not where it is yet)
    return () => { leftAt.set(key, restoreBelow.current > 0 ? restoreBelow.current : atBottom.current ? 0 : belowNow.current); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target.hubId, target.threadId, target.agent, target.peer]);
  // What loads after the messages (a finished task's steps, a picture)
  // makes the conversation taller above the bottom: at the bottom, it
  // stays at the bottom instead of the last lines being pushed down.
  const timelineRef = useRef<HTMLDivElement>(null);
  // When something in the conversation was last opened by a click (see below).
  const openedAt = useRef(0);
  // Each finished run's time, tokens and cost (from its trace), shown next
  // to its answer's time and copy instead of under the steps.
  const [usageOf, setUsageOf] = useState<Record<string, string>>({});
  const onUsage = (taskId: string, line: string) =>
    setUsageOf((u) => (u[taskId] === line ? u : { ...u, [taskId]: line }));
  // Trace polls run twice as often as the conversation; use their status
  // immediately to choose the single moving run after it is picked up.
  const [traceStatuses, setTraceStatuses] = useState<ReadonlyMap<string, string>>(() => new Map());
  const onTraceStatus = useCallback((taskId: string, status: string) =>
    setTraceStatuses((prev) => prev.get(taskId) === status ? prev : new Map(prev).set(taskId, status)), []);
  // Messages taken out of your own view (删除), and the last one taken,
  // for a moment, to take back (撤销).
  const hiddenIds = useHidden(target.hubId);
  // With nothing typed, the round button starts a voice chat with this
  // agent (the Mac app; VoiceBar).
  const voice = useVoiceReady();
  const [undo, setUndo] = useState<{ ids: string[]; n: number } | null>(null);
  useEffect(() => {
    if (!undo) return;
    const t = window.setTimeout(() => setUndo((u) => (u?.n === undo.n ? null : u)), 6000);
    return () => window.clearTimeout(t);
  }, [undo]);
  const removeMessage = (m: Message) => {
    setUndo({ ids: [m.id], n: Date.now() });
    void setHidden(target.hubId, [m.id], true).catch((e) => { setUndo(null); setSendError(e); });
  };
  // What each run's trace says (TraceView's pausedRun): stopped (turn over,
  // set aside, silent) or at work. Not known until its trace is read.
  // Kept while the App is open (knownActivity): back in a chat, a task that
  // was at work a moment ago is not shown as quiet (可能卡住) for the second
  // its trace takes to be read again.
  const [runActivity, setRunActivity] = useState<ReadonlyMap<string, { at: number; ended: boolean }>>(() => new Map(knownActivity));
  const onActivity = useCallback((taskId: string, at: number, ended: boolean) => {
    rememberActivity(taskId, { at, ended });
    setRunActivity((s) => {
      const o = s.get(taskId);
      return o && o.at === at && o.ended === ended ? s : new Map(s).set(taskId, { at, ended });
    });
  }, []);
  const [runPaused, setRunPaused] = useState<ReadonlyMap<string, boolean>>(() => new Map());
  const onPaused = useCallback((taskId: string, paused: boolean) =>
    setRunPaused((s) => (s.get(taskId) === paused ? s : new Map(s).set(taskId, paused))), []);
  useEffect(() => {
    const el = timelineRef.current;
    const sc = scrollRef.current;
    if (!el || !sc || typeof ResizeObserver === "undefined") return;
    let h = el.offsetHeight;
    // Something you opened in the conversation (展开, a run's steps, a
    // fold): it grows where you are reading, so the view stays put instead
    // of following the bottom, and is no longer at the bottom after it.
    const onClick = (e: MouseEvent) => {
      const t = e.target as Element | null;
      if (t?.closest?.("button, summary, details, [role=button], [aria-expanded]")) openedAt.current = Date.now();
    };
    el.addEventListener("click", onClick, true);
    const ro = new ResizeObserver(() => {
      const nh = el.offsetHeight;
      if (nh > h && Date.now() - openedAt.current < 1000) {
        atBottom.current = sc.scrollHeight - sc.scrollTop - sc.clientHeight < 80;
        restoreBelow.current = 0;
      } else if (nh !== h && restoring()) putBack(sc);
      else if (nh !== h && atBottom.current) sc.scrollTop = sc.scrollHeight;
      h = nh;
    });
    ro.observe(el);
    return () => { ro.disconnect(); el.removeEventListener("click", onClick, true); };
  }, [messages === null]);

  // Finding a message (ChatSearch): ⌘F in the chat last clicked in.
  const rootRef = useRef<HTMLDivElement>(null);
  const [searching, setSearching] = useState(false);
  const [findFirst, setFindFirst] = useState("");
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.key.toLowerCase() !== "f") return;
      if (document.querySelector(".app-modal, .fp-panel, .lightbox") || !ownsFind(rootRef.current)) return;
      e.preventDefault();
      setSearching(true);
      // Already open: back to its box.
      window.setTimeout(() => rootRef.current?.querySelector<HTMLInputElement>(".conv-search input")?.select(), 0);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  useEffect(() => { setSearching(false); }, [target.hubId, target.agent, target.peer, target.threadId]);

  // Scrolled well up: a button back to the bottom, with how many new
  // messages came since you left it.
  const [away, setAway] = useState(false);
  const awaySince = useRef(0);
  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    const below = el.scrollHeight - el.scrollTop - el.clientHeight;
    const yours = pointerHeld.current || Date.now() - userScrollAt.current <= 300 || Date.now() < jumpUntil.current;
    if (yours) restoreBelow.current = 0; // you took over
    else if (restoring()) {
      putBack(el);
      const far0 = restoreBelow.current > 400;
      if (far0 !== away) {
        if (far0) awaySince.current = Date.now();
        setAway(far0);
      }
      return;
    }
    if (below >= 80 && atBottom.current && !yours && Date.now() - openedAt.current >= 1000) {
      // Not you: content moved the view. At the bottom it stays.
      el.scrollTop = el.scrollHeight;
      return;
    }
    atBottom.current = below < 80;
    belowNow.current = below;
    const far = below > 400;
    if (far !== away) {
      if (far) awaySince.current = Date.now();
      setAway(far);
    }
  };
  const toBottom = () => {
    const el = scrollRef.current;
    if (!el) return;
    atBottom.current = true;
    restoreBelow.current = 0;
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  // A message put after a task that is still open waits on the hub: it is
  // not part of the conversation yet (hubs from v0.19.2 hold it; older hubs
  // never see one). It is shown above the box until that task ends.
  const known = new Map((messages || []).map((m) => [m.id, m]));
  const afterOpen = (m: Message): boolean => {
    if (!m.after || !isOpen(m.status)) return false;
    const task = known.get(m.after);
    return !!task && task.type === "task" && isOpen(task.status);
  };
  // A message put behind another (⌘↩) and stopped before it went out was
  // never sent: it is not in the conversation (not 已停止 there).
  const answeredIds = new Set((messages || []).map((m) => m.reply_to).filter(Boolean) as string[]);
  const neverSent = (m: Message): boolean => m.type === "task" && !!m.after && m.status === "failed"
    && m.error === STOPPED && isMe(m.from, row?.hub) && !answeredIds.has(m.id);
  // (a task taken out of view takes its progress and supplements with it;
  // its answer stays until taken out too)
  const hiddenMessage = (m: Message): boolean => withdrawn.has(m.id) || afterOpen(m) || neverSent(m) || hiddenIds.has(m.id)
    || ((m.type === "progress" || m.type === "steer") && !!m.reply_to && hiddenIds.has(m.reply_to));
  const hubQueue = (messages || []).filter((m) => afterOpen(m) && !withdrawn.has(m.id));

  // Everything shown: in the exchange view, older pages before the newest.
  const alsoSet = new Set(target.also || []);
  // In a member's chat (a project): what it and the other members said to
  // each other is not drawn as messages here (it would read as if it were
  // talking to you), only as a handoff card per exchange (Handoff.tsx).
  // (between agents only: a person writing to it is a message, named)
  const person = (n: string) => {
    if (n.startsWith("@")) return true;
    const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === n);
    return !!r && agentKind(r.st.agent) === "human";
  };
  const sideOf = (m: Message) => alsoSet.size > 0 && !isMe(m.from, row?.hub) && !isMe(m.to, row?.hub)
    && (alsoSet.has(m.from) || alsoSet.has(m.to)) && !person(m.from) && !person(m.to);
  const loaded: Message[] | null = messages === null ? null : (exchange
    ? [...older.filter((o) => !messages.some((m) => m.id === o.id)), ...messages]
    : messages).filter((m) => !hiddenMessage(m));
  const shown: Message[] | null = loaded && (alsoSet.size ? loaded.filter((m) => !sideOf(m)) : loaded);
  const handoffs = alsoSet.size && loaded
    ? withStale(handoffsFrom(loaded.filter(sideOf), target.agent), (id) => rows.some((x) => x.hub.id === target.hubId && (x.st.stale_tasks || []).includes(id)))
    : [];

  const consoleTasks = (messages || []).filter((m) => m.type === "task" && isMe(m.from, row?.hub) && !hiddenMessage(m));
  const lastTask = consoleTasks[consoleTasks.length - 1];
  // An answer names its task (reply_to); one that does not (older runners)
  // counts when it came after the task in the same thread. Replies to other
  // tasks still open here (say, one just resent) do not close this one.
  const hasAnswer = (task: Message) => (messages || []).some((m) =>
    (m.type === "reply" || m.type === "event") && m.id !== task.id &&
    (m.reply_to ? m.reply_to === task.id
      : m.thread_id === task.thread_id && new Date(m.created_at).getTime() >= new Date(task.created_at).getTime()),
  );
  // At work, by one rule for the loading and the box: not stopped by its
  // trace; one set aside (held) only once its trace says it moved again.
  const atWork = (m: Message): boolean => {
    const p = runPaused.get(m.id);
    return (traceStatuses.get(m.id) || m.status) === "held" ? p === false : p !== true;
  };
  // A task it set aside (held: waiting on someone else, it answers later),
  // or one it stopped working on (its turn is over, pausedRun) does not
  // take what you type: that is a new message, as in a chat, and there is
  // nothing to stop. One held on a question to you does: what you type
  // answers it.
  const openTask = [...consoleTasks].reverse().find((task) =>
    isOpen(task.status) && (atWork(task) || !!task.asking) && !hasAnswer(task)) || null;

  // Only an event after the newest task says something about this task; an
  // old failure should not paint a later, healthy conversation red.
  const afterTask = lastTask
    ? (messages || []).filter((m) => new Date(m.created_at).getTime() >= new Date(lastTask.created_at).getTime())
    : [];
  const stopped = lastTask?.status === "failed" && lastTask.error === STOPPED;
  const failed = lastTask?.status === "failed" || afterTask.some((m) => m.type === "event");
  const busy = !!row && sessionBusy(row.st);
  const offline = row?.st.state === "offline";
  // A task picked up long ago and quiet since: its result may never come
  // back. Said in the chat, with a way out, instead of a spinner.
  // When a task of yours last showed signs of life: the task itself, what
  // it reported (progress) and its steps (TraceView says). And whether it
  // ended its turn or set the task aside: then it waits (for you, or for
  // others), it is not stuck.
  const saidAt = new Map<string, number>();
  for (const m of messages || []) {
    if (m.type !== "progress" || !m.reply_to) continue;
    const t = new Date(m.created_at).getTime();
    if (t > (saidAt.get(m.reply_to) || 0)) saidAt.set(m.reply_to, t);
  }
  const activeAt = (m: Message) => Math.max(lastActivityAt(m) || 0, saidAt.get(m.id) || 0, runActivity.get(m.id)?.at || 0);
  // The hub says which tasks show no life (hubs from v0.20.28): left open
  // after the work ended, or their worker went away.
  const hubStale = new Set(row?.st.stale_tasks || []);
  // (what shows reaches as far as the last answer: see syncedAt)
  const seenAt = syncedAt.current || syncOpened.current;
  const behind = !!(target.peer || target.threadId) && now - seenAt > SEEN_SLACK_MS;
  const isStuck = (m: Message): boolean => {
    const offline = row?.st.state === "offline";
    if (hubStale.has(m.id)) return true;
    if (!offline && runActivity.get(m.id)?.ended) return false;
    return possiblyStuck({ ...m, active_at: new Date(activeAt(m)).toISOString() }, { agent: row?.st.agent, receiving: row?.st.receiving, offline, seenAt }, now);
  };
  const stuckLine = (m: Message): string => {
    const quiet = quietFor(seenNow(now, seenAt), activeAt(m));
    return row?.st.state === "offline"
      ? L(`${quiet}没有动静，它也不在线：这条任务没收尾`, `No activity for ${quiet} and it is offline: this task was left open`)
      : L(
        `${quiet}没有动静：可能做完了没收尾，也可能卡住了`,
        `No activity for ${quiet}: it may be done but left open, or stuck`,
      );
  };
  // Every task of yours still open in this conversation, not only the newest:
  // an older one that went quiet may be stuck too. (One waiting behind the
  // running task is not in the conversation yet.)
  const openTasks = consoleTasks.filter((m) => isOpen(m.status));
  const allStuck = openTasks.length > 0 && openTasks.every(isStuck);
  // The header's 可能卡住 takes you to the (first) stuck task: in a long
  // conversation it can be far above the bottom you are looking at.
  const firstStuck = openTasks.find(isStuck);
  const toStuck = () => {
    const el = firstStuck && scrollRef.current?.querySelector<HTMLElement>(`[data-stuck="${firstStuck.id}"]`);
    if (!el) return;
    // Taken up there on purpose: what grows at the bottom must not pull
    // the view back down (nor a place being put back).
    atBottom.current = false;
    restoreBelow.current = 0;
    markJump();
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    el.classList.remove("is-flash");
    void el.offsetWidth; // restart the highlight
    el.classList.add("is-flash");
  };
  const state = busy
    ? { dot: "pending", label: L("会话被占用", "Session in use") }
    : allStuck
    ? { dot: "pending", label: L("可能卡住", "May be stuck") }
    : openTask?.asking
    ? { dot: "pending", label: L("等你决定", "Needs your decision") }
    : openTask && offline
    ? { dot: "idle", label: L("离线 · 等它上线", "Offline · waiting") }
    : openTask && openTask.status !== "acked"
    ? { dot: "pending", label: L("等待中", "Waiting") }
    : openTask
    ? { dot: "working", label: L("处理中", "Working") }
    // At work on something someone else gave it.
    : row?.st.state === "working" && (!exchange || !!target.peer && (isMe(target.peer, row?.hub) || isMe(target.agent, row?.hub)))
    ? { dot: "working", label: target.also?.length ? L("在干活", "Working") : L("在忙别的", "Busy with something else") }
    : stopped
      ? { dot: "idle", label: L("已停止", "Stopped") }
    : failed
      ? { dot: "failed", label: L("失败", "Failed") }
      : lastTask
        ? { dot: "idle", label: L("已完成", "Done") }
        : { dot: "idle", label: L("空闲", "Idle") };
  // An older hub cannot hold a queued message: the App sends it once the
  // round it waits for has ended (all of them, in order).
  const [flushStalled, setFlushStalled] = useState(false);
  const flushing = useRef(false);
  useEffect(() => {
    if (openTask || flushStalled || flushing.current || localQueue.length === 0) return;
    flushing.current = true;
    const items = localQueue;
    // The box may move to another conversation while this runs: its queue is
    // this one's, and its messages belong in this one's flow.
    const flushKey = chatQueueKey(target.hubId, target.agent);
    const here = () => queueKeyRef.current === flushKey;
    (async () => {
      let done = 0;
      try {
        for (; done < items.length; done++) {
          const it = items[done];
          const send = (thread_id?: string) => appApi.sendMessage(target.hubId, {
            to: target.agent, body: it.body, thread_id, attachments: it.attachments,
          });
          let m: Message;
          try {
            m = await send(it.threadId);
          } catch (e) {
            // The thread was closed or renamed under us: the message still goes.
            if (!it.threadId || (e as { code?: string }).code !== "not_found") throw e;
            m = await send(undefined);
          }
          if (here()) setMessages((prev) => mergeById(prev || [], [m]));
        }
        keepQueue(flushKey, []);
        if (here()) setLocalQueue([]);
      } catch (e) {
        keepQueue(flushKey, items.slice(done));
        if (here()) {
          setQueueError(e);
          setFlushStalled(true);
          setLocalQueue(items.slice(done));
        }
      } finally {
        flushing.current = false;
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openTask?.id, localQueue, flushStalled, target.hubId, target.agent]);
  // The runner found the agent's session open elsewhere (a terminal, an
  // editor, the Codex app): it waits and retries rather than write into it.
  // Why a task has not started: said in the chat instead of a spinner.
  const waitingReason = busy
    ? L("对方的会话正开在别处（终端、编辑器或桌面版），空出来后会处理", "The session is open elsewhere (a terminal, an editor or a desktop app); it starts when that is free")
    : offline
      ? agent && isInbox(agent)
        ? L("对方只在会话里接收消息，等它的会话再挂上 inbox wait 就会处理", "The node receives only in its session; it starts when its session runs inbox wait again")
        : L("对方离线，上线后会处理", "The node is offline; it starts when the node is back")
      : row && runnerAway(row.st, row.hub)
        ? L("它那台电脑上的 myco 没在运行（可能在重启），恢复后会接着做", "The node's runner is not running (maybe restarting); it starts when the runner is back")
        : undefined;
  // The same, in a few words: under your message that waits (the full
  // sentence when the pointer is on it).
  const waitingShort = busy ? L("会话开在别处，空出来再看", "Its session is busy elsewhere")
    : offline ? L("对方离线，上线后再看", "Offline; it reads it when back")
    : waitingReason ? L("对方的 myco 没在运行，恢复后再看", "Its runner is not running")
    : "";

  // Busy elsewhere: a copy of the session (a new node on the same machine
  // and folder) can take the work now; the original stays as it is.
  const machine = row ? machineOfRow(row) : undefined;
  // An inbox node has no CLI session to copy and no runner to start it.
  const forkable = !!agent && !isInbox(agent) && (agent.binding.harness === "claude" || agent.binding.harness === "codex") &&
    !!agent.binding.session_id && !!agent.binding.workdir && !!machine?.can_spawn && canManage(row?.hub, machine?.owner_id);
  const [copying, setCopying] = useState(false);
  const [copyError, setCopyError] = useState<unknown>(null);
  const copyAndContinue = async () => {
    if (!agent || !machine || !row) return;
    setCopying(true);
    setCopyError(null);
    try {
      const copy = await forkAgent(row);
      // The waiting task moves to the copy.
      if (openTask) {
        await appApi.stop(target.hubId, openTask.id).catch(() => {});
        await appApi.sendMessage(target.hubId, { to: copy, body: openTask.body });
      }
      const me = row.hub.me?.member ? CONSOLE_PREFIX + row.hub.me.member.name : target.peer || "";
      onOpen?.(target.hubId, copy, target.peer || me);
    } catch (e) {
      setCopyError(e);
    } finally {
      setCopying(false);
    }
  };

  // A thread you take no part in (two nodes working together) is only
  // shown: writing into it would land in their conversation, not yours.
  const observed = exchange
    ? !isMe(target.agent, row?.hub) && !isMe(target.peer || "", row?.hub)
    : !!messages && messages.length > 0 && !messages.some((m) => isMe(m.from, row?.hub) || isMe(m.to, row?.hub));
  // Your chat with it, opened: one of the recent ones (⌘K).
  useEffect(() => {
    if (exchange && !observed && target.agent && !target.agent.startsWith("@")) noteChat(target.hubId, target.agent);
  }, [target.hubId, target.agent, exchange, observed]);
  const parties = exchange
    ? [target.agent, target.peer!]
    : observed && messages
      ? [...new Set(messages.flatMap((m) => [m.from, m.to]))].filter((n) => n && !n.startsWith("@"))
      : [];

  // With an exec (shell) node on one side, the messages read as a terminal.
  const isExecNode = (n?: string) => !!n && rows.some((x) => x.hub.id === target.hubId && x.st.agent.name === n && x.st.agent.binding.harness === "exec");
  const execNode = [target.agent, target.peer, ...parties].find((n) => isExecNode(n));
  // Your own session with a shell node: you type at a prompt, as in a terminal.
  const shellMode = !observed && isExecNode(target.agent);
  // The other side is one of your computers (a folder on one): its header
  // offers its settings.
  const computer = !observed && !!row && isWorkspace(row.st.agent);
  // Another session took it over (「换一个来接着做」): a line where it did,
  // and the header offers it to whoever may.
  const handoverOk = !observed && !!row && agentKind(row.st.agent) === "agent" && hubHasHandover(row.hub.hub_version);
  const handovers = useHandovers(target.hubId, target.agent, handoverOk);
  const hoEntry = handoverOk ? handoverEntry(row || undefined) : { show: false, why: undefined };
  // What the agent offers you to say next (on your newest task, once it is
  // not at work any more): the empty input shows it, Tab takes it.
  const suggestion = !shellMode && !openTask && lastTask?.suggest?.trim() && !lastTask.unreadable ? lastTask.suggest.trim() : "";

  // Enter adds to the running task; ⌘/Ctrl+Enter puts the next one behind
  // it. A hub from v0.19.2 holds both (a steer may carry files, a queued
  // message knows what it waits for); older hubs get the text as a steer,
  // and the queue is kept in the App until the round ends.
  const queueOnHub = hubAtLeast(row?.hub.hub_version, "0.19.2");
  const supplementing = !shellMode && !!openTask;
  const mod = modKey();
  // Either way (typing at it, or reading what other nodes ran on it) the
  // pane is a terminal.
  const termMode = shellMode || !!execNode;
  // 聊天背景: this conversation's own, else the default (not a terminal).
  const chatBg = useChatBg(termMode ? "" : chatKey(target.hubId, target.agent));

  // 置顶 (pins.tsx): what this conversation keeps at hand, above it; the
  // same list for both parties (hubs from v0.20.3). Not in a terminal.
  const pinState = usePins(target.hubId, termMode ? "" : target.agent, target.peer);
  const pins = pinState.supported ? pinState.pins || [] : null;
  const [pinAdding, setPinAdding] = useState(false);
  const pinMenu = useContextMenu();
  const pinOf = (m: Message) => pins?.find((p) => p.kind === "message" && p.target === m.id);
  const pinMessage = async (m: Message, el: Element) => {
    if (!pins) return;
    const files = (m.attachments || []).filter((f) => !isPicture(f));
    const one = (body: () => Promise<unknown>) => void body().catch((e) => setSendError(e));
    const asMessage = () => one(() => pinThing(target.hubId, { with: target.agent, ...(target.peer ? { a: target.peer } : {}), kind: "message", target: m.id, title: titleOfMessage(m.body) || L("一条消息", "A message") }));
    const done = pinOf(m);
    if (done) {
      one(async () => { await appApi.removePin(target.hubId, done.id, target.agent, target.peer); pinsChanged(target.hubId); });
      return;
    }
    if (!files.length) { asMessage(); return; }
    // A message with files: the message, or one of its files.
    pinMenu.openUnder(el, [
      { label: L("置顶这条消息", "Pin this message"), onSelect: asMessage },
      "sep",
      ...files.map((f) => ({ label: L(`置顶文件「${f.name}」`, `Pin the file “${f.name}”`), onSelect: () => one(() => pinThing(target.hubId, { with: target.agent, ...(target.peer ? { a: target.peer } : {}), kind: "file", file: f, title: f.name })) })),
    ]);
  };
  // A page's card (in a message or a progress update): right-click pins
  // its link above the conversation, or takes it off.
  const pagePinOf = (id: string) => pins?.find((p) => {
    if (p.kind !== "link" || !p.target) return false;
    const t = pageTargetOf(p.target);
    return t?.kind === "page" && t.id === id;
  });
  const pagePins: PagePins | null = pins ? {
    pinned: (id) => !!pagePinOf(id),
    toggle: ({ id, url, name }) => {
      const done = pagePinOf(id);
      const work = done
        ? appApi.removePin(target.hubId, done.id, target.agent, target.peer).then(() => pinsChanged(target.hubId))
        : pinThing(target.hubId, { with: target.agent, ...(target.peer ? { a: target.peer } : {}), kind: "link", target: url, title: name });
      void work.catch((e) => setSendError(e));
    },
  } : null;
  // A pinned message: brought to the middle; in a long exchange, older
  // pages load until it turns up (a few at most).
  const [seek, setSeek] = useState<{ id: string; tries: number } | null>(null);

  // Commands sent here, for ↑ / ↓ (newest last); -1 = not browsing.
  const history = consoleTasks.map((m) => m.body).filter((b) => b && !b.includes("\n"));
  const histPos = useRef(-1);
  const histDraft = useRef("");
  // In a chat, ↑ / ↓ from an empty box: what you sent here (tasks and
  // what you added to them).
  const chatHistory = useInputHistory(
    sentTexts((messages || []).filter((m) => (m.type === "task" || m.type === "steer") && isMe(m.from, row?.hub) && !hiddenMessage(m) && !m.unreadable).map((m) => m.body)),
    draft, (v) => writeDraft(v));

  const nameOf = (from: string): string => {
    if (isConsole(from)) return personLabel(from, row?.hub);
    const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === from);
    return r ? nodeName(r.st.agent, r.hub) : from;
  };

  // The rail at the left: each turn, with how the answer starts. In your own
  // conversation a turn is something you said. Watching two nodes, it is a
  // task either of them gave the other, and the preview says who gave it.
  // A terminal has no rail: it is read from the bottom, like a terminal.
  const turns: Turn[] = [];
  for (const m of shown || []) {
    // A supplement belongs to the task it names, not a turn of its own.
    if (m.type === "progress" || m.type === "event" || m.type === "steer") continue;
    // With others' work merged in, a task one of them gave it starts a
    // turn too (a member's pane is mostly the coordinator's tasks).
    const fromOther = !observed && alsoSet.has(m.from) && m.type === "task" && m.to === target.agent;
    const starts = observed ? m.type === "task" : isMe(m.from, row?.hub) || fromOther;
    if (starts) turns.push({ id: m.id, who: observed || fromOther ? nameOf(m.from) : undefined, question: summarize(m), answer: "" });
    else if (turns.length && !turns[turns.length - 1].answer) turns[turns.length - 1].answer = summarize(m);
  }
  const turnIds = new Set(turns.map((t) => t.id));
  const hasRail = !shellMode && !execNode && turns.length >= 2;

  // Someone else's task that its node is still on (two nodes working
  // together): the foot of the conversation shows it at work. Your own
  // tasks show their run as it goes (TraceView) instead.
  const rowOf = (name: string) => rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === name);
  // (the handoff cards, with the rest of what goes between the messages)
  const baseInserts = handoffs.length
    ? [...(inserts || []), ...handoffs.map((h) => ({
      id: h.id,
      at: h.at,
      node: <HandoffCard h={h} row={rowOf(h.other)} name={nameOf(h.other)} now={now}
        onOpen={() => (onOpenExchange ? onOpenExchange(target.agent, h.other, h.id) : onOpen?.(target.hubId, target.agent, h.other))} />,
    }))]
    : inserts;
  const handoverLines = row ? handovers.filter((h) => h.state === "done").map((h) => ({
    id: "ho_" + h.id, at: h.done_at || h.at, node: <HandoverDivider h={h} hub={row.hub} />,
  })) : [];
  const allInserts = handoverLines.length ? [...(baseInserts || []), ...handoverLines] : baseInserts;

  // What each task got, by the task's id. Answers and progress updates
  // point at their task (reply_to). One without it (older runners) answers
  // the earliest task its sender has not answered in that thread; progress
  // without it belongs to the latest task its sender had there.
  const answerOf = new Map<string, Message>();
  const taskOfAnswer = new Map<string, string>();
  const progressOf = new Map<string, Message[]>();
  const taskOfProgress = new Map<string, string>();
  if (!termMode && shown) {
    const lastTaskTo = new Map<string, string>();
    const unanswered = new Map<string, string[]>();
    for (const m of shown) {
      const key = (to: string) => `${m.thread_id}\u0000${to}`;
      if (m.type === "task") {
        lastTaskTo.set(key(m.to), m.id);
        unanswered.set(key(m.to), [...(unanswered.get(key(m.to)) || []), m.id]);
      }
      if (m.type === "reply" || m.type === "event") {
        const waiting = unanswered.get(key(m.from)) || [];
        const task = m.reply_to || waiting[0];
        if (task && !answerOf.has(task)) {
          answerOf.set(task, m);
          taskOfAnswer.set(m.id, task);
          unanswered.set(key(m.from), waiting.filter((id) => id !== task));
        }
      }
      if (m.type === "progress") {
        const task = m.reply_to || lastTaskTo.get(`${m.thread_id}\u0000${m.from}`);
        if (task) {
          progressOf.set(task, [...(progressOf.get(task) || []), m]);
          taskOfProgress.set(m.id, task);
        }
      }
    }
  }

  // Someone else's tasks that are not answered yet (two nodes working
  // together): the foot of the conversation shows the node at work on the
  // first, and how many wait behind it. Your own tasks show their run as it
  // goes (TraceView) instead. A task left open for half a day, or one for a
  // node that is offline, is not being worked on.
  const working = (() => {
    if (termMode || !shown) return null;
    // (one that has gone quiet may be stuck: the lists say so; no mushroom)
    const open = shown.filter((m) => {
      if (m.type !== "task" || isMe(m.from, row?.hub) || !isOpen(m.status) || answerOf.has(m.id)) return false;
      // A task someone else gave the member shown here has its own line of
      // work under it (with the mushroom): not a second one at the bottom.
      if (!observed && alsoSet.has(m.from) && m.to === target.agent) return false;
      const w = rowOf(m.to);
      return Date.now() - new Date(m.created_at).getTime() < OPEN_FOR_MS && w?.st.state !== "offline"
        && !possiblyStuck(m, { agent: w?.st.agent, receiving: w?.st.receiving, offline: false, seenAt: w?.hub.seen_at }, now);
    });
    if (!open.length) return null;
    // Only an acknowledged task is at work: queued, pulled-but-not-acked
    // (delivered) and held all wait.
    const task = open.find((m) => m.status === "acked") || open[0];
    const said = progressOf.get(task.id);
    return {
      task,
      queued: task.status !== "acked",
      behind: open.filter((m) => m !== task && m.to === task.to).length,
      // what it last said about this task, else what it says it is doing
      status: said?.length ? said[said.length - 1].body.split("\n")[0] : rowOf(task.to)?.st.agent.status_text,
    };
  })();

  // An agent in one long session (it receives in its conversation) picks up
  // what you send next and goes on, while its steps and progress still go
  // to the task it took first, which stays open. In time, then: a run shows
  // what came before your next message (until), and the one for that next
  // message shows what the earlier ones still open got since (borrow).
  // (A runner's agent starts the next one only after the first is done.)
  const untilOf = new Map<string, string>();
  const borrowOf = new Map<string, { ids: string[]; since: string; label?: string }>();
  {
    // In time, always: what an earlier task still open gets after you sent
    // your next message goes under that message, not above it (whether or
    // not the agent has taken the next one up: until it has, it says it is
    // still on the one before).
    const mine = (shown || []).filter((m) => m.type === "task" && !m.e2e && isMe(m.from, row?.hub))
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
    mine.forEach((t, i) => {
      const st = traceStatuses.get(t.id) || t.status;
      const taken = (st !== "queued" && st !== "delivered") || !!progressOf.get(t.id)?.length;
      const earlier = mine.slice(0, i).filter((p) => isOpen(p.status) && !answerOf.has(p.id));
      const ids = earlier.map((p) => p.id);
      if (!ids.length) return;
      const last = earlier[earlier.length - 1];
      borrowOf.set(t.id, { ids, since: t.created_at, label: taken ? undefined : titleOfMessage(last.body) || L("上一件", "the one before") });
      for (const id of ids) if (!untilOf.has(id)) untilOf.set(id, t.created_at);
      // (and this one's own window ends where a later one takes over)
      const next = mine[i + 1];
      if (next && !untilOf.has(t.id)) untilOf.set(t.id, next.created_at);
    });
  }
  // What each run shows of what was reported, by the same windows.
  const saidIn = (taskId: string): Message[] | undefined => {
    const until = untilOf.get(taskId);
    const borrow = borrowOf.get(taskId);
    const own = (progressOf.get(taskId) || []).filter((m) => !until || m.created_at < until);
    const more = borrow ? borrow.ids.flatMap((id) => progressOf.get(id) || [])
      .filter((m) => m.created_at >= borrow.since && (!until || m.created_at < until)) : [];
    const all = more.length ? [...own, ...more].sort((a, b) => a.created_at.localeCompare(b.created_at)) : own;
    return all.length ? all : undefined;
  };

  // One loading in the chat, the newest: of the runs at work (acknowledged,
  // or reporting progress), only the newest one moves; an older one still
  // open says quietly that it is not finished. Someone else's task at the
  // foot counts too.
  const liveRun = (() => {
    if (termMode || !shown) return null;
    let newest: Message | null = null;
    for (const m of shown) {
      if (m.type !== "task" || m.e2e || !isOpen(m.status) || answerOf.has(m.id) || m.asking || !atWork(m)) continue;
      const status = traceStatuses.get(m.id) || m.status;
      if (status !== "acked" && status !== "held" && !progressOf.get(m.id)?.length) continue;
      const ours = isMe(m.from, row?.hub) && !isStuck(m);
      const theirs = !observed && alsoSet.has(m.from) && m.to === target.agent;
      if (!ours && !theirs) continue;
      if (!newest || new Date(m.created_at).getTime() >= new Date(newest.created_at).getTime()) newest = m;
    }
    return newest;
  })();
  const footLive = !!working && !working.queued
    && (!liveRun || new Date(working.task.created_at).getTime() > new Date(liveRun.created_at).getTime());
  const liveId = footLive ? "" : liveRun?.id || "";
  // The one at work, wherever its loading is (another waits behind it).
  const atWorkId = liveId || (footLive && working ? working.task.id : "");
  // A task of yours not opened yet, or set aside: a few words under it,
  // like a receipt, not a line of its own (it is a message, not a state).
  // Gone once it starts; nothing is said while it works or after it stops.
  const receiptOf = (m: Message): { text: string; title?: string } | null => {
    if (m.type !== "task" || m.e2e || !isMe(m.from, row?.hub) || !isOpen(m.status) || answerOf.has(m.id) || m.asking || isStuck(m)) return null;
    const st = traceStatuses.get(m.id) || m.status;
    if (st === "held") return { text: L("稍后回复", "Will answer later") };
    if (st === "acked" || progressOf.get(m.id)?.length) return null;
    if (waitingShort) return { text: waitingShort, title: waitingReason };
    if (atWorkId && atWorkId !== m.id) return { text: L("排在后面", "Queued behind the current one"), title: L("它做完手上这件就看这条", "It reads this once it is done with the one at hand") };
    return { text: L("还没开始看", "Not opened yet") };
  };

  // Messages under the run at work (tasks it handed out, your supplements):
  // its loading goes to the foot of the chat, under them, not above.
  const liveIndex = liveId && shown ? shown.findIndex((m) => m.id === liveId) : -1;
  const liveFoot = liveIndex >= 0 && (shown!.slice(liveIndex + 1).some((m) =>
    m.type !== "progress" && !(m.reply_to === liveId && (m.type === "reply" || m.type === "event" || m.type === "steer")))
    || (allInserts || []).some((x) => x.at > shown![liveIndex].created_at));
  // Jump to a message (from a quote of it) and light it up for a moment.
  const jumpTo = (id: string, smooth = true) => {
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-msg="${CSS.escape(id)}"]`);
    if (!el) return false;
    atBottom.current = false;
    restoreBelow.current = 0;
    markJump();
    el.scrollIntoView({ block: "center", behavior: smooth ? "smooth" : "auto" });
    el.classList.remove("is-flash");
    void el.offsetWidth;
    el.classList.add("is-flash");
    window.setTimeout(() => el.classList.remove("is-flash"), 1600);
    return true;
  };

  // Opened at a message (a task in the project's group): once it is shown,
  // bring it to the middle; in a long exchange, load older pages until it
  // turns up (a few at most).
  useEffect(() => {
    if (!target.focus || focusDone.current || shown === null) return;
    const id = target.focus;
    const t = requestAnimationFrame(() => {
      if (focusDone.current) return;
      if (jumpTo(id, false)) {
        focusDone.current = true;
        // Found by search (⌘K): the words it was found by are marked here.
        const q = takePendingFind(id);
        if (q) { setFindFirst(q); setSearching(true); }
        return;
      }
      if (exchange && more && !loadingOlder && focusTries.current < 10) {
        focusTries.current++;
        void loadOlder();
      } else if (!more || focusTries.current >= 10) {
        // Not found: show the newest, as usual.
        focusDone.current = true;
        atBottom.current = true;
        const el = scrollRef.current;
        if (el) el.scrollTop = el.scrollHeight;
      }
    });
    return () => cancelAnimationFrame(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shown?.length, more, loadingOlder, target.focus]);
  useEffect(() => {
    if (!seek || shown === null) return;
    const t = requestAnimationFrame(() => {
      if (jumpTo(seek.id)) { setSeek(null); return; }
      if (exchange && more && !loadingOlder && seek.tries < 10) {
        setSeek({ id: seek.id, tries: seek.tries + 1 });
        void loadOlder();
      } else if (!loadingOlder) {
        setSeek(null);
        setSendError(new Error(L("没找到这条消息：可能被删了，或者太早了", "Could not find that message: it may be deleted, or too far back")));
      }
    });
    return () => cancelAnimationFrame(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seek, shown?.length, more, loadingOlder]);

  // The thread a message continues in the exchange view: the latest one you
  // take part in under your current name (older tasks were signed with the
  // device, and the hub only lets a thread's participants write into it).
  const continuedThread = (): string | undefined => {
    const me = row?.hub.me?.member ? CONSOLE_PREFIX + row.hub.me.member.name : null;
    for (let i = (shown?.length || 0) - 1; i >= 0; i--) {
      const m = shown![i];
      if (!me || m.from === me || m.to === me) return m.thread_id;
    }
    return undefined;
  };

  const projectStarted = useRef(false);
  // "clear" in a shell session is the terminal's own business, not a command
  // for the node (which has no screen to clear): everything so far goes off
  // the screen, and comes back with the line that then heads the terminal.
  // It is remembered on this computer, across restarts of the App, by the
  // time of the last message cleared (the hub's time, as every message
  // carries it, not this clock's).
  const clearKey = `myco-term-clear:${target.hubId}/${target.agent}`;
  const [clearedAfter, setClearedAfter] = useState<string | null>(() => {
    try { return localStorage.getItem(clearKey); } catch { return null; }
  });
  const setCleared = (id: string | null) => {
    setClearedAfter(id);
    try { if (id) localStorage.setItem(clearKey, id); else localStorage.removeItem(clearKey); } catch { /* storage unavailable */ }
  };
  const clearScreen = () => {
    // The latest by time, which is not always the last in the list.
    const latest = (shown || []).reduce<Message | null>((x, m) => (!x || new Date(m.created_at) > new Date(x.created_at) ? m : x), null);
    if (latest) setCleared(latest.created_at);
    writeDraft("");
    histPos.current = -1;
  };
  const clearedTime = clearedAfter ? new Date(clearedAfter).getTime() : NaN;
  const onScreen = shown && !Number.isNaN(clearedTime) ? shown.filter((m) => new Date(m.created_at).getTime() > clearedTime) : shown;
  const cleared = !!shown && !!onScreen && onScreen.length < shown.length;

  const send = async (mode: "now" | "next" = "now") => {
    const typed = draft.trim();
    if (shellMode && (typed === "clear" || typed === "cls")) { clearScreen(); return; }
    if ((!typed && !att.any) || att.busy || sending || busy) return;
    const body = att.body(typed);
    const attachments = att.ids.length ? att.ids : undefined;
    // While this round runs: Enter adds to it, ⌘/Ctrl+Enter puts the next
    // one behind it. With nothing running, both send a message.
    const supplement = mode === "now" && supplementing && !!openTask;
    const queuedNext = mode === "next" && supplementing && !!openTask;
    setSending(true);
    setSendError(null);
    setQueueError(null);
    const startedAt = Date.now();
    // Shown in the chat at once and out of the box (not a spinner in the
    // box); put back in the box if it does not go. A queued one (kept here
    // for an older hub) shows in the queue instead.
    const files = att.take();
    const localQueued = queuedNext && !queueOnHub;
    if (!localQueued && !shellMode) setPending({ body: typed, files, queued: queuedNext });
    writeDraft("");
    histPos.current = -1;
    atBottom.current = true;
    let putBackDone = false;
    const putBack = () => {
      putBackDone = true;
      // (anything typed meanwhile stays, after it)
      const now = inputRef.current?.value || "";
      writeDraft(now.trim() ? `${typed}\n${now}` : typed);
      att.restore(files);
    };
    try {
      if (!target.peer && !target.threadId) {
        // The first message of a new conversation: then the whole one.
        const first = await appApi.sendMessage(target.hubId, { to: target.agent, body, attachments, ...(target.project ? { project: target.project } : {}) });
        chatHistory.reset();
        if (onOpen) onOpen(target.hubId, target.agent, first.from);
        else setMessages([first]);
        return;
      }
      // Opened from a project: the first message starts a thread in it.
      const inProject = !!target.project && !projectStarted.current;
      const threadId = inProject ? undefined : exchange ? continuedThread() : target.threadId;
      const sendNew = (thread_id?: string, after?: string) => appApi.sendMessage(target.hubId, {
        to: target.agent, body, thread_id, attachments,
        ...(inProject && !thread_id ? { project: target.project } : {}),
        ...(after ? { after } : {}),
      });
      const sendThread = (thread_id?: string) => sendNew(thread_id).catch((e) => {
        // The hub does not count you in that thread (sent under an older
        // name): start a new one; the exchange view shows every thread.
        if (exchange && thread_id && (e as { code?: string }).code === "not_found") return sendNew(undefined);
        throw e;
      });
      if (inProject) projectStarted.current = true;
      let m: Message;
      if (supplement) {
        // An older hub has nowhere to put files with a correction: they go
        // as a new message, as before.
        // The round may have ended just now: then this is a new message.
        m = attachments && !queueOnHub
          ? await sendThread(threadId)
          : await appApi.steer(target.hubId, openTask!.id, body, attachments).catch((e) => {
            if ((e as { code?: string }).code === "invalid_state") return sendThread(threadId);
            throw e;
          });
      } else if (queuedNext) {
        if (queueOnHub) {
          // Behind the last one already waiting: they go one after another.
          m = await sendNew(threadId, hubQueue[hubQueue.length - 1]?.id || openTask!.id);
        } else {
          // This hub cannot hold it: keep it here and send it when the
          // round ends (queued.ts).
          writeQueue([...localQueue, { id: queuedId(), body, attachments, threadId, at: Date.now() }]);
          setFlushStalled(false);
          chatHistory.reset();
          inputRef.current?.focus();
          return;
        }
      } else {
        m = await sendThread(threadId);
      }
      // The hub signs your messages with your name there: when this view
      // assumed another, follow the one it used.
      if (exchange && m.from && target.peer && m.from !== target.peer && m.type !== "steer" && onOpen) {
        onOpen(target.hubId, target.agent, m.from);
        return;
      }
      setMessages((prev) => mergeById(prev || [], [m]));
      chatHistory.reset();
      atBottom.current = true;
      inputRef.current?.focus();
    } catch (e) {
      // Slow or dropped: it may have got there all the same. Look before
      // saying it did not (and before anyone sends it twice).
      if (isTransient(e)) {
        const got = await landed(body, startedAt).catch(() => null);
        if (got) {
          setMessages((prev) => mergeById(prev || [], [got]));
          chatHistory.reset();
          return;
        }
      }
      putBack();
      setSendError(e);
    } finally {
      // Sent: the pictures' previews are not needed any more.
      if (!putBackDone) files.forEach((f) => f.preview?.startsWith("blob:") && URL.revokeObjectURL(f.preview));
      setPending(null);
      setSending(false);
    }
  };
  /** A message of yours with this text, sent since then, if the hub has it. */
  const landed = async (body: string, since: number): Promise<Message | null> => {
    await new Promise((r) => setTimeout(r, 1500));
    const list = target.peer ? (await appApi.exchanges(target.hubId, target.agent, target.peer)).messages
      : target.threadId ? (await appApi.thread(target.hubId, target.threadId)).messages : [];
    return list.find((m) => m.body === body && isMe(m.from, row?.hub) && new Date(m.created_at).getTime() >= since - 10_000) || null;
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Tab takes what the agent offered you to say next.
    if (e.key === "Tab" && !e.shiftKey && !e.nativeEvent.isComposing && suggestion && !draft) {
      e.preventDefault();
      writeDraft(suggestion);
      chatHistory.reset();
      return;
    }
    if (!shellMode && chatHistory.onKeyDown(e)) return;
    if (shellMode && (e.key === "ArrowUp" || e.key === "ArrowDown") && !e.nativeEvent.isComposing && !draft.includes("\n") && history.length > 0) {
      e.preventDefault();
      let pos = histPos.current;
      if (e.key === "ArrowUp") {
        if (pos === -1) { histDraft.current = draft; pos = history.length - 1; } else if (pos > 0) pos -= 1;
      } else {
        if (pos === -1) return;
        pos = pos + 1 < history.length ? pos + 1 : -1;
      }
      histPos.current = pos;
      writeDraft(pos === -1 ? histDraft.current : history[pos]);
      return;
    }
    // Ctrl+L clears the screen, as in a terminal.
    if (shellMode && e.ctrlKey && e.key === "l") {
      e.preventDefault();
      clearScreen();
      return;
    }
    // Ctrl+C with nothing typed stops the running command, as in a terminal.
    if (shellMode && e.ctrlKey && e.key === "c" && !draft && openTask) {
      e.preventDefault();
      void stop(openTask);
      return;
    }
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
    e.preventDefault();
    // Enter adds to the running task; ⌘/Ctrl+Enter puts the next one behind
    // it. In a terminal both just send the command, as before.
    if (!shellMode && openTask && (e.metaKey || e.ctrlKey)) void send("next");
    else void send("now");
  };

  // Stop a running task you sent: the hub closes it, its runner ends the run.
  const [stopError, setStopError] = useState<unknown>(null);
  const stop = async (task: Message) => {
    setStopError(null);
    try {
      const m = await appApi.stop(target.hubId, task.id);
      setMessages((prev) => prev && prev.map((x) => (x.id === m.id ? m : x)));
    } catch (e) {
      setStopError(e);
    }
  };
  // Its open tasks that are not in this conversation (other agents gave
  // them, or other chats): said above the box, with a way to them, since
  // nothing here would show them (a question it asked their sender, say).
  // Not: one shown here as a handoff card (a member's task to it, which
  // says how it goes), unless it shows no life; nor one that just came (its
  // card follows with the conversation's next read).
  const staleTasks = new Set(row?.st.stale_tasks || []);
  const elsewhere = (row?.st.current || []).filter((m) => m.to === target.agent && m.type === "task"
    && (m.status === "acked" || m.status === "held") && !(messages || []).some((x) => x.id === m.id)
    && !((alsoSet.has(m.from) && !staleTasks.has(m.id)) || (now - new Date(m.created_at).getTime() < 90_000 && !staleTasks.has(m.id))));
  const elsewhereStale = elsewhere.filter((m) => staleTasks.has(m.id)).length;
  const askingOthers = elsewhere.filter((m) => m.status === "held" && !!m.asking && !isMe(m.from, row?.hub));
  const whoIs = (n: string) => {
    const x = (row?.hub.overview?.agents || []).find((y) => y.agent.name === n);
    return x ? nodeName(x.agent, row?.hub) : personLabel(n, row?.hub);
  };
  // A task left open: close it as done (nothing more to come), or ask its
  // agent where it stands (a note in the task it reads with it).
  const [nudged, setNudged] = useState<Set<string>>(new Set());
  // Closing it for its recipient is for whoever manages that agent.
  const canClose = !!row && canManage(row.hub, row.st.agent.owner_id);
  const markDone = async (task: Message) => {
    setStopError(null);
    setResending(task.id);
    try {
      const m = await appApi.dismissMessage(target.hubId, task.id);
      setMessages((prev) => prev && prev.map((x) => (x.id === m.id ? m : x)));
      onRefresh?.();
    } catch (e) {
      setStopError(e);
    } finally {
      setResending(null);
    }
  };
  const nudge = async (task: Message) => {
    setStopError(null);
    setResending(task.id);
    try {
      await appApi.steer(target.hubId, task.id, L(
        "这条任务现在怎么样了？做完了就回复结果；还没做完就接着做，并发一下进度。",
        "Where does this task stand? If it is done, reply with the result; if not, carry on and send your progress."));
      setNudged((s) => new Set(s).add(task.id));
    } catch (e) {
      setStopError(e);
    } finally {
      setResending(null);
    }
  };

  // Commands other nodes sent to the computer in view. The terminal shows
  // only your own conversation with it, so these have no row there: the
  // overview knows what it is on, and which of it may have gone quiet.
  const shellRow = termMode
    ? rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === (shellMode ? target.agent : execNode))
    : undefined;
  const [stoppedShell, setStoppedShell] = useState<ReadonlySet<string>>(() => new Set());
  const [moreStuck, setMoreStuck] = useState(false);
  const [stuckError, setStuckError] = useState<unknown>(null);
  const [stoppingStuck, setStoppingStuck] = useState(false);
  const shellStuck = (shellRow?.st.current || []).filter((m) =>
    m.type === "task" && m.to === shellRow?.st.agent.name && !isMe(m.from, shellRow?.hub) && !stoppedShell.has(m.id)
    && possiblyStuck(m, { agent: shellRow?.st.agent, receiving: shellRow?.st.receiving, offline: shellRow?.st.state === "offline", seenAt: shellRow?.hub.seen_at }, now));
  const canStopShell = canManage(shellRow?.hub, shellRow?.st.agent.owner_id);
  const shellFrom = [...new Set(shellStuck.map((m) => nameOf(m.from)))];
  const stoppedShellNow = (ids: string[]) => setStoppedShell((prev) => new Set([...prev, ...ids]));
  const stopOnShell = async (m: Message) => {
    setStuckError(null);
    try {
      const stopped = await appApi.stop(target.hubId, m.id);
      stoppedShellNow([m.id]);
      setMessages((prev) => prev && prev.map((x) => (x.id === stopped.id ? stopped : x)));
      onRefresh?.();
    } catch (e) {
      setStuckError(e);
    }
  };
  const stopAllOnShell = async () => {
    if (stoppingStuck) return;
    setStuckError(null);
    setStoppingStuck(true);
    try {
      const all = shellStuck;
      const results = await Promise.allSettled(all.map((m) => appApi.stop(target.hubId, m.id)));
      stoppedShellNow(all.filter((_, i) => results[i].status === "fulfilled").map((m) => m.id));
      // Some may be already closed or out of this app's reach: say why.
      const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
      if (failed) setStuckError(failed.reason);
      onRefresh?.();
    } finally {
      setStoppingStuck(false);
    }
  };

  // A task that may have lost its result: stop it, then send the same message
  // to the same node again in the same conversation, as a fresh task.
  const [resending, setResending] = useState<string | null>(null);
  const resend = async (task: Message) => {
    if (resending) return;
    setStopError(null);
    setResending(task.id);
    try {
      const stopped = await appApi.stop(target.hubId, task.id);
      const fresh = await appApi.sendMessage(target.hubId, { to: task.to, body: task.body, thread_id: task.thread_id });
      setMessages((prev) => {
        const next = prev ? prev.map((x) => (x.id === stopped.id ? stopped : x)) : [stopped];
        return mergeById(next, [fresh]);
      });
      atBottom.current = true;
    } catch (e) {
      setStopError(e);
    } finally {
      setResending(null);
    }
  };

  // A task that failed: the same message to the same node again, in the same
  // conversation, as a fresh task (the failed one stays, marked).
  const sendAgain = async (task: Message) => {
    if (resending) return;
    setStopError(null);
    setResending(task.id);
    try {
      const fresh = await appApi.sendMessage(target.hubId, { to: task.to, body: task.body, thread_id: task.thread_id });
      setMessages((prev) => mergeById(prev || [], [fresh]));
      atBottom.current = true;
    } catch (e) {
      setStopError(e);
    } finally {
      setResending(null);
    }
  };

  // Messages waiting behind this round, in order: on the hub (v0.19.2) or
  // here. Both are one list above the box, with edit and withdraw.
  type QueueItem = { key: string; body: string; server?: Message; local?: Queued; sending?: boolean };
  const queueRows: QueueItem[] = [
    ...localQueue.map((q) => ({ key: q.id, body: q.body, local: q })),
    ...hubQueue.map((m) => ({ key: m.id, body: m.body, server: m })),
    // (on its way to the hub's queue: in it at once, its buttons once it is there)
    ...(pending?.queued ? [{ key: "pending", body: pending.body || pending.files.map((f) => f.name).join("、"), sending: true }] : []),
  ];
  const dropLocal = (id: string) => {
    writeQueue(localQueue.filter((x) => x.id !== id));
    setFlushStalled(false);
  };
  // Taking one out of the queue on the hub: the ones waiting behind it go
  // back behind what it waited on, in order (they are stopped last first,
  // so none is let go on the way, then sent again).
  const unqueue = async (gone: Message) => {
    const chain: Message[] = [];
    for (let cur = gone.id; ;) {
      const next = hubQueue.find((x) => x.after === cur);
      if (!next || chain.includes(next)) break;
      chain.push(next);
      cur = next.id;
    }
    for (const x of [...chain].reverse()) await appApi.stop(target.hubId, x.id);
    await appApi.stop(target.hubId, gone.id);
    setWithdrawn((prev) => new Set([...prev, gone.id, ...chain.map((x) => x.id)]));
    let after = gone.after;
    const resent: Message[] = [];
    for (const x of chain) {
      const re = await appApi.sendMessage(target.hubId, {
        to: x.to, body: x.body, thread_id: x.thread_id, ...(after ? { after } : {}),
        ...(x.attachments?.length ? { attachments: x.attachments.map((a) => a.file_id) } : {}),
      });
      resent.push(re);
      after = re.id;
    }
    if (resent.length) setMessages((prev) => mergeById(prev || [], resent));
  };
  const withdrawQueueItem = async (item: QueueItem) => {
    setQueueError(null);
    if (item.local) { dropLocal(item.local.id); return; }
    if (!item.server || queueBusy) return;
    setQueueBusy(item.key);
    try {
      await unqueue(item.server);
    } catch (e) {
      setQueueError(e);
    } finally {
      setQueueBusy(null);
    }
  };
  // Taking a queued message back to edit: its words and its files go back
  // into the box (files sent alone had words made for them: not those).
  const filesOnly = (body: string, n: number) => n > 0
    && /^(发来一个文件：|发来 \d+ 个文件：|Sent a file: |Sent \d+ files: )/.test(body);
  const editQueueItem = async (item: QueueItem) => {
    if (item.local) {
      dropLocal(item.local.id);
      const ids = item.local.attachments || [];
      writeDraft(filesOnly(item.body, ids.length) ? "" : item.body);
      att.restore(ids.map((id, i) => ({ id, name: L(`附件 ${i + 1}`, `File ${i + 1}`) })));
      inputRef.current?.focus();
      return;
    }
    if (!item.server || queueBusy) return;
    setQueueError(null);
    setQueueBusy(item.key);
    try {
      await unqueue(item.server);
      const files = item.server.attachments || [];
      writeDraft(filesOnly(item.body, files.length) ? "" : item.body);
      att.restore(files.map((a) => ({
        id: a.file_id, name: a.name, size: a.size,
        preview: /^image\//.test(a.content_type || "") ? appApi.attachmentUrl(target.hubId, a.file_id) : undefined,
      })));
      inputRef.current?.focus();
    } catch (e) {
      setQueueError(e);
    } finally {
      setQueueBusy(null);
    }
  };

  // What you send shows at the end at once: the view goes there.
  useEffect(() => {
    if (!pending) return;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [pending]);
  // The view stays at the bottom while something there grows (a live trace).
  const followBottom = () => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  };

  // What the timeline lists. Watching two nodes, progress is folded under
  // the answer it led to, or shown by the node at work; only the last
  // progress of a task that ended without an answer here stands in for
  // all of it.
  // A task with its run shown under it (yours, or one another member gave
  // the agent you are looking at): its progress is part of that run.
  const inRun = (task?: Message) => !!task && task.type === "task" && !task.e2e
    && (isMe(task.from, row?.hub) || (alsoSet.has(task.from) && task.to === target.agent));
  const byId = new Map((shown || []).map((m) => [m.id, m]));
  // A supplement is drawn inside the task it names, not as a turn of its
  // own (only one loading/status for the task remains).
  const steersOf = new Map<string, Message[]>();
  // What answered a question an agent asked: shown on its card.
  const answerTo = new Map<string, Message>();
  // Only while nothing came between the task and it: in time order, a
  // supplement sent after other messages (an answer to another task, work
  // handed out) is drawn where it was sent, quoting its task.
  const at = (m: { created_at: string }) => new Date(m.created_at).getTime();
  const between = (task: Message, s: Message) => (shown || []).some((m) =>
    at(m) > at(task) && at(m) < at(s) && m.id !== task.id && m.type !== "progress"
      && !(m.type === "steer" && (m.reply_to === task.id || m.answers)))
    || (allInserts || []).some((x) => new Date(x.at).getTime() > at(task) && new Date(x.at).getTime() < at(s));
  // A task whose run is shown: what you added goes into the run at the time
  // you sent it, so what the agent did after it is under it (as in Claude
  // Code and Codex), and the loading stays last.
  const runSteers = new Map<string, Message[]>();
  for (const m of shown || []) {
    if (m.type === "steer" && m.answers && byId.get(m.answers)) { answerTo.set(m.answers, m); continue; }
    if (m.type !== "steer" || !m.reply_to || !byId.get(m.reply_to)) continue;
    const task = byId.get(m.reply_to)!;
    // (sent after something else came, its own task's answer too: where it
    // was sent, in time, not back up inside that task's run)
    if (between(task, m)) continue;
    if (inRun(task)) { runSteers.set(task.id, [...(runSteers.get(task.id) || []), m]); continue; }
    steersOf.set(m.reply_to, [...(steersOf.get(m.reply_to) || []), m]);
  }
  const answerAsk = (task: Message) => async (text: string) => {
    const m = await appApi.steer(target.hubId, task.id, text, undefined);
    setMessages((prev) => mergeById(prev || [], [m]));
    atBottom.current = true;
  };
  // A message queued behind a task joins the conversation when that task
  // ends (its answer), not when it was written: it goes after that answer.
  const sinceAt = (m: Message): number => {
    const t = new Date(m.created_at).getTime();
    const prev = m.after ? byId.get(m.after) : undefined;
    if (!prev) return t;
    const end = answerOf.get(prev.id);
    return Math.max(t, end ? new Date(end.created_at).getTime() + 1 : !isOpen(prev.status) ? new Date(prev.updated_at).getTime() : t);
  };
  if (arrived.current.key !== cacheKey(target)) arrived.current = { key: cacheKey(target), at: Date.now(), first: null };
  const now0 = Date.now();
  for (const m of messages || []) if (!firstSeen.has(m.id)) firstSeen.set(m.id, now0);
  // What the first load brought (however long it took): already there, so
  // it never grows in again.
  if (messages && !arrived.current.first) arrived.current.first = new Set(messages.map((m) => m.id));
  const growsIn = (m: Message) => m.type === "reply" && !isMe(m.from, row?.hub) && !m.unreadable && !revealed.has(m.id)
    && !(m.reply_to && drafted.has(m.reply_to))
    && !arrived.current.first?.has(m.id)
    // (written before you opened the conversation: you are not watching it come)
    && new Date(m.created_at).getTime() > arrived.current.at - 30_000
    && (firstSeen.get(m.id) ?? 0) - arrived.current.at > 2000;
  const visible = (shown || []).filter((m) => {
    if (m.type === "steer" && m.answers && answerTo.get(m.answers) === m) return false;
    if (m.type === "steer" && m.reply_to && (steersOf.get(m.reply_to)?.includes(m) || runSteers.get(m.reply_to)?.includes(m))) return false;
    if (m.type !== "progress") return true;
    if (!observed) return !inRun(byId.get(taskOfProgress.get(m.id) || ""));
    const task = taskOfProgress.get(m.id) || "";
    if (answerOf.has(task) || task === working?.task.id) return false;
    const all = progressOf.get(task);
    return !all || all[all.length - 1] === m;
  });
  if (visible.some((m) => m.after)) {
    const at = new Map(visible.map((m) => [m.id, sinceAt(m)]));
    // What answers a released message comes after it too (stable sort).
    visible.sort((a, b) => Math.max(at.get(a.id)!, a.reply_to && at.has(a.reply_to) ? at.get(a.reply_to)! : 0)
      - Math.max(at.get(b.id)!, b.reply_to && at.has(b.reply_to) ? at.get(b.reply_to)! : 0));
  }
  // Inserts by time: each goes before the first message newer than it.
  const placed = new Map<string, React.ReactNode[]>();
  const trailing: React.ReactNode[] = [];
  {
    // All of them, from the start (an empty chat shows them too, so none
    // appear all at once when you write); only while older messages are
    // still to load, the ones from before what is loaded wait for them.
    const start = exchange && more && visible.length ? new Date(visible[0].created_at).getTime() : -Infinity;
    const ins = [...(allInserts || [])]
      .filter((x) => new Date(x.at).getTime() >= start)
      .sort((a, b) => a.at.localeCompare(b.at));
    let k = 0;
    for (const m of visible) {
      while (k < ins.length && new Date(ins[k].at).getTime() <= new Date(m.created_at).getTime()) {
        placed.set(m.id, [...(placed.get(m.id) || []), <Fragment key={"x" + ins[k].id}>{ins[k].node}</Fragment>]);
        k++;
      }
    }
    for (; k < ins.length; k++) trailing.push(<Fragment key={"x" + ins[k].id}>{ins[k].node}</Fragment>);
  }
  // The message just before each one (what an answer may be answering).
  const prevShown = new Map<string, string>();
  visible.forEach((m, i) => { if (i > 0) prevShown.set(m.id, visible[i - 1].id); });

  // showName: the first of a run of messages from one sender (watching two
  // nodes, names are shown once per run, as a chat groups them).
  // The newest message shown: one above it still growing in finishes at
  // once (it would read out of order).
  const newestId = visible[visible.length - 1]?.id;
  const renderMessage = (m: Message, showName = true) => {
      if (m.type === "progress") {
        const line = (
          <div className="conv-progress" key={m.id} title={clockTime(m.created_at)}>
            <span className="conv-progress-dot" aria-hidden />
            <span>{m.body}</span>
          </div>
        );
        if (!observed) return line;
        // Watching two nodes, progress is folded under the answer, or shown
        // by the node at work; what is left (a task that ended without an
        // answer here) is one folded line on its sender's side.
        const items = progressOf.get(taskOfProgress.get(m.id) || "") || [m];
        return (
          <div key={m.id} className={cn("conv-msg is-progress", m.from === target.agent ? "is-me" : "is-agent")}>
            {showName && <div className="conv-msg-name">{nameOf(m.from)}</div>}
            <ProgressFold items={items} />
          </div>
        );
      }
      if (m.type === "event") {
        return (
          <div className="conv-event" key={m.id}>
            <AlertTriangle />
            <span>{m.body}</span>
            <span className="conv-event-time">{clockTime(m.created_at)}</span>
          </div>
        );
      }
      // In an observed thread, the node you opened is on the right. With
      // others merged in, whoever gives it work (you or another member) is
      // on the right, named; its answers to them say whom they answer.
      const fromOther = !observed && alsoSet.has(m.from);
      const toOther = !observed && m.from === target.agent && alsoSet.has(m.to);
      const mine = observed ? m.from === target.agent : isMe(m.from, row?.hub) || fromOther;
      // An answer that does not come right after what it answers (several
      // answered at once, others in between) quotes it, as chat apps do, so
      // it is never an answer to nothing. Watching two nodes, its progress
      // folds under it (in your own chats it is in the run).
      const answers = taskOfAnswer.get(m.id);
      // A supplement drawn where it was sent quotes its task the same way.
      const quoted = answers || (m.type === "steer" && !m.answers ? m.reply_to : undefined);
      const asked = quoted ? shown?.find((x) => x.id === quoted) : undefined;
      // (an answer naming a message not loaded here quotes "an earlier one")
      const quote = !!quoted && prevShown.get(m.id) !== quoted && (!!asked || !!m.reply_to);
      const steps = observed && answers ? progressOf.get(answers) : undefined;
      const usage = answers ? usageOf[answers] : undefined;
      const voiced = m.unreadable ? null : viaVoice(m.body);
      const failedNote = m.status === "failed" && m.error === STOPPED ? L("已停止", "Stopped")
        : m.status === "failed" && m.error ? failureWords(m.error).short : "";
      // A task of yours that failed (not one you stopped): what happened, in
      // words, and sending it again is one click.
      const retry = m.type === "task" && m.status === "failed" && !!m.error && m.error !== STOPPED && isMe(m.from, row?.hub) && !observed;
      const receipt = receiptOf(m);
      return (
        <Fragment key={m.id}>
        <div className={cn("conv-msg", mine ? "is-me" : "is-agent", (fromOther || toOther || (alsoSet.size > 0 && showName && isMe(m.from, row?.hub))) && "is-other")} data-msg={m.id} data-turn={turnIds.has(m.id) ? m.id : undefined}>
          {fromOther ? <div className="conv-msg-name">{nameOf(m.from)}</div>
            : alsoSet.size > 0 && isMe(m.from, row?.hub) ? (showName ? <div className="conv-msg-name">{L("你", "You")}</div> : null)
            : toOther ? <div className="conv-msg-name">{L(`回复 ${nameOf(m.to)}`, `To ${nameOf(m.to)}`)}</div>
            : showName && <div className="conv-msg-name">{nameOf(m.from)}</div>}
          {steps && steps.length > 0 && <ProgressFold items={steps} />}
          <div className="conv-bubble">
            {quote && (asked ? (
              <button type="button" className="conv-quote" onClick={() => jumpTo(asked.id)} title={L("跳到这条消息", "Go to this message")}>
                <span className="conv-quote-name">{nameOf(asked.from)}</span>
                <span className="conv-quote-text">{asked.unreadable ? L("已加密的消息", "An encrypted message") : asked.body.replace(/\s+/g, " ").trim()}</span>
              </button>
            ) : (
              <span className="conv-quote is-gone">{L("回复更早的一条消息", "Replying to an earlier message")}</span>
            ))}
            {m.type === "note" && <span className="conv-tag neutral">{L("备注", "Note")}</span>}
            {mine && voiced && <VoiceMark heard={voiced.heard} />}
            {m.unreadable
              ? <span className="conv-unreadable"><Lock />{L("已加密，只有双方的主人能看", "Encrypted: only the owners of both sides can read it")}</span>
              : mine && voiced ? <MessageContent body={voiced.text} />
              : growsIn(m) ? <Reveal body={m.body} foldId={m.id} finish={m.id !== newestId} onDone={() => { revealed.add(m.id); setRevealedTick((n) => n + 1); }} />
              // What an agent wrote can be read in the App's language.
              : !mine && m.type !== "event" ? <TranslatableBody id={m.id} body={m.body} />
              : <MessageContent body={m.body} foldId={m.id} />}
            {/* (after its text has grown in: pictures first, then the
                words filling in above them, reads oddly) */}
            {!growsIn(m) && m.attachments && m.attachments.length > 0 && <MessageAttachments hubId={target.hubId} items={m.attachments} />}
            {!growsIn(m) && !m.unreadable && <PageCards body={m.body} />}
            {(steersOf.get(m.id) || []).map((s) => (
              <div className="conv-supplement" key={s.id} data-supplement={s.id}>
                <div className="conv-supplement-head">
                  <span className="conv-supplement-time">{clockTime(s.created_at)}</span>
                </div>
                {s.unreadable
                  ? <span className="conv-unreadable"><Lock />{L("已加密，只有双方的主人能看", "Encrypted: only the owners of both sides can read it")}</span>
                  : (() => {
                    const v = viaVoice(s.body);
                    return v ? <><VoiceMark heard={v.heard} /><MessageContent body={v.text} /></> : <MessageContent body={s.body} />;
                  })()}
                {s.attachments && s.attachments.length > 0 && <MessageAttachments hubId={target.hubId} items={s.attachments} />}
              </div>
            ))}
          </div>
          {/* The time and copy stay out of the way until the pointer is on the
              message; a failure or an encryption mark is always shown. */}
          <div className={cn("conv-msg-meta", (failedNote || receipt || m.automation || m.workflow) && "is-shown", m.status === "failed" && m.error !== STOPPED && "is-failed", (m.e2e || (row?.hub.e2e && !m.from.startsWith("@"))) && "is-shown")}>
            {m.e2e && <Lock className="conv-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
            {!m.e2e && row?.hub.e2e && !m.from.startsWith("@") && <span className="conv-plain">{L("未加密", "Unencrypted")}</span>}
            {usage && <span className="conv-usage">{usage}</span>}
            {m.automation && <span className="conv-auto" data-testid="via-automation" title={L("由自动化替你发出", "Sent for you by an automation")}>{L("自动", "Auto")}</span>}
            {m.workflow && <span className="conv-auto" data-testid="via-workflow" title={L("工作流里的一步，由工作区替你发出", "A step of a workflow, sent for you by the workspace")}>{L("工作流", "Workflow")}</span>}
            {receipt && <span className="conv-receipt" title={receipt.title}>{receipt.text}</span>}
            {failedNote || clockTime(m.created_at)}
            {!m.unreadable && !mine && m.type !== "event" && <TranslateToggle id={m.id} body={m.body} />}
            {!m.unreadable && <CopyMessage text={m.body} />}
            {pins && !m.unreadable && (() => {
              const on = !!pinOf(m);
              return (
                <button type="button" className={cn("conv-msg-copy conv-msg-pin", on && "is-on")} onClick={(e) => void pinMessage(m, e.currentTarget)}
                  title={on ? L("取消置顶", "Unpin") : L("置顶：放到对话上方，点一下就到", "Pin: above the conversation, one click away")}
                  aria-label={on ? L("取消置顶", "Unpin") : L("置顶", "Pin")} aria-pressed={on}>
                  {on ? <PinOff /> : <PinIcon />}
                </button>
              );
            })()}
            <button type="button" className="conv-msg-copy conv-msg-del" onClick={() => removeMessage(m)}
              title={L("删除（只从你这边去掉）", "Delete (from your view only)")} aria-label={L("删除", "Delete")}><Trash2 /></button>
          </div>
        </div>
        {retry && (
          <div className="conv-stuck is-failed" data-failed={m.id}>
            <span className="dot failed" aria-hidden />
            <span className="conv-stuck-text">{failureWords(m.error!).long}</span>
            <span className="conv-stuck-actions">
              <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void sendAgain(m)}>
                {resending === m.id ? L("正在发送…", "Sending…") : L("重新发送", "Send again")}
              </button>
            </span>
          </div>
        )}
        {/* What the agent did for a task you gave it (its runner reports it):
            the start of its turn, on its side of the conversation. A task that
            went quiet after it was picked up may have lost its result: say so,
            and offer to stop it and send it again. */}
        {m.type === "task" && !m.e2e && isMe(m.from, row?.hub) && (
          <>
            {/* (its run stays: what it did and said; the line about being
                stuck goes under it) */}
            <TraceView hubId={target.hubId} task={m} cli={rowOf(m.to)?.st.agent.runtime?.cli} onStatus={onTraceStatus} onGrow={followBottom} answered={answerOf.has(m.id)} quiet={m.id !== liveId} footed={m.id === liveId && liveFoot} onPaused={onPaused} onActivity={onActivity} until={untilOf.get(m.id)} borrow={borrowOf.get(m.id)} steers={runSteers.get(m.id)} said={saidIn(m.id)} onUsage={answerOf.has(m.id) ? onUsage : undefined}
          answers={answerTo} canAnswer={isMe(m.from, row?.hub)} onAnswer={answerAsk(m)} />
            {isStuck(m) && (<div className="conv-stuck" data-stuck={m.id}>
            <span className="dot pending" aria-hidden />
            <span className="conv-stuck-text">{stuckLine(m)}</span>
            <span className="conv-stuck-actions">
              {canClose && (
                <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void markDone(m)}
                  title={L("已经做完了：关掉这条任务，不再算在干活", "It is done: close this task so it no longer counts as work")}>
                  {L("标记完成", "Mark done")}
                </button>
              )}
              <button type="button" className="conv-steer-btn" disabled={resending === m.id || nudged.has(m.id)} onClick={() => void nudge(m)}
                title={L("问它这条任务怎么样了", "Ask it where this task stands")}>
                {nudged.has(m.id) ? L("已经催了", "Asked") : L("再催一下", "Nudge")}
              </button>
              <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void stop(m)}>
                {L("停止", "Stop")}
              </button>
              <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void resend(m)}>
                {resending === m.id ? L("正在重发…", "Resending…") : L("重发", "Resend")}
              </button>
            </span>
          </div>)}
          </>
        )}
        {/* What it did for a task someone else gave it, the same way. */}
        {m.type === "task" && !m.e2e && fromOther && m.to === target.agent && (
          <TraceView hubId={target.hubId} task={m} cli={rowOf(m.to)?.st.agent.runtime?.cli} onStatus={onTraceStatus} onGrow={followBottom} answered={answerOf.has(m.id)} quiet={m.id !== liveId} footed={m.id === liveId && liveFoot} onPaused={onPaused} steers={runSteers.get(m.id)} said={progressOf.get(m.id)} onUsage={answerOf.has(m.id) ? onUsage : undefined} answers={answerTo} />
        )}
        </Fragment>
      );
  };

  // What search looks in: the messages shown, and supplements drawn inside
  // their task (found as that task).
  const searchItems: SearchItem[] = [];
  if (searching) {
    for (const m of visible) {
      if (m.unreadable) continue;
      searchItems.push({ id: m.id, text: m.body });
      for (const st of steersOf.get(m.id) || []) if (!st.unreadable) searchItems.push({ id: m.id, text: st.body });
      for (const st of runSteers.get(m.id) || []) if (!st.unreadable) searchItems.push({ id: st.id, text: st.body });
    }
  }

  // A file sent again under the same name is its next version.
  const versions = versionsOf(shown || []);

  // The header's ⋯ menu: what is used now and then.
  const headMore: MenuItem[] = [
    ...(!termMode ? [
      { label: L("文件和应用", "Files and apps"), icon: Images, onSelect: () => setFilesOpen(true) },
      { label: L("聊天背景", "Chat background"), icon: Paintbrush, onSelect: () => setBgOpen(true) },
      "sep" as const,
    ] : []),
    ...(!termMode && pins ? [{ label: L("置顶一个网址", "Pin an address"), icon: PinIcon, onSelect: () => setPinAdding(true) }] : []),
    ...(hoEntry.show ? [{ label: hoEntry.why ? L("换一个来接着做（现在不行）", "Hand over (not now)") : L("换一个来接着做", "Hand over to another session"), icon: ArrowRightLeft, disabled: !!hoEntry.why, onSelect: () => openHandover({ hubId: target.hubId, name: target.agent }) }] : []),
    ...(!observed && (shown?.length || 0) > 0 ? [{ label: L("导出这段对话", "Export this conversation"), icon: Download, onSelect: () => setExporting(true) }] : []),
  ].filter((it, i, all) => it !== "sep" || i < all.length - 1);
  // Everything sent in the chat so far (what is loaded), for 文件和应用:
  // files, and the apps (pages) linked in its messages.
  const chatFiles: ChatFile[] = filesOpen ? (shown || []).flatMap((m) => [
    ...(m.unreadable ? [] : (m.attachments || []).map((a) => ({ a, msg: m.id, from: m.from, at: m.created_at }))),
    ...(steersOf.get(m.id) || []).flatMap((x) => (x.unreadable ? [] : (x.attachments || []).map((a) => ({ a, msg: m.id, from: x.from, at: x.created_at })))),
  ]) : [];
  const chatApps: ChatApp[] = filesOpen ? (shown || []).flatMap((m) => [
    ...(m.unreadable ? [] : pageLinksIn(m.body || "").map((p) => ({ ...p, msg: m.id, from: m.from, at: m.created_at }))),
    ...(steersOf.get(m.id) || []).flatMap((x) => (x.unreadable ? [] : pageLinksIn(x.body || "").map((p) => ({ ...p, msg: m.id, from: x.from, at: x.created_at })))),
  ]) : [];

  return (
    <VersionsContext.Provider value={versions}>
    <PagePinsContext.Provider value={pagePins}>
    <div className={cn("conv-detail", hasRail && "has-rail", chatBg && "has-bg")} ref={rootRef} onPointerDown={() => markActivePane(rootRef.current)}>
      {/* 聊天背景 (chatBg.ts): this conversation's own, filling its pane */}
      {chatBg && (
        <div className="conv-bg" aria-hidden>
          <div className="cbg-layer" style={{ background: chatBg.css }} />
          {chatBg.veil > 0 && <div className="cbg-veil" style={{ opacity: chatBg.veil }} />}
        </div>
      )}
      {!bare && <header className="conv-head">
        {(showBack || target.returnTo) && !isSa && (
          <button type="button" className="app-btn app-btn-plain" onClick={onBack}
            aria-label={backLabel || L("返回", "Back")} title={backLabel || L("返回", "Back")}>
            <ArrowLeft />
          </button>
        )}
        {/* Who this is with: their faces, their names, and what they do. */}
        <span className="conv-head-faces">
          {(observed ? parties : [target.agent]).map((n) => {
            const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === n);
            return r ? <Avatar key={n} agent={r.st.agent} size={30} badge={false} /> : null;
          })}
        </span>
        <div className="conv-head-titles">
          <h2>{observed ? parties.map(nameOf).join(" 和 ") : agentName}</h2>
          <p>
            {observed
              ? L("它们之间的对话，你只能看", "Their conversation; you can only read it")
              : shellMode ? (agent ? whereOf({ hub: row!.hub, st: row!.st }) : "") : [role, agent?.description].filter(Boolean).join(" · ") || L("和你的一对一", "Just you and it")}
            {row?.hub.stale && <> · <span className="app-stale" title={row.hub.stale.error || undefined}>{L(`连不上 · 显示的是 ${timeAgo(row.hub.stale.at, now)}的内容`, `Can't reach it · showing ${timeAgo(row.hub.stale.at, now)}`)}</span></>}
          </p>
        </div>
        {/* No running state here (已完成, 在干活…): the foot of the chat
            shows that. Only a task that may be stuck is called out, and
            this takes you to it. */}
        {!observed && firstStuck && (
          <button type="button" className="conv-head-state is-link" onClick={toStuck} title={L("跳到可能卡住的任务", "Go to the task that may be stuck")}>
            <span className={cn("dot", state.dot)} />
            {state.label}
          </button>
        )}
        {/* Find, and the pane's own buttons (full width, close): one group,
            one spacing, one icon size. */}
        {(!termMode || headActions || computer) && (
          <span className="conv-head-actions">
            {/* A computer: who may use it, its projects, removing it. */}
            {computer && (
              <button type="button" className={cn("app-btn app-btn-plain conv-head-find", headActions && "sp-pane-btn")} data-testid="computer-settings-open"
                onClick={() => window.dispatchEvent(new CustomEvent("myco-computer-settings", { detail: { hubId: target.hubId, name: target.agent } }))}
                title={L("这台电脑：谁能用、项目、移除", "This computer: who can use it, projects, remove")} aria-label={L("这台电脑的设置", "This computer's settings")}>
                <Settings2 />
              </button>
            )}
            {/* Used now and then: one ⋯ menu; find stays out, next to the
                pane's own buttons (full width, close). */}
            {headMore.length > 0 && (
              <button type="button" className={cn("app-btn app-btn-plain conv-head-find", headActions && "sp-pane-btn")} data-testid="chat-more"
                onClick={(e) => headMenu.openUnder(e.currentTarget, headMore)} title={L("更多", "More")} aria-label={L("更多", "More")} aria-haspopup="menu">
                <MoreHorizontal />
              </button>
            )}
            {!termMode && (
              <button type="button" className={cn("app-btn app-btn-plain conv-head-find", headActions && "sp-pane-btn", searching && "is-on")} onClick={() => setSearching((v) => !v)}
                title={L(`在这段对话里找（${mod}F）`, `Find in this chat (${mod}F)`)} aria-label={L("在这段对话里找", "Find in this chat")}>
                <Search />
              </button>
            )}
            {headActions}
          </span>
        )}
      </header>}
      {pins && (
        <PinBar hubId={target.hubId} withWhom={target.agent} a={target.peer} pins={pins} setPins={(p) => pinState.setPins(p)}
          onJump={(id) => setSeek({ id, tries: 0 })} adding={pinAdding} onAdding={setPinAdding} />
      )}
      {pinMenu.element}
      {headMenu.element}
      {!termMode && <ChatBgDialog open={bgOpen} onClose={() => setBgOpen(false)} chat={chatKey(target.hubId, target.agent)} name={agentName} />}
      <ChatFiles open={filesOpen} onOpenChange={setFilesOpen} hubId={target.hubId} items={chatFiles} apps={chatApps} now={now}
        nameOf={(n) => (isMe(n, row?.hub) ? L("你", "You") : nameOf(n))}
        more={exchange && more} loadingMore={loadingOlder} onMore={() => void loadOlder()} onJump={(id) => setSeek({ id, tries: 0 })} />
      <ExportChat open={exporting} onClose={() => setExporting(false)} hubId={target.hubId} withWhom={agentName}
        messages={shown || []} nameOf={(n) => (isMe(n, row?.hub) ? L("你", "You") : nameOf(n))} />

      {/* The terminal shows only your own commands; ones other nodes sent to
          this computer may be stuck with nothing on screen. Say so above it,
          with what they were and a way to stop them. */}
      {termMode && shellStuck.length > 0 && (
        <div className={cn("shell-stuck", moreStuck && "is-open")} data-shell-stuck>
          <div className="shell-stuck-line">
            <span className="dot pending" aria-hidden />
            <span className="shell-stuck-text">
              {L(
                `这台电脑上有 ${shellStuck.length} 条命令可能卡住（来自 ${shellFrom.join("、")}）`,
                `${shellStuck.length} command${shellStuck.length === 1 ? "" : "s"} on this computer may be stuck (from ${shellFrom.join(", ")})`,
              )}
            </span>
            <button type="button" className="shell-stuck-toggle" aria-expanded={moreStuck} onClick={() => setMoreStuck((v) => !v)}>
              <ChevronRight />
              {moreStuck ? L("收起", "Hide") : L("查看", "Show")}
            </button>
            {canStopShell && (
              <button type="button" className="conv-steer-btn" disabled={stoppingStuck} onClick={() => void stopAllOnShell()}>
                {stoppingStuck ? L("正在停止…", "Stopping…") : L("全部停止", "Stop all")}
              </button>
            )}
          </div>
          {moreStuck && (
            <ul className="shell-stuck-list">
              {shellStuck.map((m) => {
                const cmd = (m.body || "").split("\n")[0].trim();
                const quiet = quietFor(seenNow(now, shellRow?.hub.seen_at), lastActivityAt(m));
                return (
                  <li key={m.id} className="shell-stuck-row">
                    <span className="shell-stuck-cmd" title={m.body}>{cmd || nameOf(m.from)}</span>
                    <span className="shell-stuck-meta">{nameOf(m.from)} · {L(`${quiet}没有动静`, `quiet for ${quiet}`)}</span>
                    {canStopShell && (
                      <button type="button" className="conv-steer-btn" onClick={() => void stopOnShell(m)}>
                        {L("停止", "Stop")}
                      </button>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
          <ErrorText error={stuckError} />
        </div>
      )}

      <div className={cn("conv-body", hasRail && "has-rail", termMode && "is-shell")}>
      {hasRail && <TurnRail turns={turns} scroller={scrollRef} />}
      {searching && !termMode && (
        <ChatSearch items={searchItems} root={timelineRef} initial={findFirst} onJump={(id) => jumpTo(id)} onClose={() => { setSearching(false); setFindFirst(""); }}
          onLoadMore={exchange && more ? () => void loadOlder() : undefined} loading={loadingOlder} />
      )}
      {undo && (
        <div className="conv-undo" role="status">
          <span>{L("已删除", "Deleted")}</span>
          <button type="button" onClick={() => {
            const ids = undo.ids;
            setUndo(null);
            void setHidden(target.hubId, ids, false).catch(setSendError);
          }}>{L("撤销", "Undo")}</button>
        </div>
      )}
      {away && (() => {
        const fresh = (visible || []).filter((m) => !isMe(m.from, row?.hub) && m.type !== "progress" && (firstSeen.get(m.id) ?? 0) > awaySince.current).length;
        const label = fresh ? L(`${fresh} 条新消息`, `${fresh} new`) : L("回到底部", "Back to the bottom");
        return (
          <button type="button" className={cn("conv-to-bottom", fresh > 0 && "has-new")} onClick={toBottom} title={L("回到底部", "Back to the bottom")} aria-label={label}>
            <ArrowDown />{fresh > 0 && <span>{label}</span>}
          </button>
        );
      })()}
      <div className="conv-scroll" ref={scrollRef} onScroll={onScroll}
        onWheel={markUserScroll} onTouchMove={markUserScroll} onKeyDown={markUserScroll}
        onPointerDown={() => { pointerHeld.current = true; markUserScroll(); }}>
        <div ref={timelineRef} className={cn("conv-timeline", termMode && "is-shell", observed && "is-observed")}>
          {shown === null ? (
            loadError && !isTransient(loadError) ? (
              <ErrorText error={loadError} />
            ) : loadError ? (
              // The hub is slow or down: keep the placeholders, say so
              // quietly; polling retries and the note goes once it answers.
              <>
                <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
                <div className="conv-retry-note">{L("暂时连不上，正在重试…", "Can't reach this hub right now; retrying…")}</div>
              </>
            ) : (
              <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
            )
          ) : shown.length === 0 && !shellMode && trailing.length === 0 ? (
            observed
              ? <div className="app-empty-note">{L("它们还没说过话。", "They haven't talked yet.")}</div>
              : isSa ? <SuperagentIntro agent={agent} />
              : <EmptyChat key={target.agent} agent={agent} name={agentName} project={projectTitle}
                onStarter={(t) => { writeDraft(t); window.setTimeout(() => { const el = inputRef.current; if (el) { el.focus(); el.setSelectionRange(t.length, t.length); } }, 0); }} />
          ) : (
            <>
            {exchange && more && !(shellMode && cleared) && (
              <div className="conv-older">
                <button type="button" className="app-btn app-btn-small" onClick={() => void loadOlder()} disabled={loadingOlder}>
                  <Busy busy={loadingOlder}>{L("加载更早的消息", "Load earlier messages")}</Busy>
                </button>
              </div>
            )}
            {shellMode ? (
              <>
              <div onMouseUp={() => { if (window.getSelection()?.isCollapsed !== false) inputRef.current?.focus(); }} className="term-wrap">
                {cleared && (
                  <button type="button" className="term-restore" onClick={() => setCleared(null)}>
                    {L("已清屏 · 显示之前的内容", "Cleared · show what was here")}
                  </button>
                )}
                <Terminal messages={onScreen || shown} execNode={target.agent} nameOf={nameOf} hubE2E={!!row?.hub.e2e} live cwd={agent?.binding.workdir}
                  hubId={target.hubId} onGrow={followBottom}>
                  <div className="term-input-line">
                    <span className="term-prompt">{agent?.binding.workdir ? <span className="term-cwd">{agent.binding.workdir}</span> : null}<PromptSign /></span>
                    <textarea
                      ref={inputRef}
                      className="term-input"
                      value={draft}
                      onChange={(e) => { writeDraft(e.target.value); histPos.current = -1; }}
                      onKeyDown={onKeyDown}
                      rows={1}
                      autoFocus
                      spellCheck={false}
                      autoCapitalize="off"
                      autoCorrect="off"
                      placeholder={shown.length === 0 ? L("输入命令，回车执行", "Type a command, Enter to run") : ""}
                      aria-label={L("命令", "Command")}
                    />
                  </div>
                </Terminal>
              </div>
              <div className="term-foot">
                {openTask
                  ? <button type="button" className="term-foot-stop" onClick={() => void stop(openTask)}>{L("正在执行 · Ctrl+C 停止", "Running · Ctrl+C to stop")}</button>
                  : <span>{L(`在 ${agentName} 上执行`, `Runs on ${agentName}`)}</span>}
                <span>{L("回车执行 · Shift+Enter 换行 · ↑↓ 历史命令", "Enter to run · Shift+Enter for a new line · ↑↓ history")}</span>
              </div>
              <ErrorText error={sendError || stopError} />
              </>
            ) : execNode ? <Terminal messages={shown} execNode={execNode} nameOf={nameOf} hubE2E={!!row?.hub.e2e} hubId={target.hubId} onGrow={followBottom} /> : visible.map((m, i) => {
              // Inserts that come before this message (by time).
              const before = placed.get(m.id);
              // A divider with the time only where the conversation picks up
              // again after a while, not between every two threads.
              const divider = i > 0 && new Date(m.created_at).getTime() - new Date(visible[i - 1].created_at).getTime() > GAP_MS
                ? <div className="conv-thread-divider" key={"d" + m.id}><span>{dayTime(m.created_at)}</span></div>
                : null;
              const prev = visible[i - 1];
              const runStart = !prev || !!divider || prev.from !== m.from || prev.type === "event" || m.type === "event";
              const item = renderMessage(m, runStart);
              // Keyed by the message, not its place: older messages loaded
              // above (or one hidden) would remount everything below, and an
              // unfolded long message would fold again.
              return <Fragment key={"m" + m.id}>{before}{divider}{item}</Fragment>;
            })}
            {!shellMode && !execNode && trailing}
            {/* What you just sent, until the workspace has it: drawn as
                yours, but not one of yours yet (not .is-me). */}
            {pending && !pending.queued && (
              <div className="conv-msg is-pending" data-testid="chat-sending">
                <div className="conv-bubble">
                  {pending.body && <MessageContent body={pending.body} />}
                  {pending.files.length > 0 && <MessageAttachments hubId={target.hubId} items={pending.files.map((f) => ({ file_id: f.id, name: f.name, size: f.size, sha256: "" }))} />}
                </div>
                <div className="conv-msg-meta is-shown conv-sending"><Loader2 size={11} className="spin" />{L("发送中…", "Sending…")}</div>
              </div>
            )}
            {/* The run at work, when messages sit under it. */}
            {liveFoot && liveRun && (() => {
              // What it is doing now, not what it said when it took this one
              // up: an agent in one long session goes on answering newer
              // messages while this task stays open. Its newest progress
              // (any task), unless it has answered since (then what it says
              // it is doing, or a word); the clock from the latest thing in
              // the chat.
              const by = liveRun.to;
              const newest = (pred: (m: Message) => boolean) => (shown || []).filter(pred).reduce<Message | null>((x, m) => (!x || m.created_at > x.created_at ? m : x), null);
              const said = newest((m) => m.type === "progress" && m.from === by);
              const answered = newest((m) => (m.type === "reply" || m.type === "event") && m.from === by);
              const status = said && (!answered || said.created_at > answered.created_at) ? said.body.split("\n")[0]
                : rowOf(by)?.st.agent.status_text || undefined;
              const since = [liveRun.created_at, ...(runSteers.get(liveRun.id) || []).map((x) => x.created_at),
                ...(visible || []).filter((m) => m.type !== "progress").map((m) => m.created_at)]
                .reduce((a, c) => (new Date(c).getTime() > new Date(a).getTime() ? c : a));
              return <WorkingLine key={liveRun.id} name={nameOf(by)} since={since} status={status} side="left" />;
            })()}
            {working && footLive && (
              <WorkingLine key={working.task.id} name={nameOf(working.task.to)} since={working.task.created_at}
                status={working.status} behind={working.behind}
                side={observed && working.task.to === target.agent ? "right" : "left"} />
            )}
            </>
          )}
        </div>
      </div>
      </div>

      {shellMode ? null : observed ? (
        <div className="conv-observe">
          <span>{L("这是它们之间的对话，你只能看。", "This is a record of nodes working together. You can only view it.")}</span>
          <button type="button" className="app-btn" onClick={() => onNewConversation(target.hubId, target.agent)}>
            {L(`给 ${agentName} 发任务`, `Give ${agentName} a task`)}
          </button>
        </div>
      ) : (
      <div className="conv-compose">
        {busy && (
          <div className="conv-busy">
            <span className="dot pending" />
            <span className="conv-busy-text">
              {openTask
                ? L("这个会话正开在别处，任务在等它空出来。", "This session is open somewhere else; the task waits until it is free.")
                : L("这个会话正开在别处（终端、编辑器或桌面版），现在发的消息要等它空出来才会处理。", "This session is open somewhere else (a terminal, an editor or a desktop app); messages wait until it is free.")}
            </span>
            {forkable && onOpen && (
              <button type="button" className="conv-steer-btn" disabled={copying} onClick={() => void copyAndContinue()}
                title={L("复制一个一模一样的它（同一台电脑、同一个文件夹），在副本里接着做；原来的不动", "Copy this session into a new node (same machine and folder) and carry on there; the original session is left alone")}>
                {copying ? L("正在复制…", "Copying…") : openTask ? L("复制一份接着做", "Continue in a copy") : L("复制一份来对话", "Talk to a copy")}
              </button>
            )}
            {openTask && (
              <button type="button" className="conv-steer-btn" onClick={() => void stop(openTask)}>{L("停止", "Stop")}</button>
            )}
          </div>
        )}
        {busy && <ErrorText error={copyError} />}
        {elsewhere.length > 0 && (
          <div className="conv-busy" data-testid="open-elsewhere">
            <span className="dot idle" />
            <span className="conv-busy-text">
              {elsewhereStale > 0
                ? elsewhereStale === elsewhere.length
                  ? L(`它有 ${elsewhere.length} 条别处来的任务好一阵没有动静`, `It has ${elsewhere.length} open task${elsewhere.length > 1 ? "s" : ""} from elsewhere with no activity for a while`)
                  : L(`它还有 ${elsewhere.length} 条别处来的任务没做完，其中 ${elsewhereStale} 条好一阵没有动静`, `It has ${elsewhere.length} more open tasks from elsewhere, ${elsewhereStale} with no activity for a while`)
                : L(`它还有 ${elsewhere.length} 条别处来的任务没做完`, `It has ${elsewhere.length} more open task${elsewhere.length > 1 ? "s" : ""} from elsewhere`)}
              {askingOthers.length > 0 && L(`，其中 ${askingOthers.length} 条在等 ${[...new Set(askingOthers.map((m) => whoIs(m.from)))].join("、")} 回答它的问题`,
                `; ${askingOthers.length} wait${askingOthers.length > 1 ? "" : "s"} on ${[...new Set(askingOthers.map((m) => whoIs(m.from)))].join(", ")} to answer its question`)}
              {L("。", ".")}
            </span>
            <button type="button" className="conv-steer-btn"
              onClick={() => window.dispatchEvent(new CustomEvent("myco-open-profile", { detail: { hubId: target.hubId, name: target.agent } }))}>
              {L("查看并处理", "See and handle")}
            </button>
          </div>
        )}
        {!busy && offline && (
          <div className="conv-busy">
            <span className="dot idle" />
            <span className="conv-busy-text">
              {agent && isInbox(agent)
                ? L("对方只在会话里接收消息，现在发的消息等它的会话再挂上 inbox wait 后处理。", "The node receives only in its session; messages sent now are handled when its session runs inbox wait again.")
                : L("对方离线，现在发的消息会在它上线后处理。", "The node is offline; messages sent now are handled when it is back.")}
            </span>
          </div>
        )}
        {queueRows.length > 0 && (
          <div className="conv-queue" data-queue>
            {queueRows.map((item) => (
              <div className="conv-queue-row" data-queue-row={item.key} key={item.key}>
                <span className="conv-queue-text" title={item.body}>
                  <span className="conv-queue-label">{L("排队中：", "Queued: ")}</span>
                  {item.body.split("\n")[0].trim() || L("（附件）", "(files)")}
                </span>
                <button type="button" className="conv-queue-btn" disabled={queueBusy === item.key || item.sending}
                  onClick={(e) => { e.stopPropagation(); void editQueueItem(item); }}>
                  {L("编辑", "Edit")}
                </button>
                <button type="button" className="conv-queue-btn" disabled={queueBusy === item.key || item.sending}
                  onClick={(e) => { e.stopPropagation(); void withdrawQueueItem(item); }}>
                  {L("撤回", "Withdraw")}
                </button>
              </div>
            ))}
          </div>
        )}
        {queueRows.length > 0 && <ErrorText error={queueError} />}
        {voiceOn && !shellMode && (
          <VoiceBar />
        )}
        <ApprovalStrip hubId={target.hubId} agent={target.agent} name={agentName} />
        <SendTrouble error={sendError} stop={stopError} onRetry={sendError ? () => void send() : undefined}
          onClose={() => { setSendError(null); setStopError(null); }} />
        {/* Only while it is at work: a chat that has ended has nothing to
            wait for, whatever the network. */}
        {behind && !!atWorkId && !sendError && !stopError && (
          <div className="conv-trouble conv-behind" role="status" data-testid="chat-behind">
            <WifiOff />
            <span className="conv-trouble-text">
              <b>{L("网络不太好", "Poor connection")}</b>
              {now - seenAt < 60_000
                ? L("这里的消息有一会儿没更新了，", "This chat hasn't updated for a little while; ")
                : L(`这里的消息 ${quietFor(now, seenAt)}没更新了，`, `this chat hasn't updated for ${quietFor(now, seenAt)}; `)}
              {L("agent 那边可能还在继续。正在自动重试。", "the agent may well be at work. Retrying by itself.")}
            </span>
            <button type="button" className="app-btn app-btn-small" disabled={repolling}
              onClick={() => { setRepolling(true); void repoll.current().finally(() => setRepolling(false)); }}>
              {repolling ? L("正在重试…", "Retrying…") : L("重试", "Retry")}
            </button>
          </div>
        )}
        <div className="conv-compose-box" onClick={() => inputRef.current?.focus()} {...dropFiles(att.add)}>
          <AttachList items={att.items} onRemove={att.remove} />
          <textarea
            ref={inputRef}
            className="conv-compose-input"
            value={draft}
            onChange={(e) => { writeDraft(e.target.value); chatHistory.reset(); }}
            onPaste={(e) => {
              const files = pastedFiles(e);
              if (!files.length) return;
              // A file on the clipboard goes as a file (its text, if any, too).
              if (!e.clipboardData.getData("text/plain")) e.preventDefault();
              att.add(files);
            }}
            onKeyDown={onKeyDown}
            disabled={busy}
            placeholder={busy
              ? L("会话正开在别处，暂时不能发；可以「复制一份来对话」", "The session is open elsewhere, so you can't send now; you can talk to a copy")
              : supplementing && openTask?.asking
                ? L(`回答 ${agentName} 的问题…`, `Answer ${agentName}'s question…`)
              : supplementing
                ? L(`补充给 ${agentName}…（${mod}↩ 排到下一条）`, `Add to ${agentName}… (${mod}↩ queues the next one)`)
                : (suggestion && fitOneLine(suggestion, inputRef.current)) || L(`发给 ${agentName}…`, `Message ${agentName}…`)}
            rows={1}
            aria-label={L("消息", "Message")}
          />
          <div className="conv-compose-bar">
            {/* left: what you can do to the running task; room for attachments
                and voice input later */}
            <div className="conv-compose-tools">
              <AttachButton onFiles={att.add} disabled={busy} />
              {/* its model and effort, small (RunSettings) */}
              {!shellMode && !observed && row && <RunPill row={row} onRefresh={async () => { await onRefresh?.(); }} />}
              {!shellMode && !observed && row && <AccessPill row={row} onRefresh={async () => { await onRefresh?.(); }} />}
            </div>
            {/* Only what is not obvious: while it works, Enter adds to the
                task and ⌘↩ queues the next one. */}
            <span className="conv-compose-hint">
              {supplementing ? L(`Enter 补充 · ${mod}↩ 排到下一条`, `Enter to add · ${mod}↩ to queue`)
                : suggestion && !draft ? <><kbd className="conv-kbd">Tab</kbd>{L(" 填入建议", " to use the suggestion")}</> : ""}
            </span>
            {/* Speaking instead of typing, beside Send (as in Codex and Claude). */}
            {!shellMode && <DictateButton input={inputRef} value={draft} onText={(t) => { writeDraft(t); chatHistory.reset(); }} disabled={busy} />}
            {openTask && !draft.trim() && !att.items.length && !sending ? (
              // Nothing to send while it works: the same button stops it.
              <button
                type="button"
                className="conv-send is-stop"
                onClick={(e) => { e.stopPropagation(); void stop(openTask); }}
                title={L("停止这个任务：agent 不再继续做", "Stop this task: the agent stops working on it")}
                aria-label={L("停止", "Stop")}
              >
                <Square size={11} strokeWidth={0} fill="currentColor" />
              </button>
            ) : !shellMode && !voiceOn && voice?.ready && !draft.trim() && !att.items.length && !sending ? (
              // Nothing typed: the same place starts a voice chat with it (as in ChatGPT).
              <button
                type="button"
                className="conv-send is-call"
                onClick={(e) => { e.stopPropagation(); callHim(); }}
                title={voiceCall ? L(`挂断和 ${voiceCall.agentName} 的通话，改和 ${agentName} 语音聊天`, `Hang up on ${voiceCall.agentName} and voice chat with ${agentName}`)
                  : L(`和 ${agentName} 语音聊天`, `Voice chat with ${agentName}`)}
                aria-label={L("语音聊天", "Voice chat")}
                disabled={busy}
              >
                <AudioLines size={16} strokeWidth={2.2} />
              </button>
            ) : (
              <button
                type="button"
                className="conv-send"
                onClick={(e) => { e.stopPropagation(); void send(); }}
                disabled={sending || att.busy || (!draft.trim() && !att.any)}
                title={supplementing ? L("补充给这条任务", "Add to this task") : L("发送", "Send")}
                aria-label={supplementing ? L("补充给这条任务", "Add to this task") : L("发送", "Send")}
              >
                <ArrowUp size={16} strokeWidth={2.4} />
              </button>
            )}
          </div>
        </div>
      </div>
      )}
    </div>
    </PagePinsContext.Provider>
    </VersionsContext.Provider>
  );
}

/** Copies a reply; shown with the time when the pointer is on the message. */
function CopyMessage({ text }: { text: string }) {
  const { copied, copy } = useCopy(text);
  return (
    <button type="button" className="conv-msg-copy" onClick={() => void copy()} title={L("复制", "Copy")} aria-label={L("复制", "Copy")}>
      {copied ? <Check /> : <Copy />}
    </button>
  );
}

/** Why a message did not go (or a stop did not take), above the box: in
 *  words, the text still in the box, 重试 and ✕; the raw error on hover. */
function SendTrouble({ error, stop, onRetry, onClose }: { error: unknown; stop: unknown; onRetry?: () => void; onClose: () => void }) {
  const e = error || stop;
  if (!e) return null;
  const transient = isTransient(e);
  const raw = e instanceof Error ? e.message : String(e);
  const said = (e as { suggestion?: string } | null)?.suggestion;
  const head = error ? L("没发出去", "Not sent") : L("没能停下", "Did not stop");
  const why = transient
    ? L("工作区暂时连不上，或者响应太慢。", "The workspace can't be reached right now, or is answering too slowly.")
    : raw;
  return (
    <div className="conv-trouble" role="alert" data-testid="send-trouble" title={transient ? raw : undefined}>
      <AlertTriangle />
      <span className="conv-trouble-text"><b>{head}</b>{why}{error ? L("内容还在输入框里。", " What you wrote is still in the box.") : ""}{said && !transient ? <small>{said}</small> : null}</span>
      {onRetry && <button type="button" className="app-btn app-btn-small" onClick={onRetry}>{L("重试", "Retry")}</button>}
      <button type="button" className="conv-trouble-x" onClick={onClose} aria-label={L("关掉", "Dismiss")}><X /></button>
    </div>
  );
}
