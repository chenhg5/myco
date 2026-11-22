// Reusable presentation: illustrations, accessible dialogs and the four answer
// distributions. Content is translated; participant names and reflections are not.
import { useEffect, useRef, type ReactNode } from "react";
import { X } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { AXES, DIMENSIONS } from "./questions";
import { axisResult, family, type Scores } from "./engine";
import { MIXED, PROFILES } from "./profiles";
export function Art({ code, className = "" }: { code?: string; className?: string }) {
  const group = code ? family(code) : "mixed", position: Record<string,string> = { ideas:"0% 0%", meaning:"100% 0%", steady:"0% 100%", experience:"100% 100%" };
  return group === "mixed" ? <img className={`pe-hero-art ${className}`} src="./art/hero.webp" alt="" /> : <div aria-hidden="true" className={`pe-family-art ${className}`} style={{ backgroundPosition: position[group] }} />;
}
export function Modal({ title, close, children }: { title: string; close: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { const el = ref.current; el?.showModal(); return () => { if (el?.open) el.close(); }; }, []);
  return <dialog className="pe-modal" ref={ref} aria-label={title} onCancel={e => { e.preventDefault(); close(); }}><header><h2>{title}</h2><button type="button" className="icon-btn" title={uiText("关闭")} aria-label={uiText("关闭")} onClick={close}><X className="ico" /></button></header>{children}</dialog>;
}
export function Axes({ scores }: { scores: Scores }) {
  return <div className="pe-axes">{AXES.map(axis => { const s = axisResult(axis, scores), d = DIMENSIONS[axis]; return <section className="pe-axis" key={axis} aria-label={`${uiText(d.title)}: ${axis[0]} ${s.left}%, ${axis[1]} ${s.right}%`}><div className="pe-axis-title"><b>{uiText(d.title)}</b><span>{s.preference === "X" ? uiText("接近中线") : uiText("本次更偏向 {0}", `${s.preference} · ${uiText(s.preference === axis[0] ? d.left : d.right)}`)}</span></div><div className="pe-axis-labels"><span>{axis[0]} · {uiText(d.left)} <b>{s.left}%</b></span><span><b>{s.right}%</b> {axis[1]} · {uiText(d.right)}</span></div><div className="pe-axis-track" aria-hidden="true"><div style={{ width: `${s.left}%` }} /><i style={{ left: `${s.left}%` }} /><em /></div><p>{s.preference === "X" ? uiText("这组回答没有明显偏向，两种方式都可以是你的资源。") : uiText(s.preference === axis[0] ? d.leftNote : d.rightNote)}</p></section>; })}</div>;
}
export function Reading({ code }: { code: string }) {
  const p = PROFILES[code] || MIXED;
  return <div className="pe-reading">{[[uiText("你可能带来的力量"), p.strengths], [uiText("怎样合作更舒服"), p.collaboration], [uiText("给自己一个小练习"), p.growth]].map(([title,text]) => <section key={title}><h3>{title}</h3><p>{uiText(text)}</p></section>)}<div className="pe-reflection-prompt"><span className="pe-eyebrow">{uiText("留给自己的一问")}</span><p>{uiText(p.prompt)}</p></div></div>;
}
export function About({ close }: { close: () => void }) {
  return <Modal title={uiText("如何理解这份探索")} close={close}><div className="pe-prose"><p>{uiText("这是一份原创的 32 题偏好探索问卷，借用 E–I、S–N、T–F、J–P 四组偏好帮助你反思，不是官方 MBTI® 测评，也未作为标准化心理量表进行验证。")}</p><p>{uiText("百分比只是这次作答按公开规则换算的分布，不是能力、性格强度或准确概率。接近中线的维度显示 X，表示本次没有明显偏向。")}</p><p>{uiText("偏好会受情境和经历影响。所有倾向都各有价值，这份结果不用于招聘筛选、能力判断或心理诊断。你比四个字母更丰富。")}</p><p>{uiText("逐题答案和本机记录不写入共享页面。只有你确认分享时，摘要才会出现在页面分享中。")}</p><p>{uiText("每组偏好有 8 题，两种方向各 4 题。选项从 −2 到 2 分，反向题倒置后求和。左端比例为 50 加总分除以 16 再乘 50；总分在 −2 到 2 之间时显示 X。")}</p><a href="https://www.myersbriggs.org/my-mbti-personality-type/the-mbti-preferences/" target="_blank" rel="noreferrer">{uiText("阅读偏好框架的介绍")}</a></div></Modal>;
}
