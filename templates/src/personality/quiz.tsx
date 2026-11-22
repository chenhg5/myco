// One quiet question at a time. Selection does not auto-advance; keyboard
// 1–5 answers and Enter continues, while Back always preserves the draft.
import { useEffect } from "react";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { QUESTIONS } from "./questions";
import { Art } from "./components";
import type { Draft } from "./local";
export const CHOICES = ["很不像我", "不太像我", "不确定", "比较像我", "很像我"];
export function Quiz({ draft, change, exit, finish, saved }: { draft: Draft; change: (draft: Draft) => void; exit: () => void; finish: () => void; saved: boolean }) {
  const q = QUESTIONS[draft.cursor], answer = draft.answers[q.id], answered = Object.keys(draft.answers).length;
  const next = () => { if (answer === undefined) return; if (draft.cursor === 31) finish(); else change({ ...draft, cursor: draft.cursor + 1 }); };
  const pick = (value: number) => change({ ...draft, answers: { ...draft.answers, [q.id]: value } });
  useEffect(() => { const key = (e: KeyboardEvent) => { if ((e.target as Element)?.closest("input,textarea,select,dialog") || e.metaKey || e.ctrlKey || e.altKey) return; if (/^[1-5]$/.test(e.key)) { e.preventDefault(); pick(Number(e.key) - 3); } else if (e.key === "Enter" && !(e.target as Element)?.closest("button")) { e.preventDefault(); next(); } }; window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key); });
  const chapters = ["留一点时间，听听自己", "不用选最好，只选更像的", "让答案靠近日常的你", "慢慢来，快要完成了"];
  return <main className="pe-quiz"><div className="pe-quiz-top"><button className="link-btn" type="button" onClick={exit}><ArrowLeft className="ico" />{uiText("暂存并返回")}</button><span>{uiText("已回答 {0} / {1}", answered, 32)}</span></div><div className="pe-progress" role="progressbar" aria-label={uiText("答题进度")} aria-valuenow={answered} aria-valuemin={0} aria-valuemax={32}><i style={{ width: `${answered / 32 * 100}%` }} /></div>
    <div className="pe-quiz-body"><aside><Art code={["INTP","INFP","ISFJ","ISFP"][Math.floor(draft.cursor / 8)]} /><p>{uiText(chapters[Math.floor(draft.cursor / 8)])}</p><span className="pe-muted">{uiText("没有对错，也不必符合别人的期待。")}</span></aside><section className="pe-question" data-question={q.id}><span className="pe-eyebrow">{uiText("第 {0} 题", draft.cursor + 1)}</span><h1 key={q.id} aria-live="polite">{uiText(q.text)}</h1><p className="pe-muted">{uiText("想想最近的日常，哪一个选项更接近你？")}</p><div className="pe-choices" role="radiogroup" aria-label={uiText("这句话有多像你")}>
      {CHOICES.map((label,i) => <button type="button" role="radio" tabIndex={answer === i - 2 || (answer === undefined && i === 0) ? 0 : -1} onKeyDown={e => { if (["ArrowDown","ArrowRight","ArrowUp","ArrowLeft"].includes(e.key)) { e.preventDefault(); const n = (i + (e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : 4)) % 5; pick(n - 2); (e.currentTarget.parentElement?.children[n] as HTMLElement)?.focus(); } }} aria-checked={answer === i - 2} key={label} className={answer === i - 2 ? "is-selected" : ""} onClick={() => pick(i - 2)}><i aria-hidden="true">{answer === i - 2 ? "✓" : ""}</i><span>{uiText(label)}</span></button>)}
    </div><div className="pe-question-nav"><button type="button" className="btn" disabled={draft.cursor === 0} onClick={() => change({ ...draft, cursor: draft.cursor - 1 })}><ArrowLeft className="ico" />{uiText("上一题")}</button><button type="button" className="btn btn-primary" disabled={answer === undefined} onClick={next}>{draft.cursor === 31 ? uiText("看看我的结果") : uiText("下一题")}<ArrowRight className="ico" /></button></div><p className="pe-save-note">{saved ? uiText("答案仅保存在这台设备，可以稍后继续。") : uiText("此设备暂时无法保存；关闭页面后，本次答案可能无法保留。")}</p></section></div>
  </main>;
}
