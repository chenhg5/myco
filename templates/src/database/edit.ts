// Shared query writes are explicit and retain the draft's original version.
import { queries, kit, type Query } from "./model";
import { t as uiText } from "../_lib/i18n";
export async function saveQuery(id: string, data: Query, version: number) {
  if ((queries.get(id)?.version || 0) !== version || queries.pending(id)) throw { code: "conflict", message: uiText("查询已被修改，请重新打开后保存") };
  if (!data.title.trim() || data.title.length > 100 || data.sql.length > 48000 || data.note.length > 2000 || new TextEncoder().encode(JSON.stringify(data)).length > 58000) throw new Error(uiText("查询内容过长，或缺少名称"));
  return queries.put({ ...data, title: data.title.trim() }, id);
}
export async function removeQuery(id: string) { const row = queries.get(id); if (!row) return; await kit.track((async () => { await queries.remove(id); kit.flash(uiText("共享查询已删除"), () => { void kit.track(saveQuery(id, row.data, 0)); }); })()); }
