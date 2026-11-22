// Transparent scoring for this app's original 32-item reflection exercise.
// Percentages describe these answers, not ability, diagnostic certainty or norms.
import { AXES, QUESTIONS, QUESTIONNAIRE, type Axis } from "./questions";
export type Answers = Record<string, number>;
export type Scores = Record<Axis, number>;
export type LocalResult = { id: string; questionnaire: string; completedAt: string; answers: Answers; reflection: string };
export type SharedResult = { questionnaire: string; scores: Scores; completed_at: string; participant: string; reflection: string; shared_by: string; example?: boolean };
export const TYPES = ["INTJ","INTP","ENTJ","ENTP","INFJ","INFP","ENFJ","ENFP","ISTJ","ISFJ","ESTJ","ESFJ","ISTP","ISFP","ESTP","ESFP"];
export function cleanAnswers(value: unknown): Answers {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(QUESTIONS.flatMap(q => { const n = (value as Answers)[q.id]; return Number.isInteger(n) && n >= -2 && n <= 2 ? [[q.id, n]] : []; }));
}
export function score(answers: Answers): Scores {
  const valid = cleanAnswers(answers);
  if (Object.keys(valid).length !== QUESTIONS.length) throw new Error("incomplete_answers");
  const scores: Scores = { EI: 0, SN: 0, TF: 0, JP: 0 };
  for (const q of QUESTIONS) scores[q.axis] += valid[q.id] * q.pole;
  return scores;
}
export function validScores(value: unknown): value is Scores {
  if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).length !== 4) return false;
  return AXES.every(axis => Number.isInteger((value as Scores)[axis]) && Math.abs((value as Scores)[axis]) <= 16);
}
export function axisResult(axis: Axis, scores: Scores) {
  const total = scores[axis], left = Math.round(50 + total / 16 * 50);
  return { axis, left, right: 100 - left, preference: Math.abs(total) <= 2 ? "X" : total > 0 ? axis[0] : axis[1] };
}
export const typeCode = (scores: Scores) => AXES.map(a => axisResult(a, scores).preference).join("");
export const adjacentTypes = (code: string) => TYPES.filter(t => [...code].every((c, i) => c === "X" || c === t[i]));
export const family = (code: string) => code.includes("X") ? "mixed" : code[1] === "N" ? code[2] === "T" ? "ideas" : "meaning" : code[3] === "J" ? "steady" : "experience";
export function validLocalResult(r: unknown): r is LocalResult {
  const v = r as LocalResult;
  return !!v && typeof v.id === "string" && v.id.length <= 80 && v.questionnaire === QUESTIONNAIRE && typeof v.completedAt === "string" && Number.isFinite(Date.parse(v.completedAt)) && typeof v.reflection === "string" && v.reflection.length <= 2000 && Object.keys(cleanAnswers(v.answers)).length === 32;
}
export function validSharedResult(r: unknown): r is SharedResult {
  const v = r as SharedResult;
  return !!v && v.questionnaire === QUESTIONNAIRE && validScores(v.scores) && typeof v.completed_at === "string" && Number.isFinite(Date.parse(v.completed_at)) && typeof v.participant === "string" && v.participant.length <= 50 && typeof v.reflection === "string" && v.reflection.length <= 2000 && typeof v.shared_by === "string";
}
