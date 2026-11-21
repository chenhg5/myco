import { useCallback, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { IntroCard, introOf } from "@/app/Onboard";
import { FileText, KanbanSquare, LayoutTemplate, Plus, Presentation, RefreshCw, Search, Sparkles, Table2 } from "lucide-react";
import { ApiError, appApi } from "@/appapi";
import type { Page, Template } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { openPage } from "@/app/pages";
import { CardsSkeleton, PageHero, SectionHead, SectionSkeleton } from "@/app/PageHero";
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

const iconOf = (kind: string) => (kind === "doc" ? FileText : kind === "records" ? KanbanSquare : kind === "table" ? Table2 : kind === "slides" ? Presentation : LayoutTemplate);
/** An app's own icon (its SVG, as a mask in the text colour), else one by kind. */
function AppIcon({ t }: { t?: Template }) {
  if (t?.icon) return <span className="tp-svg" style={{ ["--icon" as string]: `url("data:image/svg+xml;utf8,${encodeURIComponent(t.icon)}")` }} aria-hidden="true" />;
  const Icon = iconOf(t?.kind || "");
  return <Icon />;
}

/** The shelves official apps are on (their manifest's category; hubs
 *  before v0.20.6 have none: the office kinds go on the first). */
const SHELVES: [string, string, string][] = [
  ["office", "办公套件", "Office"], ["dev", "编程开发", "Development"], ["tools", "效率工具", "Tools"], ["life", "生活", "Life"], ["creative", "创意设计", "Creative"], ["game", "游戏", "Games"], ["other", "其他", "Other"],
];
const shelfOf = (t: Template) => t.category || (["table", "records", "doc", "slides"].includes(t.kind) ? "office" : "other");

/** What each hub's page showed last: back on the page, it shows that at
 *  once and refreshes behind it (skeletons only the first time). */
const seen = new Map<string, { list?: Template[]; made?: Page[] }>();

export function TemplatesPage({ hubId, projects, now }: { hubId: string; projects: { name: string; title: string }[]; now: number }) {
  const [list, setList] = useState<Template[] | null>(() => seen.get(hubId)?.list ?? null);
  const [made, setMade] = useState<Page[]>(() => seen.get(hubId)?.made ?? []);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [using, setUsing] = useState<Template | null>(null);
  const [sync, setSync] = useState<{ busy?: boolean; text?: string }>({});
  // myco's own apps come on their own (hubs from v0.20.7 fetch them daily):
  // fetched now on request.
  const syncNow = () => {
    setSync({ busy: true });
    appApi.syncApps(hubId).then((r) => {
      setSync({ text: r.error ? L(`没拉到新的（${r.error}）`, `Could not fetch (${r.error})`)
        : r.updated?.length ? L(`更新了 ${r.updated.length} 个：${r.updated.join("、")}`, `Updated ${r.updated.length}: ${r.updated.join(", ")}`)
        : L(`已是最新，${r.apps} 个官方应用`, `Up to date: ${r.apps} official apps`) });
      load();
    }, (e) => setSync({ text: e instanceof ApiError && e.status === 404 ? L("这个 Hub 还不会自己更新官方应用：升级到 v0.20.7 或更新", "This hub cannot update its official apps yet: upgrade it to v0.20.7 or later") : e instanceof Error ? e.message : String(e) }));
  };
  const load = useCallback(() => {
    appApi.templates(hubId).then((r) => { seen.set(hubId, { ...seen.get(hubId), list: r.templates || [] }); setList(r.templates || []); setUnsupported(false); }, (e) => {
      if (e instanceof ApiError && e.status === 404) setUnsupported(true);
      else setError(e);
      setList([]);
    });
    appApi.pages(hubId).then((r) => {
      const m = (r.pages || []).filter((p) => p.template).sort((a, b) => b.updated_at.localeCompare(a.updated_at));
      seen.set(hubId, { ...seen.get(hubId), made: m });
      setMade(m);
    }, () => undefined);
  }, [hubId]);
  useEffect(() => { setList(seen.get(hubId)?.list ?? null); setMade(seen.get(hubId)?.made ?? []); load(); }, [hubId, load]);

  const [shelf, setShelf] = useState("all");
  const [q, setQ] = useState("");
  const [allMade, setAllMade] = useState(false);
  const match = (t: Template) => !q.trim() || `${t.name} ${t.description || ""} ${t.slug}`.toLowerCase().includes(q.trim().toLowerCase());
  const official = (list || []).filter((t) => t.official && match(t));
  const own = (list || []).filter((t) => !t.official && match(t));
  const shelves = SHELVES.filter(([key]) => (list || []).some((t) => t.official && shelfOf(t) === key));
  const card = (t: Template) => (
    <button key={t.id} type="button" className="tp-card" onClick={() => setUsing(t)} data-testid={`template-${t.slug}`}
      title={L(`新建一份${t.name}`, `New ${t.name}`)}>
      <span className={cn("tp-icon", `is-${t.kind} on-${shelfOf(t)}`)}><AppIcon t={t} /></span>
      <span className="tp-card-text">
        <span className="tp-card-name">{t.name}</span>
        {t.description && <span className="tp-card-desc">{t.description}</span>}
        {!t.official && <span className="tp-card-meta">{t.slug} · {t.version} · {t.publisher}</span>}
      </span>
      <span className="tp-card-new"><Plus />{L("新建", "New")}</span>
    </button>
  );
  const shownMade = allMade ? made : made.slice(0, 6);
  const nothing = !!list && !official.length && !own.length;

  return (
    <div className="pc tp">
      <PageHero title={L("应用", "Apps")}
        subtitle={L("你和 agent 一起用的表格、文档、白板和小游戏，两边的改动实时同步。", "Tables, documents, boards and games you and agents use together, changes shared as they happen.")}>
        {!unsupported && (
          <>
            <label className="tp-search"><Search /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder={L("找应用", "Find an app")} aria-label={L("找应用", "Find an app")} /></label>
            <button type="button" className="app-btn app-btn-plain tp-sync-btn" onClick={syncNow} disabled={sync.busy} data-testid="apps-sync"
              title={L("检查官方应用更新", "Check for app updates")} aria-label={L("检查官方应用更新", "Check for app updates")}>
              <RefreshCw className={cn(sync.busy && "is-spinning")} />
            </button>
          </>
        )}
      </PageHero>
      {!unsupported && <IntroCard id="apps" {...introOf("apps")} />}
      {sync.text && <p className="tp-sync"><span className="tp-sync-note">{sync.text}</span><button type="button" className="tp-sync-x" aria-label={L("知道了", "Dismiss")} onClick={() => setSync({})}>×</button></p>}
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty" data-testid="templates-unsupported">
          <div className="app-empty-icon"><LayoutTemplate /></div>
          <h2>{L("这个工作区还不能用应用", "No apps in this workspace yet")}</h2>
          <p>{L("它的 Hub 版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.20.5 或更新。", "Its hub is too old: run sudo myco upgrade on the server (v0.20.5 or later).")}</p>
        </div>
      ) : list === null ? (
        <>
          <div className="tp-chips" aria-hidden>{[64, 76, 76, 64, 48].map((w, i) => <span key={i} className="skel tp-chip-skel" style={{ width: w }} />)}</div>
          <SectionSkeleton />
          <CardsSkeleton count={6} />
        </>
      ) : (
        <>
          {made.length > 0 && !q && (
            <>
              <SectionHead title={L("最近建的", "Made recently")} count={made.length}>
                {made.length > 6 && <button type="button" className="tp-more" onClick={() => setAllMade(!allMade)}>{allMade ? L("收起", "Fewer") : L("全部", "All")}</button>}
              </SectionHead>
              <div className="tp-made" data-testid="templates-made">
                {shownMade.map((p) => {
                  const t = (list || []).find((x) => x.id === p.template?.id);
                  return (
                    <button key={p.id} type="button" className="tp-made-row" onClick={() => openPage({ kind: "page", hubId, id: p.id })} title={p.name}>
                      <span className="tp-icon is-small"><AppIcon t={t} /></span>
                      <span className="tp-made-text">
                        <span className="tp-made-name">{p.name}</span>
                        <span className="tp-made-meta">{[p.template?.name, p.project ? projects.find((x) => x.name === p.project)?.title || p.project : "", timeAgo(p.updated_at, now)].filter(Boolean).join(" · ")}</span>
                      </span>
                      {p.template?.latest && <span className="tp-made-new" title={L("有新版本", "A new version")} />}
                    </button>
                  );
                })}
              </div>
            </>
          )}
          <div className="tp-chips" role="tablist" aria-label={L("分类", "Shelves")}>
            {[["all", "全部", "All"] as [string, string, string], ...shelves, ["own", "这个工作区的", "This workspace's"] as [string, string, string]].map(([key, zh, en]) => (
              <button key={key} type="button" role="tab" aria-selected={shelf === key} className={cn("tp-chip", shelf === key && "is-on")} onClick={() => setShelf(key)}>{L(zh, en)}</button>
            ))}
          </div>
          {nothing && <p className="tp-hint">{L(`没有找到「${q}」`, `Nothing matches "${q}"`)}</p>}
          {shelf !== "own" && (
            <div data-testid="templates-official">
              {(shelf === "all" ? shelves : shelves.filter(([key]) => key === shelf)).map(([key, zh, en]) => {
                const on = official.filter((t) => shelfOf(t) === key);
                return on.length ? (
                  <section key={key} className="tp-shelf" data-shelf={key}>
                    {shelf === "all" && <SectionHead title={L(zh, en)} count={on.length} />}
                    <div className="tp-grid">{on.map(card)}</div>
                  </section>
                ) : null;
              })}
            </div>
          )}
          {(shelf === "all" || shelf === "own") && (
            <section className="tp-shelf">
              <SectionHead title={L("这个工作区的应用", "This workspace's apps")} count={own.length} />
              {own.length ? <div className="tp-grid" data-testid="templates-own">{own.map(card)}</div> : (
                <div className="tp-own-empty">
                  <span className="tp-icon"><Sparkles /></span>
                  <p>{L("还没有自己的应用。想要一个（比如周报、面试评估表），跟 agent 说：「用 myco template init 帮我做一个周报应用，做好发布到工作区」。",
                    "None of your own yet. For one (a weekly report, an interview sheet), ask an agent: \"make a weekly report app with myco template init and publish it\".")}</p>
                </div>
              )}
            </section>
          )}
        </>
      )}
      <UseTemplate hubId={hubId} template={using} projects={projects} onClose={() => setUsing(null)}
        first={!!using && !made.some((p) => p.template?.id === using.id)}
        onMade={(p) => { setUsing(null); load(); openPage({ kind: "page", hubId, id: p.id }); }} />
    </div>
  );
}

