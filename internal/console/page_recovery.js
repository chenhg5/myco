// A locally served, read-only exporter on the retired browser origin.
"use strict";
const zh = new URLSearchParams(location.search).get("myco_lang") === "zh";
const t = (en, cn) => zh ? cn : en;
document.documentElement.lang = zh ? "zh-CN" : "en";
const sources = [
  ["myco-video", "files", t("Video Editor", "视频剪辑")],
  ["myco-photo", "files", t("Photo", "修图")],
  ["myco-motion", "files", t("Motion", "动效")],
  ["myco-reader", "books", t("Reader", "阅读")],
  ["myco-reader", "settings", t("Reader settings", "阅读设置")],
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
sources.forEach(([, , label], index) => select.add(new Option(label, index)));

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
function batch(db, store, after) {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, "readonly");
    const request = tx.objectStore(store).openCursor(after === undefined ? null : IDBKeyRange.lowerBound(after, true));
    const rows = [];
    let last;
    let hasMore = false;
    request.onsuccess = () => {
      const row = request.result;
      if (!row) return;
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
async function read(append) {
  const run = ++generation;
  const [name, store] = sources[Number(select.value)];
  if (!append) { list.replaceChildren(); cursor = undefined; more.hidden = true; }
  let db;
  try {
    db = await openExisting(name);
    if (generation !== run) return;
    if (!db?.objectStoreNames.contains(store)) {
      status.textContent = t("No old data for this app.", "这个应用没有旧数据。");
      return;
    }
    const result = await batch(db, store, cursor);
    if (generation !== run) return;
    cursor = result.last;
    more.hidden = !result.hasMore;
    status.textContent = result.rows.length ? "" : t("No old data for this app.", "这个应用没有旧数据。");
    for (const row of result.rows) {
      const item = document.createElement("li");
      const save = document.createElement("button");
      save.type = "button";
      save.textContent = t("Save copy", "保存副本");
      item.append(document.createTextNode(row.name + " "), save);
      save.onclick = async () => {
        save.disabled = true;
        let copyDB;
        try {
          copyDB = await openExisting(name);
          const value = await get(copyDB, store, row.key);
          if (value === undefined) throw new Error("missing entry");
          const file = value instanceof Blob ? value : value?.file;
          if (file instanceof Blob) download(file, row.name);
          else download(new Blob([JSON.stringify({ key: row.key, value })], { type: "application/json" }), "myco-reader-settings.json");
        } catch { unavailable(); }
        finally { copyDB?.close(); save.disabled = false; }
      };
      list.append(item);
    }
  } catch { if (generation === run) unavailable(); }
  finally { db?.close(); }
}
select.onchange = () => void read(false);
more.onclick = () => void read(true);
void read(false);
