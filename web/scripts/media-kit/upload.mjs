#!/usr/bin/env node
// Explicit publication step. Screenshots never upload while being generated.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile, mkdir, stat } from "node:fs/promises";
import path from "node:path";
const args = process.argv.slice(2),
  value = (key, fallback) =>
    args.includes(key) ? args[args.indexOf(key) + 1] : fallback;
const out = path.resolve(value("--out", "../media-kit-output"));
const page = value("--page");
if (!page || page.startsWith("--")) {
  console.error("Usage: upload.mjs --page <page-id> [--out <directory>] [--upload] [--metadata-only]");
  process.exit(2);
}
const live = args.includes("--upload");
const metadataOnly = args.includes("--metadata-only");
const descriptions = {
  teamwork: "项目主页，协调者对话、成员状态和跨电脑团队",
  progress: "任务对话，进展、工具调用和执行步骤",
  terminal: "项目旁终端，测试命令和输出",
  coedit: "对话与任务追踪并排，问题状态已更新",
  workflow: "工作流详情，运行列表及成功、失败和等待状态",
  automation: "自动化列表，定时摘要、CI排查和新issue触发",
  permissions: "对话中的限时查看授权卡，目标电脑和请求原因",
  usage: "用量页，总量、每日柱状图和成员排行",
  sessions: "添加Agent对话框，本机找到的会话",
  focus: "分屏对话，协调者和成员的对话并排显示",
  rich: "发布检查卡，测试指标和操作按钮",
  providers: "成员设置，模型、推理强度和服务商",
  design: "白板画布，问题、方案和验证节点及连线",
  motion: "动效视频编辑器，场景预览和分镜列表",
  disk: "空间清理，磁盘概况、空间图和目录列表",
  servers: "服务器概览，CPU、内存、磁盘和系统信息",
};
const cli = (...args) =>
  JSON.parse(
    execFileSync("myco", args, {
      encoding: "utf8",
      maxBuffer: 20 * 1024 * 1024,
    }),
  );
const manifest = JSON.parse(
  await readFile(path.join(out, "manifest.json"), "utf8"),
);
if (!manifest.complete || manifest.errors.length)
  throw Error("Refusing to upload an incomplete run. Fix scene errors first.");
const groups = Map.groupBy
  ? Map.groupBy(manifest.shots, (s) => s.id)
  : manifest.shots.reduce(
      (m, s) => (m.set(s.id, [...(m.get(s.id) || []), s]), m),
      new Map(),
    );
const existing = cli(
  "page",
  "data",
  "list",
  "--page",
  page,
  "--collection",
  "entries",
).records;
const pictures = cli(
  "page",
  "data",
  "list",
  "--page",
  page,
  "--collection",
  "images",
).records;
const pageInfo = cli("page", "get", "--page", page);
if (pageInfo.template?.data?.images?.fields?.attachment !== "file")
  throw Error("Upgrade this Media Kit page before uploading page attachments.");
const stored = [];
let cursor = "", quota;
do {
  const result = cli("page", "file", "list", "--page", page, "--limit", "1000", ...(cursor ? ["--after", cursor] : []));
  if (result.attachments_version !== 1) throw Error("Update the CLI and Hub for page attachments; project uploads are not supported.");
  stored.push(...result.attachments); quota = result.usage; cursor = result.next || "";
} while (cursor);
const before = quota.files_used, filesMax = quota.files_max;
const priorPicture = p => pictures.find(r => r.id === "img_" + p.suffix || r.data.attachment?.name === p.name || r.data.file === p.name);
const priorAttachment = p => {
  const picture = priorPicture(p);
  return stored.find(f => picture?.data.attachment?.file_id === f.file_id || f.name === p.name || (picture?.data.file && f.legacy_name === picture.data.file));
};
const reusable = p => {
  const f = priorAttachment(p);
  return f && f.sha256 === p.sha256 && f.size === p.bytes && (!f.expires_at || Date.parse(f.expires_at) > Date.now() || f.state === "ready") ? f : undefined;
};
const planned = [];
for (const [id, shots] of groups) {
  const prior = existing.find((r) => r.id === "feature-" + id);
  if (prior && prior.data.status !== "draft")
    throw Error("Refusing to overwrite a reviewed entry: " + prior.id);
  const entry = "feature-" + id;
  for (const s of shots) {
    const suffix = ("scene" + id + s.lang + s.theme).replace(/[^a-z0-9]/g, "");
    const name = `mk-${page}-${entry}-${suffix}.png`;
    const meta = await stat(s.file);
    const png = await readFile(s.file);
    if (
      png.subarray(0, 8).toString("hex") !== "89504e470d0a1a0a" ||
      png.readUInt32BE(16) !== s.width ||
      png.readUInt32BE(20) !== s.height ||
      s.width !== 2880 ||
      s.height !== 2000
    )
      throw Error(
        "Screenshot dimensions do not match the 2× manifest: " + s.file,
      );
    if (meta.size > 20 * 1024 * 1024) throw Error("File too large: " + s.file);
    planned.push({ s, entry, name, suffix, bytes: meta.size, sha256: createHash("sha256").update(png).digest("hex") });
  }
}
if (planned.some(p => priorPicture(p)?.data.file && !priorAttachment(p)))
  throw Error("A legacy original has not migrated yet. Wait for migration or inspect its report before replacing it.");
