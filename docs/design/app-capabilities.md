# 应用用电脑的能力：能力清单

状态：梳理中（2026-11-20）。这一版只做「放大」：把客户端能开放给应用的能力全部列出来，先不考虑安全；分级只是初稿（第 4 节），下一步再收。

## 0. 已经定的

- **只做这台电脑**：App 所在的电脑。调用从网页到 App，再到 App 后端，不经过 Hub。远程电脑以后再说。
- **所有应用都能用**：官方应用、工作区自己的应用、agent 做的网页都一样。
- **能读文件内容**，不先设限。

## 1. 应用怎么拿到这些能力

```
网页（iframe，不同源）──postMessage──▶ App 界面（pages.tsx）──▶ App 后端（Go，internal/console）
                                                 │                       │
                                                 │                       ├─ 普通 Go：文件、命令、网络、系统信息
                                                 │                       └─ 系统接口：cgo / Objective-C（Mac），Win32 / WinRT（Windows）
                                                 └─ App 自己的窗口和弹框（Wails）：选文件、确认框、新窗口、菜单栏
```

- **网页这一侧**：在现有的 `window.myco` 上加一层 `myco.os.*`，调用方式和 `myco.data` 一样，都是 `postMessage` 带上 `method` 和 `args`。App 界面只核对来源，然后原样转给后端。
- **长的调用**（扫描、录屏、跑命令、监听文件）返回一个任务：带 id，可以订阅进度和输出，也可以取消。推的方式和现在的 `data.on` 一样。
- **系统接口**：Mac 版 App 本来就用 cgo 调 Objective-C（听写、灵动岛、语音都是，见 `cmd/myco-app/*_darwin.go`），新能力照这个做。Windows 用 Win32 / WinRT，Linux 能做的做，做不了的返回 `unsupported`。
- **系统授权**：调用发生在 myco App 进程里，所以 macOS 的系统授权（屏幕录制、辅助功能、摄像头、日历…）记在「myco」名下。第一次用到时系统会弹框，App 再给一个「打开系统设置」的按钮。
- **有没有某个能力**：`myco.os.has("screen.capture")`，返回 `true`、`false`，或者 `"needs-permission"`（要系统授权）。

## 2. 能力清单

每一类一张表。「系统授权」指 macOS 的 TCC 一类，Windows 一般不需要。「能做的应用」只是举例。

### 2.1 系统信息

| 能力 | 做什么 | Mac / Windows 怎么拿 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `system.info` | 系统版本、机型、芯片、内存、开机时长、主机名 | `sysctl`、`IOKit` / WMI | 无 | 电脑体检 |
| `system.disks` | 各个磁盘和分区：容量、剩余、格式、外接盘 | `statfs`、`DiskArbitration` / `GetDiskFreeSpaceEx` | 无 | 空间清理 |
| `system.usage` | CPU、内存、磁盘读写、网络流量，实时采样 | `host_statistics`、`IOKit` / PDH 计数器 | 无 | 资源监控面板 |
| `system.battery` | 电量、充放电、健康度、循环次数 | `IOPowerSources` / `GetSystemPowerStatus` | 无 | 电池健康 |
| `system.thermal` | 温度、风扇、是否降频 | `IOKit` SMC（Apple 芯片上有限）/ WMI | 无 | 性能监控 |
| `system.displays` | 显示器：分辨率、缩放、刷新率、排列 | `CGDisplay` / `EnumDisplayMonitors` | 无 | 截图、窗口管理 |
| `system.network` | 网卡、IP、Wi-Fi 名、信号、代理、DNS、VPN | `getifaddrs`、`CoreWLAN` / `GetAdaptersAddresses` | Wi-Fi 名在新版 macOS 上要定位授权 | 网络诊断 |
| `system.locale` | 语言、时区、深浅色、强调色 | `NSLocale`、`NSAppearance` | 无 | 所有应用跟随系统 |
| `system.apps` | 已装的应用：名字、版本、大小、图标、最后打开时间 | 扫 `/Applications`，读 Spotlight 元数据 / 注册表 Uninstall | 无 | 应用卸载、清理 |
| `system.processes` | 进程列表：CPU、内存、打开的文件和端口 | `libproc` / `ToolHelp32` | 无 | 活动监视器、端口占用 |
| `system.startup` | 登录项、后台服务（launchd）、开机启动 | `SMAppService`、读 `~/Library/LaunchAgents` / 注册表 Run 键 | 无 | 启动项管理 |
| `system.devices` | USB、蓝牙、外接设备、打印机 | `IOKit`、`IOBluetooth` / SetupAPI | 蓝牙要授权 | 设备管理 |

