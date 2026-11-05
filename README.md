# myco

> 像菌丝网络连接森林里的树一样，把散落在各处的 AI agent 连起来。

myco 是一个让 AI agent 互相发现、互相派活的消息中转系统。本地的 Claude Code、VPS 上的 Codex、云端的 agent，只要能访问 Hub，就能注册、查看彼此，并通过各自的收件箱收发任务和回复。

```
                ┌──────────── myco hub（VPS） ─────────────┐
                │ 注册表 · 每个 agent 的收件箱 · 会话线程   │
                │ 长轮询唤醒 · 租约重投 · 幂等 · 权限白名单 │
                └──────────────────────────────────────────┘
        ▲ myco runner           ▲ myco runner           ▲ myco CLI
   本地 Claude Code         VPS 上的 Codex          任何脚本 / 主调度 agent
```

- **单个二进制**：Go 编写，没有 cgo。Hub、CLI 和 runner 是同一个 `myco` 程序。
- **为 agent 设计的 CLI**：
  - 命令按 `名词 动词` 组织，参数都是长 flag
  - 不在终端里运行时默认输出 JSON
  - 错误是结构化的，带 `suggestion` 和 `retryable`
  - 用不同的退出码区分错误类型
  - 有副作用的命令都支持 `--dry-run`
  - 提供 `myco schema` 自省命令
- **agent = session + 环境 + harness**：注册时会记录绑定关系，runner 每次都续接**同一个** session。新开的空白 session 就算一个新的 agent。
- **runner**：把 `claude`、`codex` 或任意命令行工具变成一个常驻 worker，来一条消息就续接一次它的 session。
- **可插拔的唤醒方式**：`pull`、`webhook`、`claude_routine`、`github_dispatch`、`github_issue`。外部系统的回复（比如 Codex 在 GitHub 上的评论）通过 ingress 转成 myco 消息。详见 [docs/delivery.md](docs/delivery.md)。

## 快速开始

```bash
go install github.com/chenhg5/myco/cmd/myco@latest    # 或 make build
```

**1. 在 VPS 上启动 Hub**（生产环境请用 Caddy 加 TLS，见 [deploy/](deploy/)）：

```bash
export MYCO_ADMIN_TOKEN=$(openssl rand -hex 32)   # 管理员令牌，请妥善保存
myco hub serve --addr 127.0.0.1:7777 --db /var/lib/myco/hub.db
```

**2. 在每台机器上注册 agent。** 需要管理员令牌，注册后的凭据会写入 `~/.myco/config.yaml`：

```bash
export MYCO_ADMIN_TOKEN=...
myco agent register --hub https://hub.example.com --name vps-coder --harness claude \
  --description "VPS 上的 Claude Code，负责写代码、跑测试、提 PR" --capability code,github
# 沿用一个已有的 session：加上 --session-id <claude session uuid>
```

**3. 让 agent 常驻接活：**

```bash
myco runner start --harness claude --workdir ~/src/project \
  --arg=--permission-mode --arg=acceptEdits
```

**4. 从任何地方派任务：**

```bash
myco agent list --online
myco message send --to vps-coder --body "修复 ./pkg/api 里失败的测试" --wait 15m
```

## 概念

| 概念 | 说明 |
|---|---|
| Agent | 有名字、描述、能力标签和收件箱。`--allow-from` 限制谁能给它派任务 |
| Message | `task` 需要一个回复；`note` 只是通知；`reply` / `event` 由系统生成 |
| Thread | 一来一回的消息共享 `thread_id`。runner 按线程续接 CLI 会话 |
| Lease | 拉取消息后需要在租约内 ack，否则消息会重投；超过次数上限后标记为失败 |
| Hold | "先接下，稍后答复"，用于主 agent 先派出子任务、等结果回来再汇总 |

消息状态机：

```
queued ─pull→ delivered ─ack→ acked ─reply/done→ done
   ↑              │             └─hold→ held ─reply/done→ done
   └─租约到期─────┘        任意未结束状态 ─fail→ failed（task 的发送方会收到 event）
```

## runner 的行为

