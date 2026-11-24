import { Copy, ExternalLink, Plus } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { maxLinkLength, maxLinks, type VideoLink } from "./model";

export function VideoLinkEditor({ links, change, invalid }: {
  links: VideoLink[]; change: (links: VideoLink[]) => void; invalid: number;
}) {
  function update(index: number, patch: Partial<VideoLink>) {
    change(links.map((link, i) => i === index ? { ...link, ...patch } : link));
  }

  return <fieldset className="mk-link-editor">
    <legend>{uiText("视频链接")}</legend>
    <p>{uiText("较长的视频可以添加网页链接。")}</p>
    {links.map((link, index) => <div key={index} className="mk-link-fields">
      <label>{uiText("视频网址")}<input className="inp" type="text" inputMode="url" placeholder="https://" maxLength={maxLinkLength}
        aria-invalid={invalid === index} value={link.url} onChange={event => update(index, { url: event.target.value })}/></label>
      <label>{uiText("链接说明")}<input className="inp" placeholder={uiText("例如 B 站、YouTube 或飞书")} maxLength={240}
        value={link.label} onChange={event => update(index, { label: event.target.value })}/></label>
      <button type="button" className="btn" onClick={() => change(links.filter((_, i) => i !== index))}>{uiText("移除")}</button>
    </div>)}
    {invalid >= 0 && <p role="alert">{uiText("请填写有效的 HTTP 或 HTTPS 链接。")}</p>}
    <button type="button" className="btn" disabled={links.length >= maxLinks} onClick={() => change([...links, { url: "", label: "" }])}>
      <Plus size={14}/>{uiText("添加视频链接")}
    </button>
  </fieldset>;
}

export function VideoLinks({ links, copy }: { links: VideoLink[]; copy: (text: string) => void }) {
  if (!links.length) return null;
  return <div className="mk-video-links">
    <h3>{uiText("视频链接")}</h3>
    <ul>{links.map(({ url, label }, index) => <li key={index}>
      <a href={url} target="_blank" rel="noopener noreferrer"><ExternalLink size={14}/><span>{label || url}</span></a>
      <button type="button" className="btn" onClick={() => copy(url)}><Copy size={14}/>{uiText("复制链接")}</button>
    </li>)}</ul>
  </div>;
}
