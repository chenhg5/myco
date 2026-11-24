import { useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowLeft, ChevronLeft, ChevronRight, Copy, Download, ImageIcon, Plus, Search, Upload } from "lucide-react";
import { mount, Toast, uid, useKit } from "../_lib/appkit";
import { language, t as uiText } from "../_lib/i18n";
import { myco, type MycoRecord } from "../_lib/myco";
import { canUseOriginals, dateToday, entries, initialize, kit, pictures, statuses, thumbnail, type Entry, type Picture, type Status } from "./model";
import { saveEntry, savePicture } from "./edit";
import { upload, type PictureOptions } from "./upload";
import "../_lib/sheet.css";
import "./media-kit.css";

const blank = (): Entry => ({ title: "", date: dateToday(), version: "", pitch_zh: "", pitch_en: "", copy_zh: "", copy_en: "", tags: [], status: "draft", channels: "" });
const statusText = (status: Status) => status === "ready" ? uiText("可发布") : status === "published" ? uiText("已发布") : uiText("草稿");
const languageText = (language: Picture["language"]) => language === "zh" ? uiText("中文") : language === "en" ? uiText("英文") : uiText("无文字");
const themeText = (theme: Picture["theme"]) => theme === "dark" ? uiText("深色") : uiText("浅色");

async function copyText(text: string) {
  try { await navigator.clipboard.writeText(text); kit.flash(uiText("已复制")); }
  catch { kit.flash(uiText("剪贴板不可用，请手动选择并复制。")); }
}

function Preview({ record, full = false }: { record?: MycoRecord<Picture>; full?: boolean }) {
  const [url, setURL] = useState("");
  const file = record?.data.file;
  useEffect(() => {
    setURL("");
    if (!full || !canUseOriginals || !file) return;
    let active = true, address = "";
    void myco!.projectFiles!.get(file).then(blob => {
      if (active) { address = URL.createObjectURL(blob); setURL(address); }
    }).catch(() => undefined);
    return () => { active = false; if (address) URL.revokeObjectURL(address); };
  }, [file, full]);
  const src = url || thumbnail(record?.data.preview);
  return src ? <img loading={full ? "eager" : "lazy"} src={src} alt={record?.data.caption || record?.data.name || ""}/> : <ImageIcon size={30} aria-hidden/>;
}

