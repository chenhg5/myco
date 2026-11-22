// Shared study records and small calculations. The page and agents edit the
// same subjects, lessons, questions and attempts.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Subject = { title?: string; goal?: string; order?: number };
export type Lesson = { subject?: string; title?: string; explanation?: string; summary?: string; order?: number };
export type Question = { subject?: string; lesson?: string; prompt?: string; options?: string[]; answer?: number; explanation?: string; order?: number };
export type Attempt = { question?: string; choice?: number; correct?: boolean; date?: string; who?: string };
export type Mode = "explain" | "guide" | "practice";
export type Level = "beginner" | "school" | "university";
export type Delivery = "pending" | "sent" | "unconfirmed" | "copy";
export type Request = { prompt: string; mode: Mode; subject: string; lesson: string; who: string; order: number; level?: Level; material?: string; page?: number; delivery?: Delivery; previous?: string };
export type Answer = { text?: string; lesson?: unknown };
export type Material = { title: string; format: "pdf" | "text"; pages: number; order: number };
export type MaterialPart = { material: string; page: number; order: number; text: string };
export const subjects = new Store<Subject & Record<string, unknown>>("subjects");
export const lessons = new Store<Lesson & Record<string, unknown>>("lessons");
export const questions = new Store<Question & Record<string, unknown>>("questions");
export const attempts = new Store<Attempt & Record<string, unknown>>("attempts");
export const requests = new Store<Request & Record<string, unknown>>("tutor_requests");
export const answers = new Store<Answer & Record<string, unknown>>("tutor_answers");
export const materials = new Store<Material & Record<string, unknown>>("tutor_materials");
export const materialParts = new Store<MaterialPart & Record<string, unknown>>("tutor_material_parts");
export const kit = new Kit([subjects, lessons, questions, attempts, requests, answers, materials, materialParts], { quiet: true });

export function materialText(id: string, page: number) {
  return materialParts.rows.filter((r) => r.data.material === id && r.data.page === page).map((r) => r.data.text).join("");
}

export function latestAttempts(subject: string, who: string) {
  const by = new Map<string, Attempt>();
  for (const r of attempts.rows) {
    const a = r.data;
    if (a.who !== who || !a.question || !questions.get(a.question) || questions.get(a.question)?.data.subject !== subject) continue;
    const old = by.get(a.question);
    if (!old || String(a.date || "") > String(old.date || "")) by.set(a.question, a);
  }
  return by;
}

export function dueQuestions(subject: string, who: string) {
  const latest = latestAttempts(subject, who);
  return questions.rows.filter((q) => q.data.subject === subject && (!latest.get(q.id)?.correct ||
    Date.now() - new Date(latest.get(q.id)?.date || 0).getTime() >= 3 * 86400000));
}
