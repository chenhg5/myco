import { useCallback, useEffect, useState } from "react";
import {
  AlertTriangle, Check, Cloud, Globe, Info, Monitor, Palette, Pencil, Plus, Server, Terminal,
} from "lucide-react";
import {
  appApi, type AppInfo, type AppUpdate, type Hub, type LocalState,
  type LocalTunnel, type StartLocalHubResult,
} from "@/appapi";
import { ver } from "@/app/nodes";
import { renameHub } from "@/app/hubNames";
import { ErrorText, Modal, ConfirmDialog } from "@/app/dialogs";
import type { TextSize, ThemeMode } from "@/lib/theme";
import { cn } from "@/lib/utils";

type Section = "hubs" | "appearance" | "about";

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
      status = <span className="app-about-status"><span className="app-spinner" />正在检查…</span>;
      break;
    case "installing":
      status = <span className="app-about-status"><span className="app-spinner" />正在下载 {st.latest}…</span>;
      break;
    case "relaunching":
      status = <span className="app-about-status ok">已安装 {st.latest}，正在重新打开…</span>;
      break;
    case "failed":
      status = <span className="app-about-status bad">检查失败</span>;
      action = <button type="button" className="app-btn" onClick={() => void check()}>重试</button>;
      break;
    case "checked": {
      const u = st.u;
      if (u.error) {
        status = <span className="app-about-status bad">连不上 GitHub</span>;
        action = <button type="button" className="app-btn" onClick={() => void check()}>重试</button>;
      } else if (u.available && u.can_install) {
        status = <span className="app-about-status new">有新版本 {u.latest}</span>;
        action = <button type="button" className="app-btn app-btn-primary" onClick={() => void install(u)}>更新并重新打开</button>;
      } else if (u.available) {
        status = <span className="app-about-status new">有新版本 {u.latest}，请到 GitHub 下载</span>;
      } else {
        status = <span className="app-about-status ok"><Check />已是最新</span>;
        action = <button type="button" className="app-btn" onClick={() => void check()}>再检查一次</button>;
      }
    }
  }
  const failure = st.step === "failed" ? st.error : null;
  const cli = local?.myco;

  return (
    <>
      <p className="app-summary">myco App 把多个 Hub 上的节点放进一个窗口。</p>
      <div className="app-list">
        <div className="app-kv"><span className="app-kv-label">App 版本</span><span className="app-kv-value mono">{info?.version || "—"}</span></div>
        <div className="app-kv">
          <span className="app-kv-label">命令行工具</span>
          <span className="app-kv-value">
            {cli ? <><span className="mono">{ver(cli.version)}</span><span className="app-about-path mono">{cli.path}</span></> : "未安装"}
          </span>
        </div>
        <div className="app-kv app-about-update">
          <span className="app-kv-label">更新</span>
          <span className="app-kv-value">{status}{action}</span>
        </div>
      </div>
      <ErrorText error={failure} />
      <p className="app-about-note">
        更新会下载 GitHub 上最新发布的 App，校验 SHA256 后替换「应用程序」里的 myco，再自动重新打开。命令行工具和 runner 服务另外用 <code>myco upgrade</code> 升级。
      </p>
      <p className="app-about-note">节点、任务和对话都由 Hub 提供，App 只负责显示和转发。</p>
    </>
  );
}

const SECTIONS: { key: Section; label: string; icon: typeof Server }[] = [
  { key: "hubs", label: "Hub", icon: Server },
  { key: "appearance", label: "外观", icon: Palette },
  { key: "about", label: "关于", icon: Info },
];

const SIZES: { size: TextSize; label: string }[] = [
  { size: "small", label: "小" },
  { size: "standard", label: "标准" },
  { size: "large", label: "大" },
];

const THEMES: { mode: ThemeMode; label: string }[] = [
  { mode: "system", label: "跟随系统" },
  { mode: "light", label: "浅色" },
  { mode: "dark", label: "深色" },
];

function stripSlash(u: string) {
  return u.replace(/\/+$/, "");
}

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

const TUNNELS: { key: LocalTunnel; title: string; desc: string; needsDomain?: boolean; missing?: string }[] = [
  {
    key: "cloudflare",
    title: "Cloudflare 临时地址",
    desc: "不用账号，地址每次重启都会变，适合先试试。",
    missing: "需要先安装 cloudflared。",
  },
  {
    key: "cloudflare",
    title: "Cloudflare 自己的域名",
    desc: "需要 Cloudflare 账号和托管在 Cloudflare 的域名，地址固定。",
    needsDomain: true,
    missing: "需要先安装 cloudflared。",
  },
  {
    key: "tailscale",
    title: "Tailscale Funnel",
    desc: "地址固定（xxx.ts.net），需要 Tailscale 账号并为这台设备开启 Funnel。",
    missing: "需要先安装 Tailscale。",
  },
];

