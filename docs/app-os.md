# 应用用这台电脑：`myco.os`

在 myco App 里打开的网页（应用、agent 做的网页），可以用 App 所在的这台电脑：

- 看系统和磁盘；
- 读写文件、移进废纸篓、按大小扫描文件夹；
- 跑命令；
- 读写剪贴板；
- 发通知、截屏；
- 从 App 联网。

这些都在网页的桥上，名字是 `myco.os`。

App 版本里有了这一项，桥的 `myco.features` 里会有 `"os"`。

> **现在没有任何限制**：任何在 App 里打开的网页都能用这些能力，不会先问人。这一版先把能力做出来，授权和分级下一步再加。每次调用都记在 `~/.myco/logs/app-os.jsonl` 里（哪个网页、调了什么、结果怎样；参数仅记字节数，不记录内容）。

## 怎么调

```js
const os = window.myco.os;

await os.has("screen.capture");               // 这台电脑能不能做
const info  = await os.system.info();         // { os, name, version, arch, cpu_model, memory, uptime, … }
const disks = await os.system.disks();        // [{ mount, fs, total, free, available, removable, … }]
const dir   = await os.fs.list({ path: "~/Downloads" });
const text  = await os.fs.read({ path: "~/notes.md" });
await os.fs.trash({ paths: ["~/Downloads/old.dmg"] });
const r     = await os.exec.run({ argv: ["git", "status"], cwd: "~/code/myco" });

// 跑得久的是「任务」：边跑边发事件，可以停，最后 done 给结果
const scan = os.fs.scan({ path: "~", depth: 3, top: 50 });
scan.on("progress", (p) => show(p.files, p.bytes, p.current));
const result = await scan.done;               // { roots, largest, files, bytes, skipped, … }
// scan.cancel()

const p = os.exec.spawn({ shell: "npm run dev", cwd: "~/code/site" });
p.on("stdout", (t) => log(t));
p.write("y\n"); p.end();                      // 写给它的标准输入
```

- 路径必须是绝对路径，或者以 `~` 开头（代表个人文件夹）。
- 出错时 `err.code` 是这些之一：
  - `invalid_argument`
  - `not_found`
  - `exists`
  - `permission_denied`
  - `unsupported`：这台电脑或这个系统做不到
  - `timeout`
  - `cancelled`
  - `too_large`
  - `failed`
- 不在上面的能力，用 `os.call(name, args)` 调，任务用 `os.start(name, args)` 起。新版 App 可能多出新的能力。
- 关掉网页时，它起的任务都会停掉。
- 类型定义在 `templates/src/_lib/myco.ts` 的 `MycoOS` 里。

## 官方应用里用：SDK（`templates/src/_lib/os.ts`）

官方应用（`templates/src/<app>`）用打包好的 SDK。它比直接调 `myco.os` 多几样东西：

```ts
import { os, osState, osStateText, OSError, has, useOS, useJob, follow, formatBytes, basename, imageSrc } from "../_lib/os";

if (osState !== "ready") showNote(osStateText());  // 不在 App 里（"outside"），或 App 太旧（"old"）

const disks = await os().system.disks();            // 出错一律是 OSError，看 err.code
if (await has("fs.thumbnail")) { /* … */ }

// React：一个任务的进度、结果、错误，组件卸载时自动停
const scan = useJob(() => os().fs.scan({ path: "~", depth: 6, min_fraction: 0.001 }));
// scan.start()  scan.progress  scan.value  scan.error  scan.running  scan.cancel()

// 不用 React：follow(job, { onEvent, signal }) 等它跑完，AbortSignal 能停
const ctl = new AbortController();
const r = await follow(os().fs.search({ path: "~", name: "发票", ext: ["pdf"] }), { onEvent: (t, d) => t === "match" && add(d), signal: ctl.signal });
```

