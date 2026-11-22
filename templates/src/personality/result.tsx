// Private result review and explicit summary sharing are separate actions.
import { useState } from "react";
import { ArrowLeft, Copy, Download, Share2 } from "lucide-react";
import { t as uiText, locale } from "../_lib/i18n";
import { save } from "../_lib/myco";
import { QUESTIONS } from "./questions";
import { adjacentTypes, score, typeCode, type LocalResult, type Scores } from "./engine";
import { PROFILES, MIXED } from "./profiles";
import { Art, Axes, Reading, Modal } from "./components";
import { CHOICES } from "./quiz";
import { shareResult } from "./edit";
import { kit } from "./model";
export function Summary({ scores }: { scores: Scores }) {
  const code = typeCode(scores), profile = PROFILES[code] || MIXED;
  return <><div className="pe-result-hero"><div><span className="pe-eyebrow">{uiText("这一次，你更像")}</span><h1>{code}</h1><h2>{uiText(profile.name)}</h2><p>{uiText(profile.line)}</p></div><Art code={code} /></div><p className="pe-disclaimer">{uiText("一张理解自己的小地图，不是定义你的标签。")}</p><Axes scores={scores} /><Reading code={code} /></>;
}
export function Result({ result, update, back, explore }: { result: LocalResult; update: (r: LocalResult) => void; back: () => void; explore: () => void }) {
  const [modal, setModal] = useState<"share" | "answers" | "download" | null>(null), [participant,setParticipant] = useState(""), [include,setInclude] = useState(false), [busy,setBusy] = useState(false), [error,setError] = useState("");
  const scores = score(result.answers), code = typeCode(scores);
  const text = () => `${uiText("人格探索")} · ${code}\n${new Date(result.completedAt).toLocaleDateString(locale())}\n\n${uiText((PROFILES[code] || MIXED).line)}\n${Object.entries(scores).map(([a,n]) => `${a}: ${Math.round(50 + n / 16 * 50)}% / ${100 - Math.round(50 + n / 16 * 50)}%`).join("\n")}\n\n${result.reflection}\n\n${uiText("原创自我探索问卷，非官方 MBTI® 测评；分布不是能力或准确概率。")}`;
  const publish = async () => { setBusy(true); setError(""); try { await shareResult(result, participant.trim(), include); setModal(null); } catch (e) { setError(e instanceof Error ? e.message : uiText("分享失败，请重试")); } finally { setBusy(false); } };
  return <main className="pe-scroll"><div className="pe-result"><div className="pe-result-toolbar"><button type="button" className="link-btn" onClick={back}><ArrowLeft className="ico" />{uiText("返回探索")}</button><span className="pe-muted">{uiText("仅本机可见")} · {new Date(result.completedAt).toLocaleDateString(locale())}</span></div><Summary scores={scores} />
    {code.includes("X") && <div className="pe-neighbors"><h3>{uiText("也可以读读相邻的可能")}</h3><p>{adjacentTypes(code).join(" · ")}</p><button type="button" className="btn" onClick={explore}>{uiText("探索 16 种偏好")}</button></div>}
    <section className="pe-journal"><label htmlFor="reflection">{uiText("写给自己的小记")}</label><p className="pe-muted">{uiText("哪些贴近你？哪些不像？记录一个真实的例子。")}</p><textarea id="reflection" maxLength={2000} value={result.reflection} placeholder={uiText("从一个最近的小片刻开始……")} onChange={e => update({ ...result, reflection: e.target.value })} /><span className="pe-muted">{uiText("仅保存在本机，不会自动更新已分享的摘要。")}</span></section>
    <div className="pe-result-actions"><button className="btn" type="button" onClick={() => setModal("answers")}>{uiText("回看我的答案")}</button><button className="btn" type="button" onClick={() => { if (!navigator.clipboard) { kit.flash(uiText("复制失败，请使用导出")); return; } void navigator.clipboard.writeText(text()).then(() => kit.flash(uiText("摘要已复制"))).catch(() => kit.flash(uiText("复制失败，请使用导出"))); }}><Copy className="ico" />{uiText("复制摘要")}</button><button className="btn" type="button" onClick={() => { void save(`personality-${code}.md`, text(), "text/markdown").then(r => kit.flash(uiText("已保存：{0}", r.path))).catch(() => setModal("download")); }}><Download className="ico" />{uiText("导出小记")}</button><button className="btn btn-primary" type="button" onClick={() => { setError(""); setModal("share"); }}><Share2 className="ico" />{uiText("分享摘要到页面")}</button></div>
    <p className="pe-save-note">{uiText("原创自我探索问卷，非官方 MBTI® 测评；分布不是能力或准确概率。")}</p></div>
    {modal === "download" && <Modal title={uiText("导出小记")} close={() => setModal(null)}><div className="pe-share-form"><p>{uiText("没能保存到下载文件夹，可以改用浏览器下载。")}</p><button className="btn btn-primary" type="button" onClick={() => { const url = URL.createObjectURL(new Blob([text()], { type: "text/markdown;charset=utf-8" })); const a = document.createElement("a"); a.href = url; a.download = `personality-${code}.md`; a.click(); window.setTimeout(() => URL.revokeObjectURL(url), 30000); setModal(null); }}>{uiText("用浏览器下载")}</button></div></Modal>}
    {modal === "answers" && <Modal title={uiText("回看我的答案")} close={() => setModal(null)}><ol className="pe-answer-review">{QUESTIONS.map(q => <li key={q.id}><p>{uiText(q.text)}</p><span>{uiText(CHOICES[result.answers[q.id] + 2])}</span></li>)}</ol></Modal>}
    {modal === "share" && <Modal title={uiText("分享摘要到页面")} close={() => !busy && setModal(null)}><div className="pe-share-form"><p>{uiText("页面成员和有权读取页面的 agent 可以看到你的四维分布、完成时间、分享账号和署名。逐题答案不会分享。")}</p><label>{uiText("署名（可留空）")}<input autoComplete="off" maxLength={50} value={participant} onChange={e => setParticipant(e.target.value)} /></label><label className="pe-check"><input type="checkbox" checked={include} onChange={e => setInclude(e.target.checked)} />{uiText("同时分享这篇小记")}</label>{include && <blockquote>{result.reflection || uiText("你还没有写小记。")}</blockquote>}<p className="pe-muted">{uiText("分享后可以移除；已经被他人复制的内容无法收回。")}</p>{error && <p role="alert">{error}</p>}<button className="btn btn-primary" type="button" disabled={busy} onClick={() => void publish()}>{busy ? uiText("正在分享……") : uiText("确认分享")}</button></div></Modal>}
  </main>;
}