/** Making a page from a template: its name and where it goes. The first
 *  one of a kind in this workspace comes with the template's sample data
 *  (to see how it is used); later ones start empty. Not asked: every app
 *  works empty, and a choice here is one more thing to read. */
function UseTemplate({ hubId, template, projects, first, onClose, onMade }: {
  hubId: string;
  template: Template | null;
  projects: { name: string; title: string }[];
  /** none of this kind made here yet: it gets the sample data */
  first: boolean;
  onClose: () => void;
  onMade: (p: Page) => void;
}) {
  const [name, setName] = useState("");
  const [project, setProject] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { if (template) { setName(template.name); setError(null); setBusy(false); } }, [template]);
  const make = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!template || busy) return;
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.useTemplate(hubId, template.id, { name: name.trim() || template.name, ...(project ? { project } : {}), sample: first });
      onMade(r.page);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={!!template} onOpenChange={(o) => !o && !busy && onClose()} title={template ? L(`新建一份${template.name}`, `New ${template.name}`) : ""}
      description={template?.description} wide className="tp-use">
      {template && <Shots hubId={hubId} template={template} />}
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
        {first && <p className="app-small-note tp-sample-note" data-testid="template-sample-note">{L("这是你在这个工作区的第一份，会带上一些示例内容，看看怎么用；不要的直接删掉。", "Your first one here comes with some sample content, to show how it is used; delete what you don't need.")}</p>}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy}><Plus />{busy ? L("正在新建…", "Making…") : L("新建", "Make it")}</button>
        </div>
      </form>
    </Modal>
  );
}

