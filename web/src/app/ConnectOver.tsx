import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Monitor, RefreshCw, Server, X } from "lucide-react";
import { appApi, type ConnectJob, type RemoteError, type RemoteRoute, type SSHHost, type WSLDistro } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * Connecting computers from this App over SSH, or into this Windows
 * computer's WSL: tick them, and each is logged into, set up and joined to
 * the workspace, step by step (docs/design/computers.md, "从 SSH 或 WSL 加电脑").
 */
export function ConnectOver({ hubId, via, onConnected, purpose = "machine" }: {
  hubId: string;
  via: "ssh" | "wsl";
  /** one computer connected (its id), or with purpose "hub" the workspace set up (its id in this App) */
  onConnected: (id: string) => void;
  /** "hub": set up a workspace on one server (添加工作区) instead of connecting computers */
  purpose?: "machine" | "hub";
}) {
  const forHub = purpose === "hub";
  const [hosts, setHosts] = useState<Item[] | null>(null);
  const [listError, setListError] = useState<unknown>(null);
  const [noSSH, setNoSSH] = useState<RemoteError | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState("");
  const [manual, setManual] = useState("");
  const [manualPort, setManualPort] = useState("");
  const [jobs, setJobs] = useState<Record<string, ConnectJob>>({});
  const [startError, setStartError] = useState<unknown>(null);
  const reported = useRef(false);

  // Only the latest read counts (switching between SSH and WSL mid-read).
  const seq = useRef(0);
  const load = () => {
    setListError(null);
    const n = ++seq.current;
    const p: Promise<Item[]> = via === "ssh"
      ? appApi.sshHosts(hubId).then((r) => {
        if (n === seq.current) setNoSSH(r.error || null);
        // "Connected" is about computers of a workspace: not shown when setting one up.
        return r.hosts.map((h) => sshItem(forHub ? { ...h, connected: undefined } : h));
      })
      : appApi.wslDistros(hubId).then((r) => r.distros.map(wslItem));
    p.then((h) => { if (n === seq.current) setHosts(h); })
      .catch((e) => { if (n === seq.current) { setHosts([]); setListError(e); } });
  };
  useEffect(() => {
    setHosts(null);
    setPicked(new Set());
    setJobs({});
    reported.current = false;
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, via]);

  // Follow the computers being connected.
  const running = Object.values(jobs).filter((j) => !j.done);
  useEffect(() => {
    if (running.length === 0) return;
    const t = setTimeout(() => {
      for (const j of running) {
        appApi.connectJob(j.id).then((n) => setJobs((all) => ({ ...all, [keyOf(n.route)]: n }))).catch(() => undefined);
      }
    }, 700);
    return () => clearTimeout(t);
  }, [jobs]); // eslint-disable-line react-hooks/exhaustive-deps

  // One computer asked for and connected: show it. (The callback may be a
  // new function on every render: keep the latest without restarting.)
  const report = useRef(onConnected);
  report.current = onConnected;
  useEffect(() => {
    const all = Object.values(jobs);
    const done = all.length === 1 && all[0].done ? (forHub ? all[0].hub_id : all[0].machine_id) : undefined;
    if (reported.current || !done) return;
    reported.current = true;
    const t = setTimeout(() => report.current(done), 900);
    return () => clearTimeout(t);
  }, [jobs, forHub]);

  const start = async (routes: RemoteRoute[]) => {
    setStartError(null);
    for (const rt of routes) {
      try {
        const j = forHub ? await appApi.installHubOver(rt) : await appApi.connectMachine(hubId, rt);
        setJobs((all) => ({ ...all, [keyOf(rt)]: j }));
      } catch (e) {
        setStartError(e);
        return;
      }
    }
  };
  const busy = running.length > 0;
  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return (hosts || []).filter((h) => !q || `${h.name} ${h.sub}`.toLowerCase().includes(q));
  }, [hosts, filter]);
  // A workspace goes on one server: picking another replaces the pick.
  const toggle = (k: string) => setPicked((s) => {
    const n = new Set(forHub ? [] : s);
    if (s.has(k)) n.delete(k); else n.add(k);
    return n;
  });
  const manualRoute = (): RemoteRoute | null => {
    const t = manual.trim();
    if (!t) return null;
    const port = parseInt(manualPort, 10);
    return { via: "ssh", target: t, ...(port > 0 ? { port } : {}) };
  };
  const chosen: RemoteRoute[] = [
    ...(hosts || []).filter((h) => picked.has(h.key)).map((h) => h.route),
    ...(via === "ssh" && manualRoute() ? [manualRoute()!] : []),
  ];
  const jobList = Object.values(jobs);

  if (noSSH) {
    return <div className="app-empty-note">{L("这台电脑上没有 ssh 命令。装上 OpenSSH 后再来，或者用「复制命令」。", "This computer has no ssh command. Install OpenSSH, or use Copy a command.")}</div>;
  }
  return (
    <div className="cx" data-testid={`connect-${via}`}>
      {jobList.length > 0 ? (
        <div className="cx-jobs">
          {jobList.map((j) => <JobRow key={j.id} job={j} forHub={forHub} onRetry={() => void start([j.route])} />)}
          {!busy && (
            <div className="cx-again">
              <button type="button" className="app-group-action" onClick={() => { setJobs({}); setPicked(new Set()); setManual(""); reported.current = false; load(); }}>
                {L("再连别的", "Connect another")}
              </button>
            </div>
          )}
        </div>
      ) : (
        <>
          <div className="add-note">{forHub
            ? L("选一台能用 SSH 登录的 Linux 服务器，App 会登录上去装好工作区（自动 HTTPS，地址是 <IP>.sslip.io，要能开 80、443 端口），再把它加进这个 App。已经装过的，直接加进来。只用你自己的 ssh 和 ssh-agent，不读、不存密钥。", "Pick a Linux server you can log in to with SSH: the App installs the workspace there (automatic HTTPS at <IP>.sslip.io; ports 80 and 443 must be open) and adds it to this App. If one is there already, it is just added. It uses your own ssh and ssh-agent, and never reads or keeps keys.")
            : via === "ssh"
            ? L("勾上要连的电脑，App 会用你的 SSH 登录上去，装好 myco 并连上这个工作区。只用你自己的 ssh 和 ssh-agent，不读、不存密钥。", "Tick the computers to connect: the App logs in with your SSH, installs myco there and connects it to this workspace. It uses your own ssh and ssh-agent, and never reads or keeps keys.")
            : L("勾上要连的 WSL，App 会进去装好 myco 并连上这个工作区。", "Tick the WSL distributions to connect: the App installs myco inside and connects it to this workspace.")}</div>
          {hosts === null ? (
            <div className="app-empty-note"><span className="app-spinner" />{via === "ssh" ? L("正在读 SSH 配置…", "Reading your SSH config…") : L("正在找 WSL…", "Looking for WSL…")}</div>
          ) : listError ? (
            <ErrorText error={listError} onRetry={load} />
          ) : (
            <>
              {hosts.length > 8 && (
                <input className="app-search add-search" value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={L("找一台", "Find one")} aria-label={L("找一台", "Find one")} />
              )}
              {hosts.length === 0 ? (
                <div className="app-empty-note">{via === "ssh"
                  ? L("SSH 配置（~/.ssh/config）里没有主机。在下面直接填一个。", "Your SSH config (~/.ssh/config) has no hosts. Enter one below.")
                  : L("这台 Windows 上没有 WSL。", "This Windows computer has no WSL.")}</div>
              ) : (
                <div className="add-sessions cx-list">
                  {shown.map((h) => (
                    <label key={h.key} className={cn("add-local-row cx-row", h.connected && "is-taken")}>
                      <input type="checkbox" checked={picked.has(h.key)} onChange={() => toggle(h.key)} />
                      <span className="add-local-icon">{via === "ssh" ? <Server /> : <Monitor />}</span>
                      <span className="add-local-main">
                        <span className="add-local-name">{h.name}{h.tag && <span className="cx-tag">{h.tag}</span>}</span>
                        <span className="add-local-sub mono">{h.sub}</span>
                      </span>
                      {h.connected && <span className="cx-done"><Check />{L("连过", "Connected")}</span>}
                    </label>
                  ))}
                </div>
              )}
              <div className="cx-foot">
                {via === "ssh" && (
                  <details className="app-advanced cx-manual">
                    <summary>{L("填一个不在配置里的", "One not in your config")}<ChevronDown /></summary>
                    <div className="cx-manual-row">
                      <input className="app-input mono" value={manual} onChange={(e) => setManual(e.target.value)} placeholder="root@203.0.113.7" aria-label={L("用户名@地址", "user@address")} spellCheck={false} />
                      <input className="app-input mono cx-port" value={manualPort} onChange={(e) => setManualPort(e.target.value.replace(/\D/g, ""))} placeholder="22" aria-label={L("端口", "Port")} inputMode="numeric" />
                    </div>
                  </details>
                )}
                <button type="button" className="app-group-action" onClick={load} title={L("重新读一遍", "Read again")}><RefreshCw />{L("重新读", "Read again")}</button>
              </div>
            </>
          )}
          <ErrorText error={startError} />
          <div className="app-modal-actions">
            {chosen.length === 0 && hosts !== null && hosts.length > 0 && <span className="app-modal-hint">{forHub ? L("先选一台服务器", "Pick a server first") : L("先勾上要连的", "Tick one first")}</span>}
            <button type="button" className="app-btn app-btn-primary" disabled={chosen.length === 0} onClick={() => void start(chosen)}>
              {forHub ? L("装好并加进来", "Set it up") : chosen.length > 1 ? L(`连上这 ${chosen.length} 台`, `Connect these ${chosen.length}`) : L("连上", "Connect")}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

interface Item { key: string; name: string; sub: string; tag?: string; connected?: string; route: RemoteRoute }

function sshItem(h: SSHHost): Item {
  const at = `${h.user ? h.user + "@" : ""}${h.host}${h.port && h.port !== 22 ? `:${h.port}` : ""}`;
  return { key: h.name, name: h.name, sub: h.jump ? `${at} · ${L("经", "via")} ${h.jump}` : at, connected: h.connected, route: { via: "ssh", target: h.name } };
}

function wslItem(d: WSLDistro): Item {
  return {
    key: d.name, name: d.name, connected: d.connected, route: { via: "wsl", target: d.name },
    tag: d.default ? L("默认", "Default") : undefined,
    sub: `WSL ${d.version || 2} · ${d.running ? L("在运行", "Running") : L("没在运行，连的时候会启动它", "Stopped; connecting starts it")}`,
  };
}

function keyOf(rt: RemoteRoute): string {
  return `${rt.via}:${rt.target}:${rt.port || ""}`;
}

const stepWords: Record<string, () => string> = {
  login: () => L("登录，看看那台电脑", "Log in and look around"),
  install: () => L("装好 myco，连上这个工作区", "Install myco and connect it"),
  online: () => L("等它出现在工作区里", "Wait for it on the workspace"),
};
const hubStepWords: Record<string, () => string> = {
  login: () => L("登录，看看那台服务器", "Log in and look around"),
  install: () => L("装好工作区（已经装过就跳过）", "Install the workspace (skipped if it is there)"),
  add: () => L("加进这个 App", "Add it to this App"),
};

/** One computer being connected: its steps, and in words what went wrong. */
function JobRow({ job, onRetry, forHub }: { job: ConnectJob; onRetry: () => void; forHub?: boolean }) {
  const words = forHub ? hubStepWords : stepWords;
  const failed = job.error;
  const name = job.route.target + (job.route.port ? `:${job.route.port}` : "");
  return (
    <div className={cn("cx-job", failed && "is-failed")} data-testid="connect-job">
      <div className="cx-job-name">
        {job.route.via === "ssh" ? <Server /> : <Monitor />}
        <span className="mono">{name}</span>
        {job.done && !failed && <span className="cx-done"><Check />{forHub ? L("加好了", "Added") : L("连上了", "Connected")}</span>}
      </div>
      <ol className="cx-steps">
        {job.steps.map((s) => (
          <li key={s.key} className={cn("cx-step", `is-${s.state}`)}>
            <span className="cx-step-mark">
              {s.state === "done" ? <Check /> : s.state === "failed" ? <X /> : s.state === "running" ? <span className="app-spinner" /> : <span className="cx-dot" />}
            </span>
            <span>{words[s.key]?.() || s.key}{s.key === "login" && s.state === "done" && job.found?.os && (
              <span className="cx-step-sub"> · {job.found.os} {job.found.arch}{job.found.myco ? ` · myco ${job.found.myco.replace(/^myco\s+/, "")}` : ""}</span>
            )}</span>
          </li>
        ))}
      </ol>
      {failed && (
        <div className="cx-error">
          <div>{remoteErrorText(failed, job.route)}</div>
          {failed.command && (
            <div className="app-command"><code>{failed.command}</code><CopyButton value={failed.command} /></div>
          )}
          {(job.log?.length || 0) > 0 && (
            <details className="cx-log">
              <summary>{L("它最后打印的几行", "The last lines it printed")}<ChevronDown /></summary>
              <pre className="mono">{job.log!.join("\n")}</pre>
            </details>
          )}
          <div className="cx-retry"><button type="button" className="app-btn app-btn-small" onClick={onRetry}><RefreshCw />{L("重试", "Try again")}</button></div>
        </div>
      )}
    </div>
  );
}

/** What went wrong reaching or setting up a computer, in words. */
export function remoteErrorText(e: RemoteError, rt: RemoteRoute): string {
  const host = rt.target.replace(/^[^@]*@/, "");
  switch (e.code) {
    case "auth": return L("App 不能替你输密码或密钥口令。先用 ssh-add 把密钥放进 ssh-agent，在终端里确认下面这条不用输密码就能登录，再重试。", "The App cannot type a password or a key's passphrase. Put your key in ssh-agent with ssh-add, check in a terminal that the line below logs in without one, then try again.");
    case "host_unknown": return L("第一次连这台：先在终端里运行下面这条，核对指纹并接受，再回来重试。", "First time with this one: run the line below in a terminal, check the fingerprint and accept it, then try again.");
    case "host_changed": return L(`这台的指纹和上次不一样：可能重装过系统，也可能有人冒充。确认没问题后，运行 ssh-keygen -R ${host} 删掉旧的，再重试。`, `Its fingerprint is not the one seen before: it may have been reinstalled, or someone may be posing as it. Once you are sure, remove the old one with ssh-keygen -R ${host} and try again.`);
    case "refused": return L("连接被拒绝：那台电脑没开 SSH，或者端口不对。", "Connection refused: SSH is not running there, or the port is wrong.");
    case "unknown_host": return L("找不到这个地址：检查名字，或者 SSH 配置里的 HostName。", "No such address: check the name, or HostName in your SSH config.");
    case "unreachable": case "timeout": return L("连不上：地址或端口不对，或者那台电脑没开、被防火墙挡了。", "Cannot reach it: the address or port is wrong, or it is off or behind a firewall.");
    case "no_shell": return L("登录上了，但它不像 Linux 或 macOS 的命令行。", "Logged in, but it does not answer like a Linux or macOS shell.");
    case "unsupported": return L("这种方式只能连 Linux 和 macOS。Windows 电脑请用「复制命令」里的 PowerShell 那条。", "This way connects Linux and macOS only. For a Windows computer use the PowerShell line under Copy a command.");
    case "missing_curl": return L("那台电脑上没有 curl：先装上（比如 apt install curl），再重试。", "It has no curl: install it (apt install curl, say) and try again.");
    case "missing_bash": return L("那台电脑上没有 bash：先装上，再重试。", "It has no bash: install it and try again.");
    case "install_failed": return L("装的时候出错了，下面是它最后打印的几行。", "Installing failed; the last lines it printed are below.");
    case "not_online": return L("装好了，但它还没出现在工作区里：多半是那台电脑连不上这个工作区的地址。", "It is set up but has not shown up on the workspace: most likely it cannot reach the workspace's address.");
    case "needs_sudo": return L("装工作区要 root 权限，而那台服务器上的 sudo 要输密码，App 替你输不了。在终端里运行下面这条（会问 sudo 密码），装好后用它最后打印的链接添加；或者用 root 登录。", "Installing the workspace needs root, and sudo there asks for a password the App cannot type. Run the line below in a terminal (it asks for the sudo password) and add the workspace with the link it prints last; or log in as root.");
    case "hub_unreachable": return L("工作区装好了，但从这台电脑连不上它的地址：多半是服务器的防火墙或云服务商的安全组没放开 80、443 端口。放开后点「重试」（已经装好的会直接加进来）。", "The workspace is installed but this computer cannot reach its address: most likely the server's firewall or the cloud provider's security group blocks ports 80 and 443. Open them and try again (it is not installed twice).");
    case "no_service": return L("装好也连上了，但那台电脑上装不了 myco 后台服务（没有 systemd 用户服务，比如容器、没开 systemd 的 WSL）。在那台电脑上运行下面这条，并让它一直开着（比如放在 tmux 里）：", "Installed and joined, but the runner service cannot be installed there (no systemd user services: a container, or WSL without systemd). Run this there and keep it running (in tmux, say):");
    case "wsl_missing": return L("找不到这个 WSL：在 PowerShell 里运行 wsl -l -v 看看有哪些。", "No such WSL distribution: run wsl -l -v in PowerShell to see which there are.");
    case "ssh_missing": return L("这台电脑上没有 ssh 命令。", "This computer has no ssh command.");
    default: return e.message;
  }
}