- `osState`：`"ready"`、`"old"` 或 `"outside"`。`osStateText()` 给出一句能直接给人看的话。
- `os()`：和 `myco.os` 的用法一样，但出的错都是 `OSError`，任务的 `done` 也是。用不了时直接抛出 `unsupported`。
- `has(name)` 和 `capabilities()`：这台电脑能做什么。只问一次，之后用记下的结果。
- `useOS()`：在组件里拿到状态、系统信息和 `has`。
- `useJob(make, onEvent?)`：在组件里跑一个任务。
- `follow(job, {onEvent, signal})`：不用 React 时跟着一个任务。
- 小工具：
  - `formatBytes`：按 1000 进位，和 macOS、DaisyDisk 一样；`{binary: true}` 按 1024；
  - `basename`、`dirname`、`join`、`extname`：认得 Windows 的路径；
  - `fromBase64`、`toBase64`；
  - `readText`、`readBytes`、`writeText`、`writeBytes`、`readJSON`、`writeJSON`；
  - `imageSrc`：把截屏或缩略图变成 `<img>` 能用的地址。

agent 写的普通网页不打包，直接用 `window.myco.os`，用法见上一节。

## 能力和各系统的做法

| 能力 | 做什么 | macOS | Windows | Linux |
| --- | --- | --- | --- | --- |
| `system.info` | 系统、机型、芯片、内存、开机时长 | sysctl | 注册表、Win32 | /etc/os-release、/proc |
| `system.disks` | 各磁盘的容量和剩余 | getfsstat（不列系统卷） | GetLogicalDrives | /proc/self/mounts + statfs |
| `system.processes` | 进程：CPU、内存 | `ps` | Toolhelp（没有 CPU） | `ps` |
| `process.kill` | 结束进程（`force` 立即结束） | SIGTERM / SIGKILL | 结束 | SIGTERM / SIGKILL |
| `system.usage` | 此刻的 CPU（所有核的百分比）、内存（总量、已用、可用）、负载 | `top` 第二次采样（约 1 秒）、`vm_stat`（按「活动监视器」的算法）、sysctl | GetSystemTimes 采两次、GlobalMemoryStatusEx（没有负载） | `/proc/stat` 采两次、`/proc/meminfo`、`/proc/loadavg` |
| `system.apps` | 装了哪些应用：名字、版本、ID、位置；`size: true` 再算每个占多大 | `/Applications` 等处的 .app，读 Info.plist | 「应用」设置里那份（注册表 Uninstall，带安装程序报的大小） | 菜单里的 .desktop（系统、Flatpak、Snap、自己的） |
| `system.settings` | 打开系统设置的某一页：`full_disk`、`screen_recording`、`accessibility`、`notifications`、`privacy`、`storage`、`network`、`display`、`sound`、`bluetooth` | 「系统设置」对应的面板 | `ms-settings:` 对应的页 | GNOME 设置对应的页（没有对应的就 `unsupported`） |
| `fs.paths` | 个人文件夹、桌面、文稿、下载、图片、临时、缓存、废纸篓… | 固定位置 | Known Folders | `user-dirs.dirs` |
| `fs.list` `fs.stat` `fs.read` `fs.write` `fs.mkdir` `fs.move` `fs.copy` `fs.delete` | 文件 | 同 | 同 | 同 |
| `fs.trash` | 移进废纸篓 / 回收站 | 系统的 `trash` 命令（macOS 15 起，能「放回原处」）；没有时移进 `~/.Trash` | 回收站（.NET FileSystem） | freedesktop 废纸篓（文件管理器能还原） |
| `fs.scan`（任务） | 按占用汇总文件夹、找最大的文件。并行读，不进别的磁盘，硬链接只算一次；`min_fraction` 把占整体太小的并成「更小的项目」（`more_*`），扫一整块盘也只有几千个节点。磁盘的 `scan_path` 是该扫哪里：Mac 的启动磁盘要扫它的 Data 卷 | 同 | 同 | 同 |
| `fs.search`（任务） | 按名字（一部分，或 `report*.pdf` 这样的模式）、后缀、类型、大小、修改时间找；边找边推 `match` | 同 | 同 | 同 |
| `fs.watch`（任务） | 盯着一个文件夹（可以连子文件夹），新建、改动、删除就推 `change`；直到停掉。每隔一段时间比一次，各系统一样 | 同 | 同 | 同 |
| `fs.thumbnail` | 缩略图：图片（png、jpeg、gif）各系统都行 | 另外 PDF、视频、文档等用「快速查看」 | 只有图片 | 只有图片 |
| `fs.pick` `fs.saveAs` | 系统的打开框（文件或文件夹，可多选，可限定后缀）、存储为框；关掉是 `cancelled` | AppleScript 的选择框 | Windows 的打开 / 另存为框 | zenity 或 kdialog |
| `fs.reveal` `fs.open` | 在访达里显示，用默认程序打开（`app` 指定程序） | `open -R`、`open -a` | Explorer、默认程序 | 文件管理器、`xdg-open` |
| `exec.run` | 跑命令（`argv`）或脚本（`shell`），跑完回退出码和输出（默认 60 秒、1 MB） | `/bin/sh` | PowerShell | `/bin/sh` |
| `exec.spawn`（任务） | 起程序：输出实时推回来，能写入，能停（连同它起的子进程） | 同上 | 同上 | 同上 |
| `clipboard.read` `clipboard.write` | 剪贴板里的文字 | pbpaste / pbcopy | PowerShell | wl-clipboard、xclip 或 xsel |
| `notify` | 系统通知 | osascript | PowerShell 的通知 | notify-send |
| `screen.capture` | 截屏：全部、某个显示器（`display`）或一块区域（`region`）；PNG 或 JPEG，回 base64 | `screencapture` | .NET 截屏（按真实像素） | grim、gnome-screenshot、spectacle、import 或 scrot |
| `net.fetch` | 从 App 发 HTTP 请求（网页自己不能上网）；文本回文字，其他回 base64 | 同 | 同 | 同 |
| `audio.sources` | 能录的声音：麦克风（`mic`）、电脑放出的声音（`system`）、某个出过声音的应用（`app:…`） | 麦克风；macOS 14.2 起有电脑的声音和各个应用（Core Audio 的进程列表） | 还没有 | PulseAudio / PipeWire：默认麦克风、默认输出的 monitor、正在放声音的流 |
| `audio.record`（任务） | 录音：每个来源一条音轨，各存一个 16 kHz 单声道 WAV（`~/.myco/recordings/<时间>/<轨>.wav`），每 100 毫秒推一次音量；写入 `pause`、`resume`、`stop` 来暂停、继续、结束 | 要在 myco App 里：麦克风用 AVAudioEngine，电脑的声音用 Core Audio process tap（macOS 14.2+，第一次会问「系统录音」权限） | 还没有 | `parec` |
| `speech.status` `speech.install`（任务） | 本机转写的引擎和模型：sherpa-onnx 官方编好的程序（约 20 MB）和 SenseVoice 模型（约 160 MB，中英粤日韩），第一次用时下载到 `~/.myco/speech`（`MYCO_SPEECH_MIRROR` 换下载地址） | arm64、x64 | x64 | x64 |
| `speech.diarize`（任务） | 分出录音里的不同说话人：一段段「谁在什么时候说话」（说话人按第一次开口的顺序编号）；知道几个人时给 `speakers` 更准。模型（pyannote 切分和 3D-Speaker 声纹，约 35 MB）随 `speech.install` 下载 | 同上 | 同上 | 同上 |
| `speech.transcribe`（任务） | 把录音转成文字：一句一段，带开始和结束时间，边转边推 `segment`；长录音按四分钟左右在最安静处切开转 | WAV；别的格式用 `afconvert` 转 | WAV；装了 ffmpeg 时别的格式也行 | 同 Windows |

