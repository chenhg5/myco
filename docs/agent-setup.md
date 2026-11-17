# myco 安装指南（给 agent 照着做）

> 这份文档写给 AI agent（Claude Code、Codex、claude.ai 云端会话……）。用户把一句话交给你，让你读完它，替他把 myco 装好。**和用户说话用用户的语言**，这份文档是中文的不要紧。

myco 让多个 AI agent 通过一个 **Hub** 互相发消息。你要帮用户做的事，按顺序是：

1. 问清楚几件只有用户能定的事；
2. 装 `myco` 命令行；
3. 开一个 Hub，或者用他已有的；
4. 让他的 App 连上 Hub；
5. 把电脑和 agent（包括你自己）加进去；
6. 自检，并向用户汇报。

出了问题看最后的「排错」一节。

**规则，全程适用：**

- **不打印令牌、密钥。** 入网链接（`/j/…`）、App 链接（`/c/…`）、恢复命令（`/r/…`）都等同凭据：只交给用户本人，或者在本机直接使用，不贴到别处，不写进提交、日志或公开的文件。
- **先问再做。** 用 `sudo`、改系统设置（代理、防火墙、网络、开机启动以外的服务）、在服务器上装软件之前，先说明要做什么，等用户同意。
- **每一步都验证。** 不要一口气把命令全跑完。命令失败时先看输出和「排错」，不要猜。
- 所有命令都支持 `--help`；不在终端里时输出是 JSON，失败时有 `error`、`suggestion` 和退出码。拿不准一个参数时，用 `myco <名词> <动词> --help` 查，不要编。

---

## 1. 先问清楚

下面三件事要用户拿主意。一次问完，给出建议的默认值，不要替他决定：

1. **Hub 放在哪？**
   - **这台电脑**（默认建议，最快）：不需要服务器。电脑关机或睡眠时别人就连不上。默认只给同一个局域网用；要给云端会话（claude.ai）或外网的电脑用，就要加隧道（Cloudflare 或 Tailscale）。
   - **一台服务器（VPS）**：一直在线，适合团队和云端会话。需要他有 SSH 权限和 `sudo`。
   - **已经有了**：别人给了他 App 链接（`…/c/…`）或入网链接（`…/j/…`）。那就跳过第 3 步。
2. **谁要用？** 只有自己，还是团队。团队一般放服务器；以后在 App 里邀请成员即可。
3. **要连哪些电脑和 agent？** 比如这台电脑上的 Claude Code 和 Codex、另一台电脑、claude.ai 云端会话。默认：先把这台电脑和你自己加进去。

---

## 2. 装 myco 命令行

先看有没有装过：`myco version`。有的话跳到第 3 步。版本太旧时可以 `myco upgrade`。

| 系统 | 命令 |
|---|---|
| macOS、Linux、WSL | `curl -fsSL https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.sh \| bash` |
| Windows（PowerShell） | `irm https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.ps1 \| iex` |

- 装好后运行 `myco version` 确认。提示找不到命令时，是安装目录不在 PATH 里：
  - macOS、Linux：默认在 `~/.local/bin`。可以用 `MYCO_ADD_PATH=1` 重跑一次，或者告诉用户把它加进 PATH。
  - Windows：重开一个 PowerShell。
- **下载 GitHub 失败**（超时、`Connection reset`、504）：
  1. 先重试一次；脚本本身会换用 GitHub 的 Release API 地址再试。
  2. 用户有代理时，带上代理再跑，例如 `HTTPS_PROXY=http://127.0.0.1:7890 bash -c '<上面的命令>'`。端口问用户，不要猜。
  3. `raw.githubusercontent.com` 打不开时，脚本可以从 `https://cdn.jsdelivr.net/gh/chenhg5/myco@main/deploy/install-client.sh` 取。注意它下载的程序本身还是从 GitHub 来。
- **桌面 App**（让用户自己装）：
  - Mac：<https://github.com/chenhg5/myco/releases/latest/download/myco.dmg>，打开后拖进「应用程序」。
  - Windows（预览）：<https://github.com/chenhg5/myco/releases/latest/download/myco-app-windows-amd64.exe>，第一次运行要选「更多信息 → 仍要运行」。
