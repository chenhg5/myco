# 发版手册

给负责发版的 agent 看。只有维护者明确说「发版」「发个新版本」「发 vX.Y.Z」时才做；平时不要动 `VERSION`。每做完一步就用 `myco message progress` 报一句进展，有问题就停下来说明，不要硬发。

## 0. 准备（第一次）

- 仓库在 `~/code/myco-team/release`，只用于发版，不在这里写功能。
- 要有 `curl` 和 `python3`（查 GitHub 的状态用）。
- 不用在本地跑界面冒烟：它在 GitHub CI 里跑（`ci` 的 `ui` 任务，四种窗口组合各一台机器）。想在本地跑的话，需要 Playwright 和它的 Chromium：`npm install -g playwright && npx playwright install --with-deps chromium`。

## 1. 检查：main 上没有测试失败

发版**不等 CI 跑完**：`release` 工作流自己会构建所有平台，还会在 Linux 的二进制上跑一遍 smoke。只看 main 最新一次 CI 有没有**测试**失败：

```bash
git checkout main && git pull --rebase origin main
scripts/release-status.sh "$(git rev-parse HEAD)" ci   # 不加 --wait
```

- 失败在测试上（`go`、`windows`、`web`、界面冒烟里的某个检查）：先修，不发版。
- 还在跑，或者只是机器问题（装 Playwright 卡住、macOS 机器排队）：直接往下走，不等。
- CI 只在改了 `web/` 时才跑界面冒烟（大约 8 分钟）；新的推送会取消同一分支上旧的那次 CI。

## 2. 版本号和 CHANGELOG

- 版本号：没有特别说明就在最新的 tag 上加一位补丁号（v0.19.6 → v0.19.7）。最新 tag：`git fetch --tags && git describe --tags --abbrev=0`。
- 看改了什么：`git log --format='%h %s' <上个版本>..HEAD | grep -v ' Merge '`。必要时用 `git show -s <hash>` 读提交说明。
- 在 `CHANGELOG.md` 最上面加一节 `## vX.Y.Z`：用中文写用户能感知的变化，每条一句或几句，重要的加粗开头；纯内部的改动（测试、重构）和 iOS（不在发版产物里）不写。写法照前几节。
- 本地预览这一节：`scripts/changelog-section.sh vX.Y.Z`。

## 3. 提交、推送

```bash
echo vX.Y.Z > VERSION
git add CHANGELOG.md VERSION
git commit -m "Release vX.Y.Z"
git pull --rebase origin main   # 有新提交进来：只是 ios/ 或文档就继续；有代码改动，先等新的 HEAD 的 CI 过了（第 1 步）再推
git push origin HEAD:main
sha=$(git rev-parse HEAD)       # 记下这个 sha
```

- `VERSION` 和 `CHANGELOG.md` 必须在同一个提交里：发版流水线会把这一节当作 GitHub Release 的说明。
- 推送后 GitHub 的 `release` 工作流会构建所有平台；仓库里有签名用的 Secrets（见文末「签名放在 GitHub 上」）时，它在 macOS 机器上签名、公证、做 DMG，一起放进 Release。整个过程不需要任何个人电脑。

## 4. 等 GitHub

```bash
scripts/release-status.sh "$sha" release --wait
```

- `release` 是 `completed/success`（退出码 0）就继续，不等 `ci`。`release` 通常 5–10 分钟，大部分时间在等 Apple 公证。
- `release` 失败：发版没成功，报告失败的步骤和日志。在「创建 Release」这一步被 GitHub 拒（HTTP 403）的话，重跑一次；还不行就按下一步在 Mac 上建。
- 之后 `ci` 失败但 `release` 成功：Release 已经发出去了。找出失败的测试，修好（通常是 Windows 上的测试问题），推一个修复提交，并在报告里说明。

## 5. Release 里没有 myco.dmg 时：在 Mac 上签名（备用）

仓库还没设签名 Secrets、GitHub 上签名那一步失败了，或者 Mac 构建机一直排不上队（Mac 那一步被取消，Release 照样发出，只是没有 Mac 的文件），Release 里就没有 `myco.dmg`。这时用 Mac 签名、上传：

```bash
myco exec --to mac-exec --cmd "~/code/myco-release.sh start vX.Y.Z $sha"
myco exec --to mac-exec --cmd "~/code/myco-release.sh status"   # 每分钟查一次，直到 UPLOADED 或 FAILED
```

