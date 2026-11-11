import { clearAll, forgetHub, recall, remember } from "@/lib/memo";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle, Check, Cloud, Globe, Info, Monitor, Palette, Pencil, Plus, ScrollText, Server, Terminal, Users,
} from "lucide-react";
import {
  appApi, type AppInfo, type CacheStats, type AppUpdate, type Hub, type HubOverview, type LocalState,
  type LocalTunnel, type StartLocalHubResult,
} from "@/appapi";
import { ver } from "@/app/nodes";
import { renameHub } from "@/app/hubNames";
import { ErrorText, Modal, ConfirmDialog } from "@/app/dialogs";
import { AuditSection, MembersSection } from "@/app/MembersSection";
import type { TextSize, ThemeMode } from "@/lib/theme";
import { L, lang, type LangChoice } from "@/lib/i18n";
import { bytes, cn } from "@/lib/utils";

type Section = "hubs" | "members" | "audit" | "appearance" | "about";

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
      <p className="app-summary">{L("myco App 把多个 Hub 上的节点放进一个窗口。", "The myco app brings the nodes of several Hubs into one window.")}</p>
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
      <p className="app-about-note">{L("节点、任务和对话都由 Hub 提供，App 只负责显示和转发。", "Nodes, tasks and conversations come from the Hub; the app only displays and relays them.")}</p>
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
  { key: "hubs", label: "Hub", icon: Server },
  { key: "members", get label() { return L("成员", "People"); }, icon: Users },
  { key: "audit", get label() { return L("审计", "Audit"); }, icon: ScrollText },
  { key: "appearance", get label() { return L("外观", "Appearance"); }, icon: Palette },
  { key: "about", get label() { return L("关于", "About"); }, icon: Info },
];

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
    get title() { return L("Cloudflare 临时地址", "Cloudflare temporary address"); },
    get desc() { return L("不用账号，地址每次重启都会变，适合先试试。", "No account needed. The address changes on every restart, so it’s good for trying things out."); },
    get missing() { return L("需要先安装 cloudflared。", "Install cloudflared first."); },
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
export function SettingsPage({ hubs, hubsOv, info, theme, onTheme, textSize, onTextSize, langChoice, onLangChoice, refreshKey, sectionRequest, onRefresh, onAddHub }: {
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
}) {
  const [section, setSection] = useState<Section>(() => langSwitchSection ?? "hubs");
  // After a language remount, the request that was already handled must not
  // pull the page back to the Hub section.
  const handledRequest = useRef(langSwitchSection !== null ? sectionRequest : undefined);
  useEffect(() => { langSwitchSection = null; }, []);
  const [local, setLocalState] = useState<LocalState | null>(() => recall<LocalState | null>("local") ?? null);
  const setLocal = (d: LocalState | null) => { if (d) remember("local", d); setLocalState(d); };
  const [localError, setLocalError] = useState<unknown>(null);
  const [busy, setBusy] = useState<null | "start" | "stop">(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [renameTarget, setRenameTarget] = useState<Hub | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [stopOpen, setStopOpen] = useState(false);
  const [publicOpen, setPublicOpen] = useState(false);

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

  const isLocalHub = (h: Hub) => {
    if (!local?.hub) return false;
    const urls = [local.hub.local_url, local.hub.public_url].filter(Boolean).map((u) => stripSlash(u!));
    return urls.includes(stripSlash(h.url));
  };

  const saveRename = () => {
    if (!renameTarget) return;
    renameHub(renameTarget.id, renameValue);
    setRenameTarget(null);
  };

  const startLocal = async () => {
    setBusy("start");
    setActionError(null);
    try {
      await appApi.startLocalHub({ tunnel: "none" });
      await reloadLocal();
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(null);
    }
  };

  const stopLocal = async () => {
    setBusy("stop");
    setActionError(null);
    try {
      await appApi.stopLocalHub();
      setStopOpen(false);
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

  const localRunning = !!local?.hub?.running;

  return (
    <div className="app-page app-settings">
      <div className="app-settings-nav" role="tablist" aria-label={L("设置", "Settings")}>
        {SECTIONS.map((s) => (
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

      <div className="app-settings-body">
        {section === "hubs" && (
          <>
            <p className="app-summary">{L("Hub 是节点所在的服务器。连接后，所有页面只显示选中的 Hub。", "A Hub is the server your nodes live on. Once connected, every page shows only the selected Hub.")}</p>
            <ErrorText error={localError} />
            <ErrorText error={actionError} />
            {hubs.map((h) => {
              const localHub = isLocalHub(h);
              const hubLocal = localHub ? local?.hub : null;
              return (
                <section className="app-setting-card" key={h.id}>
                  <div className="app-setting-head">
                    <span className={cn("dot", h.ok ? "working" : "offline")} style={h.ok ? undefined : { background: "var(--red)" }} />
                    <div className="titles">
                      <h3>{h.name}</h3>
                      <p className="mono">{host(h.url)}</p>
                    </div>
                    <button
                      type="button"
                      className="app-btn app-btn-plain"
                      onClick={() => { setRenameTarget(h); setRenameValue(h.name); }}
                      aria-label={L("重命名", "Rename")}
                      title={L("重命名", "Rename")}
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
                        <span className={cn("app-pill", h.ok ? "green" : "red")}>
                          <span className={cn("dot", h.ok ? "working" : "offline")} />
                          {h.ok ? L("已连接", "Connected") : L("连不上", "Unreachable")}
                        </span>
                      </span>
                    </div>
                    <E2ERow hubId={h.id} ov={hubsOv?.find((o) => o.id === h.id)} onChanged={onRefresh} />
                    {localHub && (
                      <div className="app-kv">
                        <span className="app-kv-label">{L("访问", "Access")}</span>
                        <span className="app-kv-value">
                          {hubLocal?.running && hubLocal.public_url
                            ? <span className="mono clip" title={hubLocal.public_url}>{hubLocal.public_url}</span>
                            : L("只在本机/局域网", "This computer and local network only")}
                        </span>
                      </div>
                    )}
                  </div>
                  {localHub && !h.ok && !localRunning && (
                    <div className="app-small-note" style={{ marginTop: 8 }}>{L("本机 Hub 已停止，点「启动本机 Hub」重新启动。", "The local Hub has stopped. Start it again with “Start local Hub”.")}</div>
                  )}

                  <div className="app-setting-actions">
                    {localHub ? (
                      <>
                        <button type="button" className="app-btn" onClick={() => setPublicOpen(true)} disabled={busy !== null}>
                          <Globe />{L("开放到公网", "Open to the internet")}
                        </button>
                        {localRunning ? (
                          <button type="button" className="app-btn app-btn-danger" onClick={() => setStopOpen(true)} disabled={busy !== null}>
                            {L("停止本机 Hub", "Stop local Hub")}
                          </button>
                        ) : (
                          <button type="button" className="app-btn app-btn-primary" onClick={() => void startLocal()} disabled={busy !== null}>
                            {busy === "start" ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : L("启动本机 Hub", "Start local Hub")}
                          </button>
                        )}
                      </>
                    ) : (
                      <button type="button" className="app-btn app-btn-danger" onClick={() => { setActionError(null); setRemoveTarget(h); }}>
                        {L("移除", "Remove")}
                      </button>
                    )}
                  </div>
                </section>
              );
            })}

            {hubs.length === 0 && (
              <div className="app-empty-note" style={{ marginTop: 8 }}>{L("还没有 Hub。先连接一个，或者在这台电脑上启动一个。", "No Hubs yet. Connect one, or start one on this computer.")}</div>
            )}

            <div className="app-setting-add">
              <button type="button" className="app-setting-add-row" onClick={onAddHub}>
                <Plus />{L("添加 Hub", "Add Hub")}
              </button>
              {!localRunning && (
                <button
                  type="button"
                  className="app-setting-add-row"
                  onClick={() => void startLocal()}
                  disabled={busy !== null}
                >
                  {busy === "start" ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : <><Terminal />{L("在这台电脑上启动 Hub", "Start a Hub on this computer")}</>}
                </button>
              )}
            </div>
            <div className="app-hint">
              {L("名称只在这个 App 里显示，不会改到 Hub 上。地址和令牌保存在这台电脑上。", "Names show only in this app and don’t change the Hub. Addresses and tokens are saved on this computer.")}
            </div>
          </>
        )}

        {section === "appearance" && (
          <>
            <section className="app-setting-card app-setting-lines">
              <div className="app-setting-line">
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
              <div className="app-setting-line">
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
              <div className="app-setting-line">
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

        {section === "about" && <AboutSection info={info} local={local} />}
        {section === "members" && <MembersSection hubsOv={hubsOv} onRefresh={onRefresh} />}
        {section === "audit" && <AuditSection hubsOv={hubsOv} />}
      </div>

      <Modal
        open={!!renameTarget}
        onOpenChange={(o) => { if (!o) setRenameTarget(null); }}
        title={L("重命名 Hub", "Rename Hub")}
        description={L("只在这个 App 里显示，不会改到 Hub 上。", "Shows only in this app and doesn’t change the Hub.")}
      >
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
          <button type="button" className="app-btn app-btn-primary" onClick={saveRename} disabled={!renameValue.trim()}>{L("保存", "Save")}</button>
        </div>
      </Modal>

      <PublicAccessDialog
        open={publicOpen}
        onOpenChange={setPublicOpen}
        local={local}
        onDone={() => { void reloadLocal(); void onRefresh(); }}
      />

      <ConfirmDialog
        open={stopOpen}
        onOpenChange={(o) => !o && setStopOpen(false)}
        title={L("停止本机 Hub？", "Stop the local Hub?")}
        description={L("Hub 会在这台电脑上停止运行。", "The Hub will stop running on this computer.")}
        consequences={<p>{L("其他机器会连不上它。数据和令牌都保留，重新开启即可恢复。", "Other machines won’t be able to reach it. Data and tokens are kept; start it again to bring it back.")}</p>}
        confirmLabel={L("停止", "Stop")}
        busy={busy === "stop"}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void stopLocal()}
      />

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => !o && setRemoveTarget(null)}
        title={removeTarget ? L(`移除 ${removeTarget.name}？`, `Remove ${removeTarget.name}?`) : L("移除 Hub", "Remove Hub")}
        description={L("只会从这个 App 里移除。Hub 本身和它上面的 agent 都不受影响，仍在照常运行。", "Removes it from this app only. The Hub and its agents aren’t affected and keep running.")}
        consequences={<p>{L("你将看不到这个 Hub 的节点，之后只能凭 App 连接链接重新添加。", "You won’t see this Hub’s nodes. To add it back later, you’ll need its app link.")}</p>}
        confirmLabel={L("移除 Hub", "Remove Hub")}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void removeHub()}
      />
    </div>
  );
}

/** Reconfigure the local hub behind a public tunnel. */
function PublicAccessDialog({ open, onOpenChange, local, onDone }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  local: LocalState | null;
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
    setChoice(cf ? 0 : ts ? 2 : 0);
    setHostname("");
    setBusy(false);
    setError(null);
    setResult(null);
  }, [open, cf, ts]);

  const disabled = (i: TunnelChoice) =>
    i === 0 || i === 1 ? !cf : !ts;

  const submit = async () => {
    setBusy(true);
    setError(null);
    const input = choice === 1
      ? { tunnel: "cloudflare" as const, hostname: hostname.trim() }
      : choice === 2
        ? { tunnel: "tailscale" as const }
        : { tunnel: "cloudflare" as const };
    try {
      setResult(await appApi.startLocalHub(input));
      onDone();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const close = () => { if (!busy) onOpenChange(false); };

  return (
    <Modal
      open={open}
      onOpenChange={(o) => { if (!o) close(); }}
      title={L("开放到公网", "Open to the internet")}
      description={L("让不在同一局域网的机器也能连上这台电脑上的 Hub。", "Let machines outside your local network reach the Hub on this computer.")}
    >
      {result ? (
        <>
          <div className="app-success">
            <Check />
            <span>{L("已开放", "Now public")}</span>
          </div>
          <div className="app-kv"><span className="app-kv-label">{L("公网地址", "Public address")}</span><span className="app-kv-value mono clip" title={result.public_url}>{result.public_url}</span></div>
          {result.join_url && (
            <div className="app-kv"><span className="app-kv-label">{L("入网链接", "Join link")}</span><span className="app-kv-value mono clip" title={result.join_url}>{result.join_url}</span></div>
          )}
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
          <div className="app-list">
            {TUNNELS.map((t, i) => {
              const off = disabled(i as TunnelChoice);
              return (
                <label key={t.title} className={cn("app-choice", off && "disabled")}>
                  <input
                    type="radio"
                    name="tunnel"
                    className="app-sr-only"
                    checked={choice === i}
                    disabled={off || busy}
                    onChange={() => setChoice(i as TunnelChoice)}
                  />
                  <span className="app-radio" aria-hidden />
                  <span className="app-choice-text">
                    <span className="t">{t.title}</span>
                    <span className="d">{off ? t.missing : t.desc}</span>
                  </span>
                </label>
              );
            })}
          </div>
          {choice === 1 && (
            <label className="app-field">
              <span className="app-field-label">{L("域名", "Domain")}</span>
              <input
                className="app-input mono"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="hub.example.com"
                disabled={busy}
              />
            </label>
          )}
          {!cf && !ts && (
            <div className="app-notice">
              <Monitor />
              <span>{L("没有找到 cloudflared 或 Tailscale。先安装其中一个，才能开放到公网。", "Neither cloudflared nor Tailscale was found. Install one of them to open the Hub to the internet.")}</span>
            </div>
          )}
          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
            <button
              type="button"
              className="app-btn app-btn-primary"
              onClick={() => void submit()}
              disabled={busy || (!cf && !ts) || (choice === 1 && !hostname.trim())}
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
          <span className={cn("app-pill", on && "green")}>{on ? L("已开启", "On") : L("未开启", "Off")}</span>
          {owner && (
            <button type="button" className="app-btn app-btn-small" style={{ marginLeft: 8 }} disabled={busy} onClick={() => setConfirm(!on)}>
              {on ? L("关闭", "Turn off") : L("开启", "Turn on")}
            </button>
          )}
        </span>
      </div>
      <Modal open={confirm !== null} onOpenChange={(o) => !busy && !o && setConfirm(null)}
        title={confirm ? L("开启端到端加密", "Turn on end-to-end encryption") : L("关闭端到端加密", "Turn off end-to-end encryption")}>
        <p className="app-modal-note">
          {confirm
            ? L("开启后，新消息和附件只有收发双方、以及双方节点的主人能读；Hub、管理员和其他成员都读不到。适合几个人共用的 Hub。还没升级的节点会继续收发明文，并标为「未加密」。开启前的消息不会被加密。",
              "New messages and attachments can then be read only by the two sides and the people who own them; the Hub, its admins and other members cannot. Meant for Hubs shared by several people. Nodes that are not upgraded yet keep sending plain messages, marked “Unencrypted”. Earlier messages stay as they are.")
            : L("关闭后，新消息以明文保存在 Hub 上；已经加密的消息仍然只有当事人能读。",
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
