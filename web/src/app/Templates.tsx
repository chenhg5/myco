import { useCallback, useEffect, useState } from "react";
import { CalendarCheck, CircleDot, Code2, FileText, Hash, KanbanSquare, Wrench, LayoutTemplate, ListChecks, Network, PenTool, Plus, Presentation, Table2, Vote, Wallet } from "lucide-react";
import { ApiError, appApi } from "@/appapi";
import type { Page, Template } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { openPage } from "@/app/pages";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * 应用 (templates.md; templates in the CLI and API): ready-made pages
 * people and agents use together, filling them with data.
 * myco's own (表格, 多维表格, 文档, 幻灯片) are in every workspace; the workspace's
 * own are below them (agents make them with myco template init/publish).
 * Using one makes a page, opened in the page panel; the pages made from
 * templates are listed under them, to open again.
 */

const BY_SLUG: Record<string, typeof FileText> = {
  "myco/whiteboard": PenTool, "myco/mindmap": Network, "myco/gomoku": CircleDot, "myco/sudoku": Hash,
  "myco/code": Code2, "myco/devtools": Wrench, "myco/todo": ListChecks, "myco/poll": Vote, "myco/ledger": Wallet, "myco/habit": CalendarCheck,
};
const iconOf = (kind: string, slug = "") => BY_SLUG[slug] || (kind === "doc" ? FileText : kind === "records" ? KanbanSquare : kind === "table" ? Table2 : kind === "slides" ? Presentation : LayoutTemplate);

/** The shelves official apps are on (their manifest's category; hubs
 *  before v0.20.6 have none: the office kinds go on the first). */
const SHELVES: [string, string, string][] = [
  ["office", "办公套件", "Office"], ["dev", "编程开发", "Development"], ["tools", "效率工具", "Tools"], ["life", "生活", "Life"], ["creative", "创意设计", "Creative"], ["game", "游戏", "Games"], ["other", "其他", "Other"],
];
const shelfOf = (t: Template) => t.category || (["table", "records", "doc", "slides"].includes(t.kind) ? "office" : "other");

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
    const Icon = iconOf(t.kind, t.slug);
    return (
      <button key={t.id} type="button" className="au-template tp-card" onClick={() => setUsing(t)} data-testid={`template-${t.slug}`}
        title={L(`新建一份${t.name}`, `New ${t.name}`)}>
        <span className={cn("au-icon", `tp-icon is-${t.kind} on-${shelfOf(t)}`)}><Icon /></span>
        <span className="au-template-text">
          <span className="au-template-name">{t.name}</span>
          {t.description && <span className="au-template-hint">{t.description}</span>}
          {!t.official && <span className="au-template-hint">{t.slug} · {t.version} · {t.publisher}</span>}
        </span>
      </button>
    );
  };

  return (
    <div className="pc au tp">
      <div className="pc-head">
        <p className="pc-intro">{L("你和 agent 一起用的应用：你在页面里改，agent 用命令行读写同一份，谁改了对方马上看到。新建一份，它就在右边的面板里打开。",
          "Apps you and agents use together: you edit in the page, agents read and write the same data, and each sees the other's changes at once. New ones open in the page panel.")}</p>
      </div>
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty" data-testid="templates-unsupported">
          <div className="app-empty-icon"><LayoutTemplate /></div>
          <h2>{L("这个工作区还不能用应用", "No apps in this workspace yet")}</h2>
          <p>{L("它的 Hub 版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.20.5 或更新。", "Its hub is too old: run sudo myco upgrade on the server (v0.20.5 or later).")}</p>
        </div>
      ) : list === null ? null : (
        <>
          <div data-testid="templates-official">
            {SHELVES.map(([key, zh, en]) => {
              const on = official.filter((t) => shelfOf(t) === key);
              return on.length ? (
                <section key={key} className="tp-shelf" data-shelf={key}>
                  <h3 className="tp-section">{L(zh, en)}</h3>
                  <div className="au-templates">{on.map(card)}</div>
                </section>
              ) : null;
            })}
          </div>
          <h3 className="tp-section">{L("这个工作区的应用", "This workspace's apps")}</h3>
          {own.length ? <div className="au-templates" data-testid="templates-own">{own.map(card)}</div> : (
            <p className="tp-hint">{L("还没有。想要一个自己的（比如周报、面试评估表），跟 agent 说：「用 myco template init 帮我做一个周报应用，做好发布到工作区」。",
              "None yet. For one of your own (a weekly report, an interview sheet), ask an agent: \"make a weekly report app with myco template init and publish it\".")}</p>
          )}
          {made.length > 0 && <>
            <h3 className="tp-section">{L("最近建的", "Made recently")}</h3>
            <div className="pc-list tp-made" data-testid="templates-made">
              {made.map((p) => {
                const t = (list || []).find((x) => x.id === p.template?.id);
                const Icon = iconOf(t?.kind || "", t?.slug);
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
    <Modal open={!!template} onOpenChange={(o) => !o && !busy && onClose()} title={template ? L(`新建一份${template.name}`, `New ${template.name}`) : ""}
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