function EntryEditor({ record, close, submit }: { record?: MycoRecord<Entry>; close: () => void; submit: (data: Entry, files: File[], options: PictureOptions, baseVersion: number) => Promise<void> }) {
  const [baseVersion] = useState(record?.version ?? 0);
  const [draft, setDraft] = useState<Entry>(() => ({ ...blank(), ...record?.data }));
  const [tags, setTags] = useState(() => (draft.tags || []).join(", "));
  const [options, setOptions] = useState<PictureOptions>({ language: "none", theme: "light", caption: "" });
  const [files, setFiles] = useState<File[]>([]);
  const [drag, setDrag] = useState(false);
  const [busy, setBusy] = useState(false);
  const picker = useRef<HTMLInputElement>(null);
  function edit(key: keyof Entry, value: string) { setDraft(current => ({ ...current, [key]: value })); }
  function addFiles(list: FileList | null) {
    const incoming = Array.from(list || []);
    if (incoming.length) setFiles(current => [...current, ...incoming]);
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!draft.title.trim()) return;
    setBusy(true);
    try {
      await submit({ ...draft, title: draft.title.trim(), tags: [...new Set(tags.split(/[,，]/).map(tag => tag.trim().slice(0, 30)).filter(Boolean))].slice(0, 12) }, files, options, baseVersion);
    } finally { setBusy(false); }
  }
  return <form className="mk-editor" onSubmit={event => void save(event)}>
    <header className="mk-detail-header"><h2>{record ? uiText("编辑素材条目") : uiText("新增素材条目")}</h2><button type="button" className="btn" onClick={close} disabled={busy}>{uiText("取消")}</button></header>
    <label>{uiText("标题")}<input className="inp" required maxLength={120} value={draft.title} onChange={event => edit("title", event.target.value)}/></label>
    <div className="mk-columns">
      <label>{uiText("日期")}<input className="inp" type="date" required value={draft.date} onChange={event => edit("date", event.target.value)}/></label>
      <label>{uiText("版本")}<input className="inp" maxLength={60} value={draft.version} onChange={event => edit("version", event.target.value)}/></label>
    </div>
    <label>{uiText("标签")}<input className="inp" maxLength={360} value={tags} placeholder={uiText("例如 App，CLI，手机")} onChange={event => setTags(event.target.value)}/></label>
    <div className="mk-columns">
      <label>{uiText("中文卖点")}<textarea className="inp" rows={2} maxLength={500} value={draft.pitch_zh} onChange={event => edit("pitch_zh", event.target.value)}/></label>
      <label>{uiText("英文卖点")}<textarea className="inp" rows={2} maxLength={500} value={draft.pitch_en} onChange={event => edit("pitch_en", event.target.value)}/></label>
      <label>{uiText("中文配文")}<textarea className="inp" rows={3} maxLength={3500} value={draft.copy_zh} onChange={event => edit("copy_zh", event.target.value)}/></label>
      <label>{uiText("英文配文")}<textarea className="inp" rows={3} maxLength={3500} value={draft.copy_en} onChange={event => edit("copy_en", event.target.value)}/></label>
    </div>
    <div className="mk-columns">
      <label>{uiText("状态")}<select className="sel" value={draft.status} onChange={event => edit("status", event.target.value)}>
        {statuses.map(value => <option key={value} value={value}>{statusText(value)}</option>)}
      </select></label>
      <label>{uiText("发布渠道")}<input className="inp" maxLength={250} value={draft.channels} onChange={event => edit("channels", event.target.value)}/></label>
    </div>
    {canUseOriginals && <fieldset className="mk-upload" disabled={busy}
      onDragOver={event => { event.preventDefault(); setDrag(true); }} onDragLeave={() => setDrag(false)}
      onDrop={event => { event.preventDefault(); setDrag(false); if (!busy) addFiles(event.dataTransfer.files); }} data-drag={drag}>
      <legend>{uiText("添加图片")}</legend>
      <input ref={picker} type="file" hidden multiple accept="image/png,image/jpeg,image/webp" onChange={event => { addFiles(event.target.files); event.target.value = ""; }}/>
      <button type="button" className="btn" onClick={() => picker.current?.click()}><Upload size={14}/>{uiText("拖入或选择图片")}</button>
      <p>{uiText("PNG、JPEG 或 WebP，单张最大 20 MB。")}</p>
      {files.length > 0 && <>
        <ul>{files.map((file, index) => <li key={index}>{file.name} <button className="btn" type="button" onClick={() => setFiles(current => current.filter((_, item) => item !== index))}>{uiText("移除")}</button></li>)}</ul>
        <PictureFields options={options} update={setOptions}/>
      </>}
    </fieldset>}
    <button type="submit" className="btn btn-primary" disabled={busy}>{busy ? uiText("正在保存…") : uiText("保存")}</button>
  </form>;
}

function PictureFields({ options, update }: { options: PictureOptions; update: (value: PictureOptions) => void }) {
  return <>
    <div className="mk-columns">
          <label>{uiText("图片语言")}<select className="sel" value={options.language} onChange={event => update({ ...options, language: event.target.value as Picture["language"] })}>
            {(["none", "zh", "en"] as const).map(value => <option key={value} value={value}>{languageText(value)}</option>)}
          </select></label>
          <label>{uiText("图片主题")}<select className="sel" value={options.theme} onChange={event => update({ ...options, theme: event.target.value as Picture["theme"] })}>
            {(["light", "dark"] as const).map(value => <option key={value} value={value}>{themeText(value)}</option>)}
          </select></label>
        </div>
        <label>{uiText("图片说明")}<input className="inp" maxLength={240} value={options.caption} onChange={event => update({ ...options, caption: event.target.value })}/></label>
  </>;
}

function PictureEditor({ record }: { record: MycoRecord<Picture> }) {
  const [draft, setDraft] = useState<PictureOptions | null>(null);
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  if (!draft) return <button type="button" className="btn" onClick={() => {
    const { language, theme, caption } = record.data;
    setDraft({ language, theme, caption });
    setVersion(record.version);
  }}>{uiText("编辑图片信息")}</button>;
  return <form className="mk-picture-editor" onSubmit={async event => {
    event.preventDefault();
    setBusy(true);
    try { if (await kit.track(savePicture(record.id, { ...record.data, ...draft }, version))) setDraft(null); }
    finally { setBusy(false); }
  }}>
    <PictureFields options={draft} update={setDraft}/>
    <div className="mk-photo-actions">
      <button type="submit" className="btn btn-primary" disabled={busy}>{uiText("保存")}</button>
      <button type="button" className="btn" disabled={busy} onClick={() => setDraft(null)}>{uiText("取消")}</button>
    </div>
  </form>;
}

