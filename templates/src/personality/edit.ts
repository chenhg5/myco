// Sharing is explicit: publish an answer-free summary, or remove that summary.
// Exact versions protect other people's changes; private results remain local.
import { myco } from "../_lib/myco";
import { t as uiText } from "../_lib/i18n";
import { kit, results } from "./model";
import { score, type LocalResult, type SharedResult } from "./engine";
export async function shareResult(local: LocalResult, participant: string, includeReflection: boolean) {
  const data: SharedResult = { questionnaire: local.questionnaire, scores: score(local.answers), completed_at: local.completedAt, participant: participant.trim().slice(0,50), reflection: includeReflection ? local.reflection : "", shared_by: kit.me };
  const id = `shared-${local.id}`;
  if (results.get(id)) throw new Error(uiText("这份摘要已在页面分享中，可先移除旧分享再重新分享。"));
  const row = await kit.track(myco!.data.put("results", data, { id, ifVersion: 0 }));
  if (!row) throw new Error(uiText("分享未确认，请先检查页面分享列表。"));
  const current = results.get(row.id);
  if (!current || current.version <= row.version) results.rows = [...results.rows.filter(r => r.id !== row.id), { ...row, data }];
  kit.emit();
  kit.flash(uiText("结果摘要已分享到本页面"));
}
export async function unshare(id: string, version: number) {
  const ok = await kit.track(myco!.data.remove("results", id, { ifVersion: version }));
  if (!ok) { await results.load().catch(() => {}); kit.emit(); return; }
  results.rows = results.rows.filter(r => r.id !== id); kit.emit(); kit.flash(uiText("已移除页面分享，不影响本机记录"));
}
