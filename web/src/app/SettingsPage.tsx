import { clearAll, forgetHub, recall, remember } from "@/lib/memo";
import { CopyButton } from "@/app/copy";
import { IntroCard, introOf, noteDone } from "@/app/Onboard";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle, AudioLines, Bot, Check, Cloud, Globe, Info, Keyboard, Languages, Palette, Pencil, Plus, RotateCcw, ScrollText, Search, Server, Terminal, Users,
} from "lucide-react";
import {
  appApi, type AppInfo, type CacheStats, type AppUpdate, type Hub, type HubOverview, type LocalState,
  type LocalTunnel, type StartLocalHubResult, type VoiceInfo, type VoiceTTSProvider, type TTSVoice, type ProxySetting, type ProxyInput,
} from "@/appapi";
import { ver } from "@/app/nodes";
import { renameHub } from "@/app/hubNames";
import { setHubLook, useHubLooks, type HubLook } from "@/app/hubLooks";
import { colorIndex, HubFace } from "@/app/HubSwitcher";
import { PictureButton } from "@/app/AvatarPicker";
import { ErrorText, Modal, ConfirmDialog } from "@/app/dialogs";
import { accessLabel, localWorkspaceOf, localWorkspaces, type LocalWorkspace } from "@/app/localHubs";
import { AuditSection, MembersSection } from "@/app/MembersSection";
import { LocalAgentsSection } from "@/app/LocalAgents";
import { setSimpleMode, useSimpleMode } from "@/app/simpleMode";
import type { TextSize, ThemeMode } from "@/lib/theme";
import { setReportsOnly, useReportsOnly } from "@/app/quietRuns";
import { appTo, setAutoTranslate, setTranslateReady, useAutoTranslate, useTranslateReady } from "@/app/translate";
import type { TranslateConfig } from "@/appapi";
import { L, lang, type LangChoice } from "@/lib/i18n";
import { bytes, cn } from "@/lib/utils";
import { ChatBgDialog } from "@/app/ChatBgDialog";
import { ShortcutsSection } from "@/app/Shortcuts";
import { HubUpgrade, useLatestRelease } from "@/app/MycoUpgrade";
import { BackupRow } from "@/app/Backups";

type Section = "hubs" | "agents" | "members" | "audit" | "appearance" | "translate" | "voice" | "network" | "shortcuts" | "about";

type UpdateState =
  | { step: "checking" }
  | { step: "checked"; u: AppUpdate }
  | { step: "installing"; latest: string }
  | { step: "relaunching"; latest: string }
  | { step: "failed"; error: unknown; u?: AppUpdate };

// AboutSection shows the app's and the CLI's versions and updates the app.
function AboutSection({ info, local }: { info: AppInfo | null; local: LocalState | null }) {
  const [st, setSt] = useState<UpdateState>({ step: "checking" });
  const check = useCallback(async () => {
    setSt({ step: "checking" });
    try {
      setSt({ step: "checked", u: await appApi.update() });
    } catch (e) {
      setSt({ step: "failed", error: e });
    }
  }, []);
  useEffect(() => { void check(); }, [check]);

  const install = async (u: AppUpdate) => {
    setSt({ step: "installing", latest: u.latest || "" });
    try {
      const r = await appApi.installUpdate();
      setSt({ step: "relaunching", latest: r.installed });
    } catch (e) {
      setSt({ step: "failed", error: e, u });
    }
  };

  let status: React.ReactNode = null;
  let action: React.ReactNode = null;
  switch (st.step) {
    case "checking":
      status = <span className="app-about-status"><span className="app-spinner" />{L("正在检查…", "Checking…")}</span>;
      break;
    case "installing":
      status = <span className="app-about-status"><span className="app-spinner" />{L(`正在下载 ${st.latest}…`, `Downloading ${st.latest}…`)}</span>;
      break;
    case "relaunching":
      status = <span className="app-about-status ok">{L(`已安装 ${st.latest}，正在重新打开…`, `Installed ${st.latest}, reopening…`)}</span>;
      break;
    case "failed":
      status = <span className="app-about-status bad">{L("检查失败", "Check failed")}</span>;
      action = <button type="button" className="app-btn" onClick={() => void check()}>{L("重试", "Retry")}</button>;
      break;
    case "checked": {
      const u = st.u;
      if (u.error) {
        status = <span className="app-about-status bad">{L("连不上 GitHub", "Can’t reach GitHub")}</span>;
        action = <button type="button" className="app-btn" onClick={() => void check()}>{L("重试", "Retry")}</button>;
      } else if (u.available && u.can_install) {
        status = <span className="app-about-status new">{L(`有新版本 ${u.latest}`, `New version ${u.latest}`)}</span>;
        action = <button type="button" className="app-btn app-btn-primary" onClick={() => void install(u)}>{L("更新并重新打开", "Update and reopen")}</button>;
      } else if (u.available) {
        status = <span className="app-about-status new">{L(`有新版本 ${u.latest}，请到 GitHub 下载`, `New version ${u.latest} — download it from GitHub`)}</span>;
      } else {
        status = <span className="app-about-status ok"><Check />{L("已是最新", "Up to date")}</span>;
        action = <button type="button" className="app-btn" onClick={() => void check()}>{L("再检查一次", "Check again")}</button>;
      }
    }
  }
  const failure = st.step === "failed" ? st.error : null;
  const isWindows = /Windows/i.test(navigator.userAgent);
  const cli = local?.myco;

  return (
    <>
      <p className="app-summary">{L("myco App 把多个工作区上的 agent 放进一个窗口。", "The myco app brings the nodes of several Hubs into one window.")}</p>
      <div className="app-list">
        <div className="app-kv"><span className="app-kv-label">{L("App 版本", "App version")}</span><span className="app-kv-value mono">{info?.version || "—"}</span></div>
        <div className="app-kv">
          <span className="app-kv-label">{L("命令行工具", "Command-line tool")}</span>
          <span className="app-kv-value">
            {cli ? <><span className="mono">{ver(cli.version)}</span><span className="app-about-path mono">{cli.path}</span></> : L("未安装", "Not installed")}
          </span>
        </div>
        <div className="app-kv app-about-update">
          <span className="app-kv-label">{L("更新", "Updates")}</span>
          <span className="app-kv-value">{status}{action}</span>
        </div>
        <CacheRow />
        <div className="app-kv app-about-update" data-setting="prepare">
          <span className="app-kv-label">{L("上手引导", "Getting started")}</span>
          <span className="app-kv-value">
            <span className="app-about-status">{L("看看这台电脑上的 agent，把已有的会话加进来", "The agents on this computer, and sessions to bring in")}</span>
            <button type="button" className="app-btn app-btn-quiet" onClick={() => window.dispatchEvent(new CustomEvent("myco-prepare"))}>{L("重新打开", "Open")}</button>
          </span>
        </div>
      </div>
      <ErrorText error={failure} />
      <p className="app-about-note">
        {isWindows
          ? lang() === "zh"
            ? <>更新会下载 GitHub 上最新发布的 App，校验 SHA256 后替换正在运行的这个 exe，再自动重新打开；这台电脑上的 myco 命令行工具也会一起升级。</>
            : <>Updating downloads the latest app release from GitHub, verifies its SHA256, replaces this exe and reopens it; the myco command-line tool on this computer is upgraded along with it.</>
          : lang() === "zh"
            ? <>更新会下载 GitHub 上最新发布的 App，校验 SHA256 后替换「应用程序」里的 myco，再自动重新打开。命令行工具和 runner 服务另外用 <code>myco upgrade</code> 升级。</>
            : <>Updating downloads the latest app release from GitHub, verifies its SHA256, replaces myco in Applications and reopens it. Upgrade the command-line tool and the runner service separately with <code>myco upgrade</code>.</>}
      </p>
      <p className="app-about-note">{L("agent、任务和对话都由工作区提供，App 只负责显示和转发。", "Nodes, tasks and conversations come from the Hub; the app only displays and relays them.")}</p>
    </>
  );
}