### 2.2 文件

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `fs.list` / `fs.stat` | 列文件夹，文件的大小、时间、类型、权限、扩展属性 | Go `os` | 部分文件夹要「完全磁盘访问」 | 文件管理器 |
| `fs.read` | 读文件：文本、二进制，按范围读，大文件按块读 | Go `os` | 同上 | 日志查看、代码阅读、导入数据 |
| `fs.write` / `fs.append` | 写文件、追加、新建文件夹 | Go `os` | 同上 | 编辑器、导出到指定位置 |
| `fs.move` / `fs.copy` / `fs.rename` | 移动、复制、改名，批量 | Go `os` | 同上 | 批量改名、整理下载 |
| `fs.trash` | 移进废纸篓（能恢复） | `NSFileManager trashItemAtURL` / `IFileOperation` | 同上 | 清理 |
| `fs.delete` | 永久删除 | Go `os.RemoveAll` | 同上 | 清理缓存 |
| `fs.scan` | 按大小汇总文件夹，找大文件、旧文件、重复文件（按内容哈希） | Go，并发遍历，在后端算好 | 同上 | 空间清理、找重复照片 |
| `fs.search` | 按名字和内容搜文件，用系统索引 | Spotlight `mdfind` / Windows Search | 无 | 全局搜索、启动器 |
| `fs.watch` | 文件夹变了就通知 | `FSEvents` / `ReadDirectoryChangesW` | 同上 | 自动整理下载、热重载 |
| `fs.archive` | 压缩、解压（zip、tar.gz） | Go `archive/*` | 无 | 打包、备份 |
| `fs.hash` | 文件哈希 | Go `crypto` | 无 | 校验、去重 |
| `fs.thumbnail` / `fs.icon` | 文件缩略图（图片、PDF、视频第一帧），文件图标 | `QLThumbnailGenerator` / `IShellItemImageFactory` | 无 | 文件浏览、相册 |
| `fs.reveal` / `fs.open` / `fs.openWith` | 在访达里显示、用默认程序打开、用指定程序打开 | `NSWorkspace` / `ShellExecute` | 无 | 一切跟文件有关的 |
| `fs.pick` / `fs.pickFolder` / `fs.saveAs` | 系统的打开、选文件夹、另存为对话框 | Wails `Dialog` | 无（人选的文件本来就能读） | 导入导出 |
| `fs.drop` | 从访达拖文件到网页，网页拿到真实路径 | Wails 的拖放 | 无 | 拖进来就处理 |
| `fs.tags` / `fs.xattr` | 访达标签、扩展属性、下载来源（隔离属性） | `NSURL` resourceValues、`xattr` | 无 | 文件整理 |
| `fs.mount` / `fs.eject` | 挂载、推出磁盘镜像和外接盘 | `hdiutil`、`DiskArbitration` | 无 | 外接盘管理 |

