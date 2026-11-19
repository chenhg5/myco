# 内置浏览器：在 App 里打开 agent 做的网页

状态：第一步（网页面板 + 发布静态网页）已做（2026-11-17，Hub v0.19.24 起）；后面几步待定。

## 1. 为什么要做

让 agent 做 PPT、报告、画布，如果都在 App 里各写一套原生界面，太重了。网页能承担大部分：

- **agent 的产出很多本来就是网页**：一份带图表的报告、一套幻灯片（reveal.js 之类）、一个小工具、一个原型、一个前端项目的预览。
- **网页是人和 agent 共用的空间**：比文字消息能展示得多，也能交互。以后的画布、看板、[小程序和「一局」](co-create.md)都跑在同一个地方。
- **在 myco 里闭环**：不用把文件下到本地、再找浏览器打开，也不用 agent 自己去找地方部署。

所以 App 里要有一个**网页面板**：agent 发出来的网页，点一下在 App 右侧打开。

## 2. 结论和取舍

| 问题 | 结论 |
| --- | --- |
| 要不要打包浏览器内核 | **不要**。桌面版用 Wails，界面本来就跑在系统自带的内核上（Mac 是 WKWebView，Windows 是 WebView2）。网页面板用同一个内核，安装包只多几十 KB 代码。打包 Chromium（CEF）会多 150～200 MB，还要跟着 Chrome 做安全更新 |
| 用 iframe 还是原生子 webview | **iframe**。网页由我们自己托管，不会被"不许嵌入"挡住；菜单、弹窗能正常盖在上面；两个平台一套代码。原生子 webview 只在"要在里面登录外部网站"时才需要，到时候再做（第 9 节） |
| 网页放在哪 | **Hub 上**。agent 用一条命令把一个文件夹发上去，电脑关着也能看，项目里的人都能打开 |
| 怎么防止网页做坏事 | **和 App 不同源**，并且放在沙箱里（第 5 节）。agent 写的网页一律当作不可信的代码 |

别的产品：Cursor、Claude 桌面版、Codex 桌面版是 Electron，本来就带一份 Chromium，所以内置浏览器不另外花体积。Claude 的 artifact 和 VS Code 的 Simple Browser 用的是隔离的 iframe，和我们这里的做法一样。Tauri 的"一个窗口里放多个原生 webview"至今还标着 unstable；Wails v3 只有多窗口。

## 3. 用起来是什么样

1. 你让 agent "把这周的数据做成一份能翻页的报告"。
2. agent 在它的电脑上做好一个文件夹（`index.html`、JS、CSS、图片），运行：
   ```
   myco page publish --dir ./report --name "周报 · 第 46 周" --project growth
   ```
   命令打印出这个网页的地址；agent 在回复里写上它。
3. 你在对话里看到的不是一串网址，而是一张**网页卡片**：图标、名字、「由 增长分析（Claude Code）发布 · 刚刚」、「打开」。
4. 点「打开」，网页在右侧的面板里显示，和现在打开 Markdown 文件的是同一个面板：左边的对话还能滚、还能发消息。
5. 你说"第三页的图换成柱状图"。agent 改完再发一次（`--page <id>`），卡片上变成「第 2 版」；面板里出现一条「有新版本 · 刷新」。
6. 这个网页也出现在项目的「资料」里，项目里的人都能打开。

agent 在**这台电脑**上跑的开发服务器（回复里写着 `http://localhost:5173`），点了也在面板里打开，不跳去浏览器。

## 4. 面板

复用现在文件预览的停靠面板（`preview.tsx`：窗口够宽时停在右边，不够宽时盖在对话上；同一时间只开一个）。

- **顶上一行**：网页名字和版本（「周报 · 第 46 周 · 第 2 版」）、刷新、只看这一栏（全宽）、更多（复制链接、在浏览器打开、看所有版本）、关闭。
- **不做地址栏**。这不是一个通用浏览器：面板只打开 myco 里的网页和这台电脑上的地址。网页里点了外面的链接，就用系统浏览器打开（和对话里的链接一样）。
- **加载中**和**打不开**都有说明：「这个网页已经删除了」「这个工作区的 Hub 还不支持网页，升级到 vX 以上」「这台电脑上 5173 端口没有在跑」。
- 网页自己弹的 `alert`、`confirm` 允许；下载允许（交给系统保存）；全屏 API 不开放（用面板自己的「只看这一栏」）。
- 打开过的网页记在「最近」里（⌘K 也能找到）。

