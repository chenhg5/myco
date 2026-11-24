// A locally served, read-only exporter on the retired browser origin.
"use strict";
const params = new URLSearchParams(location.search);
const zh = params.get("myco_lang") === "zh";
const t = (en, cn) => zh ? cn : en;
document.documentElement.lang = zh ? "zh-CN" : "en";
const sources = [
  ["tp_myco_video", "myco-video", "files", t("Video Editor", "视频剪辑")],
  ["tp_myco_photo", "myco-photo", "files", t("Photo", "修图")],
  ["tp_myco_motion", "myco-motion", "files", t("Motion", "动效")],
  ["tp_myco_reader", "myco-reader", "books", t("Reader", "阅读")],
  ["tp_myco_reader", "myco-reader", "settings", t("Reader settings", "阅读设置")],
];
const select = document.getElementById("source");
const list = document.getElementById("files");
const status = document.getElementById("status");
const more = document.getElementById("more");
document.getElementById("intro").textContent = t(
  "Select a file to save an original copy, then import it into the app. This leaves older copies intact.",
  "选择文件保存原件，再导入相应应用。旧库里的原件会保留。",
);
document.getElementById("label").textContent = t("Application ", "应用 ");
more.textContent = t("Show more", "显示更多");
let completed = new Set();
let hostOrigin;

const message = (type, payload) => {
  // A desktop shell can use a custom scheme with an opaque origin. The
  // parent window itself is still the one that configured this ticketed frame.
  if (hostOrigin) parent.postMessage({ type, ...payload }, hostOrigin === "null" ? "*" : hostOrigin);
};
const keyOf = (source, key) => JSON.stringify([source[1], source[2], key]);

function openExisting(name) {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(name);
    let missing = false;
    request.onupgradeneeded = () => {
      missing = true;
      request.transaction.abort();
    };
    request.onerror = () => missing ? resolve(null) : reject(request.error);
    request.onsuccess = () => resolve(request.result);
    request.onblocked = () => reject(new Error("database unavailable"));
  });
}

// Browse only a bounded group of records and hold no copies of media bytes.
function batch(db, source, after) {
  const store = source[2];
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, "readonly");
    const request = tx.objectStore(store).openCursor(after === undefined ? null : IDBKeyRange.lowerBound(after, true));
    const rows = [];
    let last;
    let hasMore = false;
    request.onsuccess = () => {
      const row = request.result;
      if (!row) return;
      if (completed.has(keyOf(source, row.key))) { row.continue(); return; }
      if (rows.length === 24) { hasMore = true; return; }
      const value = row.value;
      const file = value instanceof Blob ? value : value?.file;
      rows.push({ key: row.key, name: file?.name || value?.name || String(row.key) });
      last = row.key;
      row.continue();
    };
    tx.oncomplete = () => resolve({ rows, last, hasMore });
    tx.onabort = () => reject(tx.error);
  });
}

