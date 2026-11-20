# MCP、技能、插件：现状与方案（调研稿）

状态：调研稿（2026-11-20），未动代码，等拍板。

一句话：myco 现在**不管理** agent 的 MCP、技能（Claude / Codex 的 Skills）和插件。它们由各 agent CLI 自己在每台电脑上读配置加载，myco 只是「照常启动 CLI」；App 里看不到、改不了、也不能按项目下发。App 里叫「技能」的，其实是另一个东西（agent 的名片）。

## 1. 现状

### 1.1 三个词在 myco 里现在分别是什么

| | myco 现在做了什么 | 没做什么 |
| --- | --- | --- |
| **MCP 服务器** | 什么都不配。runner 启动 Claude Code / Codex 时用的是那台电脑上用户自己的配置，所以用户级的 MCP 会被加载。过程里能看到 MCP 调用：Codex 的 `mcp_tool_call` 显示成「服务器.工具」；Claude 的 MCP 工具（`mcp__服务器__工具`）和其他工具一样显示。ACP 类 agent（Gemini 等）我们在 `session/new` 里传的是空列表，拿不到任何 MCP。myco 自己不提供 MCP 服务器，agent 用 myco 靠的是 `Bash(myco:*)`。 | 不知道每台电脑、每个 agent 有哪些 MCP；不能加、删、开关；不能按 agent / 项目区分。 |
| **技能（CLI 的 Skills）** | 只装一个：myco 自己的 `SKILL.md`（编进二进制）。`myco skill install` 写到 `~/.claude/skills/myco/` 和 `~/.codex/skills/myco/`，`myco upgrade` 时更新；另外 `myco join` 在用户级 `CLAUDE.md` / `AGENTS.md` 里写一小段 myco 用法。 | 用户自己的技能（`~/.claude/skills`、项目的 `.claude/skills`、`~/.codex/skills`…）完全不看、不列、不分发。 |
| **App 里的「技能」** | 是 agent 的**名片**，不是上面的 Skills：名字 + 一句话 + 例子（最多 20 个），存在工作区的 agents 表里。别的 agent 和工作流「按分工挑人」时看它；`myco agent skills`、App 资料页「技能」一节可改。 | 和 CLI 的 Skills 没有关系，名字一样容易混。 |
| **插件** | 没有。Claude Code 的插件（市场、`plugin.json`，可以打包技能、hooks、MCP）和 Codex 的插件（`config.toml` 里的 `[plugins]`）都由 CLI 自己管。myco 自己的扩展方式是「应用 / 官方应用」，那是做网页的模板，不给 agent 加能力。 | 不知道装了哪些、开没开。 |

### 1.2 各 CLI 的配置在哪（myco 不读也不写）

| | Claude Code | Codex |
| --- | --- | --- |
| MCP | 用户级 `~/.claude.json`；项目级 `.mcp.json`（要先在交互里批准才加载）；插件里的 `.mcp.json`。`claude mcp add/list/remove` | `~/.codex/config.toml` 的 `[mcp_servers.<名>]`；受信任项目的 `.codex/config.toml`。`codex mcp add/list` |
| 技能 | `~/.claude/skills/<名>/SKILL.md`、项目 `.claude/skills/`、插件带的 | `~/.codex/skills/`、项目 `.codex/skills/` 等 |
| 插件 | 市场（`marketplace.json`）装到 `~/.claude/plugins/cache/…`，开关在 `settings.json` | `config.toml` 的 `[plugins]` |
| myco 启动方式 | `claude -p --output-format stream-json`，加 `--permission-mode`、`--allowedTools "Bash(myco:*)"`、`--settings <provider 文件>` | `codex exec --json`，加 `-c sandbox_mode=…`，完全权限时 `--dangerously-bypass-approvals-and-sandbox` |

### 1.3 已知的缺口和坑

1. **看不见**。App 不知道哪台电脑、哪个 agent 有什么 MCP / 技能 / 插件，派活和「按分工挑人」也用不上（比如「谁接了 Figma」）。
2. **工作区权限下 MCP 很可能用不了**（需实测确认）。Claude Code 在 `-p` 无头模式里没人能点「允许」；我们的「工作区权限」是 `acceptEdits`，只自动放行改文件，MCP 工具调用会被拒，只有「完全权限」才通。项目级 `.mcp.json` 没在交互里批准过的，无头模式下不会加载。Codex 在 `workspace-write` 沙箱里，需要访问外部的 MCP 是否受影响也要实测。
3. **一个坏 MCP 拖住启动**。MCP 服务器起不来时 CLI 会卡在启动；我们 v0.21.1 加的「5 分钟没输出就判失败」就是为这类情况（报错里能看到）。
4. **同一台电脑上所有 agent 共用**用户级配置：不能给某个 agent / 项目单独配，一个坏的影响全部。
5. **密钥**：MCP 配置里常有 token，现在都在各 CLI 的配置文件里，myco 不碰；一旦由 myco 来配或下发，就要管密钥存放（以及和端到端加密的关系）。
6. **「技能」一词两义**：App 里的「技能」（名片）和 Claude / Codex 的 Skills 是两回事，用户会以为在 App 里加的技能 agent 会用到。
7. **ACP 类 agent 拿不到 MCP**（我们传了空列表）。
8. **云端 agent**（claude.ai、Codex 云端）的 MCP / 技能完全在 myco 之外。