命令在 App 那边跑，环境和登录 shell 一样（PATH 里有 Homebrew、nvm 等），起点是个人文件夹，环境变量里没有 myco 的令牌。

**系统授权**：有些能力要先在系统设置里给 myco 打开，否则系统会拦下来或给空的结果。

- macOS：
  - 截屏要「屏幕录制」，没有时只能截到桌面背景；
  - 读 `~/Library` 下的一部分文件夹要「完全磁盘访问」，扫描时没权限的地方记进 `skipped`；
  - 录麦克风要「麦克风」权限，录电脑的声音要「屏幕与系统录音」里的「仅系统录音」权限（第一次录时系统会问）。
- Linux：剪贴板、通知、截屏要装上表里对应的程序，没有时 `os.list()` 里会写出缺什么。

## 在开发时试

```bash
go run ./internal/console/devos -addr 127.0.0.1:7790 &
cd web && MOCK_OS_URL=http://127.0.0.1:7790 npm run dev:app
```

模拟的 App 会把 `myco.os` 转给这台电脑，并多出一个网页「这台电脑」，上面把能力挨个试一遍。注意它是真的在用你的电脑。

## Local proxy controller

`proxy.profile` and `proxy.control` are optional single-call capabilities for an
already running Mihomo or sing-box Clash API. Check `os.has("proxy.profile")`
before offering a connection. Both require the App's local credential store.
They never alter system proxy, DNS, services or TUN settings.

