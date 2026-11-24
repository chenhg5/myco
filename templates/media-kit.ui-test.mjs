// Exercise the built template, the real iframe bridge and the desktop file
// dispatcher. The server below supplies fixture data and project file routes.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(path.join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const web = createRequire(new URL("../web/package.json", import.meta.url));
const { build } = web("esbuild");
const source = fileURLToPath(new URL("../web/src/", import.meta.url));
const { outputFiles } = await build({
  entryPoints: [path.join(source, "app/page-project-files.ts")],
  alias: { "@": source }, bundle: true, format: "esm", write: false,
});
const bridge = readFileSync(new URL("../internal/console/pagebridge.js", import.meta.url), "utf8");
const root = new URL("../internal/templates/official/media-kit/", import.meta.url);
const files = new Map();
const rows = new Map();
let lastDownload;
let downloaded;
let requests = 0;
const json = (res, data) => res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(data));
const host = `
  import { answerProjectFiles, projectFileScope } from "/dispatcher.js";
  const frame = document.querySelector("iframe");
  const params = new URLSearchParams(location.search);
  const page = { id: "pg_test", project: "pilot", template: { id: "tp_myco_media-kit" } };
  window.scopeTest = async () => {
    const cases = [
      [{ ...page, project: undefined }, "mk-pg_test-entry-valid000.png"],
      [{ ...page, template: { id: "tp_other" } }, "mk-pg_test-entry-valid000.png"],
      [page, "mk-pg_another-entry-valid000.png"],
      [page, "mk-pg_test-../valid000.png"],
      [page, "readme.md"],
    ];
    const denied = answerProjectFiles("hub", { ...page, read_only: true }, "projectFiles.put", {
      name: "mk-pg_test-entry-valid000.png", blob: new Blob(["bytes"], { type: "image/png" }),
    }).then(() => "accepted", error => error.code);
    return Promise.all([denied, ...cases.map(async ([value, name]) => {
      try { await answerProjectFiles("hub", value, "projectFiles.get", { name }); }
      catch (error) { return error.code; }
      return "accepted";
    })]);
  };
  addEventListener("message", async e => {
    const { id, method, args, myco } = e.data || {};
    if (e.source !== frame.contentWindow || e.origin !== location.origin) return;
    if (myco === "open-url") { window.openedURL = e.data.url; return; }
    if (myco !== "call") return;
    const reply = payload => e.source.postMessage({ myco: "result", id, ...payload }, e.origin);
    try {
      if (method === "ready") return reply({ value: {
        page, me: { name: "tester", can_edit: !params.has("readonly") }, projectFiles: !params.has("legacy") && Boolean(projectFileScope(page)),
        projectMedia: !params.has("legacy") && !params.has("old-files") && Boolean(projectFileScope(page)),
        preferences: { theme: params.has("dark") ? "dark" : "light", language: params.has("zh") ? "zh" : "en" },
      } });
      if (method === "data.watch") return reply({ value: true });
      if (method.startsWith("data.")) {
        const response = await fetch("/records", { method: "POST", body: JSON.stringify({ method, args }) });
        return reply(await response.json());
      }
      const value = await answerProjectFiles("hub", page, method, args);
      reply({ value });
    } catch (error) { reply({ error: error.message, code: error.code }); }
  });
  frame.src = "/template/";
`;
const server = createServer(async (req, res) => {
  const route = new URL(req.url, "http://test").pathname;
  if (route === "/") return res.writeHead(200, { "content-type": "text/html" }).end(
    '<style>html,body,iframe{height:100%;width:100%;margin:0;border:0;display:block}</style><iframe allow="clipboard-write"></iframe><script type="module" src="/host.js"></script>',
  );
  if (route === "/host.js" || route === "/dispatcher.js") return res.writeHead(200, { "content-type": "text/javascript" }).end(route === "/host.js" ? host : outputFiles[0].text);
  if (route.startsWith("/template/")) {
    const name = route.slice(10) || "index.html";
    if (name === "bridge.js") return res.writeHead(200, { "content-type": "text/javascript" }).end(bridge);
    try {
      const body = readFileSync(new URL(name, root));
      if (name === "index.html") return res.writeHead(200, {
        "content-type": "text/html",
        "content-security-policy": "default-src 'none'; script-src 'self'; style-src 'self'; img-src blob: data:; media-src blob:",
      }).end(
        body.toString().replace("<head>", '<head><script src="./bridge.js"></script>'));
      const type = name.endsWith(".js") ? "text/javascript" : "text/css";
      return res.writeHead(200, { "content-type": type }).end(body);
    } catch { return res.writeHead(404).end(); }
  }
  if (route === "/app/hubs/hub/projects/pilot") {
    return json(res, { docs: Array.from({ length: 91 + files.size }, () => ({})) });
  }
  const fileRoute = /^\/app\/hubs\/hub\/projects\/pilot\/files\/([^/]+)(\/save)?$/.exec(route);
  if (fileRoute) {
    requests++;
    const [, filename, save] = fileRoute;
    if (save) {
      const file = files.get(filename);
      if (!file) return res.writeHead(404).end();
      lastDownload = filename;
      downloaded = file.bytes;
      return json(res, { path: "/Downloads/" + filename });
    }
    if (req.method === "PUT") {
      const chunks = [];
      for await (const chunk of req) chunks.push(chunk);
      files.set(filename, { bytes: Buffer.concat(chunks), type: req.headers["content-type"] });
      return json(res, { name: filename });
    }
    const file = files.get(filename);
    return file ? res.writeHead(200, { "content-type": file.type }).end(file.bytes) : res.writeHead(404).end();
  }
  if (route === "/records") {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const { method, args } = JSON.parse(Buffer.concat(chunks));
    if (method === "data.list") return json(res, { value: { records: [...rows.values()].filter(row => row.collection === args.collection), seq: 0 } });
    const key = args.collection + "/" + args.id;
    const version = rows.get(key)?.version || 0;
    if (args.ifVersion !== version) return json(res, { error: "Record changed", code: "conflict" });
    const row = { id: args.id, collection: args.collection, data: args.data, version: version + 1 };
    rows.set(key, row);
    return json(res, { value: row });
  }
  res.writeHead(404).end();
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
try {
  const base = `http://127.0.0.1:${server.address().port}`;
  const page = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  page.setDefaultTimeout(10000);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(base);
  const content = page.frameLocator("iframe");
  await content.getByText("Nearly full", { exact: false }).waitFor();
  assert.deepEqual(await page.evaluate(() => window.scopeTest()), ["permission_denied", "permission_denied", "permission_denied", "invalid_argument", "invalid_argument", "invalid_argument"]);
  assert.equal(requests, 0, "forbidden requests must stop at the desktop scope");

  await content.getByRole("button", { name: "New entry", exact: true }).click();
  await content.getByLabel("Title").fill("Usage reporting");
  await content.getByLabel("Labels").fill("App, CLI");
  await content.getByLabel("English tagline").fill("Track tokens by agent.");
  await content.getByLabel("English caption").fill("Share usage totals across the workspace.");
  const png = await page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 200; canvas.height = 120;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#f06741"; ctx.fillRect(0, 0, 200, 120);
    return canvas.toDataURL("image/png").split(",")[1];
  });
  const sourcePNG = Buffer.from(png, "base64");
  // PNG decoders stop at IEND. The trailing bytes test the actual original
  // upload size and byte preservation without committing a huge fixture.
  const original = Buffer.concat([sourcePNG, Buffer.alloc(11 * 1024 * 1024)]);
  await content.locator('input[type="file"]').setInputFiles([
    { name: "usage.png", mimeType: "image/png", buffer: original },
    { name: "alt.png", mimeType: "image/png", buffer: sourcePNG },
  ]);
  await content.getByLabel("Media language").selectOption("en");
  await content.getByLabel("Media theme").selectOption("dark");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText("Uploaded 2 files").waitFor();
  const [first, second] = [...rows.values()].filter(row => row.collection === "images");
  assert.equal(first.data.width, 200);
  assert.equal(first.data.height, 120);
  assert.equal(first.data.language, "en");
  assert.match(first.data.file, /^mk-pg_test-entry_[a-z0-9]+-[a-z0-9]{20}\.png$/);
  assert.ok(files.get(first.data.file).bytes.equals(original));
  assert.equal(first.data.kind, "image");
  // Exercise records made before the kind field existed.
  delete first.data.kind;
  await content.getByRole("button", { name: "Next asset" }).click();
  await content.getByText("2 / 2", { exact: true }).waitFor();
  await content.getByRole("button", { name: "Copy text" }).last().click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), "Share usage totals across the workspace.");
  await content.getByRole("button", { name: "Copy image" }).click();
  await content.getByText("Image copied", { exact: true }).waitFor();
  assert.ok(await page.evaluate(async () => (await navigator.clipboard.read())[0].types.includes("image/png")));
  await content.getByRole("button", { name: "Download original" }).click();
  await content.getByText("Downloaded", { exact: true }).waitFor();
  assert.equal(lastDownload, second.data.file);
  await content.getByRole("button", { name: "Edit media details" }).click();
  await content.getByLabel("Media note").fill("Dark layout");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText(/Dark layout/).waitFor();
  await content.getByRole("button", { name: "Edit or add files" }).click();
  await content.getByLabel("Status").selectOption("published");
  await content.getByLabel("Published to").fill("Community group");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText("Community group", { exact: true }).waitFor();
  await content.getByRole("button", { name: "Back", exact: true }).click();
  await content.getByLabel("Search assets").fill("missing");
  await content.getByText("No entries match these filters.").waitFor();
  await content.getByLabel("Search assets").fill("Track tokens");
  await content.getByLabel("Labels", { exact: true }).selectOption("CLI");
  await content.getByLabel("Status", { exact: true }).selectOption("published");
  await content.getByRole("button", { name: /Usage reporting/ }).waitFor();
  await content.getByLabel("Search assets").fill("");
  await content.getByLabel("Labels", { exact: true }).selectOption("");
  await content.getByLabel("Status", { exact: true }).selectOption("");
  await content.getByRole("button", { name: "New entry", exact: true }).click();
  await content.getByLabel("Title").fill("Launch video");
  await content.getByRole("button", { name: "Add video link" }).click();
  await content.getByLabel("Video URL").fill("javascript:alert(1)");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByRole("alert").getByText("Enter a valid HTTP or HTTPS URL.").waitFor();
  assert.ok(![...rows.values()].some(row => row.data.title === "Launch video"));
  await content.getByLabel("Video URL").fill("https://example.com/demo");
  await content.getByLabel("Link description").fill("Launch walkthrough");
  await content.getByRole("button", { name: "Add video link" }).click();
  await content.getByLabel("Video URL").last().fill("http://example.com/replay");
  await content.getByLabel("Link description").last().fill("Full replay");

  const makeMedia = (format, args) => execFileSync("ffmpeg", [
    "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12",
    "-t", "2", "-threads", "2", ...args, "-f", format, "pipe:1",
  ]);
  const mp4 = makeMedia("mp4", ["-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "frag_keyframe+empty_moov"]);
  // Give a valid MP4 a 15 MiB representation without a large fixture in Git.
  const padding = Buffer.alloc(15 * 1024 * 1024 - mp4.length);
  padding.writeUInt32BE(padding.length);
  padding.write("free", 4);
  const originalVideo = Buffer.concat([mp4, padding]);
  const gif = makeMedia("gif", []);
  const webm = makeMedia("webm", ["-c:v", "libvpx-vp9"]);
  await content.locator('input[type="file"]').setInputFiles([
    { name: "launch.mp4", mimeType: "video/mp4", buffer: originalVideo },
    { name: "launch.gif", mimeType: "image/gif", buffer: gif },
    { name: "launch.webm", mimeType: "video/webm", buffer: webm },
  ]);
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText("Uploaded 3 files").waitFor();
  const shot = content.locator("video");
  await shot.evaluate(async video => {
    await video.play();
    if (video.currentTime < 0.2) await new Promise(resolve => {
      const moved = () => { if (video.currentTime >= 0.2) { video.removeEventListener("timeupdate", moved); resolve(); } };
      video.addEventListener("timeupdate", moved);
    });
    video.pause();
  });
  assert.equal(await shot.evaluate(video => video.videoWidth), 320);
  assert.equal(await content.getByRole("button", { name: "Copy image" }).count(), 0);
  const launch = [...rows.values()].find(row => row.data.title === "Launch video");
  assert.deepEqual(launch.data.video_links, [
    { url: "https://example.com/demo", label: "Launch walkthrough" },
    { url: "http://example.com/replay", label: "Full replay" },
  ]);
  const launchFiles = [...rows.values()].filter(row => row.data.entry === launch.id);
  assert.deepEqual(launchFiles.map(row => [row.data.kind, row.data.bytes]), [["video", originalVideo.length], ["image", gif.length], ["video", webm.length]]);
  assert.ok(launchFiles.every(row => row.data.preview.startsWith("data:image/jpeg;base64,")));
  await content.getByRole("button", { name: "Download original file" }).click();
  await content.getByText("Downloaded", { exact: true }).waitFor();
  assert.equal(lastDownload, launchFiles[0].data.file);
  assert.ok(downloaded.equals(originalVideo));
  const link = content.getByRole("link", { name: "Launch walkthrough" });
  assert.equal(await link.getAttribute("target"), "_blank");
  await link.click();
  await page.waitForFunction(() => window.openedURL === "https://example.com/demo");
  await content.getByRole("button", { name: "Copy link" }).first().click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), "https://example.com/demo");

  await content.getByRole("button", { name: "Next asset" }).click();
  await content.locator('.mk-photo img[src^="blob:"]').evaluate(image => image.decode());
  assert.equal(await content.locator(".mk-photo video").count(), 0);
  assert.ok(await content.getByRole("button", { name: "Copy image" }).isEnabled());
  await content.getByRole("button", { name: "Next asset" }).click();
  await content.locator("video").evaluate(async video => { await video.play(); video.pause(); });
  await content.getByRole("button", { name: "Back", exact: true }).click();
  const card = content.getByRole("button", { name: /Launch video/ });
  assert.match(await card.locator("img").getAttribute("src"), /^data:image\/jpeg/);
  assert.ok(await card.locator(".mk-play-mark").isVisible());

  // Agent-authored values pass the same URL checks before any link is rendered.
  launch.data.video_links.push({ url: "data:text/html,unsafe", label: "Unsafe link" });
  const shots = process.env.MEDIA_KIT_SHOTS;
  if (shots) mkdirSync(shots, { recursive: true });
  for (const width of [400, 1200]) {
    for (const theme of ["light", "dark"]) {
      for (const locale of ["en", "zh"]) {
        const view = await browser.newPage({ viewport: { width, height: 800 }, colorScheme: theme });
        const query = [theme, locale].join("&");
        await view.goto(base + "?" + query);
        const frame = view.frameLocator("iframe");
        await frame.getByRole("button", { name: /Launch video/ }).click();
        assert.equal(await frame.getByRole("link", { name: "Unsafe link" }).count(), 0);
        assert.ok(await frame.getByRole("heading", { name: locale === "zh" ? "视频链接" : "Video links" }).isVisible());
        assert.equal(await frame.locator(".mk-app").evaluate(el => el.scrollWidth), width);
        const video = frame.locator("video");
        await video.evaluate(async media => { await media.play(); media.pause(); });
        if (shots && ((width === 400 && theme === "dark" && locale === "en") || (width === 1200 && theme === "light" && locale === "zh"))) {
          await view.screenshot({ path: path.join(shots, `media-kit-${width}-${theme}-${locale}.png`) });
        }
        await view.close();
      }
    }
  }
  const old = await browser.newPage();
  await old.goto(base + "?old-files");
  const oldContent = old.frameLocator("iframe");
  await oldContent.getByRole("button", { name: /Launch video/ }).click();
  await oldContent.getByText("Update the desktop app to open and download this file.").waitFor();
  assert.ok(await oldContent.getByRole("button", { name: "Download original file" }).isDisabled());
  assert.ok(await oldContent.getByRole("link", { name: "Launch walkthrough" }).isVisible());
  await old.close();
  assert.deepEqual(errors, []);

  const narrow = await browser.newPage({ viewport: { width: 400, height: 780 }, colorScheme: "dark" });
  narrow.setDefaultTimeout(10000);
  await narrow.goto(base + "?legacy&dark");
  const fallback = narrow.frameLocator("iframe");
  await fallback.getByText(/Open this project page in an updated desktop app/).waitFor();
  await fallback.getByRole("button", { name: /Usage reporting/ }).click();
  assert.ok(await fallback.getByRole("button", { name: "Download original" }).isDisabled());
  const dimensions = await fallback.locator(".mk-app").evaluate(el => [el.scrollWidth, el.clientWidth]);
  assert.equal(dimensions[0], dimensions[1]);
  await narrow.goto(base + "?readonly&dark");
  await fallback.getByText("View only", { exact: true }).waitFor();
  assert.equal(await fallback.getByRole("button", { name: "New entry" }).count(), 0);
  await fallback.getByRole("button", { name: /Usage reporting/ }).click();
  assert.equal(await fallback.getByRole("button", { name: "Edit or add files" }).count(), 0);
  assert.ok(await fallback.getByRole("button", { name: "Download original" }).isEnabled());
  console.log("Media Kit: scoped files, 11 MiB PNG, 15 MiB MP4 playback and byte-for-byte download, WebM, GIF, validated links, legacy bridge and bilingual responsive views passed");
} finally {
  await browser.close();
  server.close();
}
