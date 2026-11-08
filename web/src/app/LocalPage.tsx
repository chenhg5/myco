import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  AlertTriangle, Check, Cloud, Copy, ExternalLink, Terminal, Waypoints,
} from "lucide-react";
import {
  appApi, errorMessage, type LocalState, type StartLocalHubInput, type StartLocalHubResult,
} from "@/appapi";
import { ConfirmDialog, ErrorText } from "@/app/dialogs";
import { cn } from "@/lib/utils";

type Choice = "cf-temp" | "cf-domain" | "tailscale" | "none";

const CHOICES: { key: Choice; title: string; desc: string; missing?: string }[] = [
  {
    key: "cf-temp",
    title: "Cloudflare 临时地址",
    desc: "不用账号，地址每次重启都会变，适合先试试。",
    missing: "需要先安装 cloudflared。",
  },
  {
    key: "cf-domain",
    title: "Cloudflare 自己的域名",
    desc: "需要 Cloudflare 账号和托管在 Cloudflare 的域名，地址固定。",
    missing: "需要先安装 cloudflared。",
  },
  {
    key: "tailscale",
    title: "Tailscale Funnel",
    desc: "地址固定（xxx.ts.net），需要 Tailscale 账号并为这台设备开启 Funnel。",
    missing: "需要先安装 Tailscale。",
  },
  {
    key: "none",
    title: "只在本机/局域网",
    desc: "不对外公开，只有同一局域网里的电脑能连上。",
  },
];

function useCopy(value: string) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | null>(null);
  useEffect(() => () => { if (timer.current !== null) window.clearTimeout(timer.current); }, []);
  const copy = useCallback(async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(value);
    } catch {
      const ta = document.createElement("textarea");
      ta.value = value;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand("copy"); } catch { /* ignore */ }
      document.body.removeChild(ta);
    }
    setCopied(true);
    if (timer.current !== null) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 1500);
  }, [value]);
  return { copied, copy };
}

function CopyButton({ value }: { value: string }) {
  const { copied, copy } = useCopy(value);
  return (
    <button type="button" className="app-btn app-btn-small" onClick={() => void copy()}>
      {copied ? <Check /> : <Copy />}{copied ? "已复制" : "复制"}
    </button>
  );
}

function UrlRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="app-kv">
      <span className="app-kv-label">{label}</span>
      <span className="app-kv-value mono clip" title={value}>{value}</span>
      <CopyButton value={value} />
    </div>
  );
}

function ToolRow({ icon: Icon, title, installed, detail, action }: {
  icon: typeof Terminal;
  title: string;
  installed: boolean;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="app-row app-row-static">
      <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
      <span className="app-row-main">
        <span className="app-row-title">{title}</span>
        <span className="app-row-sub" title={detail}>
          <span className={cn("dot", installed ? "working" : "offline")} />
          <span className="txt">{detail}</span>
        </span>
      </span>
      {action && <span className="app-row-trail">{action}</span>}
    </div>
  );
}