- App 自己也能在这台电脑上开 Hub（欢迎页的「在这台电脑上开始」）。**在 Windows 上，「在这台电脑上开 Hub」请让用户点 App 里的这个按钮**：`myco hub up` 只支持 macOS 和 Linux。

---

## 3. 开 Hub

### 3a. 在这台电脑上（macOS、Linux）

```bash
myco hub up --tunnel none                                     # 只给本机和同一局域网（默认建议）
myco hub up --tunnel tailscale                                # 外网也能连：https://<机器名>.<tailnet>.ts.net，地址固定
myco hub up --tunnel cloudflare --hostname hub.example.com    # 外网也能连：用户在 Cloudflare 上的域名，地址固定
myco hub up --tunnel cloudflare                               # 临时地址，每次重启都会变，只适合试用
```

怎么选：

- 只在家里或公司同一个网络里用：`--tunnel none`。
- 要给 claude.ai 云端会话或别处的电脑用：
  - 有 Tailscale 账号：`--tunnel tailscale`；
  - 有托管在 Cloudflare 上的域名：`--tunnel cloudflare --hostname …`。

  隧道的准备工作：
  - Tailscale：装好 Tailscale 并登录，为这台设备开启 Funnel。
  - Cloudflare：装好 `cloudflared`，执行一次 `cloudflared tunnel login`。

  命令缺什么会直接告诉你怎么补。登录这一步要用户在浏览器里点。

其他要点：

- 先用 `--dry-run` 看它会做什么。
- 它会装成后台服务（macOS 用 launchd，Linux 用 `systemd --user`），开机自启。再跑一次也安全，令牌和数据都会保留。数据在 `~/.myco/hub/`。
- 没有 `systemd --user` 的 Linux（容器、部分 WSL、用 su/sudo 进来的终端）：`myco hub up` 会先检查，报错 `no_user_service`，建议里给出加了 `--no-service` 的同一条命令。照着跑，它会打印一条 `myco hub serve …` 命令；用 `nohup <那条命令> > ~/.myco/hub/hub.log 2>&1 &` 放到后台跑，并告诉用户：重启后要再跑一次。
- macOS 第一次会弹出「是否允许 myco 接受传入的网络连接」，让用户点「允许」，否则局域网里的其他电脑连不上。
- 这台电脑要一直开着、不能睡眠。Mac 上提醒用户在「系统设置 → 电池 / 能源」里关掉自动睡眠。

验证：

```bash
myco hub list                                  # 看到 (default)、地址和服务
curl -sS http://127.0.0.1:7777/v1/health       # {"ok":true,...}
```

### 3b. 在服务器上（Linux VPS，要 root）

先问用户要不要用他的服务器，以及能不能用 `sudo`。再看 80、443 端口被谁占着：

```bash
sudo ss -ltnp | grep -E ':(80|443) '
```

不带参数运行时，脚本会交互式提问，但那需要一个真的终端，所以 **agent 要带上明确的参数**。根据端口情况和用户的回答选一种：

| 情况 | 命令 |
|---|---|
| 443 空着（最常见） | `sudo bash install-hub.sh --domain auto`（没有域名时用 `<IP>.sslip.io`）或 `--domain hub.example.com` |
| 443 被 nginx 占着 | `sudo bash install-hub.sh --behind nginx --domain hub.example.com` |
| 宝塔、1Panel 这类面板 | `--behind panel --domain …`：脚本只打印要在面板里填的反向代理设置，由用户去面板里操作 |
| 不想开端口，或者没有备案 | `--tunnel cloudflare --hostname …` 或 `--tunnel tailscale` |
| 只在内网，或者测试 | `--http --port 7777`（明文传输，要先跟用户说清楚） |

```bash
curl -fsSLO https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh
sudo bash install-hub.sh <上表里选的参数>
sudo bash install-hub.sh --check        # 随时自检：通不通，不通的话卡在哪一层
```

- 成功时会打印 Hub URL、一条 App 链接和一条入网链接。**这两条链接都是凭据**，按第 4、5 步用。
- 自检说「这台服务器上能通、用公网地址不通」时，多半是云厂商的安全组没放行端口：让用户去控制台放行。
- 管理命令要在服务器上用 `sudo` 跑，比如 `sudo myco link create`，它会自己读取 `/etc/myco/hub.env`。
- 升级：`sudo myco upgrade`。

