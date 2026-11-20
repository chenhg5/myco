import { myco } from "../_lib/myco";
import type { MarketData } from "./model";
import type { CacheStore, ProviderOptions, Transport } from "./providers";
const memory = new Map<string, unknown>();
let dbp: Promise<IDBDatabase | null> | undefined;
function db() {
  return (dbp ??= new Promise((resolve) => {
    try {
      const r = indexedDB.open("myco-stock-watch", 1);
      r.onupgradeneeded = () => r.result.createObjectStore("cache");
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => resolve(null);
    } catch {
      resolve(null);
    }
  }));
}
async function read<T>(key: string): Promise<T | null> {
  if (memory.has(key)) return memory.get(key) as T;
  const d = await db();
  if (!d) return null;
  return new Promise((resolve) => {
    try {
      const r = d.transaction("cache").objectStore("cache").get(key);
      r.onsuccess = () => {
        if (r.result !== undefined) memory.set(key, r.result);
        resolve(r.result ?? null);
      };
      r.onerror = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
}
async function put(key: string, v: unknown) {
  memory.set(key, v);
  const d = await db();
  if (!d) return;
  await new Promise<void>((resolve) => {
    try {
      const tx = d.transaction("cache", "readwrite");
      tx.objectStore("cache").put(v, key);
      tx.oncomplete = () => resolve();
      tx.onerror = () => resolve();
    } catch {
      resolve();
    }
  });
}
export const cache: CacheStore = { get: (k) => read<MarketData>(k), set: put };
export type LocalSettings = ProviderOptions & {
  autoRefresh: boolean;
  redUp: boolean;
  theme: "system" | "light" | "dark";
};
export const defaults: LocalSettings = {
  source: "auto",
  gateway: "",
  autoRefresh: false,
  redUp: true,
  theme: "system",
};
export const loadSettings = async () => ({
  ...defaults,
  ...(await read<Partial<LocalSettings>>("settings")),
});
export const saveSettings = (s: LocalSettings) => put("settings", s);
/** Network requests leave from the App, not an internet-enabled iframe or Hub. */
export const transport: Transport = async (url) => {
  if (!myco?.os?.net?.fetch)
    throw new Error("请在支持 myco.os 的新版桌面 App 中打开，才能获取行情");
  const r = await myco.os.net.fetch({
    url,
    headers: { "User-Agent": "Mozilla/5.0" },
    response: "text",
    timeout_ms: 15000,
    max_bytes: 4000000,
  });
  if (r.status === 429) throw new Error("数据源限流，请稍后刷新或切换数据源");
  if (r.status < 200 || r.status >= 300)
    throw new Error(
      `数据源请求失败（HTTP ${r.status}），请稍后重试或切换数据源`,
    );
  if (r.truncated) throw new Error("数据源返回内容过大，已停止读取");
  let text = r.body;
  if (r.encoding === "base64") {
    const bytes = Uint8Array.from(atob(text), (c) => c.charCodeAt(0));
    text = new TextDecoder("utf-8").decode(bytes);
  }
  try {
    return JSON.parse(text);
  } catch {
    throw new Error("数据源没有返回有效 JSON，请切换数据源");
  }
};