### 2.3 命令和进程

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `exec.run` | 跑一条命令，回退出码和输出 | Go `os/exec`，argv 形式 | 无 | 包装任何命令行工具 |
| `exec.shell` | 跑一段 shell 脚本（zsh、bash、PowerShell） | `sh -c` / `pwsh -c` | 无 | 运维小工具 |
| `exec.spawn` | 起长期运行的进程：标准输入输出实时流，可以写入、可以停 | Go `os/exec` 加管道 | 无 | 本地服务管理、日志跟踪 |
| `exec.pty` | 一个真终端（带 PTY），网页里用 xterm.js 画 | `creack/pty` / ConPTY（App 的终端节点已经有） | 无 | 网页里的终端 |
| `exec.sudo` | 用管理员权限跑一条命令，系统弹框要密码 | `AuthorizationExecuteWithPrivileges` 或 `osascript … with administrator privileges` / UAC | 系统密码框 | 清理系统缓存、改 hosts |
| `exec.script` | 跑 AppleScript、JXA、快捷指令 / PowerShell | `osascript`、`shortcuts run` | 控制别的应用要「自动化」授权 | 控制访达、邮件、音乐、Safari |
| `process.kill` / `process.signal` | 结束、暂停进程 | `kill` / `TerminateProcess` | 无 | 活动监视器 |
| `app.launch` / `app.quit` / `app.activate` | 打开、退出、切到前台某个应用 | `NSWorkspace`、`NSRunningApplication` | 无 | 工作区一键布置 |
| `app.uninstall` | 删掉应用和它在 `~/Library` 里的残留 | 组合 `fs.trash` | 完全磁盘访问 | 应用卸载器 |
| `pkg.*` | 用 brew、winget、npm、pip 装、升级、列出 | 组合 `exec.run` | 无 | 开发环境管理 |

### 2.4 屏幕和 computer use

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `screen.capture` | 截全屏、某个显示器、某个窗口、某个区域，回 PNG 或 WebP | `ScreenCaptureKit` / `Windows.Graphics.Capture` | 屏幕录制 | 截图标注、给 agent 看屏幕 |
| `screen.pick` | 让人用系统的选框选区域或窗口再截 | `screencapture -i` / Snipping 接口 | 屏幕录制 | 截图工具 |
| `screen.record` | 录屏（可带系统声音和麦克风），回视频文件 | `ScreenCaptureKit` + `AVAssetWriter` / Graphics Capture + Media Foundation | 屏幕录制，麦克风 | 录屏、教程、bug 复现 |
| `screen.stream` | 屏幕画面实时给到网页（低帧率），能拿去识别 | `ScreenCaptureKit` 出帧，WebP 推给网页 | 屏幕录制 | 远程协助、屏幕上的 AI 助手 |
| `screen.ocr` | 识别截图或图片里的文字（带位置） | `Vision` 的 `VNRecognizeTextRequest` / `Windows.Media.Ocr` | 无 | 截图取字、翻译 |
| `screen.colorAt` | 取屏幕上某点的颜色 | `NSColorSampler` | 无 | 取色器 |
| `windows.list` | 所有窗口：应用、标题、位置、大小、在哪个桌面 | `CGWindowListCopyWindowInfo` / `EnumWindows` | 标题要屏幕录制 | 窗口管理、切换器 |
| `windows.move` / `windows.resize` / `windows.focus` | 移动、缩放、聚焦别的应用的窗口 | `AXUIElement` / `SetWindowPos` | 辅助功能 | 分屏、布局管理 |
| `ui.tree` | 读前台应用的界面结构：按钮、输入框、文字、位置 | `AXUIElement` 遍历 / UI Automation | 辅助功能 | 给 agent 看界面、自动化测试 |
| `ui.act` | 对界面元素做动作：点按钮、填输入框、选菜单 | `AXUIElementPerformAction` / UIA patterns | 辅助功能 | 自动填表、操作别的应用 |
| `input.mouse` | 移动鼠标、点击、拖动、滚动 | `CGEvent` / `SendInput` | 辅助功能 | computer use |
| `input.keyboard` | 打字、按快捷键 | `CGEvent` / `SendInput` | 辅助功能 | computer use、文本扩展 |
| `input.watch` | 监听全局键鼠（比如统计、快捷键） | `CGEventTap` / 低级钩子 | 输入监控 | 键盘统计、自定义快捷键 |
| `computer.use` | 上面几项组合起来，给 agent 一个「看屏幕 → 点 / 打字」的回合接口：截图（缩放过）、按坐标点、打字、滚动、等待 | 组合 `screen.capture`、`input.*`、`ui.tree` | 屏幕录制，辅助功能 | 让 agent 操作这台电脑 |

