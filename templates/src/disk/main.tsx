import { themeMedia } from "../_lib/appearance";
import { t as uiText, locale } from "../_lib/i18n";
// 空间清理: what takes the room on this computer's disks, and making room.
// The disks first (each one's room, one click to scan); then a map of the
// disk (rings, as DaisyDisk draws it) beside the list of what is in the
// folder, both following each other; what to clean goes into a basket at
// the bottom, and from there to the trash (after one more look).
import "../_lib/sheet.css";
import "./disk.css";
import { useEffect, useMemo, useRef, useState, type DragEvent } from "react";
import {
  ArrowLeft, ChevronRight, File, Folder, FolderSearch, HardDrive, Network, RefreshCw, Search, Server, ShieldAlert, Trash2, Usb, X, Plus, ExternalLink,
} from "lucide-react";
import { mount, useKit, Toast } from "../_lib/appkit";
import type { OSDisk, OSScan, OSScanNode } from "../_lib/myco";
import { OSError, basename, formatBytes as bytes, has, os, osState, osStateText, useJob, useOS } from "../_lib/os";
import { cleanups, kit, Tree } from "./model";
import { Sunburst, colorOf, layout, type Seg } from "./sunburst";

// The map shows this many rings; a scan keeps this many levels, and folds
// what is under this share of the whole into "smaller items".
const LEVELS = 4;
const DEPTH = 10;
const FRACTION = 0.0008;

type Target = { name: string; path: string; disk?: OSDisk };
type Item = { path: string; name: string; disk: number; size: number; files: number; dir: boolean };

const useDark = () => {
  const q = themeMedia();
  const [dark, setDark] = useState(q.matches);
  useEffect(() => {
    const f = () => setDark(q.matches);
    q.addEventListener("change", f);
    return () => q.removeEventListener("change", f);
  }, [q]);
  return dark;
};

function diskName(d: OSDisk) {
  if (d.mount === "/" && d.scan_path) return "Macintosh HD";
  return d.label || d.mount;
}
function DiskIcon({ d }: { d: OSDisk }) {
  const I = d.network ? Network : d.removable ? Usb : d.mount === "/" || /^[A-Z]:\\?$/.test(d.mount) ? HardDrive : Server;
  return <I className="dk-disk-ico" strokeWidth={1.5} />;
}

function when(at?: string) {
  if (!at) return "";
  const d = new Date(at);
  const days = Math.floor((Date.now() - d.getTime()) / 86400000);
  if (days <= 0) return "今天";
  if (days === 1) return "昨天";
  if (days < 30) return `${days} 天前`;
  return d.toLocaleDateString(locale());
}

