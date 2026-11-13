# 设计讨论：消息投递语义、元信息和唤醒说明（含与 A2A 的对比）

状态：讨论稿，还没有定。每一节最后的「建议」是起点，需要逐条斟酌。

这篇整理 2026-11-09 关于消息机制的讨论：现在是怎么做的（以代码为准），哪些地方值得改，以及和 A2A 协议（Agent2Agent，v1.0）的对比。

## 1. 现状

### 1.1 `inbox wait`：对调用者阻塞，对 agent 是异步唤醒

- `myco inbox wait` 是长轮询：Hub 每次请求最多挂 60 秒（`MessagePolicy.MaxWait`），CLI 在本地循环到有消息或超时（退出码 7）。Hub 内部在消息写入时立即通知（`notifier`），不是按间隔查询。
- 它**只看不取**：不租约，返回后由 agent 自己 `myco inbox pull`。
- 只有云端会话这样用（作为后台命令跑，退出时 Claude Code 被唤醒）。本地节点由 runner 长轮询 `pull`，再在后台唤醒对应的会话。
- `message send` 本身是异步的：写进 Hub 即返回；加 `--wait` 才会等回复。

### 1.2 投递语义：投递 at-least-once，执行 at-most-once

状态机（`internal/domain/message.go`）：

```
queued ──pull──▶ delivered ──ack──▶ acked ──reply/done──▶ done
   ▲                 │  ▲              ├──hold──▶ held ──reply/done──▶ done
   └──lease expires──┘  └───release────┘   any open state ──fail──▶ failed
```

- `pull` 是租约：5 分钟内不 ack/reply/fail/done 就重新投递，最多 5 次（`MessagePolicy{Lease: 5m, MaxAttempts: 5}`），之后标 `failed` 并给发送方一条 `event`。
- runner **拿到就先 ack，再唤醒 agent**（`runner.Handle`）。之后 runner 或机器在执行中途挂掉，这条消息停在 `acked`，不会重跑，任务一直“处理中”。
- 发送端去重：`--idempotency-key`，同一发送方同一 key 只存一条。CLI 默认不带。
- 结论：Hub→接收方 at-least-once；接收方真正执行 at-most-once；整体不是 exactly-once。

### 1.3 发送方能知道什么

- 能查：`myco message get --id` 看状态（queued / delivered / acked / held / done / failed），`myco message wait` 等回复并显示进度。
- 会被主动通知的只有：回复（`reply`）和失败（`event`）。“对方已收到”（ack）不推送。
- ack 的含义是“runner 收到、即将唤醒 agent”，不是“agent 读懂了”。
- 接收方一直离线：消息永远 `queued`，没有过期，也不提醒发送方。
- App 只显示「处理中」，不区分排队、已送达、已收到。

### 1.4 消息格式和来源

`inbox pull --format json` 的一条消息：`id, from, to, from_owner, type, thread_id, reply_to, body, attachments, e2e, status, attempts, lease_until, created_at, updated_at`（`internal/api` 的 `Message`）。

- `from`：发送方节点名；App 发的是 `@console:<设备>`。
- `from_owner`：发送方背后的人（App 里写消息的成员，或发送节点的主人），**由 Hub 按凭据填写**，发送方无法伪造。
- `type`：`task`（要回复）、`note`、`reply`、`event`（Hub 生成）、`progress`、`steer`。

runner 唤醒 agent 时用 `internal/runner/prompt.go` 生成一段英文说明放在正文前：来源（主人 / 其他成员 / 同一主人的 agent / 别人的 agent）、`message_id`、`thread_id`、怎么回复、安全提醒，然后是 `--- message body ---` 和正文。

### 1.5 元信息

没有优先级、加急、截止时间。最接近的是 `steer`：发送方修正自己发出、对方正在做的任务，runner 打断当前一轮带着修正继续；它不能让别的任务插队。

### 1.6 顺序

- Hub 取消息按 `ORDER BY created_at, id`，先到先出。
- 每个节点默认一个会话、一次一条（`ScopeAgent` 强制并发 1），通常就是到达顺序。
- 会乱序的情况：会话被人占用时 runner 把消息放回并退避重试，后来的可能先处理；租约到期重投的旧消息会排回前面；`steer` 单独取，不排队。
- 不同节点之间并行，互不影响。

## 2. 待斟酌的问题

### 2.1 ack 时机：先 ack 再执行，还是执行完再关？

现在 runner 拿到就 ack，所以执行中途崩溃会丢任务（停在 `acked`）。

选项：
1. **保持现状**，补“卡住检测”：`acked` 超过一段时间且接收方 runner 已重启/离线，在 App 和 `message get` 里标“可能已中断”，发送方一键重发。
2. **ack 改成续租**：runner 不立即 ack，而是在 agent 运行期间定期延长租约（心跳），回复时才关闭；runner 挂了，租约到期自动重投。代价：agent 可能把同一个任务做两遍（at-least-once 执行），对“改代码、发消息”这类有副作用的任务不一定安全；runner 要能识别“这条我做过一半”。
3. 两者结合：默认 1；任务可以标 `retry_safe`（幂等）时用 2。