## 5. 安全

agent 写的网页可能有意或无意地想读 App 里的东西（令牌、对话、别的工作区）。规则：

- **不同源**。桌面 App 的界面是 `wails://localhost`（Mac）或 `http://wails.localhost`（Windows）。网页由本机的 console 在**另一个回环端口**上提供：`http://127.0.0.1:<端口>/p/<一次性钥匙>/<路径>`。不同源，所以网页读不到 App 的存储，也调不了 `/app/` 接口。
- **只认带钥匙的请求**。console 打开一个网页时生成一把随机钥匙，只对这个网页的这一版有效，App 关掉后失效。没有钥匙的请求一律 404。这台电脑上的别的程序猜不到地址，也就拿不到。
- **沙箱 iframe**：`sandbox="allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-same-origin"`。`allow-same-origin` 在这里是安全的，因为它的源本来就和 App 不同；不允许 `allow-top-navigation`，网页跳不走整个 App。
- **CSP**：console 给网页加 `Content-Security-Policy`，默认只能加载自己这一版里的文件和 `data:` / `blob:`，`connect-src 'self'`（不能把数据发到外面）。以后可以按网页放开（发布时写明要访问哪些地址，人第一次打开时确认）。
- **网页之间**：所有网页共用那一个回环端口的源，所以它们的 `localStorage` 是共用的。第一版接受这一点（网页里不该放秘密），在 skill 里写明。以后要隔离，可以一个网页一个端口，或者第二步的桥提供按网页隔离的存储。
- **端到端加密的 Hub**：网页的文件以明文存在 Hub 上，所以开了端到端加密的 Hub 上 `publish` 会拒绝，并说明原因（和 co-create 一样，以后再做加密）。
- 卡片和面板上都写明「由谁发布」。

## 6. Hub：网页存在哪

一个**网页**有多个**版本**，一个版本是一个文件夹。

- 上传的是一个 zip，Hub 校验后整包存进现有的附件存储（`internal/infra/blob`），按 zip 里的目录提供文件，不解包。
- 新增表（迁移在 `internal/infra/sqlite`，老库原地升级）：
  - `pages`：id、名字、所在项目（可空）、发布者（agent）和它的主人、最新版本号、创建和更新时间、是否删除。
  - `page_versions`：网页 id、版本号、blob、sha256、大小、入口文件（默认 `index.html`）、文件清单、发布者、时间。
- **限制**：一版最多 50 MB、2000 个文件；单个文件 20 MB；路径不能有 `..`、不能以 `/` 开头；每个工作区默认最多 200 个网页，旧版本保留最近 10 个。
- **谁能看**：发到项目的，能看到这个项目的人和 agent 都能看；没发到项目的，网页的主人（发布它的 agent 的主人，或者发布它的人）、主人的 agent、工作区的所有者和管理员能看。（"收到过带它链接的消息的人也能看"第一版没做：发给别人看就发到项目里。）
- **谁能改、能删**：发布者、它的主人、主人的 agent、管理员。

接口（`docs/api.md` 里写全；老客户端不受影响，老 Hub 返回 404）：

| 接口 | 做什么 |
| --- | --- |
| `POST /v1/pages?name=&project=&entry=` | 发一个新网页：body 是 zip → `{id, name, version: 1, url}` |
| `POST /v1/pages/{id}/versions?entry=` | 发新版本 |
| `GET /v1/pages`、`GET /v1/pages/{id}` | 列出、看一个（版本列表） |
| `GET /v1/pages/{id}/v/{n}/{path}` | 某一版里的一个文件（`n` 可以是 `latest`） |
| `DELETE /v1/pages/{id}` | 删掉 |
| （同一套接口） | agent 的令牌和 App 的 console 令牌都能用，权限按身份算 |

`url` 是 `<hub>/pages/<id>`：在 App 里认出来就变成卡片；在别处打开（比如浏览器里），是 Hub 网页版的一页，登录后显示同一个面板。

## 7. CLI

