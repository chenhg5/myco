// Learning opens in the tutor conversation. Notes and practice are available
// beside it and provide context for the next tutoring request.
import { t as uiText } from "../_lib/i18n";
import "../_lib/sheet.css";
import "./tutor.css";
import { useState } from "react";
import { BookOpen, GraduationCap, ListChecks, MessageCircle, Plus } from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { addSubject } from "./edit";
import { kit, subjects, lessons } from "./model";
import { TutorChat } from "./chat";
import { Study } from "./study";

type View = "tutor" | "notes" | "quiz";
function App() {
  useKit(kit);
  const [view, setView] = useState<View>("tutor");
  const [subjectId, setSubject] = useState("");
  const [lessonId, setLesson] = useState("");
  const [newSubject, setNewSubject] = useState("");
  const subject = subjects.get(subjectId);
  const lesson = lessons.get(lessonId);
  return <div className="app tu">
    <aside className="tu-sidebar">
      <div className="tu-brand"><span><GraduationCap size={20} /></span>{uiText("学习工作台")}</div>
      <nav aria-label={uiText("学习视图")}>
        <button aria-current={view === "tutor" ? "page" : undefined} onClick={() => setView("tutor")}><MessageCircle size={17}/>{uiText("导师对话")}</button>
        <button aria-current={view === "notes" ? "page" : undefined} onClick={() => setView("notes")}><BookOpen size={17}/>{uiText("学习笔记")}</button>
        <button aria-current={view === "quiz" ? "page" : undefined} onClick={() => setView("quiz")}><ListChecks size={17}/>{uiText("练习与复习")}</button>
      </nav>
      <section className="tu-subjects" aria-label={uiText("学习主题")}>
        <h2>{uiText("学习主题")}</h2>
        {subjects.rows.map(({ id, data }) => <button key={id} aria-pressed={id === subjectId} onClick={() => {
          setSubject(id);
          setLesson("");
          if (view === "tutor") setView("notes");
        }}>{data.title || uiText("未命名主题")}</button>)}
        <form onSubmit={(e) => {
          e.preventDefault();
          if (!newSubject.trim()) return;
          setSubject(addSubject(newSubject));
          setNewSubject("");
          setLesson("");
          setView("notes");
        }}>
          <input className="inp" maxLength={120} aria-label={uiText("新主题名称")} placeholder={uiText("加一个学习主题")} value={newSubject} onChange={(e) => setNewSubject(e.target.value)}/>
          <button className="icon-btn" disabled={!newSubject.trim()} aria-label={uiText("添加主题")}><Plus size={16}/></button>
        </form>
      </section>
    </aside>
    <main className="tu-main">
      <div hidden={view !== "tutor"} className="tu-chat-view">
        <TutorChat subject={subject} lesson={lesson} clearContext={() => { setSubject(""); setLesson(""); }}/>
      </div>
      {view !== "tutor" && <Study
        view={view}
        subject={subject || subjects.rows[0]}
        lessonId={lessonId}
        setLesson={setLesson}
        ask={(id, card) => { setSubject(id); setLesson(card); setView("tutor"); }}
      />}
    </main>
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App);
