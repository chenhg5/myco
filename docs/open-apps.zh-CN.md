# 用 Agent 重构 100 个优秀应用，免费开放，持续更新

[English](open-apps.md) · **中文** · [应用列表](apps.zh-CN.md) · [README](../README.zh-CN.md)

> 我们会用 agent 把人们每天离不开的 100 个应用重新做一遍，全部免费开放，并且一直更新下去。
> 这些应用不再分"付得起的人"和"付不起的人"，也不再分"人能用"和"agent 能用"。

## 为什么是现在

做一个好用的应用，过去要一个团队花上几个月甚至几年。所以它很贵，所以它要收费，所以好用的东西先给付得起的人用。

agent 改变了这件事。今天，一个人带着几个 agent，就能把一个成熟应用的核心体验做出来，而且做得越来越快。我们正在进入一个"许愿"的时代：你说出想要什么，agent 帮你做出来。

当做应用的成本降到接近于零，再把它锁在付费墙后面，就是在和潮水较劲。我们不想螳臂当车，而是想换一种更开放的姿态：**既然做出来已经这么便宜，那就让所有人都用上。**

## 我们要打掉两堵墙

### 第一堵：付费墙

很多应用最好的体验，被放在了订阅和会员后面：好看的模板、顺手的剪辑、像样的协作、聪明的整理。它们本身并不难，只是贵。

我们想做的是**体验平权**和**知识平权**：

- 学生、自由职业者、小团队、刚起步的人，和大公司的人用上同样好用的工具；
- 一个人在小城市里能做到的事，不因为付不起几个订阅而少一截；
- 做应用的方法本身也公开：代码开源，谁都可以学、可以改、可以拿去用。

### 第二堵：数据墙

你的文档、表格、笔记、账本、会议记录，其实是你的东西。可它们常常被关在各家应用里：agent 看不到，导不出来，换个工具就要从头再来。

在 agent 时代，这堵墙的代价更大：你的 agent 再聪明，碰不到你的数据也帮不上忙。

所以我们做的每一个应用都**生来就让 agent 能用**：

- 你和 agent 用的是同一个页面，agent 帮你填、帮你改，你这边实时看到；
- **只要你授权**，你的 agent 就能读写这些应用里的内容；你不授权，谁也碰不到；
- 数据默认留在你自己的电脑或你自己的 Hub 上，随时能导出成通用格式（Markdown、Excel、CSV、MP4、PowerPoint……）。

## 我们坚持的原则

1. **免费开放。** 应用免费用，代码开源。不靠会员墙，也不靠卖你的数据。
2. **你的数据归你。** 本地优先；录音、照片、视频这类内容只留在打开它的电脑上。能导出，能带走。
3. **人和 agent 平等使用。** 每个应用都同时为人和 agent 设计，而不是事后补一个接口。
4. **授权才能访问。** agent 能做什么，由你决定，可以随时收回。
5. **学的是体验，不抄别人的东西。** 我们参考的是优秀应用"好用在哪里"，自己从头实现；不复制别人的代码、素材和商标，不破解、不绕过任何人的付费或权限。
6. **持续更新。** 不是做完一版就放着。每个应用都会跟着大家的反馈一直改进。

## 这件事不是什么

- **不是盗版，也不是破解。** 我们不碰别人的软件，只是用 agent 把同类体验重新做一遍，并且开源。
- **不是反对做应用的人赚钱。** 好的产品值得被付费。我们只是相信，"基本好用"这件事，在 agent 时代应该成为人人都有的公共品；在这之上，大家仍然可以去做更好、更专业、值得付费的东西。
- **不是一个封闭的平台。** 应用跑在你自己的电脑和你自己的 Hub 上，代码在 GitHub 上，谁都可以拿去改、拿去部署。

## 100 个应用怎么选

我们按三个问题挑选：

1. **用的人多吗？** 每天、每周都会打开的那种。
2. **门槛高吗？** 好用的版本是不是要付费、要会员、要贵的订阅。
3. **agent 能帮上大忙吗？** 让 agent 一起用之后，体验会不会明显变好。

目前已经上架 **26 个**，覆盖创意设计、办公套件、编程开发、效率工具、生活和游戏，完整列表见 [应用](apps.zh-CN.md)。剩下的，我们希望和大家一起决定。

## 进度

