#!/usr/bin/env node
// Local, isolated mock only. See media-kit/README.md; add scenes in scenes.mjs.
import { spawn, execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { createRequire } from "node:module";
import { readFile, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { localizeFixtures } from "./media-kit/fixture-i18n.mjs";
import { scenes } from "./media-kit/scenes.mjs";
const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const args = process.argv.slice(2),
  option = (k, d) => (args.includes(k) ? args[args.indexOf(k) + 1] : d);
const out = path.resolve(
  option("--out", path.join(root, "../media-kit-output")),
);
process.chdir(path.join(root, "web"));
const only = option("--scene", "").split(",").filter(Boolean),
  langs = option("--lang", "zh,en").split(",");
for (const id of only)
  if (!scenes.some((s) => s.id === id)) throw Error("Unknown scene: " + id);
const require = createRequire(
  path.join(
    execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
    "noop.js",
  ),
);
const { chromium } = require("playwright");
const { createServer: createVite } = await import(
  "../node_modules/vite/dist/node/index.js"
);
const port = async () => {
  const s = createServer();
  await new Promise((r) => s.listen(0, "127.0.0.1", r));
  const p = s.address().port;
  await new Promise((r) => s.close(r));
  return p;
};
const report = {
  videos: [],
  complete: false,
  source: execFileSync("git", ["rev-parse", "HEAD"], {
    cwd: root,
    encoding: "utf8",
  }).trim(),
  mock: true,
  scale: 2,
  shots: [],
  skipped: [],
  errors: [],
};
report.expected = scenes.reduce(
  (n, s, i) =>
    n +
    (s.skip || (only.length && !only.includes(s.id))
      ? 0
      : (s.themes?.length || (i < 6 ? 2 : 1)) * langs.length),
  0,
);
if (args.includes("--resume")) {
  const previous = JSON.parse(
    await readFile(path.join(out, "manifest.json"), "utf8"),
  );
  if (previous.source !== report.source)
    throw Error("Cannot mix screenshots from different source commits.");
  Object.assign(report, previous, {
    complete: false,
    expected:
      previous.expected || previous.shots.length + previous.errors.length,
  });
  const selected = (x) =>
    (!only.length || only.includes(x.id)) && langs.includes(x.lang);
  report.shots = report.shots.filter((x) => !selected(x));
  report.errors = report.errors.filter((x) => !selected(x));
}
await mkdir(out, { recursive: true });
const browser = await chromium.launch(process.env.UI_SMOKE_EXECUTABLE ? { executablePath: process.env.UI_SMOKE_EXECUTABLE } : {});
process.on("beforeExit", () => {
  if (!report.complete) process.exitCode = 1;
});
try {
  for (const lang of langs) {
    if (!["zh", "en"].includes(lang)) throw Error("language must be zh or en");
    const mockPort = await port();
    const env = { ...process.env };
    for (const k of Object.keys(env))
      if (/^(MYCO_|CLAUDE|ANTHROPIC|OPENAI|MOCK_)/.test(k)) delete env[k];
    Object.assign(env, {
      APP_MOCK_PORT: String(mockPort),
      MOCK_SHOWCASE: lang,
      MOCK_SHELL: "mac",
      MOCK_MEDIA_KIT: "1",
    });
    const mock = spawn(process.execPath, ["mock/app-server.mjs"], {
      cwd: path.join(root, "web"),
      env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let log = "";
    mock.stdout.on("data", (b) => (log += b));
    mock.stderr.on("data", (b) => (log += b));
    let vite;
    try {
      let ready = false;
      for (let i = 0; i < 100; i++) {
        try {
          ready = (await fetch(`http://127.0.0.1:${mockPort}/app/info`)).ok;
          if (ready) break;
        } catch {}
        await new Promise((r) => setTimeout(r, 100));
      }
      if (!ready) throw Error("mock did not start: " + log);
      vite = await createVite({
        root: path.join(root, "web"),
        cacheDir: path.join(out, ".vite-" + lang),
        configFile: path.join(root, "web/vite.config.ts"),
        server: {
          host: "127.0.0.1",
          port: await port(),
          strictPort: true,
          proxy: {
            "/app": `http://127.0.0.1:${mockPort}`,
            "/api": `http://127.0.0.1:${mockPort}`,
          },
        },
        logLevel: "error",
      });
      await vite.listen();
      const url = `http://127.0.0.1:${vite.config.server.port}`;
      for (const [index, scene] of scenes.entries()) {
        if (only.length && !only.includes(scene.id)) continue;
        if (scene.skip) {
          report.skipped.push({ id: scene.id, reason: scene.skip });
          continue;
        }
        for (const theme of scene.themes || (index < 6 ? ["light", "dark"] : ["light"])) {
          const dir = path.join(out, scene.id);
          await mkdir(dir, { recursive: true });
          const record =
            args.includes("--video") && scene.video && theme === "light";
          const context = await browser.newContext({
            viewport: { width: 1440, height: 1000 },
            deviceScaleFactor: 2,
            colorScheme: theme,
            locale: lang === "zh" ? "zh-CN" : "en-US",
            ...(record
              ? {
                  recordVideo: {
                    dir: path.join(dir, "video"),
                    size: { width: 1440, height: 1000 },
                  },
                }
              : {}),
          });
          // Never send browser requests to a real Hub, model provider or third party.
          await context.route("**/*", (route) => {
            const u = new URL(route.request().url());
            return ["127.0.0.1", "localhost"].includes(u.hostname) ||
              ["data:", "blob:"].includes(u.protocol)
              ? route.continue()
              : route.abort();
          });
          await context.addInitScript(
            ({ lang, theme }) => {
              if (window !== window.top) return;
              localStorage.setItem("myco-theme", theme);
              localStorage.setItem("myco-lang", lang);
            },
            { lang, theme },
          );
          if (lang === "en") await localizeFixtures(context);
          const page = await context.newPage();
          page.setDefaultTimeout(20000);
          page.setDefaultNavigationTimeout(60000);
          const errors = [];
          page.on("pageerror", (e) => errors.push(e.message));
          const T = (zh, en) => (lang === "zh" ? zh : en);
          const api = async (p, method = "GET", body) => {
            const r = await fetch(`http://127.0.0.1:${mockPort}${p}`, {
              method,
              headers: { "Content-Type": "application/json" },
              ...(body ? { body: JSON.stringify(body) } : {}),
            });
            if (!r.ok) throw Error(`${method} ${p}: ${r.status}`);
            return r.status === 204 ? null : r.json();
          };
          try {
            await page.goto(url + `/?lang=${lang}`, {
              waitUntil: "domcontentloaded",
            });
            await page.locator(".app-shell").waitFor();
            await page.waitForTimeout(1100);
            await page
              .getByText(lang === "zh" ? "官网改版" : "Website redesign", {
                exact: true,
              })
              .first()
              .click();
            await page.waitForTimeout(400);
            console.log(`BEGIN ${scene.id} ${lang}/${theme}`);
            await scene.prepare({
              page,
              context,
              api,
              lang,
              theme,
              T,
              root,
              url,
              mockPort,
            });
            await page.evaluate(() => document.fonts.ready);
            await page.waitForTimeout(600);
            if (errors.length) throw Error(errors.join("\n"));
            const visibleText = (
              await Promise.all(
                page.frames().map((f) =>
                  f
                    .locator("body")
                    .innerText()
                    .catch(() => ""),
                ),
              )
            ).join("\n");
            await writeFile(
              path.join(dir, `${lang}-${theme}.text.txt`),
              visibleText,
            );
            if (lang === "en" && /\p{Script=Han}/u.test(visibleText))
              throw Error(
                "Untranslated fixture copy; inspect .text.txt before publishing.",
              );
            const file = path.join(dir, `${lang}-${theme}.png`);
            await page.screenshot({ path: file });
            report.shots.push({
              id: scene.id,
              lang,
              theme,
              file,
              width: 2880,
              height: 2000,
              title: scene.title,
              pitch_zh: scene.zh,
              pitch_en: scene.en,
              tags: ["App", ...scene.tags],
              boundary: scene.boundary || "Mock demonstration only.",
              description: scene.description,
            });
            console.log(`PASS ${scene.id} ${lang}/${theme}`);
            if (record) {
              await scene.video({ page, T, api, lang });
              await page.waitForTimeout(10000);
            }
          } catch (e) {
            await writeFile(
              path.join(dir, `${lang}-${theme}.debug.txt`),
              (
                await Promise.all(
                  page.frames().map(
                    async (f) =>
                      f.url() +
                      "\n" +
                      (await f
                        .locator("body")
                        .innerText()
                        .catch(() => "")),
                  ),
                )
              ).join("\n---FRAME---\n"),
            );
            report.errors.push({ id: scene.id, lang, theme, error: e.message });
            await page.screenshot({
              path: path.join(dir, `${lang}-${theme}.FAILED.png`),
            });
            console.error(`FAIL ${scene.id} ${lang}/${theme}: ${e.message}`);
          } finally {
            if (scene.id === "permissions")
              await api("/app/mock/superagent-ask", "DELETE");
            const video = page.video();
            await page.keyboard.up("Control").catch(() => {});
            await context.close();
            if (
              record &&
              video &&
              !report.errors.some(
                (e) =>
                  e.id === scene.id && e.lang === lang && e.theme === theme,
              )
            ) {
              const raw = await video.path();
              const file = path.join(dir, `${lang}-demo.webm`);
              execFileSync(
                "ffmpeg",
                [
                  "-y",
                  "-sseof",
                  `-${scene.videoSeconds || 18}`,
                  "-i",
                  raw,
                  "-t",
                  String(scene.videoSeconds || 18),
                  "-vf",
                  "fps=15",
                  "-c:v",
                  "libvpx-vp9",
                  "-deadline",
                  "realtime",
                  "-cpu-used",
                  "8",
                  "-threads",
                  "2",
                  "-crf",
                  "36",
                  "-b:v",
                  "0",
                  file,
                ],
                { stdio: "ignore" },
              );
              report.videos.push({ id: scene.id, lang, file });
            }
          }
        }
      }
    } finally {
      const stopped =
        mock.exitCode !== null
          ? Promise.resolve()
          : new Promise((r) => mock.once("exit", r));
      mock.kill("SIGTERM");
      await Promise.race([stopped, new Promise((r) => setTimeout(r, 2000))]);
      await Promise.race([
        vite?.close(),
        new Promise((r) => setTimeout(r, 2000)),
      ]);
    }
  }
} finally {
  report.complete =
    report.shots.length === report.expected && report.errors.length === 0;
  await browser.close();
  await writeFile(
    path.join(out, "manifest.json"),
    JSON.stringify(report, null, 2),
  );
}
if (report.errors.length) process.exitCode = 1;