```js
const profile = await os.call("proxy.profile", {
  action: "save", engine: "mihomo", endpoint: "http://127.0.0.1:9090",
  secret: enteredPassword // never put this in shared page records
}); // {endpoint, engine, has_secret}; never returns secret
const state = await os.call("proxy.control", {action: "snapshot"});
await os.call("proxy.control", {action: "select", group: "Proxy", name: "Node 1"});
await os.call("proxy.control", {action: "mode", mode: "rule"});
const probe = await os.call("proxy.control", {action: "delay", name: "Node 1"});
await os.call("proxy.control", {action: "close", id: connectionID});
await os.call("proxy.profile", {action: "forget"}); // does not stop the core
```

Profile actions: `get` (default), `save`, `forget`. `save` accepts `engine`
(`mihomo`/`sing-box`), `endpoint`, optional `secret`; omitted secrets stay unchanged
for the same endpoint, an empty string clears them, and endpoint changes clear
the previous secret unless explicitly supplied. Profiles are scoped by the
calling workspace and page. The system keychain (or configured Linux credential
fallback) stores the entire profile.

Control actions: `snapshot`, `mode`, `select`, `delay`, `close`. Snapshot returns
`{version, config, proxies, traffic, warnings, at}`. `config` contains only mode, optional mode-list,
ports, allow-lan, log-level, and tun.enable. `proxies` maps names to
`{name,type,now?,all?,history?:[{delay}]}`. `traffic` contains `downloadTotal`,
`uploadTotal` and connections with id, limited metadata, upload/download, start,
chains and rule. Unsupported snapshot sections are warnings; callers must not
present their empty values as live measurements. `delay` returns `{delay}` in ms.
Mutations return `true`. Authentication failures are `permission_denied`, missing
controller operations `unsupported`, unavailable/invalid controllers `failed`.
Every request is deadline- and size-bounded, follows no redirects, and accepts
only literal loopback addresses. See [Proxy console](proxy-console.md).

## App language and appearance

The host App supplies its **resolved** display settings, including manual choices
that differ from the operating system:

```js
const { preferences } = await myco.ready;
// preferences: { language: "en" | "zh", theme: "light" | "dark" }
console.log(myco.preferences); // the current value, not just the ready snapshot
const off = myco.on("preferences", (value) => updateAppearance(value));
```

Check `myco.features.includes("preferences")` for support. The bridge sets the
page's `html.lang`, `data-myco-theme` and `color-scheme`. The frame also gets an
explicit color scheme, and a `myco_lang` URL hint before the app module runs.
Changing language reopens the frame with its new language; changing appearance
updates the open frame without reloading or writing page data. Display hints
are local to the App and are never stored as page records.

Official app sources use `uiText` from `templates/src/_lib/i18n.ts` for source UI
strings and `locale()` for dates and numbers. Pass only application copy, never
user titles, notes, document contents, option values or enum keys, to the text
helper. Rich text editors choose the matching BlockNote dictionary. The template
builder preserves system media styles for old Apps and also generates explicit
`data-myco-theme` styles for current Apps. Canvas/editor code uses `themeMedia()`
from `_lib/appearance.ts`; the browser's global `matchMedia` API is not replaced.

Templates can include `locales.json` alongside `myco.json`:

```json
{
  "en": {"name": "Notes", "description": "Write together."},
  "zh": {"name": "笔记", "description": "一起写。"}
}
```