/** Whether the App is dark now (it can change while a dialog is open). */
function useDark() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    const o = new MutationObserver(() => setDark(document.documentElement.classList.contains("dark")));
    o.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => o.disconnect();
  }, []);
  return dark;
}

/** What an app looks like: its screenshots (the dark ones in the dark),
 *  one at a time, with dots and arrows when there are more. Nothing when
 *  it has none (an older hub, or an app made without). */
function Shots({ hubId, template }: { hubId: string; template: Template }) {
  const [shots, setShots] = useState(template.shots || []);
  const [i, setI] = useState(0);
  const [ready, setReady] = useState<Record<string, boolean>>({});
  const dark = useDark();
  useEffect(() => {
    setI(0);
    setShots(template.shots || []);
    // (a workspace's own app lists its screenshots when got on its own)
    if (template.shots || template.official) return;
    let live = true;
    appApi.template(hubId, template.id).then((t) => { if (live) setShots(t.shots || []); }, () => undefined);
    return () => { live = false; };
  }, [hubId, template.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const url = (s: { src: string; dark?: string }) => appApi.templateShotURL(hubId, template.id, dark && s.dark ? s.dark : s.src);
  // the others load while the first is looked at
  useEffect(() => { for (const s of shots) { const im = new Image(); im.src = url(s); } }, [shots, dark]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!shots.length) return null;
  const cur = shots[Math.min(i, shots.length - 1)];
  const src = url(cur);
  const go = (d: number) => setI((x) => (x + d + shots.length) % shots.length);
  return (
    <div className="tp-shots" data-testid="template-shots">
      <div className={cn("tp-shot", !ready[src] && "is-loading")}
        onKeyDown={(e) => { if (e.key === "ArrowLeft") { e.preventDefault(); go(-1); } else if (e.key === "ArrowRight") { e.preventDefault(); go(1); } }}>
        <img key={src} src={src} alt={L(`${template.name}的样子（第 ${i + 1} 张）`, `${template.name}, screenshot ${i + 1}`)} draggable={false}
          onLoad={() => setReady((r) => ({ ...r, [src]: true }))} onError={() => setReady((r) => ({ ...r, [src]: true }))} />
        {shots.length > 1 && (
          <>
            <button type="button" className="tp-shot-nav is-prev" aria-label={L("上一张", "Previous")} onClick={() => go(-1)}><ChevronLeft /></button>
            <button type="button" className="tp-shot-nav is-next" aria-label={L("下一张", "Next")} onClick={() => go(1)}><ChevronRight /></button>
          </>
        )}
      </div>
      {shots.length > 1 && (
        <div className="tp-shot-dots" role="tablist" aria-label={L("截图", "Screenshots")}>
          {shots.map((s, k) => (
            <button key={s.src} type="button" role="tab" aria-selected={k === i} aria-label={L(`第 ${k + 1} 张`, `Screenshot ${k + 1}`)}
              className={cn("tp-shot-dot", k === i && "is-on")} onClick={() => setI(k)} />
          ))}
        </div>
      )}
    </div>
  );
}
