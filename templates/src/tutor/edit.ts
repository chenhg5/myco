// Record writes stay here so the interface and future undo support share a
// single path. A question attempt is immutable history, never an answer key.
import { uid } from "../_lib/appkit";
import { kit, subjects, lessons, questions, attempts, type Subject, type Lesson, type Question } from "./model";

export function addSubject(title: string) {
  const id = uid("s");
  kit.track(subjects.put({ title: title.trim(), goal: "", order: subjects.rows.length + 1 }, id));
  kit.emit();
  return id;
}
export function updateSubject(id: string, patch: Partial<Subject>) {
  const old = subjects.get(id);
  if (!old) return;
  kit.track(subjects.put({ ...old.data, ...patch }, id));
  kit.emit();
}
export function addLesson(subject: string, title: string) {
  const id = uid("l");
  kit.track(lessons.put({ subject, title: title.trim(), explanation: "", summary: "", order: lessons.rows.length + 1 }, id));
  kit.emit();
  return id;
}
export function updateLesson(id: string, patch: Partial<Lesson>) {
  const old = lessons.get(id);
  if (!old) return;
  kit.track(lessons.put({ ...old.data, ...patch }, id));
  kit.emit();
}
export function addQuestion(subject: string, lesson: string, prompt: string) {
  const id = uid("q");
  kit.track(questions.put({ subject, lesson, prompt: prompt.trim(), options: ["", "", "", ""], answer: 0, explanation: "", order: questions.rows.length + 1 }, id));
  kit.emit();
  return id;
}
export function updateQuestion(id: string, patch: Partial<Question>) {
  const old = questions.get(id);
  if (!old) return;
  kit.track(questions.put({ ...old.data, ...patch }, id));
  kit.emit();
}
export function answerQuestion(question: string, choice: number, who: string) {
  const q = questions.get(question)?.data;
  if (!q || !who) return;
  kit.track(attempts.put({ question, choice, correct: choice === q.answer, who, date: new Date().toISOString() }, uid("a")));
  kit.emit();
}