### 2.5 剪贴板

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `clipboard.read` | 读剪贴板：文字、图片、文件、富文本 | `NSPasteboard` / Win32 剪贴板 | macOS 新版会提示「粘贴自…」 | 剪贴板工具、翻译 |
| `clipboard.write` | 写剪贴板（网页现在只能写文字） | 同上 | 无 | 复制图片、复制文件 |
| `clipboard.watch` | 剪贴板变了就通知 | 轮询 `changeCount` / `AddClipboardFormatListener` | 同上 | 剪贴板历史 |

### 2.6 摄像头、麦克风、声音

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `media.camera` | 摄像头画面、拍照 | 网页 `getUserMedia`（要放开 iframe 的 `allow`）或 `AVFoundation` | 摄像头 | 拍照存档、扫码 |
| `media.microphone` | 录音、实时音频 | 同上（App 的语音已经有一套） | 麦克风 | 录音笔记、会议记录 |
| `media.systemAudio` | 录系统声音 | `ScreenCaptureKit` 音频 / WASAPI loopback | 屏幕录制 | 录会议、录播客 |
| `speech.transcribe` | 语音转文字（本机） | `SFSpeechRecognizer` / Windows Speech（App 的听写已经有） | 语音识别 | 听写、字幕 |
| `speech.say` | 文字转语音 | `AVSpeechSynthesizer` / SAPI，或现有的豆包语音 | 无 | 朗读 |
| `audio.volume` / `audio.devices` | 音量、静音、输入输出设备切换 | `CoreAudio` / `IAudioEndpointVolume` | 无 | 会议一键静音 |
| `audio.play` | 放声音（网页本来就能放） | 网页 | 无 | 提示音 |

### 2.7 通知、菜单栏、快捷键、窗口

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `notify` | 系统通知，带按钮，点了回到应用 | `UNUserNotificationCenter` / Toast | 通知 | 提醒、任务完成 |
| `tray.item` | 应用在菜单栏 / 托盘放一个图标和菜单，显示数字 | Wails 托盘（App 已经有自己的） | 无 | 计时器、监控指标 |
| `dock.badge` | Dock 图标上的角标、弹跳 | `NSDockTile` | 无 | 未读数 |
| `hotkey.global` | 注册全局快捷键，按了唤起应用或做一件事 | `RegisterEventHotKey` / `RegisterHotKey` | 无 | 启动器、快捷截图 |
| `window.open` | 应用开自己的独立窗口（不在 App 主窗口里），可以置顶、无边框、半透明、固定大小 | Wails 多窗口 | 无 | 悬浮计时器、迷你播放器、画中画 |
| `window.panel` | 菜单栏下拉的小面板（像快捷面板） | 现有的 quick panel 那一套 | 无 | 速记、翻译 |
| `app.openURL` | 用默认浏览器打开链接（现在已经有） | `NSWorkspace` | 无 | 所有 |
| `app.autostart` | 开机后在后台跑这个应用 | 见 2.11 | 无 | 监控、定时任务 |
| `share` | 系统分享菜单（AirDrop、邮件、信息） | `NSSharingServicePicker` | 无 | 分享导出的文件 |

### 2.8 联网

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `net.fetch` | 从 App 后端发 HTTP 请求（网页的 CSP 不让直接连），可以带自己的 header 和 cookie | Go `net/http`，结果流回网页 | 无 | 调第三方 API、抓网页、RSS |
| `net.socket` | TCP / UDP / WebSocket 连接 | Go `net` | 无 | 调试工具、IoT |
| `net.serve` | 在本机开一个端口（比如收 webhook，给手机传文件） | Go `net/http` | 本机防火墙可能问 | 局域网传文件 |
| `net.discover` | 局域网发现：Bonjour / mDNS、ping、扫端口 | `dns-sd`、Go | 本地网络（macOS 15 起） | 局域网设备列表 |
| `net.diagnose` | ping、traceroute、DNS 查询、测速 | Go 或系统命令 | 无 | 网络诊断 |
| `browser.automate` | 开一个受控的浏览器（Chromium / CDP）去点网页、抓内容 | Playwright / chromedp | 无 | 网页自动化、比价 |