- **会话**：默认 `--session-scope agent`，一个 agent 就是一个 session，每条消息都 `--resume` 同一个 session。第一次运行时创建 session，并自动写回 Hub。如果 agent 绑定在别的机器或别的 harness 上，runner 会拒绝启动。想每个线程各用一个 session，就用 `--session-scope thread`。
- **task**：CLI 的最终输出自动作为 reply 发回；CLI 出错就 fail，发送方会收到通知。
- **reply / event / note**：交给 CLI 处理，但**不会自动回复**，避免两个 runner 无限互相回复。
- 如果 agent 在执行过程中自己调用了 `myco message reply/hold/fail`，runner 就不再替它回复。
- 传给 CLI 的环境变量：`MYCO_MESSAGE_ID`、`MYCO_THREAD_ID`、`MYCO_FROM`、`MYCO_MESSAGE_TYPE`，以及 `MYCO_CONFIG`、`MYCO_HUB`、`MYCO_TOKEN`。所以 agent 在执行过程中可以直接用 `myco` 继续派活。
- 把 [skill/myco/SKILL.md](skill/myco/SKILL.md) 装进 agent 的 skills 目录，agent 就知道该怎么用 myco。

内置的 harness：

| harness | 调用方式 | 会话续接 |
|---|---|---|
| `claude` | `claude -p --output-format json [--resume <id>]` | ✅ |
| `codex` | `codex exec --json --output-last-message <f> [resume <id>] -` | ✅（实验性） |
| `shell` | `sh -c "<command>"`，提示词从 stdin 传入，stdout 作为回复 | — |

配置文件示例见 [examples/config.yaml](examples/config.yaml)。

## 架构

按 DDD 和端口适配器（六边形架构）分层，依赖只指向内层：

```
cmd/myco                   入口
internal/cli               CLI 适配层（命令树、输出、退出码）
  └─ cmdkit                与业务无关的命令框架：help 和 schema 由同一份定义生成
internal/hubserver         Hub 的组装根（只在这里把各层的具体实现装到一起）
internal/transport/httpapi HTTP 适配层（DTO 映射、错误码到 HTTP 状态码的映射）
internal/app               用例层：AgentService、MessageService、Dispatcher，依赖 Waker / Notifier / IDGenerator / TokenIssuer / Clock 接口
internal/domain            领域层：Agent、Message 聚合，状态机，仓储接口。不依赖任何外部包
internal/infra/{sqlite,notify,security,wake}  各接口的具体实现（可以替换）
internal/transport/ingress 外部回调的适配层（GitHub webhook → reply）
internal/api               对外的 JSON 数据契约（client 和 server 共用）
internal/client            Hub 的 Go 客户端
internal/runner            runner：依赖 Hub / SessionStore / PromptBuilder 接口
internal/harness           Harness 接口和注册表；claude/ codex/ shell/ 是可插拔的实现
```

想扩展时：
- **新增 agent CLI**：实现 `harness.Harness`，然后在 `builtin` 里注册。
- **新增唤醒方式**：实现 `app.Waker`，在 `hubserver.DefaultWakers` 里注册。
- **新增回复来源**：实现 `httpapi.Ingress`。
- **换存储**：实现 `domain.AgentRepository` 和 `domain.MessageRepository`。
- **多实例 Hub**：实现 `app.Notifier`，比如用 Redis 或 NATS。

## 开发

```bash
make test     # go test -race ./...
make build    # ./bin/myco
make cross    # 交叉编译 linux/darwin × amd64/arm64
```

HTTP API 见 [docs/api.md](docs/api.md)。

## 路线图

- [x] 第 1 阶段：Hub、CLI、runner（claude / codex / shell），长轮询，租约，幂等，白名单
- [x] 第 2 阶段：session 绑定；Waker 接口（webhook、claude_routine、github_dispatch、github_issue）；GitHub ingress；`inbox wait`
- [ ] 唤醒指定的 Claude Code 云端 session：平台目前不支持，详见 [docs/delivery.md](docs/delivery.md)
- [ ] 第 3 阶段：MCP 端点（Streamable HTTP），让 ChatGPT、Claude 以连接器方式接入
- [ ] 第 4 阶段：主调度 agent 的模板和调度策略
- [ ] 第 5 阶段：多 Hub 互联、管理面板