function Detail({ record, edit, back, uploading }: { record: MycoRecord<Entry>; edit: () => void; back: () => void; uploading: boolean }) {
  const [index, setIndex] = useState(0);
  const photos = pictures(record.id);
  const n = Math.min(index, photos.length - 1);
  const picture = photos[n];
  const entry = record.data;
  function cycle(direction: number) { if (photos.length) setIndex((n + direction + photos.length) % photos.length); }
  async function copyImage() {
    if (!picture || !canUseOriginals) return;
    try {
      if (!navigator.clipboard?.write || typeof ClipboardItem === "undefined") throw new Error();
      // Pass the promise into ClipboardItem so the click's activation survives
      // the image fetch on WebKit.
      const png = myco!.projectFiles!.get(picture.data.file).then(async blob => {
        if (blob.type === "image/png") return blob;
        const image = await createImageBitmap(blob);
        try {
          const canvas = document.createElement("canvas");
          canvas.width = image.width;
          canvas.height = image.height;
          const ctx = canvas.getContext("2d");
          if (!ctx) throw new Error();
          ctx.drawImage(image, 0, 0);
          return await new Promise<Blob>((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error()), "image/png"));
        } finally { image.close(); }
      });
      await navigator.clipboard.write([new ClipboardItem({ "image/png": png })]);
      kit.flash(uiText("已复制图片"));
    } catch { kit.flash(uiText("复制图片失败，请改用下载。")); }
  }
  async function download() {
    if (!picture) return;
    try { await myco!.projectFiles!.download(picture.data.file); kit.flash(uiText("已下载")); }
    catch { kit.flash(uiText("下载原图失败，请稍后重试。")); }
  }
  return <article className="mk-detail">
    <header className="mk-detail-header">
      <button className="btn" type="button" disabled={uploading} onClick={back}><ArrowLeft size={15}/>{uiText("返回")}</button>
      <h2>{entry.title}</h2>
      {kit.canEdit && <button className="btn" type="button" disabled={uploading} onClick={edit}>{uploading ? uiText("正在上传图片…") : canUseOriginals ? uiText("编辑或添加图片") : uiText("编辑")}</button>}
    </header>
    <div className="mk-meta"><span>{entry.date}</span>{entry.version && <span>{entry.version}</span>}<span>{statusText(entry.status)}</span>{entry.channels && <span>{entry.channels}</span>}</div>
    <div className="mk-detail-grid">
      <section aria-label={uiText("截图")}>
        <div className="mk-photo"><Preview record={picture} full/></div>
        {picture && <>
          <div className="mk-photo-actions">
            <button className="btn" type="button" aria-label={uiText("上一张")} disabled={photos.length < 2} onClick={() => cycle(-1)}><ChevronLeft size={14}/></button>
            <span>{n + 1} / {photos.length}</span>
            <button className="btn" type="button" aria-label={uiText("下一张")} disabled={photos.length < 2} onClick={() => cycle(1)}><ChevronRight size={14}/></button>
            <span className="mk-spacer"/>
            <button className="btn" type="button" disabled={!canUseOriginals || !picture.data.file} onClick={() => void copyImage()}><Copy size={14}/>{uiText("复制图片")}</button>
            <button className="btn" type="button" disabled={!canUseOriginals || !picture.data.file} onClick={() => void download()}><Download size={14}/>{uiText("下载原图")}</button>
          </div>
          <p className="mk-photo-meta">{languageText(picture.data.language)} · {themeText(picture.data.theme)} · {picture.data.width} × {picture.data.height}{picture.data.caption ? " · " + picture.data.caption : ""}</p>
          {kit.canEdit && <PictureEditor key={picture.id} record={picture}/>}
        </>}
      </section>
      <section className="mk-copy">
        {(["zh", "en"] as const).map(lang => {
          const pitch = entry["pitch_" + lang as "pitch_zh" | "pitch_en"];
          const body = entry["copy_" + lang as "copy_zh" | "copy_en"];
          return <div key={lang}><header><h3>{lang === "zh" ? uiText("中文") : uiText("英文")}</h3>
            <button className="btn" type="button" disabled={!body && !pitch} onClick={() => void copyText(body || pitch)}><Copy size={14}/>{uiText("复制配文")}</button></header>
            {pitch && <p>{pitch}</p>}{body && <p className="mk-long-copy">{body}</p>}
          </div>;
        })}
      </section>
    </div>
  </article>;
}