export function LocalPage({ refreshKey, onRefresh, onViewHub }: {
  /** Bumped by the global refresh / poll so this page stays current. */
  refreshKey: number;
  onRefresh: () => Promise<void>;
  onViewHub: (localUrl: string) => void;
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [choice, setChoice] = useState<Choice | null>(null);
  const [hostname, setHostname] = useState("");
  const [busy, setBusy] = useState<null | "install" | "start" | "stop" | "add">(null);
  const [installError, setInstallError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [result, setResult] = useState<StartLocalHubResult | null>(null);
  const [confirmStop, setConfirmStop] = useState(false);

  const reload = useCallback(async () => {
    try {
      const d = await appApi.local();
      setLocal(d);
      setLoadError(null);
    } catch (e) {
      setLoadError(e);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    appApi.local()
      .then((d) => { if (!cancelled) { setLocal(d); setLoadError(null); } })
      .catch((e) => { if (!cancelled) setLoadError(e); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [refreshKey]);

  const cf = !!local?.tools?.cloudflared;
  const ts = !!local?.tools?.tailscale;

  // Pick a usable default, and drop a choice whose tool went away.
  useEffect(() => {
    setChoice((cur) => {
      const ok = (c: Choice) => (c === "cf-temp" || c === "cf-domain" ? cf : c === "tailscale" ? ts : true);
      if (cur && ok(cur)) return cur;
      return cf ? "cf-temp" : "none";
    });
  }, [cf, ts]);

  const tunnelInput = (): StartLocalHubInput => {
    if (choice === "cf-domain") return { tunnel: "cloudflare", hostname: hostname.trim() };
    if (choice === "tailscale") return { tunnel: "tailscale" };
    if (choice === "none") return { tunnel: "none" };
    return { tunnel: "cloudflare" };
  };

  const install = async () => {
    setBusy("install");
    setInstallError(null);
    try {
      await appApi.installMyco();
      await reload();
    } catch (e) {
      setInstallError(e);
    } finally {
      setBusy(null);
    }
  };

  const start = async (mode: "start" | "add") => {
    setBusy(mode);
    setActionError(null);
    if (mode === "start") setResult(null);
    try {
      const r = await appApi.startLocalHub(tunnelInput());
      setResult(r);
      await reload();
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(null);
    }
  };

  const stop = async () => {
    setBusy("stop");
    setActionError(null);
    try {
      await appApi.stopLocalHub();
      setConfirmStop(false);
      setResult(null);
      await reload();
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(null);
    }
  };

  if (!local) {
    return (
      <div className="app-page">
        {loadError ? (
          <div className="app-list">
            <div className="app-error-row">
              <AlertTriangle />
              <span className="err" title={errorTitle(loadError)}>{errorTitle(loadError)}</span>
            </div>
          </div>
        ) : (
          <p className="app-summary">{loading ? "正在读取本机状态…" : "没有读到本机状态。"}</p>
        )}
      </div>
    );
  }

  const hub = local.hub;
  const running = !!hub?.running;
  const myco = local.myco;
  const startDisabled = busy !== null || !myco || (choice === "cf-domain" && !hostname.trim());

  return (
    <div className="app-page">
      <p className="app-summary">在这台电脑上开一个 Hub，不需要服务器。电脑要保持开机。</p>

      <section className="app-group">
        <div className="app-group-title"><span className="name">运行环境</span></div>
        <div className="app-list">
          <ToolRow
            icon={Terminal}
            title="myco 命令行"
            installed={!!myco}
            detail={myco ? `已安装 v${myco.version} · ${myco.path}` : "未安装，本机 Hub 需要它"}
            action={myco ? undefined : (
              <button type="button" className="app-btn" onClick={() => void install()} disabled={busy !== null}>
                {busy === "install" ? <><span className="app-spinner" />安装中…</> : "安装"}
              </button>
            )}
          />
          <ToolRow
            icon={Cloud}
            title="cloudflared"
            installed={cf}
            detail={cf
              ? `已安装 · ${local.tools.cloudflared!.path}`
              : "未安装，在终端运行 brew install cloudflared"}
          />
          <ToolRow
            icon={Waypoints}
            title="Tailscale"
            installed={ts}
            detail={ts ? `已安装 · ${local.tools.tailscale!.path}` : "未安装"}
            action={ts ? undefined : (
              <a className="app-btn" href="https://tailscale.com/download/mac" target="_blank" rel="noreferrer">
                下载 Tailscale<ExternalLink />
              </a>
            )}
          />
        </div>
        {busy === "install" && (
          <div className="app-progress"><span className="app-spinner" />正在安装 myco 命令行，可能要 10–60 秒…</div>
        )}
        {installError !== null && <ErrorText error={installError} />}
      </section>

      {hub && (
        <section className="app-group">
          <div className="app-group-title"><span className="name">本机 Hub</span></div>
          <div className="app-list">
            <div className="app-kv">
              <span className="app-kv-label">状态</span>
              <span className="app-kv-value">
                <span className={cn("app-pill", running ? "green" : "red")}>
                  <span className={cn("dot", running ? "working" : "offline")} />
                  {running ? "运行中" : "已停止"}
                </span>
              </span>
            </div>
            <UrlRow label="本机地址" value={hub.local_url} />
            {hub.public_url && <UrlRow label="公网地址" value={hub.public_url} />}
            {hub.version && (
              <div className="app-kv">
                <span className="app-kv-label">版本</span>
                <span className="app-kv-value">v{hub.version}</span>
              </div>
            )}
          </div>

          {running && (
            <div className="app-actions">
              {hub.added ? (
                <button
                  type="button"
                  className="app-btn app-btn-primary"
                  onClick={() => onViewHub(hub.public_url || hub.local_url)}
                >
                  在节点页查看
                </button>
              ) : (
                <button type="button" className="app-btn app-btn-primary" onClick={() => void start("add")} disabled={busy !== null}>
                  {busy === "add" ? <><span className="app-spinner" />正在加入…</> : "加入 App"}
                </button>
              )}
              <button type="button" className="app-btn app-btn-danger" onClick={() => setConfirmStop(true)} disabled={busy !== null}>
                停止本机 Hub
              </button>
            </div>
          )}
        </section>
      )}

      {result && (
        <section className="app-group">
          <div className="app-group-title"><span className="name">已开启</span></div>
          <div className="app-list">
            <UrlRow label="公网地址" value={result.public_url} />
            {result.join_url && <UrlRow label="入网链接" value={result.join_url} />}
          </div>
          {result.join_url && (
            <div className="app-hint">
              在其他电脑上运行 <code>curl -fsSL {result.join_url} | bash</code> 加入。
            </div>
          )}
          {(result.warnings || []).map((w) => (
            <div className="app-notice" key={w}>
              <AlertTriangle />
              <span>{w}</span>
            </div>
          ))}
        </section>
      )}

      {!running && (
        <section className="app-group">
          <div className="app-group-title"><span className="name">公网访问</span></div>
          <div className="app-list">
            {CHOICES.map((c) => {
              const disabled = c.key === "cf-temp" || c.key === "cf-domain" ? !cf : c.key === "tailscale" ? !ts : false;
              return (
                <label key={c.key} className={cn("app-choice", disabled && "disabled")}>
                  <input
                    type="radio"
                    name="tunnel"
                    className="app-sr-only"
                    checked={choice === c.key}
                    disabled={disabled}
                    onChange={() => setChoice(c.key)}
                  />
                  <span className="app-radio" aria-hidden />
                  <span className="app-choice-text">
                    <span className="t">{c.title}</span>
                    <span className="d">{disabled ? c.missing : c.desc}</span>
                  </span>
                </label>
              );
            })}
          </div>

          {choice === "cf-domain" && (
            <>
              <label className="app-field">
                <span className="app-field-label">域名</span>
                <input
                  className="app-input mono"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  placeholder="hub.example.com"
                />
              </label>
              <div className="app-hint">
                先在这台电脑上执行一次 <code>cloudflared tunnel login</code>，并确认域名已托管在 Cloudflare。
              </div>
            </>
          )}

          <ErrorText error={actionError} />
          <div className="app-actions">
            <button
              type="button"
              className="app-btn app-btn-primary"
              onClick={() => void start("start")}
              disabled={startDisabled}
            >
              {busy === "start" ? <><span className="app-spinner" />正在启动 Hub…</> : "开启本机 Hub"}
            </button>
            {!myco && <span className="app-small-note">先安装 myco 命令行。</span>}
          </div>
          {busy === "start" && (
            <div className="app-progress"><span className="app-spinner" />正在启动 Hub，可能要 20–90 秒…</div>
          )}
        </section>
      )}

      <ConfirmDialog
        open={confirmStop}
        onOpenChange={(o) => { if (!o) setConfirmStop(false); }}
        title="停止本机 Hub？"
        description="Hub 会在这台电脑上停止运行。"
        consequences={<p>其他机器会连不上它。数据和令牌都保留，重新开启即可恢复。</p>}
        confirmLabel="停止"
        busy={busy === "stop"}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void stop()}
      />

      <div className="app-sr-only" aria-live="polite">
        {result ? "本机 Hub 已开启。" : busy === "start" ? "正在启动 Hub。" : ""}
      </div>
    </div>
  );
}

function errorTitle(e: unknown): string {
  return errorMessage(e);
}