It contains at most 20 language tags and 32 KiB. Names are one line of at most
80 characters; descriptions are at most 300 characters. Missing entries fall
back to the manifest. Keeping this file separate allows old Hubs with strict
manifest validation to install the same apps. New Hubs include its `locales` in
template list/get results; old Apps ignore the new field. A page's chosen name
and all its user content remain unchanged. The current App also bundles official
catalog translations for Hubs that do not return `locales` yet; custom catalog
translations need a Hub that supports this sidecar.

## Playing public audio and video

A page cannot load media from the internet itself (its CSP allows only its
own origin). Where `myco.features` includes `"net-media"`, `myco.media.url(u)`
turns a public `http(s)` address into one the page may play:

```ts
const audio = new Audio(myco.media.url("https://example.com/episode.mp3"));
await audio.play();
```

The App streams the file as it plays, passing `Range` on so seeking works,
and stops when the page stops asking (pause, seek, close). Only public
addresses are reached: a name that resolves to this computer or the local
network is refused (`403`), redirects included. Only audio, video, HLS
playlists and `application/octet-stream` come back; anything else is `415`.
Older Apps and the browser console lack the feature: check it and say so,
rather than fetching the whole file with `net.fetch`.

## Optional Remote Desktop SDK

`myco.features.includes("remote")` exposes `myco.remote.call(action, args)`.
It is separate from `myco.os`, which operates on the App's local computer.
The SDK uses the page's current workspace, not an arbitrary Hub URL. The trusted
App asks before installation, removal, enabling hosting or connecting; an
embedded app cannot approve its own request. These permissions do not inherit
from a page's ability to run local commands.

```ts
const computers = await myco.remote.call("devices");
const component = await myco.remote.call("status");
// Both actions below need explicit App consent.
await myco.remote.call("install");
const sharing = await myco.remote.call("enable", {control: false});
// sharing.pairing_code is private to this page: never write it to shared data.
await myco.remote.call("disable");
```

`templates/src/_lib/remote.ts` provides the typed controller and browser media
adapter. The host process, codec and input implementations live in the optional
`components/remote` module. Core myco does not link its WebRTC dependency.
Closing/reloading a host page stops sharing; changing language reloads the page,
while theme changes update live. Hosts require Mac/Windows, system permissions
and an unlocked desktop. The initial adapter supports direct LAN/Tailscale video
and keyboard/mouse, without audio, automatic public relay or unattended service.
The component is downloaded separately and may be uninstalled from the app's
settings. It is not installed or started for people who do not use it.
An App keeps using an older compatible release it finds installed; `status`
then returns `update_available: true`, and `install` fetches the App's own
release. Agent actions (`remote.desktop`) need component 0.3.0 or later.

The iOS page bridge advertises `remote-controller`: `devices`, `connect`,
`session`, `disconnect` and `close` use the current workspace credential.
`status` returns `reason: "controller_only"`; installation and hosting actions
are refused. Native confirmation names the authorized computer and requested
control mode. Each page can access only its own sessions. Backgrounding the App,
reloading or closing a page revokes pending requests and active sessions.
`myco.remote.onDisconnect(fn)` lets a page immediately clear its video and input
state on backgrounding. Returning to the App requires an explicit new connection.
No camera or microphone permission is needed for this receive-only video path.

The shared controller implements a relative touchpad, two-finger scrolling,
explicit drag mode, zoom and a keyboard. Component 0.2.0 sends an encrypted
data-channel capability event `{type:"capabilities",input_features:["text"]}`.
Only then may a controller send `{kind:"text",text:"…"}` (at most 256 Unicode
scalars / 1,024 UTF-8 bytes; control characters are rejected). Text uses native
Unicode key events, without the clipboard. Legacy components still accept
physical keys; the UI retains unsupported text and asks for a component update.

The iOS controller enters native landscape presentation only after an authorized
connection exists. Its App toolbar and status bar are hidden; a native exit
button remains above untrusted page content. Disconnect, page dismissal and
background revocation restore the normal portrait presentation.

### Local screen recordings

`myco.os.screen.record({interactive?, microphone?, clicks?, max_seconds?})`
returns a job. Write `"stop"` to finalize the movie; `done` resolves to
`{path, bytes}`. Originals remain in a private directory under
`~/.myco/recordings/screen-*`; `myco.os.mediaURL(path)` reads them on the page's
own origin. MOV, MP4 and WebM are allowed alongside audio recordings, with the
same directory and symlink checks. Files are never uploaded by this capability.

