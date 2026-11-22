// Learning opens in the tutor conversation. Notes and practice are available
// beside it and provide context for the next tutoring request.
import { t as uiText } from "../_lib/i18n";
import "../_lib/sheet.css";
import "./tutor.css";
import { useState } from "react";
import { ArrowRight, BookOpen, FolderOpen, GraduationCap, ListChecks, MessageCircle, Plus } from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { addSubject } from "./edit";
import { kit, subjects, lessons, type Level } from "./model";
import { TutorChat } from "./chat";
import { Study } from "./study";
import { Library, Reader, type MaterialLocation } from "./materials";
import { LessonView } from "./lesson-view";
import { levelLabels, riemannExample } from "./example";

type View = "tutor" | "library" | "example" | "notes" | "quiz";
function App() {
  useKit(kit);
  const [view, setView] = useState<View>("tutor");
  const [subjectId, setSubject] = useState("");
  const [lessonId, setLesson] = useState("");
  const [newSubject, setNewSubject] = useState("");
  const [level, setLevel] = useState<Level>("beginner");
  const [reading, setReading] = useState<MaterialLocation>();
  const [source, setSource] = useState<MaterialLocation>();
  const subject = subjects.get(subjectId);
  const lesson = lessons.get(lessonId);
  function navigate(next: View) {
    setView(next);
    if (window.matchMedia("(max-width: 780px)").matches) setReading(undefined);
  }
  return <div className="app tu">
    <aside className="tu-sidebar">
      <div className="tu-brand"><span><GraduationCap size={20} /></span>{uiText("学习工作台")}</div>
      <nav aria-label={uiText("学习视图")}>
        <button aria-current={view === "tutor" ? "page" : undefined} onClick={() => navigate("tutor")}><MessageCircle size={17}/>{uiText("导师对话")}</button>
        <button aria-current={view === "library" ? "page" : undefined} onClick={() => navigate("library")}><FolderOpen size={17}/>{uiText("教材库")}</button>
        <button aria-current={view === "notes" ? "page" : undefined} onClick={() => navigate("notes")}><BookOpen size={17}/>{uiText("学习笔记")}</button>
        <button aria-current={view === "quiz" ? "page" : undefined} onClick={() => navigate("quiz")}><ListChecks size={17}/>{uiText("练习与复习")}</button>
      </nav>
      <button className="tu-example-link" type="button" aria-current={view === "example" ? "page" : undefined}
        onClick={() => navigate("example")}>{uiText("打开示例课")}<ArrowRight size={15}/></button>
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
    <div className="tu-workspace" data-reader={Boolean(reading)}>
    <main className="tu-main">
      <div hidden={view !== "tutor"} className="tu-chat-view">
        <TutorChat subject={subject} lesson={lesson} level={level} onLevel={setLevel} source={source}
          openSource={setReading} openLibrary={() => setView("library")} openExample={() => setView("example")}
          clearSource={() => setSource(undefined)} clearContext={() => { setSubject(""); setLesson(""); }}/>
      </div>
      {view === "library" && <Library onOpen={setReading}/>}
      {view === "example" && <section className="tu-study">
        <header className="tu-study-head">
          <div><h1>{uiText("示例课")}</h1><p>{uiText("切换学习基础，看看同一个问题如何讲给不同的人。")}</p></div>
          <button className="btn" type="button" onClick={() => setView("tutor")}>{uiText("去问导师")}<ArrowRight size={15}/></button>
        </header>
        <div className="tu-levels" aria-label={uiText("学习基础")}>
          {Object.entries(levelLabels()).map(([id, label]) => <button className="btn" type="button" key={id} aria-pressed={id === level} onClick={() => setLevel(id as Level)}>{label}</button>)}
        </div>
        <LessonView key={level} content={riemannExample(level)}/>
        <p className="tu-example-source"><a target="_blank" rel="noreferrer" href="https://www.claymath.org/millennium/riemann-hypothesis/">{uiText("延伸阅读：克雷数学研究所")}</a></p>
      </section>}
      {(view === "notes" || view === "quiz") && <Study
        view={view}
        subject={subject || subjects.rows[0]}
        lessonId={lessonId}
        setLesson={setLesson}
        ask={(id, card) => { setSubject(id); setLesson(card); setView("tutor"); }}
      />}
    </main>
    {reading && <Reader location={reading} onPage={setReading} onClose={() => setReading(undefined)} onAsk={(location) => {
      setSource(location); setView("tutor");
      if (window.matchMedia("(max-width: 780px)").matches) setReading(undefined);
    }}/>}
    </div>
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App);