### 2.9 个人数据（系统自带的应用）

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `calendar.*` | 读写日历事件 | `EventKit` / Outlook、Windows 日历 | 日历 | 日程助手、会议准备 |
| `reminders.*` | 读写提醒事项 | `EventKit` | 提醒事项 | 待办同步 |
| `contacts.*` | 读写通讯录 | `Contacts` 框架 | 通讯录 | CRM、名片 |
| `photos.*` | 读照片和视频（含位置、时间），写入相册 | `PhotoKit` | 照片 | 相册整理、找重复照片 |
| `mail.*` | 读邮件、起草、发送 | AppleScript 驱动「邮件」，或读 `~/Library/Mail`（要完全磁盘访问） | 自动化 / 完全磁盘访问 | 邮件摘要 |
| `messages.*` | 读「信息」记录，发信息 | 读 `chat.db`，用 AppleScript 发 | 完全磁盘访问、自动化 | 消息摘要 |
| `notes.*` | 读写「备忘录」 | AppleScript | 自动化 | 笔记导入 |
| `browser.history` | Safari、Chrome 的历史、书签、打开的标签页 | 读它们的 SQLite（拷一份再读），AppleScript 读标签页 | 完全磁盘访问 / 自动化 | 稍后读、研究助手 |
| `music.*` | 控制「音乐」或 Spotify：播放、暂停、正在放什么 | AppleScript / MediaRemote | 自动化 | 正在播放挂件 |
| `location` | 当前位置 | `CoreLocation` / `Windows.Devices.Geolocation` | 定位 | 天气、打卡 |
| `keychain.app` | 应用把自己的密钥存进钥匙串（每个应用分开存） | `Security` 框架（App 已经有 `keychain.go`） / Credential Manager | 无 | 存第三方 API 的 key |
| `auth.biometric` | 用 Touch ID / Windows Hello 确认是本人 | `LocalAuthentication` / `UserConsentVerifier` | 无 | 敏感操作前确认 |

### 2.10 系统控制

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `power.*` | 睡眠、锁屏、关机、重启、防止睡眠 | `IOPMAssertion`、`pmset` / `SetThreadExecutionState` | 关机要自动化授权 | 下载完自动关机、演示时防睡眠 |
| `display.*` | 亮度、夜览、深浅色切换 | `CoreDisplay`、AppleScript | 自动化 | 专注模式 |
| `focus.*` | 勿扰 / 专注模式开关 | 快捷指令 | 无 | 番茄钟 |
| `wifi.*` / `bluetooth.*` | 开关 Wi-Fi 和蓝牙，连某个网络 | `CoreWLAN`、`IOBluetooth` | 定位 / 蓝牙 | 网络切换 |
| `defaults.*` | 读写系统和应用的偏好设置（`defaults`）、注册表 | `defaults` / 注册表 API | 无 | 隐藏设置调整 |
| `hosts` / `dns` | 改 hosts、DNS、代理 | 要 `exec.sudo` | 系统密码框 | 开发环境切换、屏蔽网站 |
| `wallpaper` | 换桌面壁纸 | `NSWorkspace` | 无 | 每日壁纸 |

### 2.11 后台和定时

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `background.run` | 网页关了也继续跑：App 后台保留一个看不见的网页，或者跑应用声明的脚本 | 隐藏的 Wails 窗口 / runner 里跑 | 无 | 监控、自动整理 |
| `schedule.*` | 定时触发（每天 9 点、每小时） | App 后端的定时器；App 没开时用 launchd | 无 | 定时备份、每日报告 |
| `trigger.*` | 事件触发：文件夹变了、插了 U 盘、连上某个 Wi-Fi、电量低、某个应用启动 | `fs.watch`、`DiskArbitration`、`NSWorkspace` 通知 | 无 | 自动化规则（像 Hazel、快捷指令的自动化） |

### 2.12 本机 AI 和 myco 自己