function App() {
  useKit(kit);
  const sys = useOS();
  const dark = useDark();
  const [unit, setUnit] = useState(() => { try { return localStorage.getItem("myco-disk-unit") || "auto"; } catch { return "auto"; } });
  const binary = unit === "binary" || (unit === "auto" && sys.info?.os === "windows");
  const formatBytes = (n: number) => bytes(n, { binary });
  const unitPicker = <label className="dk-units">{uiText("容量单位")} <select className="sel" aria-label={uiText("容量单位")} value={unit} onChange={e => { setUnit(e.target.value); try { localStorage.setItem("myco-disk-unit", e.target.value); } catch { /* private storage */ } }}><option value="auto">{uiText("跟随系统")}</option><option value="decimal">GB · 1000</option><option value="binary">GiB · 1024</option></select></label>;
  const [disks, setDisks] = useState<OSDisk[] | null>(null);
  const [diskErr, setDiskErr] = useState("");
  const [target, setTarget] = useState<Target | null>(null);
  const [tree, setTree] = useState<Tree | null>(null);
  const [scan, setScan] = useState<OSScan | null>(null);
  const [at, setAt] = useState<OSScanNode | null>(null);
  const [hover, setHover] = useState<string | null>(null);
  const [tab, setTab] = useState<"here" | "largest">("here");
  const [basket, setBasket] = useState<Item[]>([]);
  const [confirm, setConfirm] = useState(false);
  const [refining, setRefining] = useState(false);
  const [canPick, setCanPick] = useState(false);
  const [, setTick] = useState(0);
  const started = useRef(0);
  // the note about places it could not read: closed once, closed for good
  const [warnOff, setWarnOff] = useState(() => { try { return localStorage.getItem("myco-disk-warn-off") === "1"; } catch { return false; } });
  const hideWarn = () => { setWarnOff(true); try { localStorage.setItem("myco-disk-warn-off", "1"); } catch { /* (a private window) */ } };
  const scanning = useRef<Target | null>(null);

  const loadDisks = () => os().system.disks().then((d) => { setDisks(d.filter((x) => !x.read_only || x.mount === "/")); setDiskErr(""); }, (e: OSError) => setDiskErr(e.message));
  useEffect(() => {
    if (osState !== "ready") return;
    void loadDisks();
    void has("fs.pick").then(setCanPick);
  }, []);

  const job = useJob<OSScan, Progress>(() =>
    os().fs.scan({ path: scanning.current!.path, depth: DEPTH, min_fraction: FRACTION, top: 200 }));
  // (a clock for the time it has taken)
  useEffect(() => {
    if (!job.running) return;
    const t = window.setInterval(() => setTick((n) => n + 1), 500);
    return () => window.clearInterval(t);
  }, [job.running]);

  const start = async (t: Target) => {
    setTarget(t); setTree(null); setScan(null); setAt(null); setBasket([]); setHover(null); setTab("here");
    started.current = Date.now();
    scanning.current = t;
    const r = await job.start();
    if (scanning.current !== t || !r || !r.roots[0]) return;
    r.roots[0].name = t.disk ? diskName(t.disk) : t.name;
    setScan(r); setTree(new Tree(r.roots[0])); setAt(r.roots[0]);
  };
  const pickFolder = async () => {
    try {
      const r = await os().fs.pick({ title: "选一个文件夹来扫描", folders: true });
      const p = r.paths[0];
      if (p) void start({ name: basename(p), path: p });
    } catch (e) {
      if ((e as OSError).code !== "cancelled") kit.flash((e as Error).message);
    }
  };

  // Going into a folder whose children were mostly folded: scan it again, finer.
  const open = async (n: OSScanNode) => {
    if (!tree || !n.dir) return;
    setAt(n); setHover(null);
    if ((n.more_disk || 0) > n.disk * 0.25 && n.disk > 0 && !(n as { refined?: boolean }).refined) {
      setRefining(true);
      try {
        const j = os().fs.scan({ path: n.path, depth: 6, min_fraction: 0.002, top: 0 });
        const r = await j.done;
        if (r.roots[0]) { tree.graft(n, r.roots[0]); (n as { refined?: boolean }).refined = true; setTick((x) => x + 1); }
      } catch { /* (the coarse one stays) */ }
      setRefining(false);
    }
  };
  const up = () => { if (tree && at) { const p = tree.parent.get(at); if (p) { setAt(p); setHover(null); } } };
  useEffect(() => {
    const f = (e: KeyboardEvent) => { if ((e.key === "Escape" || e.key === "Backspace") && !confirm && !(e.target instanceof HTMLInputElement)) up(); };
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  });

  const add = (it: Item) => setBasket((b) => {
    if (b.some((x) => it.path === x.path || it.path.startsWith(x.path + "/") || it.path.startsWith(x.path + "\\"))) return b;
    return [...b.filter((x) => !x.path.startsWith(it.path + "/") && !x.path.startsWith(it.path + "\\")), it];
  });
  const itemOf = (n: OSScanNode): Item => ({ path: n.path, name: n.name, disk: n.disk, size: n.size, files: n.files, dir: n.dir });
  const inBasket = (p: string) => basket.some((x) => p === x.path || p.startsWith(x.path + "/") || p.startsWith(x.path + "\\"));
  const total = basket.reduce((s, x) => s + x.disk, 0);

  const clean = async () => {
    setConfirm(false);
    const items = basket;
    try {
      const r = await os().fs.trash({ paths: items.map((x) => x.path) });
      let freed = 0;
      for (const p of r.done) {
        const it = items.find((x) => x.path === p);
        if (!it) continue;
        freed += it.disk;
        tree?.remove(it.path, it.disk, it.size, it.files);
      }
      setBasket(items.filter((x) => !r.done.includes(x.path)));
      setTick((x) => x + 1);
      void loadDisks();
      if (r.done.length) {
        kit.flash(uiText("腾出了 {0}，{1} 项在废纸篓里，倒掉之前都能放回去", formatBytes(freed), r.done.length));
        void kit.track(cleanups.put({ at: new Date().toISOString(), computer: sys.info?.hostname || "", disk: target?.name || "", freed, count: r.done.length, by: kit.me }));
      }
      if (r.failed.length) kit.flash(uiText("有 {0} 项没移走：{1}", r.failed.length, r.failed[0].error.message));
    } catch (e) {
      kit.flash((e as Error).message);
    }
  };

  const last = useMemo(() => [...cleanups.rows].sort((a, b) => String(b.data.at).localeCompare(String(a.data.at)))[0], [cleanups.rows.length, kit.version]); // eslint-disable-line react-hooks/exhaustive-deps

  if (osState !== "ready") {
    return <div className="dk-empty"><HardDrive className="dk-empty-ico" strokeWidth={1.2} /><h2>{uiText("空间清理")}</h2><p>{osStateText()}</p></div>;
  }

  // --- scanning ---
  if (job.running || (target && !tree && !job.error)) {
    const p = job.progress;
    const secs = Math.max(0, Math.round((Date.now() - started.current) / 1000));
    // a disk: how much of what is used on it has been read (an estimate: a
    // disk holds a little more than its files)
    const used = target?.disk ? Math.max(0, target.disk.total - target.disk.available) : 0;
    const share = used ? Math.min(0.99, (p?.disk || 0) / used) : 0;
    return (
      <div className="dk-scanning" style={{ ["--glow" as string]: p?.top?.[0] ? scanColor(p, 0, used, dark) : "transparent" }}>
        <ScanRing p={p} used={used} dark={dark} />
        <div className="dk-scan-num">{used ? `${Math.floor(share * 100)}%` : formatBytes(p?.disk || 0)}</div>
        <div className="dk-scan-sub">
          {used ? `${formatBytes(p?.disk || 0)} / ${formatBytes(used)} · ` : ""}{(p?.files || 0).toLocaleString()} {" "}{uiText("个文件 ·")}{" "}{secs < 60 ? uiText("{0} 秒", secs) : uiText("{0} 分 {1} 秒", Math.floor(secs / 60), secs % 60)}
        </div>
        <div className="dk-scan-path" title={p?.current}>{ltr(p?.current || target?.path || "")}</div>
        {!!p?.top?.length && (
          <ul className="dk-scan-top" aria-label={uiText("目前最大的")}>
            {p.top.slice(0, 5).map((t, i) => (
              <li key={t.path}><i style={{ background: scanColor(p, i, used, dark) }} /><span>{t.name}</span><b>{formatBytes(t.disk)}</b></li>
            ))}
          </ul>
        )}
        <button type="button" className="btn" onClick={() => { scanning.current = null; job.cancel(); setTarget(null); }}>{uiText("停止")}</button>
      </div>
    );
  }

  // --- the map ---
  if (tree && at && scan && target) {
    const chain = tree.chain(at);
    const kids = at.children || [];
    const segs = layout(at, 1);
    const segOf = new Map(segs.map((s) => [s.key, s]));
    const hoverSeg = hover ? layout(at, LEVELS).find((s) => s.key === hover) || null : null;
    const onSeg = (s: Seg) => { if (s.node?.dir) void open(s.node); };
    const biggest = scan.largest.find((f) => f.path.startsWith(at.path))?.disk || 0;
    const drag = (e: DragEvent, it: Item) => { e.dataTransfer.setData("text/x-myco-path", it.path); e.dataTransfer.effectAllowed = "copy"; dragged.current = it; };
    return (
      <div className="dk">
        <header className="dk-top">
          <button type="button" className="tool" onClick={() => { setTarget(null); setTree(null); }}><ArrowLeft className="ico" />{uiText("磁盘")}</button>
          <nav className="dk-crumbs" aria-label={uiText("位置")}>
            {chain.map((n, i) => (
              <span key={n.path} className="dk-crumb">
                {i > 0 && <ChevronRight className="dk-crumb-sep" />}
                <button type="button" className={i === chain.length - 1 ? "is-here" : ""} onClick={() => setAt(n)}>{n.name || n.path}</button>
              </span>
            ))}
          </nav>
          <span className="top-gap" />{unitPicker}
          {target.disk && <span className="dk-free">{uiText("可用")}{" "}<b>{formatBytes(target.disk.available)}</b></span>}
          <button type="button" className="icon-btn" title={uiText("重新扫描")} onClick={() => void start(target)}><RefreshCw className="ico" /></button>
        </header>
        {scan.skipped > 0 && !warnOff && (
          <div className="dk-warn">
            <ShieldAlert className="ico" />
            <span>{uiText("有")}{" "}{scan.skipped.toLocaleString()} {" "}{uiText("处没读到（没有权限），实际占用会比这里多。")}</span>
            {sys.info?.os === "darwin" && <button type="button" className="link-btn" onClick={() => void os().system.settings({ pane: "full_disk" })}>{uiText("给 myco「完全磁盘访问」")}</button>}
            <button type="button" className="icon-btn dk-warn-x" title={uiText("不再提示")} aria-label={uiText("不再提示")} onClick={hideWarn}><X className="ico" /></button>
          </div>
        )}
        <div className="dk-body">
          <section className="dk-map">
            <Sunburst formatSize={formatBytes} node={at} levels={LEVELS} dark={dark} hover={hover} busy={refining} canUp={chain.length > 1}
              onHover={(k) => setHover(k)} onOpen={onSeg} onUp={up} />
            <div className="dk-legend">
              {hoverSeg ? (hoverSeg.more ? uiText("{0} 里 {1} 个更小的项目", hoverSeg.parent.name, hoverSeg.parent.more) : ltr(hoverSeg.node!.path))
                : uiText("扫了 {0} 个文件，用了 {1} 秒{2}", scan.files.toLocaleString(), (scan.ms / 1000).toFixed(1), scan.skipped && warnOff ? `，${scan.skipped.toLocaleString()} 处没权限读` : "")}
            </div>
          </section>
          <aside className="dk-side">
            <div className="seg dk-tabs" role="tablist">
              <button type="button" role="tab" aria-selected={tab === "here"} className={tab === "here" ? "is-on" : ""} onClick={() => setTab("here")}>{uiText("这里有什么")}</button>
              <button type="button" role="tab" aria-selected={tab === "largest"} className={tab === "largest" ? "is-on" : ""} onClick={() => setTab("largest")}>{uiText("最大的文件")}</button>
            </div>
            <div className="dk-list" role="list">
              {tab === "here" && kids.map((k) => {
                const s = segOf.get(k.path);
                const it = itemOf(k);
                const taken = inBasket(k.path);
                return (
                  <div key={k.path} role="listitem" className={`dk-row${hover && (hover === k.path || hover.startsWith(k.path + "/")) ? " is-on" : ""}${taken ? " is-taken" : ""}`}
                    draggable onDragStart={(e) => drag(e, it)} onMouseEnter={() => setHover(k.path)} onMouseLeave={() => setHover(null)}
                    onDoubleClick={() => k.dir && void open(k)} onClick={() => k.dir && void open(k)}>
                    <RowIcon dir={k.dir} color={s ? colorOf(s, dark) : ""} />
                    <span className="dk-name" title={k.path}>{k.name}</span>
                    <Meter share={at.disk ? k.disk / at.disk : 0} color={s ? colorOf(s, dark) : "var(--faint)"} />
                    <span className="dk-size">{formatBytes(k.disk)}</span>
                    <span className="dk-acts">
                      <button type="button" className="icon-btn" title={uiText("在访达里显示")} onClick={(e) => { e.stopPropagation(); void os().fs.reveal({ path: k.path }); }}><ExternalLink className="ico" /></button>
                      <button type="button" className="icon-btn" title={taken ? uiText("已在待清理里") : uiText("加到待清理")} disabled={taken} onClick={(e) => { e.stopPropagation(); add(it); }}><Plus className="ico" /></button>
                    </span>
                  </div>
                );
              })}
              {tab === "here" && (at.more || 0) > 0 && (
                <div className="dk-row is-more" role="listitem">
                  <span className="dk-more-ico" aria-hidden>⋯</span>
                  <span className="dk-name">{at.more} {" "}{uiText("个更小的项目")}</span>
                  <Meter share={at.disk ? (at.more_disk || 0) / at.disk : 0} color={dark ? "hsl(220 6% 40%)" : "hsl(220 8% 74%)"} />
                  <span className="dk-size">{formatBytes(at.more_disk || 0)}</span>
                </div>
              )}
              {tab === "here" && !kids.length && !(at.more || 0) && <div className="dk-none">{uiText("这里是空的")}</div>}
              {tab === "largest" && scan.largest.filter((f) => f.path.startsWith(at.path)).slice(0, 100).map((f) => {
                const it: Item = { path: f.path, name: basename(f.path), disk: f.disk ?? f.size, size: f.size, files: 1, dir: false };
                const taken = inBasket(f.path);
                return (
                  <div key={f.path} role="listitem" className={`dk-row${taken ? " is-taken" : ""}`} draggable onDragStart={(e) => drag(e, it)}>
                    <RowIcon dir={false} color="" />
                    <span className="dk-name" title={f.path}>{it.name}<small>{f.path.slice(at.path.length).replace(/[\\/][^\\/]*$/, "") || "/"}</small></span>
                    <Meter share={it.disk / (biggest || 1)} color="var(--faint)" />
                    <span className="dk-size">{formatBytes(it.disk)}</span>
                    <span className="dk-acts">
                      <button type="button" className="icon-btn" title={uiText("在访达里显示")} onClick={() => void os().fs.reveal({ path: f.path })}><ExternalLink className="ico" /></button>
                      <button type="button" className="icon-btn" title={taken ? uiText("已在待清理里") : uiText("加到待清理")} disabled={taken} onClick={() => add(it)}><Plus className="ico" /></button>
                    </span>
                  </div>
                );
              })}
            </div>
          </aside>
        </div>
        <footer className={`dk-basket${basket.length ? " has-items" : ""}`} onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = "copy"; }}
          onDrop={(e) => { e.preventDefault(); if (dragged.current) add(dragged.current); dragged.current = null; }}>
          <Trash2 className="dk-basket-ico" strokeWidth={1.6} />
          {basket.length ? (
            <div className="dk-chips">
              {basket.map((x) => (
                <span key={x.path} className="dk-chip" title={x.path}>
                  {x.dir ? <Folder className="ico" /> : <File className="ico" />}{x.name}<b>{formatBytes(x.disk)}</b>
                  <button type="button" aria-label={uiText("拿掉 {0}", x.name)} onClick={() => setBasket((b) => b.filter((y) => y.path !== x.path))}><X /></button>
                </span>
              ))}
            </div>
          ) : <span className="dk-basket-hint">{uiText("把要清理的拖到这里，或者点每行的 +")}</span>}
          <span className="top-gap" />
          {basket.length > 0 && <>
            <span className="dk-basket-total">{basket.length} {" "}{uiText("项 ·")}{" "}<b>{formatBytes(total)}</b></span>
            <button type="button" className="tool" onClick={() => setBasket([])}>{uiText("清空")}</button>
            <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>{uiText("移到废纸篓")}</button>
          </>}
        </footer>
        {confirm && (
          <div className="dk-modal-bg" onClick={() => setConfirm(false)}>
            <div className="dk-modal" role="dialog" aria-modal aria-label={uiText("移到废纸篓")} onClick={(e) => e.stopPropagation()}>
              <h3>{uiText("把这")}{" "}{basket.length} {" "}{uiText("项移到废纸篓？")}</h3>
              <p className="faint">{uiText("一共")}{" "}{formatBytes(total)}{uiText("。倒掉废纸篓之前，都能放回原处。")}</p>
              <ul>{basket.map((x) => <li key={x.path}><span title={x.path}>{ltr(x.path)}</span><b>{formatBytes(x.disk)}</b></li>)}</ul>
              <div className="dk-modal-acts">
                <button type="button" className="btn" onClick={() => setConfirm(false)} autoFocus>{uiText("取消")}</button>
                <button type="button" className="btn btn-danger" onClick={() => void clean()}>{uiText("移到废纸篓")}</button>
              </div>
            </div>
          </div>
        )}
        <Toast kit={kit} />
      </div>
    );
  }

  // --- the disks ---
  return (
    <div className="dk-home">
      <header className="dk-home-head">
        <h1>{uiText("空间清理")}</h1>{unitPicker}
        <p className="faint">{sys.info ? `${sys.info.hostname} · ${sys.info.name} ${sys.info.version}` : " "}</p>
      </header>
      {job.error && job.error.code !== "cancelled" && <div className="dk-warn"><ShieldAlert className="ico" />{job.error.message}</div>}
      {diskErr && <div className="dk-warn"><ShieldAlert className="ico" />{diskErr}</div>}
      <p className="dk-accounting faint">{uiText("GB 按 1000 进位，GiB 按 1024 进位；扫描汇总文件占用，不等于整盘已用空间。系统保留、快照和无权限内容可能造成差异。")}</p><div className="dk-disks">
        {(disks || []).map((d) => {
          const used = Math.max(0, d.total - d.available);
          const pct = d.total ? (used / d.total) * 100 : 0;
          return (
            <article key={d.mount} className="dk-disk">
              <div className="dk-disk-top">
                <DiskIcon d={d} />
                <div className="dk-disk-name"><b>{diskName(d)}</b><span className="faint">{d.fs}{d.removable ? uiText(" · 外接") : d.network ? uiText(" · 网络") : ""}</span></div>
              </div>
              <div className="dk-disk-free"><b>{formatBytes(d.available)}</b> {" "}{uiText("可用")}</div>
              <div className={`dk-meter${pct > 90 ? " is-full" : ""}`}><i style={{ width: `${pct}%` }} /></div>
              <div className="dk-disk-foot">
                <span className="faint">{uiText("已用")}{" "}{formatBytes(used)} {" "}{uiText("/ 共")}{" "}{formatBytes(d.total)}</span>
                <button type="button" className="btn btn-primary" onClick={() => void start({ name: diskName(d), path: d.scan_path || d.mount, disk: d })}><Search className="ico" />{uiText("扫描")}</button>
              </div>
            </article>
          );
        })}
        {disks && (
          <article className="dk-disk is-folder">
            <FolderSearch className="dk-disk-ico" strokeWidth={1.5} />
            <b>{uiText("只看一个文件夹")}</b>
            <span className="faint">{uiText("比整块盘快：比如个人文件夹、下载")}</span>
            <div className="dk-folder-acts">
              <button type="button" className="btn" onClick={() => void start({ name: "个人文件夹", path: "~" })}>{uiText("个人文件夹")}</button>
              {canPick && <button type="button" className="btn" onClick={() => void pickFolder()}>{uiText("选一个…")}</button>}
            </div>
          </article>
        )}
        {!disks && !diskErr && [0, 1].map((i) => <article key={i} className="dk-disk is-loading" aria-hidden />)}
      </div>
      {last && (
        <p className="dk-last faint">
          {uiText("上次清理：")}{when(String(last.data.at || ""))}{last.data.by ? `，${last.data.by} ` : ""}{uiText("在")}{" "}{String(last.data.computer || "这台电脑")} {" "}{uiText("上腾出了")}{" "}{formatBytes(Number(last.data.freed) || 0)}（{Number(last.data.count) || 0} {" "}{uiText("项）")}{" "}</p>
      )}
      <Toast kit={kit} />
    </div>
  );
}

