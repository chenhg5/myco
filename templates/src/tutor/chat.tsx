// Send a saved question to the page's agent. The answer arrives as a separate
// shared record and survives closing or reopening the app.
import { useEffect, useRef, useState } from "react";
import { ArrowUp, BookOpen, Compass, Copy, FileText, GraduationCap, Lightbulb, ListChecks, X } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { MycoRecord } from "../_lib/myco";
import { canSend, saveRequest, sendRequest } from "./edit";
import { answers, kit, materials, materialText, requests, type Lesson, type Level, type Mode, type Request, type Subject } from "./model";
import { levelLabels } from "./example";
import { readLesson } from "./lesson-content";
import { LessonView } from "./lesson-view";
import { locationLabel, type MaterialLocation } from "./materials";

const MAX_PROMPT = 2000;

function modes() {
  return [
    { id: "explain", label: uiText("讲懂一个概念"), example: uiText("用一个生活例子解释贝叶斯定理。"), icon: Lightbulb },
    { id: "guide", label: uiText("一步步学"), example: uiText("从零开始教我 SQL，每次只讲一步。"), icon: Compass },
    { id: "practice", label: uiText("考考我"), example: uiText("考考我对大语言模型的理解，一次问一题。"), icon: ListChecks },
  ] as const;
}

function agentBrief(id: string, request: Request) {
  return uiText(
    "请担任我的学习导师。学习页面 ID：{0}。请读取本页 agent.md 和 tutor_requests 中 ID 为 {1} 的记录。按所选学习基础，用我的语言从直觉和例子讲起；若附了教材，请读对应原文并引用。把分层讲解、交互图和理解检查写进 tutor_answers，使用同一个记录 ID。\n\n我的问题：{2}",
    kit.page, id, request.prompt,
  );
}

const normalizeExcerpt = (s: string) => s.replace(/\s+/g, " ").trim();

