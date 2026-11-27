import type { LocalPreferences } from "@/api";

/** Only this App profile's durable choices. Never copy all browser storage. */
const keys = new Set([
  "myco-app-hub-looks", "myco-app-hub-names", "myco-theme", "myco-lang", "myco-text-size", "myco-ui-style",
  "myco-nav-hidden", "myco-nav-chat-sort", "myco-conv-pins", "myco-focus-bg", "myco-chat-bg", "myco-simple-mode",
  "myco-island", "myco-runs-reports-only", "myco-translate-auto", "myco-usage-days", "myco-drafts", "myco-queued",
  "myco-onboard", "myco-checklist-off", "myco-intro-seen", "myco-onboard-did", "myco-starters-off", "myco-plan-bar-closed", "myco-page-recovery-completed",
]);
const prefixes = ["myco-usage-project:", "myco-nav-pinned:", "myco-nav-order\0", "myco-hidden-", "myco-sent\0"];
export const isDurablePreference = (key: string) => key.length <= 1024 && (keys.has(key) || prefixes.some((p) => key.startsWith(p) && key.length > p.length));
export const PREFERENCES_CHANGED = "myco-local-preferences-changed";
export const PREFERENCES_ERROR = "myco-local-preferences-error";
let problem = false;
let backgroundPending = false;
export const preferencesProblem = () => problem;
let mode: "browser" | "app" | "recovering" = "browser";
const browserOnly = () => mode === "browser";
let values: Record<string, string | null> = {};
const pending = new Map<string, string | null>();
let writing: Promise<void> | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;
const own = (o: object, key: string) => Object.prototype.hasOwnProperty.call(o, key);
const notice = () => { problem = true; window.dispatchEvent(new Event(PREFERENCES_ERROR)); };
function cached(key: string) { try { return localStorage.getItem(key); } catch { return null; } }
function cache(key: string, value: string | null) {
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); } catch { /* The backend is authoritative. */ }
}
async function request(path: string, method = "GET", body?: unknown): Promise<LocalPreferences> {
  const ctl = new AbortController(); const timeout = setTimeout(() => ctl.abort(), 8000);
  try {
    const r = await fetch(path, { method, credentials: "same-origin", headers: body === undefined ? {} : { "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body), signal: ctl.signal });
    if (r.status === 404 || r.status === 405) throw Object.assign(new Error("Preferences are unavailable in this browser"), { unsupported: true });
    if (!r.ok) throw new Error("Could not save local preferences. Please try again.");
    if (method === "GET" && r.headers.get("Content-Type")?.includes("text/html")) throw Object.assign(new Error("No local App backend"), { unsupported: true });
    const v = await r.json() as LocalPreferences;
    if (v.version !== 1 || !v.values || Array.isArray(v.values) || typeof v.values !== "object") throw new Error("Invalid local preferences response");
    return v;
  } finally { clearTimeout(timeout); }
}
function apply(remote: LocalPreferences) {
  problem = backgroundPending;
  values = { ...remote.values, ...Object.fromEntries(pending) };
  for (const [key, value] of Object.entries(values)) if (isDurablePreference(key)) cache(key, value);
  window.dispatchEvent(new Event(PREFERENCES_CHANGED));
}

async function legacyPicture(id: string): Promise<Blob | null> {
  return new Promise((resolve) => {
    let finished = false;
    const done = (blob: Blob | null) => { if (!finished) { finished = true; clearTimeout(timeout); resolve(blob); } };
    const timeout = setTimeout(() => done(null), 3000);
    // Opening a missing database must not create a second, empty image store.
    const open = indexedDB.open("myco-chat-bg", 1);
    open.onupgradeneeded = () => { open.transaction?.abort(); done(null); };
    open.onerror = () => done(null);
    open.onsuccess = () => {
      const db = open.result;
      if (finished) { db.close(); return; }
      if (!db.objectStoreNames.contains("images")) { db.close(); done(null); return; }
      const transaction = db.transaction("images", "readonly");
      const read = transaction.objectStore("images").get(id);
      read.onsuccess = () => done(read.result instanceof Blob ? read.result : null);
      read.onerror = () => done(null);
      transaction.oncomplete = () => db.close();
    };
  });
}
export async function savePreferenceImage(blob: Blob): Promise<string | null> {
  if (mode === "browser") return null;
  const ctl = new AbortController(); const timeout = setTimeout(() => ctl.abort(), 8000);
  let r: Response;
  try { r = await fetch("/app/preferences/images", { method: "POST", credentials: "same-origin", body: blob, signal: ctl.signal }); }
  finally { clearTimeout(timeout); }
  if (!r.ok) throw new Error("Could not save the picture. Please try again.");
  const { url } = await r.json() as { url: string };
  if (!/^\/app\/preferences\/images\/[a-f0-9]{64}\.(png|jpg|gif|webp)$/.test(url)) throw new Error("Invalid picture reference");
  return url;
}
async function importBackground(raw: string): Promise<string> {
  const data = JSON.parse(raw) as { def?: { image?: string }; by?: Record<string, { image?: string }> };
  for (const bg of [data.def, ...Object.values(data.by || {})]) {
    if (!bg?.image || bg.image.startsWith("/app/preferences/images/")) continue;
    const blob = await legacyPicture(bg.image);
    // Retain the original metadata if the browser's image cannot be read.
    if (!blob) throw new Error("The previous background picture could not be read. No preferences were replaced.");
    bg.image = await savePreferenceImage(blob) || bg.image;
  }
  return JSON.stringify(data);
}

/** Hydrate before importing UI modules, so their initial state is durable. */
export async function initializePreferences(): Promise<void> {
  try {
    const remote = await request("/app/preferences"); mode = "app";
    const legacy: Record<string, string | null> = {};
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key && isDurablePreference(key) && !own(remote.values, key)) legacy[key] = localStorage.getItem(key);
      }
    } catch { /* Storage may be disabled; backend preferences still work. */ }
    backgroundPending = false;
    if (legacy["myco-chat-bg"]) {
      try { legacy["myco-chat-bg"] = await importBackground(legacy["myco-chat-bg"]!); }
      catch { delete legacy["myco-chat-bg"]; backgroundPending = true; }
    }
    apply(Object.keys(legacy).length ? await request("/app/preferences/import", "POST", { values: legacy }) : remote);
  } catch (e) {
    if ((e as { unsupported?: boolean }).unsupported) { mode = "browser"; return; }
    mode = "recovering"; notice(); // Never overwrite an unreadable profile with defaults.
  }
}

