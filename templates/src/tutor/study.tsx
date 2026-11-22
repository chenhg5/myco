// Shared notes and the question bank stay editable. The practice view keeps
// the answer key hidden until the learner enters question editing.
import { useState } from "react";
import { ArrowUpRight, Check, Plus } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { MycoRecord } from "../_lib/myco";
import { addLesson, addQuestion, answerQuestion, updateLesson, updateQuestion, updateSubject } from "./edit";
import { dueQuestions, kit, latestAttempts, lessons, questions, type Question, type Subject } from "./model";

type StudyProps = {
  view: "notes" | "quiz";
  subject?: MycoRecord<Subject> | null;
  lessonId: string;
  setLesson: (id: string) => void;
  ask: (subject: string, lesson: string) => void;
};

export function Study({ view, subject, lessonId, setLesson, ask }: StudyProps) {
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [review, setReview] = useState(false);
  if (!subject) return <div className="tu-study">
    <h1>{view === "notes" ? uiText("学习笔记") : uiText("练习与复习")}</h1>
    <p className="faint">{uiText("先添加学习主题，或让导师帮你制定学习计划。")}</p>
    <button className="btn" onClick={() => ask("", "")}>{uiText("问导师")}<ArrowUpRight size={15}/></button>
  </div>;
  const cards = lessons.rows.filter((r) => r.data.subject === subject.id);
  const card = cards.find((r) => r.id === lessonId) || cards[0];
  const bank = questions.rows.filter((r) => r.data.subject === subject.id);
  const due = dueQuestions(subject.id, kit.me);
  const latest = latestAttempts(subject.id, kit.me);
  return <div className="tu-study">
    <header className="tu-study-head">
      <div>
        <h1>{view === "notes" ? uiText("学习笔记") : uiText("练习与复习")}</h1>
        <p>{subject.data.title || uiText("未命名主题")}</p>
      </div>
      <button className="btn" onClick={() => ask(subject.id, view === "notes" ? card?.id || "" : "")}>{uiText("结合当前内容问导师")}<ArrowUpRight size={15}/></button>
    </header>
    {view === "notes" ? <>
      <label className="tu-field">{uiText("主题名称")}
        <input className="inp" value={subject.data.title || ""} onChange={(e) => updateSubject(subject.id, { title: e.target.value })}/>
      </label>
      <label className="tu-field">{uiText("学习目标")}
        <input className="inp" value={subject.data.goal || ""} onChange={(e) => updateSubject(subject.id, { goal: e.target.value })}/>
      </label>
      <div className="tu-cards">
        {cards.map((r) => <button className="btn" aria-pressed={r.id === card?.id} key={r.id} onClick={() => setLesson(r.id)}>{r.data.title || uiText("未命名知识卡")}</button>)}
      </div>
      {card && <div className="tu-note">
        <label className="tu-field">{uiText("知识卡标题")}
          <input className="inp" value={card.data.title || ""} onChange={(e) => updateLesson(card.id, { title: e.target.value })}/>
        </label>
        <label className="tu-field">{uiText("讲解")}
          <textarea value={card.data.explanation || ""} onChange={(e) => updateLesson(card.id, { explanation: e.target.value })}/>
        </label>
        <label className="tu-field">{uiText("一句话总结")}
          <input className="inp" value={card.data.summary || ""} onChange={(e) => updateLesson(card.id, { summary: e.target.value })}/>
        </label>
      </div>}
    </> : <>
      <div className="tu-quiz-tools">
        <div className="tu-cards">
          <button className="btn" aria-pressed={!review} onClick={() => setReview(false)}>{uiText("全部练习")} · {bank.length}</button>
          <button className="btn" aria-pressed={review} onClick={() => setReview(true)}>{uiText("待复习")} · {due.length}</button>
        </div>
        <button className="btn" onClick={() => { setEditing(!editing); setReview(false); }}>{editing ? uiText("完成编辑") : uiText("编辑题目")}</button>
      </div>
      {(review ? due : bank).map((r) => <Quiz key={r.id} question={r} editing={editing} correct={latest.get(r.id)?.correct}/>)}
      {bank.length === 0 && <p className="faint">{uiText("添加一道题，或请 agent 根据知识卡出题。")}</p>}
      {review && bank.length > 0 && due.length === 0 && <p className="faint">{uiText("今天没有待复习的题。")}</p>}
    </>}
    {(view === "notes" || editing) && <form className="tu-add" onSubmit={(e) => {
      e.preventDefault();
      if (!draft.trim()) return;
      if (view === "notes") setLesson(addLesson(subject.id, draft));
      else addQuestion(subject.id, "", draft);
      setDraft("");
    }}>
      <input className="inp" value={draft} onChange={(e) => setDraft(e.target.value)}
        aria-label={view === "notes" ? uiText("新知识卡标题") : uiText("新题目")}
        placeholder={view === "notes" ? uiText("加一张知识卡") : uiText("加一道练习题")}/>
      <button className="btn" disabled={!draft.trim()}><Plus size={15}/>{view === "notes" ? uiText("添加知识卡") : uiText("添加题目")}</button>
    </form>}
  </div>;
}

function Quiz({ question, editing, correct }: { question: MycoRecord<Question>; editing: boolean; correct?: boolean }) {
  const [choice, setChoice] = useState<number | null>(null);
  const { id, data } = question;
  const options = Array.isArray(data.options) ? data.options.filter((o) => typeof o === "string") : [];
  return <article className="tu-quiz">
    {editing ? <input className="inp" aria-label={uiText("题目内容")} value={data.prompt || ""} onChange={(e) => updateQuestion(id, { prompt: e.target.value })}/>
      : <h2>{data.prompt || uiText("未命名题目")}</h2>}
    <div className="tu-options">
      {options.map((option, i) => editing
        ? <input className="inp" key={i} value={option} aria-label={uiText("选项 {0} 内容", i + 1)}
            onChange={(e) => updateQuestion(id, { options: options.map((o, index) => index === i ? e.target.value : o) })}/>
        : <button type="button" key={i} aria-pressed={choice === i} onClick={() => setChoice(i)}>
            <span>{String.fromCharCode(65 + i)}</span>{option}
          </button>)}
    </div>
    {editing ? <>
      <label className="tu-field">{uiText("正确选项")}
        <select className="sel" value={data.answer ?? 0} onChange={(e) => updateQuestion(id, { answer: Number(e.target.value) })}>
          {options.map((_, i) => <option key={i} value={i}>{String.fromCharCode(65 + i)}</option>)}
        </select>
      </label>
      <label className="tu-field">{uiText("答案讲解")}
        <input className="inp" value={data.explanation || ""} onChange={(e) => updateQuestion(id, { explanation: e.target.value })}/>
      </label>
    </> : <>
      <button className="btn" disabled={choice === null || !options[choice]?.trim() || !Number.isInteger(data.answer)}
        onClick={() => { if (choice !== null) { answerQuestion(id, choice, kit.me); setChoice(null); } }}>{uiText("提交答案")}</button>
      {correct !== undefined && <p className={correct ? "tu-correct" : "tu-feedback"} role="status">
        {correct && <Check size={15}/>} {correct ? uiText("答对了") : uiText("再试一次")}
        {correct && data.explanation ? ` · ${data.explanation}` : ""}
      </p>}
    </>}
  </article>;
}