type Progress = { files: number; disk: number; current: string; top?: { name: string; path: string; dir: boolean; disk: number }[] };

/** How much of the folder it takes: a short bar in its colour, before the size. */
function Meter({ share, color }: { share: number; color: string }) {
  return <span className="dk-meter-sm" aria-hidden><i style={{ width: `${Math.max(4, Math.min(100, share * 100))}%`, background: color }} /></span>;
}

/** A folder or file in the list: its icon in its colour on the map. */
function RowIcon({ dir, color }: { dir: boolean; color: string }) {
  const I = dir ? Folder : File;
  return <I className="dk-ico" strokeWidth={1.7} style={color ? { color, fill: `color-mix(in srgb, ${color} 28%, transparent)` } : undefined} />;
}

/** The i-th folder read while scanning, coloured where it will be on the map. */
function scanColor(p: Progress | null, i: number, used: number, dark: boolean) {
  const top = p?.top || [];
  const whole = used || top.reduce((s, t) => s + t.disk, 0) || 1;
  let a = 0;
  for (let j = 0; j < i; j++) a += (top[j].disk / whole) * Math.PI * 2;
  const w = ((top[i]?.disk || 0) / whole) * Math.PI * 2;
  return colorOf({ key: "", node: { dir: true } as OSScanNode, parent: {} as OSScanNode, depth: 1, a0: a, a1: a + w }, dark);
}

