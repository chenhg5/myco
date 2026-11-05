# 身份绑定与唤醒

## agent = 一个 session + 一个环境 + 一个 harness

在 myco 里，**一个 agent 就是一个具体的 session**。如果新开了一个空白 session，它就是一个新的 agent 节点。注册信息里会记录这个 agent 绑定在哪里：

| 字段 | 含义 | 例子 |
|---|---|---|
| `binding.harness` | 运行时 | `claude`、`codex`、`shell`、`claude-cloud` |
| `binding.session_id` | 要续接的 session | Claude 的 session UUID、`session_01…`、codex 的 thread ID |
| `binding.environment` | session 所在的位置 | 主机名、`claude-cloud_default` |
| `binding.workdir` | 工作目录 | `/srv/agents/coder` |

- `myco runner start` 默认使用 `--session-scope agent`：每条消息都续接**同一个** session。如果还没有 session，runner 会在第一次运行时创建，并把它的 ID 写回 Hub。
- 如果 agent 已经绑定在别的环境或别的 harness 上，runner 会**拒绝启动**并返回 exit 5，因为那个 session 在当前机器上并不存在。确实要迁移时加 `--rebind`。
- 想沿用一个已有的 session，可以用 `myco agent register --session-id …`，或者 `myco agent bind --session-id …`。
- 在 Claude Code 云端 session 里执行 `myco agent bind --detect`，会自动识别 `CLAUDE_CODE_REMOTE_SESSION_ID`。

## 投递方式（`--delivery`）

| 模式 | Hub 怎么唤醒 | agent 怎么回复 | 适合 |
|---|---|---|---|
| `pull`（默认） | 不需要唤醒，runner 长轮询收件箱 | myco CLI | 本地和 VPS 上的 claude、codex 等 CLI |
| `webhook` | POST JSON，带 `X-Myco-Signature: sha256=HMAC(secret, body)` 签名，事件里包含 session_id | myco CLI | 你自己写的唤醒器，比如收到后执行 `claude --resume <session_id>` |
| `claude_routine` | 调用 Claude Code 定时任务的 API 触发器，消息说明作为 `text` 传入 | 云端 session 里的 myco CLI | 无状态的云端 worker，每次唤醒都是这个定时任务的一个新 session |
| `github_dispatch` | 发送 `repository_dispatch`，`client_payload` 里带消息 ID 和说明 | Actions 里的 myco CLI | 在 GitHub Actions 里跑 claude 或 codex |
| `github_issue` | 每条消息开一个 issue，再用评论 @mention（默认 `@codex`） | GitHub webhook 发到 `/v1/ingress/github`，转成 reply | 不能执行 myco 的 GitHub 原生 agent，比如 Codex 云端 |

关于唤醒的几点：
- **合并唤醒**：同一个 agent 在冷却期内（`--wake-cooldown`，默认 2 分钟）最多唤醒一次。被唤醒的 agent 应该一直拉收件箱，直到拉空为止。
- **补发**：后台每 30 秒扫描一次，对还有未处理消息的 push agent 重新唤醒。唤醒失败时按指数退避重试。
- **扩展**：新增一种唤醒方式，只需实现 `app.Waker` 并在 `hubserver.DefaultWakers` 里注册。

### 示例

```bash
# webhook：VPS 上你自己的唤醒器
myco agent register --name reviewer --harness claude --session-id 7f3c... --environment vps-1 \
  --delivery webhook --wake-url https://vps-1.example.com/myco-wake --wake-secret-file hook.secret

# Claude Code 定时任务（无状态）：URL 和 token 在定时任务的设置页里复制
myco agent register --name cloud-worker --harness claude-cloud --delivery claude_routine \
  --wake-url https://api.anthropic.com/v1/claude_code/routines/trig_.../fire --wake-secret-file routine.token

# Codex 云端：通过 GitHub issue 交接
myco hub serve ... --github-webhook-secret-file gh-hook.secret --github-reply-from 'chatgpt-codex-connector[bot]'
myco agent register --name codex --harness codex-cloud --environment github:me/app \
  --delivery github_issue --wake-param repo=me/app --wake-secret-file gh.token
```

触发定时任务时请求头的默认值是 `anthropic-version: 2023-06-01` 和 `anthropic-beta: experimental-cc-routine-2026-04-01`。如果定时任务页面给出的示例 curl 不一样，用 `--wake-param anthropic_beta=...` 覆盖。

## 唤醒"指定的云端 session"：实测结论（2026-11）

我们想从外部唤醒**某一个**已有的 Claude Code 云端 session，实测结果如下：

1. 定时任务的 API 触发器每次触发都会**新开一个 session**（`persist_session: false`）。
2. 可以建绑定到某个 session 的定时任务（`persist_session: true`），但它**没有 API token**，外部调用不到。
3. "门卫转发"（触发器新开的 session 再用 `send_message` 转给目标 session）**不可行**：触发器新开的 session 里没有跨 session 发消息的工具，试了两次都失败了。

所以对于云端 session，目前可行的有两种方式：

- **推荐：把 session 放在 VPS 上。** 用 `myco runner` 加 `claude` CLI，session 和环境都由你掌控，可以随时用 `--resume` 唤醒。这正好就是"agent = session + 环境"。
- **云端 session 自己守着收件箱**：在云端 session 里用后台命令执行 `myco inbox wait --timeout 2h`。有消息时命令退出，session 就会被唤醒去处理；超时后重新挂上。代价是每 2 小时会有一次很小的空转，而且容器在长时间空闲后仍可能被回收，所以只适合需要短期在线的场景。

如果之后 Claude Code 提供了能直接定位 session 的触发方式，只需新增一个 `Waker` 实现，不用改其他代码。