function App() {
  useKit(kit);
  const [id, setID] = useState("");
  const [editing, setEditing] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [search, setSearch] = useState("");
  const [tag, setTag] = useState("");
  const [state, setState] = useState("");
  const [quota, setQuota] = useState<{ used: number; max: number }>();
  const record = entries.get(id) || undefined;
  const refreshQuota = () => {
    if (canUseOriginals) void myco!.projectFiles!.info().then(setQuota).catch(() => undefined);
  };
  useEffect(refreshQuota, []);
  const tags = [...new Set(entries.rows.flatMap(row => row.data.tags || []))].sort((a, b) => a.localeCompare(b));
  const query = search.trim().toLocaleLowerCase();
  const visible = entries.rows.filter(({ data }) => (!tag || data.tags?.includes(tag)) && (!state || (data.status || "draft") === state) &&
    (!query || [data.title, data.date, data.version, data.pitch_zh, data.pitch_en, data.copy_zh, data.copy_en, data.channels, ...(data.tags || [])]
      .some(value => value?.toLocaleLowerCase().includes(query)))).sort((a, b) => (b.data.date || "").localeCompare(a.data.date || "") || b.id.localeCompare(a.id));

  async function submit(data: Entry, files: File[], options: PictureOptions, baseVersion: number) {
    if (quota && quota.used + files.length > quota.max) {
      kit.flash(uiText("项目文件空间不足，请清理后再上传。"));
      return;
    }
    const entryID = record?.id || uid("entry_");
    if (!await kit.track(saveEntry(entryID, data, baseVersion))) return;
    setID(entryID);
    setEditing(false);
    if (!files.length) return;
    setUploading(true);
    let completed = 0;
    try {
      for (const file of files) {
        await upload(entryID, file, options);
        completed++;
      }
      kit.flash(uiText("已上传 {0} 张图片", completed));
    } catch (error) {
      const filename = (error as { file?: string }).file;
      kit.flash(filename
        ? uiText("图片已上传，但记录未建成。请凭文件名补录：{0}", filename)
        : uiText("上传在第 {0} 张停止。请检查图片类型、大小和项目配额。", completed + 1));
    } finally {
      setUploading(false);
      refreshQuota();
    }
  }

  return <div className="mk-app">
    <header className="mk-header"><div><h1>{uiText("素材库")}</h1>{quota && <p className="mk-quota" data-near={quota.used >= quota.max - 10}>{uiText("项目文件已用 {0}/{1}", quota.used, quota.max)}{quota.used >= quota.max - 10 ? " · " + uiText("配额快满了") : ""}</p>}</div>
      {kit.canEdit && !id && !editing && <button className="btn btn-primary" onClick={() => setEditing(true)}><Plus size={16}/>{uiText("新增条目")}</button>}</header>
    {!kit.canEdit && <p className="mk-notice">{uiText("只能看")}</p>}
    {!canUseOriginals && <p className="mk-notice">{uiText("要上传、下载或复制原图，请在新版桌面 App 打开此项目页面。")}</p>}
    {editing ? <EntryEditor key={id} record={record} close={() => setEditing(false)} submit={submit}/> :
      record ? <Detail key={id} record={record} uploading={uploading} edit={() => setEditing(true)} back={() => setID("")}/> :
      <main className="mk-main">
        <div className="mk-filters">
          <label className="mk-search"><Search size={15}/><input className="inp" aria-label={uiText("搜索素材")} placeholder={uiText("搜索素材")} value={search} onChange={event => setSearch(event.target.value)}/></label>
          <select className="sel" aria-label={uiText("标签")} value={tag} onChange={event => setTag(event.target.value)}><option value="">{uiText("所有标签")}</option>
            {tags.map(value => <option value={value} key={value}>{value}</option>)}</select>
          <select className="sel" aria-label={uiText("状态")} value={state} onChange={event => setState(event.target.value)}><option value="">{uiText("所有状态")}</option>
            {statuses.map(value => <option value={value} key={value}>{statusText(value)}</option>)}</select>
        </div>
        <div className="mk-grid">
          {visible.map(item => <button className="mk-card" type="button" key={item.id} onClick={() => setID(item.id)}>
            <span className="mk-cover"><Preview record={pictures(item.id)[0]}/></span>
            <span className="mk-card-body"><strong>{item.data.title}</strong>
              <span>{language() === "zh" ? item.data.pitch_zh || item.data.pitch_en : item.data.pitch_en || item.data.pitch_zh}</span>
              <small>{[item.data.date, item.data.version, statusText(item.data.status), ...(item.data.tags || [])].filter(Boolean).join(" · ")}</small>
            </span>
          </button>)}
        </div>
        {visible.length === 0 && <p className="mk-empty">{entries.rows.length ? uiText("没有符合筛选条件的素材。") : uiText("还没有素材条目。")}</p>}
      </main>}
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App, initialize);
