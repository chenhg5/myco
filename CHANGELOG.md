# Changelog

One section per release (`## vX.Y.Z`, newest first): the changes users notice, in English, then the same in Chinese. The release workflow publishes the section as the GitHub release notes.

更新记录：每次发版一节，英文在前，`**中文**` 下面是同样的中文。

## v0.22.16

- Goals: set a bounded goal on a chat and the agent takes it up on idle turns instead of waiting for your next message.
- Watching: a chat shows a bar at the top for a watch (a bounded command wait on a permitted local runner) and its ended lines, and the bar fits a narrow pane.
- A new clean (清爽) appearance in Settings → Appearance, which also hides the workspace's face in the bottom-left corner.
- Chat layout: a chat keeps your place when bars appear above it, and the newest unfinished plan stays pinned when nothing is running.
- Browser control: replay a literal draft inside a live control session without recording again; close receipts are kept, unreadable screenshots are redacted, and profile lifecycle and scoped navigation feedback are fixed.
- Live browser features in this release need browser component 0.6.0, and the App prompts you to update it.

**中文**

- 目标：在聊天里设定一个有边界的目标，agent 会在空闲轮次里继续推进，而不是等你的下一条消息。
- 盯着：聊天气泡顶部会显示监视条——在被允许的本地执行器上等待一条有界命令——以及已结束的记录，窄窗口下也能正常显示。
- 设置 → 外观新增「清爽」风格，左下角的工作区头像也会一并隐藏。
- 聊天界面：上方出现条栏时不会跳走你的位置；没有任务在跑时，最新的未完成计划会固定显示。
- 浏览器控制：在实时会话里可直接回放字面量草稿，不必重新录制；保留关闭回执，无法读取的截图会被隐去，并修复配置档生命周期与限定域导航的反馈。
- 本次发布的实时浏览器功能需要浏览器组件 0.6.0，App 会提示更新。

## v0.22.15

- After a separate grant (up to 30 minutes), an agent can operate the browser directly in a dedicated clean agent profile, optionally restricted to the domains you allow; the panel shows the agent is operating and you can stop it at any time.
- Actions an agent performs in the browser are recorded into a draft script you can review, edit and run later; an interrupted session discards unfinished drafts, and private screenshots stay on your machine.
- Live browser control needs browser component 0.5.0, and the App prompts you to update the component; older component versions remain usable for existing recording and playback.

**中文**

- 在你单独授权后（最长 30 分钟），agent 可以直接操作浏览器：默认使用 agent 专用的干净配置，可限定域名；面板会显示 agent 正在操作，并可随时停止。
- agent 在浏览器里的操作会录成草稿脚本，你可以查看、修改后再运行；会话中断会丢弃未完成的草稿，截图等私有产物只保存在本机。
- 实时浏览器控制需要浏览器组件 0.5.0，App 会提示更新组件；旧版组件仍可用于原有的录制与回放。

## v0.22.14

- **The Windows App icon is the mushroom alone**: on Windows the icon now fills its square edge to edge — the mushroom with a thin cream rim, drawn as the tray icon is — instead of a small mushroom on a cream tile that vanished on light taskbars. The Mac icon keeps its tile.
- **An interrupted request no longer takes away an agent's automation permissions**: if reading a permission request was cancelled (for example while leaving a page or switching to something else), the agent's automation grants could be revoked as if it had lost access. A cancelled read now abandons only that request and leaves the grants, and the files they use, alone.

Upgrade: update the App.

**中文**

- **Windows 版的应用图标换成蘑菇本身**：Windows 上图标现在铺满整个方形——带一圈细奶油边的蘑菇，和托盘图标一致——不再是奶油色方块上缩小一截的蘑菇（在浅色任务栏上方块会看不见）。Mac 图标保留原来的方块。
- **被打断的请求不会再收走 agent 的自动化权限**：如果读取权限请求的过程被取消（比如离开页面或切换到别的内容），agent 的自动化授权可能被当成失去访问权限而收回。现在取消的读取只放弃这一次请求，不动授权和授权用到的文件。

升级：更新应用即可。

## v0.22.13

- **The recorder looks and works like BrowserWing, and can capture one part of the page**: actions are listed in recording order with a status line for the latest one, deleting a step asks inside the panel, hovering outlines top-level elements, and **Extract data** offers text, HTML or an attribute — also by right-clicking an element. **Region screenshot** lets you drag a rectangle in the page; the panel and highlights hide while you do, and that drag never becomes a recorded step. This needs browser component 0.4.0, which the App offers to install or update; an installed 0.3.x keeps working until you update, without region screenshots.
- **Recording no longer treats ad or embedded-frame jumps as steps**: only navigations you start in the top-level page are recorded, so redirects inside iframes (ads, widgets) no longer add steps of their own.
- **A page open in another myco App says so**: opening a page that another copy of the App is using now says "This page is open in another myco App. Quit that App, then try again" instead of a generic error. Quit that App rather than only closing its page or window, since the page's local address stays in use.
- **CodeBuddy, Qoder and Qwen sign-in is detected properly**: when a configured account can be seen, the CLI shows as ready; when the sign-in cannot be told, it now shows **Installed** with an optional **Go to sign in** instead of guessing "Sign in", so a CLI that already works needs nothing. A local agent that finished a task successfully in the last 24 hours also counts as available.
- **A task on hold resumes when what it waited for is done**: inside a runner, an agent can hold a task until a delegated task ends (`myco message hold --until`), and it resumes with that task's reply or failure. A hold left over from before an upgrade or restart is requeued once after 30 minutes with a progress note; intentional manual holds and tasks waiting on a question stay put. The App now shows held tasks separately as **Held**.
- **The plan of the task at work is pinned at the top of the chat**: while a task has a plan (Claude Code's to-do list, Codex's plan, a checklist), a bar under the title shows "Plan 2/5 · Doing: …" with how far it is; a click opens the whole list, and × unpins it for that task. Once everything is done it says so for a moment and goes.
- **Handoff cards are quieter**: on cards for work between agents, only **Working** keeps the accent (with a small breathing dot); Done, Handed back, Waiting and Queued are grey and a finished card's words go quiet, while Trouble keeps its colour. An exchange opened from a card in a chat gets a back button to that chat.
- **Pages can keep their own files on the Hub**: a page can now store attachments of its own, separate from project files, following the page's permissions; they do not appear in project files, project pull or an agent's project context. Unlinked uploads expire after 24 hours; defaults are 1,000 files, 2 GiB in total and 20 MiB each per page. The CLI has `myco page file put/list/get/remove/migrate`, and the Hub automatically migrates project files that page records reference. This needs a Hub upgrade to use; an App with an older Hub works as before, and the media kit switches over later.

- **Voice calls and dictation know the words to listen for**: the workspace's agents, projects and members, common words (Claude, Codex, GitHub, PR, CI, DeepSeek…) and your own, added under Settings › Voice › Words to hear right, are passed to the recognizer as hints, and only those words are corrected afterwards — casing ("github" → "GitHub"), words said apart ("Deep Seek"), and how a Chinese recognizer writes them ("克劳德", a lone "cloud" → "Claude"); English words next to other English are left alone.
- **Agents learn structured replies as they join**: an agent's joining notes and the runner's first project context now show when to reply with a myco-ui block — test results, changed files, release checks, comparisons, key numbers, plan steps — and that ordinary short answers stay text, so the App gets cards instead of a wall of text.
- **A workspace removed during a refresh no longer comes back**: removing a workspace closes its views at once and reads that were already running cannot restore it; leaving the usage page, switching workspaces or changing its filters cancels the outstanding usage request.
Upgrade: update the App. The recorder changes need browser component 0.4.0, which the App offers to install or update. Page attachments need a Hub upgrade to use; nothing breaks without it.

**中文**

- **录制面板改成 BrowserWing 的样子，还能只截页面的一部分**：动作按录制顺序列出，最新一步有一行状态提示；删除步骤会在面板里再确认；鼠标悬停会框出顶层元素；「提取数据」可以取文字、HTML 或某个属性，右键元素也能选。**区域截图**可以在页面里拖一个矩形，选择时面板和框选提示会隐藏，这次拖动也不会变成录制步骤。这需要浏览器组件 0.4.0，应用会提示安装或更新；已安装的 0.3.x 在更新前继续可用，只是没有区域截图。
- **录制不再把广告和嵌入框的跳转记成步骤**：只记录你在顶层页面发起的导航，iframe（广告、挂件）里的跳转不再单独产生步骤。
- **页面在另一个 myco App 里打开时会说清楚**：如果另一个 App 副本正占着这个页面，现在会提示「这个页面正在另一个 myco App 里打开，请先退出另一个 App 再试」，而不是笼统报错。请退出那个 App，只关它的页面或窗口不行，因为页面的本机地址仍被占用。
- **CodeBuddy、Qoder、Qwen 的登录状态能正确识别**：能看到已配置的账号时显示为可用；看不出登录状态时显示「已安装」，可选「去登录」，不再误显示成「登录」，本来就能用的不用做任何事。本机 agent 在最近 24 小时内成功跑过任务也算可用。
- **暂挂的任务会在等的条件满足后自动恢复**：在 runner 里，agent 可以先把活派给别的 agent，再用 `myco message hold --until` 暂挂原任务，等那个任务结束（成功或失败）后带着结果恢复。升级或重启前留下的旧暂挂，30 分钟后会带一条进展说明重新排队一次；人为的手动暂挂和还在等答复的任务不会动。App 里把暂挂任务单独显示成「暂挂」。
- **在做的任务的计划固定在聊天顶部**：任务有计划时（Claude Code 的待办、Codex 的计划、清单），标题下方用一行显示「计划 2/5 · 正在：……」，带一条进度细线；点击展开完整列表，× 可以对该任务取消固定。全部完成后会提示一下就收起。
- **交接卡片安静下来了**：agent 之间的工作卡片上，只有「进行中」保留强调色（带一个轻微呼吸的小点）；「完成」「已交回」「等待」「排队」都是灰色，完成卡片的文字也变淡，「出问题」保留原来的颜色。从聊天里的卡片打开往来记录时，多了一个返回那个聊天的按钮。
- **页面可以在 Hub 上存自己的文件**：页面现在能存自己的附件，和项目文件分开，权限跟随页面；不会出现在项目文件、项目拉取或 agent 的项目上下文里。没被记录引用的上传 24 小时后过期；默认每页 1,000 个文件、总共 2 GiB、单个 20 MiB。CLI 有 `myco page file put/list/get/remove/migrate`，Hub 会自动把页面记录引用到的项目文件迁移过来。这需要升级 Hub 才能用；旧 Hub 上应用照常工作，素材库之后切换。

- **语音通话和听写知道该听哪些词**：工作区里的 agent、项目和成员，常用词（Claude、Codex、GitHub、PR、CI、DeepSeek……）以及你自己在「设置 › 语音 › 要听对的词」里加的词，会作为提示交给识别器，之后只对这些词做纠正——大小写（「github」→「GitHub」）、分开说的词（「Deep Seek」），以及中文识别器写出来的样子（「克劳德」、单独一个「cloud」→「Claude」）；和其他英文连在一起的英文保持原样。
- **agent 加入时就知道怎么发结构化回复**：agent 的加入说明和 runner 第一次给的项目上下文里，现在会说明什么时候该用 myco-ui 块回复——测试结果、改动的文件、发版检查、对比、关键数字、计划步骤——以及普通的简短回答继续用文字，这样应用里看到的是卡片而不是一大段文字。
- **刷新期间删除的工作区不会再回来**：删除工作区会立刻关掉它的各个视图，删除前已经开始的读取也无法把它恢复；离开用量页、切换工作区或改它的筛选条件时，会取消还没返回的用量请求。
升级：更新应用即可。录制改动需要浏览器组件 0.4.0，应用会提示安装或更新。页面附件需要升级 Hub 才能用，不升级也不影响现有功能。

## v0.22.12

- **Recording no longer picks up scrolling you did not make**: the browser recorder adds a scroll step only after a deliberate wheel gesture, a page-navigation key (Page Up/Down, Home/End, space) or a scrollbar drag in that area, and waits for a short quiet moment after other actions. Clicking, typing, focusing, layout changes and scrolling done by the page itself no longer create steps of their own, so a recording replays the scrolls you actually made.
- **Closing a browser stops all of it, and a crash no longer keeps the profile locked**: on macOS and Linux every browser myco opens runs in its own process group, so closing it — or quitting the App — stops the whole group instead of leaving helper processes behind with the profile. After a crash, the next start verifies the leftover processes and cleans them up before opening the profile again, and never signals a process it cannot verify as its own.
- **A profile that is still in use says why**: when a previous browser still holds a profile, the page now receives a specific reason — a leftover it could not verify, cleanup still running, or cleanup failed — and can tell you to close the earlier window and try again, or restart the computer, instead of showing a generic error.

Upgrade: this App release needs no Hub upgrade. The recorder change needs browser component 0.3.1, which the App offers to install or update; an installed 0.3.0 keeps working until you update.

**中文**

- **录制不再带上你没做的滚动**：浏览器录制只在你有明确意图时才加滚动步骤——滚轮、翻页键（Page Up/Down、Home/End、空格），或在那个区域拖动滚动条，并且会等其他动作安静一小会儿再记。点击、输入、聚焦、页面布局变化和网页自己触发的滚动都不再单独产生步骤，回放的就是你真正做过的滚动。
- **关浏览器会把整组进程都停掉，崩溃后也不再占着配置**：macOS 和 Linux 上，myco 打开的每个浏览器都在自己的进程组里运行，关闭它（或退出应用）会停掉整组进程，不再留下占着配置的辅助进程。崩溃后，下次启动会先核对残留进程并清理，再打开配置；核对不了的进程一律不动。
- **配置还被占用时会说明原因**：之前的浏览器还占着配置时，网页现在会收到具体原因——无法核对的残留、正在清理、清理失败——可以据此提示你先关掉之前的窗口再试，或重启电脑，而不是只给一个笼统的错误。

升级：本次应用发布不需要升级 Hub。录制改动需要浏览器组件 0.3.1，应用会提示安装或更新；已安装的 0.3.0 在更新前继续可用。

## v0.22.11

- **Record a browser workflow inside Chrome**: every browser myco opens for a profile now shows a small recorder in the page corner. Start recording, review the recent steps, delete any of them, or capture an element's text, an attribute or its HTML as an extract step; the panel hides itself while a run or a debugging action uses the browser. A recording started in the panel is also listed for the app, so Web automation can pick it up and continue from the page. This needs browser component 0.3.0, which the App offers to install or update; an installed 0.2.0 keeps working until you update.
- **A page opens in a window of its own, and installs as an app on macOS**: a page's menu can open it in a separate window that shows only that page; on macOS you can also install it, which adds a launcher with the page's icon to `~/Applications/myco 应用`, so Launchpad, Spotlight and the Dock start it like an app.
- **The voice call is the agent itself**: it now speaks in the first person — "Sure, I'm on it", not "I'll tell it" — and its caption lines show its small face instead of a "Voice" label. The microphone, speech-recognition, cut-off and setup notices follow the call's language.
- **A voice call starts with the agent saying hello**: when the call connects, the agent speaks first with a short line, mentioning work in progress or a question it is waiting on; speaking over it cuts it off like any turn.
- **Chrome no longer shows an unsupported-flag warning**: myco's browsers start without experimental Blink switches, so the warning bar above the page is gone. Chrome keeps its native user agent, with no stealth scripts; sites can still ask a person to interact.
- **A capability allowed for a page now follows its code**: when a page is republished or its app is upgraded, the new code is asked again before it can use what you allowed earlier. A "no" still holds for every version, and allows saved before this release are asked once more.
- **Official apps declare the computer capabilities they use**: the bundled apps are updated to versions whose manifests list their computer capabilities, so a Hub that holds apps to their declarations lets them use those capabilities without asking each time.
- **A runner upgrade no longer makes the computer look offline for minutes**: restarting the runner, for example during `myco upgrade`, now stops within about 30 seconds instead of up to ten minutes; while it restarts the App shows **Updating** instead of **Away**, and a task it was running is queued again once.
- **A long thing an agent says in a run fades out instead of being cut**: text between steps now fades at the bottom with **Show all** under it, and is held short only when it actually runs past the visible area, so it no longer looks like text was lost.
- **Files picked for a chat stay with that chat**: a picture or file added to a chat's box but not sent yet stays with that chat (or the group box) while the App runs, as typed words do; switching to another page and back no longer loses it, and one still uploading finishes.
- **Requests waiting for your OK are decided one at a time**: approving one no longer greys out the others until the workspace answers; a decided card goes at once, the list is read again behind, and errors show on the card they belong to.
- **Your decision shows in the chat as one quiet line**: the note the hub sends an agent about your approval now reads in words, such as "You approved: create the scheduled task "x" in project "p"", instead of the raw code-like text.

Upgrade: this App release needs no Hub upgrade. The browser recorder needs browser component 0.3.0, which the App offers to install or update.

**中文**

- **可以直接在 Chrome 里录制浏览器流程**：myco 为某个资料打开的浏览器，页面角落会出现一个小录制面板。可以开始录制、查看最近的步骤、删除任意一步，或把某个元素的文字、属性或 HTML 捕获成 extract 步骤；运行或原生调试占用浏览器时面板会自动隐藏。在面板里开始的录制，网页自动化页面也能发现，并可以直接接上继续处理。这需要浏览器组件 0.3.0，应用会提示安装或更新；已安装的 0.2.0 在更新前继续可用。
- **页面可以单独开一个窗口，macOS 上还能装成应用**：页面菜单里可以把页面放到单独窗口里只显示它；macOS 上还可以安装，在 `~/Applications/myco 应用` 里生成带页面图标的启动器，Launchpad、聚焦和程序坞都能像应用一样打开。
- **语音通话里说话的就是 agent 本人**：现在用第一人称说话——「好，我这就去做」，而不是「我去告诉它」；字幕行前面显示它的小头像，不再是「语音」标签。麦克风、语音识别、被打断和设置提示都跟着通话语言。
- **语音通话接通后 agent 先打招呼**：电话接通时，agent 先用一句简短的话开口，说说手上的活或正在等的问题；你直接开口就会像普通对话一样打断它。
- **Chrome 顶部不再出现不支持参数的警告**：myco 打开的浏览器不再带实验性 Blink 参数启动，页面上方的警告条没有了。Chrome 仍使用原生 UA，没有伪装脚本；网站仍可能要求人手动操作。
- **给网页的能力允许现在只对当时的代码有效**：网页重新发布、或者它的应用升级以后，新代码再用到你之前允许的能力时会重新询问。「不允许」对所有版本都有效；这次发布之前保存的允许会再问一次。
- **官方应用补上了会用到的电脑能力声明**：内置应用更新到在清单里声明电脑能力的版本；对按声明校验应用的 Hub，这些应用使用已声明的能力时不再逐次询问。
- **runner 升级时电脑不再离线好几分钟**：重启 runner（比如 `myco upgrade` 时）现在约 30 秒就停好，不再可能卡到十分钟；重启期间应用显示「正在更新」而不是「不在线」，正在跑的任务会重新排队一次。
- **agent 在运行过程中说的一长段话改成淡出，不再硬截断**：步骤之间的文字现在在底部淡出，下面有「展开全文」，而且只有内容真的超出可视区域时才收起，不会再看起来像丢了文字。
- **放进聊天输入框的文件会留在那个聊天里**：为某个聊天（或群聊框）选好但还没发送的图片或文件，在应用运行期间会一直留在那里，和打好的字一样；切到别的页面再回来不会丢，正在上传的也会传完。
- **待你确认的请求现在逐个处理**：批准一个请求后，其余请求不会一起变灰等后台回应；已处理的卡片会立即消失，列表在后台重新读取，出错时错误显示在对应的卡片上。
- **你的决定在聊天里显示成一句安静的话**：Hub 告诉 agent 你批准了哪件事的那条提示，现在用一句话显示，例如「你同意了：创建项目「p」的定时任务「x」」，不再是带编号的原始文本。

升级：本次应用发布不需要升级 Hub。浏览器录制需要浏览器组件 0.3.0，应用会提示安装或更新。

## v0.22.10

- **A browser settings change is allowed only after the whole list is read**: the confirmation now shows every proxy, bypass rule, user agent, language and launch argument in a monospace layout that also fits narrow windows, and **Allow** stays disabled until all of them have been scrolled into view. Declining or closing the dialog changes nothing.
- **An Android preview APK is attached to the release**: `myco-android-debug.apk` is the Android app's CI debug build, provided next to the desktop downloads. It is a preview for trying the Android app, not a signed store release: it is debug-signed, its signature can differ between releases, so uninstall an older APK before installing a new one.

Upgrade: this App release needs no Hub upgrade. Update the App, and for browser profile settings use browser component 0.2.0, which the App offers to install or update.

**中文**

- **浏览器设置的确认框要看完才能允许**：确认框会列出代理地址、不走代理的地址、User-Agent、语言和每个启动参数，这些值用等宽字体显示，窄窗口下也能完整排版；在所有内容都滚动显示过之前，「允许」按钮保持不可用。拒绝或关闭确认框不会有任何改动。
- **发布中附带 Android 预览 APK**：`myco-android-debug.apk` 是 CI 构建的 Android 应用 debug 版，和桌面版安装包一起提供。它只是用来试用 Android 应用的预览版，不是签名的商店版本：它使用 debug 签名，不同版本之间的签名可能不同，安装新版本前请先卸载旧的 APK。

升级：本次应用发布不需要升级 Hub。更新应用即可；使用浏览器资料设置需要浏览器组件 0.2.0，应用会提示安装或更新。

## v0.22.9

- **A browser profile can use a proxy, custom user agent, language and launch arguments**: Web automation can ask to change these for a profile, and the App shows its own confirmation with the exact proxy, bypass list, user agent, language and arguments before anything changes; a proxy password is never displayed, and a saved one is kept only when the server and user stay the same. Settings apply the next time the browser starts and do not restart a browser that is open. This needs browser component 0.2.0, which the App offers to install.
- **An installed browser component now shows when it can be updated**: the browser status a page sees reports a compatible but older component, so Web automation shows "Update component" on a computer still running 0.1.1 or 0.1.0 instead of only reporting the component as installed.
- **Project storage is counted and limited separately**: a project's files and total size no longer share the context-document allowance (still 100 documents), and a workspace owner or admin can set a project's own limits in Settings → Workspaces — 1 to 10,000 files, up to 20 GiB, defaults 1,000 files and 2 GiB — or return it to the workspace defaults. Lowering a limit never hides or deletes existing files; new uploads must fit. This part needs Hub 0.22.9 on the server, which also runs a database migration on upgrade.
- **Apps may only use what they declare**: on a Hub that says an app's manifest is complete, an operating-system capability the app does not declare in `myco.json`, or `myco.send`/`myco.download` without the matching permission, is refused with the group to add instead of asking the person each time; a file the person picks is no exception. Public network hosts outside the app's `net` list are still asked one by one, and pages with no app, myco's own apps that declare nothing yet, and older Hubs keep the previous behaviour. This part needs Hub 0.22.9.

Upgrade: update the Hub on the server to 0.22.9 (`sudo myco upgrade`) for project storage limits and app-declaration enforcement. The browser changes only need the new browser component, which the App installs or offers to update.

**中文**

- **浏览器资料可以使用代理、自定义 User-Agent、语言和启动参数**：网页自动化可以申请修改某个资料的这些设置，应用会先用自己的确认框列出确切的代理地址、不走代理的列表、UA、语言和每个启动参数，确认后才生效；代理密码从不显示，只有服务器和账号都没变时才保留已保存的密码。设置在下次启动浏览器时生效，不会重启已经打开的浏览器。这需要浏览器组件 0.2.0，应用会提示安装。
- **装了旧版浏览器组件时页面会提示更新**：页面看到的浏览器状态会说明装的是可兼容但较旧的组件，还停留在 0.1.1 或 0.1.0 的电脑上，网页自动化会显示「更新组件」，而不只是显示已安装。
- **项目存储单独计算、上限可配置**：项目的文件数和总大小不再占用上下文文档的额度（文档仍限 100 个），工作区所有者或管理员可以在「设置 → 工作区」里为单个项目设置上限——1 到 10,000 个文件、最多 20 GiB，默认 1,000 个文件和 2 GiB——也可以恢复为工作区默认值。调低上限不会隐藏或删除已有文件，只影响新的上传。这部分需要服务器上的 Hub 0.22.9，升级时会执行数据库迁移。
- **应用只能使用声明过的能力**：如果 Hub 说明某个应用的清单就是它的全部声明，那么 `myco.json` 的 `"os"` 里没有的操作系统能力，或 `"permissions"` 里没有的 `myco.send`/`myco.download`，会直接拒绝并提示要补哪一组，不再每次询问；人自己选的文件也不例外。应用 `"net"` 之外的公网地址仍会逐个询问；没有清单的普通页面、暂时没写声明的 myco 官方应用和旧 Hub 保持原来的行为。这部分需要 Hub 0.22.9。

升级：为了让项目存储额度和应用声明拦截生效，请把服务器上的 Hub 升级到 0.22.9（`sudo myco upgrade`）。浏览器相关的改动只需要新的浏览器组件，应用会安装或提示更新。

## v0.22.8

- **Web automation records inside cross-origin frames**: clicks and typing in a page's cross-site iframes, including frames nested inside them, are recorded with a selector path that replays on the right frame. Pages whose verification widget loads in a nested frame, such as Cloudflare Turnstile on V2EX, no longer stay stuck while the recorder attaches.
- **Browser component 0.1.1**: the app now installs the new component. A component already installed as 0.1.0 keeps working until you choose "Update component" in the app; the update is verified before the old process stops, so a failed download or checksum check leaves the current component, its browser sessions and profiles untouched. Updating does interrupt sessions still using the old process, so finish recording or playback first.
- **Agents' browser scripts are approved where they run**: when an agent asks to run a script, the computer's owner sees the exact version, every URL it visits, its executable code, its sub-scripts and the full snapshot before allowing. Approval belongs to that App setup, workspace, account, page and browser profile, and the agent still needs its own short-lived grant. A paused run waits at most 15 minutes and never past the grant, and results and screenshots stay private files.
- **A resumed browser run keeps reporting**: after you confirm or start a paused run it goes on reporting steps and results instead of stopping at the first acknowledgement, and a pause with no new events no longer ends the job.
- **Search shows the mushroom looking**: while ⌘K has not found anything yet, the pixel mushroom paces with a magnifying glass and names the words being searched; message search also no longer starts over on every overview refresh, which could keep it searching forever on a slow network.
- **Context menus stay open while another pane scrolls**: a background chat scroll or scrolling inside the menu no longer closes it; only scrolling the pane the menu came from does.

Upgrade: this App release needs no Hub upgrade. Agents using a computer's browser through the Hub still need Hub 0.22.7 on the server: run `sudo myco upgrade` there.

**中文**

- **网页自动化可以录制跨源 frame**：页面里跨站的 iframe（包括嵌套在其中的 frame）中的点击和输入也会被录下来，选择器路径能在正确的 frame 上回放。验证组件加载在嵌套 frame 里的页面，例如 V2EX 的 Cloudflare Turnstile，不会再因为录制器附加而卡住。
- **浏览器组件 0.1.1**：应用会安装新的组件。已经装了 0.1.0 的照常可用，在应用里点「更新组件」即可升级；更新会先校验再停掉旧进程，下载或校验失败时现有组件、浏览器会话和资料都不受影响。更新会中断仍在使用旧进程的会话，请先结束录制或回放。
- **agent 的浏览器脚本在运行它的电脑上批准**：agent 申请运行脚本时，电脑主人会先看到确切的版本、它访问的每个网址、可执行代码、子脚本和完整快照，然后再决定是否允许。批准只属于这个 App 的配置、工作区、账号、页面和浏览器资料，agent 仍需单独申请短期授权。暂停的运行最多等 15 分钟，且不会超过授权到期；结果和截图只作为私有文件返回。
- **恢复后的浏览器运行继续上报**：确认或启动暂停的运行后，它会继续上报步骤和结果，不再在收到第一条确认后就停下；没有新事件的暂停心跳也不会结束任务。
- **搜索时有蘑菇在找**：⌘K 还没搜到结果时，像素蘑菇拿着放大镜来回找，并显示正在搜索的词；消息搜索也不再随每次总览刷新重新开始，慢网络下不会一直搜不完。
- **别的面板滚动时右键菜单不会关闭**：后台聊天自动滚动或在菜单内部滚动不再关掉菜单，只有菜单来源所在的面板滚动才会关闭。

升级：这个 App 版本本身不需要升级 Hub。让 agent 通过 Hub 使用某台电脑的浏览器仍需要服务器上的 Hub 0.22.7（`sudo myco upgrade`）。

## v0.22.7

- **Web automation records and replays a workflow in a dedicated browser profile**: the official Web automation app can record the clicks, typing and navigation you make in a visible browser window, then replay that sequence step by step or in one run, with a breakpoint or a confirmation before anything consequential. It needs the optional browser component, which the app installs from its own dialog on Mac (Apple silicon and Intel) and Windows; the component never downloads a browser or uses your everyday profile, and profiles, cookies, passwords and downloads stay on this computer.
- **Agents can use a computer's browser through the Hub**: a computer's owner can approve a short-lived grant for a dedicated browser profile, and an agent can then list that profile, run a script version the owner reviewed, and check, pause or cancel the run (`browser.profiles.list`, `browser.script.run`, `browser.run.status/resume/cancel`). Passwords and other secret parameters are supplied for each run; screenshots and results come back as private files. This needs the Hub at 0.22.7.
- **Older local files appear only when there is something to recover**: the page recovery entry is shown only for an app that actually has old data, instead of on every page.
- **Page records can be queried on the Hub**: the CLI and the page bridge can ask for a filtered, paged view of a page's collection instead of loading every record; an older Hub keeps answering with the full collection and the client filters it itself.
- **Replies type out as they are written**: text and thinking arrive piece by piece with a caret; a run's whole process folds into one line (process · 32 steps · 12 minutes), opens while it runs and folds once the answer is in; plan items fade in as they are added; Enter adds to your newest open task.
- **The App guide links to Telegram and Discord**: the README and the in-app guide's Community page now list the Telegram and Discord groups.
- **Android**: appearance settings persist across restarts, and a page can save its downloads to the device with the system save dialog.

Upgrade: this App release needs no Hub upgrade for recording and replaying, the recovery entry, page queries or the interface changes. Agents driving a computer's browser through the Hub need Hub 0.22.7: on the server run `sudo myco upgrade`, then update the runner/CLI on each computer.

**中文**

- **网页自动化在专用浏览器资料里录制和回放**：官方「网页自动化」应用可以在一个看得见的浏览器窗口里录制你的点击、输入和跳转，然后逐步或整段回放这个流程；在提交等有后果的步骤前可以加断点或确认。它需要可选的浏览器组件，应用会从自己的确认对话框里安装（Mac 的 Apple 芯片和 Intel、Windows）；组件不会下载浏览器，也不会使用你日常的资料，资料、Cookie、密码和下载文件都留在这台电脑上。
- **agent 可以通过 Hub 使用某台电脑的浏览器**：电脑主人可以为专用的浏览器资料批准一个短期授权，agent 随后可以列出这个资料、运行主人审阅过的脚本版本，并查看、暂停或取消这次运行（`browser.profiles.list`、`browser.script.run`、`browser.run.status/resume/cancel`）。密码等秘密参数在每次运行时单独提供；截图和结果作为私有文件返回。这项功能需要 Hub 0.22.7。
- **旧本机文件只在确实有可恢复数据时出现**：页面恢复入口只对存在旧数据的应用显示，不再出现在每个页面上。
- **页面记录可以在 Hub 上查询**：CLI 和页面桥可以按条件、分页读取一个页面的集合，不必把全部记录都加载下来；旧 Hub 仍返回完整集合，由客户端自己筛选。
- **回复随写随出**：文字和思考会带着光标一段段出现；一次运行的全部过程折叠成一行（过程 · 32 步 · 12 分钟），运行中展开、拿到回答后折叠；计划项加入时淡入；回车会追加到你最新的未完成任务的下面。
- **App 指南加了 Telegram 和 Discord**：README 和 App 内指南的「社区」页现在列出 Telegram 和 Discord 群。
- **Android**：外观设置（如主题）重启后保留，页面可以用系统的保存对话框把下载文件存到本机。