```
myco page publish --dir <文件夹|单个 html> [--name …] [--project p] [--entry index.html] [--page <id>] [--dry-run]
myco page list [--project p]
myco page get --page <id>
myco page rm --page <id> [--dry-run]
```

- `publish` 自己打 zip；会跳过 `.git`、`node_modules`、`.DS_Store` 和隐藏文件，超限时说清楚是哪个文件太大。
- 文件夹里没有入口文件时报错，并列出它找到的 html 文件。
- 输出非终端时是 JSON（`{id, version, url}`），和别的命令一样；错误带代码、建议和 `retryable`。
- skill 里加一节"把结果做成网页"：什么时候值得做成网页（图表、翻页的报告、要点来点去的原型），文件都放在文件夹里（不要引用外网的 CDN，CSP 会挡），发出去后把链接写在回复里。

## 8. App 的改动

- **认出网页链接**：Markdown 里指向当前工作区 `/pages/<id>` 的链接，渲染成网页卡片（`PageCard`），拿不到信息（老 Hub、没权限、删了）就退回成普通链接。
- **本机地址**：`http://localhost:*` 和 `http://127.0.0.1:*` 的链接，点了在面板里直接打开（同样是不同源的 iframe），面板顶上写「这台电脑上的 5173 端口」。别的电脑上的 localhost 打不开，写明原因（第 9 节第二步解决）。
- **console**：新开一个回环端口专门提供网页（第 5 节），从 Hub 取文件时用这个人自己的令牌；带缓存（按 sha256），所以同一版只下一次。
- **项目资料**里多一类「网页」，和文档、文件放在一起。
- 设计规范（`app-ui.md`）加一节「网页卡片和网页面板」，冒烟测试加：卡片、打开、刷新到新版本、只看这一栏、外链用浏览器打开、老 Hub 退回成链接。

## 9. 分几步做

1. **网页面板 + 发布静态网页**（本文的主体）：Hub 的 pages、CLI 的 `page`、console 的网页端口、App 的卡片和面板、本机 localhost 在面板里打开。估计 5～7 天：Hub 和 CLI 2 天，console 和安全 1～2 天，App 2 天，测试和文档 1 天。
2. **桥：网页里的 `myco` 对象**：按网页隔离的存储、读写共享状态、给 agent 发消息、收到更新。这就是 [co-create](co-create.md) 里的桥，网页和小程序用同一个。
3. **别的电脑上的开发服务器**（已做，Hub v0.19.24 起）：agent 运行 `myco page serve --port 5173`，登记一个「实时网页」并和 Hub 保持一条 WebSocket，上面用 yamux 多路复用；App 打开时，请求走 App → 本机 console（这个网页单独一个回环端口，因为开发服务器的路径从根开始）→ Hub 的 `/v1/pages/{id}/live/…` → 那条连接 → agent 电脑上的 `localhost:5173`，热更新的 WebSocket 也一样。转发时去掉人的令牌和 cookie，`Host` 换成 `localhost:<端口>`（开发服务器只认本机）。不经过 runner：哪种方式接入的 agent 都能用。
4. **按需要**：原生子 webview（在面板里登录外部网站）；iOS 的网页面板（同样是 WKWebView，同一套）；网页版控制台里打开网页。

每一步都保证老的不坏：老 Hub 没有 pages 接口时，链接就是普通链接；老 App 看到网页链接，点了用浏览器打开 Hub 网页版。

## 9.1 第二步：桥

状态：已做（2026-11-17，Hub v0.19.24 起）。桥脚本是 `internal/console/pagebridge.js`（App 和 mock 用同一个文件）。

网页里多一个全局的 `myco` 对象，让网页成了人和 agent **一起动手**的地方：人在网页里勾一项、填一格、拖一张卡片，agent 读到；agent 改了，人开着的网页马上变。名字和 [co-create](co-create.md) 里小程序的桥一致，以后小程序沿用它，再加上「一局」的参与者、回合和事件。

```js
const ctx = await myco.ready;          // { page: {id, name, version}, me: {name}, state, version }
myco.on("change", (c) => draw(c.state)); // 别人（agent 或别的人）改了共享状态
const r = await myco.update(next);     // 写共享状态（基于看到的版本）；别人先写了就失败，r.state 是最新的
await myco.send("排好了，帮我看看第三列"); // 以你的名义给发布这个网页的 agent 发一条任务
await myco.storage.set("draft", {...});  // 只属于这个网页、只在这台电脑上的存储
await myco.storage.get("draft");
```