## 2. 可选路线

| 路线 | 做什么 | 工作量 | 风险 | 影响到 |
| --- | --- | --- | --- | --- |
| **E. 先修权限坑** | 「工作区权限」下也允许 MCP 工具（Claude 加 `--allowedTools` 放行 MCP；或在运行设置里加一个「允许 MCP 工具」开关）；Codex 实测后同样处理。 | 小（1–2 天，含实测） | 低；是安全取舍：MCP 能做的事不受文件夹限制 | runner 的 harness 参数、运行设置 |
| **A. 只看（清单）** | 电脑上的 myco 读各 CLI 的配置，上报每台电脑 / 每个 agent 的 MCP（名字、本地还是远程、开没开，**不含密钥**）、技能（名字 + 描述）、插件（名字、来源、开没开）。App 在 agent 资料和电脑页显示；agent 名片可以自动带上（如「有 Figma MCP」），派活和按分工挑人能用。 | 中小（3–5 天） | 低：只读，不改用户配置 | 电脑上报（machine inventory、runtime）、App 两处 |
| **B. 在 App 里管** | 在 A 的基础上，App 里给某台电脑的某个 CLI 加 / 删 / 开关 MCP、技能、插件：调用 CLI 自己的命令（`claude mcp add`、`codex mcp add`、`claude plugin install`），不自己改配置文件。只在这台电脑、或 App 用 SSH / WSL 连得到的电脑上做（和「允许的文件夹」同一原则）；别的电脑给出要在那边跑的命令。 | 中（1–2 周） | 中：改用户配置、要输入密钥、CLI 版本差异 | App、App 后端、电脑上的 myco |
| **C. 由 myco 下发** | 工作区 / 项目定义一组 MCP 和技能，runner 跑任务时临时注入（Claude 用临时的 `--mcp-config`、技能放到临时目录；Codex 用 `-c mcp_servers.…`）。密钥放工作区（加密）或只放各电脑本地。 | 大（2–4 周） | 高：工作区能让你的电脑跑某个 MCP 命令，等于一种远程执行，必须受电脑本地规则约束；密钥分发 | runner、工作区数据表、App、加密 |
| **D. myco 自己做 MCP 服务器** | 把 myco 的能力（发消息、看项目、派活、报进度）做成 MCP 工具给 agent，替代或补充 `Bash(myco:*)`；顺带用 Claude 的 `--permission-prompt-tool`，把 agent 卡住的权限请求变成 App 里的「同意 / 不同意」卡片（`client-plan.md` 里提过）。 | 中（1–2 周） | 中 | runner、harness、App 卡片（已有 superagent 的确认卡片可复用） |

### 推荐

**先做 E + A，B 看需要，C 暂缓，D 单独排期。**

- E 和 A 都小、风险低，马上解决两个实际问题：「工作区权限下 MCP 用不了」和「不知道谁有什么」。A 还让名片和派活有了真实依据。
- B 是 A 的自然下一步，但会动用户配置、要输入密钥；等真的有人想在 App 里加 MCP 再做。
- C 价值在团队场景（项目统一的 MCP / 技能），但安全面最大：相当于工作区能决定你电脑上跑什么程序。现在单人使用为主，不值得先做。
- D 的「权限请求变 App 卡片」价值很大（无头 agent 卡在权限上是常见坑），和 superagent 的确认卡片是同一套界面，建议作为独立项目做，不和 MCP 清单绑在一起。

## 3. 需要你拍板的点

1. **「技能」改不改名**：App 里 agent 名片的「技能」要不要改成别的词（比如「擅长」「能接的活」），和 Claude / Codex 的 Skills 区分开？
2. **工作区权限下默认放行 MCP 工具吗**（路线 E）：放行更好用，但 MCP 能做的事不受文件夹限制；不放行就只有完全权限能用。还是做成运行设置里的一个开关、默认关？
3. **只看，还是也要在 App 里改**（A 还是 A+B）？如果要改，远程电脑只给命令（和允许文件夹一样），还是可以经电脑上的 myco 远程改（需要主人确认）？
4. **项目级共享要不要**（路线 C）：项目统一一组 MCP / 技能自动给成员？要的话，密钥放工作区（加密）还是只放各电脑本地？
5. **范围**：只管 Claude Code 和 Codex，还是 ACP 类（Gemini、Qwen…）也要（它们现在一个 MCP 都拿不到）？
6. **云端 agent**（claude.ai、Codex 云端）不管，可以吗？
7. **myco 做 MCP 服务器 / 权限请求卡片**（路线 D）要不要排期，排在 A 之前还是之后？

## 4. 参考

- Claude Code 插件与设置：<https://code.claude.com/docs/en/plugins-reference>、<https://code.claude.com/docs/en/settings>
- Codex MCP 与配置：<https://developers.openai.com/codex/mcp>、<https://developers.openai.com/codex/config-reference>
- 代码：`internal/harness/claude/claude.go`（启动参数）、`internal/harness/codex/codex.go`、`internal/harness/codex/events.go`（MCP 调用事件）、`internal/harness/acp/acp.go`（空的 `mcpServers`）、`internal/cli/cmd_skill.go`（myco 技能安装）、`internal/domain/skills.go`（名片技能）