升级：这个 App 版本本身不需要升级 Hub 就能录制回放、使用恢复入口、页面查询和界面改动。让 agent 通过 Hub 使用某台电脑的浏览器需要 Hub 0.22.7：在服务器上运行 `sudo myco upgrade`，然后更新每台电脑上的 runner/CLI。

## v0.22.6

- **Each page keeps its own local storage**: in the desktop App every workspace, account and page now opens on its own local address, so what a page saves (a draft, a sign-in, an app's data) is no longer visible to another page or another workspace. A page keeps that address across App restarts, so it reopens with its data.
- **Older local page data is not lost**: the previous shared address now opens a read-only "Older local files" page that lists what old pages left behind; "Save copy" downloads a file, and older page settings can be exported. Nothing is deleted.
- **Apps can keep an encrypted vault in the App**: an app that needs isolated page storage, such as Password Vault 1.2.0 from the apps catalog, opens only on its own page origin and shows "Update the myco App" on an older App or a shared address, with no password field.
- **Project coordinators can invite agents**: a coordinator can create a join link for one ordinary agent (`myco project invite-agent`), valid for 24 hours; the agent joins the project owned by the project owner and gets no member-management or invitation rights. Whether coordinators may invite is a per-project switch, off by default: until the owner enables it, each request arrives as an approval card.
- **Project coordinators can schedule tasks**: a coordinator can create cron automations bounded by start and end dates (`myco automation create --project ... --cron ... --starts ... --ends ...`, or `--once`). Scheduling is off by default, so creating, changing, pausing or deleting a task asks the project owner to approve that change; with it on, coordinators apply changes directly and the tasks still belong to the owner. The App editor uses local times and shows the active dates and why a task is disabled.
- **Waiting tasks are visible**: the App shows “Queued: this computer has 3/3 slots in use, position 2”, and computer details show the limit and the waiting count. Workspaces share free slots fairly instead of one busy workspace taking every new slot; running tasks are never interrupted.
- **A line added to a running task is answered under it**: the steps, progress and loading that follow your message now appear below that line, not above it.
- **Working counts follow agents**: project dots, the notch island and the menu bar's working line count agents only; a shell node running commands no longer lights whole projects and keeps its own dot under Computers.

Upgrade: this release needs the Hub updated as well. On the server run `sudo myco upgrade`, then update the App and the runner/CLI on each computer. Invitations, scheduling and the queue view are Hub-side features, and the isolated page addresses and the vault need Hub 0.22.6 with this App. Older Hubs keep accepting runners, but these features stay unavailable until the Hub is upgraded.

**中文**

- **每个页面有自己的本机存储**：桌面 App 里每个工作区、账号和页面现在都在各自的本机地址上打开，一个页面保存的内容（草稿、登录状态、应用数据）不会再被另一个页面或另一个工作区看到。App 重启后页面仍用同一个地址，所以数据还在。
- **旧的页面数据不会丢**：原来的共享地址现在打开一个只读的「旧本机文件」页面，列出旧页面留下的文件；「保存副本」可以下载文件，「导出旧页面设置」可以导出设置。不会删除任何东西。
- **应用可以把加密密码库放在 App 里**：需要独立页面存储的应用（例如应用目录里的密码库 1.2.0）只在自己的页面来源上打开；旧版 App 或共享地址会显示「请升级 myco App」，不显示密码输入框。
- **项目协调者可以邀请 agent**：协调者可以为一个普通 agent 生成加入链接（`myco project invite-agent`），24 小时内有效；agent 以项目所有者名下的普通成员加入，没有成员管理或邀请权限。是否允许协调者邀请是每个项目的开关，默认关闭：所有者打开前，每次请求都会变成一张审批卡。
- **项目协调者可以安排定时任务**：协调者可以创建带起止日期的 cron 自动化（`myco automation create --project ... --cron ... --starts ... --ends ...`，或 `--once`）。定时默认关闭，所以新建、修改、暂停或删除任务都要项目所有者批准这次改动；打开后协调者可以直接改，任务仍归所有者。App 编辑器用本地时间，并显示生效日期和任务被停用的原因。
- **等待中的任务看得见**：App 显示「排队中：这台电脑 3/3 个位置都在用，排第 2 位」，电脑详情显示上限和等待数量。多个工作区公平分享空出的位置，不再让一个忙的工作区占走所有新位置；正在跑的任务不会被打断。
- **在运行中的任务里插一句话，回复显示在它下面**：你发消息之后 agent 做的步骤、进度和加载现在显示在这句话下面，而不是上面。
- **“正在工作”只算 agent**：项目圆点、刘海岛和菜单栏的工作行只统计 agent；执行命令的 shell 节点不再点亮整个项目，而是在「电脑」下面保留自己的圆点。

升级：这个版本需要同时升级 Hub。在服务器上运行 `sudo myco upgrade`，然后更新 App 和每台电脑上的 runner/CLI。邀请、定时任务和排队显示是 Hub 侧的功能，独立页面地址和密码库需要 Hub 0.22.6 配这个 App。旧 Hub 仍接受 runner 连接，但升级前这些功能不可用。

## v0.22.5

- **Project changes show at once**: adding an agent to a project, or changing a project's name, members or computers, appears in the App immediately instead of waiting for every workspace to be read again; a new project opens and joins the sidebar the same way.
- **The connection notice lines up with the input box**: "网络不太好" and an unsent message now keep to the chat's column, as wide as the message box, instead of running across a wide window.
- **Official apps moved to github.com/myco-ai/apps**: application source, the page SDK and app releases now live in that repository. Each myco release bundles a pinned set of official apps whose bytes are checked against `apps.lock.json`; a Hub still fetches the rolling catalog from myco-ai/apps, and older Hubs keep receiving the identical apps from the old address through the compatibility mirror. Existing workspaces, pages and apps need no change.
- Upgrade: update the App for the two interface fixes. No hub upgrade is needed for the app move; older Hubs keep working through the mirror.

**中文**

- **项目改动立即显示**：往项目里加 agent，或修改项目名称、成员、电脑，App 立即显示，不再等把所有工作区重新读一遍；新建项目也会马上出现在侧栏并打开。
- **网络提示与输入框对齐**：「网络不太好」和没发出去的消息现在和消息框同宽、同列，不会在大窗口里横跨整个面板。
- **官方应用搬到 github.com/myco-ai/apps**：应用源码、网页 SDK 和应用发布现在都在这个仓库。每个 myco 版本会内置一组固定版本的官方应用，字节与 `apps.lock.json` 里的 SHA256 核对；Hub 仍从 myco-ai/apps 拉取滚动目录，旧 Hub 通过兼容镜像从老地址拿到完全相同的应用。已有的工作区、页面和应用不需要任何改动。
- 升级：更新 App 即可获得两处界面修复。应用搬家不需要升级 Hub；旧 Hub 通过镜像照常工作。

## v0.22.4

- **The repository moved to github.com/myco-ai/myco**: the old github.com/chenhg5/myco address redirects automatically, so existing App connections, update checks and `myco upgrade` on older versions keep working; new links and downloads use the new address.
- **Removing a workspace is instant**: the dialog closes as soon as this App has forgotten the workspace; it leaves the list, the overview and the projects at once, and a space or chat shown in it closes (the next workspace shows) instead of pages going on asking the removed one and showing errors. The full refresh runs behind.
- Upgrade: update the App for instant removal. Nothing else is needed for the move: the old address redirects and existing installs upgrade as before.

**中文**

- **仓库搬到 github.com/myco-ai/myco**：旧的 github.com/chenhg5/myco 地址会自动跳转，已有的 App 连接、更新检查和旧版本上的 `myco upgrade` 都照常工作；新的链接和下载使用新地址。
- **移除工作区是即时的**：只要这个 App 忘掉了该工作区，对话框就关闭；它同时从列表、概览和项目里消失，正在显示它的空间或对话也会关闭（切换到下一个工作区），不会再出现页面去问已移除的工作区而报错。完整刷新在后台进行。
- 升级：更新 App 即可获得即时移除。仓库迁移不需要其他操作：老地址会自动跳转，已有安装照常升级。

## v0.22.3

- **素材库 finds the right picture, and opens it large**: filter the library by language, theme, platform and orientation, alone or together with the search box; click a picture to see it larger and step through the set. Uploading now asks for a one-line description, language and theme for each file, which is what the filters and search use.
- Upgrade: the official apps reach the workspace by themselves (or `myco template sync` fetches them now), and an existing 素材库 page takes the new version with `myco page upgrade`. No hub or CLI upgrade is needed.

**中文**

- **素材库更好找图，也能看大图**：可以按语言、主题、平台和横竖方向筛选，也可以和搜索框一起用；点开图片看大图，并能在当前套图里前后翻。上传时每个文件会要求填一句画面描述、语言和主题，筛选和搜索用的就是这些信息。
- 升级：官方应用会自动同步到工作区（也可以用 `myco template sync` 立即获取），已有的素材库页面用 `myco page upgrade` 升级到新版本。不需要升级 Hub 或 myco。

## v0.22.2

- **A page asks before it uses your computer**: the first time a page calls a group of capabilities (files, writing files, deleting files, commands, the screen, the microphone, speech, the clipboard, notifications, the system, the network, SSH, databases, the proxy), the App asks you: don't allow, allow this time, or always allow. Commands, deleting files and the screen are only ever allowed this time; files you picked and writing the clipboard need no question. A page reaches the network hosts its app declares with the network's consent and is asked for any other host one by one. The answer is kept per workspace and page.
- **A page can be read-only for everyone else**: its managers choose in the page card's menu, or with `myco page set --page <id> --editors managers`, whether everyone who sees it may change its records and shared state (as before) or only the publisher, their agents and the workspace's managers. Others see "you can only view"; a template can make one collection managers-only too.
- **A task's question reaches an agent sender**: when the task came from an agent, `myco message ask` now queues the question to its inbox instead of storing it as a closed progress update, so a session waiting there or its runner wakes and answers, and the task no longer sits held.
- **New app: 素材库 (Media Kit)**: screenshots, video files and links, with Chinese and English copy organized by feature, the originals and each entry's release status shared with the team and agents.
- **Usage moved into the workspace menu**: it is looked at now and then, so it leaves the top of the sidebar and joins the workspace menu at the bottom, marked while its page is open; that menu now grows to fit its workspaces and items without scrolling.
- **The user guide moved into Settings**: it leaves the workspace menu and becomes the last row of Settings' navigation (Settings › About still opens it too).
- **iOS**: the phone gets the desktop changes to list screens, Markdown and UI blocks; media streaming is verified end to end.
- Upgrade: update the hub to v0.22.2 for page editors and for questions reaching agent senders; update myco on computers that run `myco page set` or send `myco message ask`; update the App for the consent prompts, view-only pages, the new app and the moved Usage and user guide.

**中文**

- **网页用电脑之前先问你**：网页第一次调用某一组能力（读文件、写文件、删文件、执行命令、屏幕、麦克风、语音、剪贴板、通知、系统、网络、SSH、数据库、代理）时，App 会先问：「不允许」「这次允许」「一直允许」。执行命令、删文件和屏幕只能「这次允许」；你自己选的文件和写入剪贴板不用问。网页访问应用声明的网络主机时随网络这一组一起允许，其他主机逐个询问。回答按工作区和网页分别记住。
- **网页可以对其他人只读**：网页的管理者可以在页面卡片的菜单里，或运行 `myco page set --page <id> --editors managers`，决定是所有能看到的人都能改它的记录和共享状态（和以前一样），还是只有发布者、其名下的 agent 和工作区管理员能改。其他人会看到「只能查看」；模板也可以让某一个集合只有管理者能改。
- **任务的提问能叫醒 agent 发起方**：任务由 agent 发出时，`myco message ask` 现在把提问放进它的收件箱，而不是存成一条已关闭的进度；在那里等待的会话或它的 runner 会被叫醒并回答，任务不会再一直挂起。
- **新应用：素材库**：按功能整理截图、视频文件和链接，配上中英文配文，原图和每条的发布状态与团队及 agent 共享。
- **用量入口移到工作区菜单**：用量只是偶尔看，所以从侧栏顶部移到工作区菜单底部，打开它的页面时旁边有标记；这个菜单现在会随工作区和条目变高，不用滚动。
- **使用文档移到设置里**：它从工作区菜单移出，成为设置导航的最后一项（设置 › 关于 里也仍然能打开）。
- **iOS**：手机上也跟上了桌面的列表页、Markdown 和 UI 块改动；媒体串流已完成端到端验证。
- 升级：把 Hub 升级到 v0.22.2 才能设置网页编辑权限、让提问送达 agent 发起方；需要运行 `myco page set` 或发送 `myco message ask` 的电脑更新 myco；更新 App 才能看到授权弹窗、只读提示、新应用，以及移动后的用量和使用文档入口。

## v0.22.1

- **Updates come from GitHub or Gitee, whichever is faster**: `myco upgrade`, the hub's self-upgrade and the App's update time both sources first and download from the faster one; if a copy is missing (Gitee keeps only the newest release's binaries) or fails its checksum, the other is used. The version check does the same: GitHub's API, then the Gitee mirror, then the release page. `MYCO_RELEASE_DOWNLOAD` and `MYCO_RELEASE_API` still pin a single source.
- **Member groups can be dragged into order** in the members column, with a line where the group will land. A member's menu has one 移到分组 row that opens the group list where the menu was, so many groups no longer make the menu long.
- **Runner slots are for agent CLIs only**: with `--max-concurrent`, shell commands and the machine node's own work (upgrading, starting an agent) no longer queue behind running agents.
- **Pages keep up and stay writable**: after a page was away and the change feed reset, it now hears the reset and lists its records again; records also keep a field that a template upgrade dropped writable, instead of refusing every save.
- Upgrade: update myco on computers that upgrade or run runner nodes, and the hub to v0.22.1 for the page fixes; update the App for group dragging and the update copy.

**中文**

- **更新从 GitHub 和 Gitee 里较快的那个下载**：`myco upgrade`、Hub 自升级和 App 内更新都会先测两边速度再下载；一边没有这个版本的文件（Gitee 只保留最新一版的二进制）或校验不过，就自动换另一边。版本检测也一样：先问 GitHub API，再问 Gitee 镜像，最后看 release 页面。设置 `MYCO_RELEASE_DOWNLOAD` 或 `MYCO_RELEASE_API` 时仍然只使用指定的源。
- **成员分组可以拖动排序**：在成员栏里拖动分组，落点处会显示一条线。成员菜单里的「移到分组」只占一行，点开后在菜单原来的位置列出分组，分组再多也不会把菜单撑得很长。
- **runner 的并发名额只算 agent CLI**：设置 `--max-concurrent` 后，shell 命令和机器节点自己的活（升级、启动 agent）不会再排在正在运行的 agent 后面。
- **页面不掉队、也能继续保存**：页面离开后变更过多、变更流重置时，现在能收到重置并重新拉取记录；模板升级去掉的字段也不会再让记录永远保存失败。
- 升级：需要升级或跑 runner 节点的电脑更新 myco；要修页面的问题，把 Hub 升级到 v0.22.1；更新 App 才能拖动分组并看到新的更新文案。

## v0.22.0

- **The Usage page is live**: totals, a day-by-day chart and members, projects and models ranked by tokens, for 7, 30 or 90 days, for the whole workspace or one project. The chart has a daily-average line and compares the last seven days with the seven before; tables rank by tokens with each row's share. It is laid out like the Automations page and fits beside the sidebar in a narrow window.
- **Token usage is now reported and kept**: updated runners and Claude sessions (`myco inbox wait`) report numeric token usage independently of traces, so it works with tracing off and in end-to-end encrypted workspaces. The hub stores per-day figures per agent, project and model. Members see their own agents; owners and admins see the whole workspace. Set `MYCO_NO_USAGE=1` to turn reporting off.
- **`myco usage show`**: `--by agent` (default), project, day or model, with `--from`, `--to` and `--project`, using member or admin credentials.
- **Member groups**: they are reachable from the folded members column too (the ⋯ under its unfold button), and moving a member between groups stays put while slow saves and refreshes cross; a failed save goes back and says why in the members column.
- Costs are present only when the CLI reports them: Claude Code uses Anthropic pricing, so figures for compatible proxies or other models are reference estimates. Older clients do not report usage and traces are not backfilled.
- Upgrade: update the hub to v0.22.0 for usage storage and the API, and update myco on computers that should report usage or run `myco usage show`. Update the App for the Usage page and the group fixes. No other hub upgrade needed.

**中文**

- **用量页正式可用**：显示总量、按天的柱状图和按 token 排名的成员、项目、模型，可以看 7、30 或 90 天，范围是整个工作区或单个项目。图表有日均虚线，并把最近 7 天和前 7 天做对比；表格按 token 排名并显示每行占比。版式与「自动化」页一致，窄窗口下也能放在侧栏旁边。
- **token 用量开始上报和保存**：更新后的 runner 和 Claude 会话（`myco inbox wait`）会独立于 trace 上报数字用量，关闭 trace 和端到端加密的工作区也不会漏报。Hub 按天保存每个 agent、项目和模型的数字。成员只能看自己的 agent，所有者和管理员可以看整个工作区。设置 `MYCO_NO_USAGE=1` 可以关闭上报。
- **`myco usage show`**：`--by agent`（默认）、project、day 或 model，支持 `--from`、`--to` 和 `--project`，用成员或管理员凭证查询。
- **成员分组**：成员栏折叠后也能进入分组（展开按钮下面的 ⋯）；在保存慢、刷新交叉时，成员在分组间移动后不会再跳回去；保存失败会退回并说明原因。
- 费用只有 CLI 自己上报时才有：Claude Code 按 Anthropic 价格计算，经过兼容代理或其他模型时仅供参考。旧客户端不会上报，trace 也不会回填。
- 升级：把工作区（Hub）升级到 v0.22.0 才能保存用量和提供接口；需要上报用量或运行 `myco usage show` 的电脑更新 myco；更新 App 才能看到用量页和分组修复。其他改动不需要升级 Hub。

## v0.21.34

- **A project's members can be grouped**: put people into 前端, 后端, 应用 or any group of your own. The workspace keeps the groups per project (up to 30, each name 1–40 characters). In the members column's ⋯ create a group; move a member from its own menu or drag it onto a group; a group's ⋯ renames, moves or deletes it. The coordinator stays on top and members in no group are under 其他. The column can show the groups folded (the default: a folded group's heading still shows its faces and who is at work or asking), as a row of faces per group, or as filter chips on top; the choice is kept on this computer. An older workspace says to upgrade first instead of hiding the ⋯.
- The CLI can group too: `myco project add --group 后端` puts a member in a group and creates it if it is new; `myco project show` lists each member's group.
- **A Usage page**: next to Automations and Workflows, totals, a bar a day, and members, projects and models ranked by tokens, for 7, 30 or 90 days, for the whole workspace or one project. The figures come from what each member's CLI reports, so the page fills in as members upgrade myco; a workspace whose hub cannot report usage yet says to upgrade the hub.
- Upgrade: update the hub to v0.21.34 to use groups; update the App for the grouped members column and the Usage page; update myco on computers that run `myco project add --group` or should report usage.

**中文**

- **项目成员可以分组**：把成员按前端、后端、应用或你自己起的名字分组。工作区为每个项目保存分组（最多 30 个，组名 1–40 个字符）。在成员栏的 ⋯ 里新建分组；成员可以从自己的菜单移动，也可以直接拖到组上；分组自己的 ⋯ 可以改名、调整顺序或删除。协调者始终在最上面，没有分组的成员在「其他」下面。成员栏的分组可以折叠显示（默认；折叠时标题上仍然显示组里的面孔，以及正在工作或提问的人）、每组一行面孔，或用顶部的筛选标签；这个选择保存在这台电脑上。旧工作区会在成员栏提示先升级，而不是把 ⋯ 藏起来。
- 命令行也能分组：`myco project add --group 后端` 把成员放进分组，分组不存在时会顺带创建；`myco project show` 会列出每个成员所在的分组。
- **用量页**：在「自动化」和「工作流」旁边，显示总量、每天一根柱子，以及按 token 排名的成员、项目和模型，可以看 7、30 或 90 天，范围是整个工作区或单个项目。数字来自各成员的 CLI 上报，成员升级 myco 后会陆续出现；工作区的 Hub 还不支持用量统计时，页面会提示升级 Hub。
- 升级：把工作区（Hub）升级到 v0.21.34 才能使用分组；更新 App 才能看到分组后的成员栏和用量页；需要用 `myco project add --group` 或需要上报用量的电脑更新 myco。

## v0.21.33

- **Disk usage is right on Windows, and capacity units are clear**: compressed and sparse files show the storage they actually take, and a file that takes 0 bytes shows 0. The Disk app's capacity selector defaults to binary GiB on Windows and decimal GB elsewhere, applies to the disk cards, scans, map, file lists and cleanup basket on that computer, and never changes the underlying byte counts. The Windows allocation fix needs the updated desktop App; updating the Disk app alone is not enough.
- **⌘K searches only the workspace you are in**: members, projects, computers, messages and recent chats from other workspaces are no longer mixed in; switch workspace to search there.
- **Panes stay put when you switch workspaces**: the members column is measured before the first paint, so in a wide window the panes beside the coordinator no longer jump wider and back.
- Upgrade: update the App for the Windows disk accounting, the ⌘K scope and the workspace-switch layout. No hub upgrade needed.

**中文**

- **Windows 上的磁盘占用更准确，容量单位也更清楚**：压缩和稀疏文件显示实际占用的空间，占用为 0 的文件显示 0。磁盘应用的容量单位选择器默认在 Windows 上用二进制 GiB、其他平台用十进制 GB，作用于这台电脑上的磁盘卡片、扫描、地图、文件列表和清理篮，不会改变底层字节数。Windows 的实际占用查询需要更新桌面 App，单独更新磁盘应用不够。
- **⌘K 只搜索当前工作区**：成员、项目、电脑、消息和最近会话不再混入其他工作区；要搜别的工作区，先切换过去。
- **切换工作区时，旁边的面板不再跳动**：成员列在第一次绘制前就量好宽度，宽窗口下协调者旁边的面板不会再先变宽再缩回。
- 升级：更新 App 才能获得 Windows 磁盘占用、⌘K 搜索范围和切换工作区布局的修复。不需要升级 Hub。

## v0.21.32

- **The notch island no longer shows 0**: a workspace that cannot be read keeps who was at work there, the island only comes out if someone is still at work once its window is ready, and it always folds away when nobody is. While folding it keeps the last number.
- Upgrade: update the App to see the island fix. No hub upgrade needed.

**中文**

- **灵动岛不再显示 0**：读不到的工作区会保留原来在工作的人；窗口就绪时仍然有人在工作，灵动岛才会展开，没人时总是收起；收起过程中保留上一个数字。
- 升级：更新 App 才能看到灵动岛的修复。不需要升级 Hub。

## v0.21.31

- **The Dock's red dot clears when you bring the App to the front**: it counts only what came after you last looked.
- Pictures that a progress update or a follow-up places in its text now show there, instead of "attachment not found", and are not listed again below.
- Picture rows in messages (a list item that starts with a picture) no longer overlap the line below.
- myco-ui cards: a card's colour is a small dot before its title, not a bar down the side.
- The binary ships only the everyday official apps, compressed, with smaller screenshots.
- Upgrade: update the App for the Dock dot, pictures in progress updates and the card dot; the smaller binary comes with the normal update. No hub upgrade needed.

**中文**

- **把 App 切到前台，Dock 上的红点就会消失**：只计算你上次看过之后新来的。
- 进展更新、补充消息里插在正文中的图片，现在会显示在原位，不再提示「没找到附件」，下面也不会重复列出。
- 消息里「左图右文」的列表行，图片不再压到下一行。
- myco-ui 卡片的颜色改成标题前的小圆点，不再是左侧的竖条。
- 安装包只带日常用的官方应用，并做了压缩，截图也更小。
- 升级：更新 App 才能看到 Dock 红点、进展更新里的图片和卡片小圆点；安装包变小随正常更新生效。不需要升级 Hub。

## v0.21.30

- **Messages can carry diagrams, coloured code and pictures**: a fenced `mermaid` block is drawn as a diagram (flowchart, sequence, ER, state, gantt), and one that does not parse shows its source and why. Code blocks that name a language are coloured, quietly in light and dark. Attach pictures and place them in the text with `![words](attachment:<name>)`: several on one line sit side by side, titled ones show their name and a line under it, and pictures placed in the text are not listed again below. Click a picture or diagram to see it large. Older Apps and the phone Apps show all of it as readable text.
- **Structured UI in a message (myco-ui)**: an agent can put a `myco-ui` block of JSON in a message, and the App draws it with a fixed set of components — cards, grids, tabs, headings, text, callouts, badges, key–value lists, tables, metrics, progress, steps, images, galleries, media lists, diagrams, charts, code, changed files, test results and buttons (a button can put words in your input, open a link or copy text). Anything outside that set is left out. Agents can print the components with `myco ui schema` and check a block with `myco ui check`; older clients show the message as text.
- Upgrade: update the App to see diagrams, code colours, pictures in the text and myco-ui blocks; update myco on computers where agents should write and check these blocks. No hub upgrade needed.

**中文**

- **消息里可以放图表、彩色代码和图片**：标了 `mermaid` 的代码块会画成图（流程图、时序图、ER 图、状态图、甘特图），解析失败会显示原文和原因。标了语言的代码块会着色，深浅色下都很克制。把图片作为附件上传后，可以用 `![说明](attachment:<文件名>)` 放进正文：同一行放几张会并排显示，带标题的会显示名称和下面一行说明，放进正文的图片不会在下面重复列出。点图片或图表可以放大看。旧版 App 和手机 App 会把它当普通文字显示。
- **消息里的结构化界面（myco-ui）**：agent 可以在消息里放一个 `myco-ui` 的 JSON 代码块，App 会用固定的组件把它画出来——卡片、网格、标签页、标题、文字、提示、徽章、键值列表、表格、指标、进度、步骤、图片、画廊、媒体列表、示意图、图表、代码、改动文件、测试结果和按钮（按钮可以把文字放进你的输入框、打开链接或复制文字）。不在固定组件里的内容不会画出来。agent 可以用 `myco ui schema` 查看组件，用 `myco ui check` 检查代码块；旧客户端会把消息当文字显示。
- 升级：更新 App 才能看到图表、代码着色、正文图片和 myco-ui 块；跑 agent 的电脑更新 myco 后，agent 才能写和检查这些块。不需要升级 Hub。

## v0.21.29

- **App updates work behind a system proxy, and a failed update no longer leaves you without an App**: with the proxy set to "System" and no proxy variables in the environment, the App now also reads the proxy in Windows' or macOS' network settings — the system proxy Clash or v2rayN turns on — and connects directly when that proxy program is not running. A release download cut off midway resumes where it stopped, reconnects after 60 seconds without data, and goes direct when the proxy fails. On Windows, a new version that cannot start is stopped and the current version is put back and keeps running. When an update fails, Settings says "更新失败" and explains the reason in plain words, with the raw error under details.
- **Sessions keep the variables they use to reach their model**: adding a Codex or Claude Code session (or running `myco join`) now saves the variables that session uses for its model into the agent's private env file (`~/.myco/env/<agent>.env`, mode 600): `CODEX_HOME`, the keys of magpie / cc-switch providers, `OPENAI_*` / `ANTHROPIC_*` and proxy settings. The runner no longer loses them when the shell's exports are gone. The App reads your login shell's environment (including a `CODEX_HOME` set there) and lists sessions under it, `codex exec` ones included. `myco join --no-env` skips this.
- **Focus mode backgrounds**: a palette button beside the exit button opens a small panel with quiet background tones (each in a light and a dark shade), the chat backgrounds' gradients and patterns, and a custom colour from the system picker, kept separately for light and dark. Choose to recolour the whole page or only around the conversation ("只改外面"); when text would be hard to read on a whole-page colour, the panel says so and offers "改成只改外面". Dark tones are softer, cards sit a shade above the background, and the chat header, pinned row and search line up with the messages. ⌘⇧F no longer also opens the chat search.
- Saving an agent's name or face in the App now shows at once, without waiting for the next refresh.
- Upgrade: update the App (updates behind a proxy, focus backgrounds, profiles showing at once); update myco on computers that run agents. Sessions added from then on keep their model variables — for an existing agent, run `myco runner env capture --agent <name>` in a shell where those variables are set. No hub upgrade needed.

**中文**

- **App 更新在系统代理下也能用，更新失败也不会把 App 弄没**：代理选「系统」且环境里没有代理变量时，App 现在会读 Windows / macOS 网络设置里的系统代理（Clash、v2rayN 打开的那个），代理程序没在运行就直连。下载中断会断点续传，60 秒没有数据自动重连，代理失败再直连。Windows 上新版本启动不了会被停掉，自动换回旧版，当前版本继续运行。更新失败时「设置」里显示「更新失败」，用大白话说明原因，原始错误收在详情里。
- **会话会带上连模型用的变量**：添加 Codex 或 Claude Code 会话（或运行 `myco join`）时，会把该会话连模型用的变量存进这个 agent 的私有 env 文件（`~/.myco/env/<agent>.env`，权限 600）：`CODEX_HOME`、magpie / cc-switch 各 provider 的 key、`OPENAI_*` / `ANTHROPIC_*` 和代理设置。shell 里 export 的变量没了，runner 唤醒时也照样能连上模型。App 会读登录 shell 的环境（包括在 shell 里设置的 `CODEX_HOME`），连 `codex exec` 的会话也能找到。`myco join --no-env` 可以跳过保存。
- **专注模式的背景**：退出按钮旁的调色按钮会打开一个小面板，里面有安静的背景色调（每个都有深浅两档）、聊天背景的渐变和图案，以及用系统取色器的自定义颜色，浅色和深色分别记住。可以选改整个页面，或只改对话外面（「只改外面」）；整页颜色会让文字难读时，面板会提示，并给出「改成只改外面」。深色色调更柔和，卡片比背景亮一档；对话标题、置顶行和搜索与消息对齐。⌘⇧F 也不会再顺带打开聊天搜索。
- 在 App 里改 agent 的名字或头像，保存后马上就会显示，不用等下一次刷新。
- 升级：更新 App（代理下的更新、专注模式背景、资料即时显示）；跑 agent 的电脑更新 myco。之后添加的会话才会带上模型变量——已有的 agent 可以在设了这些变量的 shell 里运行 `myco runner env capture --agent <名字>`。不需要升级 Hub。

## v0.21.28

- **New app: Server Console (服务器)**: manage Linux hosts over the OpenSSH setup already on your computer — CPU, memory and disk overview with hostname and online CPU count, systemd services and Docker containers, logs, and reviewed start/stop/restart. Profiles, host trust and keys stay on the computer that connects, and a shared page never runs a command. An agent can be given a grant for one exact server that expires: read-only for status and logs, or read and manage for services and containers. Needs the updated desktop App and Hub (`sudo myco upgrade`); ProxyJump and end-to-end encrypted workspaces are not supported yet.
- **New app: Database studio (数据库工作台)**: connect to MySQL, PostgreSQL, SQLite and MongoDB from the desktop App, browse schemas and tables, edit rows and documents with concurrent-write checks, run one SQL statement or MongoDB find/aggregate, and export results as CSV or typed JSON. Passwords and results stay on your computer; only a saved query draft is shared with the page. Needs the updated desktop App.
- **New app: Color studio (配色助手)**: build palettes of 2–12 colors, extract up to six colors from a local image, generate harmonies, check text/background contrast, and export SVG, CSS or JSON. Palettes, favorites and notes are shared with the page, so its agents can create and revise them too.
- **New app: AIGC Canvas (AIGC 画布)**: connect notes, text, images and videos into a workflow on a canvas, run a node with its upstream chain using your own OpenRouter (text) and Replicate (images, video) keys, and share nodes and results. Keys stay in the session; generated media links can expire, so save what you want to keep.
- **Voyage 1.2 (航海)**: a full day passes on the sea — the sun crosses the sky with dawn and dusk colors, a moon and stars light the night, and the boat shows its navigation lights. Weather now changes the scene: clear, cloudy, overcast, rain, storm with lightning and thunder, and sea fog. Time is shared in the page (still, a day in 48 or 12 minutes), so everyone sees the same hour.
- Text in Server Console and Database studio is larger and easier to read: table values and SQL at 14px, supporting text at least 12px, including in narrow windows.
- Upgrade: run `sudo myco upgrade` on the hub and update the App for Server Console; the other new apps and Voyage need only the updated App.

**中文**

- **新应用：服务器（Server Console）**：用电脑上已有的 OpenSSH 管理 Linux 主机——查看 CPU、内存和磁盘概览（含主机名和在线 CPU 数）、systemd 服务与 Docker 容器、日志，以及确认后的启动/停止/重启。连接配置、主机信任和密钥都留在发起连接的电脑上，共享页面里不会执行命令。可以给 agent 一个只针对某台服务器、会过期的授权：只读可看状态和日志，读写还能启停服务和容器。需要新版桌面 App 和 Hub（`sudo myco upgrade`）；暂不支持 ProxyJump 和端到端加密工作区。
- **新应用：数据库工作台（Database studio）**：在桌面 App 里连接 MySQL、PostgreSQL、SQLite 和 MongoDB，浏览库表结构，编辑数据行和文档（并发写入有校验），执行单条 SQL 或 MongoDB 的 find/aggregate，结果可导出 CSV 或带类型的 JSON。密码和结果都留在本机，只有主动保存的查询草稿会共享给页面。需要新版桌面 App。
- **新应用：配色助手（Color studio）**：创建 2–12 种颜色的色卡，从本地图片提取最多 6 种主色，生成邻近色、互补色等搭配，检查文字与背景的对比度，导出 SVG、CSS 或 JSON。色卡、收藏和笔记与页面共享，agent 也能一起创建和修改。
- **新应用：AIGC 画布（AIGC Canvas）**：在画布上把便签、文本、图片和视频连成工作流，用自己的 OpenRouter（文本）和 Replicate（图片、视频）密钥运行节点及其上游链路，节点和结果共享给页面。密钥只留在当前会话；生成的媒体链接可能过期，想留的请及时保存。
- **航海 1.2（Voyage）**：海上过完一整天的昼夜——太阳划过天空，早晚有金色和玫红色，夜里月亮和星星照亮海面，船会亮起航行灯。天气会改变画面：晴、多云、阴、雨、带闪电和雷声的暴风雨，还有海雾。时间在页面里共享（静止、48 分钟或 12 分钟一天），大家看到的是同一个时刻。
- 服务器和数据库工作台的文字更大更好读：表格数值和 SQL 为 14px，辅助文字至少 12px，窄窗口里也一样。
- 升级：服务器应用需要在 Hub 上执行 `sudo myco upgrade` 并更新 App；其他新应用和航海只需更新 App。

## v0.21.27

- **Focus mode**: a chat can fill the whole window on its own — press ⌘⇧F (Ctrl+Shift+F) or use the chat's ⋯ menu, which now shows the shortcut. The sidebar, top bar, members, pane tabs and a docked page hide, and the pane you were in zooms in with the chat centred on a plain background. ⌃Tab and ⌥↑/⌥↓ still switch chats; Esc, the shortcut again, or the button at the top right brings everything back.
- **Shopping comparison queried by an agent**: the app now works with a Maishou API adapter that an agent runs on its own computer; the agent queries prices and purchase links directly and writes them into the shared page. Bring your own Maishou access (`MAISHOU_OPENID`, an optional invitation code and token, or a private config file); credentials never go into page records, a chat or the repository. Authenticated results still depend on your own provider account — until then the page says what it could not confirm.
- The members column's hover cards show only the first line of what a member is doing, cut short.
- No hub upgrade needed: update the App for focus mode. The shopping comparison app reaches hubs through the official apps update (v0.20.7 and later); run `sudo myco upgrade` only if your hub is older than that.

**中文**

- **专注模式**：一个对话可以单独铺满整个窗口——按 ⌘⇧F（Ctrl+Shift+F），或用对话的 ⋯ 菜单（菜单里现在会显示这个快捷键）。侧栏、顶栏、成员、面板标签和停靠的页面都会藏起来，你正在用的面板放大，对话居中放在干净的背景上。⌃Tab 和 ⌥↑/⌥↓ 还能切换对话；按 Esc、再按一次快捷键，或点右上角的按钮就全部回来。
- **比价购物可以由 agent 来查了**：应用现在连上 agent 自己电脑上跑的 Maishou API 适配器，agent 直接查价格和购买链接，写进大家共享的页面。要带你自己的 Maishou 访问权限（`MAISHOU_OPENID`，可选的邀请码和 token，或一个私有的配置文件）；密钥不会写进页面记录、聊天或仓库。真正登录后的结果仍取决于你自己的服务商账号，在那之前页面会标明哪些没能确认。
- 成员栏的悬浮卡片只显示成员在做什么的第一行，超出部分截断。
- 不需要升级 hub：专注模式更新 App 即可。比价购物应用通过官方应用更新分发（v0.20.7 及以后的 hub）；hub 比这旧才需要 `sudo myco upgrade`。

## v0.21.26

- **A member's chat leaves terminal commands out**: since v0.21.24 a member's chat in a project showed its exchanges with everyone, which brought each command it ran on a terminal (`ls`, `cat` on demo-shell) in as a card. Only its exchanges with other AIs show now; a terminal still shows when the project lists it.
- **Pages play public audio and video**: a page can play a podcast or a video from a public address through the App, streamed as it plays (seeking works), instead of downloading it whole first.
- **New apps**: Voyage (航海), a 3D sea to explore by boat, with a lifelike sea, sky and weather; a podcast globe to discover and listen to public podcasts by region; a shopping comparison where an agent finds offers with their sources.
- Cinema's full screen works in the Mac desktop App, without corner overlays.
- Installing the runner service (from `myco join` or a join link's script) no longer waits silently for running tasks to finish: the runner restarts once they are done, and the install says so.
- No hub upgrade needed: update the App (pages playing media, the member chat), and myco on your computers for the runner install fix.

**中文**

- **成员对话里不再出现终端命令**：从 v0.21.24 起，项目里成员的对话会显示它和所有人的往来，结果它在终端上跑的每条命令（比如在 demo-shell 上的 `ls`、`cat`）也成了一张卡片。现在只显示它和其他 AI 的往来；项目里本来就列着的终端照常显示。
- **页面可以播放公开的音频和视频**：页面能通过 App 播放公开地址上的播客或视频，边播边传（可以拖进度），不用先整个下载下来。
- **新应用**：航海（坐船在 3D 海面上航行，海、天空和天气都很逼真）；播客地球（按地区发现并收听公开播客）；比价购物（agent 帮你找报价，并附上来源）。
- 影视在 Mac 桌面 App 里可以正常全屏，四角也不再有遮挡。
- 安装 runner 服务（`myco join` 或加入链接里的脚本）时，不再一声不响地等正在跑的任务结束：任务做完 runner 会自己重启，安装时会说明这一点。
- 不需要升级 hub：更新 App 即可（页面播放媒体、成员对话）；runner 安装的修复需要升级各台电脑上的 myco。

## v0.21.25

- **Google Antigravity as an agent**: pick "Antigravity" when you add an agent. myco drives the agy CLI through the community `google-antigravity-acp` adapter (`npm install -g google-antigravity-acp`, with agy signed in on that computer), and shows each step live. Experimental. The adapter cannot ask you for approval: only at full access does agy approve everything; below it, agy refuses what needs approval. Reaching Antigravity through third-party tools with a personal login is against Google's terms: use an AI Studio or Vertex AI key.
- **New app: Reader**: a personal bookshelf for PDF and TXT books on your own device, with themes, page turns, vertical paging or continuous scrolling, contents, search and bookmarks. Books and reading progress stay on the device.
- Upgrade myco on the computer that runs the Antigravity agent.

**中文**

- **Google Antigravity 可以当 agent 用了**：添加 agent 时选「Antigravity」。myco 通过社区的 `google-antigravity-acp` 适配器驱动 agy（在那台电脑上 `npm install -g google-antigravity-acp`，并登录 agy），每一步都能实时看到。实验性。这个适配器没法向你要审批：只有「完全权限」时 agy 才会全部自动批准；权限低于完全时，需要审批的操作 agy 会直接拒绝。用个人账号通过第三方工具访问 Antigravity 违反 Google 的条款，请改用 AI Studio 或 Vertex AI 的密钥。
- **新应用：阅读**：在自己的设备上放 PDF、TXT 的个人书架，支持主题、翻页动画、上下翻页或连续滚动、目录、搜索和书签。书和阅读进度都只存在这台设备上。
- 跑 Antigravity agent 的那台电脑要升级 myco。

## v0.21.24

- **Links straight to a film and episode**: a link to a Cinema page can name a title, source and episode, and the desktop App opens that spot (the iPhone and Android Apps get it with their next builds). Agents can find a title for you and send such a link; copying the link of what is playing works too. Older Apps open the page from its start.
- **Page exports with two Apps open**: two Apps signed in as the same person (two computers, or a preview beside the App) no longer take the export host from each other, which made agents' page exports stall and flooded the hub's audit log. Each App can render exports; one that closes no longer ends the agent's grant.
- **Agents waiting their turn stay online**: a computer runs at most 3 agent CLIs at once by default. An agent whose task waits for a free one no longer shows as offline, and its message reads "busy with another task" instead of "its myco is not running". With several agents on long tasks on one computer, raise the number: `myco runner service install --max-concurrent <n>`.
- A member's chat in a project shows all its exchanges and pages back through them.
- An agent's grant to export a page reads as "may export <page>", not as access to a screen. After the hub is upgraded, an App that was already open offers page exports within minutes.
- **New apps**: a logo studio where an agent draws geometric logos with guides; a personality exploration with private answers; an arcade with CHIP-8 and NES emulators, native MAME for desktop ROMs and an iPhone as the controller. Design Studio text lines up the same in every browser.
- Upgrade the server's hub (`sudo myco upgrade`, or from the App) and myco on your computers (for agents waiting their turn).

**中文**

- **直达某部片、某一集的链接**：影视页面的链接可以指定作品、线路和集数，桌面 App 点开直接到那里（iPhone 和 Android 的 App 下一次出包时带上）。可以让 agent 帮你找片、回一个这样的链接；正在播放的那一集也能直接复制链接。旧版 App 点开只会打开页面首页。
- **开着两个 App 时页面导出不再卡住**：同一个人登录的两个 App（两台电脑，或者正式版旁边开着预览版）不再互相抢导出宿主。之前会让 agent 的页面导出卡住，还会刷爆 hub 的审计日志。现在每个 App 都能渲染导出；其中一个关掉，agent 的授权也不会因此结束。
- **排队的 agent 不再显示离线**：一台电脑默认同时最多跑 3 个 agent。任务在排队等空位的 agent 不再显示离线，消息下面显示「在忙别的，做完再看」，不再说「对方的 myco 没在运行」。一台电脑上好几个 agent 都在做长任务时，可以把上限调大：`myco runner service install --max-concurrent <n>`。
- 项目里某个成员的对话，能看到它和所有人的往来，也能往前翻页。
- agent 导出页面的授权条写的是「可以导出「页面名」」，不再写成看屏幕。hub 升级以后，已经开着的 App 几分钟内就能接页面导出。
- **新应用**：标志工坊（agent 画几何标志，带辅助线）；人格探索（答案只存在本机）；街机（CHIP-8、NES 模拟器，桌面用原生 MAME 运行 ROM，iPhone 当手柄）。设计工作台的文字在各个浏览器里对齐一致。
- 需要升级服务器上的 hub（`sudo myco upgrade`，或在 App 里升级），电脑上的 myco 也要升级（排队不掉线那条）。

## v0.21.23

- **Agents can export a shared page as a file**: an agent you approve (or one that can already see the page) can export a whiteboard (SVG, PNG), a design (SVG, PNG, HTML, JSON) or a slide deck (PPTX, Markdown) without anyone clicking. The App renders the page in the background and the file goes only to that agent. Revoking the approval stops the export at once.
- **New apps**: a calendar for personal scheduling and shared bookings across time zones, and a learning workspace where a tutor teaches from the materials you import and guides you through a course.
- Fixes: approval cards line up with the message box; calendar time grids and event buttons line up.
- Upgrade the server's hub for page exports (`sudo myco upgrade`, or from the App).

**中文**

- **agent 能把共享页面导出成文件**：经你批准的 agent（或本来就能看到这个页面的 agent）可以直接导出白板（SVG、PNG）、设计稿（SVG、PNG、HTML、JSON）、幻灯片（PPTX、Markdown），不用人点。App 在后台渲染页面，文件只给这个 agent；撤销批准会立刻停止导出。
- **新应用**：日历，安排个人日程、跨时区预约；学习工作台，导师按你导入的资料讲课、带你学完一门课。
- 修复：审批卡片和输入框对齐；日历时间格和事件按钮对齐。
- 页面导出需要升级服务器上的 hub（`sudo myco upgrade`，或在 App 里升级）。

## v0.21.22

- **Use any model gateway for an agent**: an agent's Provider can now be a custom address (base URL, model and key; Codex also picks Responses or Chat Completions), so it can run through magpie, LiteLLM, OneAPI or any Anthropic- or OpenAI-compatible endpoint. The key stays in a private file on that computer and is never shown again.
- **See where an agent's requests really go**: an agent that follows its computer's settings shows the actual route, such as "magpie gateway (127.0.0.1:3425)" or the cc-switch provider in use. See "Use it with magpie or cc-switch" in INSTALL.md.
- **Change the provider of an agent on another computer**: pick "follow that computer", one of that computer's cc-switch providers, or clear its custom address, from the App on any of your computers. A custom address with a key can be set remotely in end-to-end encrypted workspaces only: the key is encrypted for that agent, and the hub passes on only ciphertext.
- The App: an agent's details are in four tabs (Settings, Skills, Records, Identity), and its tools are listed one per row, grouped by kind.
- The claude.ai guide starts with a diagram of how everything connects; the App's user guide has fuller pages with screenshots.
- Upgrade the server's hub for the provider changes (`sudo myco upgrade`, or from the App), and myco on the computer whose agents you change from elsewhere.

**中文**

- **agent 可以用任何模型网关了**：agent 的「服务商」可以填自定义地址（base URL、模型和密钥；Codex 还能选 Responses 或 Chat Completions），所以能走 magpie、LiteLLM、OneAPI，或任何兼容 Anthropic / OpenAI 的地址。密钥只存在那台电脑的私有文件里，存了以后不再显示。
- **看得到 agent 的请求实际走哪**：「跟随这台电脑」的 agent 会显示实际去处，比如「magpie 网关（127.0.0.1:3425）」或正在用的 cc-switch 渠道。见 INSTALL.zh-CN.md 的「和 magpie / cc-switch 一起用」。
- **能改别的电脑上 agent 的服务商**：在你任何一台电脑的 App 里，选「跟随它那台电脑」、它那台电脑上的某个 cc-switch 渠道，或清除自定义地址。带密钥的自定义地址只在开了端到端加密的工作区里能远程填：密钥先加密给那个 agent，hub 只经手密文。
- App：agent 详情分成四个标签（设置、擅长、记录、身份），工具按种类分组、一行一个。
- claude.ai 教程开头加了一张「怎么连起来的」图；App 的使用文档内容更全，配了截图。
- 服务商相关的改动需要升级服务器上的 hub（`sudo myco upgrade`，或在 App 里升级）；要从别处改哪台电脑上的 agent，那台电脑上的 myco 也要升级。

## v0.21.21

- **Reliable public workspace setup**: Windows waits for a public address and verifies the workspace is reachable before reporting success. An unfinished tunnel shows a retryable error while the local workspace keeps running.
- Tailscale setup offers official downloads, platform-specific install and start commands, and instructions to give an agent. The installation guide disappears after Tailscale is detected; failed connections show troubleshooting help.
- **Desktop notifications**: the App sends system notifications for workspace events that need your attention, opens the relevant conversation when clicked, and shows an unread badge on Mac. Upgrade the workspace hub (`sudo myco upgrade`) to enable the notice feed.
- Mac App: fixed a crash at startup when updating the Dock badge.
- Agents can request time-limited permission to use a computer; you can approve view-only access or revoke permission from the conversation. Upgrade the hub to enable grants; actual computer control also requires a compatible remote component, published separately.

**中文**

- **更可靠的公网开放流程**：Windows 会等公网地址生成并验证工作区可访问，再显示成功。隧道尚未就绪时给出可重试的错误，本地工作区继续运行。
- Tailscale 安装提供官方下载、各平台的安装启动命令，以及可交给 agent 的操作说明。检测到已安装后收起安装指引，连接失败时显示排障帮助。
- **桌面系统通知**：工作区有需要你处理的事情时，App 会发送系统通知；点击可打开对应对话，Mac 还会显示未读角标。工作区 Hub 需升级（`sudo myco upgrade`）才能提供通知。
- Mac App：修复更新 Dock 角标时导致启动崩溃的问题。
- Agent 可以申请限时使用电脑；你能在对话里只批准查看，或撤销授权。授权功能需升级 Hub；实际控制电脑还需另行发布的兼容远控组件。

## v0.21.20

- **Add your phone in simple mode too**: a new install starts in simple mode, which hid My devices (on Settings › Members). It now shows at the foot of Settings › Workspaces, with Add another device, so you can add your iPhone.
- The App: a page link from a workspace on this computer that serves the local network (http://192.168.x.x:7777/pages/…) opens in the App's page panel instead of the browser.
- The App: pictures in messages keep their size while they load again, so switching workspaces or reopening a chat no longer makes the messages jump.
- Docs: a step-by-step guide with screenshots, from installing the App to your agent on claude.ai and your phone (docs/guide-claude-ai.md).

**中文**

- **简单模式下也能加手机了**：新装的 App 默认是简单模式，它把「我的设备」（在 设置 › 成员 里）藏了。现在它显示在 设置 › 工作区 最下面，点「添加另一台设备」就能把 iPhone 加进来。
- App：这台电脑上对局域网开放的工作区给出的页面链接（http://192.168.x.x:7777/pages/…），现在在 App 的页面面板里打开，不再跳到浏览器。
- App：消息里的图片重新加载时保留原来的大小，切换工作区或重新打开对话时，消息不再跳动。
- 文档：一篇带截图的图文教程，从安装 App 到连上你在 claude.ai 的 agent，再到手机上指挥（docs/guide-claude-ai.zh-CN.md）。

## v0.21.19

- **A user guide in the App** (使用文档): short pages on everything the App does, with search, links between pages, and the previous and next page at the foot. Open it from the workspace menu, Settings › About, the Mac's Help menu, or the last step of the getting-started checklist. It covers voice, split panes, shortcuts and the community too.
- **Whiteboard 1.3**: drag a node's edge point onto another to connect them; elbow, curved or straight connectors, dashed lines and labels; box select, copy, cut, paste and duplicate (connectors come along); align and distribute; lock items; undo and redo; export SVG. See docs/whiteboard.md.
- Proxy console and Remote Desktop: their icons show correctly in the app library.
- The new Whiteboard and icons reach a workspace when its hub is upgraded (`sudo myco upgrade`, or from the App).

**中文**

- **App 里有使用文档了**：每个功能一小页，可以搜索，页面之间能互相跳转，页尾有上一页、下一页。从左下角工作区菜单、设置 › 关于、Mac 的「帮助」菜单，或者新手清单的最后一步打开。语音、分屏、快捷键和社群也都有介绍。
- **白板 1.3**：从节点边上的连接点拖到另一个节点就能连线；折线、曲线、直线，虚线和标签；框选、复制、剪切、粘贴、复制一份（连线一起带上）；对齐和均匀分布；锁定；撤销和重做；导出 SVG。说明见 docs/whiteboard.md。
- 代理工作台和远程桌面：在应用库里的图标显示正常了。
- 新版白板和图标在工作区的 Hub 升级后生效（`sudo myco upgrade`，或在 App 里升级）。

## v0.21.18

- **New installs can join or start another workspace**: the workspace menu at the bottom left offers "Join or start a workspace" and "New workspace on this computer" in simple mode too (a new install starts in simple mode, and the welcome page sends people there).
- **Codex sessions added from this computer can use myco**: they now run with workspace access by default. Before, they had no level, so Codex used its own sandbox without network and could not reach the workspace. An agent that runs with its CLI's default access now shows "Default access" next to the input, and its owner can switch it there.
- Recording Studio (Mac): a five-second countdown you can cancel before recording starts, so you can get the window ready.

**中文**

- **新装的 App 也能加入或新建工作区**：简单模式下，左下角的工作区菜单也有「加入或新建工作区」和「在这台电脑上新建工作区」了（新装的 App 默认是简单模式，欢迎页又会引导大家去那里）。
- **从这台电脑导入的 Codex 会话能用 myco 了**：默认用工作区权限。以前没给它设权限，Codex 用的是自己那个不能联网的沙箱，连不上工作区。用 CLI 默认权限运行的 agent，输入框旁会显示「默认权限」，它的主人可以在那里切换。
- 录屏工作台（Mac）：开始录制前有 5 秒倒计时，可以取消，方便先把窗口准备好。

## v0.21.17

- **Project notes, redesigned**: a project's page now has a header like other detail pages (name, goal, Edit, New task) and three tabs, Notes (brief, memory, files, links), Members (with the computers it may start agents on) and Conversations, instead of eight sections in a row. A long brief or memory folds, and long file and link lists show the first five.
- The App: project notes open at once again after you have read long conversations; before, a run's steps could push the page out of the App's memory, so it loaded from scratch. A project file you opened before also shows at once when it has not changed.
- The App: a project's members are listed with the most recently active first within each group.
- Mac App: the ⌘⇧Space quick panel no longer pops up by itself when the App starts.
- Recording Studio (Mac): you pick the area with the App's own selector, then the recording starts; the area, elapsed time and a Stop & edit button stay visible while recording (and are not recorded), stopping keeps the whole movie, and the system asks for permission at most once per session. Focus zooms ease in and out more gently, with adjustable durations.
- Cinema: catalogs scroll with touch again. The updated Cinema and Recording Studio reach a workspace when its hub is upgraded (`sudo myco upgrade`, or from the App).

**中文**

- **项目资料改版**：项目页顶部和其他详情页一样（名字、目标、编辑、新任务），下面分三个标签：资料（说明、记忆、文件、链接）、成员（以及它能在哪些电脑上开 agent）、对话，不再是八段排下来。说明和记忆太长会折叠，文件和链接先显示前五个。
- App：看过几段长对话后，再打开项目资料也能立刻显示了；之前执行过程会把它挤出 App 的缓存，只能从头加载。项目里打开过的文件，没改过的话再打开也立刻显示。
- App：项目成员在每组里按最近活跃排在前面。
- Mac App：⌘⇧Space 快捷面板不会再在 App 启动时自己弹出来。
- 录屏工作台（Mac）：先用 App 自己的选框选好区域再开始录；录制时区域、计时和「停止并剪辑」按钮一直可见（不会被录进去），停止后完整保存视频，同一次运行里系统最多只问一次权限。重点放大的进出更柔和，时长可调。
- 影视：片库又能用触摸滚动了。新版影视和录屏工作台在工作区的 Hub 升级后生效（`sudo myco upgrade`，或在 App 里升级）。

## v0.21.16

- **New official app: Cinema (影视)**: watch movies and series from sources you add yourself (none are bundled): search catalogs, choose episodes, keep a shared watchlist, and resume where you left off on this computer, with playback speed and intro/outro skipping. It follows the App's language and appearance. See docs/cinema.md.
- Recording Studio: screen recording on the Mac works again with the interactive area picker, and no longer waits for a permission prompt when recording is already allowed.
- The App: what you add while an agent is working stays with its run when the agent hands work to others meanwhile, instead of dropping below what it did after.
- Codex agents: Codex's own internal log lines (such as "failed to record rollout items") no longer show as red errors under a task; problems you can act on (a dropped connection it retries, a refused key) still do.

**中文**

- **新的官方应用：影视**：用你自己添加的影视源（不内置任何源）搜索电影和剧集、选集播放，收藏共享的待看清单，在这台电脑上续播，支持倍速和跳过片头片尾。跟随 App 的语言和外观。说明见 docs/cinema.md。
- 录屏工作台：Mac 上用框选区域录屏又能用了；已经授权录屏时，不再等待权限弹窗。
- App：agent 干活时你补充的话，即使它中途把活派给了别人，也会留在这次运行里，不会掉到它后面做的事情下面。
- Codex agent：Codex 自己的内部日志（比如 "failed to record rollout items"）不再显示成任务下的红色错误；需要你处理的问题（断线重试、key 被拒）照常显示。

## v0.21.15

- **New official app: Recording Studio (录屏工作台)**: record your screen from the App, then trim and split, add text and focus zooms (blurred areas follow the zoom), and export MP4, all on your computer; the footage and its thumbnails never leave it, and an agent can help with the edit. The Mac App asks for screen-recording permission first and offers a direct link to its setting.
- **Motion Videos 1.1**: notes and a shot plan for each video, a reference video to work from, sound and captions, steadier reference shots, and more templates.
- The App: a message you queue with ⌘↩ while the agent is at work shows at once in the Queued list above the box (edit or withdraw it there).
- Remote Desktop (preview): installing or updating the component from Settings, clearer errors that can be dismissed, and no update while a session is being shared.

**中文**

- **新的官方应用：录屏工作台**：在 App 里录屏，然后裁剪、分割、加文字和重点放大（打码区域会跟着放大移动），在本机导出 MP4。视频和缩略图都留在你的电脑上，不会上传；剪辑可以交给 agent 帮忙。Mac App 会先申请录屏权限，并提供直达设置的按钮。
- **动效视频 1.1**：每个视频有备注和分镜，可以参照一段参考视频来做，支持声音和字幕，参考画面更稳定，模板也更多了。
- App：agent 干活时按 ⌘↩ 排队的消息，会立刻出现在输入框上方的「排队中」里，可以在那里修改或撤回。
- 远程桌面（预览）：可以在设置里安装或更新组件；出错提示更清楚，也能关掉；正在共享时不会更新组件。

## v0.21.14

- **New official app: Motion Videos (动效视频)**: product promos, data stories and vertical shorts made of scenes played in order. Each scene is HTML and an animation (written by an agent, or one of eight templates) with its words and pictures in a form; preview, a timeline, transitions, 16:9 / 9:16 / 1:1 / 4:5, colour sets and fonts, and "ask an agent". Export to MP4 happens in the page, on your computer.
- Remote Desktop (preview) on phones: typing goes straight to the remote computer again (the system keyboard, with modifier keys in the side menu); the controls sit in a compact rail on the right and the keyboard no longer resizes the screen; a brief network drop (up to 10 seconds) no longer ends the session.
- The App: opening or switching an agent in a pane no longer makes the chat or the line between panes jump sideways.

**中文**

- **新的官方应用：动效视频**：做产品宣传片、数据故事、竖屏短视频，由一个个场景依次播放组成。每个场景是 HTML 加动画（agent 写，或者从八个模板里选），文字和图片在表单里改；有预览、时间线、转场，支持 16:9 / 9:16 / 1:1 / 4:5，配色和字体可选，也可以「让 agent 来做」。导出 MP4 在网页里完成，就在你自己的电脑上。
- 远程桌面（预览）手机端：打字又改回直接发到远程电脑（用系统键盘，修饰键在侧边菜单里）；控制按钮收成右侧一条窄栏，弹出键盘时画面大小不变；网络短暂断开（10 秒以内）不会再结束会话。
- App：在分栏里打开或切换 agent 时，对话和分栏之间的分隔线不会再左右晃动。

## v0.21.13

- Remote Desktop (preview): on a phone, what you type goes into a draft above the keyboard and is sent when you confirm (dictation and corrections no longer stream half-typed text to the computer); two fingers zoom and pan the view, and a short hold with two fingers scrolls the remote screen. Leaving a session asks first, and the controls and the keyboard switch are larger.
- Remote Desktop (preview): after an App upgrade, a remote component already installed and still compatible keeps working instead of asking to be installed again; uninstalling removes every version the App installed.

**中文**

- 远程桌面（预览）：手机上输入的文字先放在键盘上方的草稿里，确认后才发送（语音输入和修改不再把打了一半的文字发到电脑上）；两指可以缩放、平移画面，两指按住片刻再滑动才是滚动远程屏幕。退出会话前会先确认；按钮和键盘开关做得更大了。
- 远程桌面（预览）：App 升级后，已经装好、仍然兼容的远程组件可以继续用，不用重新安装；卸载时会删掉 App 装过的所有版本。

## v0.21.12

- **Workspace backups**: every workspace backs up its own data once a day (a snapshot of its database taken while it runs, plus its files) and keeps the last 7 on its server. In the App, Settings › Workspaces › Backups › Manage (the workspace's owner only) backs up now and saves the copy to this computer, and can keep the newest one here every day (the last 3), so a failed server disk does not take the data with it. On the server, `myco hub backup` and `myco hub restore` (it checks the whole backup first, stops and starts the service, and keeps what was there aside). See INSTALL, section 8.1.
- **No more false "session open somewhere else"**: a task for a session busy with another task of the same agent (in another conversation, or the same session joined to another workspace) now just waits its turn; a task run again after a restart is no longer held back by its own earlier run.
- **A long, quiet run no longer looks stuck**: during a long command with no new output (a build, a test run) the App now sees that the agent's runner is still on the task; the "may be stuck" note for runner tasks also waits 20 quiet minutes instead of 10.
- Remote Desktop (preview): touch gestures on phones (zoom, drag, a touch keyboard); the component still is not published.
- The App: in a project, the task you send shows in the feed at once while it is being sent.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) for backups and the quiet-run fix, and your computers (their runners) for the "session open somewhere else" fix.

**中文**

- **工作区备份**：每个工作区每天自动备份一次自己的数据（运行中给数据库拍快照，再加上文件），服务器上保留最近 7 份。App 里「设置 › 工作区 › 备份 › 管理」（只有工作区主人能看到）可以立即备份并存到这台电脑，也可以打开「每天在这台电脑上留一份」（留最近 3 份），服务器磁盘坏了数据也不会丢。服务器上可以用 `myco hub backup` 和 `myco hub restore`（恢复前会完整检查备份，自动停、启服务，原来的数据挪到旁边保留）。详见 INSTALL 第 8.1 节。
- **不再误报「会话在别处打开」**：同一个 agent 的会话正在处理另一个任务时（另一个对话里的任务，或者同一个会话加入了另一个工作区），新任务只是排队等着；重启后重跑的任务，也不会再被它自己上一次运行挡住。
- **长时间安静的运行不再像卡住了**：跑编译、测试这类长时间没有新输出的命令时，App 能看到 runner 还在处理这个任务；runner 任务的「可能卡住」提示也从安静 10 分钟改为 20 分钟。
- 远程桌面（预览）：手机上支持触控手势（缩放、拖动、触控键盘）；组件仍未发布。
- App：在项目里发的任务，会立刻出现在动态里，不用等发送完成。
- 升级工作区（设置 › 工作区，或在服务器上 `sudo myco upgrade`）才有备份和「安静运行」的修复；升级各台电脑（runner）才有「会话在别处打开」的修复。

## v0.21.11

- **Remote Desktop (preview, not installable yet)**: a new official app, Remote Desktop, to see and control another of your computers (Mac and Windows) from the App: encrypted WebRTC video at 720p, keyboard and mouse, a view-only mode, a one-time pairing code, and an explicit disconnect. It works between computers that can reach each other directly (the same network); a relay for other networks comes later. Capture and encoding run in a separate component that the App installs on demand; that component is not published yet, so "Install" shows it is unavailable for now. Needs an upgraded workspace (hub).
- The App: a message you send shows in the chat at once, while it is being sent.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) for Remote Desktop.

**中文**

- **远程桌面（预览，暂时还装不上）**：新的官方应用「远程桌面」，在 App 里查看和控制你的另一台电脑（Mac、Windows）：加密的 WebRTC 720p 画面、键盘鼠标、仅查看模式、一次性配对码，可以随时断开。目前只支持能直接连通的两台电脑（同一网络），其他网络的中继以后再加。截屏和编码在一个单独的组件里，由 App 按需安装；这个组件还没有发布，所以现在「安装」会显示不可用。需要升级工作区（Hub）。
- App：发出的消息会马上出现在对话里，不用等发送完成。
- 升级工作区（设置 › 工作区，或在服务器上 `sudo myco upgrade`）才有远程桌面。

## v0.21.10

- **Project memory**: a project now has a shared memory. Members add what everyone should keep in mind (decisions, conventions, the owner's preferences) with `myco project memory add --name <project> --text "..."`, and each entry records who wrote it and when. The runner gives member agents the memory with the project, and gives it again when it changes. In the App, the project notes have a Memory section where you add an entry or edit and tidy the whole memory. In a test with real Claude Code agents, a decision or preference told to one member reached the others in 5 of 5 runs (0 of 5 before).
- **Longer project brief**: up to 16,000 characters (it was 8 KB), and all of it goes into the members' prompt (only the first 4 KB did before).
- The App: code inside your own message bubbles stands out in light mode; mushroom avatars are easier to read on light tiles; a little more space above the time and buttons under an agent's reply.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) and your computers (their runners) for project memory and the longer brief.

**中文**

- **项目记忆**：项目有了一份共享的记忆。成员用 `myco project memory add --name <项目> --text "..."` 记下大家都该记住的事（定下的决定、约定、主人的偏好），每条自动记下是谁、哪天写的。runner 会把记忆和项目一起给成员 agent，记忆有变化时再给一次。App 的项目资料里有「项目记忆」，可以添加一条，也可以整份编辑、整理。用真实的 Claude Code 测试：告诉一个成员的决定或偏好，5 次里 5 次都传到了其他成员（以前是 0 次）。
- **项目说明放宽**：最多 16000 字（以前是 8 KB），并且全部放进成员的提示（以前只放前 4 KB）。
- App：浅色模式下，你自己消息气泡里的代码更醒目；蘑菇头像在浅色底上更清楚；agent 回复下方的时间和按钮上面多留了一点空。
- 升级工作区（设置 › 工作区，或在服务器上 `sudo myco upgrade`）和各台电脑（runner）才能用上项目记忆和更长的项目说明。

## v0.21.9

- **Official apps follow the App's language and appearance**: every official app now has Chinese and English text and switches with the App's language and light / dark setting, including their editors and canvases. Your records are unchanged.
- **A restart no longer loses a running task**: when the runner restarts (an upgrade restarts it too) in the middle of a task, it puts the task back in the queue once and runs it again, telling the agent to check what it already did. A shell command on an exec node, which may have run in part, still fails, with a clear reason.
- The Poll app is no longer offered among the official apps; pages already made from it keep working.
- Upgrade the App and your computers (their runners) for these; upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) for the translated apps.

**中文**

- **官方应用跟随 App 的语言和外观**：所有官方应用都有中英文，随 App 的语言和浅色 / 深色设置切换，编辑器和画布也一样。原有数据不变。
- **重启不再丢掉正在跑的任务**：任务进行中 runner 重启了（升级也会重启它），会把任务放回队列重新跑一次，并提醒 agent 先检查已经做过的部分。exec 节点上的 shell 命令可能已执行一半，仍标为失败，并写明原因。
- 官方应用里不再提供「投票」；已经用它做的网页照常可用。
- 升级 App 和各台电脑（runner）即可用上这些；升级工作区（设置 › 工作区，或在服务器上 `sudo myco upgrade`）才有翻译好的应用。

## v0.21.8

- **New official app: Meeting Notes (会议记录)**: record a meeting from any app (your microphone as "me", the computer's sound or one meeting app as "them") and have it transcribed on this computer (SenseVoice: Chinese, English, Cantonese, Japanese, Korean; downloaded the first time). Sentences by speaker, played from where you click, corrected in place, speakers renamed, search, and export as text, Markdown or SRT; several people at one microphone are told apart, and an agent can write the notes. Recording works in the Mac App (macOS 14.2+ for the computer's sound) and on Linux; not on Windows yet.
- **New official apps**: Photo Edit (修图: crop, adjustments, filters, portrait retouch, text, stickers, frames; photos stay on the computer that opened them), Design Studio (设计工作台: briefs, layers, HTML prototypes, feedback and snapshots), Market Watch (行情工作台: China, Hong Kong and US instruments with interchangeable data providers, watchlists and charts) and Proxy Console (代理工作台: manage existing Mihomo and sing-box cores; controller credentials stay in this computer's keychain).
- Making a page from an app: the first one in a workspace comes with sample data, later ones start empty (no checkbox any more).
- Official apps: when GitHub still hands out the old checksum right after the apps are published, the hub tries again instead of failing.
- What apps do on this computer is logged without the raw arguments.
- Upgrade the App for Meeting Notes (recording and speech to text) and the sample-data change; upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) for the retry.

**中文**

- **新的官方应用：会议记录**：从任意应用录会议（麦克风算「我」，电脑播放的声音或某个会议软件算「对方」），在这台电脑上转成文字（SenseVoice：中、英、粤、日、韩，第一次用时下载）。按说话人分句，点哪句从哪播，就地改错、改说话人名字、搜索，导出文本、Markdown 或 SRT；一个麦克风前的几个人也能分开，agent 可以写纪要。Mac App 能录（录电脑声音要 macOS 14.2+），Linux 也能录；Windows 还不行。
- **新的官方应用**：修图（裁剪、调色、滤镜、人像美化、文字、贴纸、相框；照片只存在打开它的那台电脑上）、设计工作台（需求、图层、HTML 原型、反馈和快照）、行情工作台（A 股、港股、美股，数据源可换，自选和图表）、代理工作台（管理已有的 Mihomo、sing-box 内核；控制器凭据存在本机钥匙串）。
- 从应用新建网页：工作区里第一份自带示例数据，之后的从空的开始（不再有勾选框）。
- 官方应用：刚发布、GitHub 还给出旧校验值时，Hub 会再试，不再直接报错。
- 应用在这台电脑上做的事，日志里不再记录原始参数。
- 要升级 App 才有会议记录（录音和转文字）和示例数据的改动；要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）才有重试。

## v0.21.7

- **New official app: Disk Cleanup (空间清理)**, after DaisyDisk: your disks with their free room, one click to scan (or only your home folder, or a folder you pick), a map of rings beside a list of what takes the room, largest first, and a basket at the bottom for what to clean; it goes to the trash, so it can be put back. The scan keeps going when the page panel is tucked away, and you can watch it fill.
- **Apps can use the computer the App runs on** (`myco.os`): system information and disks, processes, files and the trash, a scan of folders by size, commands, the clipboard, notifications and screenshots. Only pages opened in the App on that same computer can, and each call is logged to `~/.myco/logs/app-os.jsonl`. There are no permission prompts yet: open only apps you trust.
- **English first**: the install guide, the agent setup guide and the other docs are in English (the Chinese install guide is `INSTALL.zh-CN.md`), and the release notes are in English and Chinese. The pages a join link or an App link opens are in English, or Chinese when your browser prefers it, and the sentence to give an AI session is "Join myco: <link>".
- The sentence the App gives your agent when something fails points to the guide's "Troubleshooting" section.

**中文**

- **新的官方应用：空间清理**（像 DaisyDisk）：列出各个磁盘和剩余空间，一键扫描（或只扫用户目录、选一个文件夹），左边环形图、右边按大小排的清单，底部一个待清理的篮子；清理是移到废纸篓，可以放回。收起网页面板时扫描不会停，扫描过程看得见。
- **应用可以使用 App 所在的这台电脑**（`myco.os`）：系统信息和磁盘、进程、文件和废纸篓、按大小扫描文件夹、运行命令、剪贴板、通知、截图。只有在这台电脑的 App 里打开的网页能用，每次调用都记在 `~/.myco/logs/app-os.jsonl`。目前还没有授权弹窗：只打开你信得过的应用。
- **以英文为主**：安装指南、给 agent 的安装指南和其他文档改成英文（中文安装指南是 `INSTALL.zh-CN.md`），发版说明中英双语。入网链接和 App 链接打开的网页默认英文，浏览器偏好中文时显示中文；给 AI 会话的那句话是「Join myco: <链接>」。
- App 出错时让你发给 agent 的那句话，指向安装指南的「Troubleshooting」一节。

## v0.21.6

- **The ⌘⇧Space quick panel works now**: press Return to send; the panel closes and the App opens your chat with Shroom, where you read the answer. "Open chat" at the top right takes you there too. Before, commands the App sent to the page were held back and never arrived (this also broke opening a member from the notch island and refreshing when the panel pops up); all of that is fixed. The panel has no white edge and no workspace picker: it follows the App's current workspace. When something is waiting for your OK, one line shows "See".
- **The superagent feels more like your own assistant**: in the App it is always called Shroom, and its avatar wears a small crown. It is not in the sidebar's "Conversations" list; find it from the bottom-left menu or with ⌘⇧Space. Its chat has no back button, and an empty chat shows an introduction (who it is and what it can do for you). It can't be added to projects.
- **The agent card's "Skills" section is renamed (技能 → 擅长 in Chinese)**: it is the section others read to choose who to hand work to, and it is now kept apart from Claude Code / Codex's own skills.
- **See the MCP servers, skills and plugins on each computer**: the computer details page lists the MCP servers (local / remote, user-level / project-level), skills and plugins configured for Claude Code and Codex on each computer; an agent's profile page has a new "Tools" section. Read-only; no keys are sent.
- **"Allow MCP tools" switch** (Claude Code, off by default, only the agent's owner can change it): when it is on, the agent can use the MCP tools configured on its computer even with workspace access.
- Redesigned computer details page: tabs ("Agents and terminals", "Access", "AIs and tools", "Check and fix"), with buttons that match the other details pages.
- Apps: when you create a page you see screenshots of each app (light / dark); in Task Tracker 1.3.0 the filter menus are roomier, and active filters show at the top.
- Workflow page header buttons are now the same size; runs given only a path or a link are named by their start time.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) to get the superagent kept out of projects, the MCP switch and app screenshots; the lists of what is on a computer need `myco upgrade` on that computer.

**中文**

- **⌘⇧Space 快捷面板可以用了**：回车发出去，面板收起，App 打开和 Shroom 的对话，回答在对话里看；右上「打开对话」点了也能跳过去。之前 App 发给页面的指令被扣着没送到（刘海岛打开成员、面板弹出时刷新也受影响），现在都好了。面板没有白边、不用选工作区，跟着 App 当前的工作区；有等你点头的事时一行「去看」。
- **superagent 更像一个专属助手**：在 App 里一律叫 Shroom，头像戴一顶小皇冠；不在侧栏「会话」列表里，从左下角菜单或 ⌘⇧Space 找它；它的对话没有返回按钮，空对话是一段介绍（它是谁、能帮你做什么）；不能被加进项目。
- **「技能」改叫「擅长」**：agent 名片上那一节（别人按它挑人派活），和 Claude Code / Codex 自己的技能（Skills）分开。
- **电脑上的 MCP、技能、插件看得到了**：电脑详情页列出每台电脑上 Claude Code 和 Codex 配的 MCP（本地 / 远程、用户级 / 项目级）、技能、插件；agent 资料页多一节「工具」。只读，不传密钥。
- **「允许 MCP 工具」开关**（Claude Code，默认关，只有 agent 的主人能改）：打开后在工作区权限下也能用它电脑上配的 MCP 工具。
- 电脑详情页改版：标签页（Agent 和终端、权限、AI 和工具、检查和修复），按钮和其他详情页一致。
- 应用：新建页面时能看到应用的截图（浅色 / 深色）；任务追踪 1.3.0 的筛选菜单更宽松，已选的筛选显示在顶上。
- 工作流页头按钮大小统一；只给了路径或链接的运行，按开始时间命名。
- 要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）：superagent 不进项目、MCP 开关、应用截图；电脑上的清单要那台电脑 `myco upgrade`。

## v0.21.5

- **Set up a superagent by a sentence**: the "superagent" dialog has a new "Join by a sentence". Copy that sentence and send it to any Claude Code or Codex session (including claude.ai cloud sessions and sessions that receive messages with `myco inbox wait`). Once it joins, it becomes your superagent in this workspace and the App opens your chat with it. The item in the bottom-left menu is now simply called "superagent".
- **Fixed Codex agents failing as soon as they took a task** ("--skip-git-repo-check cannot be used multiple times"): older Codex agent configs already included this flag, and newer Codex doesn't allow it twice. myco now passes it only once (run `myco upgrade` on that computer).
- The official apps Notes and Task Tracker have larger text and menus on phones, so they are easier to tap; Notes no longer shows "You are You".
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) to use join by a sentence for the superagent.

**中文**

- **superagent 可以一句话接入**：「superagent」弹窗里新加「一句话接入」，复制那句话发给任意 Claude Code 或 Codex 会话（包括 claude.ai 云端、用 `myco inbox wait` 收消息的那种），它加入后就成了你在这个工作区的 superagent，App 自动打开和它的对话。左下角菜单里这一项就叫「superagent」。
- **修复 Codex agent 接活就失败**（「--skip-git-repo-check cannot be used multiple times」）：老的 Codex agent 配置里带了这个参数，新版 Codex 不许重复，现在 myco 只传一次（要在那台电脑上 `myco upgrade`）。
- 官方应用「笔记」「任务追踪」在手机上字和菜单更大、好点；笔记不再显示「你是 你」。
- 要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）才能用 superagent 的一句话接入。

## v0.21.4

- **New official app: Notes** (like Notion): a page tree on the left that you can nest, drag to reorder and add to favorites, with a trash; pages use inline icons.
- **New official app: Task Tracker** (like Linear): numbered issues (such as MYC-12), status, priority, assignee (a person or an agent), labels, projects, due dates and comments.
- When you open a page made with an official app, the workspace first checks for a newer version of the app (if it hasn't checked in over an hour), so after a new app version is merged you no longer wait a day for the "Upgrade" prompt.
- **Switching model, reasoning effort and access takes effect right away** (on screen), without waiting for the whole App to refresh; task cards waiting for a reply say "Waiting" instead of "Working".
- While a page made by an agent is opening, you see a bouncing little mushroom and "Opening…" instead of a black screen.
- **Windows App**: if the window still shows nothing after 25 seconds (WebView2 failed to start), a dialog lets you open it in your browser instead, or restart in a compatibility mode that applies only to myco; the taskbar icon is larger; scrollbars are thinner and only appear while you scroll.
- Workflows and automations pages tidied up: one kind of button, filters on the left; delete in menus just says "Delete" and turns red only when you hover over it.
- iPhone: workflows, the superagent, what an agent is working on, a project's computers, and files and apps are now on the phone too, and you can upgrade myco from it.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) to get the new update checks for official apps.

**中文**

- **新的官方应用：笔记**（像 Notion）：左边是页面树，可以嵌套、拖动排序、收藏，有回收站；页面用行内图标。
- **新的官方应用：任务追踪**（像 Linear）：编号的问题（如 MYC-12）、状态、优先级、负责人（人或 agent）、标签、项目、截止日期和评论。
- 用官方应用做的页面，打开时工作区会先看看有没有新版本（超过一小时没看过的话），合并了新版应用后不用再等一天才提示「升级」。
- **切换模型、推理强度和权限马上生效**（显示上），不再等整个 App 刷新；在等回复的任务卡片写「在等回复」，不再写「在做」。
- 打开 agent 做的网页时，加载中显示跳动的小蘑菇和「正在打开…」，不再是一块黑屏。
- **Windows App**：窗口 25 秒还显示不出内容（WebView2 启动失败）时会弹框，可以改在浏览器里打开，或用只对 myco 生效的兼容模式重启；任务栏图标更大；滚动条更细，只在滚动时出现。
- 工作流和自动化页面整理：同一种按钮、筛选放左边；菜单里的删除只写「删除」，鼠标移上去才变红。
- iPhone：工作流、superagent、agent 手上的活、项目的电脑、文件和应用，在手机上也能升级 myco。
- 要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）才能用到官方应用的新检查。

## v0.21.3

- **⌃Tab switches agents** (like ⌘Tab switches apps): hold ⌃ and press Tab to bring up a row of avatars (on frosted glass); choose with Tab / ⇧Tab or ← →, and let go to open your chat with it. Order: the current chat, agents working in this project, other members of the project, agents working elsewhere, then the rest; offline agents are not listed. On a Mac ⌥Tab works too; in a browser use ⌃`.
- **New agents no longer start out showing "Session in use"**: once the computer has opened the session, the first task starts right away instead of idling for a minute (run `myco upgrade` on that computer).
- In all dialogs, there is now space between the description under the title and the content below it.

**中文**

- **⌃Tab 切换 agent**（像 ⌘Tab 切应用）：按住 ⌃ 再按 Tab，弹出一排头像（毛玻璃底），Tab / ⇧Tab 或 ← → 选，松手就打开和它的对话。顺序：当前对话、这个项目里正在干活的、项目里其他成员、别处在干活的、其余的；离线的不列。Mac 上 ⌥Tab 也行，浏览器里可以用 ⌃`。
- **新开的 agent 不再一开始就显示「会话被占用」**：电脑开好会话后，第一个任务马上开始，不再空等一分钟（要在那台电脑上 `myco upgrade`）。
- 所有弹窗标题下面的说明文字和下面的内容之间留出了空间。

## v0.21.2

- **Projects can choose their own "Computers it may start agents on"**: in the project's profile, under the members, each computer has a switch (only the project's owner can change them; all off by default). On a computer that is switched on, the project's coordinator can start agents directly, without being on that computer's allow list; folders and access still follow that computer's own settings. This is separate from the project's terminals, which only let agents run commands there.
  - The coordinator uses `myco project machines --name 项目` to see which computers the workspace has and which ones this project may use; you decide which one to start on.
  - Without permission, the coordinator can ask (`myco project machine request`): you see a card in your chat with it saying which computer it wants and why; click "Approve" or "Deny", and it is notified and carries on.
  - You can also switch computers on or off with `myco project machine allow/disallow`.
- **Cards waiting for your OK appear in the chat of the agent that asked**: not only for the superagent, but for coordinator requests too.
- "Poor connection" only shows while the agent is working; it no longer appears in chats where the agent has already stopped.
- When a computer is online but the App can't change its allowed folders directly, the App explains why: it is that computer's own rule and has to be changed on it (for WSL, use the App on the same Windows machine).
- Several dialogs have more space below their buttons; the "Who can give tasks" dialog is wider and easier to choose from.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server); for a coordinator to start agents on a computer, that computer also needs `myco upgrade`.