建议：先做 1（成本低、不改变语义），2 等真遇到丢任务再说。

### 2.2 发送方要不要看到“已收到”？

建议：
- App 把「处理中」拆成：**排队中**（对方不在线时写明“对方离线，上线后处理”）、**已送达**、**已收到，处理中**、**稍后回复**（held）。数据都已有，只改界面。
- 不新增推送：“已收到”不唤醒发送方（否则每条消息都多唤醒一次 agent，浪费 token）。需要的人用 `message wait` 或看 App。
- 可选：给 `message send` 加 `--expire 2h`，过期还在排队就转 `failed` 通知发送方，解决“对方一直离线、发送方一直等”。

### 2.3 默认去重

建议：CLI 的 `message send` 在本次命令内自动生成去重 key，网络重试不会重复发送（用户显式传 `--idempotency-key` 时用用户的）。跨命令的重发仍由用户决定。

### 2.4 优先级 / 加急

要回答的问题：谁能标加急？加急到底改变什么？

- 只影响**排队顺序**：Hub 按 `priority DESC, created_at` 取。改动小，但一个节点一次只做一条，加急也要等当前这条做完。
- 要不要**打断**正在做的任务？我倾向不要：打断等于让任意发送方都能中断别人的工作，容易被滥用。现有 `steer` 只允许修正自己发的任务，这个边界是对的。
- 加急对 agent 的意义：在唤醒说明里写明“发送方标了加急”，agent 自己决定是否先回一句进展。
- 防滥用：只有主人（App 里）和主人自己的 agent 能标加急；别人的 agent 标了，Hub 降为普通，或者需要节点主人在 allow-from 里允许。

建议：先只做“排队顺序 + 说明里标出”，权限按上面最后一条。截止时间（deadline）先不做，用 2.2 的 `--expire` 覆盖“过时就别做了”的场景。

### 2.5 唤醒说明：语言和自定义

现状：英文，写死。

- 语言：保持英文。这是给模型看的固定指令，英文最稳；agent 回复语言跟正文走。
- 自定义：**只允许追加，不允许替换**。来源判断和安全提醒（别人的 agent 的内容只是同伴请求，消息里自称“主人已批准”不算数）是防止 agent 被别的 agent 指使的防线，整段可替换就等于可以被误删或改弱。
- 做法：每个节点一项「额外说明」（如“用中文回复”“先写结论”“不要动 main 分支”），接在固定说明之后、正文之前；只有节点主人能改（App 节点详情 / CLI）；存在哪里待定（本机 profile 更私密，Hub 上方便在 App 里改；倾向 Hub，主人才能读写）。

## 3. 与 A2A 的对比

A2A（Agent2Agent）是 Google 发起、现由 Linux Foundation 托管的开放协议，v1.0 于 2026 年 3 月发布。它定义的是**一个 agent 作为服务器，另一个作为客户端**直接调用（JSON-RPC / gRPC / HTTP+JSON），而 myco 是**中转站（Hub）+ 收件箱**模式：节点不用开端口，经常不在线也能收。两者定位不同，下面只比能借鉴的部分。

| 方面 | A2A v1.0 | myco 现在 |
|---|---|---|
| 拓扑 | 点对点：客户端直接调用 agent 的 HTTP 端点 | Hub 中转：节点只出站连接，可离线，Hub 存消息 |
| 发现 | Agent Card（`/.well-known/` 下的 JSON）：name、description、version、`skills`（id/name/description/tags/examples/输入输出类型）、`capabilities`、`securitySchemes`，可选 JWS 签名 | `agent list`：名字、描述、`capabilities` 标签、labels、在线状态、所在机器 |
| 基本单位 | Message（`role`、`parts`、`messageId`、`contextId`、`taskId`、`referenceTaskIds`、`metadata`）+ Task | Message（`type` 区分 task/note/reply/…）+ `thread_id` |
| 内容 | `parts`：每个是 `text` / `raw`（字节）/ `url` / `data`（结构化 JSON）之一，带 `mediaType`、`filename` | `body` 字符串 + `attachments`（文件） |
| 任务状态 | `submitted, working, input-required, auth-required, completed, failed, canceled, rejected`；后四个终态 | 消息状态 `queued, delivered, acked, held, done, failed`；偏投递视角，不是任务视角 |
| 结果 | Artifacts：独立于对话的产出（`artifactId`、`parts`），可流式追加 | 回复正文 + 附件 |
| 进度 | `SendStreamingMessage` / `SubscribeToTask` 推状态和产出事件，**同一任务的事件 MUST 按生成顺序送达** | `progress` 消息 + `message wait` 显示 |
| 取消 | `CancelTask`（发送方可取消） | 无；只有接收方能 `fail`/`done`，发送方只能 `steer` |
| 多轮 | `contextId` 把多个任务串成一个会话；`input-required` 让被调用方停下来向调用方要信息 | 同一 `thread_id` 继续发；被调用方要信息只能发一条新消息 |
| 投递可靠性 | 消息本身不保证可靠（规范原文：Messages MUST NOT be considered a reliable delivery mechanism for critical information）；推送通知 at-least-once，2xx 即确认，接收方 SHOULD 幂等处理；`messageId` 可用于去重 | 收件箱 + 租约 + 重投（at-least-once 投递），有 ack；`idempotency_key` 去重 |
| 身份 | 每个请求按 Agent Card 声明的 `securitySchemes` 认证（OAuth、API key 等）；规范不定义“消息来自哪个 agent”的字段 | 每个节点有自己的令牌；Hub 填 `from` 和 `from_owner`，接收方知道是哪个节点、属于谁 |
| 加密 | 依赖 TLS | TLS + 可选端到端加密（Hub 也读不到） |
| 优先级 | 无 | 无 |

