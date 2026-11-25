import { appApi } from "@/appapi";

// Pages installed as apps (the desktop App on macOS): a launcher in
// ~/Applications/myco 应用 that opens the page in a window of its own
// (internal/console/installs.go). Whether this App can, and which pages are.

let supported = false;
export function setInstallSupport(on: boolean) { supported = on; }
export function canInstall() { return supported; }

let list: Promise<Set<string>> | null = null;
const key = (hubId: string, id: string) => `${hubId}/${id}`;
/** Which pages are installed (hub/page), fetched once and after a change. */
export function installed(): Promise<Set<string>> {
  if (!list) list = appApi.installs().then((r) => new Set(r.installs.map((e) => key(e.hub, e.page))), () => new Set<string>());
  return list;
}
export async function isInstalled(hubId: string, id: string) { return (await installed()).has(key(hubId, id)); }

const TILES = ["#2f6f5e", "#3b5b92", "#7a4b8c", "#a35a2a", "#8a3b4b", "#2f6c86", "#5d6b2f", "#4a4a5a"];

/** The app's icon, 512 px: its own shape (the template's SVG) in white on a
 *  coloured tile, or the first letter of its name. */
export async function iconPNG(name: string, svg?: string): Promise<string> {
  const size = 512, c = document.createElement("canvas");
  c.width = c.height = size;
  const g = c.getContext("2d")!;
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) >>> 0;
  g.fillStyle = TILES[h % TILES.length];
  const r = 92, m = 50; // (a tile with the room macOS icons leave around them)
  g.beginPath();
  g.roundRect(m, m, size - 2 * m, size - 2 * m, r);
  g.fill();
  let drawn = false;
  if (svg) {
    try {
      const img = new Image();
      img.src = `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
      await img.decode();
      // the shape, then white where it is
      const s = document.createElement("canvas");
      s.width = s.height = 256;
      const sg = s.getContext("2d")!;
      sg.drawImage(img, 0, 0, 256, 256);
      sg.globalCompositeOperation = "source-in";
      sg.fillStyle = "#fff";
      sg.fillRect(0, 0, 256, 256);
      g.drawImage(s, (size - 256) / 2, (size - 256) / 2);
      drawn = true;
    } catch { /* (the letter instead) */ }
  }
  if (!drawn) {
    g.fillStyle = "#fff";
    g.font = `600 ${size * 0.46}px -apple-system, "PingFang SC", "Segoe UI", sans-serif`;
    g.textAlign = "center";
    g.textBaseline = "middle";
    g.fillText(([...name.trim()][0] || "m").toUpperCase(), size / 2, size / 2 + size * 0.02);
  }
  return c.toDataURL("image/png");
}

/** Installs a page as an app (its launcher, with its icon). */
export async function installApp(hubId: string, id: string, name: string, templateId?: string) {
  let svg: string | undefined;
  if (templateId) svg = await appApi.template(hubId, templateId).then((t) => t.icon || undefined, () => undefined);
  await appApi.installPage(hubId, id, name, await iconPNG(name, svg));
  list = null;
}

export async function uninstallApp(hubId: string, id: string) {
  await appApi.uninstallPage(hubId, id);
  list = null;
}
