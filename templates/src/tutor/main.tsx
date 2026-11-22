// A compact learning workspace: organize topics, read agent-written lessons,
// practise questions and see personal progress from shared records.
import { t as uiText } from "../_lib/i18n";
import "../_lib/sheet.css";
import "./tutor.css";
import { useState } from "react";
import { BookOpen, Check, ChevronRight, Plus, RotateCcw } from "lucide-react";
import { Bar, Toast, mount, useKit } from "../_lib/appkit";
import { addSubject, updateSubject, addLesson, updateLesson, addQuestion, updateQuestion, answerQuestion } from "./edit";
import { kit, subjects, lessons, questions, latestAttempts, dueQuestions } from "./model";

type Tab = "learn" | "practice" | "progress";
function App() {
  useKit(kit);
  const [subjectId, setSubjectId] = useState("");
  const [lessonId, setLessonId] = useState("");
  const [tab, setTab] = useState<Tab>("learn");
  const [newSubject, setNewSubject] = useState("");
  const [newLesson, setNewLesson] = useState("");
  const [newQuestion, setNewQuestion] = useState("");
  const [choice, setChoice] = useState<Record<string, number>>({});
  const [editingQuestions, setEditingQuestions] = useState(false);
  const subject = subjects.get(subjectId) || subjects.rows[0];
  const ownLessons = lessons.rows.filter((r) => r.data.subject === subject?.id);
  const lesson = ownLessons.find((r) => r.id === lessonId) || ownLessons[0];
  const ownQuestions = questions.rows.filter((r) => r.data.subject === subject?.id);
  const latest = latestAttempts(subject?.id || "", kit.me);
  const due = dueQuestions(subject?.id || "", kit.me);
  const done = ownQuestions.filter((r) => latest.get(r.id)?.correct).length;
  return <div className="app tu">
    <Bar kit={kit} title={subject?.data.title || uiText("学习工作台")} placeholder={uiText("学习工作台")} onTitle={(value) => { if (subject) updateSubject(subject.id, { title: value }); }} />
    <div className="tu-layout">
      <aside className="tu-sidebar">
        <div className="tu-caption">{uiText("学习主题")}</div>
        {subjects.rows.map((r) => <button type="button" key={r.id} className={`tu-subject${subject?.id === r.id ? " is-on" : ""}`} onClick={() => { setSubjectId(r.id); setLessonId(""); setTab("learn"); }}><BookOpen className="ico" /><span>{r.data.title || uiText("未命名主题")}</span><ChevronRight className="ico" /></button>)}
        <form className="tu-add" onSubmit={(e) => { e.preventDefault(); if (newSubject.trim()) { setSubjectId(addSubject(newSubject)); setNewSubject(""); } }}><input className="inp" value={newSubject} onChange={(e) => setNewSubject(e.target.value)} placeholder={uiText("加一个学习主题")} aria-label={uiText("新主题名称")} /><button className="icon-btn" type="submit" disabled={!newSubject.trim()} title={uiText("添加主题")} aria-label={uiText("添加主题")}><Plus className="ico" /></button></form>
      </aside>
      <main className="tu-main">{!subject ? <div className="tu-empty"><BookOpen /><h2>{uiText("从一个想学的主题开始")}</h2><p>{uiText("写下主题，或请 agent 帮你搭一条学习路径。")}</p></div> : <>
        <header className="tu-header"><div><input className="tu-title" value={subject.data.title || ""} onChange={(e) => updateSubject(subject.id, { title: e.target.value })} aria-label={uiText("主题名称")} /><input className="tu-goal" value={subject.data.goal || ""} onChange={(e) => updateSubject(subject.id, { goal: e.target.value })} placeholder={uiText("这次想学会什么？")} aria-label={uiText("学习目标")} /></div><span className="tu-count">{done}/{ownQuestions.length} {uiText("题已掌握")}</span></header>
        <nav className="tu-tabs" aria-label={uiText("学习视图")}>{([ ["learn", "学习内容"], ["practice", "练习"], ["progress", "进度"] ] as const).map(([id, label]) => <button type="button" key={id} className={tab === id ? "is-on" : ""} onClick={() => setTab(id)}>{uiText(label)}</button>)}</nav>
        {tab === "learn" && <div className="tu-learn"><div className="tu-lessons"><div className="tu-caption">{uiText("知识卡")}</div>{ownLessons.map((r, i) => <button type="button" key={r.id} className={`tu-lesson${lesson?.id === r.id ? " is-on" : ""}`} onClick={() => setLessonId(r.id)}><span>{String(i + 1).padStart(2, "0")}</span>{r.data.title || uiText("未命名知识卡")}</button>)}<form className="tu-add" onSubmit={(e) => { e.preventDefault(); if (newLesson.trim()) { setLessonId(addLesson(subject.id, newLesson)); setNewLesson(""); } }}><input className="inp" value={newLesson} onChange={(e) => setNewLesson(e.target.value)} placeholder={uiText("加一张知识卡")} aria-label={uiText("新知识卡标题")} /><button className="icon-btn" type="submit" disabled={!newLesson.trim()} title={uiText("添加知识卡")} aria-label={uiText("添加知识卡")}><Plus className="ico" /></button></form></div><article className="tu-article">{lesson ? <><input className="tu-lesson-title" value={lesson.data.title || ""} onChange={(e) => updateLesson(lesson.id, { title: e.target.value })} aria-label={uiText("知识卡标题")} /><label>{uiText("讲解")}</label><textarea value={lesson.data.explanation || ""} onChange={(e) => updateLesson(lesson.id, { explanation: e.target.value })} placeholder={uiText("把这个概念讲清楚。也可以让 agent 来写。")}/><label>{uiText("记住这句")}</label><textarea className="tu-summary" value={lesson.data.summary || ""} onChange={(e) => updateLesson(lesson.id, { summary: e.target.value })} placeholder={uiText("一句话总结")}/></> : <div className="tu-empty"><h2>{uiText("还没有知识卡")}</h2><p>{uiText("自己写一张，或让 agent 按学习目标生成。")}</p></div>}</article></div>}
        {tab === "practice" && <div className="tu-practice"><div className="tu-section-head"><div><h2>{uiText("随堂练习")}</h2><p>{due.length} {uiText("题待复习")}</p></div><button type="button" className="btn" onClick={() => setEditingQuestions(!editingQuestions)}>{editingQuestions ? uiText("完成编辑") : uiText("编辑题目")}</button></div>{ownQuestions.length === 0 && <div className="tu-empty"><h2>{uiText("还没有练习题")}</h2><p>{uiText("添加一道题，或请 agent 根据知识卡出题。")}</p></div>}{ownQuestions.map((r, i) => { const q = r.data, a = latest.get(r.id), picked = choice[r.id]; const options = Array.isArray(q.options) ? q.options : []; return <section className="tu-question" key={r.id}><div className="tu-question-head"><span>{uiText("第 {0} 题", i + 1)}</span>{a?.correct && <span className="tu-correct"><Check className="ico" />{uiText("已掌握")}</span>}</div>{editingQuestions ? <input className="tu-prompt" value={q.prompt || ""} onChange={(e) => updateQuestion(r.id, { prompt: e.target.value })} aria-label={uiText("题目内容")} placeholder={uiText("输入题目")}/> : <h3 className="tu-prompt">{q.prompt}</h3>}<div className="tu-options">{options.map((opt, n) => <div className="tu-option" key={n}><button type="button" className={picked === n ? "is-picked" : ""} onClick={() => setChoice((v) => ({ ...v, [r.id]: n }))} aria-label={uiText("选项 {0}", n + 1)}>{String.fromCharCode(65 + n)}</button>{editingQuestions ? <input value={opt} onChange={(e) => updateQuestion(r.id, { options: options.map((x, k) => k === n ? e.target.value : x) })} aria-label={uiText("选项 {0} 内容", n + 1)} placeholder={uiText("写一个选项")}/> : <span>{opt}</span>}</div>)}</div><div className="tu-answer">{editingQuestions && <label>{uiText("正确选项")} <select className="sel" value={q.answer ?? 0} onChange={(e) => updateQuestion(r.id, { answer: Number(e.target.value) })}>{options.map((_, n) => <option key={n} value={n}>{String.fromCharCode(65 + n)}</option>)}</select></label>}<button className="btn" type="button" disabled={picked === undefined || !q.prompt || options.some((x) => !x.trim())} onClick={() => answerQuestion(r.id, picked, kit.me)}>{uiText("提交答案")}</button></div>{a && <p className={a.correct ? "tu-feedback is-right" : "tu-feedback"}>{a.correct ? uiText("答对了") : uiText("再试一次")} · {q.explanation || uiText("可以请 agent 解释这道题。")}</p>}{editingQuestions && <input className="tu-explanation" value={q.explanation || ""} onChange={(e) => updateQuestion(r.id, { explanation: e.target.value })} placeholder={uiText("答案讲解（可选）")} aria-label={uiText("答案讲解")}/>}</section>; })}{editingQuestions && <form className="tu-add tu-add-question" onSubmit={(e) => { e.preventDefault(); if (newQuestion.trim()) { addQuestion(subject.id, lesson?.id || "", newQuestion); setNewQuestion(""); } }}><input className="inp" value={newQuestion} onChange={(e) => setNewQuestion(e.target.value)} placeholder={uiText("加一道练习题")} aria-label={uiText("新题目")} /><button className="btn" type="submit" disabled={!newQuestion.trim()}><Plus className="ico" />{uiText("添加题目")}</button></form>}</div>}
        {tab === "progress" && <div className="tu-progress"><h2>{uiText("学习进度")}</h2><div className="tu-metrics"><div><strong>{ownLessons.length}</strong><span>{uiText("知识卡")}</span></div><div><strong>{ownQuestions.length}</strong><span>{uiText("练习题")}</span></div><div><strong>{done}</strong><span>{uiText("已掌握")}</span></div><div><strong>{due.length}</strong><span>{uiText("待复习")}</span></div></div><h3>{uiText("复习清单")}</h3>{due.length ? due.map((r) => <button type="button" className="tu-review" key={r.id} onClick={() => setTab("practice")}><RotateCcw className="ico" />{r.data.prompt || uiText("未命名题目")}<ChevronRight className="ico" /></button>) : <p className="faint">{uiText("今天没有待复习的题。")}</p>}</div>}
      </>}</main>
    </div><Toast kit={kit} />
  </div>;
}
void mount(kit, App);