if (metadataOnly && planned.some(p => !priorPicture(p) || !reusable(p)))
  throw Error("Metadata-only mode requires matching originals already stored as page attachments.");
const uploads = metadataOnly ? [] : planned.filter(p => !reusable(p));
const newFiles = uploads.length;
const addedBytes = uploads.reduce((n,p)=>n+p.bytes,0);
if (newFiles && before + newFiles > filesMax)
  throw Error(`Page file quota exceeded: ${before}+${newFiles}>${filesMax}. Remove unused attachments or ask the Hub administrator to adjust defaults.`);
if (newFiles && quota.bytes_used + addedBytes > quota.bytes_max)
  throw Error(`Page byte quota exceeded: ${quota.bytes_used}+${addedBytes}>${quota.bytes_max}. Remove unused attachments or ask the Hub administrator to adjust defaults.`);
console.log(
  JSON.stringify(
    {
      dryRun: !live,
      entries: groups.size,
      files: planned.length,
      pageFilesBefore: before,
      newFiles,
      pageFilesAfter: before + newFiles,
      pageFilesMax: filesMax,
      pageBytesAfter: quota.bytes_used + addedBytes, pageBytesMax: quota.bytes_max,
    },
    null,
    2,
  ),
);
if (!live) process.exit(0);
await mkdir(path.join(out, "records"), { recursive: true });
for (const [id, shots] of groups) {
  const entry = "feature-" + id,
    s = shots[0],
    old = existing.find((r) => r.id === entry);
  const data = {
    ...(old?.data || {}),
    title: s.title,
    date: new Date().toISOString().slice(0, 10),
    pitch_zh: s.pitch_zh,
    pitch_en: s.pitch_en,
    tags: s.tags,
    status: "draft",
    version: manifest.source.slice(0, 8),
    copy_zh: "使用隔离模拟数据展示界面，不代表真实远程执行。",
    copy_en: s.boundary,
  };
  const file = path.join(out, "records", entry + ".json");
  await writeFile(file, JSON.stringify(data));
  cli(
    "page",
    "data",
    "put",
    "--page",
    page,
    "--collection",
    "entries",
    "--id",
    entry,
    "--if-version",
    String(old?.version || 0),
    "--file",
    file,
  );
  for (const p of planned.filter((p) => p.entry === entry)) {
    const oldPic = priorPicture(p);
    const imageId = oldPic?.id || "img_" + p.suffix;
    const priorFile = priorAttachment(p);
    // Stable content keys recover pending uploads without consuming another slot.
    const attachment = reusable(p) || cli("page", "file", "put", "--page", page, "--collection", "images", "--path", p.s.file, "--as", p.name, "--key", "media-kit:" + createHash("sha256").update(page + "\0" + p.name + "\0" + p.sha256).digest("hex"));
    const preview = execFileSync(
      "python3",
      [
        "-c",
        "from PIL import Image; import io,base64,sys; im=Image.open(sys.argv[1]).convert('RGB'); im.thumbnail((360,250)); b=io.BytesIO(); im.save(b,format='JPEG',quality=65); print('data:image/jpeg;base64,'+base64.b64encode(b.getvalue()).decode())",
        p.s.file,
      ],
      { encoding: "utf8" },
    ).trim();
    if (preview.length > 18000) throw Error("Thumbnail too large");
    const image = {
      kind: "screenshot",
      platform: "app",
      aspect: "landscape",
      tags: p.s.tags,
      description: `${p.s.description || descriptions[p.s.id] || p.s.title.split(" / ")[0]}，${p.s.lang === "zh" ? "中文" : "英文"}界面、${p.s.theme === "light" ? "浅色" : "深色"}。模拟数据。`,
      entry,
      attachment,
      file: priorFile?.file_id === attachment.file_id ? oldPic?.data.file || "" : "",
      name: p.s.lang + " · " + p.s.theme,
      language: p.s.lang,
      theme: p.s.theme,
      width: p.s.width,
      height: p.s.height,
      bytes: p.bytes,
      order: (p.s.lang === "zh" ? 0 : 2) + (p.s.theme === "dark" ? 1 : 0),
      caption:
        p.s.lang === "zh" ? "模拟数据 · 2倍截图" : "Demo data · 2× screenshot",
      preview,
    };
    const f = path.join(out, "records", imageId + ".json");
    await writeFile(f, JSON.stringify(image));
    cli(
      "page",
      "data",
      "put",
      "--page",
      page,
      "--collection",
      "images",
      "--id",
      imageId,
      "--if-version",
      String(oldPic?.version || 0),
      "--file",
      f,
    );
    console.log("uploaded " + entry + " " + p.s.lang + "/" + p.s.theme);
  }
}
const after = cli("page", "file", "list", "--page", page, "--limit", "1").usage;
const result = {
  entries: [...groups.keys()].map((id) => "feature-" + id),
  files: planned.length,
  pageFiles: after.files_used,
  pageBytes: after.bytes_used,
};
await writeFile(
  path.join(out, "uploaded.json"),
  JSON.stringify(result, null, 2),
);
console.log(JSON.stringify(result, null, 2));
