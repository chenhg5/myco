// Private answers and history stay in this browser/device, scoped by page and
// account. Storage failures fall back to memory and are surfaced in the UI.
import { cleanAnswers, validLocalResult, type Answers, type LocalResult } from "./engine";
import { QUESTIONNAIRE } from "./questions";
export type Draft = { answers: Answers; cursor: number };
export type PrivateState = { questionnaire: string; draft: Draft | null; history: LocalResult[] };
export const blankState = (): PrivateState => ({ questionnaire: QUESTIONNAIRE, draft: null, history: [] });
export const localKey = (page: string, me: string) => `myco:personality:${page}:${me}:v1`;
export function readPrivate(storage: Pick<Storage,"getItem"> | null, key: string): { state: PrivateState; saved: boolean } {
  if (!storage) return { state: blankState(), saved: false };
  try {
    const raw = storage.getItem(key); if (!raw) return { state: blankState(), saved: true };
    const v = JSON.parse(raw);
    if (v.questionnaire !== QUESTIONNAIRE) return { state: blankState(), saved: false };
    const draft = v.draft && typeof v.draft === "object" ? { answers: cleanAnswers(v.draft.answers), cursor: Number.isInteger(v.draft.cursor) ? Math.max(0, Math.min(31, v.draft.cursor)) : 0 } : null;
    return { state: { questionnaire: QUESTIONNAIRE, draft, history: Array.isArray(v.history) ? v.history.filter(validLocalResult).slice(0, 12) : [] }, saved: true };
  } catch { return { state: blankState(), saved: false }; }
}
export function writePrivate(storage: Pick<Storage,"setItem"> | null, key: string, state: PrivateState) {
  if (!storage) return false;
  try { storage.setItem(key, JSON.stringify(state)); return true; } catch { return false; }
}
