// A small, data-only lesson format. Agent records supply text and values;
// they never supply executable markup or asset URLs.
export type Stage = { title: string; text: string };
export type Point = { x: number; y: number; label: string };
export type Diagram =
  | { type: "coordinate"; title: string; description: string; xLabel: string; yLabel: string; line: number; point: Point }
  | { type: "primes"; title: string; description: string }
  | { type: "sequence"; title: string; description: string; steps: Stage[] };
export type LessonContent = {
  title: string;
  stages: Stage[];
  diagram?: Diagram;
  check?: { question: string; choices: string[]; answer: number; feedback: string };
  sources: { page: number; quote: string }[];
};
const object = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
const string = (value: unknown, max = 2000) => typeof value === "string" ? value.trim().slice(0, max) : "";
const finite = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const rows = (value: unknown): unknown[] => Array.isArray(value) ? value : [];

function stages(value: unknown) {
  return rows(value).slice(0, 6).map((raw) => {
    const item = object(raw);
    return { title: string(item.title, 120), text: string(item.text, 3000) };
  }).filter((s) => s.title && s.text);
}

function diagram(value: unknown): Diagram | undefined {
  const d = object(value);
  const title = string(d.title, 120);
  const description = string(d.description, 800);
  if (!title) return;
  if (d.type === "primes") return { type: "primes", title, description };
  if (d.type === "sequence") {
    const steps = stages(d.steps);
    if (steps.length >= 2) return { type: "sequence", title, description, steps };
  }
  if (d.type === "coordinate") {
    const p = object(d.point);
    if (!finite(d.line) || !finite(p.x) || !finite(p.y)) return;
    if (d.line <= 0 || d.line >= 1 || p.x < 0 || p.x > 1 || p.y < -3 || p.y > 3) return;
    return {
      type: "coordinate", title, description,
      xLabel: string(d.xLabel, 30) || "x", yLabel: string(d.yLabel, 30) || "y",
      line: d.line, point: { x: p.x, y: p.y, label: string(p.label, 55) },
    };
  }
}

export function readLesson(value: unknown): LessonContent | undefined {
  const item = object(value);
  const title = string(item.title, 160);
  const entries = stages(item.stages);
  if (!title || entries.length === 0) return;
  const q = object(item.check);
  const question = string(q.question, 550);
  const choices = rows(q.choices).slice(0, 5).map((v) => string(v, 400));
  const check = question && choices.length >= 2 && choices.every(Boolean) && finite(q.answer) &&
    Number.isInteger(q.answer) && q.answer >= 0 && q.answer < choices.length
    ? { question, choices, answer: q.answer, feedback: string(q.feedback, 900) } : undefined;
  const sources = rows(item.sources).slice(0, 8).flatMap((value) => {
    const entry = object(value);
    const quote = string(entry.quote, 450);
    return finite(entry.page) && Number.isInteger(entry.page) && entry.page > 0 && quote ? [{ page: entry.page, quote }] : [];
  });
  return { title, stages: entries, diagram: diagram(item.diagram), check, sources };
}