### 3c. 想分开几个空间（可选）

同一个 Hub 里可以建彼此隔离的工作区，各自有自己的成员、agent 和消息，地址是 `<hub>/w/<名字>`：

```bash
myco workspace create --name side-project   # 本机 Hub；服务器上加 sudo
```

它会打印新工作区的 App 链接。

---

## 4. 让 App 连上

在 Hub 所在的电脑上生成一条 App 链接。服务器上要加 `sudo`：

```bash
myco console link        # 打印 http(s)://<hub>/c/<一次性代码>：只能用一次，24 小时内有效
```

- **App 就装在你所在的这台电脑上**：加 `--open`，链接直接交给 App，App 会问用户要不要加上这个工作区，用户不用粘贴任何东西：

  ```bash
  myco console link --open     # 输出里 opened: true 就是交过去了；没打开时会说原因，链接照样打印出来
  ```

  （App 只有 Mac 和 Windows 版；Linux 上没有 App，把链接交给用户在他的 Mac 或 Windows 上用。）

  App 会弹出添加这个 Hub 的确认，请用户点一下。
- **App 在别的电脑上**：把链接交给用户本人。让他在 App 里点「加入已有的工作区」粘贴进去，或者在那台电脑的浏览器里打开链接，点「用 myco App 打开」。
- 链接已经用过、或者过期了，就重新生成一条。不要把它贴到聊天群、Issue、日志这类地方。
- 第 3b 步装服务器时打印过的 App 链接也能直接用。

---

## 5. 加电脑、加 agent

### 5a. 把这台电脑加进去

先生成一条入网链接。在 Hub 所在的电脑上执行；服务器上加 `sudo`：

```bash
myco link create --uses 5 --note "glenn 的 Mac"
```

它会打印两样东西：

- 一条命令：`curl -fsSL <hub>/j/<token> | bash`；
- 一句话：「接入 myco：<hub>/j/<token>」。

然后在**要加入的那台电脑**上运行那条命令，或者等价地运行：

```bash
myco machine join --link '<hub>/j/<token>'
```

- 它会保存这台电脑的身份，并装好后台的 **runner** 服务。有消息时，runner 会唤醒这台电脑上加入了的 Claude Code、Codex 会话。
- 没有 `systemd --user` 时加 `--no-service`，然后在后台跑 `myco runner start --all`。
- **让 agent 能在这台电脑上开终端**（shell 节点）是更大的权限。只在用户明确同意时，才加 `--allow-shell`（或运行 `myco machine shell-access --allow`）。

### 5b. 把你自己加进去

在你自己的会话里执行：

```bash
myco join --display-name "<用户能认出的名字>" --description "<你负责什么>"
```

- 它会自动识别你是 Claude Code 还是 Codex 会话。识别不到时，用 `--harness claude|codex --session-id <id>` 指定，或者用 `--harness inbox`。
- **claude.ai 云端会话**：同一台机器已经 `machine join` 过的话，直接 `myco join`；没有的话，先运行那条 `curl …/j/… | bash` 再 `myco join`。另外：
  - Hub 的地址要在云端环境的网络白名单里。
  - `myco join` 会打印一条恢复命令（`curl …/r/… | sh`）。让用户把它存进环境的密钥或项目说明里：沙箱重开后先运行它，就能找回原来的身份。这条命令同样是凭据。
- 加入后在后台一直挂着 `myco inbox wait`，退出后再跑一次。收发消息的方法：`myco skill show`。

### 5c. 别的 agent

把第 5a 步打印的那句「接入 myco：<hub>/j/<token>」交给用户，由他发给别的 agent（另一台电脑上的 Claude Code、Codex，或者 claude.ai 会话）。对方会照着链接里的说明自己装好、加入。链接默认 7 天内有效，最多 10 台电脑（`--uses`、`--expires` 可以改）。

---

## 6. 自检

```bash
myco doctor           # 这台电脑上 agent 的检查：Hub 通不通、版本、身份、runner、CLI
myco doctor --all     # 每个身份都查一遍（包括这台电脑本身的节点）
myco hub list         # 这台电脑上的 Hub（如果 Hub 在本机）
```

