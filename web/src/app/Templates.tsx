import { useCallback, useEffect, useState } from "react";
import { FileText, KanbanSquare, LayoutTemplate, Plus, Presentation, Table2 } from "lucide-react";
import { ApiError, appApi } from "@/appapi";
import type { Page, Template } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { openPage } from "@/app/pages";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * 模板 (templates.md): ready-made pages people and agents fill with data.
 * myco's own (表格, 多维表格, 文档, 幻灯片) are in every workspace; the workspace's
 * own are below them (agents make them with myco template init/publish).
 * Using one makes a page, opened in the page panel; the pages made from
 * templates are listed under them, to open again.
 */

const iconOf = (kind: string) => (kind === "doc" ? FileText : kind === "records" ? KanbanSquare : kind === "table" ? Table2 : kind === "slides" ? Presentation : LayoutTemplate);

export function TemplatesPage({ hubId, projects, now }: { hubId: string; projects: { name: string; title: string }[]; now: number }) {
  const [list, setList] = useState<Template[] | null>(null);
  const [made, setMade] = useState<Page[]>([]);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [using, setUsing] = useState<Template | null>(null);
  const load = useCallback(() => {
    appApi.templates(hubId).then((r) => { setList(r.templates || []); setUnsupported(false); }, (e) => {
      if (e instanceof ApiError && e.status === 404) setUnsupported(true);
      else setError(e);
      setList([]);
    });
    appApi.pages(hubId).then((r) => setMade((r.pages || []).filter((p) => p.template).sort((a, b) => b.updated_at.localeCompare(a.updated_at))), () => undefined);
  }, [hubId]);
  useEffect(() => { setList(null); load(); }, [load]);

  const official = (list || []).filter((t) => t.official);
  const own = (list || []).filter((t) => !t.official);
  const card = (t: Template) => {
    const Icon = iconOf(t.kind);
    return (
      <button key={t.id} type="button" className="au-template tp-card" onClick={() => setUsing(t)} data-testid={`template-${t.slug}`}
        title={L(`用「${t.name}」新建一个网页`, `New page from ${t.name}`)}>
        <span className={cn("au-icon", `tp-icon is-${t.kind}`)}><Icon /></span>
        <span className="au-template-text">
          <span className="au-template-name">{t.name}{t.official && <span className="tp-tag">{L("官方", "Official")}</span>}</span>
          {t.description && <span className="au-template-hint">{t.description}</span>}
          {!t.official && <span className="au-template-hint">{t.slug} · {t.version} · {t.publisher}</span>}
        </span>
      </button>
    );
  };

  return (
    <div className="pc au tp">
      <div className="pc-head">
        <p className="pc-intro">{L("做好的网页，你和 agent 一起往里填数据：你在网页里改，agent 用命令行读写同一份，谁改了对方马上看到。用模板新建一个，它就在右边的面板里打开。",
          "Ready-made pages you and agents fill together: you edit in the page, agents read and write the same data, and each sees the other's changes at once. New ones open in the page panel.")}</p>
      </div>
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty" data-testid="templates-unsupported">
          <div className="app-empty-icon"><LayoutTemplate /></div>
          <h2>{L("这个工作区还不能用模板", "No templates in this workspace yet")}</h2>
          <p>{L("它的 Hub 版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.20.5 或更新。", "Its hub is too old: run sudo myco upgrade on the server (v0.20.5 or later).")}</p>
        </div>
      ) : list === null ? null : (
        <>
          <h3 className="tp-section">{L("官方", "Official")}</h3>
          <div className="au-templates" data-testid="templates-official">{official.map(card)}</div>
          <h3 className="tp-section">{L("这个工作区的", "This workspace's")}</h3>
          {own.length ? <div className="au-templates" data-testid="templates-own">{own.map(card)}</div> : (
            <p className="tp-hint">{L("还没有。想要一个自己的（比如周报、面试评估表），跟 agent 说：「用 myco template init 帮我做一个周报模板，做好发布到工作区」。",
              "None yet. For one of your own (a weekly report, an interview sheet), ask an agent: \"make a weekly report template with myco template init and publish it\".")}</p>
          )}
          {made.length > 0 && <>
            <h3 className="tp-section">{L("用模板建的网页", "Pages made from templates")}</h3>
            <div className="pc-list tp-made" data-testid="templates-made">
              {made.map((p) => {
                const t = (list || []).find((x) => x.id === p.template?.id);
                const Icon = iconOf(t?.kind || "");
                return (
                  <button key={p.id} type="button" className="tp-made-row" onClick={() => openPage({ kind: "page", hubId, id: p.id })}>
                    <Icon />
                    <span className="tp-made-name">{p.name}</span>
                    <span className="tp-made-meta">{[p.template?.name, p.project ? projects.find((x) => x.name === p.project)?.title || p.project : "", timeAgo(p.updated_at, now)].filter(Boolean).join(" · ")}</span>
                  </button>
                );
              })}
            </div>
          </>}
        </>
      )}
      <UseTemplate hubId={hubId} template={using} projects={projects} onClose={() => setUsing(null)}
        onMade={(p) => { setUsing(null); load(); openPage({ kind: "page", hubId, id: p.id }); }} />
    </div>
  );
}

/** Making a page from a template: its name, where it goes, sample data. */
function UseTemplate({ hubId, template, projects, onClose, onMade }: {
  hubId: string;
  template: Template | null;
  projects: { name: string; title: string }[];
  onClose: () => void;
  onMade: (p: Page) => void;
}) {
  const [name, setName] = useState("");
  const [project, setProject] = useState("");
  const [sample, setSample] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { if (template) { setName(template.name); setError(null); setBusy(false); } }, [template]);
  const make = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!template || busy) return;
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.useTemplate(hubId, template.id, { name: name.trim() || template.name, ...(project ? { project } : {}), sample });
      onMade(r.page);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={!!template} onOpenChange={(o) => !o && !busy && onClose()} title={template ? L(`用「${template.name}」新建`, `New ${template.name}`) : ""}
      description={template?.description}>
      <form onSubmit={make} className="pin-edit">
        <label className="app-field">
          <span className="app-field-label">{L("叫什么", "Name")}</span>
          <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} autoFocus maxLength={80} data-testid="template-name" />
        </label>
        <label className="app-field">
          <span className="app-field-label">{L("放在哪", "Where")}</span>
          <select className="app-select-field" value={project} onChange={(e) => setProject(e.target.value)} data-testid="template-project">
            <option value="">{L("不放进项目（只有你和你的 agent 能看到）", "No project (you and your agents)")}</option>
            {projects.map((p) => <option key={p.name} value={p.name}>{L(`项目「${p.title}」：项目里的人和 agent 都能用`, `Project ${p.title}: everyone in it`)}</option>)}
          </select>
        </label>
        <label className="tp-sample">
          <input type="checkbox" checked={sample} onChange={(e) => setSample(e.target.checked)} />
          {L("先放一些示例数据，看看怎么用", "Start with some sample data")}
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy}><Plus />{busy ? L("正在新建…", "Making…") : L("新建", "Make it")}</button>
        </div>
      </form>
    </Modal>
  );
}
