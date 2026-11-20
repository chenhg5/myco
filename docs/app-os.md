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

> **现在没有任何限制**：任何在 App 里打开的网页都能用这些能力，不会先问人。这一版先把能力做出来，授权和分级下一步再加。每次调用都记在 `~/.myco/logs/app-os.jsonl` 里（哪个网页、调了什么、结果怎样；参数只记前 300 个字）。

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