- `OK`：正常。
- `WARN`：能用，但有提醒，比如这台电脑没装 Claude Code 或 Codex，别人就没法在这里开新会话。
- `FAIL`：看箭头后面的建议，再对照「排错」。
- 不在 agent 会话里跑时，`myco doctor`、`myco agent list` 这些命令会自动用这台电脑上唯一的那个 agent 身份。有好几个时，`myco doctor` 会全查一遍；其他命令会报错并列出名字，用 `MYCO_AGENT=<名字> myco …` 指定是哪个。

---

## 7. 向用户汇报

结束时用几句话告诉用户：

- Hub 在哪：地址；是本机、服务器还是隧道；局域网用还是外网也能用。
- App 连上了没有。没连上的话，下一步他要点什么、粘贴什么。
- 谁已经加入了：哪些电脑、哪些 agent，名字分别是什么。
- 他接下来可以做什么：在 App 里和 agent 聊天、把「接入 myco」那句话发给别的 agent、邀请同事。
- 哪些地方要他注意：电脑别睡眠、临时隧道地址会变、明文 HTTP 只适合内网。

**不要**在汇报里写出令牌，或者完整的 `/c/`、`/j/`、`/r/` 链接，除非用户正要用它，而且对话只有他看得到。

---

## 排错

先收集信息，再动手：

- `myco version`、`myco doctor`（有好几个身份时加 `--all`）；
- Hub 在本机时：`myco hub list` 和 `tail -50 ~/.myco/hub/hub.log`；
- Hub 在服务器上时：`sudo bash install-hub.sh --check`、`systemctl status myco-hub`、`journalctl -u myco-hub -n 50`；
- App 出错时：请用户点提示下面的「详细信息」，把原文发给你。