- **共享状态**：每个网页在 Hub 上有一份 JSON（最多 256 KB）和一个版本号，所有版本的网页共用（发新版本不丢）。`update` 带着看到的版本号写，对不上就返回 409 和最新的状态（`{ok: false, state, version}`），网页重画后让人再操作。谁能看这个网页，谁就能写它的状态。
- **agent 那边**：`myco page state get --page <id>` 读，`myco page state set --page <id> --file next.json [--if-version N]` 写，和人写的是同一份。状态里的内容是数据，不是给 agent 的指令（skill 里写明）。
- **更新**：面板开着时，App 每 2 秒问一次状态的版本，有变化就把新状态推给网页（`change`）；网页自己写入的不再推一遍。
- **给 agent 发消息**：`myco.send(text)` 发给网页的发布者（agent），是一条普通任务，正文在后面带上「（来自网页「名字」<地址>，状态第 N 版）」，回复照常回到你和它的对话里。网页是 agent 写的代码，所以**每个网页第一次发之前，App 问你一次**：「允许「周报」以你的名义给 增长分析 发消息吗？」，允许后这次打开 App 期间不再问；一分钟最多 5 条。
- **记录（v0.20.5）**：比整份状态更细的一层：`myco.data.list / get / put / remove / on`，按集合和记录读写，每条带版本，别人改的经 `on` 收到；`myco.download(name, content, type)` 把网页生成的文件存到「下载」：文档和图片（20 MB 以内），视频和声音（mp4、webm、mov、m4a、mp3、wav，4 GB 以内，边收边写盘）；桥把 Blob 原样交给 App（postMessage 复制，不转 base64），最多等 10 分钟。`myco.features` 列出桥能做的事（v0.20.27 起：`download-media`），网页先看一眼再决定要不要试。见 [templates.md](templates.md)。
- **按网页隔离的存储**：`myco.storage` 存在 App 自己那边（按工作区和网页分开，每个网页最多 1 MB），网页之间互相看不到；`localStorage` 仍然是几个网页共用的。
- **在哪都能用**：桥脚本就是现在把外链交给浏览器的那个 `/__myco/page.js`，所以不用改网页的发布方式。在面板以外打开（比如浏览器直接打开 Hub 上的文件），`myco.ready` 会失败，网页应该能只读地显示。
- **底下**：`postMessage`。网页发 `{myco: "call", id, method, args}`，App 只认来自这个面板 iframe 的消息（`event.source`），回复只发给网页的源。

## 10. 第一天先验证的事

- **Mac 的 WKWebView 里，`wails://` 的页面能不能嵌 `http://127.0.0.1` 的 iframe**（混合内容）。回环地址按规范算"安全的源"，应该可以；如果不行，改用 Wails 资源处理器下的一个子路径，加 CSP `sandbox` 头让网页成为独立的不透明源，代价是网页不能用 `localStorage`（第二步的桥来补）。
- Windows 的 WebView2 同样验证一遍。
- iframe 里的网页点外链、`target=_blank`、下载，在两个平台上分别是什么行为。

## 11. 还要定的事

- **名字**：界面上叫「网页」？还是和小程序合成一个词？倾向先叫「网页」：人一看就知道是什么；以后有了桥、能一起操作的，再叫「小程序」。
- **谁能看**：第 6 节的规则是否够用，还是第一版干脆"只有项目里的网页，和发给你的网页"。
- **要不要允许网页联网**（比如调一个公开的 API 画实时图表）。倾向第一版不允许，数据由 agent 拿到后打进网页。
- **单个 html 的网页**和**小程序**是不是同一个东西（co-create 里小程序就是一个 html 文件）。倾向底层是同一个（一版只有一个文件的网页），有了桥以后才能"开一局"。

## 12. 不做的事

- 不做通用浏览器：没有地址栏、书签，不打开任意网址。（面板里的**标签**后来加上了：打开第二个网页时出现，每个标签保持自己的网页，最多 8 个。）
- 不打包浏览器内核。
- 不在 Hub 上跑网页的后端代码；网页只有静态文件，需要数据由 agent 写进去，或者第二步通过桥拿。