macOS uses native recording APIs: ScreenCaptureKit recording output on macOS 15+,
with AVFoundation on older Macs, a region or the main screen, optional microphone
and click indicators. Stop waits for the movie-file completion callback; it does
not interrupt a `screencapture` subprocess. Screen Recording permission
(and microphone permission when requested) is controlled by macOS. Windows uses
a separately installed FFmpeg on the login-shell PATH, with `gdigrab` and
`h264_mf`, and records the desktop without audio; region/microphone options are
rejected. Other systems report the native capability as unavailable. Apps should
check `os.has("screen.record")` and can offer browser display capture where the
host supports it, or import an existing recording.

The default limit is 600 seconds, maximum 3600. Only one native screen recording
runs at a time. Cancelling requests a graceful stop to finalize the original;
Mac finalization has a ten-second timeout; Windows kills a stuck encoder after
eight seconds. The recording app cancels its
job when leaving and warns before closing an active recording. No capture starts
merely by opening the app, and no remote-desktop component is started.

The recording app waits for capability discovery before enabling capture. On a
Mac or Windows App without native recording support it does not silently fall
back to browser display capture: an embedded WebView may reject that request
without opening a picker. The UI explains whether the App needs updating or the
Windows encoder is missing. On supported Macs it offers a button for System
Settings → Privacy & Security → Screen & System Audio Recording, naming the
currently used myco / myco-preview app; microphone permission is separate.

On macOS, `screen.record` also accepts a non-interactive `region: {x,y,w,h}`
(rectangle in screen points; each side 16–16384 points). This is useful for
bounded capture of a known region. `region` and `interactive` are mutually
exclusive; Windows does not support regions yet. The Mac GUI advertises
`screen.region`, a native drag-to-select overlay returning `{x,y,w,h}`. With
`interactive: true`, `screen.record` emits `selecting`, waits for the user to drag
a rectangle and confirm Start Recording. A cancellable five-second countdown
then gives time to prepare; no movie capture begins during that countdown.
The `started` event follows the native recording-start callback. Cancelling the
picker produces `cancelled`, not a successful recording. The App hides its
windows during capture. A click-through region outline and a movable floating
bar stay visible, showing preparation, recording time, and saving. Its Stop & edit
button (also available in the menu bar) finalizes the movie and returns to the
editor. On ScreenCaptureKit, recorder-control windows are explicitly excluded
from the movie. Headless servers do not advertise the picker. Older
Apps without `screen.region` should offer main-screen recording instead of
claiming a region picker is available. Coordinates use screen points, not Retina
movie pixels; negative monitor coordinates are preserved.
Recorder failures preserve bounded diagnostic text rather than misreporting every
failed launch as a permission problem.

The Mac App rechecks recording access on every attempt, but actively asks the
system at most once per process. A denied or pending grant therefore does not
open repeated consent dialogs; changing permission in Settings is recognized
when the system makes it effective (which can require quitting and reopening
the App). Revoked access is never cached as granted.

## Network access on iPhone

The iOS page bridge exposes `myco.net.fetch(args)` and the `net` feature, without
claiming support for desktop `myco.os` operations. Official apps should use the
shared `templates/src/_lib/net.ts` adapter: it selects `myco.net` on iPhone and
falls back to `myco.os.net` in existing desktop Apps. Test network availability
separately from the `os` feature. A page-only update cannot add this native
capability to an older installed iPhone App.

The request/response shapes match desktop `net.fetch`, including custom headers,
text/base64 bodies, byte caps, redirects and HTTP status/response headers. iOS
supports GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS, through the phone's
network/proxy settings. The default response cap is 16 MB, the maximum is 64 MB;
responses over the cap set `truncated`. Requests have a bounded timeout (at most
ten minutes) and are cancelled on navigation, reload, close or backgrounding.
No workspace authorization, saved HTTP credentials or ambient cookies are
attached, and cross-origin redirects strip credential headers. The sandbox's
external connection policy is unchanged; all requests use this bridge.