**中文**

- **项目可以单独设置「可以开 agent 的电脑」**：在项目资料里，成员下面，每台电脑一个开关（项目主人才能开关，默认都关）。打开的电脑，项目的协调者可以直接在上面开 agent，不用再进那台电脑的允许名单；文件夹和权限还是按那台电脑自己的设置。和项目里的终端无关，终端只管让 agent 在那里跑命令。
  - 协调者用 `myco project machines --name 项目` 看工作区有哪些电脑、这个项目能用哪几台；开在哪台由你定。
  - 没权限时协调者可以申请（`myco project machine request`）：你在和它的对话里看到一张卡片，写着它要哪台电脑、为什么，点「同意」或「不同意」，它会收到通知接着做。
  - 你也可以用 `myco project machine allow/disallow` 打开或关掉。
- **等你点头的卡片出现在对应 agent 的对话里**：不只是 superagent，协调者的申请也一样。
- 「网络不太好」只在 agent 正在干活时才提示；它已经停下的对话不再出现。
- 电脑在线、但 App 不能直接改它允许的文件夹时，说清楚原因：这是那台电脑自己的规定，要在它上面改（WSL 用同一台 Windows 上的 App）。
- 几个弹窗的按钮下面留出了空间；「允许谁派任务」弹窗更宽、更好选。
- 要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）；要让协调者在某台电脑上开 agent，那台电脑也要 `myco upgrade`。