export function flushPreferences(): Promise<void> {
  if (mode === "browser") return Promise.resolve();
  if (writing) return writing;
  writing = (async () => {
    if (mode === "recovering" || backgroundPending) {
      await initializePreferences();
      if (mode === "recovering") throw new Error("Could not read local preferences. Please retry.");
      if (browserOnly()) { pending.clear(); return; }
    }
    while (pending.size) {
      const batch = Object.fromEntries(pending);
      const remote = await request("/app/preferences", "PATCH", { values: batch });
      for (const [key, value] of Object.entries(batch)) if (pending.get(key) === value) pending.delete(key);
      apply(remote);
    }
  })().finally(() => { writing = null; });
  return writing;
}
/** Explicit saves (e.g. avatar + name) update the UI only after the backend commits. */
export async function savePreferences(changes: Record<string, string | null>): Promise<void> {
  if (Object.keys(changes).some((key) => !isDurablePreference(key))) throw new Error("Unsupported preference key");
  await flushPreferences();
  if (mode === "browser") {
    // This path is only for the standalone web console / older App backends.
    for (const [key, value] of Object.entries(changes)) { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); }
    window.dispatchEvent(new Event(PREFERENCES_CHANGED)); return;
  }
  apply(await request("/app/preferences", "PATCH", { values: changes }));
}
export const localPreferences = {
  getItem(key: string): string | null { return mode === "app" && isDurablePreference(key) ? values[key] ?? null : cached(key); },
  setItem(key: string, value: string) {
    if (!isDurablePreference(key) || mode === "browser") { localStorage.setItem(key, value); return; }
    values[key] = value; cache(key, value); pending.set(key, value);
    clearTimeout(timer); timer = setTimeout(() => { void flushPreferences().catch(notice); }, 80);
  },
  removeItem(key: string) {
    if (!isDurablePreference(key) || mode === "browser") { localStorage.removeItem(key); return; }
    values[key] = null; cache(key, null); pending.set(key, null);
    clearTimeout(timer); timer = setTimeout(() => { void flushPreferences().catch(notice); }, 80);
  },
};
window.addEventListener("storage", (e) => {
  if (e.key && isDurablePreference(e.key) && mode === "app") { values[e.key] = e.newValue; window.dispatchEvent(new Event(PREFERENCES_CHANGED)); }
});

/** A single workspace edit merges under the backend lock, so two windows
 * editing different workspaces cannot overwrite each other's aliases. */
export async function saveWorkspaceAppearance(id: string, name: string, look: { color?: number; pic?: string }): Promise<void> {
  await flushPreferences();
  if (mode === "browser") {
    const names = JSON.parse(cached("myco-app-hub-names") || "{}");
    const looks = JSON.parse(cached("myco-app-hub-looks") || "{}");
    if (name.trim()) names[id] = name.trim(); else delete names[id];
    if (look.color !== undefined || look.pic) looks[id] = look; else delete looks[id];
    await savePreferences({ "myco-app-hub-names": JSON.stringify(names), "myco-app-hub-looks": JSON.stringify(looks) });
    return;
  }
  apply(await request(`/app/preferences/hubs/${encodeURIComponent(id)}`, "PATCH", { name, look }));
}