type TunnelChoice = 0 | 1 | 2;

/** Settings: hubs (including the local one), appearance and about. */
export function SettingsPage({ hubs, info, theme, onTheme, textSize, onTextSize, refreshKey, sectionRequest, onRefresh, onAddHub }: {
  hubs: Hub[];
  info: AppInfo | null;
  theme: ThemeMode;
  onTheme: (mode: ThemeMode) => void;
  textSize: TextSize;
  onTextSize: (size: TextSize) => void;
  /** Bumped by the App poll so the local hub state stays current. */
  refreshKey: number;
  /** Bumped when the Hub switcher asks for the Hub section. */
  sectionRequest?: number;
  onRefresh: () => Promise<void>;
  onAddHub: () => void;
}) {
  const [section, setSection] = useState<Section>("hubs");
  const [local, setLocal] = useState<LocalState | null>(null);
  const [localError, setLocalError] = useState<unknown>(null);
  const [busy, setBusy] = useState<null | "start" | "stop">(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [renameTarget, setRenameTarget] = useState<Hub | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [stopOpen, setStopOpen] = useState(false);
  const [publicOpen, setPublicOpen] = useState(false);

  // "Hub 设置…" in the switcher should land on the Hub list, even when this
  // page was last left on another section.
  useEffect(() => {
    if (sectionRequest) setSection("hubs");
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
      setRemoveTarget(null);
      await onRefresh();
    } catch (e) {
      setActionError(e);
    }
  };

  const localRunning = !!local?.hub?.running;

  return (
    <div className="app-page app-settings">
      <div className="app-settings-nav" role="tablist" aria-label="设置">
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
            <p className="app-summary">Hub 是节点所在的服务器。连接后，所有页面只显示选中的 Hub。</p>
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
                      aria-label="重命名"
                      title="重命名"
                    >
                      <Pencil />
                    </button>
                  </div>

                  <div className="app-setting-rows">
                    <div className="app-kv"><span className="app-kv-label">地址</span><span className="app-kv-value mono clip" title={h.url}>{h.url}</span></div>
                    <div className="app-kv"><span className="app-kv-label">版本</span><span className="app-kv-value">{h.hub_version ? ver(h.hub_version) : "—"}</span></div>
                    <div className="app-kv">
                      <span className="app-kv-label">状态</span>
                      <span className="app-kv-value">
                        <span className={cn("app-pill", h.ok ? "green" : "red")}>
                          <span className={cn("dot", h.ok ? "working" : "offline")} />
                          {h.ok ? "已连接" : "连不上"}
                        </span>
                      </span>
                    </div>
                    {localHub && (
                      <div className="app-kv">
                        <span className="app-kv-label">访问</span>
                        <span className="app-kv-value">
                          {hubLocal?.running && hubLocal.public_url
                            ? <span className="mono clip" title={hubLocal.public_url}>{hubLocal.public_url}</span>
                            : "只在本机/局域网"}
                        </span>
                      </div>
                    )}
                  </div>
                  {localHub && !h.ok && !localRunning && (
                    <div className="app-small-note" style={{ marginTop: 8 }}>本机 Hub 已停止。可以在下面重新启动。</div>
                  )}

                  <div className="app-setting-actions">
                    {localHub ? (
                      <>
                        <button type="button" className="app-btn" onClick={() => setPublicOpen(true)} disabled={busy !== null}>
                          <Globe />开放到公网…
                        </button>
                        <button type="button" className="app-btn app-btn-danger" onClick={() => setStopOpen(true)} disabled={busy !== null || !localRunning}>
                          停止本机 Hub
                        </button>
                      </>
                    ) : (
                      <button type="button" className="app-btn app-btn-danger" onClick={() => { setActionError(null); setRemoveTarget(h); }}>
                        移除
                      </button>
                    )}
                  </div>
                </section>
              );
            })}

            {hubs.length === 0 && (
              <div className="app-empty-note" style={{ marginTop: 8 }}>还没有 Hub。先连接一个，或者在这台电脑上启动一个。</div>
            )}

            <div className="app-setting-add">
              <button type="button" className="app-setting-add-row" onClick={onAddHub}>
                <Plus />添加 Hub…
              </button>
              {!localRunning && (
                <button
                  type="button"
                  className="app-setting-add-row"
                  onClick={() => void startLocal()}
                  disabled={busy !== null}
                >
                  {busy === "start" ? <><span className="app-spinner" />正在启动…</> : <><Terminal />在这台电脑上启动 Hub</>}
                </button>
              )}
            </div>
            <div className="app-hint">
              名称只在这个 App 里显示，不会改到 Hub 上。地址和令牌保存在这台电脑上。
            </div>
          </>
        )}

        {section === "appearance" && (
          <>
            <p className="app-summary">外观会立即生效。</p>
            <section className="app-setting-card">
              <div className="seg seg-fill" role="tablist" aria-label="外观">
                {THEMES.map((t) => (
                  <button
                    key={t.mode}
                    type="button"
                    role="tab"
                    aria-selected={theme === t.mode}
                    className={cn("seg-btn", theme === t.mode && "active")}
                    onClick={() => onTheme(t.mode)}
                  >
                    {theme === t.mode && <Check />}
                    {t.label}
                  </button>
                ))}
              </div>
              <div className="app-hint">跟随系统时，App 会随 macOS 的浅色/深色切换。</div>
            </section>
            <p className="app-summary">文字大小</p>
            <section className="app-setting-card">
              <div className="seg seg-fill" role="tablist" aria-label="文字大小">
                {SIZES.map((t) => (
                  <button
                    key={t.size}
                    type="button"
                    role="tab"
                    aria-selected={textSize === t.size}
                    className={cn("seg-btn", textSize === t.size && "active")}
                    onClick={() => onTextSize(t.size)}
                  >
                    {textSize === t.size && <Check />}
                    {t.label}
                  </button>
                ))}
              </div>
              <div className="app-hint">整个窗口一起放大或缩小。</div>
            </section>
          </>
        )}

        {section === "about" && <AboutSection info={info} local={local} />}
      </div>

      <Modal
        open={!!renameTarget}
        onOpenChange={(o) => { if (!o) setRenameTarget(null); }}
        title="重命名 Hub"
        description="只在这个 App 里显示，不会改到 Hub 上。"
      >
        <label className="app-field">
          <span className="app-field-label">名称</span>
          <input
            className="app-input"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            autoFocus
            placeholder={renameTarget?.name}
          />
        </label>
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setRenameTarget(null)}>取消</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveRename} disabled={!renameValue.trim()}>保存</button>
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
        title="停止本机 Hub？"
        description="Hub 会在这台电脑上停止运行。"
        consequences={<p>其他机器会连不上它。数据和令牌都保留，重新开启即可恢复。</p>}
        confirmLabel="停止"
        busy={busy === "stop"}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void stopLocal()}
      />

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => !o && setRemoveTarget(null)}
        title={removeTarget ? `移除 ${removeTarget.name}？` : "移除 Hub"}
        description="只会从这个 App 里移除。Hub 本身和它上面的 agent 都不受影响，仍在照常运行。"
        consequences={<p>你将看不到这个 Hub 的节点，之后只能凭 App 连接链接重新添加。</p>}
        confirmLabel="移除 Hub"
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
      title="开放到公网"
      description="让不在同一局域网的机器也能连上这台电脑上的 Hub。"
    >
      {result ? (
        <>
          <div className="app-success">
            <Check />
            <span>已开放</span>
          </div>
          <div className="app-kv"><span className="app-kv-label">公网地址</span><span className="app-kv-value mono clip" title={result.public_url}>{result.public_url}</span></div>
          {result.join_url && (
            <div className="app-kv"><span className="app-kv-label">入网链接</span><span className="app-kv-value mono clip" title={result.join_url}>{result.join_url}</span></div>
          )}
          {(result.warnings || []).map((w) => (
            <div className="app-notice" key={w}>
              <AlertTriangle />
              <span>{w}</span>
            </div>
          ))}
          <div className="app-modal-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={close}>完成</button>
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
              <span className="app-field-label">域名</span>
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
              <span>没有找到 cloudflared 或 Tailscale。先安装其中一个，才能开放到公网。</span>
            </div>
          )}
          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>取消</button>
            <button
              type="button"
              className="app-btn app-btn-primary"
              onClick={() => void submit()}
              disabled={busy || (!cf && !ts) || (choice === 1 && !hostname.trim())}
            >
              {busy ? <><span className="app-spinner" />正在开放…</> : <><Cloud />开放</>}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