export function TutorChat({ subject, lesson, level, onLevel, source, clearSource, openSource, openLibrary, openExample, clearContext }: {
  subject: MycoRecord<Subject> | null;
  lesson: MycoRecord<Lesson> | null;
  level: Level;
  onLevel: (level: Level) => void;
  source?: MaterialLocation;
  clearSource: () => void;
  openSource: (location: MaterialLocation) => void;
  openLibrary: () => void;
  openExample: () => void;
  clearContext: () => void;
}) {
  const [prompt, setPrompt] = useState("");
  const [mode, setMode] = useState<Mode>("explain");
  const [working, setWorking] = useState(false);
  const [notice, setNotice] = useState("");
  const [manual, setManual] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  const bottom = useRef<HTMLDivElement>(null);
  const timeline = requests.rows.filter((r) => r.data.who === kit.me);
  const last = timeline[timeline.length - 1];
  const activeSource = source && materials.get(source.id);

  useEffect(() => {
    if (last?.id) bottom.current?.scrollIntoView({ block: "nearest" });
  }, [last?.id]);

  async function copy(message: string) {
    try {
      await navigator.clipboard.writeText(message);
      setNotice(uiText("已复制，粘贴到你常用的 agent 对话。"));
      setManual("");
    } catch {
      setManual(message);
    }
  }

  async function submit() {
    if (working || !kit.me || !prompt.trim()) return;
    setWorking(true);
    setNotice("");
    setManual("");
    const request = {
      prompt: prompt.trim(), mode, subject: subject?.id || "", lesson: lesson?.id || "",
      who: kit.me, order: Date.now(),
      level, ...(source && activeSource ? { material: source.id, page: source.page } : {}),
    };
    try {
      const id = await saveRequest(request);
      const message = agentBrief(id, request);
      setPrompt("");
      if (!canSend) await copy(message);
      else {
        try {
          await sendRequest(message);
          setNotice(uiText("已发给导师。你可以继续看笔记，导师的回答会显示在这里。"));
        } catch {
          setNotice(uiText("暂时发不出去。可以复制这条提问，发给你常用的 agent。"));
          setManual(message);
        }
      }
    } catch {
      setNotice(uiText("问题没有保存，请检查连接后重试。"));
    } finally {
      setWorking(false);
    }
  }

  const actions = modes();
  return <div className={`tu-chat${timeline.length ? " has-turns" : ""}`}>
    <div className="tu-chat-content">
      {timeline.length === 0
        ? <header className="tu-welcome">
            <span className="tu-mark"><GraduationCap size={28}/></span>
            <h1>{uiText("今天想学什么？")}</h1>
            <p>{uiText("让导师讲懂一个概念、带你一步步学，或者出题检验理解。")}</p>
            <div className="tu-welcome-links">
              <button className="btn" type="button" onClick={openLibrary}><FileText size={16}/>{uiText("导入教材")}</button>
              <button className="btn" type="button" onClick={openExample}><BookOpen size={16}/>{uiText("打开示例课")}</button>
            </div>
          </header>
        : <section className="tu-turns" aria-label={uiText("导师对话")} aria-live="polite">
            {timeline.map(({ id, data }) => {
              const answer = answers.get(id)?.data;
              const content = readLesson(answer?.lesson);
              const materialID = data.material;
              const material = materialID && materials.get(materialID);
              if (content) content.sources = materialID && material
                ? content.sources.filter(({ page, quote }) => page <= material.data.pages &&
                  normalizeExcerpt(materialText(materialID, page)).includes(normalizeExcerpt(quote)))
                : [];
              return <article className="tu-turn" key={id}>
                <p className="tu-user-turn">{data.prompt}</p>
                {content || answer?.text
                  ? <div className="tu-tutor-turn"><div className="tu-role"><GraduationCap size={18}/>{uiText("导师")}</div>
                    {content ? <LessonView key={answers.get(id)?.version} content={content}
                      cite={materialID && material ? (page) => openSource({ id: materialID, page }) : undefined}
                      pageLabel={material ? (page) => locationLabel(material.data.format, page) : undefined}/>
                      : <div className="tu-answer-text">{answer?.text}</div>}
                  </div>
                  : <div className="tu-wait"><span>{uiText("还没有导师回复")}</span><button type="button" className="btn" onClick={() => void copy(agentBrief(id, data))}><Copy size={14}/>{uiText("复制给 agent")}</button></div>}
              </article>;
            })}
            <div ref={bottom}/>
          </section>}
    </div>
    <div className="tu-compose-area">
      {timeline.length === 0 && <div className="tu-starters">
        {actions.map(({ id, label, example, icon: Icon }) => <button type="button" key={id} onClick={() => {
          setMode(id); setPrompt(example); input.current?.focus();
        }}><Icon size={17}/>{label}</button>)}
      </div>}
      <form className="tu-compose" onSubmit={(e) => { e.preventDefault(); void submit(); }}>
        {activeSource && source && <div className="tu-context"><FileText size={14}/>
          <span>{activeSource.data.title} · {locationLabel(activeSource.data.format, source.page)}</span>
          <button className="icon-btn" type="button" aria-label={uiText("移除教材引用")} onClick={clearSource}><X size={14}/></button>
        </div>}
        {subject && <div className="tu-context"><BookOpen size={14}/><span>{lesson?.data.title || subject.data.title || uiText("未命名主题")}</span>
          <button className="icon-btn" type="button" aria-label={uiText("移除学习上下文")} onClick={clearContext}><X size={14}/></button>
        </div>}
        <textarea ref={input} maxLength={MAX_PROMPT} value={prompt} onChange={(e) => setPrompt(e.target.value)}
          aria-label={uiText("问导师")} placeholder={uiText("输入你想学的内容，或接着追问…")}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void submit();
            }
          }}/>
        <div className="tu-compose-controls">
          <div className="tu-compose-options">
            <select className="sel" aria-label={uiText("学习基础")} value={level} onChange={(e) => onLevel(e.target.value as Level)}>
              {Object.entries(levelLabels()).map(([id, label]) => <option value={id} key={id}>{label}</option>)}
            </select>
            <select className="sel" aria-label={uiText("辅导方式")} value={mode} onChange={(e) => setMode(e.target.value as Mode)}>
              {actions.map(({ id, label }) => <option value={id} key={id}>{label}</option>)}
            </select>
          </div>
          <button className="btn btn-primary" aria-label={canSend ? uiText("问导师") : uiText("复制给 agent")} disabled={working || !prompt.trim()}>
            {canSend ? <ArrowUp size={16}/> : <Copy size={15}/>}
            {working ? uiText("正在准备…") : canSend ? uiText("问导师") : uiText("复制给 agent")}
          </button>
        </div>
      </form>
      <p className="tu-send-note" role="status">{notice || (canSend
        ? uiText("问题会发给发布这个页面的 agent。")
        : uiText("把问题复制到 agent 对话，回答会显示在这里。"))}</p>
      {manual && <div className="tu-copy">
        <textarea className="tu-manual" aria-label={uiText("给导师的消息")} readOnly value={manual} onFocus={(e) => e.target.select()}/>
        <button type="button" className="btn" onClick={() => void copy(manual)}><Copy size={14}/>{uiText("复制给 agent")}</button>
      </div>}
    </div>
  </div>;
}
