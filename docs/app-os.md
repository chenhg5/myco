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

## 能力和各系统的做法

| 能力 | 做什么 | macOS | Windows | Linux |
| --- | --- | --- | --- | --- |
| `system.info` | 系统、机型、芯片、内存、开机时长 | sysctl | 注册表、Win32 | /etc/os-release、/proc |
| `system.disks` | 各磁盘的容量和剩余 | getfsstat（不列系统卷） | GetLogicalDrives | /proc/self/mounts + statfs |
| `system.processes` | 进程：CPU、内存 | `ps` | Toolhelp（没有 CPU） | `ps` |
| `process.kill` | 结束进程（`force` 立即结束） | SIGTERM / SIGKILL | 结束 | SIGTERM / SIGKILL |
| `fs.paths` | 个人文件夹、桌面、文稿、下载、图片、临时、缓存、废纸篓… | 固定位置 | Known Folders | `user-dirs.dirs` |
| `fs.list` `fs.stat` `fs.read` `fs.write` `fs.mkdir` `fs.move` `fs.copy` `fs.delete` | 文件 | 同 | 同 | 同 |
| `fs.trash` | 移进废纸篓 / 回收站 | 系统的 `trash` 命令（macOS 15 起，能「放回原处」）；没有时移进 `~/.Trash` | 回收站（.NET FileSystem） | freedesktop 废纸篓（文件管理器能还原） |
| `fs.scan`（任务） | 按占用汇总文件夹、找最大的文件；不进别的磁盘，硬链接只算一次 | 同 | 同 | 同 |
| `fs.reveal` `fs.open` | 在访达里显示，用默认程序打开（`app` 指定程序） | `open -R`、`open -a` | Explorer、默认程序 | 文件管理器、`xdg-open` |
| `exec.run` | 跑命令（`argv`）或脚本（`shell`），跑完回退出码和输出（默认 60 秒、1 MB） | `/bin/sh` | PowerShell | `/bin/sh` |
| `exec.spawn`（任务） | 起程序：输出实时推回来，能写入，能停（连同它起的子进程） | 同上 | 同上 | 同上 |
| `clipboard.read` `clipboard.write` | 剪贴板里的文字 | pbpaste / pbcopy | PowerShell | wl-clipboard、xclip 或 xsel |
| `notify` | 系统通知 | osascript | PowerShell 的通知 | notify-send |
| `screen.capture` | 截屏：全部、某个显示器（`display`）或一块区域（`region`）；PNG 或 JPEG，回 base64 | `screencapture` | .NET 截屏（按真实像素） | grim、gnome-screenshot、spectacle、import 或 scrot |
| `net.fetch` | 从 App 发 HTTP 请求（网页自己不能上网）；文本回文字，其他回 base64 | 同 | 同 | 同 |

命令在 App 那边跑，环境和登录 shell 一样（PATH 里有 Homebrew、nvm 等），起点是个人文件夹，环境变量里没有 myco 的令牌。

**系统授权**：有些能力要先在系统设置里给 myco 打开，否则系统会拦下来或给空的结果。

- macOS：
  - 截屏要「屏幕录制」，没有时只能截到桌面背景；
  - 读 `~/Library` 下的一部分文件夹要「完全磁盘访问」，扫描时没权限的地方记进 `skipped`。
- Linux：剪贴板、通知、截屏要装上表里对应的程序，没有时 `os.list()` 里会写出缺什么。

## 在开发时试

```bash
go run ./internal/console/devos -addr 127.0.0.1:7790 &
cd web && MOCK_OS_URL=http://127.0.0.1:7790 npm run dev:app
```

模拟的 App 会把 `myco.os` 转给这台电脑，并多出一个网页「这台电脑」，上面把能力挨个试一遍。注意它是真的在用你的电脑。