## v0.21.1

- **New: superagent (Shroom)**. Everyone can set up one agent per workspace that manages the workspace for them; it is always called Shroom.
  - In the workspace menu at the bottom left, click "Set up a superagent": start a new Shroom (Claude Code or Codex) on one of your computers, or make one of your existing agents the superagent. Once it is set up, click "superagent" to open your chat with it.
  - It can see the whole workspace, create projects, bring agents into projects, start agents on your computers, run workflows, hand out work and keep track of progress. Before deleting or stopping things, changing other people's access, or changing automations, it asks you first: a card appears above the input box in its chat with "Approve / Deny", and the menu shows a count; requests not handled within 24 hours expire. It always speaks in its own name and can't pose as you.
  - Its permissions are enforced by the workspace, not by prompts; the audit log records its actions as "you (via superagent shroom)". CLI: `myco superagent set/show/unset/guide`, `myco approval list/approve/deny`.
  - **Mac: ⌘⇧Space** brings up a small panel at any time (even when the App is not in front): type a line to hand to Shroom; below it are its latest answer, what it is working on, and cards waiting for your OK. "Open chat" jumps to the full chat; Esc closes the panel.
- **See how workflows are doing**: number of runs, success rate, how long each run and each step took (working and waiting for people counted separately), cost, and why runs failed; logs for every step (the task, progress and screenshots, the reply, which tools it used), outputs and scores; compare two runs side by side. The workflow page is also easier to use: tabs for runs, "How it goes", steps, and "Inputs and outputs"; editing now shows the graph on the left and the current step on the right.
- **‹ › back and forward at the top**: like in a browser, go back to the workspace, page, project or chat you were just looking at (⌘[ / ⌘] and the mouse side buttons work too); pages no longer have their own back buttons.
- **The model list follows the computer's Codex**: when you create an agent or pick a model in a chat, you see the models and reasoning efforts offered by that computer's own Codex.
- **Change the folders agents may start in from the App**: directly for this computer, and also for computers connected over SSH / WSL; CLI: `myco machine dirs --add/--remove`.
- **Agents that fail to start no longer show "Working" forever**: if Codex / Claude Code produces no output for 5 minutes after starting (it can't reach the model, is stuck at sign-in, or a tool server hangs), the task fails with the end of the error output attached.
- **A bad network no longer makes agents look stuck**: App requests wait at most 60 seconds, so one stuck request can't block every refresh; when a chat gets no new data for 20 seconds, it shows "Poor connection" and retries on its own.
- When an agent joins, myco adds a few lines on how to use myco to Claude Code / Codex's user-level instructions (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`): report progress, include screenshots, suggest next steps, wait for the next message. `myco skill memory --remove` removes them; `myco join --no-memory` doesn't add them.
- When an agent is in several projects, every project message says which project it is from, so work from one project doesn't spill into another.
- Apps (sheets, docs and so on) now use a black, white and gray color scheme instead of blue.
- The superagent and workflow insights need a workspace upgrade (Settings › Workspace, or `sudo myco upgrade` on the server); agents get the new instructions, model list and startup check after `myco upgrade` on each computer.

**中文**

- **新功能：superagent（Shroom）**。每个人在每个工作区可以配一个替你管工作区的 agent，它固定叫 Shroom。
  - 左下角工作区菜单里点「配一个 superagent」：在你的一台电脑上新开一个 Shroom（Claude Code 或 Codex），或者把你已有的 agent 设成它。配好后点「superagent」打开和它的对话。
  - 它能看全工作区、建项目、把 agent 拉进项目、在你的电脑上开 agent、跑工作流、派活、盯进度。删东西、停东西、改别人的权限、改自动化这类事会先问你：卡片出现在它对话的输入框上方，「同意 / 不同意」，菜单上有数字提醒；24 小时没处理就作废。它永远以自己的名义说话，不能冒充你。
  - 权限由工作区卡住，不靠提示词；审计里记成「你（经 superagent shroom）」。命令行：`myco superagent set/show/unset/guide`、`myco approval list/approve/deny`。
  - **Mac：⌘⇧空格** 随时叫出一个小面板（App 不在前台也行）：一行输入交给 Shroom，下面是它最新的回答、在做什么和等你点头的卡片，「打开对话」跳到完整对话，Esc 收起。
- **工作流能看出跑得怎么样**：跑了几次、成功率、每次和每一步用了多久（干活和等人分开）、花了多少、失败原因；每一步的日志（任务、进展和截图、回复、它用了哪些工具）、产物、评分，两次运行可以并排对比。工作流页面也更好用了：分成运行、运行情况、步骤、输入和输出几栏，编辑改成左边图、右边当前这一步。
- **顶上的 ‹ › 前进后退**，像浏览器一样回到刚才看的工作区、页面、项目或对话（⌘[ / ⌘]、鼠标侧键也行），各页面不再各有返回按钮。
- **模型列表跟着电脑上的 Codex 走**：新建 agent 和对话里选模型时，列出那台电脑的 Codex 自己提供的模型和思考强度。
- **在 App 里改允许开 agent 的文件夹**：这台电脑直接改，通过 SSH / WSL 连的电脑也能改；命令行 `myco machine dirs --add/--remove`。
- **启动不起来的 agent 不再一直显示「在做」**：Codex / Claude Code 启动后 5 分钟没有任何输出（连不上模型、卡在登录、工具服务卡住），任务直接失败并附上报错末尾。
- **网络不好不再说 agent 卡住**：App 的请求最多等 60 秒，不会一个卡住的请求挡住所有刷新；对话 20 秒没拉到新数据时提示「网络不太好」并自动重试。
- agent 加入后，在 Claude Code / Codex 的用户级说明（`~/.claude/CLAUDE.md`、`~/.codex/AGENTS.md`）里留几行 myco 用法：报进度、带截图、给下一步建议、等下一条消息。`myco skill memory --remove` 可以去掉，`myco join --no-memory` 不加。
- 一个 agent 在几个项目里时，每条项目消息都会写明是哪个项目的，不会把这个项目的活带到另一个里。
- 应用（表格、文档等）改成黑白灰配色，不再用蓝色。
- superagent 和工作流的观测要升级工作区（「设置 › 工作区」一键升级，或服务器上 `sudo myco upgrade`）；各台电脑 `myco upgrade` 后，agent 才有新的说明、模型列表和启动检查。

## v0.21.0

- **New: workflows**. Write down a fixed way of doing something as a flow, and the workspace works through it in order: each step goes to an agent or a computer, or waits for your decision. It keeps going even when your computer is off.
  - "Workflows" is the second item at the top left of the App, with a number on the right when decisions are waiting for you. At the top are cards for the decisions waiting on you (a few options, optionally with a note; an expired one counts as not doing it, never as a default yes), then your workflows and their run history.
  - New workflow: four ready-made ones (write-review-revise, run tests after changes, split up then combine, ask you before a release) where you only pick who does each part; or have an agent write one from a sentence.
  - Open a workflow: a flow chart (loops back, conditions and merges are all drawn), "Inputs and outputs" (what each step receives and returns), and every run. Open a run: the chart marks each step's status, and each step shows who did it, the input it got, the result it returned and whether that matched the contract; "Chat" jumps to that task. You can cancel a run, or restart it from the step that failed.
  - "Edit": change who does a step, the timeout, retries, the maximum number of rounds and what to ask for; edit what each step returns field by field (or edit the JSON directly); saving creates a new version, and runs in progress keep the old one.
  - A step can leave out the person and give a "What it does" instead: when it starts, it picks the agent whose "What it does" matches and who has the least work on hand.
  - Each step's input and output contract is a full JSON Schema (same as agentflow): if the input doesn't match, the step fails; if the result doesn't match, it is sent back to be redone once. By default each step's result goes to the next step, or you can assemble it on the edge.
  - Agents hand in results with `myco message reply --result`; agents can also write, save and run workflows (`myco workflow …`), and are notified when runs they started finish.
  - In chats, tasks sent by a workflow are marked "Workflow".
- **Automations can start workflows**: when editing an automation, choose "Start a workflow", and fill inputs from a webhook's payload with placeholders like `{{payload.issue.title}}`; in the run history, "The run" opens that run directly.
- **Keyboard shortcuts**: Settings › Shortcuts lists every shortcut in the App; new ones: ⌘/ opens that list, ⌥↑/⌥↓ moves between projects and conversations in the sidebar, ⌘N new project, ⌘\ sidebar, ⌘U add files, ⌘= ⌘- ⌘0 text size, ⌘⇧L light/dark.
- Upgrade the workspace (Settings › Workspace, or `sudo myco upgrade` on the server) to get workflows; for agents to hand in results with `--result` and use `myco workflow`, run `myco upgrade` on each computer (JSON code blocks handed in by older versions are still accepted).

**中文**

- **新功能：工作流**。把固定的做法写成流程，工作区按顺序往下走：每一步交给一个 agent、一台电脑，或者等你决定。电脑关着也照常。
  - App 左上第二个「工作流」：有决定在等你时右边一个数字；最上面是等你决定的卡片（几个选项，可以带一句话；过期当作不做，从不默认同意），然后是你的工作流和运行记录。
  - 新建：四个现成的（写-审-改、改完跑测试、分头做再汇总、发布前问你一句），只选谁来做；或者用一句话让一个 agent 写一个。
  - 打开一个：流程图（回头的环、条件、合流都画出来）、「输入和输出」（每一步收到什么、交回什么），以及每一次运行。打开一次运行：图上标出每一步的状态，每一步谁做的、收到的输入、交回的结果、是否符合约定，「看对话」跳到那条任务；可以取消，或从失败的那一步重来。
  - 「编辑」：换人、改超时、重试、最多几轮、要它做什么，按字段改每一步交回什么（或直接改 JSON）；保存成新的一版，正在跑的照旧。
  - 一步可以不指定人，写「分工」：开始时挑分工对得上、手上活最少的 agent。
  - 每一步的输入和输出约定是完整的 JSON Schema（和 agentflow 一样）：输入不符合这一步就失败，结果不符合退回重交一次。上一步的结果默认交给下一步，也可以在边上组装。
  - agent 交结果用 `myco message reply --result`；agent 也能写、存、运行工作流（`myco workflow …`），它启动的运行结束时会收到通知。
  - 对话里，工作流发的任务标着「工作流」。
- **自动化可以启动工作流**：编辑自动化时选「启动一个工作流」，Webhook 调用的内容用 `{{payload.issue.title}}` 这样填进输入；运行记录里「看运行」直接打开那次运行。
- **快捷键**：「设置 › 快捷键」列出 App 里所有快捷键；新增 ⌘/ 打开它、⌥↑/⌥↓ 在侧栏的项目和会话间切换、⌘N 新建项目、⌘\ 侧栏、⌘U 加文件、⌘= ⌘- ⌘0 文字大小、⌘⇧L 深浅色。
- 工作流要升级工作区（「设置 › 工作区」里一键升级，或服务器上 `sudo myco upgrade`）；agent 用 `--result` 交结果、用 `myco workflow`，要在各台电脑上 `myco upgrade`（旧版交的 JSON 代码块也认）。

## v0.20.29

- **Upgrade myco in the App, no terminal needed**:
  - **Computers**: the "Computers" page lists computers whose myco isn't the latest; you can "Upgrade all", or open one and upgrade it on its own. Each computer's own runner downloads the new version and restarts a few seconds later (tasks running on it are interrupted). This computer is upgraded by the App directly. For computers that are offline, or whose myco is older than this version, the App shows the `myco upgrade` to run on them.
  - **Workspace**: in Settings › Workspace, a workspace that isn't on the latest version has an "Upgrade the workspace" button. While the workspace restarts it can't be reached for about half a minute; no messages are lost, and the App waits for it to come back on the new version. On servers installed with install-hub.sh, run `sudo myco upgrade` on the server once first, which installs the small service that handles upgrades; after that you can upgrade from the App. Newly installed servers come with this service.
- **"Needs your decision" now only means questions for you**: when an agent's question is for another agent (such as the one that gave it the task), the yellow dot no longer shows.
  - The chat says which tasks from elsewhere it still hasn't finished and who it is waiting on, with "See and handle".
  - The top of its profile lists its current work: each item says who it is waiting on, or "Left open", and you can "Mark done" or "Stop".
  - Tasks waiting for an answer no longer count as "May be stuck".

**中文**

- **在 App 里升级 myco，不用开终端**：
  - **电脑**：「电脑」页会列出 myco 不是最新的电脑，可以「全部升级」，也可以点开一台单独升级。每台电脑由它自己的 myco 后台下载新版，几秒后重启后台（上面正在跑的任务会中断）。这台电脑由 App 直接升级。不在线的电脑，或者 myco 还早于这一版的电脑，会给出要在它上面运行的 `myco upgrade`。
  - **工作区**：「设置 › 工作区」里，版本不是最新的工作区有「升级工作区」按钮。工作区重启时大约半分钟连不上，消息不会丢，App 会等它用新版本回来。用 install-hub.sh 装的服务器，要先在服务器上 `sudo myco upgrade` 一次，装上负责升级的小服务，之后就能在 App 里升级。新装的服务器自带这个服务。
- **「在等你决定」只表示问你的问题**：agent 问的是别的 agent（比如派活给它的那个），不再显示黄点。
  - 对话里会说明它还有哪些别处来的任务没做完、在等谁回答，并带「查看并处理」。
  - 它的资料最上面列出当前工作：每条都写着在等谁或「没收尾」，可以「标记完成」或「停止」。
  - 在等回答的任务不再算「可能卡住」。

## v0.20.28

- **A "Working" status you can trust more**: before, a task an agent had acknowledged never expired. If the agent forgot to reply, its session ended or the runner died, it showed "Working" forever, with a yellow dot on the left and a spot in the notch island. Now an agent only counts as working if it is online and the task has had recent activity: for tasks run by the runner, 5 minutes without activity ends that; for agents in a session, 30 minutes. The time of the last activity is stored on the task, so it survives workspace restarts.
- **Tasks left open can be wrapped up**: an agent whose only remaining tasks are left open shows a hollow circle on the left, and the notch island no longer lists it. In the chat, under that task, you get:
  - "Mark done": close it without a reply;
  - "Nudge": send it a follow-up asking how the task is going;
  - "Stop" and "Resend", as before.
- **Results are no longer lost when the runner can't deliver them**: if the workspace is briefly unreachable, the runner keeps the result and tries again every 2 minutes until it gets through. Before, it only wrote a log line and the task stayed hanging.
- **Agents are reminded to wrap up**: `myco inbox pull` lists the tasks it acknowledged but hasn't closed, and reminds it to reply or mark them done.
- Takes effect after you upgrade the workspace server (`sudo myco upgrade`). Redelivering results from the runner and the wrap-up reminder need `myco upgrade` on each computer.

**中文**

- **「在干活」的状态更可信**：以前 agent 确认过的任务永远不会过期。只要 agent 忘了回复、会话结束了、或者 runner 挂了，它就一直显示「在干活」，左边亮黄点、灵动岛里也有它。现在只有 agent 在线、并且任务最近有动静，才算在干活：runner 跑的任务 5 分钟没动静就不算，会话里的 agent 是 30 分钟。动静的时间存在任务上，工作区重启也不丢。
- **没收尾的任务能自己收尾**：只剩没收尾任务的 agent，左边显示一个空心圈，灵动岛不再列它。在对话里，那条任务下面有：
  - 「标记完成」：直接关掉，不回复；
  - 「再催一下」：给它发一条补充，问这条任务怎么样了；
  - 「停止」和「重发」，和以前一样。
- **runner 送不回结果时不再丢**：工作区一时连不上，runner 会把结果留着，每 2 分钟再送一次，送到为止。以前只记一条日志，任务就一直挂着。
- **agent 会被提醒收尾**：`myco inbox pull` 会列出它确认过、还没关的任务，并提醒它回复或标记完成。
- 要升级工作区服务器（`sudo myco upgrade`）才会生效。runner 补送结果和 agent 收尾提醒，要在各台电脑上 `myco upgrade`。

## v0.20.27

- **Videos exported from Video Editor can be saved to Downloads**: before, the App only let apps save documents and images up to 20 MB, so exported mp4 files were rejected even though they showed as saved. Now apps can save video and audio (mp4, webm, mov, m4a, mp3, wav, up to 4 GB), written to disk as they are produced, without filling memory. A failed save says why; if the App is too old to save video, you are told before you export.
- **Long videos can be exported too**: video is written to disk as it is made and audio is mixed in segments, so exporting a few minutes of 1080p video uses very little memory and no longer fails halfway.
- Fixed a few issues in the automated UI tests.

**中文**

- **视频剪辑导出的视频能存到「下载」了**：以前 App 只让应用保存 20 MB 以内的文档和图片，导出的 mp4 被拒，却显示已保存。现在应用可以保存视频和音频（mp4、webm、mov、m4a、mp3、wav，最大 4 GB），边写边存，不占内存。保存失败会说清楚；App 版本太旧存不了视频时，导出前就会提醒。
- **长视频也能导出**：视频边做边写到磁盘，声音分段混合，几分钟的 1080p 视频导出时内存占用很小，不会再中途失败。
- 修了几处自动化界面测试的问题。

## v0.20.26

- **Right-click to copy an agent**: right-click a Claude Code or Codex agent in the conversations list on the left or among the project members on the right (the avatars at the top in a narrow window), choose "Fork a copy" and give it a name. A copy made in a project can go straight into that project, and its chat opens once it is ready. The copy duplicates the original session, so it remembers everything said before, and works on the same computer in the same folder; the original carries on as usual.
- **New app: Video Editor**:
  - import mp4, then cut, trim both ends and drag clips to reorder them on the timeline;
  - change speed (0.25–4×) and volume;
  - add text, and blur or pixelate part of the frame;
  - export mp4 in the browser: choose the resolution (up to the original), frame rate and quality, with the estimated size and time left.

  Video files stay on the computer that opened them; the workspace only stores the edit. Agents can edit with you, for example to split clips, add captions or blur things out.
- **What apps save survives restarts**: the App used a new port every time it started, so data apps kept in the browser (such as videos imported into Video Editor) was gone after a restart. Now it reuses the last port.
- **More reliable chat**:
  - when you open a chat, existing replies show in full instead of typing out again;
  - when sending times out, the App first checks with the workspace whether the message went through; if it really didn't, it says so above the input box, keeps your text and offers a retry;
  - images that fail to load are retried twice automatically;
  - the pages panel no longer reloads when you switch it to full screen.

**中文**

- **右键复制一份 agent**：Claude Code 和 Codex 的 agent 在左边会话列表、右边项目成员（窄窗口是顶上的头像）上右键，选「复制一份」，起个名字就好。在项目里复制的可以顺手放进这个项目，复制好后自动打开和它的对话。副本复制了原来的会话，之前聊过的都记得，在同一台电脑、同一个文件夹里干活；原来的照常用。
- **新应用「视频剪辑」**：
  - 导入 mp4，在时间线上剪切、调整两端、拖动排序；
  - 调速度（0.25–4 倍）和音量；
  - 加文字，给画面的一块打模糊或马赛克；
  - 在浏览器里导出 mp4：可选分辨率（最高到原视频）、帧率和画质，显示预计大小和剩余时间。

  视频文件只留在打开它的电脑上，工作区只存剪辑。agent 也能一起改，比如切分、加字幕、打码。
- **应用里存的东西重启后还在**：App 每次启动都换一个端口，应用存在浏览器里的数据（比如视频剪辑导入的视频）重启后就没了。现在沿用上次的端口。
- **聊天更稳**：
  - 打开对话时，已有的回复直接整段显示，不再重新逐字出现；
  - 发送超时会先去工作区确认有没有发出去，真没发出去时在输入框上方说明，保留文字并提供重试；
  - 图片加载失败会自动重试两次；
  - 网页面板切换全屏时不再重新加载。

## v0.20.25

- **Change an agent's access right next to the input box**: there is a new access button next to the model, with four levels: follow the computer's setting, read-only, workspace, full. It works for both Claude Code and Codex; only the agent's owner can change it, and it takes effect the next time the agent wakes. Choosing "Full" asks you to confirm once; if the computer allows at most "Workspace", you can lift that limit right from the menu.
- **If the workspace or runner on this computer stops, the App starts it again**: before, once they stopped, nothing in the App could connect, and adding a session from the onboarding page failed with connection refused. Now, as long as the service is still installed, the App notices and restarts it with its original settings; if you run into this while adding a session, it starts it first and retries automatically. Anything you stopped with "Stop" in the App is not restarted.
- **Updating the App on a Mac upgrades the myco CLI too**: before, only Windows did this, and the CLI on a Mac stayed at the version you first installed. The background service switches to the new version the next time it restarts; click "Fix it" in the Add an agent dialog to restart it right away.
- **"Images and files" is now "Files and apps"**: apps linked in the chat come first, each listed once, and you can jump back to the message it is in; images and files come after them. Tabs at the top switch between All, Apps, Images and Files.
- A message you just sent is no longer set back to "not started" by a result that arrives late, and replies already on screen no longer disappear.

**中文**

- **在输入框旁边直接改 agent 的权限**：模型旁边多了一个权限按钮，四档可选：跟随电脑设置、只读、工作区、完全。Claude Code 和 Codex 都能改，只有 agent 的主人能改，下次唤醒时生效。选「完全」前会确认一次；电脑最多只允许「工作区」时，菜单里能直接放开。
- **本机工作区或 runner 停了，App 会自己把它启动回来**：以前停了以后，App 里什么都连不上，引导页加会话也报 connection refused。现在只要服务还装着，App 发现后就按原来的设置重启它；加会话时碰上，会先启动再自动重试。在 App 里点「停止」的不会被拉起。
- **Mac 上更新 App 时，命令行 myco 也会一起升级**：以前只有 Windows 会这样，Mac 上的命令行一直停在第一次装的版本。后台服务下次重启时换成新版；在添加 agent 弹窗里点「修复」可以马上重启。
- **「图片和文件」改成「文件和应用」**：对话里链接过的应用排在最前面，每个只列一次，可以跳回它所在的消息；再往下是图片和文件。上面有标签切换：全部、应用、图片、文件。
- 刚发出的消息不会再被晚到的结果改回「还没开始」，已经显示的回复也不会消失。

## v0.20.24

- **After you change networks, this computer can start new sessions in the workspace it hosts again**: when the workspace runs on this computer, the computer joined it with the local network address it had at the time. After a network change the old address no longer works, so the runner that starts sessions stayed offline. Now, when it can't connect, it connects to the same workspace through `127.0.0.1` on this computer, first checking with the token that it is the same workspace. Run `myco upgrade` on this computer, or click "Fix it" (described below) in the App.
- **When a computer can't start sessions, the App says why and fixes it**: before, it always said "too old, run myco upgrade in a terminal", even when myco was already the latest. Now there are two cases:
  - myco is too old: click "Upgrade myco" for this computer;
  - the runner that starts sessions isn't connected: click "Fix it", which upgrades to the latest version and restarts the runner.

  Neither needs a terminal. Other computers can also be fixed in one click once SSH or WSL is set up for them on the "Computers" page; otherwise the App gives the command to run on that computer.
- **Imported sessions keep their original titles**:
  - Codex sessions use their session name;
  - Claude Code sessions use the name set with /rename, or the title Claude Code generated;
  - if there is neither, the first message, shortened.

  No more computer name plus a number.
- **A clearer dialog for opening to the internet**:
  - the options are cards, each with a tag;
  - while opening, it shows the steps, the time taken and a progress bar;
  - when done, it lists addresses you can copy, and only closes when you click "Done".
- Settings › Workspace is kept in simple mode too. The getting-started checklist in "Automations" ticks items off right away.

**中文**

- **换了网络后，这台电脑又能在本机工作区开新会话**：工作区建在这台电脑上时，电脑是用当时的局域网地址加入的。换网络后旧地址连不上，开会话的后台就一直掉线。现在连不上时会改从本机 `127.0.0.1` 连同一个工作区，先用令牌确认是同一个。要在这台电脑上 `myco upgrade`，或者在 App 里点下面的「修复」。
- **电脑开不了会话时说清原因，并在 App 里修好**：以前一律说「版本太旧，去终端运行 myco upgrade」，即使已经是最新。现在分成两种：
  - myco 太旧：在这台电脑上点「升级 myco」；
  - 开会话的后台没连上：点「修复」，会升到最新并重启后台。

  都不用开终端。别的电脑在「电脑」页设好 SSH 或 WSL 后也能一键修，否则给出要在那台电脑上运行的命令。
- **导入的会话用原来的标题**：
  - Codex 用它的会话名；
  - Claude Code 用 /rename 起的名字，或它自己生成的标题；
  - 都没有时，用第一句话截短。

  不再是一串电脑名加编号。
- **开放到公网的对话框更清楚**：
  - 选项做成卡片，每个带标签；
  - 开放时显示步骤、用时和进度条；
  - 完成后列出可以复制的地址，点「完成」才关。
- 简洁模式下也保留「设置 › 工作区」。「自动化」里的上手清单会马上打勾。

## v0.20.23

- **When Claude can't sign in, you choose how to sign in**: before, you only got an error. Now, when a new Claude agent can't sign in, the App says which method agents started on this computer currently use, and lists the methods available on this computer; click one and it retries automatically:
  - use a provider from cc-switch (settings written by tools like Magpie count too);
  - keep using the API settings exported in your terminal (the address is shown, the key is not);
  - sign in to your Claude account again (opens a terminal; type `/login`).

  New agents started on this computer keep using the method you picked. Takes effect after `myco upgrade` on the computer the agent runs on.
- **10 more AIs supported (experimental)**: Qoder, Qwen Code, Kimi, goose, GitHub Copilot, Auggie, CodeBuddy, Kilo, Cline and Grok, all connected through ACP. Only the ones installed appear in this computer's agent list, and you can also choose them when starting a new agent.
- **New projects can have a goal and files**: besides the name, you can write a sentence on what the project is for, and drag in or choose a few files (up to 20 MB each), which are uploaded once the project is created; failed uploads can be retried right away or later.
- **Settings is now a dialog floating over the page**: categories and search on the left, content on the right; press Esc or click outside to close it, and the page underneath stays as it was. In a narrow window it fills the whole window.
- Cards on the "Computers", "Apps" and "Automations" pages are more compact and the same size on all three pages.

**中文**

- **Claude 登录不上时让你选怎么登录**：以前只显示一段报错。现在新开的 Claude agent 登录不上时，会说清楚这台电脑启动的 agent 现在用的是哪种方式，并给出这台电脑上有的几种，点一个就自动再试：
  - 用 cc-switch 里的某个服务商（Magpie 这类工具写的设置也算）；
  - 沿用终端里导出的 API 设置（会显示地址，不显示 key）；
  - 重新登录 Claude 账号（打开终端，输入 `/login`）。

  选过的方式这台电脑之后新开的 agent 都会沿用。要在 agent 所在的电脑上 `myco upgrade` 才生效。
- **新支持 10 个 AI（实验性）**：Qoder、Qwen Code、Kimi、goose、GitHub Copilot、Auggie、CodeBuddy、Kilo、Cline、Grok，都通过 ACP 接入。装了的才会出现在本机的 agent 列表里，新开 agent 时也可以选。
- **新建项目可以写目标、带文件**：除了名字，还能写一句项目要做什么，拖进或选几个文件（每个最多 20 MB），项目建好后上传；传失败的可以重试或以后再传。
- **设置改成浮在页面上的对话框**：左边是分类和搜索，右边是内容；按 Esc 或点外面关掉，下面的页面保持原样。窗口窄时占满整个窗口。
- 「电脑」「应用」「自动化」页面的卡片更紧凑，三个页面大小一致。

## v0.20.22

- **Joining an existing session copies it almost instantly**: before, the copied session first had to run one turn (tens of seconds per turn for Codex, so minutes for two sessions); now joining only registers it, and the copy is made the first time you send it a task.
- **Waits longer before falling back when a session is briefly busy**: it retries in place 6 times, 5 seconds apart (about 30 seconds); before, it tried only 3 times.
- **"Start it" for a new agent no longer stays disabled**: the App asks that computer which folders it allows, and if there is no answer within 8 seconds you can start anyway.
- When you set up this computer, the sessions you can add are in a scrollable box, with search when there are many.

**中文**

- **加入已有会话的复制几乎是瞬间的**：以前要先让复制出来的会话跑一轮（Codex 一轮几十秒，加两个就是几分钟），现在加入时只登记，第一次给它发任务时才复制。
- **会话被短暂占用时等得更久再退回**：先原地重试 6 次、每次隔 5 秒（约 30 秒），以前只试 3 次。
- **新开 agent 的「开一个」不再一直点不了**：问那台电脑允许哪些文件夹，8 秒还没回答就可以先开。
- 设置这台电脑时，可以加入的会话放在一个能滚动的框里，多的时候有搜索。

## v0.20.21

- **New Codex agents no longer fail with "not in a trusted directory"**: Codex used to refuse to start when the new folder wasn't a git repository.
- **Open to the internet in one click**: the default is "One click (recommended)", which uses a temporary Cloudflare address and needs no account. If cloudflared isn't installed, the App downloads it (on a Mac through Homebrew if you have it, otherwise from Cloudflare's official releases, with the download verified). The address changes after this computer restarts; just click again.
- **A freshly copied session no longer says "in use" right away**: when a session is briefly busy, the App first retries in place a few times, and only goes back to the queue if it really is in use.
- **The name on a Mac is the computer name**: before, it could be the IP address your router assigned (like `192-168-1-42-…`); now it is the computer name from System Settings.
- Sent messages no longer appear twice.
- "Meet your workspace" is ticked off as soon as you open Settings › Workspace.

