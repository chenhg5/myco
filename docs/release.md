# 发版手册

给负责发版的 agent 看。只有维护者明确说「发版」「发个新版本」「发 vX.Y.Z」时才做；平时不要动 `VERSION`。每做完一步就用 `myco message progress` 报一句进展，有问题就停下来说明，不要硬发。

## 0. 准备（第一次）

- 仓库在 `~/code/myco-team/release`，只用于发版，不在这里写功能。
- UI 冒烟测试要 Playwright 模块和它的 Chromium（装一次就行，不改仓库依赖）：`npm install -g playwright && npx playwright install --with-deps chromium`。找不到浏览器时，用 `UI_SMOKE_EXECUTABLE=<Chromium 路径>` 指过去。

## 1. 检查

```bash
git checkout main && git pull --rebase origin main
make check                     # vet + 测试 + smoke，必须全过
make web && git status --short # dist 不能有变化；有变化说明有人改了 web/src 没重新生成，停下来报告
cd web && npm run typecheck && npm run test:ui && cd ..
```

`test:ui` 前如果 7781 端口被占用，先结束残留的 mock 和 vite 进程。任何一步失败：不发版，报告失败的输出。

## 2. 版本号和 CHANGELOG

- 版本号：没有特别说明就在最新的 tag 上加一位补丁号（v0.19.6 → v0.19.7）。最新 tag：`git fetch --tags && git describe --tags --abbrev=0`。
- 看改了什么：`git log --format='%h %s' <上个版本>..HEAD | grep -v ' Merge '`。必要时用 `git show -s <hash>` 读提交说明。
- 在 `CHANGELOG.md` 最上面加一节 `## vX.Y.Z`：用中文写用户能感知的变化，每条一句或几句，重要的加粗开头；纯内部的改动（测试、重构）和 iOS（不在发版产物里）不写。写法照前几节。
- 本地预览这一节：`scripts/changelog-section.sh vX.Y.Z`。

## 3. 提交并推送

```bash
echo vX.Y.Z > VERSION
git add CHANGELOG.md VERSION
git commit -m "Release vX.Y.Z"
git pull --rebase origin main   # 有新提交进来时，看看是什么（git log）：只是 ios/ 或文档就继续；有代码改动就回到第 1 步重新检查
git push origin HEAD:main
git rev-parse HEAD              # 记下这个 sha
```

`VERSION` 和 `CHANGELOG.md` 必须在同一个提交里：发版流水线会把这一节当作 GitHub Release 的说明。

## 4. 等 GitHub

每隔一分钟查一次，直到 `ci` 和 `release` 都结束：

```bash
curl -s "https://api.github.com/repos/chenhg5/myco/actions/runs?head_sha=<sha>" \
  | python3 -c 'import sys,json; print(" ".join(f"{r[\"name\"]}={r[\"status\"]}/{r[\"conclusion\"]}" for r in json.load(sys.stdin)["workflow_runs"]))'
```

- 两个都是 `completed/success` 才继续。
- `release` 失败：发版没成功，报告失败的步骤和日志。
- `ci` 失败但 `release` 成功：Release 已经发出去了，找出失败的测试，修好（通常是测试在 Windows 上的问题），推一个修复提交，并在报告里说明。
- 确认 Release 的说明是 CHANGELOG 那一节：`curl -s https://api.github.com/repos/chenhg5/myco/releases/tags/vX.Y.Z`。

## 5. Mac 签名、公证、上传

Mac 上有现成的脚本，签名的细节都在里面，这里只调用：

```bash
myco exec --to mac-exec --cmd "~/code/myco-release.sh start vX.Y.Z <sha>"
# 然后每分钟查一次，直到出现 UPLOADED 或 FAILED（通常 5–15 分钟）：
myco exec --to mac-exec --cmd "~/code/myco-release.sh status"
```

- `mac-exec` 不在线（Mac 睡着了）：告诉维护者把 Mac 唤醒并解锁，等它在线再继续。
- 出现 FAILED，或者公证一直卡在 In Progress 超过 30 分钟：停下来，把日志的最后几行报给维护者。

## 6. 核对

```bash
d=$(mktemp -d) && cd "$d"
for f in myco.dmg myco-app-darwin-universal.zip SHA256SUMS; do
  curl -fsSL -o "$f" "https://github.com/chenhg5/myco/releases/download/vX.Y.Z/$f"
done
grep -E ' (myco.dmg|myco-app-darwin-universal.zip)$' SHA256SUMS | sha256sum -c
```

两行都是 `OK` 才算发完。

## 7. 报告

回复派活的人：版本号、Release 链接、CHANGELOG 的要点、CI 和 Mac 的结果、核对结果。提醒：Hub 所在的服务器要 `sudo myco upgrade` 才能用上新版本的 Hub 功能。

## 规则

- 不打印、不提交任何令牌或密钥；签名身份只在 Mac 上，不写进仓库。
- 只在维护者要求时发版；一次只发一个版本。
- 拿不准就问，不要猜。