| 能力 | 做什么 | 怎么做 | 系统授权 | 能做的应用 |
| --- | --- | --- | --- | --- |
| `ai.local` | 调这台电脑上的本地模型（Ollama、LM Studio、Apple 自带模型） | `net.fetch` 到 localhost / Foundation Models 框架 | 无 | 离线摘要、分类 |
| `ai.vision` | 本机图像识别：分类、找人脸、条码、抠图 | `Vision` / Windows ML | 无 | 相册整理、扫码 |
| `agents.ask` | 让一个 agent（Claude Code、Codex）做件事，等它回（现在的 `send` 只能找发布者） | myco 消息 | 无 | 应用里调 agent 干活 |
| `agents.spawn` | 在这台电脑上开一个新会话，指定文件夹和权限 | machine 节点的 spawn（已经有） | 无 | 一键开个 agent 处理这批文件 |
| `myco.pages` / `myco.files` | 读写别的网页的数据，用工作区的文件存储 | Hub API | 无 | 跨应用的汇总面板 |
| `myco.workspace` | 工作区里的人、agent、电脑、项目 | Hub API | 无 | 团队看板 |

## 3. 已经有的、能直接接上的

| 现有的 | 在哪里 | 能接哪些能力 |
| --- | --- | --- |
| 存到下载并在访达里显示 | `internal/console/download.go` | `fs.saveAs`、`fs.reveal` |
| 打开链接 | `openurl.go` | `app.openURL` |
| 选图片（Mac） | `pickimage.go`、Wails `Dialog` | `fs.pick` |
| 钥匙串 | `keychain.go` | `keychain.app` |
| 听写、语音、TTS | `dictation*`、`voice*` | `speech.*`、`media.microphone` |
| 终端（PTY） | shell / exec 节点 | `exec.pty`、`exec.run` |
| 菜单栏、快捷面板、灵动岛 | `traymenu.go`、`quick_*`、`island_*` | `tray.item`、`window.panel` |
| 在这台电脑上开会话 | machine 节点 spawn | `agents.spawn` |
| 系统检查（各个 AI 装没装） | `info`、`ComputerCheck` | `system.apps` 的一部分 |

## 4. 分级（初稿，下一步再收）

先按「出了事有多严重」分五级，后面每一级再定授权方式：

| 级 | 是什么 | 例子 | 想法（待定） |
| --- | --- | --- | --- |
| **0 无感** | 不碰个人数据，不改东西 | `system.info`、`system.locale`、`notify`、`app.openURL`、`clipboard.write`、`speech.say`、`window.open`、`fs.reveal` | 直接给 |
| **1 读这台电脑** | 读到电脑的状况，但不是个人内容 | `system.*`、`fs.list`、`fs.stat`、`fs.scan`、`processes`、`apps`、`net.fetch` | 第一次用时问一次 |
| **2 读你的东西** | 读到文件内容、屏幕、声音、个人数据 | `fs.read`、`screen.*`、`clipboard.read`、`media.*`、`calendar` / `photos` / `contacts`、`browser.history`、`ui.tree`、`location` | 第一次问，界面上一直显示「正在用」 |
| **3 改东西** | 写、移动、删，能撤回的 | `fs.write`、`fs.trash`、`fs.move`、`app.quit`、写日历、`power.*`、`defaults.*` | 每批操作由 App 弹框确认 |
| **4 完全控制** | 等于把电脑交出去，撤不回 | `exec.*`、`exec.sudo`、`fs.delete`、`input.*`、`ui.act`、`computer.use`、`hosts` | 只给明确信任的应用，每次都能看到在做什么，随时一键停 |

另外有几条横向的问题，下一步一起定：

- **读到的东西会流到哪里**：读到的内容写进 `myco.data` 就是整个工作区可见，再加上 `net.fetch` 就能发到外面。读和发同时都给，等于能把东西带走。
- **谁写的应用**：官方的、工作区成员写的、agent 改过的。授权要不要绑定应用的版本，应用一改就重新问。
- **怎么让人随时看到、随时停**：菜单栏里显示「某某应用正在录屏 / 控制鼠标」，有一个总的「停下所有应用」。
- **记录**：每次调用记一行，电脑页能看。

## 5. 下一步

1. 你看一遍这份清单：有没有漏的，哪些一开始就要。
2. 按第 4 节逐项定分级和授权方式。
3. 定第一批做哪些。建议按「空间清理」和「让 agent 看屏幕、操作电脑」两个例子倒推：
   - 空间清理要 `system.disks`、`fs.scan`、`fs.trash`、`system.apps`、`exec.run`；
   - computer use 要 `screen.capture`、`input.*`、`ui.tree`。
