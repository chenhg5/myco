// Record writes stay here so the interface and future undo support share a
// single path. A question attempt is immutable history, never an answer key.
import { uid } from "../_lib/appkit";
import { myco } from "../_lib/myco";
import { kit, subjects, lessons, questions, attempts, requests, type Subject, type Lesson, type Question, type Request, type Delivery } from "./model";

const tutorBridge = myco as (typeof myco & { send?: (text: string) => Promise<{ id: string }> });
export const canSend = typeof tutorBridge?.send === "function";

export async function saveRequest(request: Request) {
  // Wait for the Hub to assign the ID before the agent can see the message.
  const saved = await requests.put(request);
  if (!saved) throw new Error("Missing tutor request");
  kit.emit();
  return saved.id;
}

export async function sendRequest(message: string) {
  if (!tutorBridge?.send) throw new Error("Missing send bridge");
  await tutorBridge.send(message);
}

export async function saveDelivery(id: string, delivery: Delivery) {
  const record = requests.get(id);
  if (!record || !myco) return;
  const saved = await myco.data.put(requests.collection, { ...record.data, delivery }, { id, ifVersion: record.version });
  requests.apply({ collection: requests.collection, record: id, data: saved.data, version: saved.version, op: "put", seq: 0 });
  kit.emit();
}

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