**中文**

- **新开 Codex 不再报「不在受信任的目录」**：新建的文件夹不是 git 仓库时，Codex 以前会拒绝启动。
- **开放到公网一键完成**：默认是「一键开放（推荐）」，用 Cloudflare 临时地址，不用账号。没装 cloudflared 时 App 会自己下载（Mac 上有 Homebrew 就用它，否则从 Cloudflare 的官方发布下载并校验）。这台电脑重启后地址会变，再点一次就行。
- **刚复制的会话不再马上说「被占用」**：会话短暂被占用时先原地重试几次，真被占用才退回排队。
- **Mac 上的名字用电脑名**：以前可能用的是路由器分的 IP（比如 `192-168-1-42-…`），现在用「系统设置」里的电脑名。
- 发出去的消息不再出现两次。
- 「认识你的工作区」打开设置 › 工作区就打勾。

## v0.20.20

- **Install a workspace on a server over SSH**: Join a workspace › No link yet? › "Over SSH"; pick a server, and the App signs in, installs the workspace and adds it for you.
  - Automatic HTTPS, at `<IP>.sslip.io`; ports 80 and 443 must be open.
  - If a workspace is already installed there, it is just added, without changing its settings.
  - Needs root login, or sudo without a password; if that isn't the case, the App says so and gives you a command to run in a terminal.
- **Opening to the internet no longer hangs**:
  - with Tailscale, every step has a time limit; if one gets stuck, it stops and says why: Tailscale didn't respond, the Tailscale app is running under another macOS user, or Funnel isn't turned on yet (with a direct link to turn it on).
  - while it says "Opening…" you can click "Cancel", which also stops the work already running in the background.
  - the hints shown when you connect a computer always include "Open to the internet", and for a workspace on another computer they explain how to open it.
- **Install Claude Code and Codex under a macOS user that isn't an admin**: the npm install command the App gives installs into that user's own `~/.local`, instead of failing with `EACCES` because Homebrew's folders belong to someone else.
- **Onboarding explains what a workspace is**: the welcome page, "Set up this computer" and the getting-started checklist all say what a workspace is, where it runs and how others join it.
- Expanding long content or run steps in a chat no longer jumps straight to the bottom.
- When you open the Mac DMG, a window tells you to drag the App into Applications.

**中文**

- **用 SSH 在服务器上装工作区**：加入工作区 › 还没有链接？ › 「用 SSH 装到服务器上」，选一台服务器，App 登录上去装好工作区，再自动加进来。
  - 自动 HTTPS，地址是 `<IP>.sslip.io`，要能开 80、443 端口。
  - 已经装过的直接加进来，不改原来的设置。
  - 要 root 登录，或者 sudo 不用输密码；不满足时说清楚，并给一条在终端里跑的命令。
- **开放到公网不再卡死**：
  - 用 Tailscale 时每一步都限时，卡住就停下，并说清原因：Tailscale 没回应、Tailscale App 跑在别的 macOS 用户下、Funnel 还没开（直接给出去开启的链接）。
  - 「正在开放」时可以点「取消」，后台正在做的也会停掉。
  - 连电脑时的提示里总有「开放到公网」，在别的电脑上的工作区也给出怎么开放。
- **在不是管理员的 macOS 用户下也能装 Claude Code、Codex**：App 给的 npm 安装命令装到这个用户自己的 `~/.local`，不再因为 Homebrew 的目录属于别人而报 `EACCES`。
- **新手引导说清工作区是什么**：欢迎页、「设置这台电脑」和上手清单里都讲了工作区是什么、在哪运行、别人怎么加入。
- 在聊天里展开长内容或运行步骤，不再一下子跳到最底下。
- Mac 的 DMG 打开后有一个窗口，告诉你把 App 拖进「应用程序」。

## v0.20.19

- **Getting-started checklist**: on computers set up from the welcome page, the bottom of the sidebar lists five things worth doing once. Each ticks itself off when done; click one to go and do it.
- The first time you open the projects, apps or computers page, a card explains what it is for. The checklist and the cards can each be dismissed in one click and won't come back.

**中文**

- **上手清单**：从欢迎页开始用的电脑上，侧栏底部有五件值得做一次的事。做过的会自己打勾，点一下就能去做。
- 第一次打开项目、应用、电脑页时，有一张卡片说明它是做什么的。清单和卡片都能一键关掉，关掉后不再出现。

## v0.20.18

- **Connect computers over SSH and WSL**: "Connect a computer" now offers three ways at the top: "Over SSH / WSL on this Windows / Copy a command".
  - The SSH page lists the hosts in your SSH config. Tick them and click "Connect": the App signs in with your own ssh and ssh-agent, installs myco and connects it to the workspace, ticking off each step. If you connect only one, the App switches to it once it is connected.
  - When something fails, it says why and gives a command you can try in a terminal (for example, run `ssh-add` first, or `ssh` once in a terminal to accept the fingerprint).
  - The App doesn't read or store keys; the join link only goes through SSH's standard input and never appears on the command line. Only Linux and macOS computers can be connected.
  - On Windows, "WSL on this Windows" lists this computer's distributions; tick one to install into it.
- **"Check and fix" on the computers page**:
  - "Check" tells you item by item what is going on with that computer: the myco version, whether the runner is running, whether it stops when you log out, the proxy the service uses, whether the workspace can be reached through the proxy and directly, and the last 20 lines of the log. For example: "Can't reach the workspace through the proxy; a direct connection works."
  - There are also "Restart runner" and "Upgrade myco" buttons. For a computer that wasn't connected from the App, first pick which SSH host or WSL it is.
- **Set up this computer right after installing**: after the workspace starts for the first time, a page lists the AIs on this computer (which you can install and sign in to) and recent Claude Code and Codex sessions; the ones you tick are added. You can open it again from Settings › About. The welcome page has a new look too.
- **An empty chat suggests three things to ask**; click one to put it in the input box.
- **Automations**: each automation has its own page listing every run. For a run triggered by a webhook, you can also see what the sender posted (without credentials or signatures).
- **Settings**: search at the top left, and the navigation is split into "This computer / Workspace / About".
- **The runner on Linux no longer stops after you log out**: `myco runner service install` now also turns on linger. Before, the runner stopped a little while after you closed the last SSH or WSL window.
- **Reconnecting the same computer no longer turns it into "name-2"**: connecting again with a new join link recognizes it as the same computer.
- **Installs no longer get stuck on a sudo password**: when you connect over SSH from the App and that computer already has an old myco in `/usr/local/bin`, the App installs into `~/.local/bin` instead.
- On computers that can't open a browser, such as servers, the Codex sign-in command is now `codex login --device-auth`.
- The remove-computer dialog no longer counts the computer's own node as an agent.
- Automation run history is kept on the server, so upgrade the hub (`sudo myco upgrade`). The "name-2" fix and linger need an upgrade on **the computer being connected** (`myco upgrade`).

**中文**

- **用 SSH 连电脑、连 WSL**：「连一台电脑」上面多了「用 SSH 连 / 这台 Windows 里的 WSL / 复制命令」三种方式。
  - SSH 那页列出你 SSH 配置里的主机。勾上点「连上」，App 用你自己的 ssh 和 ssh-agent 登录上去，装好 myco 并连上工作区，每一步打勾。只连一台时，连上后自动切到它。
  - 出错时说清原因，并给一条能在终端里试的命令（比如先 `ssh-add`、先在终端里 `ssh` 一次接受指纹）。
  - App 不读、不存密钥；加入链接只走 SSH 的标准输入，不出现在命令行里。只支持连 Linux 和 macOS。
  - 在 Windows 上，「这台 Windows 里的 WSL」列出这台电脑的发行版，勾上就装进去。
- **电脑页的「检查和修复」**：
  - 「检查」逐项告诉你那台电脑怎么了：myco 版本、后台在不在跑、注销后会不会停、服务用的代理、经代理和直连各能不能连上工作区、日志最后 20 行。比如「经代理连不上工作区，直连是通的」。
  - 还有「重启后台」「升级 myco」两个按钮。不是从 App 连的电脑，先选它对应哪个 SSH 主机或 WSL。
- **新装好后先把这台电脑设好**：工作区第一次启动后，一页列出这台电脑上的 AI（可以安装、登录）和最近的 Claude Code、Codex 会话，勾上的会加进来。设置 › 关于里可以再打开。欢迎页也换了新样子。
- **空聊天里有三个可以问的**，点一下就填进输入框。
- **自动化**：每个自动化有自己的页面，列出每一次运行。Webhook 触发的那次还能看到对方发来的内容（不含凭据、签名）。
- **设置**：左上角可以搜索，导航分成「这台电脑 / 工作区 / 关于」。
- **Linux 上的后台服务注销后不再停**：`myco runner service install` 会顺手打开 linger。以前关掉最后一个 SSH 或 WSL 窗口一会儿后，后台就会停。
- **重新连同一台电脑不再变成「名字-2」**：用新的加入链接再连一次，会认出是同一台电脑。
- **安装时不再卡在 sudo 密码**：从 App 用 SSH 连时，那台电脑 `/usr/local/bin` 里已有旧 myco 的话，改装到 `~/.local/bin`。
- 在服务器这种打不开浏览器的电脑上，Codex 的登录命令改成 `codex login --device-auth`。
- 移除电脑的弹窗不再把电脑自己的节点算成一个 agent。
- 自动化的运行记录在服务器上，要升级 Hub（`sudo myco upgrade`）。「重新连不变成名字-2」和 linger 要**被连的那台电脑**升级（`myco upgrade`）。

## v0.20.17

- **A new look for the computers page**:
  - like "Automations" and "Apps", the top has "Connect a computer" and three numbers (computers online, agents, terminals), with a card for each computer below.
  - each computer opens to its own page: rename it, open a terminal, add an agent; its agents and terminals, the folders it allows work in, the AIs installed on it; and remove it.
- **Fixed "machine not found" when removing old computers**: computers running a very old myco that never reported themselves to the workspace can now be removed too (together with their agents). This needs the hub on the server upgraded (`sudo myco upgrade`).

**中文**

- **电脑页换了样子**：
  - 和「自动化」「应用」一样，最上面是「连一台电脑」和三个数字（在线的电脑、agent、终端），下面每台电脑一张卡片。
  - 点开一台电脑有它自己的页面：改名、开终端、加 agent，上面的 agent 和终端，它允许在哪些文件夹开工、装了哪些 AI，以及移除。
- **修好移除老电脑时报 "machine not found"**：上面的 myco 很老、从没向工作区报过自己的电脑，现在也能移除（会连同上面的 agent 一起移除）。这一条要升级服务器上的 Hub（`sudo myco upgrade`）。

## v0.20.16

- **"Start a new one" on another computer no longer fails with "folder doesn't exist"**: when that computer only allows work in certain folders, leaving the folder empty creates it in an allowed place. The App also waits until it knows which folders that computer allows before you can click "Start it".
- **Sessions on other computers show up as soon as you open it**: when the App starts, it asks each computer for its sessions and remembers them, so Add an agent shows them right away and swaps in the latest in the background. "Details" on the computers page works the same way.
- **Managing computers**:
  - a "Computers" row at the top of the sidebar, in simple mode too.
  - hover over the name on a computer card to rename it.
  - computer names are unique within a workspace: renaming one to another computer's name is refused, and a newly connected computer with a clashing name gets `-2` added.
  - computers connected from WSL are named "hostname-wsl" by default, instead of having the same name as the Windows machine they run on.
- **"This computer" is recognized correctly**: before, it was guessed from the hostname, so when WSL and Windows had the same name it guessed wrong and it looked as if there were duplicate computers. Now it is recognized by this computer's own node.
- The first item needs an upgrade on **the computer where the session is started** (run `myco upgrade` on it). The check for unique computer names runs on the server, so upgrade the hub (`sudo myco upgrade`).

**中文**

- **在别的电脑上「新开一个」不再报"文件夹不存在"**：那台电脑只允许在某些文件夹里开工时，不填文件夹会建在它允许的地方。App 也会等问清那台电脑允许哪些文件夹后，再让「开一个」能点。
- **另一台电脑上的会话打开就有**：App 一启动就先去问各台电脑有哪些会话并记下来，打开添加 agent 时直接显示，后台再换成最新的。电脑页的「详情」也一样。
- **管理电脑**：
  - 侧栏最上面多了一行「电脑」，简单模式里也有。
  - 电脑卡片上鼠标移到名字上就能改名。
  - 一个工作区里的电脑不会重名：改成别的电脑的名字会被拒绝，新连上的撞名电脑自动加 `-2`。
  - 在 WSL 里连上的电脑默认叫「主机名-wsl」，不再和它所在的 Windows 同名。
- **「这台电脑」认得准了**：以前按主机名猜，WSL 和 Windows 同名时会认错，看起来像有重复的电脑。现在按这台电脑自己的节点来认。
- 第一条要**被开会话的那台电脑**升级（在它上面运行 `myco upgrade`）。电脑不重名的检查在服务器上，要升级 Hub（`sudo myco upgrade`）。

## v0.20.15

- **Sessions on other computers show up as soon as you open it**: when Add an agent opens, it asks all online computers at once; each computer remembers its last result and answers immediately (updating in the background as well), and sessions appear row by row, with no idle waiting.
- **Session lists are complete**:
  - before, only the last 30 days and the newest 400 records were read, and sessions started by programs weren't listed at all, so computers that myco used a lot showed almost nothing. Now it looks back half a year, up to 200 sessions.
  - sessions you started come first; ones started by programs (myco itself, scripts, `claude -p`, `codex exec`) are folded at the bottom under "N more sessions started by programs"; click to see them, and searching expands them automatically.
- **The loading hint at the bottom of a chat says what it is doing now**: when a session goes on to handle new messages, it no longer keeps showing the progress and time of the first one.
- The first two items need **the computer being looked at** upgraded to this version (run `myco upgrade` on it). The hub on the server doesn't need an upgrade.

**中文**

- **另一台电脑上的会话一打开就有**：添加 agent 一打开就同时去问所有在线的电脑，那台电脑记着上次的结果，立刻就能回答（后台再顺手更新），会话一行接一行出来，不用再干等。
- **会话列得全了**：
  - 以前只看 30 天内、只读最新的 400 个记录，程序开的会话也直接不列，常被 myco 用的电脑上几乎什么都看不到。现在看半年内的，最多 200 个。
  - 你自己开的会话排在前面；程序开的（myco 自己、脚本、`claude -p`、`codex exec`）折在最下面「还有 N 个是程序开的会话」里，点开就能看，搜索时自动展开。
- **对话底部的加载提示说的是它现在在做什么**：一个会话里接着处理新消息时，不再一直显示最早那件事的进度和用时。
- 前两条要**被看的那台电脑**升级到这个版本（在它上面运行 `myco upgrade`）。服务器上的 Hub 不用升级。