/** The saved page data (app-cache.db): its size, and a way to empty it. */
function CacheRow() {
  const [stats, setStats] = useState<CacheStats | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(() => { appApi.cacheStats().then(setStats).catch(() => setStats(null)); }, []);
  useEffect(load, [load]);
  if (!stats?.enabled) return null;
  const clear = async () => {
    setBusy(true);
    setError(null);
    try {
      await clearAll();
      load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="app-kv app-about-update">
      <span className="app-kv-label">{L("本机缓存", "Local cache")}</span>
      <span className="app-kv-value">
        <span className="app-about-status">{stats.entries ? bytes(stats.bytes) : L("空", "Empty")}</span>
        <button type="button" className="app-btn" disabled={busy || !stats.entries} onClick={() => void clear()}
          title={L("页面上次显示的内容（加密存在这台电脑上），App 重开时先显示它", "What pages showed last (stored encrypted on this computer), shown first when the app reopens")}>
          {L("清除", "Clear")}
        </button>
        <ErrorText error={error} />
      </span>
    </div>
  );
}

const SECTIONS: { key: Section; label: string; icon: typeof Server }[] = [
  { key: "hubs", get label() { return L("工作区", "Workspaces"); }, icon: Server },
  { key: "agents", get label() { return L("本机 agent", "Agents"); }, icon: Bot },
  { key: "members", get label() { return L("成员", "People"); }, icon: Users },
  { key: "audit", get label() { return L("审计", "Audit"); }, icon: ScrollText },
  { key: "appearance", get label() { return L("外观", "Appearance"); }, icon: Palette },
  { key: "translate", get label() { return L("翻译", "Translation"); }, icon: Languages },
  { key: "voice", get label() { return L("语音", "Voice"); }, icon: AudioLines },
  { key: "network", get label() { return L("代理", "Proxy"); }, icon: Globe },
  { key: "shortcuts", get label() { return L("快捷键", "Shortcuts"); }, icon: Keyboard },
  { key: "about", get label() { return L("关于", "About"); }, icon: Info },
];

/** The nav in groups: what is about this computer, what is about the
 *  workspace (shared with the others in it), and About. */
const GROUPS: { title: () => string; keys: Section[] }[] = [
  { title: () => L("这台电脑", "This computer"), keys: ["appearance", "voice", "translate", "network", "agents"] },
  { title: () => L("工作区", "Workspace"), keys: ["hubs", "members", "audit"] },
  { title: () => "", keys: ["shortcuts", "about"] },
];

/** Each section's one line, under its title. */
const SECTION_ABOUT: Record<Section, () => string> = {
  hubs: () => L("工作区是你的 agent 碰头的地方，跑在某一台电脑或服务器上。所有页面只显示左下角选中的那个。", "A workspace is where your agents meet; it runs on a computer or a server. Every page shows the one picked at the bottom left."),
  agents: () => L("这台电脑上装了哪些 AI 命令行，能不能用，怎么装、怎么登录。", "The AI command lines on this computer: whether they work, and how to install or sign in."),
  members: () => L("谁在这个工作区里、能做什么；邀请新的人。", "Who is in this workspace and what they may do; invite someone."),
  audit: () => L("工作区里谁在什么时候做了什么。", "Who did what in this workspace, and when."),
  appearance: () => L("主题、文字大小、语言，以及对话怎么显示。", "Theme, text size, language, and how chats look."),
  translate: () => L("把 agent 说的话翻译成你的语言。", "Translate what agents say into your language."),
  voice: () => L("和 agent 语音聊天，以及语音输入。", "Voice chat with agents, and dictation."),
  network: () => L("这台电脑连外网时走的代理。", "The proxy this computer uses to reach the internet."),
  shortcuts: () => L("App 里能用的键盘快捷键。", "The keyboard shortcuts in this app."),
  about: () => L("版本、更新和这个 App。", "Version, updates and this app."),
};

/** What Settings can find (search at the top of its nav): a row of a
 *  section (data-setting on it), its words in both languages. */
const FINDABLE: { section: Section; key?: string; title: () => string; words: string }[] = [
  { section: "appearance", key: "theme", title: () => L("外观（浅色 / 深色）", "Appearance (light / dark)"), words: "外观 主题 浅色 深色 跟随系统 theme dark light appearance" },
  { section: "appearance", key: "simple-mode", title: () => L("简单模式", "Simple mode"), words: "简单模式 完整 simple mode" },
  { section: "appearance", key: "text-size", title: () => L("文字大小", "Text size"), words: "文字大小 字号 缩放 放大 text size zoom font" },
  { section: "appearance", key: "steps", title: () => L("对话里的过程（只看进度）", "Steps in chats"), words: "对话里的过程 只看进度 思考 命令 steps progress" },
  { section: "appearance", key: "chat-bg", title: () => L("聊天背景", "Chat background"), words: "聊天背景 壁纸 背景图 chat background wallpaper" },
  { section: "appearance", key: "island", title: () => L("灵动岛", "Notch island"), words: "灵动岛 刘海 island notch" },
  { section: "appearance", key: "language", title: () => L("语言", "Language"), words: "语言 中文 英文 language english chinese" },
  { section: "voice", key: "voice-chat", title: () => L("和 agent 语音聊天", "Voice chat with an agent"), words: "语音 聊天 麦克风 朗读 voice chat microphone speech tts" },
  { section: "voice", key: "voice-keep", title: () => L("把语音设置存到工作区", "Keep voice settings on the workspaces"), words: "语音 设置 存到工作区 同步 voice sync" },
  { section: "translate", key: "translate", title: () => L("自动翻译", "Translate automatically"), words: "翻译 自动翻译 密钥 api key translate deepl" },
  { section: "network", key: "proxy", title: () => L("代理", "Proxy"), words: "代理 网络 http socks proxy network" },
  { section: "agents", key: "local-agents", title: () => L("这台电脑上的 agent", "Agents on this computer"), words: "本机 agent claude code codex gemini 安装 登录 cli install sign in" },
  { section: "hubs", title: () => L("工作区", "Workspaces"), words: "工作区 服务器 连接 加入 新建 停止 公网 cloudflare tailscale workspace hub server" },
  { section: "members", title: () => L("成员", "People"), words: "成员 邀请 角色 管理员 权限 people invite role admin member" },
  { section: "audit", title: () => L("审计", "Audit"), words: "审计 日志 记录 操作 audit log" },
  { section: "shortcuts", title: () => L("快捷键", "Keyboard shortcuts"), words: "快捷键 键盘 按键 热键 shortcuts keyboard hotkeys keys" },
  { section: "about", title: () => L("关于", "About"), words: "关于 版本 更新 升级 about version update upgrade" },
  { section: "about", key: "prepare", title: () => L("上手引导", "Getting started"), words: "上手 引导 新手 准备这台电脑 onboarding getting started setup guide" },
];

/** What 简单模式 leaves out of Settings. */
// (工作区 stays: even with only the one on this computer, opening it to the
// internet, its name and stopping it are there)
const SIMPLE_HIDDEN: Section[] = ["members", "audit"];
function simpleModeNow(): boolean {
  try { return localStorage.getItem("myco-simple-mode") === "1"; } catch { return false; }
}

const SIZES: { size: TextSize; label: string }[] = [
  { size: "small", get label() { return L("小", "Small"); } },
  { size: "standard", get label() { return L("标准", "Standard"); } },
  { size: "large", get label() { return L("大", "Large"); } },
];

const THEMES: { mode: ThemeMode; label: string }[] = [
  { mode: "system", get label() { return L("跟随系统", "System"); } },
  { mode: "light", get label() { return L("浅色", "Light"); } },
  { mode: "dark", get label() { return L("深色", "Dark"); } },
];

/** Language names stay in their own language so either reader can find theirs. */
const LANGS: { choice: LangChoice; label: string }[] = [
  { choice: "system", get label() { return L("跟随系统", "System"); } },
  { choice: "zh", label: "中文" },
  { choice: "en", label: "English" },
];

/** A section another page asks Settings to open on (say, voice from the
 *  call button); taken once. */
let nextSettingsSection: Section | null = null;
export function openSettingsSection(s: "voice" | "about" | "agents" | "hubs" | "shortcuts") { nextSettingsSection = s; }
/** (read as the page starts; cleared once it is up, so a second run of
 *  its first render, as React's strict mode does, reads the same) */
function takeSettingsSection(): Section | null {
  return nextSettingsSection;
}

/** The section to come back to when a language change remounts the page. */
let langSwitchSection: Section | null = null;

function stripSlash(u: string) {
  return u.replace(/\/+$/, "");
}

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

const TUNNELS: { key: LocalTunnel; title: string; desc: string; needsDomain?: boolean; missing?: string }[] = [
  {
    key: "cloudflare",
    get title() { return L("一键开放", "One click"); },
    get desc() { return L("不用账号，也不用装别的：一个 Cloudflare 临时地址。这台电脑重启后地址会变，到时再点一次就行。", "No account, nothing to install: a temporary Cloudflare address. It changes when this computer restarts; click again then."); },
    get missing() { return L("不用账号：第一次会自动下载 Cloudflare 的小工具（cloudflared，约 20 MB）。这台电脑重启后地址会变。", "No account: the first time, Cloudflare's small tool (cloudflared, about 20 MB) is downloaded for you. The address changes when this computer restarts."); },
  },
  {
    key: "cloudflare",
    get title() { return L("Cloudflare 自己的域名", "Cloudflare with your own domain"); },
    get desc() { return L("需要 Cloudflare 账号和托管在 Cloudflare 的域名，地址固定。", "Needs a Cloudflare account and a domain hosted on Cloudflare. The address stays the same."); },
    needsDomain: true,
    get missing() { return L("需要先安装 cloudflared。", "Install cloudflared first."); },
  },
  {
    key: "tailscale",
    title: "Tailscale Funnel",
    get desc() { return L("地址固定（xxx.ts.net），需要 Tailscale 账号并为这台设备开启 Funnel。", "Fixed address (xxx.ts.net). Needs a Tailscale account with Funnel turned on for this device."); },
    get missing() { return L("需要先安装 Tailscale。", "Install Tailscale first."); },
  },
];

type TunnelChoice = 0 | 1 | 2;

/** Settings: hubs (including the local one), appearance and about. */
export function SettingsPage({ hubs, hubsOv, info, theme, onTheme, textSize, onTextSize, langChoice, onLangChoice, refreshKey, sectionRequest, onRefresh, onAddHub, titleId }: {
  hubs: Hub[];
  hubsOv?: HubOverview[];
  info: AppInfo | null;
  theme: ThemeMode;
  onTheme: (mode: ThemeMode) => void;
  textSize: TextSize;
  onTextSize: (size: TextSize) => void;
  langChoice: LangChoice;
  onLangChoice: (c: LangChoice) => void;
  /** Bumped by the App poll so the local hub state stays current. */
  refreshKey: number;
  /** Bumped when the Hub switcher asks for the Hub section. */
  sectionRequest?: number;
  onRefresh: () => Promise<void>;
  onAddHub: () => void;
  /** in a dialog: the title at the top of the nav (labels the dialog) */
  titleId?: string;
}) {
  const simple = useSimpleMode();
  const [section, setSection] = useState<Section>(() => takeSettingsSection() ?? langSwitchSection ?? (simpleModeNow() ? "agents" : "hubs"));
  const askedFor = useRef<Section | null>(takeSettingsSection());
  useEffect(() => { nextSettingsSection = null; }, []);
  // Asked for a section while already here (the 上手清单, a chat's voice
  // button): go to it.
  useEffect(() => {
    const on = (e: Event) => {
      const s = (e as CustomEvent<{ section?: string }>).detail?.section;
      if (s && SECTIONS.some((x) => x.key === s)) { askedFor.current = s as Section; setSection(s as Section); }
    };
    window.addEventListener("myco-open-settings", on);
    return () => window.removeEventListener("myco-open-settings", on);
  }, []);
  // Being on 工作区 is the 上手清单's 认识你的工作区.
  useEffect(() => { if (section === "hubs") noteDone("workspaces"); }, [section]);
  // Simple mode hides the workspace, people and audit sections.
  // (简单模式 hides some; one asked for by name, as the 上手清单 does, shows while on it)
  const sections = SECTIONS.filter((s) => !simple || !SIMPLE_HIDDEN.includes(s.key) || s.key === section);
  // Search: rows of any section, by their words (each word typed must be
  // there); picking one opens its section and points the row out.
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const [pointAt, setPointAt] = useState<{ key: string; n: number } | null>(null);
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const found = FINDABLE.filter((f) => sections.some((x) => x.key === f.section)
    && words.every((w) => `${f.title()} ${f.words} ${SECTIONS.find((x) => x.key === f.section)?.label || ""}`.toLowerCase().includes(w)));
  const goTo = (f: (typeof FINDABLE)[number]) => {
    setSection(f.section);
    setQuery("");
    if (f.key) setPointAt((p) => ({ key: f.key!, n: (p?.n || 0) + 1 }));
    else bodyRef.current?.scrollTo({ top: 0 });
  };
  useEffect(() => {
    if (!pointAt) return;
    // (its section may still be drawing: a few tries)
    let tries = 0;
    const t = window.setInterval(() => {
      const el = bodyRef.current?.querySelector<HTMLElement>(`[data-setting="${pointAt.key}"]`);
      if (el || ++tries > 20) {
        window.clearInterval(t);
        if (!el) return;
        el.scrollIntoView({ block: "center", behavior: "smooth" });
        el.classList.remove("is-found");
        void el.offsetWidth;
        el.classList.add("is-found");
        window.setTimeout(() => el.classList.remove("is-found"), 1800);
      }
    }, 60);
    return () => window.clearInterval(t);
  }, [pointAt]);
  // ⌘F (Ctrl+F) on Settings: its search.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && (e.key === "f" || e.key === "F")) {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, []);
  // (one asked for by name stays: the 上手清单 opens 工作区 in 简单模式 too)
  useEffect(() => { if (simple && SIMPLE_HIDDEN.includes(section) && section !== askedFor.current) setSection("agents"); }, [simple, section]);
  // After a language remount, the request that was already handled must not
  // pull the page back to the Hub section.
  const handledRequest = useRef(langSwitchSection !== null ? sectionRequest : undefined);
  useEffect(() => { langSwitchSection = null; }, []);
  const [local, setLocalState] = useState<LocalState | null>(() => recall<LocalState | null>("local") ?? null);
  const setLocal = (d: LocalState | null) => { if (d) remember("local", d); setLocalState(d); };
  const [localError, setLocalError] = useState<unknown>(null);
  // Which local workspace is starting or stopping ("start:<name>").
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [renameTarget, setRenameTarget] = useState<Hub | null>(null);
  const hubLooks = useHubLooks();
  const [renameLook, setRenameLook] = useState<HubLook>({});
  const [lookError, setLookError] = useState<unknown>(null);
  const [renameValue, setRenameValue] = useState("");
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [stopTarget, setStopTarget] = useState<LocalWorkspace | null>(null);
  const [publicTarget, setPublicTarget] = useState<LocalWorkspace | null>(null);
  const latest = useLatestRelease();

  // "Hub 设置" in the switcher should land on the Hub list, even when this
  // page was last left on another section.
  useEffect(() => {
    if (sectionRequest && sectionRequest !== handledRequest.current) setSection("hubs");
  }, [sectionRequest]);

  const reloadLocal = useCallback(async () => {
    try {
      const d = await appApi.local();
      setLocal(d);
      setLocalError(null);
    } catch (e) {
      setLocalError(e);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    appApi.local()
      .then((d) => { if (!cancelled) { setLocal(d); setLocalError(null); } })
      .catch((e) => { if (!cancelled) setLocalError(e); });
    return () => { cancelled = true; };
  }, [refreshKey, reloadLocal]);

  const isLocalHub = (h: Hub) => !!localWorkspaceOf(local, h.url);

  const saveRename = () => {
    if (!renameTarget) return;
    renameHub(renameTarget.id, renameValue);
    setHubLook(renameTarget.id, renameLook);
    setRenameTarget(null);
  };
  const openRename = (h: Hub) => {
    setRenameTarget(h);
    setRenameValue(h.name);
    setRenameLook(hubLooks[h.id] || {});
    setLookError(null);
  };

  const startLocal = async (name?: string) => {
    setBusy(`start:${name || ""}`);
    setActionError(null);
    try {
      await appApi.startLocalHub({ tunnel: "none", name: name || undefined });
      await reloadLocal();
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(null);
    }
  };

  const stopLocal = async () => {
    if (!stopTarget) return;
    const name = stopTarget.name;
    setBusy(`stop:${name}`);
    setActionError(null);
    try {
      await appApi.stopLocalHub(name || undefined);
      setStopTarget(null);
      await reloadLocal();
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(null);
    }
  };

  const removeHub = async () => {
    if (!removeTarget) return;
    setActionError(null);
    try {
      await appApi.removeHub(removeTarget.id);
      forgetHub(removeTarget.id);
      setRemoveTarget(null);
      await onRefresh();
    } catch (e) {
      setActionError(e);
    }
  };

  const localWs = localWorkspaces(local);
  const otherHubs = hubs.filter((h) => !isLocalHub(h));

  return (
    <div className="app-page app-settings">
      <div className="app-settings-nav" role="tablist" aria-label={L("设置", "Settings")}>
        {titleId && <h2 id={titleId} className="app-settings-title">{L("设置", "Settings")}</h2>}
        <label className="app-settings-search">
          <Search />
          <input ref={searchRef} value={query} onChange={(e) => setQuery(e.target.value)} placeholder={L("搜索设置", "Search settings")}
            aria-label={L("搜索设置", "Search settings")}
            onKeyDown={(e) => {
              // (a search to clear takes Esc; an empty one leaves it to close Settings)
              if (e.key === "Escape") { if (query) { e.preventDefault(); setQuery(""); } return; }
              if (e.key === "Enter" && found[0]) { e.preventDefault(); goTo(found[0]); }
            }} />
        </label>
        {query.trim() ? (
          found.length ? found.map((f) => (
            <button key={`${f.section}/${f.key || ""}`} type="button" className="app-settings-result" onClick={() => goTo(f)}>
              <span>{f.title()}</span>
              <small>{SECTIONS.find((x) => x.key === f.section)?.label}</small>
            </button>
          )) : <div className="app-settings-none">{L("没有找到", "Nothing found")}</div>
        ) : GROUPS.map((g, gi) => {
          const items = g.keys.map((k) => sections.find((x) => x.key === k)).filter((x): x is (typeof sections)[number] => !!x);
          if (!items.length) return null;
          return (
            <div key={gi} className="app-settings-group">
              {g.title() && <div className="app-settings-group-title">{g.title()}</div>}
              {items.map((s) => (
                <button
                  key={s.key}
                  type="button"
                  role="tab"
                  aria-selected={section === s.key}
                  className={cn("app-settings-tab", section === s.key && "active")}
                  onClick={() => setSection(s.key)}
                >
                  <s.icon strokeWidth={1.75} />
                  <span>{s.label}</span>
                </button>
              ))}
            </div>
          );
        })}
      </div>

      <div className="app-settings-body" ref={bodyRef}>
        <header className="app-settings-head">
          <h2>{SECTIONS.find((x) => x.key === section)?.label}</h2>
          <p>{SECTION_ABOUT[section]()}</p>
        </header>
        {section === "hubs" && (
          <>
            <IntroCard id="workspaces" {...introOf("workspaces")} />
            <ErrorText error={localError} />
            <ErrorText error={actionError} />

            {/* The workspaces this computer runs: one row each, with what they
                are reachable at and how they are opened. */}
            {localWs.length > 0 && (
              <h4 className="app-setting-group">{L("这台电脑上的工作区", "Workspaces on this computer")}</h4>
            )}
            {localWs.map((w) => {
              const urls = [w.local_url, w.public_url].filter(Boolean).map((u) => stripSlash(u!));
              const linked = hubs.find((h) => urls.includes(stripSlash(h.url)));
              const name = linked?.name || w.name || L("默认工作区", "Default workspace");
              const starting = busy === `start:${w.name}`;
              const stopping = busy === `stop:${w.name}`;
              return (
                <section className="app-setting-card" key={`local:${w.name}`} data-local-ws={w.name || "default"}>
                  <div className="app-setting-head">
                    <HubFace name={name} id={linked?.id || `local:${w.name}`} size={32} />
                    <div className="titles">
                      <h3>{name}</h3>
                      <p className="mono">{w.local_url}</p>
                    </div>
                    <span className="app-setting-tag">{L("这台电脑", "This computer")}</span>
                    {linked && (
                      <button
                        type="button"
                        className="app-btn app-btn-plain"
                        onClick={() => openRename(linked)}
                        aria-label={L("改名字和头像", "Name and face")}
                        title={L("改名字和头像", "Name and face")}
                      >
                        <Pencil />
                      </button>
                    )}
                  </div>

                  <div className="app-setting-rows">
                    <div className="app-kv"><span className="app-kv-label">{L("地址", "Address")}</span><span className="app-kv-value mono clip" title={w.local_url}>{w.local_url}</span></div>
                    <div className="app-kv"><span className="app-kv-label">{L("版本", "Version")}</span><span className="app-kv-value">{(w.version || linked?.hub_version) ? ver(w.version || linked?.hub_version) : "—"}</span></div>
                    <div className="app-kv">
                      <span className="app-kv-label">{L("状态", "Status")}</span>
                      <span className="app-kv-value">
                        <span className={cn("app-status-text", !w.running && "is-bad")}>
                          <span className={cn("dot", w.running ? "working" : "offline")} />
                          {w.running ? L("运行中", "Running") : L("已停止", "Stopped")}
                        </span>
                      </span>
                    </div>
                    <div className="app-kv">
                      <span className="app-kv-label">{L("访问", "Access")}</span>
                      <span className="app-kv-value">
                        {accessLabel(w)}
                        {w.running && w.public_url && (
                          <span className="app-kv-sub mono" title={w.public_url}>{w.public_url}</span>
                        )}
                      </span>
                    </div>
                    {linked && <E2ERow hubId={linked.id} ov={hubsOv?.find((o) => o.id === linked.id)} onChanged={onRefresh} />}
                    {linked && w.running && <BackupRow hubId={linked.id} me={hubsOv?.find((o) => o.id === linked.id)?.me} />}
                  </div>
                  {w.running && <HubUpgrade version={w.version || linked?.hub_version} local manages latest={latest} onDone={() => { void reloadLocal(); onRefresh(); }} />}
                  {!w.running && (
                    <div className="app-small-note" style={{ marginTop: 8 }}>{L("这个工作区已停止，点「启动」重新运行。", "This workspace is stopped. Start it again to bring it back.")}</div>
                  )}

                  <div className="app-setting-actions">
                    <button type="button" className="app-btn" onClick={() => setPublicTarget(w)} disabled={busy !== null}>
                      <Globe />{L("开放到公网", "Open to the internet")}
                    </button>
                    {w.running ? (
                      <button type="button" className="app-btn app-btn-quiet-danger" onClick={() => setStopTarget(w)} disabled={busy !== null}>
                        {stopping ? <><span className="app-spinner" />{L("正在停止…", "Stopping…")}</> : L("停止", "Stop")}
                      </button>
                    ) : (
                      <button type="button" className="app-btn app-btn-primary" onClick={() => void startLocal(w.name)} disabled={busy !== null}>
                        {starting ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : L("启动", "Start")}
                      </button>
                    )}
                  </div>
                </section>
              );
            })}

            {otherHubs.map((h) => (
              <section className="app-setting-card" key={h.id}>
                <div className="app-setting-head">
                  <HubFace name={h.name} id={h.id} size={32} />
                  <div className="titles">
                    <h3>{h.name}</h3>
                    <p className="mono">{host(h.url)}</p>
                  </div>
                  <button
                    type="button"
                    className="app-btn app-btn-plain"
                    onClick={() => openRename(h)}
                    aria-label={L("改名字和头像", "Name and face")}
                    title={L("改名字和头像", "Name and face")}
                  >
                    <Pencil />
                  </button>
                </div>

                <div className="app-setting-rows">
                  <div className="app-kv"><span className="app-kv-label">{L("地址", "Address")}</span><span className="app-kv-value mono clip" title={h.url}>{h.url}</span></div>
                  <div className="app-kv"><span className="app-kv-label">{L("版本", "Version")}</span><span className="app-kv-value">{h.hub_version ? ver(h.hub_version) : "—"}</span></div>
                  <div className="app-kv">
                    <span className="app-kv-label">{L("状态", "Status")}</span>
                    <span className="app-kv-value">
                      <span className={cn("app-status-text", !h.ok && "is-bad")}>
                        <span className={cn("dot", h.ok ? "working" : "offline")} />
                        {h.ok ? L("已连接", "Connected") : L("连不上", "Unreachable")}
                      </span>
                    </span>
                  </div>
                  <E2ERow hubId={h.id} ov={hubsOv?.find((o) => o.id === h.id)} onChanged={onRefresh} />
                  {h.ok && <BackupRow hubId={h.id} me={hubsOv?.find((o) => o.id === h.id)?.me} />}
                </div>
                {h.ok && <HubUpgrade hubId={h.id} version={h.hub_version} local={false} manages={!!hubsOv?.find((o) => o.id === h.id)?.me?.can_manage} latest={latest} onDone={onRefresh} />}

                <div className="app-setting-actions">
                  <button type="button" className="app-btn app-btn-quiet-danger" onClick={() => { setActionError(null); setRemoveTarget(h); }}>
                    {L("移除", "Remove")}
                  </button>
                </div>
              </section>
            ))}

            {hubs.length === 0 && localWs.length === 0 && (
              <div className="app-empty-note" style={{ marginTop: 8 }}>{L("还没有工作区。先连接一个，或者在这台电脑上启动一个。", "No workspaces yet. Connect one, or start one on this computer.")}</div>
            )}

            <div className="app-setting-add">
              <button type="button" className="app-setting-add-row" onClick={onAddHub}>
                <Plus />{L("添加工作区", "Add workspace")}
              </button>
              {localWs.length === 0 && (
                <button
                  type="button"
                  className="app-setting-add-row"
                  onClick={() => void startLocal()}
                  disabled={busy !== null}
                >
                  {busy === "start:" ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : <><Terminal />{L("在这台电脑上启动工作区", "Start a workspace on this computer")}</>}
                </button>
              )}
            </div>
            <div className="app-hint">
              {L("名称只在这个 App 里显示，不会改到工作区上。地址和令牌保存在这台电脑上。", "Names show only in this app and don’t change the workspace. Addresses and tokens are saved on this computer.")}
            </div>
          </>
        )}

        {section === "shortcuts" && <ShortcutsSection />}
        {section === "appearance" && (
          <>
            <section className="app-setting-card app-setting-lines">
              <div data-setting="theme" className="app-setting-line">
                <div className="app-setting-line-text">
                  <b>{L("外观", "Appearance")}</b>
                  <span>{L("跟随系统时，App 会随 macOS 的浅色/深色切换。", "With System, the app follows macOS light and dark mode.")}</span>
                </div>
                <div className="seg" role="tablist" aria-label={L("外观", "Appearance")}>
                  {THEMES.map((t) => (
                    <button
                      key={t.mode}
                      type="button"
                      role="tab"
                      aria-selected={theme === t.mode}
                      className={cn("seg-btn", theme === t.mode && "active")}
                      onClick={() => onTheme(t.mode)}
                    >
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>
              <div className="app-setting-line" data-setting="simple-mode">
                <div className="app-setting-line-text">
                  <b>{L("简单模式", "Simple mode")}</b>
                  <span>{L("一个人在这台电脑上用：只显示对话和 agent，工作区、成员、其他电脑这些先藏起来。关掉就是完整模式，什么都不会删。",
                    "One person on this computer: only conversations and agents show; workspaces, people and other computers are hidden. Turn it off for everything; nothing is removed.")}</span>
                </div>
                <button type="button" role="switch" aria-checked={simple} aria-label={simple ? L("关掉简单模式", "Turn simple mode off") : L("打开简单模式", "Turn simple mode on")}
                  title={simple ? L("关掉简单模式", "Turn simple mode off") : L("打开简单模式", "Turn simple mode on")}
                  className={cn("app-switch", simple && "is-on")} onClick={() => setSimpleMode(!simple)}><span className="app-switch-knob" /></button>
              </div>
              <div data-setting="text-size" className="app-setting-line">
                <div className="app-setting-line-text">
                  <b>{L("文字大小", "Text size")}</b>
                  <span>{L("整个窗口一起放大或缩小。", "Scales the whole window up or down.")}</span>
                </div>
                <div className="seg" role="tablist" aria-label={L("文字大小", "Text size")}>
                  {SIZES.map((t) => (
                    <button
                      key={t.size}
                      type="button"
                      role="tab"
                      aria-selected={textSize === t.size}
                      className={cn("seg-btn", textSize === t.size && "active")}
                      onClick={() => onTextSize(t.size)}
                    >
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>
              <ReportsOnlyLine />
              <ChatBgLine />
              {info?.shell === "mac" && <IslandLine />}
              <div data-setting="language" className="app-setting-line">
                <div className="app-setting-line-text">
                  <b>语言 / Language</b>
                  <span>{L("跟随系统时，App 使用 macOS 的语言设置。", "With System, the app uses the macOS language.")}</span>
                </div>
                <div className="seg" role="tablist" aria-label="语言 / Language">
                  {LANGS.map((t) => (
                    <button
                      key={t.choice}
                      type="button"
                      role="tab"
                      aria-selected={langChoice === t.choice}
                      className={cn("seg-btn", langChoice === t.choice && "active")}
                      onClick={() => { langSwitchSection = section; onLangChoice(t.choice); }}
                    >
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>
            </section>
          </>
        )}

        {section === "translate" && <TranslateSection />}
        {section === "agents" && <LocalAgentsSection />}
        {section === "voice" && <VoiceSection />}
        {section === "network" && <ProxySection hubsOv={hubsOv} />}
        {section === "about" && <AboutSection info={info} local={local} />}
        {section === "members" && <MembersSection hubsOv={hubsOv} onRefresh={onRefresh} />}
        {section === "audit" && <AuditSection hubsOv={hubsOv} />}
      </div>

      <Modal
        open={!!renameTarget}
        onOpenChange={(o) => { if (!o) setRenameTarget(null); }}
        title={L("工作区的名字和头像", "The workspace's name and face")}
        description={L("只在这个 App 里显示，不会改到工作区上。", "Shows only in this app and doesn’t change the Hub.")}
      >
        <div className="av-top">
          <HubFace name={renameValue || renameTarget?.name} id={renameTarget?.id} look={renameLook} size={56} />
          <div className="av-top-actions">
            <PictureButton onPicture={(pic) => setRenameLook({ ...renameLook, pic })} onError={setLookError} />
            {(renameLook.pic || renameLook.color !== undefined) && (
              <button type="button" className="app-btn app-btn-quiet" onClick={() => setRenameLook({})}>
                <RotateCcw />{L("用回原来的", "Back to the original")}
              </button>
            )}
          </div>
        </div>
        <div className="av-colors is-hub" role="radiogroup" aria-label={L("颜色", "Colour")}>
          {Array.from({ length: 8 }, (_, i) => {
            const on = !renameLook.pic && (renameLook.color ?? colorIndex(renameTarget?.id || "")) === i;
            return (
              <button key={i} type="button" role="radio" aria-checked={on} className={cn("av-color app-hub-avatar", `c${i}`, on && "is-on")}
                aria-label={L(`颜色 ${i + 1}`, `Colour ${i + 1}`)} onClick={() => setRenameLook({ color: i })}>
                {on && <Check />}
              </button>
            );
          })}
        </div>
        <ErrorText error={lookError} />
        <label className="app-field">
          <span className="app-field-label">{L("名称", "Name")}</span>
          <input
            className="app-input"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            autoFocus
            placeholder={renameTarget?.name}
          />
        </label>
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setRenameTarget(null)}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveRename}>{L("保存", "Save")}</button>
        </div>
      </Modal>

      <PublicAccessDialog
        open={!!publicTarget}
        onOpenChange={(o) => !o && setPublicTarget(null)}
        local={local}
        name={publicTarget?.name}
        // (stays open on its result: the address to copy; 完成 closes it)
        onDone={() => { void reloadLocal(); void onRefresh(); }}
      />

      <ConfirmDialog
        open={!!stopTarget}
        onOpenChange={(o) => !o && setStopTarget(null)}
        title={L("停止这个工作区？", "Stop this workspace?")}
        description={L("它会在这台电脑上停止运行。", "It will stop running on this computer.")}
        consequences={<p>{L("其他电脑会连不上它。数据和令牌都保留，重新开启即可恢复。", "Other machines won’t be able to reach it. Data and tokens are kept; start it again to bring it back.")}</p>}
        confirmLabel={L("停止", "Stop")}
        busy={busy === `stop:${stopTarget?.name || ""}`}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void stopLocal()}
      />

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => !o && setRemoveTarget(null)}
        title={removeTarget ? L(`移除 ${removeTarget.name}？`, `Remove ${removeTarget.name}?`) : L("移除工作区", "Remove Hub")}
        description={L("只会从这个 App 里移除。工作区本身和它上面的 agent 都不受影响，仍在照常运行。", "Removes it from this app only. The Hub and its agents aren’t affected and keep running.")}
        consequences={<p>{L("你将看不到这个工作区的 agent，之后只能凭 App 连接链接重新添加。", "You won’t see this Hub’s nodes. To add it back later, you’ll need its app link.")}</p>}
        confirmLabel={L("移除工作区", "Remove Hub")}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void removeHub()}
      />
    </div>
  );
}

/** Reconfigure the local hub behind a public tunnel. */
export function PublicAccessDialog({ open, onOpenChange, local, name, onDone }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  local: LocalState | null;
  /** which hub on this computer ("") for the default one */
  name?: string;
  onDone: () => void;
}) {
  const [choice, setChoice] = useState<TunnelChoice>(0);
  const [hostname, setHostname] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<StartLocalHubResult | null>(null);

  const cf = !!local?.tools?.cloudflared;
  const ts = !!local?.tools?.tailscale;

  useEffect(() => {
    if (!open) return;
    setChoice(0);
    setHostname("");
    setBusy(false);
    setError(null);
    setResult(null);
  }, [open, cf, ts]);

  // The one-click way gets cloudflared itself; the others need their tool.
  const disabled = (i: TunnelChoice) =>
    i === 0 ? false : i === 1 ? !cf : !ts;

  // Opening can take a minute or two (Funnel's certificate, a tunnel's
  // DNS): say how long, and let it be cancelled.
  const abort = useRef<AbortController | null>(null);
  const [since, setSince] = useState(0);
  const [, tick] = useState(0);
  useEffect(() => {
    if (!busy) return;
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [busy]);
  useEffect(() => () => abort.current?.abort(), []);
  const cancel = () => {
    abort.current?.abort();
    abort.current = null;
    setBusy(false);
  };

  const submit = async () => {
    setBusy(true);
    setError(null);
    setSince(Date.now());
    const ctl = new AbortController();
    abort.current = ctl;
    const which = name ? { name } : {};
    const input = choice === 1
      ? { tunnel: "cloudflare" as const, hostname: hostname.trim(), ...which }
      : choice === 2
        ? { tunnel: "tailscale" as const, ...which }
        : { tunnel: "cloudflare" as const, ...which };
    try {
      setResult(await appApi.startLocalHub(input, ctl.signal));
      onDone();
    } catch (e) {
      if (!ctl.signal.aborted) setError(e);
    } finally {
      if (abort.current === ctl) {
        abort.current = null;
        setBusy(false);
      }
    }
  };

  const close = () => { cancel(); onOpenChange(false); };
  const waited = busy && since ? Math.floor((Date.now() - since) / 1000) : 0;

  return (
    <Modal
      open={open}
      onOpenChange={(o) => { if (!o) close(); }}
      title={L("开放到公网", "Open to the internet")}
      description={L("让不在同一局域网的电脑也能连上这台电脑上的工作区。", "Let machines outside your local network reach the Hub on this computer.")}
      className="pub-modal"
    >
      {result ? (
        <>
          <div className="pub-done" data-testid="opened">
            <span className="pub-done-icon"><Check /></span>
            <span><b>{L("已开放到公网", "Open to the internet")}</b>
              <small>{L("别的电脑和同事现在可以用下面的地址连上这个工作区。", "Other computers and people can now reach this workspace at the address below.")}</small></span>
          </div>
          <div className="pub-links">
            <div className="pub-link"><span className="pub-link-label">{L("公网地址", "Public address")}</span>
              <code title={result.public_url}>{result.public_url}</code><CopyButton value={result.public_url} /></div>
            {result.join_url && (
              <div className="pub-link"><span className="pub-link-label">{L("入网链接", "Join link")}</span>
                <code title={result.join_url}>{result.join_url}</code><CopyButton value={result.join_url} /></div>
            )}
          </div>
          {(result.warnings || []).map((w) => (
            <div className="app-notice" key={w}>
              <AlertTriangle />
              <span>{w}</span>
            </div>
          ))}
          <div className="app-modal-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={close}>{L("完成", "Done")}</button>
          </div>
        </>
      ) : (
        <>
          {busy ? (
            <div className="pub-progress" data-testid="opening">
              <div className="pub-progress-head">
                <span className="app-spinner" />
                <b>{choice === 2 ? L("正在用 Tailscale Funnel 开放…", "Opening with Tailscale Funnel…") : L("正在用 Cloudflare 开放…", "Opening with Cloudflare…")}</b>
                <span className="pub-progress-time">{L(`${waited} 秒`, `${waited}s`)}</span>
              </div>
              <div className="pub-bar" aria-hidden><i /></div>
              <ol className="pub-steps">
                {(choice === 2
                  ? [L("让 Tailscale 为这台电脑打开 Funnel", "Turn on Funnel for this computer"), L("第一次会申请 HTTPS 证书（可能要一两分钟）", "The first time, get an HTTPS certificate (a minute or two)"), L("拿到 xxx.ts.net 的公网地址", "Get the xxx.ts.net address")]
                  : [!cf ? L("下载 Cloudflare 的小工具 cloudflared（约 20 MB，只有第一次）", "Download Cloudflare's cloudflared (about 20 MB, only the first time)") : L("启动 cloudflared", "Start cloudflared"),
                    L("打通到 Cloudflare 的隧道", "Open the tunnel to Cloudflare"), L("拿到公网地址，并确认外面能连上", "Get the public address and check it is reachable")]
                ).map((t) => <li key={t}>{t}</li>)}
              </ol>
              <p className="app-small-note" data-testid="opening-wait">{choice === 2
                ? L("通常一两分钟。不想等可以「取消」，工作区照常在本机和局域网里能用。", "Usually a minute or two. Cancel if you would rather not wait: the workspace keeps working on this computer and its network.")
                : L("通常 10–60 秒。不想等可以「取消」，工作区照常在本机和局域网里能用。", "Usually 10–60 seconds. Cancel if you would rather not wait: the workspace keeps working on this computer and its network.")}</p>
            </div>
          ) : (
            <div className="pub-options" role="radiogroup" aria-label={L("怎么开放", "How")}>
              {TUNNELS.map((t, i) => {
                const off = disabled(i as TunnelChoice);
                const on = choice === i;
                return (
                  <label key={t.title} className={cn("app-choice pub-option", on && "is-on", off && "disabled")}>
                    <input type="radio" name="tunnel" className="app-sr-only" checked={on} disabled={off}
                      onChange={() => setChoice(i as TunnelChoice)} />
                    <span className="pub-option-text">
                      <span className="t">{t.title}<em className="pub-badge">{i === 0 ? L("推荐", "Recommended") : L("地址固定", "Fixed address")}</em></span>
                      <span className="d">{off || (i === 0 && !cf) ? t.missing : t.desc}</span>
                      {i === 1 && on && (
                        <input className="app-input mono pub-domain" value={hostname} onChange={(e) => setHostname(e.target.value)}
                          placeholder="hub.example.com" aria-label={L("域名", "Domain")} autoFocus />
                      )}
                    </span>
                    <span className="pub-check" aria-hidden>{on && <Check />}</span>
                  </label>
                );
              })}
            </div>
          )}
          {error && choice === 2 && <div className="app-small-note">{L("Tailscale 这一步没成功，原因和办法在下面（来自 Tailscale）。常见的是：这个 macOS 用户下没开 Tailscale App 或没登录、还没在 Tailscale 后台为这台设备打开 Funnel。", "Tailscale's step failed; why and what to do are below (from Tailscale). Usually: the Tailscale app is not open and signed in for this macOS user, or Funnel is not on for this device in the Tailscale admin console.")}</div>}
          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={busy ? cancel : close}>{L("取消", "Cancel")}</button>
            <button
              type="button"
              className="app-btn app-btn-primary"
              onClick={() => void submit()}
              disabled={busy || disabled(choice) || (choice === 1 && !hostname.trim())}
            >
              {busy ? <><span className="app-spinner" />{L("正在开放…", "Opening…")}</> : <><Cloud />{L("开放", "Open")}</>}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}

/** The hub's end-to-end encryption: shown to everyone, changed by its owner. */
function E2ERow({ hubId, ov, onChanged }: { hubId: string; ov?: HubOverview; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<boolean | null>(null);
  const [error, setError] = useState<unknown>(null);
  if (!ov?.ok || !ov.me?.member) return null;
  const on = !!ov.e2e;
  const owner = ov.me.member.role === "owner";
  const apply = async (next: boolean) => {
    setBusy(true);
    setError(null);
    try {
      await appApi.setHubSettings(hubId, { e2e: next });
      setConfirm(null);
      await onChanged();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <div className="app-kv">
        <span className="app-kv-label">{L("端到端加密", "End-to-end encryption")}</span>
        <span className="app-kv-value">
          <span className="app-status-text">{on ? L("已开启", "On") : L("未开启", "Off")}</span>
          {owner && (
            <button type="button" className="app-link-btn" disabled={busy} onClick={() => setConfirm(!on)}>
              {on ? L("关闭", "Turn off") : L("开启", "Turn on")}
            </button>
          )}
        </span>
      </div>
      <Modal open={confirm !== null} onOpenChange={(o) => !busy && !o && setConfirm(null)}
        title={confirm ? L("开启端到端加密", "Turn on end-to-end encryption") : L("关闭端到端加密", "Turn off end-to-end encryption")}>
        <p className="app-modal-note">
          {confirm
            ? L("开启后，新消息和附件只有收发双方、以及双方 agent 的主人能读；工作区、管理员和其他成员都读不到。适合几个人共用的工作区。还没升级的 agent 会继续收发明文，并标为「未加密」。开启前的消息不会被加密。",
              "New messages and attachments can then be read only by the two sides and the people who own them; the Hub, its admins and other members cannot. Meant for Hubs shared by several people. Nodes that are not upgraded yet keep sending plain messages, marked “Unencrypted”. Earlier messages stay as they are.")
            : L("关闭后，新消息以明文保存在工作区上；已经加密的消息仍然只有当事人能读。",
              "New messages are then stored in plain text on the Hub; messages already encrypted stay readable only by the people involved.")}
        </p>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setConfirm(null)} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void apply(!!confirm)} disabled={busy}>
            {busy ? L("处理中…", "Working…") : confirm ? L("开启", "Turn on") : L("关闭", "Turn off")}
          </button>
        </div>
      </Modal>
    </>
  );
}

/** The notch's island (Mac app): on unless turned off here. */
function IslandLine() {
  const [on, setOn] = useState(() => { try { return localStorage.getItem("myco-island") !== "off"; } catch { return true; } });
  const set = (v: boolean) => {
    setOn(v);
    try { localStorage.setItem("myco-island", v ? "on" : "off"); } catch { /* not kept */ }
  };
  return (
    <div data-setting="island" className="app-setting-line">
      <div className="app-setting-line-text">
        <b>{L("灵动岛", "Notch island")}</b>
        <span>{L("有成员在干活时，屏幕顶部的刘海会长出一小截：小蘑菇在走，旁边是几个在干活；指上去看是谁在做什么。不喜欢可以关掉。", "While members work, the notch at the top of the screen grows a little: a walking mushroom and how many are at work; point at it to see who is doing what. Turn it off if you'd rather not.")}</span>
      </div>
      <div className="seg" role="tablist" aria-label={L("灵动岛", "Notch island")}>
        <button type="button" role="tab" aria-selected={on} className={cn("seg-btn", on && "active")} onClick={() => set(true)}>{L("开", "On")}</button>
        <button type="button" role="tab" aria-selected={!on} className={cn("seg-btn", !on && "active")} onClick={() => set(false)}>{L("关", "Off")}</button>
      </div>
    </div>
  );
}

/** 过程只看进度: runs in chats show only what agents reported. */
function ReportsOnlyLine() {
  const only = useReportsOnly();
  const opts: { on: boolean; label: string }[] = [
    { on: false, label: L("全部", "Everything") },
    { on: true, label: L("只看进度", "Progress only") },
  ];
  return (
    <div data-setting="steps" className="app-setting-line">
      <div className="app-setting-line-text">
        <b>{L("对话里的过程", "Steps in chats")}</b>
        <span>{L("只看进度时，只显示 agent 自己报的进度和计划，不显示它的思考和跑的命令；在不在干活照样看得到。",
          "With progress only, chats show what agents report and their plans, not their thinking or commands; whether they are at work still shows.")}</span>
      </div>
      <div className="seg" role="tablist" aria-label={L("对话里的过程", "Steps in chats")}>
        {opts.map((t) => (
          <button key={String(t.on)} type="button" role="tab" aria-selected={only === t.on}
            className={cn("seg-btn", only === t.on && "active")} onClick={() => setReportsOnly(t.on)}>
            {t.label}
          </button>
        ))}
      </div>
    </div>
  );
}

const SERVICES: { key: TranslateConfig["provider"]; label: string; note: string }[] = [
  { key: "deepl", label: "DeepL", get note() { return L("在 DeepL 官网申请 API 密钥，免费版每月 50 万字（密钥以 :fx 结尾）。", "An API key from DeepL; the free plan has 500,000 characters a month (its key ends in :fx)."); } },
  { key: "google", get label() { return L("Google 翻译", "Google Translate"); }, get note() { return L("Google Cloud 的 Translation API 密钥，按字数收费。", "A Google Cloud Translation API key, paid by the character."); } },
  { key: "openai", get label() { return L("OpenAI 兼容", "OpenAI-compatible"); }, get note() { return L("OpenAI、DeepSeek、OpenRouter 等兼容 OpenAI 接口的服务：填地址、模型和密钥。", "OpenAI, DeepSeek, OpenRouter and others with an OpenAI-style API: an address, a model and a key."); } },
];

/** Translating what agents write: on or off, and with which service. Keys
 *  go to the system keychain through the console and never come back. */
function TranslateSection() {
  const autoOn = useAutoTranslate();
  const [cfg, setCfg] = useState<TranslateConfig | null>(null);
  const [provider, setProvider] = useState<TranslateConfig["provider"]>("deepl");
  const isReady = useTranslateReady();
  const [baseUrl, setBaseUrl] = useState("");
  const [model, setModel] = useState("");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [tried, setTried] = useState("");
  useEffect(() => {
    appApi.translateConfig().then((c) => {
      setCfg(c); setProvider(c.provider); setBaseUrl(c.base_url || ""); setModel(c.model || "");
    }, setError);
  }, []);
  const save = async (extra: { key?: string; clear_key?: boolean } = {}) => {
    setBusy(true); setError(null); setTried("");
    try {
      const c = await appApi.putTranslateConfig({ provider, base_url: provider === "openai" ? baseUrl.trim() : undefined, model: provider === "openai" ? model.trim() : undefined, ...extra });
      setCfg(c);
      setKey("");
      setTranslateReady(!!c.keys?.[c.provider]);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const tryIt = async () => {
    setBusy(true); setError(null); setTried("");
    try {
      const to = appTo();
      const r = await appApi.translate(to === "zh" ? "The tests pass and the release notes are ready." : "测试都过了，发布说明也写好了。", to);
      setTried(r.text);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const svc = SERVICES.find((x) => x.key === provider)!;
  const needsKey = true;
  const hasKey = !!cfg?.keys?.[provider];
  const dirty = !!cfg && (provider !== cfg.provider || (provider === "openai" && (baseUrl.trim() !== (cfg.base_url || "") || model.trim() !== (cfg.model || ""))));
  return (
    <>
      <section className="app-setting-card app-setting-lines">
        <div data-setting="translate" className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("自动翻译", "Translate automatically")}</b>
            <span>{isReady
              ? L("agent 的回复和进展跟 App 的语言不一样时，自动翻成 App 的语言（每条下面能看原文）。打开后，这些消息会发给下面选的翻译服务。每条消息时间旁的「翻译」随时可以手动翻。",
                "When what an agent says is in another language, show it in the app's language (each with its original a click away). With this on, those messages are sent to the service below. 「Translate」 by any message's time works either way.")
              : L("先在下面选一个翻译服务，填好密钥并保存；之后消息时间旁才有「翻译」，这里也才会生效。",
                "First choose a service below and save its key; then messages get 「Translate」 and this takes effect.")}</span>
          </div>
          <div className="seg" role="tablist" aria-label={L("自动翻译", "Translate automatically")}>
            {[false, true].map((on) => (
              <button key={String(on)} type="button" role="tab" aria-selected={autoOn === on} className={cn("seg-btn", autoOn === on && "active")} onClick={() => setAutoTranslate(on)}>
                {on ? L("开", "On") : L("关", "Off")}
              </button>
            ))}
          </div>
        </div>
      </section>
      <h4 className="app-setting-group">{L("翻译服务", "Service")}</h4>
      <section className="app-setting-card app-setting-lines">
        <div className="app-setting-line tr-services">
          <div className="seg" role="tablist" aria-label={L("翻译服务", "Service")}>
            {SERVICES.map((x) => (
              <button key={x.key} type="button" role="tab" aria-selected={provider === x.key} className={cn("seg-btn", provider === x.key && "active")} onClick={() => { setProvider(x.key); setTried(""); setError(null); }}>
                {x.label}
              </button>
            ))}
          </div>
          <p className="tr-note">{svc.note}</p>
        </div>
        {provider === "openai" && (
          <div className="app-setting-line tr-fields">
            <label>{L("地址", "Address")}<input className="app-input mono" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.openai.com/v1" spellCheck={false} /></label>
            <label>{L("模型", "Model")}<input className="app-input mono" value={model} onChange={(e) => setModel(e.target.value)} placeholder="gpt-4o-mini" spellCheck={false} /></label>
          </div>
        )}
        {needsKey && (
          <div className="app-setting-line tr-fields">
            <label>{L("密钥", "Key")}
              <input className="app-input mono" type="password" value={key} onChange={(e) => setKey(e.target.value)} autoComplete="off" spellCheck={false}
                placeholder={hasKey ? L("已保存在系统钥匙串里（不显示）", "Kept in the system keychain (not shown)") : L("粘贴 API 密钥", "Paste the API key")} />
            </label>
            {hasKey && <button type="button" className="app-btn app-btn-quiet" disabled={busy} onClick={() => void save({ clear_key: true })}>{L("删掉密钥", "Remove the key")}</button>}
          </div>
        )}
        <div className="app-setting-line tr-actions">
          <button type="button" className="app-btn app-btn-primary" disabled={busy || (!dirty && !key.trim())} onClick={() => void save(key.trim() ? { key: key.trim() } : {})}>
            {L("保存", "Save")}
          </button>
          <button type="button" className="app-btn" disabled={busy || dirty || (needsKey && !hasKey)} onClick={() => void tryIt()}>{L("试一下", "Try it")}</button>
          {tried && <span className="tr-tried"><Check />{tried}</span>}
        </div>
        <ErrorText error={error} />
      </section>
    </>
  );
}

/** How the App reaches its workspaces, update checks, translation and
 *  voice services: the proxy in the environment (as before), none, or one
 *  set here (HTTP or SOCKS5; a password goes to the keychain). Addresses on
 *  this computer and the local network go direct. 「试一下」 tries the
 *  setting before it is saved, against a workspace's address. */
function ProxySection({ hubsOv }: { hubsOv?: HubOverview[] }) {
  const [cfg, setCfg] = useState<ProxySetting | null>(null);
  const [mode, setMode] = useState<ProxySetting["mode"]>("system");
  const [type, setType] = useState<NonNullable<ProxySetting["type"]>>("socks5");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [user, setUser] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [tried, setTried] = useState<{ ok: boolean; text: string } | null>(null);
  const [saved, setSaved] = useState(false);
  const fill = (c: ProxySetting) => {
    setCfg(c); setMode(c.mode); setType(c.type || "socks5"); setHost(c.host || ""); setPort(c.port ? String(c.port) : ""); setUser(c.user || "");
  };
  useEffect(() => { appApi.proxy().then(fill, setError); }, []);
  const input = (): ProxyInput => mode === "manual"
    ? { mode, type, host: host.trim(), port: Number(port) || 0, user: user.trim(), ...(password ? { password } : {}) }
    : { mode };
  const dirty = !!cfg && (mode !== cfg.mode || (mode === "manual" && (type !== (cfg.type || "socks5") || host.trim() !== (cfg.host || "") || (Number(port) || 0) !== (cfg.port || 0) || user.trim() !== (cfg.user || "") || !!password)));
  const save = async (extra: { clear_password?: boolean } = {}) => {
    setBusy(true); setError(null); setTried(null); setSaved(false);
    try {
      fill(await appApi.putProxy({ ...input(), ...extra }));
      setPassword("");
      setSaved(true);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const reasons: Record<string, string> = {
    timeout: L("12 秒内没有回应", "No answer within 12 seconds"),
    refused: L("代理拒绝了连接：它在运行吗，端口对吗？", "The proxy refused the connection: is it running, on that port?"),
    not_found: L("找不到这个地址", "That address was not found"),
    auth: L("代理要用户名和密码，或者没接受填的这组", "The proxy wants a user name and password, or did not accept these"),
  };
  // (against the workspace you are on, else GitHub)
  const target = hubsOv?.find((h) => h.ok)?.id;
  const tryIt = async () => {
    setBusy(true); setError(null); setTried(null); setSaved(false);
    try {
      const hubUrl = target ? (await appApi.hubs().catch(() => [])).find((h) => h.id === target)?.url : undefined;
      const r = await appApi.testProxy({ ...input(), url: hubUrl });
      setTried(r.ok
        ? { ok: true, text: L(`连通了，用时 ${r.ms} 毫秒`, `Connected in ${r.ms} ms`) }
        : { ok: false, text: (r.reason && reasons[r.reason]) || r.error || L("连不上", "Could not connect") });
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const modes: { key: ProxySetting["mode"]; label: string }[] = [
    { key: "system", label: L("跟系统环境", "From the environment") },
    { key: "none", label: L("不用代理", "No proxy") },
    { key: "manual", label: L("手动设置", "Set here") },
  ];
  return (
    <section className="app-setting-card app-setting-lines">
      <div data-setting="proxy" className="app-setting-line">
        <div className="app-setting-line-text">
          <b>{L("代理", "Proxy")}</b>
          <span>{L("App 连工作区、检查更新、翻译和语音服务时走的代理。这台电脑和局域网里的地址（比如本机的工作区）总是直连。",
            "The proxy the App uses to reach workspaces, update checks, translation and voice services. Addresses on this computer and the local network (a workspace here, say) are always reached directly.")}</span>
        </div>
      </div>
      <div className="app-setting-line tr-services">
        <div className="seg" role="tablist" aria-label={L("代理", "Proxy")}>
          {modes.map((x) => (
            <button key={x.key} type="button" role="tab" aria-selected={mode === x.key} className={cn("seg-btn", mode === x.key && "active")}
              onClick={() => { setMode(x.key); setTried(null); setSaved(false); setError(null); }}>{x.label}</button>
          ))}
        </div>
        <p className="tr-note">{mode === "system"
          ? L("用环境变量里的代理（HTTPS_PROXY、HTTP_PROXY、ALL_PROXY），没有就直连。", "Uses the proxy in the environment (HTTPS_PROXY, HTTP_PROXY, ALL_PROXY), else connects directly.")
          : mode === "none" ? L("所有连接都直连，不走任何代理。", "Every connection goes directly, through no proxy.")
          : L("比如 Clash、V2Ray 一类本地代理：SOCKS5 常见端口 1080 / 7891，HTTP 常见 7890 / 8080。", "Such as a local Clash or V2Ray: SOCKS5 is often on 1080 / 7891, HTTP on 7890 / 8080.")}</p>
      </div>
      {mode === "manual" && (
        <>
          <div className="app-setting-line tr-fields">
            <div className="tr-field">{L("类型", "Type")}
              <div className="seg seg-sm" role="tablist" aria-label={L("代理类型", "Proxy type")}>
                {(["socks5", "http", "https"] as const).map((t) => (
                  <button key={t} type="button" role="tab" aria-selected={type === t} className={cn("seg-btn", type === t && "active")} onClick={() => setType(t)}>
                    {t === "socks5" ? "SOCKS5" : t.toUpperCase()}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="app-setting-line tr-fields">
            <label>{L("地址", "Host")}<input className="app-input mono" value={host} onChange={(e) => setHost(e.target.value)} placeholder="127.0.0.1" spellCheck={false} autoComplete="off" /></label>
            <label>{L("端口", "Port")}<input className="app-input mono" value={port} onChange={(e) => setPort(e.target.value.replace(/[^0-9]/g, "").slice(0, 5))} placeholder={type === "socks5" ? "1080" : "7890"} inputMode="numeric" spellCheck={false} /></label>
          </div>
          <div className="app-setting-line tr-fields">
            <label>{L("用户名（可不填）", "User (optional)")}<input className="app-input mono" value={user} onChange={(e) => setUser(e.target.value)} spellCheck={false} autoComplete="off" /></label>
            <label>{L("密码（可不填）", "Password (optional)")}
              <input className="app-input mono" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="off" spellCheck={false}
                placeholder={cfg?.has_password ? L("已保存在系统钥匙串里（不显示）", "Kept in the system keychain (not shown)") : ""} />
            </label>
            {cfg?.has_password && <button type="button" className="app-btn app-btn-quiet" disabled={busy} onClick={() => void save({ clear_password: true })}>{L("删掉密码", "Remove the password")}</button>}
          </div>
        </>
      )}
      <div className="app-setting-line tr-actions">
        <button type="button" className="app-btn app-btn-primary" disabled={busy || !dirty} onClick={() => void save()}>{L("保存", "Save")}</button>
        {mode === "manual" && <button type="button" className="app-btn" disabled={busy || !host.trim() || !port} onClick={() => void tryIt()}>{L("试一下", "Try it")}</button>}
        {saved && !dirty && <span className="tr-tried"><Check />{L("已保存，马上生效", "Saved; in effect now")}</span>}
        {tried && <span className={cn("tr-tried", !tried.ok && "is-error")}>{tried.ok && <Check />}{tried.text}</span>}
      </div>
      <ErrorText error={error} />
    </section>
  );
}

/** Voice calls: the model the voice assistant talks with (by default the
 *  translation's OpenAI-compatible one) and the speech service it speaks
 *  with, one of those the backend lists (voice_sync.go). Keys go to the
 *  system keychain and are never shown. The settings are the person's: kept
 *  on the workspaces' hubs too, so their other devices have them. */
const OTHER = "\u0000other";

/** Voices by language: Chinese first, then English, then the rest in the list's order. */
function voiceGroups(voices: TTSVoice[]): [string, TTSVoice[]][] {
  const by = new Map<string, TTSVoice[]>();
  for (const v of voices) { const g = by.get(v.lang); if (g) g.push(v); else by.set(v.lang, [v]); }
  const first = ["zh", "en"];
  return [...by.entries()].sort((a, b) => {
    const i = first.indexOf(a[0]), j = first.indexOf(b[0]);
    return (i < 0 ? 9 : i) - (j < 0 ? 9 : j);
  });
}

const VOICE_LANGS: Record<string, [string, string]> = {
  zh: ["中文", "Chinese"], en: ["英语", "English"], ja: ["日语", "Japanese"], ko: ["韩语", "Korean"],
  es: ["西班牙语", "Spanish"], mx: ["西班牙语（墨西哥）", "Spanish (Mexico)"], pt: ["葡萄牙语（巴西）", "Portuguese (Brazil)"],
  fr: ["法语", "French"], de: ["德语", "German"], it: ["意大利语", "Italian"], ru: ["俄语", "Russian"],
  id: ["印尼语", "Indonesian"], ms: ["马来语", "Malay"], th: ["泰语", "Thai"], vi: ["越南语", "Vietnamese"],
  ar: ["阿拉伯语", "Arabic"], tl: ["菲律宾语", "Filipino"],
};
function voiceLangName(code: string): string {
  const n = VOICE_LANGS[code];
  return n ? L(n[0], n[1]) : code;
}

const DOUBAO: VoiceTTSProvider = { id: "doubao", name: "豆包语音", name_en: "Doubao Speech", default_speaker: "zh_male_wennuanahu_uranus_bigtts", has_key: false };

function VoiceSection() {
  const [info, setInfo] = useState<VoiceInfo | null>(null);
  const [baseUrl, setBaseUrl] = useState("");
  const [model, setModel] = useState("");
  const [llmKey, setLlmKey] = useState("");
  const [provider, setProvider] = useState("doubao");
  const [speaker, setSpeaker] = useState("");
  const [rate, setRate] = useState(0);
  const [ttsKey, setTtsKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);
  // Typing a voice ID (one not on the list, like a cloned voice).
  const [typing, setTyping] = useState(false);
  const [trying, setTrying] = useState(false);
  const audio = useRef<HTMLAudioElement | null>(null);
  const load = (v: VoiceInfo) => {
    setInfo(v);
    setBaseUrl(v.config.llm_base_url || ""); setModel(v.config.llm_model || "");
    const pid = v.config.tts_provider || "doubao";
    setProvider(pid);
    const sp = v.config.tts_speaker || "";
    setSpeaker(sp); setRate(v.config.tts_rate || 0);
    const list = v.tts_providers?.find((p) => p.id === pid)?.voices || [];
    setTyping(!!sp && list.length > 0 && !list.some((x) => x.id === sp));
  };
  useEffect(() => () => { audio.current?.pause(); }, []);
  const tryVoice = async () => {
    setTrying(true); setError(null);
    try {
      const wav = await appApi.previewVoice({ tts_provider: provider, tts_speaker: speaker.trim(), tts_rate: rate, lang: lang(), ...(ttsKey.trim() ? { tts_key: ttsKey.trim() } : {}) });
      audio.current?.pause();
      const url = URL.createObjectURL(wav);
      const a = new Audio(url);
      a.onended = () => URL.revokeObjectURL(url);
      audio.current = a;
      await a.play();
    } catch (e) { setError(e); } finally { setTrying(false); }
  };
  useEffect(() => {
    let off = false;
    // What this computer has, at once; then what another device may have
    // changed since (the hubs keep the settings).
    appApi.voice().then((v) => {
      if (off) return;
      load(v);
      appApi.voiceSync().then((n) => { if (!off) { load(n); window.dispatchEvent(new Event("myco-voice-changed")); } }, () => undefined);
    }, setError);
    return () => { off = true; };
  }, []);
  const save = async (extra: { clear_llm_key?: boolean; clear_tts_key?: boolean; share?: boolean } = {}) => {
    setBusy(true); setError(null); setSaved(false);
    try {
      const v = await appApi.putVoiceConfig({
        llm_base_url: baseUrl.trim(), llm_model: model.trim(), tts_provider: provider, tts_speaker: speaker.trim(), tts_rate: rate,
        ...(llmKey.trim() ? { llm_key: llmKey.trim() } : {}), ...(ttsKey.trim() ? { tts_key: ttsKey.trim() } : {}), ...extra,
      });
      load(v); setLlmKey(""); setTtsKey(""); setSaved(true);
      window.dispatchEvent(new Event("myco-voice-changed")); // the chats' round buttons
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  if (!info) return <ErrorText error={error} />;
  const providers = info.tts_providers?.length ? info.tts_providers : [DOUBAO];
  const svc = providers.find((p) => p.id === provider) || providers[0];
  const voices = svc.voices || [];
  // The voice on the list, or the default one when none is set.
  const chosen = speaker.trim() || svc.default_speaker || "";
  const svcName = lang() === "en" ? svc.name_en : svc.name;
  const svcHelp = lang() === "en" ? svc.help_en : svc.help;
  // The key shown as kept is the chosen service's.
  const hasTtsKey = info.tts_providers ? svc.has_key : info.has_tts_key;
  const share = info.share !== false;
  const sync = info.sync || [];
  const behind = sync.filter((h) => h.state === "unsupported");
  const away = sync.filter((h) => h.state === "unreachable");
  const kept = sync.filter((h) => h.state === "synced");
  return (
    <>
      <section className="app-setting-card app-setting-lines">
        <div data-setting="voice-chat" className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("和 agent 语音聊天", "Voice chat with an agent")}</b>
            <span>{info.available
              ? L("配好下面的模型和语音合成后，对话里输入框空着时，右边的圆按钮就是语音聊天。你说的话转给这个 agent（有活就补充，空着就是新任务），它的回复和提问挑空档说给你听；你一开口它就停下来听。录音不保存。",
                "Once the model and the speech service below are set up, the round button beside an empty box starts a voice chat with that agent. What you ask goes to it (added to its task at work, or a new task); its answers and questions are told in a pause; it stops as soon as you speak. No recordings are kept.")
              : L("语音聊天目前只在 Mac 版 App 和手机 App 里有。这里的设置会同步过去。", "Voice chat is in the Mac app and the phone app for now. What is set here is shared with them.")}</span>
          </div>
        </div>
      </section>
      <h4 className="app-setting-group">{L("语音助手用的模型", "The assistant's model")}</h4>
      <section className="app-setting-card app-setting-lines">
        <div className="app-setting-line">
          <p className="tr-note">{L("任意 OpenAI 兼容接口（OpenAI、DeepSeek、豆包、OpenRouter…）。选反应快的模型，它不用很聪明，难的事都交给 agent。不填就用「翻译」里配好的那个。",
            "Any OpenAI-compatible API (OpenAI, DeepSeek, Doubao, OpenRouter…). Pick a fast model: the hard work goes to the agents. Left empty, the one set up under Translation is used.")}</p>
        </div>
        <div className="app-setting-line tr-fields">
          <label>{L("地址", "Address")}<input className="app-input mono" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.deepseek.com/v1" spellCheck={false} /></label>
          <label>{L("模型", "Model")}<input className="app-input mono" value={model} onChange={(e) => setModel(e.target.value)} placeholder="deepseek-chat" spellCheck={false} /></label>
        </div>
        <div className="app-setting-line tr-fields">
          <label>{L("密钥", "Key")}
            <input className="app-input mono" type="password" value={llmKey} onChange={(e) => setLlmKey(e.target.value)} autoComplete="off" spellCheck={false}
              placeholder={info.has_llm_key ? L("已保存在系统钥匙串里（不显示）", "Kept in the system keychain (not shown)") : L("粘贴 API 密钥", "Paste the API key")} />
          </label>
          {info.has_llm_key && <button type="button" className="app-btn app-btn-quiet" disabled={busy} onClick={() => void save({ clear_llm_key: true })}>{L("删掉密钥", "Remove the key")}</button>}
        </div>
      </section>
      <h4 className="app-setting-group">{L("语音合成", "Speech")}</h4>
      <section className="app-setting-card app-setting-lines">
        <div className="app-setting-line tr-services">
          <div className="seg vs-services" role="tablist" aria-label={L("语音合成服务", "Speech service")}>
            {providers.map((p) => (
              <button key={p.id} type="button" role="tab" aria-selected={svc.id === p.id} className={cn("seg-btn", svc.id === p.id && "active")}
                onClick={() => { setProvider(p.id); setTtsKey(""); setSpeaker(""); setTyping(false); setSaved(false); setError(null); }}>
                {lang() === "en" ? p.name_en : p.name}
              </button>
            ))}
          </div>
          <p className="tr-note">{svcHelp || L("它把语音助手的话说出来。", "It says what the assistant answers out loud.")}</p>
        </div>
        <div className="app-setting-line tr-fields">
          {voices.length > 0 && (
            <label className="vs-voice">{L("音色", "Voice")}
              <select className="app-select-field" data-testid="voice-speaker" value={typing ? OTHER : chosen}
                onChange={(e) => { const other = e.target.value === OTHER; setTyping(other); setSpeaker(other ? "" : e.target.value); setSaved(false); }}>
                {voiceGroups(voices).map(([lg, vs]) => (
                  <optgroup key={lg} label={voiceLangName(lg)}>
                    {vs.map((v) => <option key={v.id} value={v.id}>{v.name} · {v.gender === "female" ? L("女", "female") : L("男", "male")}</option>)}
                  </optgroup>
                ))}
                <option value={OTHER}>{L("其他（填音色 ID）", "Other (type a voice ID)")}</option>
              </select>
            </label>
          )}
          {(typing || voices.length === 0) && (
            <label>{voices.length ? L("音色 ID", "Voice ID") : L("音色", "Voice")}
              <input className="app-input mono" value={speaker} onChange={(e) => setSpeaker(e.target.value)} placeholder={voices.length ? "S_…" : svc.default_speaker || ""} spellCheck={false} />
            </label>
          )}
          <label className="vs-rate">{L("语速", "Speed")}
            <input className="app-input" type="number" min={-50} max={100} step={5} value={rate} onChange={(e) => setRate(Number(e.target.value) || 0)} />
          </label>
          <button type="button" className="app-btn" data-testid="voice-try" disabled={trying || (!hasTtsKey && !ttsKey.trim()) || (typing && !speaker.trim())}
            title={!hasTtsKey && !ttsKey.trim() ? L("先填 API Key", "Type the API key first") : undefined} onClick={() => void tryVoice()}>
            {trying ? L("合成中…", "Speaking…") : L("试听", "Try it")}
          </button>
        </div>
        <div className="app-setting-line tr-fields">
          <label>{L("API Key", "API key")}
            <input className="app-input mono" type="password" value={ttsKey} onChange={(e) => setTtsKey(e.target.value)} autoComplete="off" spellCheck={false}
              placeholder={hasTtsKey ? L("已保存在系统钥匙串里（不显示）", "Kept in the system keychain (not shown)") : L(`粘贴${svcName}的 API Key`, `Paste the ${svcName} API key`)} />
          </label>
          {hasTtsKey && <button type="button" className="app-btn app-btn-quiet" disabled={busy} onClick={() => void save({ clear_tts_key: true })}>{L("删掉密钥", "Remove the key")}</button>}
        </div>
        <div className="app-setting-line tr-actions">
          <button type="button" className="app-btn app-btn-primary" disabled={busy} onClick={() => void save()}>{L("保存", "Save")}</button>
          {saved && <span className="tr-tried"><Check />{L("已保存", "Saved")}</span>}
        </div>
        <ErrorText error={error} />
      </section>
      <h4 className="app-setting-group">{L("多设备同步", "On your other devices")}</h4>
      <section className="app-setting-card app-setting-lines">
        <div data-setting="voice-keep" className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("把这些设置存到工作区", "Keep these settings on the workspaces")}</b>
            <span>{L("打开后，上面的设置和密钥存在你连着的工作区（Hub）里，你的手机和别的电脑不用再填一遍；哪台设备后改的算数。密钥只给你自己的设备，但 Hub 所在机器的管理员读得到它的数据库：不是你自己的 Hub，介意就关掉。",
              "With this on, the settings above and their keys are kept on the workspaces (hubs) you are signed in to, so your phone and other computers need not be set up again; the device that changed them last wins. The keys are handed only to your own devices, but whoever administers a hub's machine can read its database: turn this off for a hub that is not yours, if that matters.")}</span>
            {share && sync.length > 0 && (
              <span className="vs-sync" data-testid="voice-sync">
                {kept.length > 0 && L(`已存到：${kept.map((h) => h.name).join("、")}。`, `Kept on: ${kept.map((h) => h.name).join(", ")}. `)}
                {behind.length > 0 && L(`${behind.map((h) => h.name).join("、")} 的 Hub 版本太旧，还不能存，升级它的 myco 就可以。`, `${behind.map((h) => h.name).join(", ")}: the hub is too old to keep them; update its myco. `)}
                {away.length > 0 && L(`${away.map((h) => h.name).join("、")} 现在连不上，下次再试。`, `${away.map((h) => h.name).join(", ")}: not reachable now; tried again next time.`)}
              </span>
            )}
          </div>
          <div className="seg" role="tablist" aria-label={L("把这些设置存到工作区", "Keep these settings on the workspaces")}>
            {[false, true].map((on) => (
              <button key={String(on)} type="button" role="tab" aria-selected={share === on} disabled={busy} className={cn("seg-btn", share === on && "active")} onClick={() => { if (share !== on) void save({ share: on }); }}>
                {on ? L("开", "On") : L("关", "Off")}
              </button>
            ))}
          </div>
        </div>
      </section>
    </>
  );
}

/** 聊天背景: the default for every conversation (each can have its own). */
function ChatBgLine() {
  const [open, setOpen] = useState(false);
  return (
    <div className="app-setting-line" data-setting="chat-bg">
      <div className="app-setting-line-text">
        <b>{L("聊天背景", "Chat background")}</b>
        <span>{L("所有对话默认用的背景，只在这台电脑上。单个对话在它顶栏的「⋯」里换。", "The background every conversation uses, on this computer only. Change one conversation's in its header's ⋯.")}</span>
      </div>
      <button type="button" className="app-btn" onClick={() => setOpen(true)}>{L("选择…", "Choose…")}</button>
      <ChatBgDialog open={open} onClose={() => setOpen(false)} />
    </div>
  );
}

