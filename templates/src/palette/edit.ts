// All writes enter here. A draft keeps its opening version to prevent lost edits.
import { palettes, favorites, kit } from "./model";
import { validPalette, hex, type Palette } from "./engine";
import { myco, type Store } from "../_lib/myco";
import { t as uiText } from "../_lib/i18n";
async function refresh(store: Store<any>, id: string) {
  try {
    const current = await myco!.data.get(store.collection, id);
    const index = store.rows.findIndex(r => r.id === id);
    if (index >= 0) store.rows[index] = current;
    else store.rows.push(current);
  } catch (e) {
    if ((e as { code?: string }).code === "not_found") store.rows = store.rows.filter(r => r.id !== id);
  }
  kit.emit();
}
export async function putPalette(id: string, draft: Palette, version: number) {
  if (!validPalette(draft)) throw new Error(uiText("请检查名称和色值，色卡需包含 2–12 种颜色"));
  if (palettes.pending(id) || (palettes.get(id)?.version || 0) !== version) throw { code: "conflict", message: uiText("色卡已被修改，请载入最新版本后重试") };
  try { return await palettes.put({ ...draft, title: draft.title.trim(), colors: draft.colors.map(c => ({ ...c, hex: hex(c.hex)! })), updated_at: new Date().toISOString() }, id); }
  catch (e) { await refresh(palettes, id); throw e; }
}
export async function toggleFavorite(id: string) {
  const row = favorites.get(id);
  await kit.track(favorites.put({ ...(row?.data || {}), saved: row?.data.saved !== true }, id).catch(async e => { await refresh(favorites, id); throw e; }));
}
export async function deletePalette(id: string) {
  const before = palettes.get(id); if (!before || palettes.pending(id)) return;
  await kit.track((async () => {
    try { await palettes.remove(id); } catch (e) { await refresh(palettes, id); throw e; }
    kit.flash(uiText("已删除色卡"), () => { void kit.track(putPalette(id, before.data, 0)); });
  })());
}