## v0.20.14

- **A "computer" is a real computer**: a laptop, a cloud server (VPS) or a WSL each counts as one. What the sidebar used to call a "computer" (a folder where an agent runs commands, which you can open and watch) is now called a "terminal".
- **Add an agent starts by choosing where it is**: already in the workspace, this computer, each connected computer, connect a computer, or an AI in the cloud or elsewhere.
  - pick a connected computer and it lists the recent Claude Code and Codex sessions **on that computer**; click "Join" to add a copy made on that computer; or "Start a new one" there.
  - when you start a new one, AIs that aren't installed or signed in on that computer are marked, with the command to run on it.
- **Add sessions on cloud servers**: "Connect a computer" gives you a command to paste and run on the VPS; once it is connected, the App switches to it and lists its sessions. If the workspace can only be reached from this computer or the local network, the App says so first; if the workspace is on this computer, you can "Open to the internet" right there.
- **Fixed the folder error in "Start a new one"**: if you don't give a folder, the new folder is created where that computer allows. Computers that only allow some folders say which ones, and a folder outside them is marked red right away instead of failing when the agent starts.
- **Computers page**: one card per computer, with its agents and terminals below; "Details" shows the folders it allows work in, its highest access level, and the AIs installed.
- **Follow-ups stay where you sent them**: a follow-up you send for a task, when other messages came in between, now shows where you sent it (quoting the task it follows up on) instead of being moved up earlier.
- To list a computer's sessions, AIs and allowed folders, **that computer** must be upgraded to this version (run `myco upgrade` on it). The hub on the server doesn't need an upgrade.

**中文**

- **「电脑」就是一台真电脑**：笔记本、云服务器（VPS）、WSL 各算一台。以前侧栏里叫「电脑」的那种文件夹（agent 在里面跑命令、打开能看着）现在叫「终端」。
- **添加 agent 先选它在哪**：工作区里已有的、这台电脑、每台连上的电脑、连一台电脑、云端或别处的 AI。
  - 选了一台连上的电脑，会列出**那台电脑上**最近的 Claude Code、Codex 会话，点「加入」就在那台电脑上复制一份加入；也能在上面「新开一个」。
  - 新开时，那台电脑没装、没登录的 AI 会标出来，并给出要在它上面运行的命令。
- **接云服务器上的会话**：「连一台电脑」给出一条命令，贴到 VPS 上运行，连上后自动切到它、列出它的会话。工作区只在本机或局域网能连上时会先说清楚；工作区在这台电脑上的，可以当场「开放到公网」。
- **修好「新开一个」的文件夹报错**：不填文件夹时，新文件夹建在那台电脑允许的地方。只允许某些文件夹的电脑会写明允许哪些，填了范围外的当场标红，不用等开的时候报错。
- **电脑页**：一台一张卡，下面是它上面的 agent 和终端；「详情」里能看它允许在哪些文件夹开工、最高权限、装了哪些 AI。
- **补充放在发的位置**：对一个任务发的补充，如果中间已经有别的消息，现在显示在你发它的位置（引用它补充的那个任务），不再插到前面去。
- 要列出某台电脑上的会话、AI 和允许的文件夹，**那台电脑**要升级到这个版本（在那台电脑上运行 `myco upgrade`）。服务器上的 Hub 不用升级。

## v0.20.13

- **Linux App**: download `myco-app-linux-amd64` (`-arm64` for ARM), make it executable and run it; the App opens in your default browser and works on X11, Wayland and WSL. The first run adds myco to the Applications menu; opening it again returns to the one already running, and its tabs keep working after restarts and updates; `myco-app --quit` stops it. "Settings → About" updates it in one click. On machines without a system keychain, tokens are stored in a file only you can read.
- **Chat backgrounds**: in the "⋯" at the top right of a chat you can change the chat background: six understated presets, or an image from this computer, which you can fade so the text stays readable; set the default in Settings › Appearance. Stored only on this computer; in split view each chat shows its own.
- **Chats are ordered more clearly in time**: if you send a message while the agent is still working on the previous one, the new message says "Queued behind the current one"; later progress and screenshots go under your message, and the loading animation stays at the bottom of the chat, so it no longer looks as if the agent is working on the new request.

**中文**

- **Linux 版 App**：下载 `myco-app-linux-amd64`（ARM 用 `-arm64`），加上可执行权限运行，App 在默认浏览器里打开，X11、Wayland、WSL 都能用。第一次运行会把 myco 加进「应用程序」菜单；再打开会回到正在运行的那个，标签页在重启、更新后照样能用；`myco-app --quit` 停掉。「设置 → 关于」可以一键更新。没有系统钥匙串的机器，令牌存在只有你能读的文件里。
- **聊天背景**：对话右上角「⋯」里可以换聊天背景：六种素雅的预设，或者用这台电脑上的一张图，可以调淡一些以免看不清字；设置 › 外观里设默认的。只存在这台电脑上，分屏时每个对话各显示各的。
- **对话按时间排得更清楚**：agent 还在做上一件事时你又发了一条，新消息会写「排在后面」；之后的进度、截图都排在你这条消息下面，加载动画停在对话最底部，不会再显得像是在做新的那件事。

## v0.20.12

- **Settings › Agents**: lists Claude Code, Codex, Gemini CLI and opencode on this computer, showing whether each is installed and signed in, and its version. Click "Install" for one that isn't installed (official method or a mirror in China), or "Sign in" for one that isn't signed in; both open a terminal window to do it, and if a terminal can't be opened you get a command to copy. Checks run again when you come back to the App window.
- **Starting a new chat is easier**:
  - the folder is optional; computers on the new version create `~/myco/<分工>` automatically.
  - "More options" lets you pick the model, reasoning effort and access (read-only / this folder / full; "This folder" is still the default).
  - when you start one on this computer and the chosen agent isn't installed or signed in, the form offers "Install / Sign in" right there.
- **Offline reasons are explained**: when an agent is offline, the App says whether its runner was stopped, its WSL was shut down automatically by Windows (with a one-line command to keep WSL running), or the computer is asleep or off the network.
- **Simple mode**: on by default for new users who choose "Start on this computer" on the welcome page; it shows only chats and agents, and hides workspaces, members, other computers and the like for now. Turn it off in Settings › Appearance; users with an existing workspace aren't affected.
- **Split chat view**: in projects and "Conversations" you can open up to four chats at once, splitting wherever you drag them; a chat that is already open won't open a second time.
- **The Mac app is signed and notarized on GitHub**: releases no longer need a Mac online.
- To get offline reasons, the computer that runs the agent needs this version (`myco upgrade`).

**中文**

- **设置 › 本机 agent**：列出这台电脑上的 Claude Code、Codex、Gemini CLI、opencode，看得到装没装、登没登录、什么版本。没装的点「安装」（可以选官方方式或国内镜像），没登录的点「登录」，都会打开一个终端窗口去做；打不开终端就给出命令让你复制。回到 App 窗口会自动重新检查。
- **新开对话更方便**：
  - 文件夹可以不填，新版电脑会自动建 `~/myco/<分工>`。
  - 「更多选项」里可以选模型、思考强度和权限（只读 / 这个文件夹 / 完全放开，默认还是「这个文件夹」）。
  - 在这台电脑上新开时，如果选的 agent 没装或没登录，表单里直接给「安装 / 登录」。
- **离线时说清原因**：agent 离线时，会写明是后台被停了、在 WSL 里被 Windows 自动关掉了（附一行让 WSL 一直开着的命令），还是电脑睡着了或断网了。
- **简单模式**：从欢迎页「在这台电脑上开始」的新用户默认打开，只显示对话和 agent，工作区、成员、其他电脑这些先藏起来。设置 › 外观里可以关掉；已有工作区的用户不受影响。
- **聊天分屏**：项目和「会话」里最多可以同时开四个对话，拖到哪里就分在哪里；已经开着的对话不会再开一个。
- **Mac 版在 GitHub 上签名、公证**：发版不再需要一台 Mac 在线。
- 要用上「离线原因」，跑 agent 的那台电脑要升级到这个版本（`myco upgrade`）。

## v0.20.11

- **"Add an agent → On this computer" is no longer full of duplicate sessions**:
  - sessions run by scripts are no longer listed (`claude -p`, Agent SDK, `codex exec`). They often run the same prompt many times in the same folder, which looks like a row of identical sessions. Ones already added to the workspace still show.
  - of the sessions in the same folder with the same first message, only the newest is listed.
  - sessions from newer Codex versions used to have no title; now they show their first message.

**中文**

- **「添加 agent → 这台电脑上的」不再满是重复的会话**：
  - 脚本跑出来的会话不再列出来（`claude -p`、Agent SDK、`codex exec`）。它们常常在同一个文件夹里把同一句话跑很多次，看起来就是一排一模一样的会话。已经加入工作区的照常显示。
  - 同一个文件夹里、第一句话一样的会话，只列最新的一个。
  - 新版 Codex 的会话以前没有标题，现在能显示第一句话了。

## v0.20.10

- **A cleaner chat header**: only "Find" and "⋯" remain at the top right. Pinned URLs, "Hand over to another session" and "Export this conversation" have all moved into "⋯".
- **Images and files**: a new "Images and files" in "⋯" gathers everything shared in this chat:
  - images in a grid; click one for the original, and flip through with the arrow keys;
  - files one per row, to view or save;
  - each item shows who sent it and when; "Show in chat" jumps back to the original message.
- **Page cards can be pinned above the chat**: right-click a page card in a chat to "Pin above the chat" or "Unpin", or to copy its link.
- **Agents look up apps faster**: `myco template list` now gives a one-line summary per app by default, about 6 KB instead of over 20 KB; filter with `--category` and `--query`; `--full` still gives the full output.

**中文**

- **对话顶栏更清爽**：右上角只留「查找」和「⋯」。置顶网址、换一个来接着做、导出这段对话都放进「⋯」里。
- **图片和文件**：「⋯」里新增「图片和文件」，把这段对话里的东西集中在一起：
  - 图片按网格排，点开是原图，可以用方向键翻看；
  - 文件一行一个，可以直接看或者保存；
  - 每一项都写着谁发的、什么时候，点「在对话中查看」就跳回原来那条消息。
- **网页卡片可以钉在对话上方**：对话里的网页卡片右键可以「置顶到对话上方」或「取消置顶」，也可以复制链接。
- **agent 查应用更快**：`myco template list` 默认每个应用只给一行简介，大约 6 KB，以前要 20 多 KB；用 `--category`、`--query` 可以筛选，`--full` 还是完整输出。

## v0.20.9

- **The "Automations" and "Apps" pages are reorganized**:
  - both pages share the same header and width; the first time you open them a placeholder skeleton shows, and after that they show the last content right away while refreshing in the background.
  - "Automations": stats at the top, then the "Your automations" list; common templates are always shown, including a new "End-of-day notes", plus a "Start from blank" card. Icons and card styles match the "Apps" page.
  - "Apps": search, check for official app updates, recently opened apps, category tabs and shelves.
- **The App opens maximized by default** (Mac and Windows, only the first time the window is shown).

**中文**

- **「自动化」和「应用」两页重新整理**：
  - 两页用同一种页头、同样的宽度；第一次打开时先显示占位骨架，以后再进来直接显示上次的内容，同时在后台刷新。
  - 「自动化」页：上面是统计，下面是「我的自动化」列表；常用模板一直显示，新增了「下班前写日报」，还有一张「从空白开始」卡片。图标和卡片样式与「应用」页一致。
  - 「应用」页：可以搜索、检查官方应用更新，有最近打开的、分类标签和书架。
- **App 打开时默认最大化**（Mac 和 Windows，只在第一次显示窗口时）。

## v0.20.8

- **`myco upgrade` no longer fails because of GitHub rate limits**: before, it found the latest version by asking the GitHub API without signing in. Each IP address can ask only 60 times an hour, so with many machines on one network, or after frequent checks, it failed with `cannot find the latest release ... HTTP 403`. Now, when the API refuses or can't be reached, it checks GitHub's Releases page instead and reads the latest version from the redirect, which has no such limit. This applies to `myco upgrade` on the command line, the background update check, and "Check for updates" in the App. If you hit this error on an older version, first run `myco upgrade --version v0.20.8` (with `sudo` on servers).

**中文**

- **`myco upgrade` 不再因为 GitHub 限流而失败**：以前找最新版本时，是不登录去问 GitHub API 的。同一个 IP 每小时只能问 60 次，网络里机器多、或者刚查得频繁，就会报 `cannot find the latest release ... HTTP 403`。现在 API 被拒或连不上时，会改去看 GitHub 的 Release 页面，从跳转里读出最新版本，没有这个限制。命令行的 `myco upgrade`、后台的更新检查、App 里的「检查更新」都一样。还在旧版本上遇到这个错误的，先用 `myco upgrade --version v0.20.8`（服务器上加 `sudo`）。

## v0.20.7

- **Official apps are released on their own, without waiting for a new myco**: official apps such as Whiteboard, Mind Map, To-do List, Poll, Expense Book, Habit Tracker, Gomoku, Sudoku, Code Editor and Developer Toolbox are now packaged and released separately. The hub fetches new versions 20 seconds after it starts and then daily, verifying their SHA256; the App's "Apps" page has "Check for app updates" to fetch them right away (at most once a minute). Pages already made with an app show a notice when the app has a new version, and can be upgraded to it in one click (CLI: `myco page upgrade`); apps that need a newer hub wait until the hub is upgraded. **Requires the hub on the server to be upgraded to v0.20.7 (`sudo myco upgrade`).**
- **Apps can have their own icon and state the hub version they need**: an app can include an SVG icon in its config, and the App shows that shape (apps without one get the default icon for their type); it can also set `requires.hub`, so a hub that is too old refuses the app and says which version is needed, instead of installing something that won't open. The list of official apps is no longer hard-coded in the App and the skill; use `myco template list` / `myco template get` to see which apps the current workspace has and how to fill them.

**中文**

- **官方应用单独发版，不用等 myco 出新版本**：白板、思维导图、待办清单、投票、记账本、习惯打卡、五子棋、数独、代码编辑器、开发者工具箱这些官方应用现在自己打包发布。Hub 启动 20 秒后和每天会自动去拉新版本，并校验 SHA256；App 的「应用」页有「检查官方应用更新」，点一下马上拉（最多一分钟一次）。已经用某个应用做好的页面，应用有新版本时会提示，可以一键升级到新版本（命令行 `myco page upgrade`）；要求更新 Hub 的应用会等 Hub 升级后再用。**需要服务器上的 Hub 升级到 v0.20.7（`sudo myco upgrade`）。**
- **应用可以带自己的图标、注明需要的 Hub 版本**：应用可以在自己的配置里放一个 SVG 图标，App 里就按它的形状显示（没写的按类型显示默认图标）；还能写 `requires.hub`，Hub 太旧时会拒绝并说明需要哪个版本，不会出现装上却打不开。官方应用列表不再写死在 App 和 skill 里，用 `myco template list` / `myco template get` 看当前工作区有哪些应用、怎么填。

## v0.20.6

- **Apps are on shelves**: the "Apps" page groups the official apps on shelves (Office, Tools, Life, Creative, Games, Development), each app with its own icon; in the sidebar, "Automations" and "Apps" are two pinned rows that open in one click, with the current page highlighted.
- **Creative: Whiteboard, Mind Map**
  - Whiteboard: an infinite canvas with sticky notes, text, rectangles, ellipses and arrows between them; drag, zoom, pan, change colors, undo deletions, export SVG; agents can add sticky notes too.
  - Mind Map: Tab adds a subtopic, Enter adds a sibling, Space collapses; drag a topic onto another to move it underneath; export as a Markdown outline.
- **Games: Gomoku, Sudoku**
  - Gomoku: 15×15, for people and agents; two people moving at the same time won't overwrite each other; five in a row is detected automatically; undo, resign and start over.
  - Sudoku: everyone fills in the same board, each in their own color, with mistakes marked red; three difficulty levels, each puzzle has exactly one solution, and you can ask for a hint.
- **Everyday apps: To-do List, Poll, Expense Book, Habit Tracker**
  - To-do List: press Return to add; understands "明天" (tomorrow), "周五" (Friday), "@名字" (@name) and "!"; tick items off, set due dates and assignees, add notes, drag to reorder, and view All / Mine / Today / Important.
  - Poll: one question, one vote per person, and voting again changes your vote; live counts, shows who voted for what (or anonymous), single or multiple choice, and the person who started it can close the poll.
  - Expense Book: record spending and income by month, and see how much budget is left, where the money went by category (donut chart) and what was recorded each day; export to CSV.
  - Habit Tracker: everyone checks in for themselves and sees this week and their streak, and can also see others' check-ins.