/** While scanning: a ring that fills as it goes (a disk: what is used on
 *  it; a folder: what is found so far), the folders read to the end in
 *  their colours, the rest of what is read in grey, and a light running
 *  round the outside. */
function ScanRing({ p, used, dark }: { p: Progress | null; used: number; dark: boolean }) {
  const C = 150, R = 118, W = 22, TAU = Math.PI * 2;
  const top = p?.top || [];
  const done = top.reduce((s, t) => s + t.disk, 0);
  const whole = used || Math.max(done, p?.disk || 0) || 1;
  const arc = (a0: number, a1: number) => {
    if (a1 - a0 < 0.002) return "";
    a1 = Math.min(a1, a0 + TAU - 1e-4);
    const pt = (a: number) => `${(C + R * Math.sin(a)).toFixed(2)},${(C - R * Math.cos(a)).toFixed(2)}`;
    return `M${pt(a0)}A${R},${R} 0 ${a1 - a0 > Math.PI ? 1 : 0} 1 ${pt(a1)}`;
  };
  let a = 0;
  const segs = top.map((t, i) => {
    const w = (t.disk / whole) * TAU;
    const d = arc(a + 0.004, a + w - 0.004);
    a += w;
    return d ? <path key={t.path} d={d} className="dk-ring-seg" style={{ stroke: scanColor(p, i, used, dark) }} /> : null;
  });
  // what is read but not yet in a finished folder
  const reading = Math.min(TAU, ((p?.disk || 0) / whole) * TAU);
  return (
    <svg className="dk-ring" viewBox="0 0 300 300" aria-hidden>
      <circle cx={C} cy={C} r={R} className="dk-ring-track" style={{ strokeWidth: W }} />
      {reading > a && <path d={arc(a, reading)} className="dk-ring-reading" style={{ strokeWidth: W }} />}
      <g style={{ strokeWidth: W }}>{segs}</g>
      <circle cx={C} cy={C} r={R + W / 2 + 9} className="dk-ring-run" />
    </svg>
  );
}

const dragged: { current: Item | null } = { current: null };
// (a path shown cut from the left, by direction: rtl, keeps its "/" in front)
const ltr = (s: string) => "\u200E" + s + "\u200E";

void mount(kit, App);
