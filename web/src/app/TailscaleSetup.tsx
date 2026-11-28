import { useState } from "react";
import { Download, RotateCcw } from "lucide-react";
import { L, E } from "@/lib/i18n";
import { appApi } from "@/appapi";
import { CopyButton } from "@/app/copy";

type Platform = "windows" | "macos" | "linux";
const INSTALL = {
  windows: 'winget install --exact --id Tailscale.Tailscale; if ($LASTEXITCODE -eq 0) { Start-Process "$env:ProgramFiles\\Tailscale\\tailscale-ipn.exe" }',
  macos: "brew install --cask tailscale-app && open -a Tailscale",
  linux: "curl -fsSL https://tailscale.com/install.sh | sh && sudo tailscale up",
};
const START = {
  windows: 'Start-Process "$env:ProgramFiles\\Tailscale\\tailscale-ipn.exe"',
  macos: "open -a Tailscale",
  linux: "sudo tailscale up",
};
const DOWNLOAD = { windows: "https://tailscale.com/download/windows", macos: "https://tailscale.com/download/mac", linux: "https://tailscale.com/download/linux" };

/** Installing a networking app includes a person's browser login and system
 * approval. Offer a complete manual/agent handoff instead of a disabled option. */
export function TailscaleSetup({ installed, name, localURL, onCheck, onError }: {
  installed: boolean; name?: string; localURL?: string;
  onCheck: () => Promise<void>; onError: (e: unknown) => void;
}) {
  const [platform, setPlatform] = useState<Platform>(() => /Win/i.test(navigator.platform || navigator.userAgent) ? "windows" : /Mac/i.test(navigator.platform || navigator.userAgent) ? "macos" : "linux");
  const [checking, setChecking] = useState(false);
  const command = installed ? START[platform] : INSTALL[platform];
  const hubCommand = `myco hub up --tunnel tailscale${name ? ` --name ${name}` : ""}`;
  const prompt = L(
    (localURL ? E`请在这台电脑上帮我配置 Tailscale Funnel，让本机 myco 工作区可通过公网访问。本机地址：${localURL}。先检查现有 Tailscale 安装和登录状态；缺失时从官方渠道安装并启动。浏览器登录、系统网络权限和 Funnel 授权由我本人确认，不要索取或输出密码、验证码和令牌。完成后运行 ${hubCommand}，验证公网地址非空且 /v1/health 返回 ok=true，再给我可用的公网地址。失败时保留本机工作区，并说明卡在哪一步。` : E`请在这台电脑上帮我配置 Tailscale Funnel，让本机 myco 工作区可通过公网访问。先检查现有 Tailscale 安装和登录状态；缺失时从官方渠道安装并启动。浏览器登录、系统网络权限和 Funnel 授权由我本人确认，不要索取或输出密码、验证码和令牌。完成后运行 ${hubCommand}，验证公网地址非空且 /v1/health 返回 ok=true，再给我可用的公网地址。失败时保留本机工作区，并说明卡在哪一步。`),
    (localURL ? E`Help me set up Tailscale Funnel on this computer so its local myco workspace is reachable over the internet. Local address: ${localURL}. Check the existing installation and sign-in first; install from the official source and start it if missing. I will complete browser sign-in, system network approval and Funnel authorization myself; do not request or print passwords, verification codes or tokens. Then run ${hubCommand}. Verify that the public URL is nonempty and /v1/health returns ok=true before giving me the working public address. On failure, keep the local workspace running and explain the blocked step.` : E`Help me set up Tailscale Funnel on this computer so its local myco workspace is reachable over the internet. Check the existing installation and sign-in first; install from the official source and start it if missing. I will complete browser sign-in, system network approval and Funnel authorization myself; do not request or print passwords, verification codes or tokens. Then run ${hubCommand}. Verify that the public URL is nonempty and /v1/health returns ok=true before giving me the working public address. On failure, keep the local workspace running and explain the blocked step.`),
  );
  return <section className="pub-setup" data-testid="tailscale-setup" aria-label={L("安装和连接 Tailscale", "Install and connect Tailscale")}>
    <h3>{installed ? L("已找到 Tailscale，接下来登录并连接", "Tailscale found — sign in and connect next") : L("先安装并连接 Tailscale", "Install and connect Tailscale first")}</h3>
    <div className="seg seg-fill" role="group" aria-label={L("安装系统", "Installation platform")}>
      {(["windows", "macos", "linux"] as const).map(p => <button type="button" key={p} className={`seg-btn ${platform === p ? "active" : ""}`} aria-pressed={platform === p} onClick={() => setPlatform(p)}>{p === "windows" ? "Windows" : p === "macos" ? "macOS" : "Linux"}</button>)}
    </div>
    <ol>
      <li>{installed ? L("启动这台电脑上的 Tailscale。", "Start Tailscale on this computer.") : L("打开官方下载页面安装，或复制下面的命令运行。", "Install from the official download page, or copy and run the command below.")}</li>
      <li>{L("在 Tailscale 中登录账号并连接；系统要求网络权限时，请本人确认。", "Sign in and connect in Tailscale; approve system network permissions yourself when asked.")}</li>
      <li>{L("回到这里重新检测，再点「开放」。首次使用 Funnel 时，按提示完成浏览器授权，然后重试。", "Return here, check again, then click Open. For the first Funnel setup, complete the browser authorization when prompted, then retry.")}</li>
    </ol>
    <div className="pub-setup-actions">
    {!installed && <button type="button" className="app-btn" onClick={() => void appApi.openUrl(DOWNLOAD[platform]).catch(onError)}><Download />{L("打开官方下载", "Open official download")}</button>}
    <button type="button" className="app-btn" disabled={checking} onClick={() => { setChecking(true); void onCheck().catch(onError).finally(() => setChecking(false)); }}><RotateCcw />{checking ? L("正在检测…", "Checking…") : L("我已安装，重新检测", "Installed — check again")}</button>
    </div>
    <p className="app-small-note">{platform === "windows" ? L("在 PowerShell 中运行；没有 winget 时使用上面的官方下载。", "Run in PowerShell; use the official download if winget is unavailable.") : platform === "macos" && !installed ? L("已有 Homebrew 时可用下面的命令；否则使用上面的官方下载。", "Use this command if Homebrew is installed; otherwise use the official download.") : L("在这台电脑的终端中运行。", "Run in a terminal on this computer.")}</p>
    <div className="app-command"><code>{command}</code><CopyButton value={command} /></div>
    <details><summary>{L("让这台电脑上的 agent 帮我配置", "Ask an agent on this computer to set it up")}</summary><div className="app-command is-long"><code>{prompt}</code><CopyButton value={prompt} /></div></details>

  </section>;
}
