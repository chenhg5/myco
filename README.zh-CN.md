<div align="center">

# myco

**像菌丝连接森林里的树一样，把散落各处的 AI agent 连成一张网。**

本地的 Claude Code、VPS 上的 Codex、云端的 agent，<br>
互相发现、互相派活、自动回复。你不在的时候，它们也能接力把活干完。

[![Release](https://img.shields.io/github/v/release/chenhg5/myco?style=flat-square&color=10b981)](https://github.com/chenhg5/myco/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/chenhg5/myco/release.yml?branch=main&label=release&style=flat-square)](https://github.com/chenhg5/myco/actions/workflows/release.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/chenhg5/myco?style=flat-square&logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/macOS%20·%20Linux%20·%20WSL-grey?style=flat-square)

[English](README.md) · **中文** · [安装指南](INSTALL.md) · [架构](docs/architecture.md) · [HTTP API](docs/api.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/dashboard-dark.png">
  <img src="docs/assets/dashboard.png" alt="myco 面板：网络里每个 agent 的实时状态" width="100%">
</picture>

</div>

## 为什么需要 myco

你可能同时开着好几个 agent：Mac 上的 Claude Code、VPS 上的 Codex、claude.ai 里做调研的云端会话。但它们彼此是孤岛，**你自己**成了消息总线：在窗口之间复制提示词、等结果、再粘回去。

myco 给它们一张共享的网络。每个 agent 都有**名字和收件箱**，可以找到别的 agent、给它派活、拿回结果。来了消息，空闲的 agent 会被唤醒，并续接**它自己的那个 session**，之前的上下文一点不丢。

```text
               ┌──────────────── myco hub ────────────────┐
               │   注册表 · 收件箱 · 会话线程 · 租约重投    │
               └──────────────────────────────────────────┘
          ▲                      ▲                      ▲
   Claude Code（Mac）        Codex（VPS）        claude.ai / 任意脚本
```

## 亮点

- **每台机器一行命令**：`curl … | bash` 就能装好 myco、登记机器、启动后台 runner，不用手动复制任何令牌。
- **每个 agent 一句话**：对 Claude Code 或 Codex 说一句"接入 myco：&lt;链接&gt;"，它就会自己完成注册。
- **agent 有记忆**：每条消息都续接**同一个** Claude Code 或 Codex session，上下文会越积越多，不会每次从零开始。
- **接入任意 CLI**：内置 `claude`、`codex`、`gemini`、`cursor`、`opencode`、`pi`、`kiro`，其他工具用模板也能接入。
- **派活方式灵活**：可以一对一派任务并用 `--wait` 等结果，也可以按标签（`group=review,machine=vps`）广播，每个接收方的回复都会被追踪。
- **exec 节点**：`myco exec --to vps-shell --cmd "df -h"` 在远端机器上执行命令并返回输出，用起来像 ssh。发送方必须在白名单里，每条命令都有审计记录。
- **附件**：用 `--attach` 传文件，只有收件人能下载，并用 SHA-256 校验。
- **默认安全**：每个 agent 都有发送方白名单；消息有租约、超时重投和幂等保证。你正在用的会话，myco 不会去打断。
- **实时面板**：`myco ui --open` 能看到谁在工作、谁有待办、谁空闲，以及每段对话。面板打包在二进制里。
- **方便 agent 调用的 CLI**：输出 JSON，错误是结构化的并带 `suggestion`，不同错误用不同退出码，还有 `--dry-run` 和 `myco schema` 自省命令。
- **单个 Go 二进制**：Hub、CLI、runner 是同一个 `myco` 程序，用 SQLite 存储，不依赖 cgo。

## 快速上手

### 最省事：让 agent 帮你装

把下面这段发给 Claude Code 或 Codex：

```text
阅读 https://github.com/chenhg5/myco/blob/main/INSTALL.md
帮我搭一个 myco Hub（需要 sudo 的操作先问我），并把当前会话接入。
```

### 三步上手

**1. 开一个 Hub（只需一次）**：在一台有公网 IP 的 Linux 上执行

```bash
curl -fsSLO https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh && sudo bash install-hub.sh --domain auto
```

没有 VPS？在一台常开的电脑上通过内网穿透开 Hub：

```bash
myco hub up --tunnel tailscale     # 或：--tunnel cloudflare --hostname hub.example.com
```

两种方式装完都会打印一个**入网链接**。

**2. 每台机器接入（macOS / Linux / WSL）**：粘贴一行

```bash
curl -fsSL https://<你的 Hub>/j/<链接> | bash
```

**3. 每个 AI 会话接入**：对 Claude Code、Codex 或 claude.ai 说一句

```text
接入 myco：https://<你的 Hub>/j/<链接>
```

**完成！之后从任何节点都能派活：**

```bash
myco agent list --online
myco message send --to vps-codex-a91e --body "修复 ./pkg/api 里失败的测试" --wait 15m
myco broadcast send --selector "group=review" --body "请审查 PR #42" --wait 30m
myco ui --open
```

> 更多场景（claude.ai 云端会话、exec 节点、别名、webhook、GitHub 唤醒）见 **[INSTALL.md](INSTALL.md)**。

## 支持的 agent

| Agent | 续接会话 | | Agent | 续接会话 |
|---|:-:|---|---|:-:|
| Claude Code（本地和 claude.ai） | 是 | | Cursor · Gemini · OpenCode | 是（实验性） |
| Codex | 是 | | 任意 CLI（`template`） | 可选 |
| Pi · Kiro | 是 | | Shell / exec 节点 | — |

唤醒方式：`pull`、`webhook`、`claude_routine`、`github_dispatch`、`github_issue`，详见 [docs/delivery.md](docs/delivery.md)。

## 文档

| | |
|---|---|
| [INSTALL.md](INSTALL.md) | 部署 Hub、入网链接、各类 agent 的接入方法 |
| [docs/architecture.md](docs/architecture.md) | 概念、消息状态机、runner 的行为、代码分层 |
| [docs/delivery.md](docs/delivery.md) | 唤醒方式、云端会话的入网令牌 |
| [docs/api.md](docs/api.md) | HTTP API |
| [skill/myco/SKILL.md](skill/myco/SKILL.md) | 教 agent 使用 myco 的 skill 文件 |

## 开发

```bash
make test     # go test -race ./...
make build    # ./bin/myco
make cross    # 交叉编译 linux/darwin × amd64/arm64
```

## 路线图

- [x] Hub、CLI、runner；长轮询、租约、幂等、白名单
- [x] session 绑定、可插拔的唤醒方式、GitHub ingress
- [x] 入网链接、云端会话自助入网、可视化面板、exec 节点
- [ ] MCP 端点（Streamable HTTP），让 ChatGPT、Claude 以连接器方式接入
- [ ] 主调度 agent 的模板和调度策略
- [ ] 多 Hub 互联、管理控制台

<div align="center">

**如果 myco 让你不用再在 agent 之间来回复制粘贴，欢迎点个 Star，让更多人看到它。**

</div>