- **Development: Code Editor, Developer Toolbox**
  - Code Editor: built on Monaco (VS Code's editor), with a file tree, tabs, syntax highlighting, find and replace, multiple cursors and a minimap; saves automatically after a pause or on Ctrl+S, and changes others make show up in the files you have open; previews HTML directly (with its own CSS/JS), and Markdown too; download the whole project as a zip.
  - Developer Toolbox: JSON, Base64, URL, MD5/SHA, timestamps, UUID, JWT, regex, line diff, case conversion, number bases and colors, all computed in the page; saved inputs are shared with everyone.
- **How docs look**: when you widen the panel, the title area resizes to match instead of leaving a blank gap; code blocks have a light gray background and the same size as body text, instead of a big black block.
- **Fixed the publish size limit**: publishing a page or an app used to fail as "message too large" above 256 KB (every bundled app is bigger than that); pages and apps now have their own 50 MB limit. **Requires the hub on the server to be upgraded to v0.20.6 (`sudo myco upgrade`).**

**中文**

- **应用分书架了**：「应用」页把官方应用按书架分类（办公套件、效率工具、生活、创意设计、游戏、编程开发），每个应用有自己的图标；侧栏里「自动化」和「应用」变成置顶的两行，点一下就进，当前页有高亮。
- **创意设计：白板、思维导图**
  - 白板：一块无限大的画布，便签、文字、矩形、椭圆和它们之间的箭头；可以拖动、缩放、平移、改颜色、撤销删除、导出 SVG；agent 也能往上贴便签。
  - 思维导图：Tab 加子主题、Enter 加同级、Space 折叠，拖到别的主题上就换到它下面；能导出 Markdown 大纲。
- **游戏：五子棋、数独**
  - 五子棋：15×15，人和 agent 都能下，两个人同时下也不会互相盖掉；自动判五连，可以悔棋、认输、重开。
  - 数独：大家在同一张盘上填，每人一种颜色，填错标红；三档难度，每张题只有一个答案，可以要一个提示。
- **日常应用：待办清单、投票、记账本、习惯打卡**
  - 待办清单：回车就加，能听懂「明天」「周五」「@名字」「!」；可勾选、设截止日期和负责人、写备注、拖动排序，按全部 / 我的 / 今天 / 重要查看。
  - 投票：一个问题，一人一票，再投就改票；实时计数，显示谁投了什么（也可以匿名），单选或多选，发起的人可以结束投票。
  - 记账本：按月记支出和收入，看预算还剩多少、钱花在哪类（环形图）、每天记了什么，能导出 CSV。
  - 习惯打卡：每人打自己的卡，看本周和连续天数，也能看到别人打过的点。
- **编程开发：代码编辑器、开发者工具箱**
  - 代码编辑器：用的是 Monaco（VS Code 的编辑器），有文件树、标签页、语法高亮、查找替换、多光标和缩略图；停顿一下或 Ctrl+S 自动保存，别人改的文件会出现在你打开的文件里；HTML 能直接预览（带它自己的 CSS/JS），Markdown 也能；整个项目可以打包成 zip 下载。
  - 开发者工具箱：JSON、Base64、URL、MD5/SHA、时间戳、UUID、JWT、正则、行差异、命名转换、进制和颜色，全在页面里算；保存的输入大家共享。
- **文档的样子**：面板拉宽时标题的高度跟着变，不再留一块空白；代码块改成浅灰底、和正文一样大，不再是一大块黑色。
- **修好了发布大小限制**：发布页面或应用时，以前超过 256 KB 就会被当成「消息太大」而失败（打包好的应用都会超），现在页面和应用用自己的 50 MB 限制。**需要服务器上的 Hub 升级到 v0.20.6（`sudo myco upgrade`）。**

## v0.20.5

- **Apps: Sheets, Bases, Docs, Slides**: there is a new "Apps" under "Automations" in the sidebar. Pick one to create a new file in the panel beside the chat, optionally with sample data, and put it in a project. You and agents can both edit, and each sees the other's changes right away.
  - **Sheets / Bases**: spreadsheets like those in Lark (Feishu).
    - columns have types: single- and multi-select colored tags you can pick, search and create right in the cell, plus dates and people.
    - paste a whole block from Excel, drag the fill handle, resize columns, drag rows and columns; the first column stays fixed.
    - every column and row has a menu; filter, sort, search with Ctrl+F; export to Excel and CSV.
    - Bases also have kanban and calendar views.
  - **Docs**: a "/" menu, Markdown shortcuts, a formatting toolbar, and paragraphs you can drag.
  - **Slides**: agents write each slide's layout and content (cover, bullet points, charts, number cards, tables, quotes and more), with four themes to choose from.
    - drag slides to reorder them, and edit text in place;
    - present full screen;
    - export to editable PowerPoint and to a Markdown outline.
  - **How agents use them**: `myco page data` reads and writes an app's content, and `watch` shows what people change in real time; with `myco template` you can make your own apps and publish them to the workspace for everyone.
  - **Requires the hub on the server to be upgraded to v0.20.5 (`sudo myco upgrade`).**
- **The App finds the claude and codex you installed**: an App opened from the Dock doesn't get your terminal's PATH. So claude installed with npm, nvm, volta, bun and the like used to fail at "On this computer → Join" (claude not found), and the runner service the App installed couldn't find it when woken either. Now the App reads the PATH your terminal uses, and also looks in these tools' usual install locations. **If the App installed the runner service for you before, run `myco runner service install` once in a terminal after upgrading.**
- **More accurate error messages**: problems such as failing to copy a session or not finding claude no longer show as "can't connect right now"; the message says what the problem is and what to check.
- Mac signing script: when submitting for notarization fails, it says why.

**中文**

- **应用：表格、多维表格、文档、幻灯片**：侧栏「自动化」下面多了「应用」。选一个就能在聊天旁边的面板里新建一份，可以带示例数据，放进某个项目。你和 agent 都能编辑，各自的改动对方马上能看到。
  - **表格 / 多维表格**：类似飞书的表格。
    - 列有类型：单选、多选的彩色标签可以直接在格子里选、搜、新建，还有日期、人员。
    - 可以从 Excel 粘贴一整块、拖填充柄、调列宽、拖动行和列，第一列固定。
    - 每一列、每一行都有菜单；可以筛选、排序、Ctrl+F 搜索；能导出 Excel 和 CSV。
    - 多维表格还有看板和日历视图。
  - **文档**：「/」菜单、Markdown 快捷键、格式工具栏、拖动段落。
  - **幻灯片**：agent 写好每页的版式和内容（封面、要点、图表、数字卡片、表格、引用等），可以选四套主题。
    - 页面可以拖动排序，文字原地改；
    - 可以全屏演示；
    - 能导出可编辑的 PowerPoint 和 Markdown 大纲。
  - **agent 怎么用**：`myco page data` 读写一份应用里的内容，`watch` 能实时看到人改了什么；`myco template` 可以做自己的应用，发布到工作区里给大家用。
  - **需要服务器上的 Hub 升级到 v0.20.5（`sudo myco upgrade`）。**
- **App 找得到你装的 claude 和 codex 了**：从程序坞打开的 App 拿不到终端里的 PATH。所以用 npm、nvm、volta、bun 这些装的 claude，以前在「这台电脑上的 → 加入」时会失败（找不到 claude）；App 帮你装的 runner 后台服务被唤醒时也找不到。现在 App 会自己读出你终端里用的 PATH，再去这些工具的常见安装位置找。**以前由 App 装过 runner 服务的，升级后在终端里运行一次 `myco runner service install`。**
- **出错提示说得更准**：复制会话失败、找不到 claude 这类问题，不再显示成「暂时连不上」，而是直接说是什么问题、该查什么。
- Mac 签名脚本：公证提交失败时会写明原因。

## v0.20.4

- **A tidier sidebar**: Automations, Projects, Chats (previously "Chat") and Computers now use the same kind of heading, and each can be collapsed. Right-click a chat to pin it to the top; "+" adds an agent to talk to.

**中文**

- **侧栏更整齐**：自动化、项目、对话（原来叫「聊天」）、电脑四个分组用同一种标题，都能折叠；对话可以右键钉在最上面，「+」直接添加要聊的 agent。

## v0.20.3

- **One session in several workspaces, without mixing them up**:
  - When `myco join` finds that the session is already in another workspace, it joins as a copy by default (each with its own session and memory) and tells you which agent it already is, and where. To share one session, add `--same-session`.
  - When one session is shared, `myco inbox wait` and `inbox pull` receive messages from all its workspaces and say which workspace each came from; replies and progress automatically use the identity of the message they answer.
  - The runner never resumes the same session for two workspaces at once.
  - In the App, "On this computer" shows which workspaces a session has joined.
- **Pin things above a chat**: Pin a URL, a file or a message above a chat as a row of tabs, and click one to open it (myco pages and local services open in the side panel, files open for viewing, and a message is scrolled to and highlighted). A message's menu can pin the message or its attachments, and the pin button in the top bar pins a URL; drag tabs to reorder them, right-click to rename or unpin. Agents can pin too: `myco pin add|list|rename|rm`. **Requires the hub on the server to be upgraded to v0.20.3 (`sudo myco upgrade`).**
- **The sidebar is rearranged**:
  - Projects come first;
  - "Chat" lists the people and agents you can talk to; click one to open the chat. Sort by recently talked to, busy first, or name;
  - Computers come last and can be collapsed;
  - Automations take one row under search; "Manage members" moved into the chat list's menu.
- **Expanded long messages no longer collapse by themselves**: Before, the long message you were reading could suddenly collapse when a message above it was deleted, earlier messages loaded, or a reply had just finished.

**中文**

- **一个会话加入多个工作区，不再串**：
  - 命令行 `myco join` 发现这个会话已经在别的工作区里时，默认复制一份加入（各用各的会话和记忆），并说明它已经是哪里的哪个 agent；想共用同一个会话加 `--same-session`。
  - 共用一个会话时，`myco inbox wait`、`inbox pull` 同时收所有工作区的消息，标明是哪个工作区的；回复、进展会自动用这条消息所属的身份。
  - runner 不会同时为两个工作区续同一个会话。
  - App 的「这台电脑上的」里写明它加入了哪些工作区。
- **对话上方可以钉东西**：网址、文件或某条消息都能钉在对话上方，一行标签，点一下打开（myco 网页和本机服务在旁边的面板里打开，文件直接看，消息跳过去高亮）。消息的菜单里能钉它或它的附件，顶栏的钉子按钮能钉网址；标签可以拖动排序、右键改名或取消。agent 也能钉：`myco pin add|list|rename|rm`。**需要服务器上的 Hub 升级到 v0.20.3（`sudo myco upgrade`）。**
- **侧栏重新排了**：
  - 项目放在最前面；
  - 「聊天」下面直接列出能聊的人和 agent，点一下就进对话，可以按最近聊过、忙的在前或名字排序；
  - 电脑放在最后，可以折叠；
  - 自动化在搜索下面占一行；「管理成员」挪到了聊天列表的菜单里。
- **展开的长消息不会自己收起来了**：以前上面删了一条消息、加载更早的消息，或者一条回复刚刚写完时，你正在看的那条长消息会突然收起。

## v0.20.2

- **"On this computer" sessions are no longer missing or duplicated**: When you add an agent, the App lists local Claude Code and Codex sessions, and the same conversation could show up several times. Copies left by "Join as a copy", forked sessions, or one session recorded under two project folders were each counted separately. These copies also used up the list's slots and pushed other sessions out. Now each conversation shows once (its newest copy); if any copy has joined, it is marked "Already joined" with the agent's name. The list holds up to 60 conversations. Sessions that newer Claude Code versions keep under `~/.config/claude` are found too.

**中文**

- **「这台电脑上的」会话不再缺、不再重复**：添加 agent 时列出的本机 Claude Code、Codex 会话，以前同一段对话可能出现好几次。比如「复制一份加入」留下的副本、fork 出来的会话，或者同一个会话记在两个项目文件夹下，都会各算一个。这些副本还会占满名额，把别的会话挤掉。现在同一段对话只显示一次（最新的那份），任何一份加入过就标「已经加入了」，并写明是哪个 agent。列表最多 60 段对话。新版 Claude Code 放在 `~/.config/claude` 下的会话也能找到了。

## v0.20.1

- **Let an agent install myco for you**: If you already use Claude Code, Codex or another AI that can run commands, send it one sentence. It reads an install guide written for agents ([docs/agent-setup.md](https://github.com/chenhg5/myco/blob/main/docs/agent-setup.md)), sets up the hub, connects the App, adds computers and agents, and troubleshoots on its own; it asks you first wherever you need to decide, or where it needs sudo or must change system settings. You'll find the sentence:
  - on the App's first screen ("Let my agent set it up");
  - in the "Add workspace" and "Add a computer" dialogs;
  - at the end of `install-hub.sh` and `myco hub up`;
  - at the top of the README.
- **Let an agent look into errors**: Error messages in the App now have "Ask my agent". It copies the original error together with the "Troubleshooting" section of the install guide; just send that to your agent.
- **Voice calls stay on when you leave the chat**: A call is no longer tied to the chat's input bar. On other pages, a small badge in the top bar shows the call's status; you can mute or hang up there, and click the name to go back to the chat. There is only one call at a time: calling another agent hangs up the current call first.
- **A smoother command line**:
  - **Ready to use after joining**: Outside an agent's own session, if this computer has only one agent, commands such as `myco doctor` and `myco agent list` use it directly; with several, pick one with `MYCO_AGENT=<name>`.
  - **Open App links directly**: `myco console link --open` has the App on this computer (macOS, Windows) add the workspace directly, with no copy and paste; `--dry-run` now works on this command too.
  - **Without user-level systemd** (containers, some VPSes): `myco hub up` gives you a command that works (`--no-service`) instead of only reporting "Failed to connect to bus".
  - **A warning for links that only say 127.0.0.1**: When a generated link won't work from other computers, you are warned and told to set `MYCO_LAN_ADDR=<address>` or open a tunnel.

**中文**

- **让 agent 帮你装 myco**：已经在用 Claude Code、Codex 或别的能跑命令的 AI，就把一句话发给它。它会读一份专门写给 agent 的安装指南（[docs/agent-setup.md](https://github.com/chenhg5/myco/blob/main/docs/agent-setup.md)），帮你开 Hub、连上 App、加电脑和 agent，出了问题自己排查；要你拿主意、要用 sudo 或改系统设置的地方会先问你。这句话放在这些地方：
  - App 的第一屏（「让 agent 帮我装」）；
  - 「添加工作区」和「加一台电脑」的弹窗里；
  - `install-hub.sh` 和 `myco hub up` 装完时；
  - README 的开头。
- **出错时让 agent 帮你查**：App 的出错提示里多了「让 agent 帮我查」，会把错误原文和安装指南的「排错」一节一起复制，发给你的 agent 就行。
- **语音通话离开对话也不挂断**：通话不再跟着对话的输入栏。去别的页面时，顶栏有一个小标记，显示通话状态，可以静音、挂断，点名字回到对话。同一时间只有一通电话，给另一个 agent 打电话会先挂掉前一通。
- **命令行更顺手**：
  - **加入后直接能用**：在 agent 自己的会话外面，这台电脑上只有一个 agent 时，`myco doctor`、`myco agent list` 这些命令会直接用它；有好几个时用 `MYCO_AGENT=<名字>` 选。
  - **App 链接直接打开**：`myco console link --open` 直接让本机的 App（macOS、Windows）加上这个工作区，不用复制粘贴；`--dry-run` 在这条命令上也生效了。
  - **没有用户级 systemd 时**（容器、部分 VPS）：`myco hub up` 会直接给出能跑的命令（`--no-service`），不再只报「Failed to connect to bus」。
  - **链接只写着 127.0.0.1 时会警告**：生成的链接别的电脑用不了时会提示，建议用 `MYCO_LAN_ADDR=<地址>` 指定，或者开隧道。

## v0.20.0

That wraps up the 0.19 series. The main features added in recent versions (see each version below for details):
- **Automations**: scheduled tasks and webhooks (v0.19.22);
- **Export chat history** (v0.19.22);
- **Hand over to another session**: the agent behind a chat can be swapped (v0.19.23);
- **Pages made by agents open in the App**, with live preview (v0.19.24, v0.19.25);
- **New ways to install the hub on a server**: behind an existing web server, through a tunnel, or over plain HTTP (v0.19.24).

Changes in this version:
- **The App no longer keeps the CPU busy after you open a page**: Every refresh of the page panel's tabs redrew the whole panel, so from the first page you opened, the App's main thread ran at full load, even after you closed the panel. Now nothing is redrawn when nothing has changed.

**中文**

0.19 系列告一段落。最近几版加的主要能力，详见下面各版：
- **自动化**：定时任务和 webhook（v0.19.22）；
- **导出聊天记录**（v0.19.22）；
- **换一个来接着做**：会话背后的 agent 可以换（v0.19.23）；
- **agent 做的网页在 App 里打开**，可以实时预览（v0.19.24、v0.19.25）；
- **服务器装 Hub 的新装法**：放到已有的网站服务器后面、用隧道、直接 HTTP（v0.19.24）。

这一版的改动：
- **打开过网页后 App 不再一直占满 CPU**：网页面板的标签每次刷新都会让面板重画一遍，App 的主线程从打开第一个网页起就一直跑满，关掉面板也一样。现在没有变化就不重画。

## v0.19.25

- **The pages panel can be tucked away**: "Tuck away" hides the panel but keeps the open pages and any half-done work in them, and the chat gets the full width back; click "Pages N" at the right end of the top bar to bring it back as it was. Esc, clicking beside it in a narrow window, or opening a file also tucks it away first; × closes all pages (it says "Close all N pages"). When you drag the panel's edge to resize it, the chat follows, with no gap and nothing covered; the thin white line beside pages is gone too.
- **Two live preview problems fixed**: With the hub behind Caddy (HTTPS), opening a live page in the App reported "Can't reach the workspace's hub"; and after a dev server (such as Vite) closed idle connections, the next request hung. Both work now, and large files (several MB) come through in full. **Requires the hub on the server to be upgraded to v0.19.25 too (`sudo myco upgrade`).**

**中文**

- **网页面板可以收起**：「收起」把面板藏起来，打开着的网页和里面做了一半的东西都保留，聊天拿回整个宽度；顶栏右端的「网页 N」点一下就原样回来。Esc、在窄窗口里点旁边、打开一个文件，也会先收起它；× 关掉全部网页（会写「关闭全部 N 个网页」）。拖面板边缘调宽窄时，聊天跟着变，不再留空隙或被盖住；网页旁边那条细白线也去掉了。
- **实时预览修好了两个问题**：Hub 在 Caddy 后面（HTTPS）时，App 打开实时网页会报「连不上工作区的 Hub」；开发服务器（比如 Vite）关掉空闲连接后，下一次请求会卡住。现在都正常，大文件（几 MB）也能完整转过来。**需要服务器上的 Hub 也升级到 v0.19.25（`sudo myco upgrade`）。**

## v0.19.24

- **Pages made by agents open in the App**: When a message or progress update links to a workspace page (published by an agent with `myco page publish`), a card appears below it (name, version, who published it); click it to open the page in a panel beside the chat. Pages run in an isolated sandbox with no access to the App or the internet; links to other sites open in your system browser. With several pages open, they appear as tabs at the top of the panel.
  - **Work on a page together with the agent**: A page can keep shared data (the agent reads and writes it with `myco page state get/set`, and the App picks up others' changes every 2 seconds), and can send messages to its agent (it asks you the first time; at most 5 a minute).
  - **Live preview**: `myco page serve --port 5173 --name …` brings a dev server running on the agent's computer into the App, hot reload included; the card says "Live · online / offline".
- **Errors explain the cause and can be retried**: When the workspace can't be reached or answers too slowly, the message no longer says "will retry automatically" (it didn't). It tells you to click "Retry" or try again in a moment, and if it keeps happening, to check the connection status at the bottom left or the network proxy. "Details" shows the original error (such as a timeout or a refused connection) and lets you copy it to send to us. In "Add an agent", a failed link now has a "Retry" button.
- **Workspaces on your local network no longer go through the proxy**: With the network proxy set to "Follow system", local network addresses (such as 192.168.x.x) were also sent to the proxy from environment variables, which often broke the connection. Now local and LAN addresses connect directly under all three settings.
- **More ways to install the hub on a server** (`install-hub.sh`): Run without arguments, it first checks what is using port 443, then asks how you want to install, and at the end prints a command you can use directly next time.
  - Automatic HTTPS (as before, Caddy on 443);
  - Behind an existing nginx / BT Panel / 1Panel / Caddy: `--behind nginx|panel|caddy|other --domain <domain>`. For nginx it writes a separate site file, enabled only after it passes validation; for panels it only prints the settings to enter.
  - No open ports, through a tunnel: `--tunnel cloudflare --hostname <domain>`, `--tunnel cloudflare` (temporary address) or `--tunnel tailscale`.
  - Plain HTTP on a port you choose (for internal networks and testing only): `--http --port 7777`.

  After installing, it checks itself from the server, and if something doesn't connect, says which layer it's stuck at (service, firewall, cloud provider security group, certificate, DNS, tunnel); run `--check` to check again at any time.
- **Caddy's timeout is now 6 minutes**: "Start a new session on a computer" can take up to 5 minutes, but Caddy used to cut it off after 2. Running `sudo myco upgrade` on the server updates the site files the script installed (ones you edited by hand are left alone).
- While a reply is appearing bit by bit, images and page cards show up once the text has finished; when a new message arrives below, the one above finishes showing at once, and several replies arriving together play out in order.

**中文**

- **agent 做的网页在 App 里打开**：消息或进展里有工作区网页的链接（agent 用 `myco page publish` 发布的），下面会出现一张卡片（名字、版本、谁发的），点开在聊天旁边的面板里显示。网页运行在隔离的沙箱里，碰不到 App 和外网；网页里的外链用系统浏览器打开。打开好几个时，面板上方是标签页。
  - **网页可以和 agent 一起改**：网页能存一份共享数据（agent 用 `myco page state get/set` 读写，App 里每 2 秒同步别人的改动），也能给它的 agent 发消息（第一次会先问你，每分钟最多 5 条）。
  - **实时预览**：`myco page serve --port 5173 --name …` 把 agent 那台电脑上正在跑的开发服务器转到 App 里，热更新也能用；卡片上写着「实时 · 在线 / 离线」。
- **出错时说清原因、能重试**：连不上工作区或者它回得太慢时，提示不再说「会自动重试」（其实不会），而是告诉你点「重试」或者等一下再试，一直这样就去看左下角的连接状态或网络代理。点「详细信息」能看到并复制原始错误（比如超时、连接被拒），方便发给我们。「添加 agent」里生成链接失败时有「重试」按钮。
- **局域网里的工作区不再走代理**：网络代理设成「跟随系统」时，以前局域网地址（如 192.168.x.x）也会被环境变量里的代理接走，常常就连不上了。现在三种设置下，本机和局域网地址都直连。
- **服务器装 Hub 有了更多装法**（`install-hub.sh`）：不带参数运行会先看 443 端口被谁占着，再问你怎么装，最后给出下次可以直接用的命令。
  - 自动 HTTPS（和以前一样，Caddy 用 443）；
  - 放到已有的 nginx / 宝塔 / 1Panel / Caddy 后面：`--behind nginx|panel|caddy|other --domain 域名`。nginx 会单独写一个站点文件，校验通过才生效；面板只打印要填的设置。
  - 不开任何端口，用隧道：`--tunnel cloudflare --hostname 域名`、`--tunnel cloudflare`（临时地址）或 `--tunnel tailscale`。
  - 直接 HTTP、自选端口（只适合内网和测试）：`--http --port 7777`。

  装完会从这台服务器上自检，不通时说清卡在哪一层（服务、防火墙、云厂商安全组、证书、域名解析、隧道）；`--check` 可以随时再查。
- **Caddy 的超时改成 6 分钟**：「在一台电脑上新开会话」最多要等 5 分钟，以前 Caddy 2 分钟就断了。服务器上运行 `sudo myco upgrade` 会自动改好脚本装的站点文件（手改过的不动）。
- 一条回复慢慢显示出来时，图片和网页卡片等文字出完再出现；下面来了新消息，上面那条会直接显示完，几条回复同时到也按顺序读。

## v0.19.23

- **Hand over to another session**: When the session behind an agent can't go on (the claude.ai subscription ended, that computer is gone, or you want to switch from Claude to Codex), you don't need a new chat: choose "Hand over to another session" under "Manage" on the member card, from ⇄ in the chat's top bar, or from the right-click menu. The name, chat history, projects and aliases stay the same; only the session behind it changes. There are two ways to take over:
  - Start a new Claude Code / Codex session on a connected computer;
  - Get a one-time command (valid for 24 hours) and run it in the session that takes over; good for moving to another claude.ai account.
  
  The new session first reads a handover note: which computer and folder the old one was in, unfinished work, and the last 30 messages. The old session stops and gets no more messages; its unfinished tasks are handed to the new one to redo. The chat shows a divider where the handover happened; under "Handovers" on the member card you can "Switch back", and switching back only fills in the messages from while it was away. If you are only switching accounts and the session is still on this computer, the dialog points you to "Run settings" to change the provider, with all context kept. Command line: `myco agent handover`, `myco agent handover-link`, `myco join --take-over`. Not supported yet in end-to-end encrypted workspaces. **Requires the hub on the server to be upgraded to v0.19.23 (`sudo myco upgrade`)**; to take over with a new session on a computer, myco on that computer must be upgraded to v0.19.23 too.
- **A clearer Automations page**: With no automations yet, it offers a few common starting points (a morning summary, a Friday weekly report, a regular check, reading the logs when CI fails). Each row shows when it runs, who it goes to, when it runs next and how the last run went; the switch turns it off directly, and on hover you can "Send now", which is also in the right-click menu. The editor is laid out as "to whom, what to say, when"; the save button stays at the bottom, and anything missing is noted next to it.
- **Agents can publish pages** (command line first): `myco page publish --dir <folder>` publishes a folder (or a single html file) to the hub; each publish is a new version (the last 10 are kept). Only its owner, the owner's agents, admins and, when published to a project, the people in the project can see it. The panel in the App comes later. Not supported in end-to-end encrypted workspaces.

**中文**

- **换一个来接着做**：agent 背后的会话没法继续了（claude.ai 订阅到期、那台电脑没了、想从 Claude 换成 Codex），不用新开对话：在成员卡片的「管理」、对话顶栏的 ⇄、或右键菜单里选「换一个来接着做」。名字、聊天记录、项目、别名都不变，只换背后的会话。两种接法：
  - 在一台已连上的电脑上新开一个 Claude Code / Codex 会话；
  - 拿一条一次性的命令（24 小时内有效），到要接手的会话里运行，适合换到另一个 claude.ai 账号。
  
  新会话先读一份交接说明：原来在哪台电脑哪个目录、还没做完的事、最近 30 条对话。原来的会话会停下，不再收消息；它手上没做完的任务交给新的重新做。对话里在接手的位置有一条分隔线；成员卡片的「接手记录」里可以「换回这个」，换回去时只补上它离开期间的消息。只是换账号、会话还在本机的，弹窗里会引你去「运行」里换服务商，上下文都在。命令行：`myco agent handover`、`myco agent handover-link`、`myco join --take-over`。端到端加密的工作区暂不支持。**需要服务器上的 Hub 升级到 v0.19.23（`sudo myco upgrade`）**；在电脑上新开会话接手时，那台电脑的 myco 也要升级到 v0.19.23。
- **自动化页更清楚**：还没有自动化时给几个常用的起点（早上的总结、周五周报、定期检查、CI 失败时看日志）。每行写明什么时候、发给谁、下次是什么时候、上次跑得怎么样；开关直接停用，鼠标移上去可以「立刻发一次」，右键菜单里也有。编辑器按「发给谁、说什么、什么时候」排；保存按钮固定在底部，缺什么就在按钮旁边写出来。
- **agent 可以发布网页**（命令行先行）：`myco page publish --dir <文件夹>` 把一个文件夹（或一个 html 文件）发到 Hub 上，每次发布是一个新版本（保留最近 10 个）。只有它的主人、主人的 agent、管理员，以及发布到项目时项目里的人能看到。App 里的面板稍后跟上。端到端加密的工作区不支持。

## v0.19.22

- **Automations: scheduled tasks and webhooks**: A new "Automations" page in the sidebar. An automation says which agent to send to, what to say, and when (every day, weekdays, a day of the week, every few hours, or your own cron, in the time zone you choose); you can also give it a webhook URL so another system can trigger a send by calling it (the call's content fills `{{payload}}` in the message). The hub sends on schedule, even when your computer is off. What it sends is an ordinary task, and the reply shows up in your chat with that agent, marked "Auto". It can skip a run while the last one is still unfinished, you can "Run it" right away, and you can see how each recent run went. Command line: `myco automation list|show|create|update|remove|run`. **Requires the hub on the server to be upgraded to v0.19.22 (`sudo myco upgrade`)**; on older hubs this page doesn't show.
- **Export chat history**: The export button in the chat's top bar saves a chat as Markdown (for people: reading, sharing, archiving) or JSON Lines (for programs: one message per line), optionally with the tasks' steps. Command line: `myco thread export --id <chat> --as md|jsonl`.
- **Model and effort right by the input box**: Below the input box you see the model and effort this agent uses (e.g. "GPT-6 Sol · High"). For agents woken by the runner that you can manage, click it to switch; the change applies from the next run. For agents in their own sessions it shows the model they report (myco on their side must be upgraded to v0.19.22), or "Model unknown" if it isn't known yet.
- **Files open on the right while you keep chatting**: When the window is wide enough, Markdown, web pages, logs and other files open in a split on the right instead of covering the chat; scrolling, typing and sending don't close it, and you can drag its edge to resize it. In full screen or a narrow window it still opens on top.
- **Network proxy**: In Settings › Network proxy, choose to follow the system environment, use no proxy, or enter your own (SOCKS5 / HTTP / HTTPS, optionally with a username and password; the password is stored in Keychain). "Try it" tests before you save, and saving takes effect immediately; local and LAN addresses always connect directly.
- When an agent puts a task aside and then says what it is waiting for, it no longer shows as working all along, and the input box no longer stays on "Add".
- Checkboxes now match the rest of the App, and "When" in the automation editor sits in a card, for a neater layout.

**中文**

- **自动化：定时任务和 webhook**：侧栏新的「自动化」页。设一条自动化：发给哪个 agent、说什么、什么时候发（每天、工作日、每周某天、每几小时，或者自己写 cron；按你选的时区），也可以给它开一个 webhook 地址，别的系统一调用就发一次（调用带的内容会填进消息里的 `{{payload}}`）。由 Hub 按时发，你的电脑关着也照常；发出的是一条普通任务，回复出现在你和那个 agent 的对话里，标着「自动」。可以设成上一条还没做完就跳过，能立刻「运行一次」，能看最近每次运行的结果。命令行：`myco automation list|show|create|update|remove|run`。**需要服务器上的 Hub 升级到 v0.19.22（`sudo myco upgrade`）**，旧 Hub 上这一页不显示。
- **导出聊天记录**：对话顶栏的导出按钮，可以存成 Markdown（给人看、分享、存档）或 JSON Lines（给程序用，一条消息一行），可选带上任务的执行步骤。命令行：`myco thread export --id <对话> --as md|jsonl`。
- **模型和思考强度就在输入框旁边**：输入框下方显示这个 agent 用的模型和强度（例如「GPT-6 Sol · 高」）。由 runner 唤醒、你能管理的 agent，点它直接切换，下次运行生效；自己会话里的 agent 显示它报上来的模型（需要那边的 myco 升级到 v0.19.22），还不知道时显示「模型未知」。
- **打开的文件在右边，聊天照常用**：窗口够宽时，Markdown、网页、日志等文件在右侧分栏打开，不再盖住聊天，滚动、打字、发送都不会关掉它；边缘可以拖宽窄。全屏或窗口太窄时还是覆盖显示。
- **网络代理**：设置 › 网络代理，可以选跟随系统环境、不用代理，或者自己填（SOCKS5 / HTTP / HTTPS，可带用户名密码，密码存在钥匙串里）。「试一下」先测再保存，保存后立即生效；本机和局域网地址始终直连。
- agent 先放下任务、再说明在等什么时，不再显示成一直在干活，输入框也不再停在「补充」。
- 勾选框换成和 App 一致的样式，自动化编辑里的「什么时候」放进一张卡片，排得更整齐。

## v0.19.21

- **Undelivered messages explained in plain words, with one-click resend**: When an agent picked up a message several times but never started on it, the bubble used to show a raw English line ("delivery attempts exhausted…"). Now it says "Not delivered" and explains that the agent's session may not be running or may be stuck, with "Send again" next to it, which sends the same content again in the same chat. Failures for other reasons can be resent the same way.
- Removed "…" from the right-click menu of computers in the sidebar.

**中文**

- **没送到的消息说人话，能一键重发**：消息被 agent 取走几次都没开始处理时，以前气泡下面是一段英文（「delivery attempts exhausted…」）。现在写「没送到」，并说明可能是它那边的会话没在运行或卡住了，旁边有「重新发送」，点一下把同样的内容在同一个对话里再发一次。其他原因的失败也一样能重新发送。
- 侧栏电脑的右键菜单去掉了「…」。

## v0.19.20

- **All of a computer's settings in one place**: Right-click a computer under "Computers" in the sidebar (or hover and click ⋯) to open a terminal, rename it, choose who can use it and which projects it is in, or remove it. In the computer's settings, there is one checkbox per workspace agent, and only checked agents can run commands in this folder; click × on a project tag to take it out of that project, and "Put it in a project…" to add it; removing asks you first and leaves the folder and past chats untouched. Each row on the Computers page has a "Settings" button, and an open terminal has a gear at the right of its top bar; both open the same settings.
- **For sessions that stay open, steps show in the right place**: If you send a new message while the agent is working, the steps and progress as it carries on show under your new message, instead of all piling up in the previous task with only a spinner under your new message.

**中文**

- **电脑的设置都在一个地方**：侧栏「电脑」里右键一台电脑（或鼠标移上去点 ⋯），可以打开终端、改名、选谁能用和放进哪些项目、移除。电脑设置里：工作区的 agent 一行一个勾，勾上的才能在这个文件夹里跑命令；项目标签点 × 拿出来，「加进项目…」放进去；移除前会确认，文件夹和聊过的内容都不动。电脑页每行有「设置」按钮，打开终端时顶栏右边有齿轮，进的都是同一个设置。
- **一直开着的会话，过程显示在对的地方**：你在 agent 干活时又发了新消息，它接着做时的步骤和进展显示在你的新消息下面，而不是都堆在上一条任务里、新消息下面只有一个转圈。

## v0.19.19

- **Adding a computer takes one command, like adding an agent**: Click "Add a computer" (the + under "Computers" in the sidebar, the Computers page, inside a project, and the menu bar all work the same), and the dialog gives you one command (for Mac / Linux / WSL and for Windows PowerShell). On that computer, open a terminal, go to the folder you want agents to use, and run it: it installs myco and connects to the workspace, the folder becomes a "computer", and the App ticks it off automatically. If you added it from a project, it goes straight into the project, and the project's agents can use it right away. No paths to type, no extra access to grant. You can add several folders from the same computer. Only the workspace owner and admins can generate the command. **Requires the hub on the server to be upgraded to v0.19.19 (`sudo myco upgrade`)**; commands generated by older hubs use the home directory.
- **Each computer decides whether terminals can be opened on it**: When you pick a folder on a connected computer, computers that haven't allowed it are marked "Not allowed yet", along with the command to run on that computer, instead of an English error turning up after you've filled everything in. Change it on that computer: `myco machine shell-access --allow` / `--deny`, or the switch in that computer's App; the App never changes it remotely. Click a computer on the Computers page to see whether it is online, whether terminals can be opened and how to change that, and to remove it.
- **Faster, more considerate voice chat**: When it needs to pass something on, stop, or check, it answers first ("OK, I'll tell it") and then does it, instead of going silent for several seconds; it starts reading the chat as soon as you start speaking, so it is usually done by the time you finish. When a task it passed on takes a while, about every two minutes, at a quiet moment, it tells you it's still working, for how long, and how far it has got. Call the same agent again after hanging up and it remembers the last few things said in the previous call (within 24 hours, stored only on this computer).
- **Links open in your default browser**: In the desktop App, clicking a link in a chat (or in a file opened beside it) opens it in the system default browser. A link after Chinese punctuation no longer runs into the next one.
- **Nicer images and documents**: Images you haven't sent yet can be opened full size; very long screenshots open whole, centered; Markdown documents are laid out like GitHub (stronger headings, quotes, tables, code blocks).
- Tasks that may be stuck no longer hide their steps, and "how long since anything happened" counts from their last activity; tasks that have stopped or were put aside no longer show as stuck.

**中文**

- **加电脑和加 agent 一样，一条命令**：点「加一台电脑」（侧栏「电脑」的 +、电脑页、项目里、菜单栏都一样），弹窗直接给出一条命令（Mac / Linux / WSL 和 Windows PowerShell 两种）。在那台电脑上打开终端，进到要给 agent 用的文件夹，运行它：装好 myco、连上工作区，那个文件夹就成了一台「电脑」，App 里自动打勾；从项目里加的直接放进项目，项目里的 agent 马上能用。不用手写路径，也不用另外开权限。同一台电脑可以加多个文件夹。只有工作区主人和管理员能生成这条命令。**需要服务器上的 Hub 升级到 v0.19.19（`sudo myco upgrade`）**，旧 Hub 生成的命令会用 home 目录。
- **电脑自己决定能不能开终端**：在已经连上的电脑上选文件夹时，没允许的电脑会标「还没允许」，并给出要在那台电脑上运行的命令，不再填完才冒出一段英文报错。在那台电脑上改：`myco machine shell-access --allow` / `--deny`，或者那台电脑的 App 里点开关；App 从不远程改。电脑页点一台电脑能看它在不在线、能不能开终端、怎么改，以及移除它。
- **语音聊天更快、更贴心**：要转话、停下或查看时先回一句（「好，我跟它说」）再去做，不再干等好几秒；你一开口就去读对话，说完时通常已经读好。转出去的任务做得久，每隔两分钟左右在安静的时候说一句还在做、做了多久、做到哪了。挂断后再打给同一个 agent，记得上一通最后说的几句（24 小时内，只存在本机）。
- **链接用默认浏览器打开**：对话里（和旁边打开的文件里）的链接，在桌面 App 里点了直接用系统默认浏览器打开。中文标点后面的链接不再和下一个连成一个。
- **图片和文档更好看**：还没发出去的图片点开能看大图；很长的截图打开时完整显示在中间；Markdown 文档按 GitHub 的样子排版（标题更醒目、引用、表格、代码块）。
- 可能卡住的任务不再把它的过程藏起来，「多久没动静」从它最后一次有动静算起；已经停下或先放着的不再显示成卡住。

## v0.19.18

- **Switching workspaces takes you back to where you were**: When you leave a workspace, it remembers where you were (which space, which page), and each space remembers what you had open (a chat, the member beside the coordinator, activity). Switching back, or coming back from Settings, puts you where you left off; clicking a space again in the sidebar still takes you to its home page.
- **⌘1–⌘9 switch workspaces** (Ctrl on Windows): in the switcher's order, and the switcher shows each workspace's shortcut.
- **Recent chats in ⌘K**: With the search box empty, it lists the chats you opened recently, newest first.
- **Dropped hub connections are noticed quickly**: After the computer sleeps or changes networks, the runner and sessions waiting for messages used to take over a minute to notice the connection was gone; now, after 15 seconds of silence it probes the connection, and with no answer within 15 seconds it opens a new one and resends. HTTPS hubs only; myco on that computer must be upgraded to v0.19.18.

**中文**

- **切换工作区回到原来的位置**：离开一个工作区时记住你在哪（哪个空间、哪一页），每个空间也记住打开着的东西（对话、协调者旁边的成员、动态）。切回来或者从设置回来，都和离开时一样；在侧栏里再点一次空间，还是回到它的首页。
- **⌘1–⌘9 切换工作区**（Windows 上是 Ctrl）：按切换器里的顺序，切换器里标着每个工作区的快捷键。
- **⌘K 里有最近的对话**：搜索框空着时，列出最近打开的对话，最近的在最前面。
- **断掉的 Hub 连接很快被发现**：电脑睡眠、换网络之后，runner 和等消息的会话以前要等一分多钟才发现连接已经断了；现在连接 15 秒没动静就探一下，15 秒没回应就换新连接重发。只对 HTTPS 的 Hub 有效，需要那台电脑的 myco 升级到 v0.19.18。

## v0.19.17

- **Choose a voice for voice chat**: "Voice" under Settings › Voice › Speech is now a drop-down with the official voice list of Doubao speech synthesis 2.0 built in (250 voices, grouped by language: Chinese, English, Japanese, Korean, Spanish, French and more, marked male / female). Pick one and click "Try it" to hear a sentence in that language, synthesized with the key you entered or saved, without saving first. For a cloned voice, choose the last item, "Other", and enter its ID starting with S_. Voices you saved before are not affected.
- **Clicking a name in the Mac notch island works now**: Before, nothing happened. Now, when you click an agent's name: if its chat is already on screen, it gives a brief cue in place; otherwise it opens the space you are viewing (if the agent is in it); failing that, the project it is working in.
- **Dictation no longer writes a sentence twice when it stops**: When dictation stops, Apple's recognizer sends the whole sentence again (often with the first few words changed), and this used to be appended as a new sentence. On Windows, dictation no longer moves the cursor while it writes, which caused duplicates.
- **Agents that are only waiting for more messages no longer show as working**: When a session in a chat timed out waiting for messages and started waiting again, that round used to count as a step of a task, and the walking mushroom reappeared under tasks that had already been answered. Now such rounds don't count as work (new myco versions don't upload them, and the App ignores those uploaded by older versions).

**中文**

- **语音聊天可以换音色**：设置 › 语音 › 语音合成里的「音色」改成下拉选择，内置豆包语音合成 2.0 的官方音色列表（250 个，按语种分组：中文、英语、日语、韩语、西语、法语等，标了男 / 女）。选好点「试听」，用你填的或已保存的 Key 合成一句该语种的话放给你听，不用先保存。复刻的声音选最后一项「其他」，填 S_ 开头的 ID。原来存的音色不受影响。
- **Mac 灵动岛里点名字能打开了**：以前点了没反应。现在点 agent 的名字：它的对话已经在屏幕上就原地提示一下；否则打开你正在看的空间（agent 在里面时），再否则打开它在干活的那个项目。
- **语音输入停下时不再把一句话写两遍**：停止时苹果识别会把整句再发一次（常常改了开头几个字），以前被当成新的一句接在后面。Windows 上语音输入写字时也不再挪光标导致重复。
- **agent 只是在继续收消息时，不再显示成在干活**：对话里收消息的会话等待超时后重新开始等，以前这一轮会被当成任务的步骤，已经回复过的任务下面又冒出走动的蘑菇。现在这种轮次不算干活（新的 myco 不上传它，App 也会忽略旧版本上传的）。

## v0.19.16

- **Fix: cloud sessions that joined a non-default workspace got no messages**: In claude.ai cloud environments with `MYCO_HUB` set (pointing at the server's default workspace, for joining), it used to override the address saved with the session's identity. So a session that had joined a `/w/<name>` workspace sent that workspace's token to the default workspace with every command, kept getting `invalid agent token`, and resuming didn't help. Now a saved identity always uses its own address; `MYCO_HUB` only applies when there is no identity yet, or when it is set together with `MYCO_TOKEN`. **Requires myco on that computer / in that session to be upgraded to v0.19.16.**
- **Token errors no longer send you to the wrong workspace**: The `invalid agent token` hint explains that a token only works in the workspace that issued it, and includes `--hub`; when `myco agent register --rotate-token` can't find the name at the current address, it now says plainly that it "created a new one instead of rotating", and suggests adding `--hub <server>/w/<workspace>`.
- **Handoffs between agents in a project are a card**: Exchanges between two agents no longer mix into your chat as bubbles. Instead there is a card: who handed what to whom, how far it has got, and the first line of the result that came back; click it to see the full exchange beside the chat. The coordinator's home page also shows the tasks and notes members sent it.

**中文**

- **修复：加入非默认工作区的云端会话收不到消息**：claude.ai 云端环境里设了 `MYCO_HUB`（指向服务器的默认工作区，入网用），以前它会覆盖会话保存的身份地址。所以加入 `/w/<名字>` 工作区的会话，每条命令都把那个工作区的 token 发到默认工作区，一直报 `invalid agent token`，resume 也没用。现在已保存的身份始终用自己的地址；`MYCO_HUB` 只在还没有身份时、或者和 `MYCO_TOKEN` 一起设时生效。**需要那台电脑 / 会话里的 myco 升级到 v0.19.16。**
- **token 报错不再把人引到错的工作区**：`invalid agent token` 的提示说明 token 只在签发它的工作区有效，并带上 `--hub`；`myco agent register --rotate-token` 在当前地址找不到这个名字时，会明确说「新建了一个、没有轮换」，并提示加 `--hub <服务器>/w/<工作区>`。
- **项目里 agent 之间的交接是一张卡片**：两个 agent 之间的往来不再以气泡混进你的对话，而是一张卡片：谁把什么交给了谁、做到哪了、回来的结果第一行；点开在旁边看完整往来。协调者的首页也会显示成员发给它的任务和留言。

## v0.19.15

- **Agents suggest what you might say next, and Tab fills it in**: An agent can attach "what you might want to say next" to a reply (`myco message reply --suggest "..."` or `myco message suggest`). When the latest task in a chat has a suggestion and the input box is empty, the placeholder shows it (one line, cut short if too long); press Tab to fill it into the input box, and edit it before sending if you like. With end-to-end encryption on, suggestions are encrypted too. **Requires the hub to be upgraded to v0.19.15 (`sudo myco upgrade`)**, and myco on the agent's computer too.
- **Choose a speech service**: Speech under Settings › Voice is no longer fixed to Doubao; pick one from the list of services (Doubao for now), with one key saved for each service.
- **Voice settings sync across devices**: Voice chat settings (model, speech service, voice, speed, each service's key) are saved to the hub, so they work on another computer or a phone too; you can turn sync off in Settings. Requires both the hub and the App to be upgraded to v0.19.15.
- App: when an agent's turn ends, the input box goes back to normal sending, instead of staying on "Add to…" with a stop button.

**中文**

- **agent 建议你下一步说什么，Tab 一键填入**：agent 回复时可以附一句「你下一步可能想说的」（`myco message reply --suggest "..."` 或 `myco message suggest`）。对话里最新那条任务带了建议、输入框空着时，占位文字就是这句建议（只占一行，太长截短）；按 Tab 填进输入框，改了再发也行。开了端到端加密时，建议也加密。**需要 Hub 升级到 v0.19.15（`sudo myco upgrade`）**，agent 那台电脑的 myco 也要升级。
- **语音合成服务可选**：设置 › 语音里的语音合成不再写死豆包，从服务列表里选（目前是豆包），每个服务各存一个 Key。
- **语音设置多设备同步**：语音聊天的设置（模型、语音合成服务、音色、语速、各服务的 Key）保存到 Hub，换台电脑或手机也能用；可以在设置里关掉同步。需要 Hub 和 App 都升级到 v0.19.15。
- App：agent 这一轮结束后，输入框回到普通的发送状态，不再停在「补充给…」和停止按钮上。

## v0.19.14

- **Tasks in a chat read more like messages**: When an agent stops, the chat ends with the last thing it said, and status lines such as "Waiting, no reply yet", "Put aside" or "Not finished" no longer hang under the task. For a task that hasn't started or was moved back, a line under your message says so (not looked at yet, queued, will reply later, or why: the node is offline or busy), and it disappears once work starts.
- **Only one moving mushroom per chat**: Only the round that is actually working shows the walking mushroom; queued, paused and waiting rows start with a still grey dot, and rows queued behind the task in progress say so.

**中文**

- **对话里的任务读起来更像消息**：agent 停下时，对话以它最后说的话结尾，任务下面不再挂「在等，还没回复」「先放着了」「还没做完」这类状态行。还没开始或被放到后面的任务，在你的消息下面写一句（还没开始看、排在后面、稍后回复，或者节点离线、正忙的原因），开始做了就消失。
- **一个对话只有一个在动的蘑菇**：只有真正在干活的那一轮显示走动的蘑菇；排队、暂停、在等的行前面是一个静止的小灰点，排在正在做的任务后面的会写明。

## v0.19.13

- **New logo: a pixel mushroom**: myco's logo is now a pixel-art mushroom with pixel letters, matching the pixel mushroom avatars of agents in the App. The App icon is now an ink mushroom on off-white (on both Mac and Windows). The top left of the sidebar shows only the myco wordmark, and the welcome page, the web version and the browser tab icon have changed too.
- **The tray icon is a mushroom**: In the Mac menu bar the icon changes color with light and dark mode; in the Windows tray it is an ink mushroom with a thin off-white outline, easy to see on both light and dark taskbars.
- **Tray menu**: No more emoji, text only; its language follows the App's language setting (Settings › Appearance), changes as soon as you switch, and is remembered on the next launch.
- **A complete Mac menu bar**: myco (About, Check for Updates, Settings ⌘,), File (New Project ⌘N, Connect a Workspace, Connect a Computer, Refresh ⌘R), Edit, View (Search ⌘K, Show / Hide Sidebar ⌘\, Full Screen), Window, Help (Docs, Release Notes, Report an Issue). Its language also follows the App setting.

**中文**

- **新 logo：像素蘑菇**：myco 的标志换成了像素风的蘑菇和像素字母，和 App 里 agent 的像素蘑菇头像是一套。App 图标改成米白底、墨色蘑菇（Mac、Windows 都换了）。侧栏左上只留 myco 字母，欢迎页、网页版和浏览器标签图标也都换了。
- **托盘图标是蘑菇**：Mac 菜单栏里的图标会跟着浅色、深色自动变色；Windows 托盘里是墨色蘑菇加一圈米白细边，浅色、深色任务栏上都看得清。
- **托盘菜单**：去掉了 emoji，只用文字；语言跟 App 的语言设置（设置 › 外观）走，切换后马上变，下次启动也记得。
- **Mac 顶部菜单栏补全**：myco（关于、检查更新、设置 ⌘,）、文件（新建项目 ⌘N、连接工作区、连上一台电脑、刷新 ⌘R）、编辑、显示（搜索 ⌘K、显示 / 隐藏侧栏 ⌘\、全屏）、窗口、帮助（文档、更新记录、反馈问题）。语言也跟 App 设置走。

## v0.19.12

- **The voice chat button appears only once voice is set up**: Only after you configure both the model (URL, model name, key) and the Doubao speech key in Settings › Voice does the round button on the right of an empty input box become voice chat; until then it doesn't show, and the round button is send, as usual. After you save in Settings, the button in chats changes right away.
- The description in Settings › Voice now matches how it works: start voice in a chat, no longer with the headphones button in the top bar.

**中文**

- **语音聊天按钮配好了才出现**：只有在 设置 › 语音 里把模型（地址、模型名、密钥）和豆包语音的 Key 都配好后，对话输入框空着时右边的圆按钮才是语音聊天；没配好就不显示，圆按钮和平时一样是发送。在设置里保存后，对话里的按钮马上跟着变。
- 设置 › 语音 的说明改成现在的用法：在对话里开语音，不再是顶栏的耳机按钮。

## v0.19.11

- **Real-time voice chat with agents** (Mac App): In an agent's chat, with the input box empty, click the round button on the right (waveform icon) to talk to the agent as on a phone call. It works like voice in ChatGPT and Claude, one call per chat.
  - Things you ask it to do are passed straight to the agent, without asking you to confirm: if the agent is busy they are added to its current work, if it is free they are sent as a new task, the same as pressing Enter.
  - Ask about progress and the voice answers by itself from the chat. When the agent replies or has a question for you, it picks a quiet moment to tell you in a sentence or two; for long ones it says "the details are in the chat".
  - When it is speaking and you start talking, it stops to listen. The system's echo cancellation and noise suppression keep out its own voice and background noise.
  - Messages passed to the agent are marked "By voice" in the chat; hover to see your original words. The voice bar sits above the input box, with mute and hang up.
  - First, in Settings › Voice, enter a fast OpenAI-compatible model (if left empty, the one under "Translation" is used) and a Doubao speech synthesis API key. Keys are stored in the system keychain; recordings are not kept.
- **The round button in the input box follows the state**: It is voice chat when the box is empty, send when there is text or an attachment, and stop while the agent is working. Hold ⌥Space (Mac) or Ctrl+M to speak, and let go to stop. The hint under the input box shows only while the agent is working.
- **Agents hand each other only results**: When an agent waits for someone else's result with `myco message send --wait` or `myco message wait`, by default it gets only the reply, the number of progress updates and the last one, instead of having the other agent's working notes stuffed into its context; add `--progress all` to get everything. In the App you still see every step.
- App: opening a chat lands at the bottom, or where you last were, and no longer stops in the middle (common on Windows).

**中文**

- **和 agent 实时语音聊天**（Mac 版 App）：在某个 agent 的对话里，输入框空着时点右边的圆按钮（波形图标），就能像打电话一样和这个 agent 说话。用法和 ChatGPT、Claude 的语音一样，一个对话一个。
  - 你要它做的事直接转给它，不用确认：它手上有活就补充进去，空着就发成新任务，和按 Enter 一样。
  - 问进展，语音自己从对话里答。agent 回复了或在问你时，它挑安静的时候用一两句话告诉你，长的就说「详细的放在对话里了」。
  - 它说话时你一开口它就停下来听。系统的回声消除和降噪会挡掉它自己的声音和环境音。
  - 转给 agent 的消息在对话里带「经语音」标记，鼠标放上去能看原话。语音条停在输入框上方，可以静音、挂断。
  - 要先在 设置 › 语音 里填：一个 OpenAI 兼容的快模型（不填就用「翻译」里的那个），和豆包语音合成的 API Key。密钥存在系统钥匙串里，录音不保存。
- **输入框的圆按钮跟着状态变**：框里空着时是语音聊天，有字或附件时是发送，agent 在干活时是停止。按住 ⌥空格（Mac）或 Ctrl+M 说话，松开就停。输入框下面的提示只在 agent 干活时显示。
- **agent 之间只交接结果**：agent 用 `myco message send --wait` 或 `myco message wait` 等别人的结果时，默认只拿到回复、进展条数和最后一条进展，不再把对方的过程笔记都塞进自己的上下文；要全部就加 `--progress all`。你在 App 里照样能看到每一步。
- App：打开对话时停在最底下，或者上次看到的地方，不会再停在中间（Windows 版常见）。

## v0.19.10

- **Fix: projects in a new workspace couldn't be joined**: When the agent's environment had `MYCO_HUB` and `MYCO_ENROLL_TOKEN` preset for the default workspace (as in claude.ai cloud environments), joining with a new workspace's join link added the agent to the default workspace instead, with "project not found". Now `myco join --hub <workspace URL>` always uses this computer's identity in that workspace, and the last command the join script has the agent run includes `--hub` too. **Requires the hub to be upgraded to v0.19.10 (`sudo myco upgrade`)** for the join script to include it.
- **Create a workspace in the App**: When the current workspace is on a server you own, the workspace switcher has "New workspace on <server>": enter a name and it is created, you join as its owner and switch to it, with no commands to run or links to copy (hub v0.19.9 or later).
- **Add computers already on the same server in one click**: In a new workspace, "Connect another computer" lists the computers already connected to this server (in its other workspaces); click "Add", with no commands to run on that computer (myco on that computer must be v0.19.9 or later).
- App: images in messages keep their original proportions and show smaller; a run's timer counts from the start of this round, not from the first message.

**中文**

- **修复：新工作区的项目加不进去**：agent 所在的环境里预先设了默认工作区的 `MYCO_HUB`、`MYCO_ENROLL_TOKEN`（比如 claude.ai 云端环境）时，按新工作区的入网链接加入，会被加进默认工作区，提示「项目不存在」。现在 `myco join --hub <工作区地址>` 总是用这台电脑在那个工作区的身份，入网脚本最后让 agent 跑的那句也带上了 `--hub`。**需要 Hub 升级到 v0.19.10（`sudo myco upgrade`）**，入网脚本才会带上它。
- **App 里新建工作区**：当前工作区在一台你是主人的服务器上时，工作区切换器里有「在 <服务器> 上新建工作区」，输入名字就建好、以主人身份加入并切过去，不用再跑命令、复制链接（Hub 要 v0.19.9 以上）。
- **同一台服务器上的电脑一键加进来**：新工作区里「连上另一台电脑」会列出已经连着这台服务器（在它的其他工作区里）的电脑，点「加进来」就行，不用在那台电脑上运行命令（那台电脑的 myco 要 v0.19.9 以上）。
- App：消息里的图片保持原来的比例、显示小一些；一次运行的计时从这一轮开始算，不再从第一条消息算起。

## v0.19.9

- **Several workspaces in one hub**: `sudo myco workspace create --name team-b` creates a fully isolated workspace in the same hub (its own members, agents, projects, messages and files) at `https://<Hub>/w/team-b`, with no new port, domain, certificate or service. The command prints a one-time App link for the new workspace's owner; paste it under "Connect to an existing hub" in the App and it's ready. `myco workspace list` lists workspaces, and `myco workspace remove` removes one (the data isn't deleted; it moves to `workspaces/.removed/`). The original hub becomes the default workspace, and nothing changes for it; older CLIs and runners can join new workspaces too. **Requires the hub to be upgraded to v0.19.9 (`sudo myco upgrade`)**.
- **Computers can join another workspace on the same server**: The hub can let an already connected computer join another workspace on the same server through its existing connection, without running commands on that computer; only links from the same server are accepted (the App button comes in the next version).
- **Tasks put aside don't take additions**: While an agent is holding a task (waiting on someone else), a message you send is a new task, not an addition to it; but when a task is waiting for your answer because of `message ask`, your answer still counts as an addition.
- App: delete messages from your own view (they stay on the hub, the deletion syncs across all your devices, and you can undo it for a few seconds); queued items are larger and keep their attachments after editing; the files panel can be dragged wider; dictation no longer stops by itself on pauses; steps are drawn once, no longer repeated.

**中文**

- **一个 Hub 里开多个工作区**：`sudo myco workspace create --name team-b` 在同一个 Hub 里新建一个完全隔离的工作区（自己的成员、agent、项目、消息、文件），地址是 `https://<Hub>/w/team-b`，不用新端口、新域名、新证书、新服务。命令会打印新工作区主人的一次性 App 链接，在 App 里「连接已有的 Hub」粘进去就能用；`myco workspace list` 列出，`myco workspace remove` 移除（数据不删，挪到 `workspaces/.removed/`）。原来的 Hub 就是默认工作区，什么都不变；老版本的 CLI 和 runner 也能加入新工作区。**需要 Hub 升级到 v0.19.9（`sudo myco upgrade`）**。
- **电脑加入同一台服务器上的另一个工作区**：Hub 可以让已经连着的电脑通过它现有的连接加入同一台服务器上的另一个工作区，不用在这台电脑上跑命令；只接受同一台服务器的链接（App 里的按钮下一版加）。
- **先放着的任务不接补充**：agent 把任务 hold 住（在等别人）时，你这时发的消息是新任务，不会变成给它的补充；因为 `message ask` 在等你回答的任务，你的回答仍然算补充。
- App：删掉自己视图里的消息（Hub 上不删，你所有设备同步，几秒内可以撤销）；排队的条目更大、编辑后附件保留；文件面板能拖宽；语音输入停顿时不再自动停；步骤只画一次，不再重复。

## v0.19.8

- **Text appears as it's written**: Claude Code woken by the runner now sends its text and thinking up as it writes (`--include-partial-messages`), so text in the App grows bit by bit instead of appearing all at once when a block is done; within 1.5 seconds of the ack it shows "Picked up, starting Claude Code / Codex"; plan items fade in (no animation when the system's "Reduce motion" is on).
- **Change an agent's access in the App**: The agent card's "Run settings" has a new "Access" setting: follow the computer's setting / read-only / workspace / full. Only the agent's owner can change it (hub admins and the agent itself cannot); it takes effect on the next wake, with no runner restart. The highest choice is that computer's `spawn.max_access` in `machine.yaml` (workspace by default); options above it are greyed out and tell you where to allow them. **Requires both that computer and the hub to be upgraded to v0.19.8**.
- **Smoother additions**: Enter adds to the latest unfinished task in this chat; additions show within that run's steps (what it did before above, what it did after and its current state below); in a chat, only the task that is actually working shows the moving loading indicator, and turns that have ended or are waiting for you no longer show it.
- **⌘K searches everything**: members, projects, computers (across all workspaces, by name and description), and messages in your own chats; opening a message jumps to that chat and highlights the passage. You can also search within a chat (⌘F): Enter goes up, ⇧Enter goes down, Esc closes; and one click takes the chat back to the bottom.
- **Translate what agents write**: Choose a translation service in Settings; nothing is translated until you set its key; steps such as thinking and commands stay as they are.
- Files open in the right panel and can go full screen; thinking folds into the step rows; avatars can only be images; the buttons at the top of a chat are grouped together; chats are ordered by time.

**中文**

- **边写边显示**：runner 唤醒的 Claude Code 现在边写边把正文和思考传上来（`--include-partial-messages`），App 里的文字一点点长出来，不用等一整段写完才闪出来；ack 之后 1.5 秒内就显示「已接手，正在启动 Claude Code / Codex」；计划条目出现时淡入（系统开了「减少动态效果」就不动画）。
- **在 App 里改 agent 的权限**：agent 卡片的「运行」里新增「权限」：跟随电脑设置 / 只读 / 工作区 / 完全。只有 agent 的主人能改（Hub 管理员和 agent 自己都不行），下次唤醒时生效，不用重启 runner；最高能选到那台电脑 `machine.yaml` 里的 `spawn.max_access`（默认工作区），超过的选项是灰的，并告诉你在哪里放开。**需要那台电脑和 Hub 都升级到 v0.19.8**。
- **补充更顺**：Enter 补充给这个对话里最近一条还没结束的任务；补充显示在那次运行的过程里（它之前做的在上，之后做的和当前状态在下）；一个对话里只有正在干活的那条有走动的 loading，回合结束、在等你的不再显示 loading。
- **⌘K 搜索一切**：成员、项目、电脑（所有工作区，按名字和描述），以及你自己对话里的消息；打开一条消息会跳到那个对话并选中那段话。对话里也能搜（⌘F），Enter 往上、⇧Enter 往下，Esc 关闭；聊天可以一键回到底部。
- **翻译 agent 写的内容**：设置里选翻译服务，配置密钥之前不翻译；思考、命令这些过程保持原样。
- 文件从右侧面板打开，可以全屏；思考折进步骤行；头像只能用图片；对话头部的按钮整理成一组；对话按时间排列。

## v0.19.7

- **Steps show up automatically**: After you take a task with `myco message ack` in a Claude Code or Codex session, myco reads that session's own log file in the background (`myco trace follow`) and sends each later step (thinking, commands, results) to the task live, so the sender can follow it in the App just like a task woken by the runner. The agent's own to-do list (Claude Code's TodoWrite, Codex's update_plan) also shows as a plan, with items checked off as its own list is; nothing needs to be posted by hand. It stops when the task is answered or fails, when the session acks another task, or after 2 hours; it doesn't start for runner-woken tasks or end-to-end encrypted tasks; set `MYCO_NO_TRACE_FOLLOW=1` to turn it off. Sessions of other CLIs such as Cursor and Gemini are not supported yet.
- **Hub performance**: Hot queries on the messages table read only unfinished messages, and two indexes were added (built automatically the first time an old database starts; about 2 seconds for 1 million messages); an agent's `last_seen` is written at most every 15 seconds. With 1 million messages, task round trips went from 10 to 225 per second, and overview dropped from 3.15 seconds to 140 ms.
- **Ack reminders depend on the kind of session**: For sessions whose steps are already visible live, the agent is only reminded to post a progress note at key points (cause found, change of direction, stuck, a big step done), not every 2 minutes; for sessions whose files can't be read (other CLIs), it is still reminded to post a plan as a checklist first and then report progress at least every 2 minutes. SKILL.md and the joining instructions were rewritten to match.
- The repo now has a release handbook, so the agent in charge of releases can check and publish step by step.

**中文**

- **过程自动可见**：在 Claude Code 或 Codex 会话里用 `myco message ack` 接下任务后，myco 会在后台读这个会话自己的记录文件（`myco trace follow`），把之后的每一步（思考、命令、结果）实时传给这个任务，发送方在 App 里能像看 runner 唤醒的任务一样看到进展。agent 自己的待办清单（Claude Code 的 TodoWrite、Codex 的 update_plan）也会显示成计划，勾选跟着它自己的清单走，不用再手动发。任务回复或失败、这个会话 ack 了别的任务、或者 2 小时后自动停止；runner 唤醒的任务和端到端加密的任务不会启动它；设 `MYCO_NO_TRACE_FOLLOW=1` 可以关掉。Cursor、Gemini 等其他 CLI 的会话暂不支持。
- **Hub 性能**：消息表的热点查询只读还没结束的消息，并加了两个索引（老库第一次启动时自动建，100 万条消息约 2 秒）；agent 的 `last_seen` 最多每 15 秒写一次。100 万条消息时任务往返从每秒 10 个提升到 225 个，overview 从 3.15 秒降到 140 ms。
- **ack 的提醒按会话类型区分**：步骤已经实时可见的会话，只提醒在关键节点（找到原因、换方向、卡住、完成一大步）报一句进展，不用每 2 分钟重复一遍；读不到会话文件的会话（其他 CLI），仍然提醒先用清单发计划、之后至少每 2 分钟报一次进展。SKILL.md 和接入说明同步改写。
- 仓库里加了发版手册，给负责发版的 agent 按步骤检查和发布。

## v0.19.6

- **The skill upgrades with myco**: SKILL.md is now bundled into myco. `myco skill show` prints the usage guide for the current version (receiving messages, reporting progress, posting plans as checklists, asking with `message ask`, progress with screenshots, handing out work). `myco join`, `myco upgrade` and every runner start install it into the skills folders of Claude Code and Codex (`~/.claude/skills/myco`, `~/.codex/skills/myco`, only folders that already exist); files of your own with the same name are not overwritten. The joining instructions now say "read through `myco skill show`" instead of giving a GitHub link.
- **Fuller hints when a task arrives**: Besides ack, progress and reply, they remind the agent to post a plan as a checklist when there are many steps, and to ask you with `myco message ask` when you need to decide. Any harness can read them.
- **Dictation**: There is a microphone next to the input box, so you can speak instead of typing; speech is recognized in the App's language.
- App: HTML files can open directly as web pages, with the source one click away; what an agent says after a pause is added after the earlier content, not inserted in the middle; a plan with a line of explanation in front of it is still recognized as a plan, and checklists show as checkboxes.

**中文**

- **skill 跟着 myco 一起升级**：SKILL.md 打包进了 myco。`myco skill show` 打印和当前版本对应的用法说明（收消息、报进展、用清单报计划、`message ask` 提问、进展带截图、派活）。`myco join`、`myco upgrade` 和 runner 每次启动时，会把它装进 Claude Code 和 Codex 的 skills 目录（`~/.claude/skills/myco`、`~/.codex/skills/myco`，只装已经存在的目录）；你自己写的同名文件不会被覆盖。接入时的说明改成「读一遍 `myco skill show`」，不再给 GitHub 链接。
- **收到任务时的提示更全**：除了 ack、报进展、回复，还会提醒步骤多的时候用清单报计划，需要你决定的时候用 `myco message ask` 问你。任何 harness 都能读到。
- **语音输入**：输入框旁边有麦克风，可以说话代替打字，按 App 的语言识别。
- App：HTML 文件可以直接作为网页打开，源码点一下就能看；agent 停顿后再说的话会接在前面的内容后面，不会插到中间；计划前面有一行说明时也能认成计划，清单显示成勾选框。

## v0.19.5

- **Model, reasoning effort, usage limits**: In the agent card's "Run settings" you can pick the model and reasoning effort directly (or "Follow this computer"). The settings are stored on the hub, can be changed for an agent on any computer, take effect on the next wake, and override that computer's own config: Claude Code gets `--model` and `--effort`, Codex gets `-m` and `model_reasoning_effort`. After each run the runner reports the account's usage limits (Claude Code's 5-hour and 7-day windows, each of Codex's windows); the card shows one row per window, amber at 90% and red when used up; the Computers page also points out agents on that computer that are running low. For agents that receive messages themselves with `myco inbox wait`, myco can't see their model or limits; they can report their model with `myco agent runtime set --model …`.
- **Rejoin links**: Agents in temporary sandboxes can come back after losing their credentials. `myco join` prints a `curl -fsSL <hub>/r/myco_resume_… | sh` line; save it outside the sandbox, run it in a new sandbox, and it becomes the same agent again (same name, same inbox, same projects); you can also set `MYCO_RESUME` to restore automatically. The agent card shows where it last rejoined from, and lets you create a new link or revoke it.
- **inbox wait is easier to keep running**: `myco inbox wait` without arguments picks a duration by harness (55 minutes for Claude Code, 10 for others) and reminds the agent to start waiting again when it exits; the joining instructions are shorter.
- **Agents can ask you**: `myco message ask --question … --option …` lets an agent ask you to pick an option partway through, holding the task in the meantime; the App shows it as a card you can click, and your answer lets it carry on.
- **Progress can include files**: `myco message progress --attach <file>`; screenshots show under that progress update.
- **Additions and queueing**: Queued messages go out one after another, not all at once; with the input box empty, ↑ / ↓ bring back messages you sent before.
- App: text files in chats (Markdown, logs, code and so on) open for reading directly; sending a file with the same name again counts as a new version, and you can see what changed from the previous one; while you are watching, new replies appear paragraph by paragraph; drag the sidebar's edge to resize it.

**中文**

- **模型、推理强度、额度**：agent 卡片的「运行」一块里可以直接选模型和推理强度（也可以「跟随电脑设置」）。设置存在 Hub 上，任何一台电脑上的 agent 都能改，下次唤醒时生效，优先于那台电脑自己的配置：Claude Code 用 `--model`、`--effort`，Codex 用 `-m`、`model_reasoning_effort`。runner 每次运行后上报账号额度（Claude Code 的 5 小时、7 天窗口，Codex 的各个窗口），卡片上每个窗口一行，用到 90% 变琥珀色，用完变红；电脑页上也会提示这台电脑上额度紧张的 agent。用 `myco inbox wait` 自己收消息的 agent，myco 拿不到它的模型和额度，它可以用 `myco agent runtime set --model …` 报一下自己的模型。
- **重新接入链接**：临时沙箱里的 agent 丢了凭证也能回来。`myco join` 会打印一条 `curl -fsSL <hub>/r/myco_resume_… | sh`，存到沙箱外，新沙箱里跑一下就变回同一个 agent（同名、同收件箱、同项目）；也可以设 `MYCO_RESUME` 自动恢复。agent 卡片上能看到上次在哪里重新接入、生成新链接、作废。
- **inbox wait 更好挂**：`myco inbox wait` 不带参数时按 harness 自动选时长（Claude Code 55 分钟，其他 10 分钟），退出时提醒再挂上；加入时的说明精简了。
- **agent 能问你**：`myco message ask --question … --option …` 让 agent 在做到一半时请你选一个方案，任务先挂起；App 里显示成一张可以点选的卡片，你的回答会让它接着做。
- **进展可以带文件**：`myco message progress --attach <文件>`，截图会显示在那条进展下面。
- **补充和排队**：排队的消息一条接一条发出，不会一起发出去；输入框是空的时候，↑ / ↓ 能翻出你发过的消息。
- App：聊天里的文本文件（Markdown、日志、代码等）可以直接打开阅读，同名文件再发一次会算作新版本，能看和上一版的差异；你看着的时候，新回复会一段一段地出现；侧栏可以拖动边缘调整宽度。

## v0.19.4

- **Several workspaces on one computer**: The workspace switcher has a new "New workspace on this computer": give it a name and a new one starts, and the App joins it and switches to it. Workspaces on this computer carry a "This computer" tag in the switcher; "Workspaces on this computer" in Settings lists them one by one, and each can be opened to the internet, started and stopped on its own.
- **Command line**: `myco hub up --name <name>` starts another hub on the same computer, with its data in `~/.myco/hubs/<name>/`, its own background service, and a port picked automatically from 7778 up; new `myco hub list` and `myco hub down --name`; `myco upgrade` restarts all hubs. Without `--name`, everything works exactly as before.
- **Several hubs on a VPS**: `install-hub.sh --name <name> --domain auto` installs another one on the same server, with the domain `<name>.<IP>.sslip.io`; they share port 443 and each gets its own certificate; data is in `/var/lib/myco-<name>`. Admin commands on the server use `--hub-name <name>` (or `MYCO_HUB_NAME`) to choose the hub.
- **The install script no longer overwrites the whole Caddyfile**: Each hub's site goes in its own file under `/etc/caddy/myco.d/`, and the main Caddyfile only gets one import line, so other sites already on the server are kept; old myco sites are moved over automatically.
- App: the plan checklist is now black, white and grey, and easy to read in dark mode too.

**中文**

- **一台电脑上开多个工作区**：工作区切换器里新增「在这台电脑上新建工作区」，起个名字就会新开一个，自动加入并切过去。这台电脑上的工作区在切换器里带「这台电脑」标签；设置里的「这台电脑上的工作区」逐个列出，每个都能单独开放到公网、启动、停止。
- **命令行**：`myco hub up --name <名字>` 在同一台电脑上再起一个 hub，数据在 `~/.myco/hubs/<名字>/`，有自己的后台服务，端口从 7778 起自动选；新增 `myco hub list` 和 `myco hub down --name`；`myco upgrade` 会重启所有 hub。不带 `--name` 时和以前完全一样。
- **VPS 上装多个 hub**：`install-hub.sh --name <名字> --domain auto` 在同一台服务器上再装一个，域名是 `<名字>.<IP>.sslip.io`，共用 443 端口、各自签证书；数据在 `/var/lib/myco-<名字>`。服务器上的管理命令用 `--hub-name <名字>`（或 `MYCO_HUB_NAME`）选是哪个 hub。
- **安装脚本不再覆盖整个 Caddyfile**：每个 hub 的站点单独放在 `/etc/caddy/myco.d/`，主 Caddyfile 只加一行 import，服务器上已有的其他站点会保留；旧的 myco 站点自动迁移过去。
- App：执行计划的清单改成黑白灰，深色模式下也看得清。

## v0.19.3

- **Change avatars**: Click the avatar on a member card to change it: mushrooms in 10 colors and 12 shapes (with or without a sprout), or an uploaded image (cropped to a square and shrunk in the App); you can switch back to the original at any time. The avatar changes everywhere, and the member's history records it.
- **Workspace avatars**: The dialog for renaming a workspace also lets you choose its avatar, from 8 colors or an image, saved in this App.
- **Hub**: Agent profiles have a new `avatar` field, and the image is at `GET /v1/agents/{name}/avatar`, readable by anyone who can see the agent; changes are recorded in the history. Old databases upgrade in place; older hubs don't recognize the field.
- Fixed a test that occasionally failed on Windows.

**中文**

- **换头像**：在成员卡片上点头像就能换：10 种颜色、12 种形状的蘑菇（带不带小芽都行），也可以传一张图片（App 里裁成方形、压小）；随时能换回原来的。所有地方的头像都会跟着变，成员的历史里也会记下来。
- **工作区头像**：给工作区改名的对话框里也能选它的头像，8 种颜色或一张图片，保存在这个 App 里。
- **Hub**：agent 的资料多了 `avatar` 字段，图片放在 `GET /v1/agents/{name}/avatar`，能看到这个 agent 的人都能取；改动记进历史。老库原地升级；旧 Hub 不认这个字段。
- 修了一个在 Windows 上偶尔失败的测试。

## v0.19.2

- **Additions and queueing in chat**: While the other side is working, Enter adds to your previous message instead of becoming a new task, and there is only one loading indicator; ⌘/Ctrl+Enter queues it as the next message, shown above the input box, where you can edit or withdraw it; it is sent automatically when the current round ends (on older hubs the App does the queueing itself). The old "Correction" switch is gone. Additions can include attachments.
- **Agents using inbox wait don't miss additions**: The results of `myco message ack` / `progress` include new additions; `myco message reply` is blocked while there are unread additions (`--anyway` replies regardless).
- **Runner timeouts count "no activity"**: An agent that keeps working won't be interrupted; it is stopped only after 30 minutes without any output, tool call or thinking, and a round lasts at most 6 hours; failure reasons say which kind of timeout it was.
- **Stuck commands are visible and can be stopped**: When all of a computer's commands may be stuck, its dot in the sidebar turns amber; open the computer's terminal and the top lists possibly stuck commands sent by other agents, which you can stop one by one or all at once. A computer's owner or an admin can now stop tasks sent by other agents, and the sender is told it failed.
- **The runner retries if the network drops while it takes a task**, instead of leaving a task that shows as "running" forever.
- **Thinking shows as light body text**: Some models (such as DeepSeek) only say what they are doing in their thinking, so it is no longer folded into one line; long thinking shows four lines first, and you click to see it all.
- App: plans show as checklists; existing computers can be added to a workspace; click images in messages to see them large, and files are saved to Downloads; drag to reorder the sidebar; answers far from their question quote it; the layout no longer jumps while you type; chats are steadier when switching and refreshing, and drafts are kept; attachments you send display from the local copy.
- Mac packaging: a network drop during notarization no longer loses that notarization.

**中文**

- **聊天里的补充和排队**：对方正在处理时，按 Enter 是补充，拼进你上一条消息里，不会变成新任务，也只有一个 loading；按 ⌘/Ctrl+Enter 是排到下一条，显示在输入框上方，可以编辑、撤回，这一轮结束后自动发出（旧 Hub 上由 App 自己排队）。原来的「修正」开关去掉了。补充可以带附件。
- **inbox wait 的 agent 不会漏掉补充**：`myco message ack` / `progress` 的结果里会带上新的补充；还有没读的补充时 `myco message reply` 会拦住（`--anyway` 可以强行回）。
- **runner 的超时按「没有动静」算**：agent 一直在干活就不会被打断，30 分钟没有任何输出、工具调用或思考才停，一轮最长 6 小时；失败原因会写清楚是哪种。
- **卡住的命令看得见、停得掉**：电脑上的命令全都可能卡住时，侧栏的点变成琥珀色；打开电脑的终端，顶部列出别的 agent 发来、可能卡住的命令，可以逐条或全部停止。电脑的主人或管理员现在可以停掉别的 agent 发来的任务，发送方会收到失败通知。
- **runner 接手任务时网络断了会重试**，不再留下一直显示「在跑」的任务。
- **思考显示成淡色正文**：有的模型（比如 DeepSeek）只在思考里说自己在做什么，现在不再折叠成一行，长的先显示四行、点开看全。
- App：执行计划显示成清单；可以把已有的电脑加进工作区；消息里的图片点开看大图，文件存到「下载」；侧栏可以拖动排序；离问题很远的回答会引用原话；打字时界面不再跳动；切换和刷新时聊天更稳，草稿会保留；自己发的附件直接用本地副本显示。
- Mac 打包：公证时网络断了不会丢掉这次公证。
