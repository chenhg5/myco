// Read an agent's research as a shopping decision. Shared notes and shortlists
// are separate from the research, so a refresh cannot erase a person's choices.
import "../_lib/sheet.css";
import "./price-compare.css";
import { useEffect, useRef, useState } from "react";
import { ArrowDownUp, ArrowUpRight, Bookmark, Check, Clock3, Copy, Download, ExternalLink, Search, ShieldCheck, ShoppingBag, Sparkles, X } from "lucide-react";
import { mount, Toast, useKit } from "../_lib/appkit";
import { myco, save } from "../_lib/myco";
import { publicAppLink, subscribeAppRoute } from "../_lib/page-route";
import { locale, t as uiText } from "../_lib/i18n";
import { comparisons, decisions, kit } from "./model";
import { saveNote, toggleSaved } from "./edit";
import { exclusion, formatMoney, promptFor, ranked, routeID, safeURL, total, validComparison, type Comparison, type Offer, type Exclusion } from "./engine";

const bridge = myco as typeof myco & { send?: (message: string) => Promise<unknown> };
const price = (n: number | null, currency: string) => formatMoney(n, currency, locale());
const labels = () => ({ researching: uiText("正在查价"), ready: uiText("查询完成"), blocked: uiText("需要补充来源"), sample: uiText("示例") });
function reason(code: Exclusion | null): string {
  return ({ sample: uiText("示例，非实时报价"), match: uiText("规格尚未一致"), currency: uiText("币种不同"), price: uiText("价格待确认"), shipping: uiText("运费待确认"), stock: uiText("库存未确认或无货"), conditions: uiText("有专属条件，单独比较"), future: uiText("查询时间异常"), expired: uiText("优惠已过期"), stale: uiText("超过 24 小时，请重查") })[code || "sample"];
}
function when(value: string) { return new Intl.DateTimeFormat(locale(), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
async function copy(value: string) { await navigator.clipboard.writeText(value); kit.flash(uiText("已复制")); }

function Request({ current, id, close }: { current?: Comparison; id?: string; close: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const [query, setQuery] = useState(current?.query || ""), [spec, setSpec] = useState(current?.spec || "");
  const [busy, setBusy] = useState(false), [error, setError] = useState("");
  const [manual, setManual] = useState("");
  useEffect(() => { ref.current?.showModal(); }, []);
  const submit = async () => {
    if (!query.trim() || busy) return;
    setBusy(true); setError("");
    const message = promptFor(kit.page, query, spec, id);
    try {
      if (typeof bridge?.send === "function") { await bridge.send(message); kit.flash(uiText("已发给 agent，完成后会更新这个页面")); close(); }
      else { try { await copy(message); close(); } catch { setManual(message); } }
    } catch { setError(uiText("没发出去，请重试或复制给 agent")); setManual(message); }
    finally { setBusy(false); }
  };
  return <dialog ref={ref} className="pc-dialog" onCancel={e => { e.preventDefault(); if (!busy) close(); }} onClick={e => { if (e.target === e.currentTarget && !busy) close(); }}>
    <header><div><span className="pc-eyebrow">{uiText("让 agent 做功课")}</span><h2>{uiText("你想买什么？")}</h2></div><button className="icon-btn" aria-label={uiText("关闭")} title={uiText("关闭")} disabled={busy} onClick={close}><X className="ico" /></button></header>
    <label>{uiText("商品或型号")}<input autoFocus className="inp" maxLength={500} value={query} placeholder={uiText("例如：索尼 WH-1000XM6")} onChange={e => setQuery(e.target.value)} /></label>
    <label>{uiText("规格与购买要求")}<textarea maxLength={1000} value={spec} placeholder={uiText("颜色、容量、件数、预算、收货地区，以及是否接受会员或补贴价格…")} onChange={e => setSpec(e.target.value)} /></label>
    <p className="pc-muted">{uiText("agent 直接调用买手商品 API 查价。首次使用需要在执行端配置自己的买手授权信息；凭证不保存在页面。")}</p>
    {error && <p role="alert" className="pc-error">{error}</p>}
    {manual && <label>{uiText("复制以下内容发给 agent")}<textarea className="pc-prompt" readOnly value={manual} onFocus={e => e.target.select()} /></label>}
    <footer><button className="btn" disabled={busy} onClick={close}>{uiText("关闭")}</button><button className="btn btn-primary" disabled={!query.trim() || busy} onClick={() => void submit()}>{busy ? uiText("正在发送…") : typeof bridge?.send === "function" ? uiText("请 agent 查价") : uiText("复制查价需求")}</button></footer>
  </dialog>;
}

function Note({ id }: { id: string }) {
  const row = decisions.get(id), note = typeof row?.data.note === "string" ? row.data.note : "";
  const [draft, setDraft] = useState(note), [version, setVersion] = useState(row?.version || 0), [dirty, setDirty] = useState(false), [error, setError] = useState("");
  useEffect(() => { if (!dirty) { setDraft(note); setVersion(row?.version || 0); } }, [note, row?.version, dirty]);
  return <section className="pc-note"><header><h2>{uiText("我的购买考虑")}</h2><span className="pc-muted">{uiText("与这个页面的成员共享")}</span></header><textarea aria-label={uiText("购买笔记")} maxLength={4000} placeholder={uiText("记录你在意的售后、颜色和购买时机…")} value={draft} onChange={e => { setDraft(e.target.value); setDirty(true); }} />
    {error && <p className="pc-error" role="alert">{error}<button className="tool" onClick={() => { setDraft(note); setDirty(false); setError(""); }}>{uiText("重新载入")}</button></p>}
    {dirty && <button className="btn" disabled={!!kit.pending} onClick={() => { void kit.track(saveNote(id, draft, version)).then(result => { if (result) { setDirty(false); setError(""); } else setError(uiText("笔记未保存。若有其他人修改，请重新载入再编辑。")); }); }}>{uiText("保存笔记")}</button>}
  </section>;
}

function OfferCard({ offer: o, comparison: c, id, best, now }: { offer: Offer; comparison: Comparison; id: string; best: boolean; now: number }) {
  const stored = decisions.get(id)?.data.saved, saved = Array.isArray(stored) && stored.includes(o.id);
  const excluded = exclusion(o, c, now);
  return <article className={`pc-offer ${best ? "pc-best" : ""}`} data-offer={o.id}>
    <div className="pc-product"><ShoppingBag aria-hidden="true" /><span>{o.platform}</span></div>
    <div className="pc-offer-body"><div className="pc-offer-meta"><span>{o.platform} · {o.shop || uiText("店铺待确认")}</span>{best && <span className="pc-badge"><Check className="ico" />{uiText("本次可比低价")}</span>}</div>
      <h3>{o.title}</h3><p className="pc-spec">{o.spec || uiText("规格待确认")}</p>
      <div className="pc-tags"><span>{o.match === "exact" ? uiText("同款同规格") : o.match === "different" ? uiText("不同规格") : uiText("规格待核对")}</span><span>{o.eligibility === "public" ? uiText("公开优惠") : uiText("需核对资格")}</span></div>
      <div className="pc-terms">{o.coupon && <p><strong>{uiText("优惠")}</strong>{o.coupon}</p>}{o.conditions && <p><strong>{uiText("条件")}</strong>{o.conditions}</p>}{o.cashback && <p><strong>{uiText("返利")}</strong>{o.cashback} · {uiText("未从价格中扣除")}</p>}{o.affiliate && <p>{uiText("购买链接含推广归属")}</p>}</div>
      <div className="pc-evidence"><Clock3 className="ico" /><span>{when(o.checked_at)}</span>{c.status === "sample" ? <span>{o.source}</span> : <a href={safeURL(o.url)} target="_blank" rel="noopener noreferrer">{o.source}<ExternalLink className="ico" /></a>}</div>
    </div>
    <div className="pc-offer-price"><span className="pc-muted">{uiText("商品价")}</span><strong>{price(o.price, o.currency)}</strong><small>{o.shipping === null ? uiText("运费待确认") : o.shipping === 0 ? uiText("包邮") : uiText("运费 {0}", price(o.shipping, o.currency))}</small>
      {total(o) !== null && <small>{uiText("含运费 {0}", price(total(o), o.currency))}</small>}
      {excluded && <span className="pc-excluded">{reason(excluded)}</span>}
      <div className="pc-offer-actions"><button className={`icon-btn ${saved ? "is-saved" : ""}`} aria-label={saved ? uiText("取消收藏") : uiText("收藏报价")} title={saved ? uiText("取消收藏") : uiText("收藏报价")} aria-pressed={saved} disabled={!!kit.pending} onClick={() => void toggleSaved(id, o.id)}><Bookmark className="ico" fill={saved ? "currentColor" : "none"} /></button>{c.status !== "sample" && o.purchase_code && <button className="btn" onClick={() => void copy(o.purchase_code!).catch(() => kit.flash(uiText("复制失败，请重试")))}>{uiText("复制购买口令")}</button>}{c.status === "sample" ? <button className="btn" disabled>{uiText("示例")}</button> : <a className="btn" href={safeURL(o.url)} target="_blank" rel="noopener noreferrer">{o.link_kind === "evidence" ? uiText("查看来源") : uiText("查看商品")}<ArrowUpRight className="ico" /></a>}</div>
    </div>
  </article>;
}

function App() {
  useKit(kit);
  const [selected, setSelected] = useState(routeID(location.hash)), [search, setSearch] = useState("");
  const [platform, setPlatform] = useState(""), [onlySaved, setOnlySaved] = useState(false), [sort, setSort] = useState("rank");
  const [request, setRequest] = useState(false), [refresh, setRefresh] = useState(false), [pageURL, setPageURL] = useState("");
  const [now, setNow] = useState(Date.now());
  useEffect(() => { void myco!.ready.then(ctx => setPageURL(ctx.page.url || "")); return subscribeAppRoute(() => setSelected(routeID(location.hash))); }, []);
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), 30000); return () => clearInterval(t); }, []);
  const records = comparisons.rows.filter(r => validComparison(r.data));
  const ordered = [...records].sort((a, b) => Date.parse(b.data.updated_at) - Date.parse(a.data.updated_at));
  const row = selected ? ordered.find(r => r.id === selected) : ordered[0], c = row?.data;
  const id = row?.id || "", ranks = c ? ranked(c, now) : [], winner = c?.status === "ready" ? ranks[0] : undefined;
  const validPlatforms = c ? [...new Set(c.offers.map(o => o.platform))] : [];
  const saved = decisions.get(id)?.data.saved;
  const offers = c?.offers.filter(o => (!platform || o.platform === platform) && (!onlySaved || (Array.isArray(saved) && saved.includes(o.id))) && (!search || `${o.title} ${o.spec} ${o.shop} ${o.platform}`.toLowerCase().includes(search.toLowerCase()))) || [];
  offers.sort((a, b) => sort === "recent" ? Date.parse(b.checked_at) - Date.parse(a.checked_at) : c?.status === "sample" ? 0 : (exclusion(a, c!, now) === null ? 0 : 1) - (exclusion(b, c!, now) === null ? 0 : 1) || (a.currency === b.currency ? (total(a) ?? Infinity) - (total(b) ?? Infinity) : a.currency.localeCompare(b.currency)));
  const select = (value: string) => { setSelected(value); location.hash = `compare?id=${encodeURIComponent(value)}`; setSearch(""); setPlatform(""); setOnlySaved(false); };
  const exportData = async () => { if (!c) return; try { await save("shopping-comparison.json", JSON.stringify({ comparison: c, decision: decisions.get(id)?.data || null }, null, 2), "application/json"); kit.flash(uiText("已保存到下载文件夹")); } catch { kit.flash(uiText("导出失败，请重试")); } };
  return <div className="pc-app"><header className="pc-top"><div className="pc-brand"><ShoppingBag className="ico" /><strong>{uiText("购物比价")}</strong><span>/ myco</span></div><span className="pc-top-caption">{uiText("agent 查，你来选。")}</span><div className="pc-top-actions">{c && <button className="btn pc-export" aria-label={uiText("导出")} onClick={() => void exportData()}><Download className="ico" /><span>{uiText("导出")}</span></button>}<button className="btn btn-primary" onClick={() => { setRefresh(false); setRequest(true); }}><Search className="ico" />{uiText("新建比价")}</button></div></header>
    <div className="pc-workspace"><aside className="pc-sidebar"><div className="pc-sidebar-heading">{uiText("我的比价")}<span>{records.length}</span></div><nav aria-label={uiText("比价列表")}>{ordered.map(r => <button key={r.id} className={id === r.id ? "active" : ""} onClick={() => select(r.id)}><ShoppingBag className="ico" /><span><strong>{r.data.title}</strong><small>{labels()[r.data.status]} · {r.data.offers.length} {uiText("条报价")}</small></span></button>)}</nav><div className="pc-sidebar-foot"><ShieldCheck className="ico" /><span>{uiText("每个价格，都有出处。")}</span></div></aside>
    <main className="pc-main">
      {comparisons.rows.length > records.length && <p role="alert" className="pc-error">{uiText("有比价内容格式不完整，请让 agent 按页面说明修正。")}</p>}
      {selected && !row ? <section className="pc-empty"><h1>{uiText("没有找到这份比价")}</h1><p>{uiText("内容可能已被移除，从左侧选择另一份比价。")}</p><button className="btn" onClick={() => { setSelected(""); location.hash = ""; }}>{uiText("返回列表")}</button></section> : c ? <>
        <section className="pc-heading"><div><span className="pc-eyebrow">{uiText("买之前，先看清楚。")}</span><h1>{c.title}</h1><p>{c.spec || c.query}</p><div className="pc-heading-meta"><span className={`pc-status ${c.status}`}>{labels()[c.status]}</span><span>{uiText("更新于 {0}", when(c.updated_at))}</span></div></div><button className="tool" disabled={!pageURL} title={!pageURL ? uiText("此客户端尚未提供页面公开链接") : uiText("复制这份比价链接")} onClick={() => { try { void copy(publicAppLink(pageURL, `#compare?id=${encodeURIComponent(id)}`)).catch(() => kit.flash(uiText("复制失败，请从聊天中打开页面链接"))); } catch { kit.flash(uiText("复制失败，请从聊天中打开页面链接")); } }}><Copy className="ico" />{uiText("分享比价")}</button></section>
        {c.status === "sample" && <div className="pc-notice">{uiText("这是界面示例，价格与店铺均为虚构，不可用于购买。请新建比价，让 agent 查询真实商品。")}</div>}
        <section className="pc-summary"><div className="pc-summary-main"><span className="pc-eyebrow"><Sparkles className="ico" />{uiText("这次怎么选")}</span><h2>{winner ? uiText("本次可比低价 {0}", price(total(winner), c.currency)) : c.status === "sample" ? uiText("让价格和条件，一起摆上桌面。") : uiText("先看条件，再做决定。")}</h2><p>{c.summary || uiText("agent 会在这里整理结论与尚未确认的条件。")}</p><button className="tool" onClick={() => { setRefresh(true); setRequest(true); }}>{uiText("请 agent 重新查价")}<ArrowUpRight className="ico" /></button></div><div className="pc-summary-stat"><span>{uiText("来源平台")}</span><strong>{validPlatforms.length}</strong><small>{uiText("保留来源与查询时间")}</small></div><div className="pc-summary-stat"><span>{uiText("可直接比较")}</span><strong>{ranks.length}<small>/ {c.offers.length}</small></strong><small>{uiText("同规格 · 同币种 · 已知运费")}</small></div></section>
        <div className="pc-controls"><label className="pc-search"><Search className="ico" /><input aria-label={uiText("搜索报价")} placeholder={uiText("搜索商品、店铺或规格")} value={search} onChange={e => setSearch(e.target.value)} /></label><select className="sel" aria-label={uiText("筛选平台")} value={platform} onChange={e => setPlatform(e.target.value)}><option value="">{uiText("全部平台")}</option>{validPlatforms.map(p => <option key={p}>{p}</option>)}</select><button className={`tool ${onlySaved ? "is-on" : ""}`} aria-pressed={onlySaved} onClick={() => setOnlySaved(!onlySaved)}><Bookmark className="ico" />{uiText("已收藏")}</button><label className="pc-sort"><ArrowDownUp className="ico" /><select aria-label={uiText("报价排序")} value={sort} onChange={e => setSort(e.target.value)}><option value="rank">{uiText("可比价格优先")}</option><option value="recent">{uiText("最近查询优先")}</option></select></label></div>
        <div className="pc-result-count">{uiText("{0} 条报价", offers.length)}<span>{uiText("仅比较已核实的条件；实际支付以平台结算页为准。")}</span></div>
        <div className="pc-offers">{offers.map(o => <OfferCard key={o.id} offer={o} comparison={c} id={id} best={winner?.id === o.id} now={now} />)}</div>
        {!offers.length && <section className="pc-no-results"><Search /><h2>{c.status === "researching" ? uiText("agent 正在收集报价") : c.status === "blocked" ? uiText("暂时没有可靠报价") : uiText("没有符合筛选的报价")}</h2><p>{uiText("可以调整筛选，或请 agent 补充可验证的商品来源。")}</p></section>}
        <Note key={id} id={id} /><footer className="pc-footnote">{uiText("本次低价仅限已查询且条件一致的报价；不代表全网最低。超过 24 小时的报价不参与排名。")}</footer>
      </> : <section className="pc-empty"><ShoppingBag /><span className="pc-eyebrow">{uiText("少做功课，多一点确定。")}</span><h1>{uiText("把挑选，留给自己。")}</h1><p>{uiText("告诉 agent 想买什么。它整理价格、优惠与来源，你在这里看清差别，再决定。")}</p><button className="btn" onClick={() => { setRefresh(false); setRequest(true); }}><Search className="ico" />{uiText("开始第一份比价")}</button><div className="pc-empty-steps"><span>01 · {uiText("说清商品与规格")}</span><span>02 · {uiText("agent 核对来源")}</span><span>03 · {uiText("打开页面做选择")}</span></div></section>}
    </main></div>{request && <Request current={refresh ? c : undefined} id={refresh ? id : undefined} close={() => setRequest(false)} />}<Toast kit={kit} /></div>;
}
void mount(kit, App);