| | |
|---|---|
| 已上架 | 26 / 100 |
| 分类 | 创意设计 7 · 办公套件 5 · 编程开发 3 · 效率工具 6 · 生活 3 · 游戏 2 |
| 下一批 | 在 [Discussions](https://github.com/myco-ai/myco/discussions) 里由大家提名和投票 |

## 你可以怎么参与

### 1. 装上 myco，把应用用起来

| 平台 | 下载 |
|---|---|
| **Mac** | [myco.dmg](https://github.com/myco-ai/myco/releases/latest/download/myco.dmg)：Apple 芯片和 Intel 通用，已签名公证，会自动更新 |
| **Windows**（预览） | [myco-app-windows-amd64.exe](https://github.com/myco-ai/myco/releases/latest/download/myco-app-windows-amd64.exe)（[ARM 版](https://github.com/myco-ai/myco/releases/latest/download/myco-app-windows-arm64.exe)）。还没有代码签名，第一次运行选「更多信息 → 仍要运行」 |
| **Linux** | [myco-app-linux-amd64](https://github.com/myco-ai/myco/releases/latest/download/myco-app-linux-amd64)（[ARM 版](https://github.com/myco-ai/myco/releases/latest/download/myco-app-linux-arm64)），在浏览器里打开 App |

打开 App，跟着第一屏在本机开一个 Hub，然后在侧边栏的 **应用** 里就能看到全部应用。

已经在用 Claude Code、Codex 这类能跑命令的 agent？把下面这句话发给它，它会帮你装好：

```text
帮我装好 myco：先读 https://raw.githubusercontent.com/myco-ai/myco/main/docs/agent-setup.md （打不开就读 https://cdn.jsdelivr.net/gh/myco-ai/myco@main/docs/agent-setup.md ），照着做。要我决定的先问我，用 sudo 或改系统设置前也先问我，装完告诉我结果。
```

更多装法见 [安装指南](../INSTALL.zh-CN.md) 和 [使用教程](tutorial.zh-CN.md)。

### 2. 提名下一个应用

到 [Discussions](https://github.com/myco-ai/myco/discussions) 发一帖，写清楚三件事：

- **哪个应用**，你平时拿它做什么；
- **它好用在哪里**：最离不开的两三个功能；
- **它贵在哪里**：要付多少钱、卡在哪个功能上。

给别人的提名点赞，票多的会先做。

### 3. 用了告诉我们哪里不好

到 [Issues](https://github.com/myco-ai/myco/issues) 提。写上应用名、`myco version` 的输出、系统，以及怎么复现；能附截图更好。安全问题请走 [SECURITY.md](../SECURITY.md)，不要公开发。

### 4. 自己做一个应用

应用开发已迁到独立仓库 [myco-ai/apps](https://github.com/myco-ai/apps)，独立检查和发布。主仓库暂留旧副本供现有构建使用，应用改动请提交到 apps。

每个应用就是 `templates/src/` 下的一个文件夹：一个 React 页面，加上 `public/` 里的几份文件。

| 文件 | 作用 |
|---|---|
| `index.html`、`main.tsx`、`*.css` | 页面本身 |
| `public/myco.json` | 应用的说明书：id、名字、图标、分类、描述，以及它存哪些数据、每个字段是什么类型 |
| `public/agent.md` | 告诉 agent 怎么用这个应用、怎么往里填数据 |
| `public/sample.json` | 示例数据，截图和测试都用它 |
| `public/locales.json` | 中英文文案 |
| `public/shots/` | 应用列表里展示的截图（亮色和暗色各一张） |

页面通过 `myco.data.*` 读写数据（见 [HTTP API](api.md)），不从互联网加载任何东西；要用到电脑本身（文件、命令、屏幕）的，走 [`myco.os`](app-os.md)。界面规范见 [docs/design/app-ui.md](design/app-ui.md)。

**最快的起步：** 照着一个现成的改，比如 `templates/src/todo`（待办清单）。

```bash
git clone https://github.com/myco-ai/apps && cd apps
cd templates && npm ci
cp -r src/todo src/my-app        # 从一个现成的应用起步，再改掉 public/myco.json 里的 id、名字和字段
npm run build                    # 类型检查并生成到 dist/official/
node shots.mjs my-app && node build.mjs   # 生成截图
cd .. && make check              # 提交前跑一遍全部检查
```

**让你的 agent 来做：** 在仓库目录里打开 Claude Code 或 Codex，告诉它：

```text
读一下 AGENTS.md 和 templates/README.md，参考 templates/src/todo，在 templates/src/ 下做一个新的 myco 应用：<你要做的应用和它的核心功能>。界面按 AGENTS.md 链接的 myco App 界面规范，数据用 myco.data，写好 agent.md 和 sample.json，跑通 npm run build、截图和 make check。
```

做好后到 [myco-ai/apps](https://github.com/myco-ai/apps) 提一个 PR：一个 PR 只做一件事，跑通 `make pack`，不要提交生成的 `dist/`。检查与宿主兼容要求见该仓库 AGENTS.md。大一点的应用，先开 issue 或在 Discussions 里说一声，免得撞车。

所有贡献都按 [MIT 许可证](../LICENSE) 开放。

### 5. 让更多人知道

给 [仓库](https://github.com/myco-ai/myco) 点个 star，把你最喜欢的一个应用分享给需要它的朋友。

---

agent 的时代，好用的东西不该只属于少数人。我们一起，把 100 个优秀应用，免费开放给每一个人。