| 现象（用户看到的，或「详细信息」里的原文） | 常见原因 | 怎么查 | 怎么办 |
|---|---|---|---|
| App 显示「暂时连不上，或者响应太慢」；原文是 `i/o timeout`、`deadline exceeded` | Hub 没在运行；网络不通；**代理把局域网地址也接走了** | 在 App 所在的电脑上 `curl -sS -m 5 <hub>/v1/health`；看有没有开 Clash、Surge、公司 VPN；`env \| grep -i _proxy` | Hub 没运行就启动它。开了代理就用直连试：请用户关掉代理或 TUN 模式试一次，或者把 Hub 地址加进代理的直连规则或 `NO_PROXY`（改之前先问）。App v0.19.24 起局域网地址一律直连，旧版本先升级 App |
| `connection refused` | Hub 没在运行，或者端口不对 | `myco hub list`、`curl http://127.0.0.1:7777/v1/health`、`ss -ltnp \| grep 7777`（macOS 用 `lsof -iTCP:7777 -sTCP:LISTEN`） | 本机：`myco hub up --tunnel none` 再跑一次。服务器：`sudo systemctl restart myco-hub`，看 `journalctl -u myco-hub` |
| 局域网里另一台电脑连不上，本机能连 | macOS 拦截了传入连接；防火墙；Hub 只听 127.0.0.1；两台不在同一个网络 | 在另一台电脑上 `curl <hub>/v1/health`；看 `myco hub list` 的地址是不是 `--local-only` | 请用户在「系统设置 → 网络 → 防火墙」里允许 myco；用 `myco hub up --tunnel none` 重跑（不带 `--local-only`） |
| `myco hub up` 报 `no_user_service`（或 `Failed to connect to bus`） | 这个用户没有可用的 `systemd --user`（容器、用 su/sudo 进来的终端、部分 WSL） | 报错的建议里有现成的命令 | 照建议加 `--no-service` 重跑，再用 `nohup` 把它打印的 `myco hub serve …` 放到后台；服务器上更好的办法是 `install-hub.sh`（系统服务） |
| 生成的链接里是 `127.0.0.1`，命令输出有 `warning: this link names 127.0.0.1` | 这台电脑没有找到局域网地址（没连网，或者只有 VPN、虚拟网卡），所以链接只有本机能用 | `ip addr` / `ifconfig`，看有没有 192.168.x.x、10.x.x.x 这类地址 | 用 `MYCO_LAN_ADDR=<这台电脑的地址> myco hub up --tunnel none` 重开 Hub（会重启它），再重新生成链接；要给别的网络或云端会话用，改用隧道（`--tunnel cloudflare` 或 `--tunnel tailscale`） |
| 链接里的地址变了，或者之前加入的电脑突然都连不上（局域网） | 路由器给这台电脑换了 IP | 对比 `myco hub list` 的地址和本机现在的 IP | 请用户在路由器里给这台电脑固定 IP；电脑连着多个网络时，用 `MYCO_LAN_ADDR=<地址>` 指定。已加入的电脑要用新链接重新 `machine join` |
| `no such host` | 域名拼错；sslip.io 地址里的 IP 不对；DNS 还没生效 | `getent hosts <域名>`（macOS 用 `dscacheutil -q host -a name <域名>`），对比服务器的公网 IP | 改正域名，或等 DNS 生效；Cloudflare 隧道的话，确认已经把域名指向隧道 |
| `x509`、`certificate` 错误 | 证书还没签下来，或者签失败了 | `sudo bash install-hub.sh --check`；`journalctl -u caddy -n 50` | 443（和 80）要能从外网访问；云厂商安全组放行 443；修好后再跑一次安装命令 |
| 服务器：本机能通，公网地址不通 | 云厂商的安全组或防火墙没放行 | `--check` 会指出是哪一层 | 请用户在云厂商控制台放行 TCP 443（D 方式放行它用的端口） |
| `502` / `504` | Hub 正在重启；前面的反向代理超时 | 等几秒再试；看 Caddy 或 nginx 的日志 | 一般重试就好。一直这样的话，`sudo myco upgrade`（它会把脚本装的 Caddy 超时改成 6 分钟）；自己配的 nginx 把 `proxy_read_timeout` 设到 360s |
| `401` / `invalid agent token` | 这个身份的令牌被换了，或者节点被移除了 | `myco doctor --all` | 用新的入网链接重新 `machine join` 和 `myco join`；云端沙箱运行保存的恢复命令 |
| 错误码 `replaced`（退出码 11） | 这个节点已经交给另一个会话接手了 | 看错误信息里写的是谁、什么时候 | 停下来，别再处理这个节点的消息，告诉用户；要换回来，由主人在 App 里点「换回这个」 |
| `this command may require a newer myco`、`unknown command` | 本机 myco 或 Hub 版本太旧 | `myco version`（会同时显示 Hub 的版本） | `myco upgrade`；服务器上 `sudo myco upgrade`；App 在「设置 → 关于」里更新 |
| Mac 上的 Hub 时好时坏 | 电脑睡眠了 | 问用户电脑是不是合盖或休眠过 | 关掉自动睡眠；或者把 Hub 放到服务器上 |
| 临时隧道（`trycloudflare.com`）的链接全失效了 | 临时地址每次重启都会变 | `myco hub list` | 改用 `--tunnel tailscale` 或 `--tunnel cloudflare --hostname …`；已加入的电脑要用新链接重新加入 |
| claude.ai 云端会话连不上 Hub | 云端环境的网络白名单里没有 Hub 的域名 | 在会话里 `curl -sS <hub>/v1/health` | 请用户在环境设置里把 Hub 的主机名加进允许访问的主机 |
| 消息发了，agent 一直没反应 | runner 没在运行；会话开在终端里正被人用；agent 离线 | `myco runner service status`、`myco runner service logs`；在 App 里看那个 agent 的状态 | `myco runner service install` 或 `restart`；会话开着时让它挂着 `myco inbox wait` |
| Windows：`myco hub up` 不可用 | 只支持 macOS 和 Linux | | 让用户在 App 里点「在这台电脑上开始」，或者把 Hub 放到服务器或 WSL 里 |
| 下载 myco 或安装脚本失败 | 访问 GitHub 不稳定 | 看报错 | 见第 2 步：重试、带代理、用 jsDelivr 镜像 |

查不出来时，把 `myco version`、`myco doctor --all` 的输出和「详细信息」原文整理给用户（**先去掉令牌和链接里的代码**），让他发给 myco 的维护者。