- 通常 5–10 分钟；FAILED，或者公证卡在 In Progress 超过 30 分钟：停下来，把日志的最后几行报给维护者。
- `mac-exec` 不在线（Mac 睡着了）：告诉维护者把 Mac 唤醒并解锁，等它在线再执行。
- Release 里已经有 `myco.dmg` 时，Mac 的脚本不会覆盖它。
- DMG 的窗口（背景图、箭头、图标位置）由 `scripts/mac-dmg.sh` 用 dmgbuild 生成，背景在 `cmd/myco-app/build/darwin/dmg/`（`background.html` 是它的源，改了以后重新截 1x、2x 两张 PNG，凹槽位置和 `scripts/mac-dmg-settings.py` 里的图标坐标保持一致）。装不上 dmgbuild 时退回朴素的 DMG，不影响发版。只想看看效果：在 Mac 上 `make app` 后运行 `scripts/mac-dmg.sh bin/myco.app ~/Desktop/myco-preview.dmg`。

## 6. 核对

```bash
scripts/release-verify.sh vX.Y.Z
```

两个文件都是 `OK`、最后一行是 `notes: the CHANGELOG section` 才算发完。

## 7. 报告

回复派活的人：版本号、Release 链接、CHANGELOG 的要点、CI 和 Mac 的结果、核对结果。提醒：Hub 所在的服务器要 `sudo myco upgrade` 才能用上新版本的 Hub 功能。

## 规则

- 不打印、不提交任何令牌或密钥；签名身份和证书只在维护者的 Mac 和仓库的 Secrets 里，不写进仓库、文档和消息。
- 只在维护者要求时发版；一次只发一个版本。
- 拿不准就问，不要猜。

## 签名放在 GitHub 上（维护者做一次）

`release` 工作流的 macOS 任务看到这五个仓库 Secrets，就自己签名、公证。证书和密码只经过维护者自己的 Mac 和 GitHub，不经过 agent，也不会出现在日志里（工作流按证书的哈希签名，不打印名字）。

| Secret | 是什么 |
|---|---|
| `MAC_CERT_P12_BASE64` | Developer ID Application 证书和私钥导出的 .p12，base64 编码 |
| `MAC_CERT_PASSWORD` | 导出 .p12 时设的密码 |
| `NOTARY_APPLE_ID` | 公证用的 Apple ID（邮箱） |
| `NOTARY_TEAM_ID` | 开发者团队 ID（10 位）。可以不设：工作流从 Developer ID 证书里读团队 ID，读不到才用它 |
| `NOTARY_PASSWORD` | 这个 Apple ID 的 App 专用密码（appleid.apple.com → 登录与安全 → App 专用密码） |

在 Mac 上：

1. 打开「钥匙串访问」→「我的证书」，右键名字以 **Developer ID Application** 开头的那一项（不是 Apple Development）→「导出」，存成桌面上的 `myco-sign.p12`，设一个密码。在「证书」里导出的只有证书、没有私钥，不会让你设密码，那个用不了。哪个钥匙串的「我的证书」里都找不到它，多半是证书和私钥分在了两个钥匙串（比如证书在「系统」、私钥在「登录」）：把证书复制一份到私钥所在的钥匙串（`security find-certificate -c "Developer ID Application" -p /Library/Keychains/System.keychain > /tmp/devid.pem && security import /tmp/devid.pem -k ~/Library/Keychains/login.keychain-db`），它就会出现在那里的「我的证书」里。
2. 在仓库目录的终端里（`gh` 要已登录，对仓库有管理权限；会提示你输入值，输入时不显示）：

   ```bash
   scripts/mac-signing-secrets.sh ~/Desktop/myco-sign.p12   # 证书和密码：先在本机核对密码能打开证书，再一起设
   gh secret set NOTARY_APPLE_ID -R chenhg5/myco
   gh secret set NOTARY_TEAM_ID -R chenhg5/myco
   gh secret set NOTARY_PASSWORD -R chenhg5/myco
   rm ~/Desktop/myco-sign.p12
   ```

3. 不发版也能检查：手动运行 `release` 工作流并勾选 `sign_only`（Actions → release → Run workflow），它只在 macOS 机器上签名、公证，什么都不发布；成功后产物 `app` 里有签好的 `myco.dmg`。

证书过期或换了 App 专用密码，重新设对应的 Secret 就行。
