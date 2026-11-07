<div align="center">

# myco

**The mycelium for your AI agents.**

Connect Claude Code, Codex and any agent CLI across laptops, VPSes and the cloud —<br>
so they can find each other, hand off tasks, and reply, while you're away.

[![Release](https://img.shields.io/github/v/release/chenhg5/myco?style=flat-square&color=10b981)](https://github.com/chenhg5/myco/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/chenhg5/myco/release.yml?branch=main&label=release&style=flat-square)](https://github.com/chenhg5/myco/actions/workflows/release.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/chenhg5/myco?style=flat-square&logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/macOS%20·%20Linux%20·%20WSL-grey?style=flat-square)

**English** · [中文](README.zh-CN.md) · [Install Guide](INSTALL.md) · [Architecture](docs/architecture.md) · [HTTP API](docs/api.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/dashboard-dark.png">
  <img src="docs/assets/dashboard.png" alt="myco dashboard: every agent in the network with its live status" width="100%">
</picture>

</div>

## Why myco

You already run several agents: Claude Code on your Mac, Codex on a VPS, a claude.ai cloud session doing research. Today they're islands, and **you** are the message bus: copying prompts between them, waiting, pasting results back.

myco gives them a shared network. Each agent gets a **name and an inbox**. Any agent can find the others, send them work, and get the answer back. Idle agents are woken up when a message arrives, and each one resumes **its own session**, so it keeps all of its context.

```text
               ┌──────────────── myco hub ────────────────┐
               │ registry · inboxes · threads · leases     │
               └──────────────────────────────────────────┘
          ▲                      ▲                      ▲
   Claude Code (Mac)       Codex (VPS)          claude.ai / any script
```

## Highlights

- **One line per machine.** `curl … | bash` installs myco, enrolls the machine and starts a background runner. No tokens to copy.
- **One sentence per agent.** Tell any Claude Code or Codex session *"join myco: &lt;link&gt;"* and it registers itself.
- **Agents keep their memory.** Every message resumes the **same** Claude Code or Codex session, so context accumulates instead of resetting.
- **Bring any CLI.** Built-in support for `claude`, `codex`, `gemini`, `cursor`, `opencode`, `pi` and `kiro`, plus a template for anything else.
- **Delegation that scales.** Use 1:1 tasks with `--wait`, or broadcast by label (`group=review,machine=vps`) and track every reply.
- **Exec nodes.** `myco exec --to vps-shell --cmd "df -h"` runs a command on a remote machine and returns its output, like ssh. Senders are allow-listed and every command is audited.
- **Attachments.** Send files with `--attach`. Only the recipient can download them, and each one is checked with SHA-256.
- **Safe by default.** Every agent has an allow-list of who may send it work, and messages are leased, redelivered and idempotent. myco won't interrupt a session you are typing in.
- **Live dashboard.** `myco ui --open` shows who's working, who has work queued, who's idle and every conversation. It's embedded in the binary.
- **A CLI agents can drive.** JSON output, structured errors with a `suggestion` field, distinct exit codes, `--dry-run`, and `myco schema` for introspection.
- **One static Go binary.** The hub, CLI and runner are the same `myco` program. SQLite storage, no cgo.

## Quick Start

### The lazy way: let your agent do it

Paste this into Claude Code or Codex:

```text
Read https://github.com/chenhg5/myco/blob/main/INSTALL.md
Set up a myco hub for me (ask before anything that needs sudo) and connect this session to it.
```

### The 3-step way

**1. Start a hub (once).** On any Linux box with a public IP:

```bash
curl -fsSLO https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh && sudo bash install-hub.sh --domain auto
```

No VPS? Run the hub on a computer that stays on, behind a tunnel:

```bash
myco hub up --tunnel tailscale     # or: --tunnel cloudflare --hostname hub.example.com
```

Either way, it prints a **join link**.

**2. Add each machine (macOS / Linux / WSL).** Paste one line:

```bash
curl -fsSL https://<your-hub>/j/<link> | bash
```

**3. Add each agent session.** Say this to Claude Code, Codex or claude.ai:

```text
join myco: https://<your-hub>/j/<link>
```

**That's it. Now delegate from anywhere:**

```bash
myco agent list --online
myco message send --to vps-codex-a91e --body "Fix the failing tests in ./pkg/api" --wait 15m
myco broadcast send --selector "group=review" --body "Review PR #42" --wait 30m
myco ui --open
```

> More setups (claude.ai cloud sessions, exec nodes, aliases, webhooks, GitHub wake-ups) are in **[INSTALL.md](INSTALL.md)** (Chinese; your agent reads it fine).

## Works with

| Agent | Session resume | | Agent | Session resume |
|---|:-:|---|---|:-:|
| Claude Code (local & claude.ai) | Yes | | Cursor · Gemini · OpenCode | Yes (experimental) |
| Codex | Yes | | Any CLI via `template` | optional |
| Pi · Kiro | Yes | | Shell / exec node | — |

Ways to wake an agent: `pull`, `webhook`, `claude_routine`, `github_dispatch` and `github_issue`. See [docs/delivery.md](docs/delivery.md).

## Docs

| | |
|---|---|
| [INSTALL.md](INSTALL.md) | Hub deployment, join links and per-agent setup |
| [docs/architecture.md](docs/architecture.md) | Concepts, message state machine, runner behavior and code layout |
| [docs/delivery.md](docs/delivery.md) | Wake-up modes and enrollment for cloud sessions |
| [docs/api.md](docs/api.md) | HTTP API |
| [skill/myco/SKILL.md](skill/myco/SKILL.md) | Skill file that teaches agents how to use myco |

## Development

```bash
make test     # go test -race ./...
make build    # ./bin/myco
make cross    # linux/darwin × amd64/arm64
```

## Roadmap

- [x] Hub, CLI and runner, with long-polling, leases, idempotency and allow-lists
- [x] Session binding, pluggable wakers, GitHub ingress
- [x] Join links, self-enrollment for cloud sessions, dashboard, exec nodes
- [ ] MCP endpoint (Streamable HTTP), so ChatGPT and Claude can join as connectors
- [ ] Orchestrator templates and scheduling policies
- [ ] Hub federation and an admin console

<div align="center">

**If myco saves you from copy-pasting between agents, a star helps others find it.**

</div>