function get(db, store, key) {
  return new Promise((resolve, reject) => {
    const request = db.transaction(store, "readonly").objectStore(store).get(key);
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function download(file, name) {
  const anchor = document.createElement("a");
  const address = URL.createObjectURL(file);
  anchor.href = address;
  anchor.download = name || "myco-recovered";
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(address), 30000);
}

let generation = 0;
let cursor;
const unavailable = () => {
  status.textContent = t("Could not read the old storage. Reopen this panel to retry.", "无法读取旧库。请重新打开此面板重试。");
};

async function hasPending(source) {
  const [, name, store] = source;
  let db;
  try {
    db = await openExisting(name);
    if (!db?.objectStoreNames.contains(store)) return false;
    return await new Promise((resolve, reject) => {
      const request = db.transaction(store, "readonly").objectStore(store).openKeyCursor();
      request.onsuccess = () => {
        const row = request.result;
        if (!row) resolve(false);
        else if (!completed.has(keyOf(source, row.primaryKey))) resolve(true);
        else row.continue();
      };
      request.onerror = () => reject(request.error);
    });
  } finally { db?.close(); }
}

async function browse(preferred) {
  const found = await Promise.all(sources.map(source => hasPending(source).catch(() => false)));
  const available = sources.filter((_, index) => found[index]);

  select.replaceChildren();
  for (const source of available) select.add(new Option(source[3], String(sources.indexOf(source))));
  select.parentElement.hidden = available.length === 0;
  const match = available.find(source => source === preferred) || available.find(([id]) => id === preferred) || available[0];
  if (match) select.value = String(sources.indexOf(match));
  await read(false);
}

async function read(append) {
  const run = ++generation;
  const source = select.value === "" ? undefined : sources[Number(select.value)];
  if (!append) { list.replaceChildren(); cursor = undefined; more.hidden = true; }
  if (!source) {
    status.textContent = t("No files left to recover.", "没有待恢复的文件。");
    return;
  }
  const [, name, store] = source;
  let db;
  try {
    db = await openExisting(name);
    if (generation !== run) return;
    const result = db?.objectStoreNames.contains(store) ? await batch(db, source, cursor) : { rows: [] };
    if (generation !== run) return;
    cursor = result.last;
    more.hidden = !result.hasMore;
    status.textContent = "";
    for (const row of result.rows) {
      const item = document.createElement("li");
      const save = document.createElement("button");
      save.type = "button";
      save.textContent = t("Save copy", "保存副本");
      item.append(document.createTextNode(row.name + " "), save);
      save.onclick = async () => {
        save.disabled = true;
        try {
          const { file, name } = await readCopy(source, row.key, row.name);
          download(file, name);
        } catch { unavailable(); }
        finally { save.disabled = false; }
      };
      list.append(item);
    }
  } catch { if (generation === run) unavailable(); }
  finally { db?.close(); }
}
select.onchange = () => void read(false);
more.onclick = () => void read(true);
async function readCopy(source, key, filename) {
  const [, dbName, store] = source;
  const db = await openExisting(dbName);
  try {
    const value = await get(db, store, key);
    if (value === undefined) throw new Error("missing entry");
    const original = value instanceof Blob ? value : value?.file;
    return original instanceof Blob ? { file: original, name: filename } : {
      file: new Blob([JSON.stringify({ key, value })], { type: "application/json" }),
      name: "myco-reader-settings.json",
    };
  } finally { db?.close(); }
}

async function reportSources() {
  const found = await Promise.all(sources.map(source => hasPending(source).catch(() => false)));
  message("myco-recovery-sources", { sources: sources.flatMap(([template, , , label], index) => found[index] ? [{ index, template, label }] : []) });
}

let browseGeneration = 0;
let lastIndex;
let lastKey;
const listed = new Map();
async function reportRows(index, append) {
  const token = ++browseGeneration;
  const source = sources[index];
  let db;
  try {
    db = await openExisting(source[1]);
    const extending = append && lastIndex === index;
    const result = db?.objectStoreNames.contains(source[2]) ? await batch(db, source, extending ? lastKey : undefined) : { rows: [] };
    if (token !== browseGeneration) return;
    if (!extending) listed.clear();
    const entries = result.rows.map(row => {
      const id = keyOf(source, row.key);
      listed.set(id, row);
      return { id, name: row.name };
    });
    lastIndex = index;
    lastKey = result.last;
    message("myco-recovery-rows", { index, append: extending, entries, more: !!result.hasMore });
  } catch { if (token === browseGeneration) message("myco-recovery-error"); }
  finally { db?.close(); }
}

if (parent === window) {
  void browse();
} else {
  addEventListener("message", (event) => {
    if (event.source !== parent || !event.data) return;
    const { type, completed: done, index, append, id } = event.data;
    if (!hostOrigin && type === "myco-recovery-configure") {
      hostOrigin = event.origin;
      completed = new Set(done);
      void reportSources();
    }
    if (event.origin !== hostOrigin || !Number.isInteger(index) || !sources[index]) return;
    if (type === "myco-recovery-list") void reportRows(index, append);
    if (index !== lastIndex || !listed.has(id)) return;
    const row = listed.get(id);
    if (type === "myco-recovery-save") void readCopy(sources[index], row.key, row.name).then(
      copy => message("myco-recovery-file", { id, ...copy }), () => message("myco-recovery-error"),
    );
    if (type === "myco-recovery-complete") {
      completed.add(id);
      listed.delete(id);
      void reportSources();
    }
  });
}