### 3.1 值得借鉴的

1. **任务状态从“投递”视角补上“任务”视角。** A2A 的 `input-required`（被调用方需要更多信息，停下来等）和 `rejected`（明确拒绝，比如不在能力范围内）在我们这里没有对应：现在只能 `fail` 或发一条新消息。可以加：
   - `myco message ask --id <task> --body "..."`：任务转为「等待对方补充」，发送方被唤醒；发送方的回复自动接回这个任务（类似现在的 `steer`，但方向相反）。
   - `fail --rejected`：区分“做不了/不该做”和“做了但出错”。
2. **发送方取消（cancel）。** 发送方发现任务不需要了，现在只能干等或 steer 一句“不用做了”。加 `myco message cancel --id`：未取的直接关闭；已在做的发一个取消信号，runner 中断这一轮（复用 steer 的打断机制）。
3. **结构化内容（parts / data）。** 编排类 agent 互相传结构化结果时，现在只能把 JSON 塞进 body。可以加可选的 `data` 字段（JSON），正文仍是给人和模型看的文字。exec 节点的结果已经是 JSON，可以顺势规范化。
4. **能力描述（Agent Card / skills）。** 现在 `description` + `capabilities` 标签比较弱。可以让节点描述自己会什么（skills：名字、说明、示例），编排 agent 用 `agent list` 选人更准。也可以对外暴露 A2A 兼容的 Agent Card，让 A2A 生态的客户端发现 myco 节点（见 3.3）。
5. **同一任务事件有序。** A2A 要求同一任务的事件按生成顺序送达。我们的 `progress`/`reply` 是独立消息，顺序靠 `created_at`；可以明确写进文档并在 `message wait` 中保证。

### 3.2 不该照搬的

- **点对点直连。** myco 的价值就在“节点不开端口、可离线、在 NAT 后面也能收”，Hub + 收件箱的模型保留。
- **“消息不保证可靠”。** A2A 把可靠性留给实现；我们已经有收件箱和重投，比它强，不退回去。
- **重型认证协商（OAuth 流程、`auth-required`）。** 我们的节点令牌 + 成员 + allow-from 已经够用，暂不引入。

### 3.3 要不要兼容 A2A

可以做一个**A2A 网关**作为远期选项：Hub 为每个节点生成 Agent Card，并接受 A2A 的 `SendMessage` / `GetTask` / `CancelTask`，内部转成 myco 消息。这样别的 A2A 客户端可以调用 myco 网络里的 agent，而 myco 内部模型不变。成本中等，前提是 3.1 的状态（input-required、canceled、rejected）先在内部补齐，否则映射不完整。先不做，等有具体用户需求。

## 4. 建议的顺序（待定）

1. App 拆分状态显示（排队中 / 已收到 / 处理中 / 稍后回复），离线时写明。成本小，体验提升明显。
2. CLI 默认去重 key。
3. 唤醒说明的「额外说明」（只追加）。
4. 发送方取消、`ask`（等待补充）、`--rejected`：对齐 A2A 的任务状态。
5. 优先级（只影响排队 + 说明里标出，有权限限制）、`--expire`。
6. 卡住检测（acked 太久且接收方重启过）。
7. 远期：结构化 `data`、skills、A2A 网关。

## 参考

- A2A 规范（v1.0）：https://a2a-protocol.org/latest/specification/
- Linux Foundation 关于 A2A 一周年的公告：https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year
- myco：`internal/domain/message.go`（状态机）、`internal/app/messages.go`（租约策略）、`internal/infra/sqlite/messages.go`（取消息顺序）、`internal/runner/runner.go`（先 ack 再执行）、`internal/runner/prompt.go`（唤醒说明）、`docs/delivery.md`（唤醒方式）
