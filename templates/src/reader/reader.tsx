// Reading session lifecycle: one open document, cancellable PDF work, local
// progress/bookmarks, and user-invoked navigation. No desktop/OS capabilities.
import { useEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  Bookmark,
  ChevronLeft,
  ChevronUp,
  ChevronDown,
  ChevronRight,
  List,
  Maximize2,
  Minimize2,
  Search,
  Settings2,
  X,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { Book, Settings as Preferences } from "./model";
import { chapters, decodeText, occurrences, percent } from "./model";
import type { Library } from "./library";
import { loadPDF, pdfChapters, searchPDF, type PDFBook } from "./pdf";
import { PDFView } from "./pdf-view";
import { useReadingNavigation } from "./use-reading-navigation";
import { PDFScrollView } from "./pdf-scroll";
import { TextView, type TextNavigation } from "./text-view";
import { ReadingSettings } from "./settings";
import { Dialog, IconButton } from "./ui";
export function Reader({
  initial,
  library,
  settings,
  onSettings,
  onBack,
  onError,
}: {
  initial: Book;
  library: Library;
  settings: Preferences;
  onSettings: (s: Preferences) => void;
  onBack: () => void;
  onError: (error: unknown) => void;
}) {
  const [book, setBook] = useState(initial),
    [text, setText] = useState<string | null>(null),
    [pdf, setPDF] = useState<PDFBook | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const [panel, setPanel] = useState<
      "settings" | "contents" | "search" | "bookmarks" | null
    >(null),
    [outline, setOutline] = useState<{ title: string; position: number }[]>([]),
    [zoom, setZoom] = useState(1),
    [focus, setFocus] = useState(false);
  const [password, setPassword] = useState<{
      retry: boolean;
      resolve: (v: string) => void;
    } | null>(null),
    [secret, setSecret] = useState("");
  const [nav, setNav] = useState({ page: 0, total: 1 }),
    navigation = useRef<TextNavigation | null>(null),
    pdfNavigation = useRef<TextNavigation | null>(null),
    shell = useRef<HTMLDivElement>(null),
    stage = useRef<HTMLDivElement>(null);
  const [query, setQuery] = useState(""),
    [results, setResults] = useState<{ position: number; excerpt: string }[]>(
      [],
    ),
    [searching, setSearching] = useState(0),
    [didSearch, setDidSearch] = useState(false);
  const search = useRef<AbortController>(),
    position = useRef(initial.position),
    direction = useRef(1),
    touch = useRef<{ x: number; y: number } | null>(null),
    [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const persist = (fields: Partial<Book>) => {
    setBook((b) => ({ ...b, ...fields }));
    void library.patch(initial.id, fields).catch(onError);
  };
  const pdfOffset = useRef(initial.pageOffset || 0),
    pendingProgress = useRef<Partial<Book> | null>(null),
    progressTimer = useRef(0),
    errorCallback = useRef(onError);
  errorCallback.current = onError;
  const flushProgress = () => {
    window.clearTimeout(progressTimer.current);
    progressTimer.current = 0;
    const fields = pendingProgress.current;
    pendingProgress.current = null;
    if (fields)
      void library.patch(initial.id, fields).catch(errorCallback.current);
  };
  const progress = (offset: number, fraction: number, pageOffset = 0) => {
    position.current = offset;
    pdfOffset.current = pageOffset;
    const fields = {
      position: offset,
      progress: fraction,
      pageOffset,
      opened: Date.now(),
    };
    setBook((b) => ({ ...b, ...fields }));
    pendingProgress.current = fields;
    if (!progressTimer.current)
      progressTimer.current = window.setTimeout(flushProgress, 120);
  };
  useEffect(() => {
    const hidden = () => {
      if (document.hidden) flushProgress();
    };
    window.addEventListener("pagehide", flushProgress);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      window.removeEventListener("pagehide", flushProgress);
      document.removeEventListener("visibilitychange", hidden);
      flushProgress();
    };
  }, [initial.id, library]);
  useEffect(() => {
    const controller = new AbortController();
    let loaded: PDFBook | null = null;
    (async () => {
      const stored = await library.get(initial.id);
      if (!stored) throw new Error("missing_book");
      if (controller.signal.aborted) return;
      if (initial.format === "txt") {
        const { text: source } = decodeText(await stored.file.arrayBuffer());
        if (!controller.signal.aborted) {
          setText(source);
          setOutline(chapters(source));
        }
      } else {
        loaded = await loadPDF(
          stored.file,
          (retry) =>
            new Promise((resolve) => {
              if (!controller.signal.aborted) setPassword({ retry, resolve });
            }),
          controller.signal,
        );
        if (controller.signal.aborted) return;
        const p = Math.max(0, Math.min(loaded.numPages - 1, initial.position));
        position.current = p;
        setPDF(loaded);
        setNav({ page: p, total: loaded.numPages });
        persist({ pages: loaded.numPages, opened: Date.now() });
        pdfChapters(loaded)
          .then((items) => {
            if (!controller.signal.aborted) setOutline(items);
          })
          .catch(() => undefined);
      }
    })()
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(
            e?.message === "pdf_pages"
              ? uiText("PDF 超过 10,000 页，请拆分后阅读。")
              : uiText(
                  "无法打开这本书。PDF 可能已损坏，或文字编码不受支持。请尝试重新导入。",
                ),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      controller.abort();
      search.current?.abort();
      if (loaded) void loaded.destroy().catch(() => undefined);
    };
  }, [initial.id, library]);
  function go(offset: number, pageOffset = 0) {
    if (pdf && settings.mode === "scroll") {
      pdfNavigation.current?.go(offset, pageOffset);
    } else if (pdf) {
      const p = Math.max(0, Math.min(pdf.numPages - 1, offset));
      direction.current = p >= nav.page ? 1 : -1;
      setNav({ page: p, total: pdf.numPages });
      progress(p, pdf.numPages <= 1 ? 1 : p / (pdf.numPages - 1));
    } else {
      direction.current = offset >= position.current ? 1 : -1;
      navigation.current?.go(offset);
    }
  }
  function turn(delta: number) {
    if (loading || error) return;
    direction.current = delta;
    if (pdf && settings.mode === "scroll") pdfNavigation.current?.turn(delta);
    else if (pdf) go(nav.page + delta);
    else navigation.current?.turn(delta);
  }
  useReadingNavigation({
    settings,
    page: nav.page,
    direction,
    stage,
    shell,
    zoom,
    isPDF: !!pdf,
    loading,
    error,
    turn,
    setFocus,
  });
  useEffect(() => {
    const f = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", f);
    return () => document.removeEventListener("fullscreenchange", f);
  }, []);
  const marked = book.bookmarks.some((b) => b.position === position.current);
  function mark() {
    const label = pdf
      ? uiText("第 {0} 页", nav.page + 1)
      : text
          ?.slice(position.current, position.current + 70)
          .trim()
          .split("\n")[0] || book.title;
    persist({
      bookmarks: marked
        ? book.bookmarks.filter((b) => b.position !== position.current)
        : [
            ...book.bookmarks,
            {
              position: position.current,
              pageOffset: pdfOffset.current,
              label,
              at: Date.now(),
            },
          ].sort((a, b) => a.position - b.position),
    });
  }
  async function find() {
    search.current?.abort();
    const c = new AbortController();
    search.current = c;
    setDidSearch(false);
    setResults([]);
    if (!query.trim()) {
      setSearching(0);
      return;
    }
    try {
      setSearching(1);
      const found = pdf
        ? await searchPDF(pdf, query, c.signal, setSearching)
        : occurrences(text || "", query);
      if (!c.signal.aborted) {
        setResults(found);
        setDidSearch(true);
      }
    } catch (e) {
      if (!c.signal.aborted) onError(e);
    } finally {
      if (!c.signal.aborted) setSearching(0);
    }
  }
  function closePanel() {
    setPanel(null);
    search.current?.abort();
    setSearching(0);
  }
  return (
    <div
      className={`rd-reader rd-theme-${settings.theme} rd-mode-${settings.mode} ${pdf && zoom > 1 ? "rd-is-zoomed" : ""} ${focus ? "is-focus" : ""}`}
    >
      <header className="rd-reader-toolbar">
        <button className="rd-back" onClick={onBack}>
          <ArrowLeft size={18} />
          <span>{uiText("书架")}</span>
        </button>
        <span className="rd-reading-title">{book.title}</span>
        <span className="rd-toolbar-spacer" />
        {pdf && (
          <div className="rd-pdf-tools">
            <IconButton
              label={uiText("缩小页面")}
              disabled={zoom <= 0.5}
              onClick={() => setZoom(Math.max(0.5, zoom - 0.25))}
            >
              <ZoomOut size={17} />
            </IconButton>
            <button
              className="rd-zoom"
              title={
                settings.mode === "vertical"
                  ? uiText("恢复适合页面")
                  : uiText("恢复适合宽度")
              }
              onClick={() => setZoom(1)}
            >
              {Math.round(zoom * 100)}%
            </button>
            <IconButton
              label={uiText("放大页面")}
              disabled={zoom >= 3}
              onClick={() => setZoom(Math.min(3, zoom + 0.25))}
            >
              <ZoomIn size={17} />
            </IconButton>
          </div>
        )}
        <IconButton label={uiText("目录")} onClick={() => setPanel("contents")}>
          <List size={19} />
        </IconButton>
        <IconButton
          label={uiText("搜索书中内容")}
          onClick={() => setPanel("search")}
          disabled={loading}
        >
          <Search size={18} />
        </IconButton>
        <IconButton
          label={marked ? uiText("移除本页书签") : uiText("添加书签")}
          onClick={mark}
          active={marked}
          disabled={loading || !!error}
        >
          <Bookmark size={18} fill={marked ? "currentColor" : "none"} />
        </IconButton>
        <IconButton
          label={uiText("阅读设置")}
          onClick={() => setPanel("settings")}
        >
          <Settings2 size={18} />
        </IconButton>
        {document.fullscreenEnabled && (
          <IconButton
            label={fullscreen ? uiText("退出全屏") : uiText("全屏阅读")}
            onClick={() => {
              const action = document.fullscreenElement
                ? document.exitFullscreen()
                : document.documentElement.requestFullscreen();
              action.catch(onError);
            }}
          >
            {fullscreen ? <Minimize2 size={17} /> : <Maximize2 size={17} />}
          </IconButton>
        )}
      </header>
      <div
        className={`rd-reading-stage ${pdf ? "is-pdf" : ""}`}
        ref={stage}
        onTouchStart={(e) => {
          if (e.touches.length === 1)
            touch.current = {
              x: e.touches[0].clientX,
              y: e.touches[0].clientY,
            };
          else touch.current = null;
        }}
        onTouchCancel={() => {
          touch.current = null;
        }}
        onTouchEnd={(e) => {
          const start = touch.current;
          touch.current = null;
          if (
            settings.mode === "scroll" ||
            !start ||
            e.touches.length ||
            zoom > 1 ||
            document.getSelection()?.toString()
          )
            return;
          const dx = e.changedTouches[0].clientX - start.x,
            dy = e.changedTouches[0].clientY - start.y;
          const primary = settings.mode === "vertical" ? dy : dx,
            cross = settings.mode === "vertical" ? dx : dy;
          if (
            Math.abs(primary) > 60 &&
            Math.abs(primary) > Math.abs(cross) * 1.5
          )
            turn(primary < 0 ? 1 : -1);
        }}
      >
        <button
          className="rd-edge rd-edge-prev"
          aria-label={uiText("上一页")}
          disabled={!nav.page || loading}
          onClick={() => turn(-1)}
        >
          {settings.mode === "vertical" ? (
            <ChevronUp size={24} strokeWidth={1.2} />
          ) : (
            <ChevronLeft size={24} strokeWidth={1.2} />
          )}
        </button>
        <article
          className="rd-page-shell"
          ref={shell}
          style={{ maxWidth: pdf ? "none" : settings.width + 100 }}
        >
          {loading && (
            <div className="rd-loading" role="status">
              <span className="rd-spinner" />
              {uiText("正在打开书籍…")}
            </div>
          )}
          {error && (
            <div className="rd-loading rd-error" role="alert">
              <p>{error}</p>
              <button className="rd-primary" onClick={onBack}>
                {uiText("返回书架")}
              </button>
            </div>
          )}
          {!loading && !error && text !== null && (
            <>
              <div className="rd-running-head">{book.title}</div>
              <TextView
                text={text}
                settings={settings}
                initial={position.current}
                onPosition={progress}
                onNavigation={(n) => {
                  navigation.current = n;
                  setNav({ page: n.page, total: n.total });
                }}
              />
            </>
          )}
          {!loading &&
            !error &&
            pdf &&
            (settings.mode === "scroll" ? (
              <PDFScrollView
                doc={pdf}
                zoom={zoom}
                initial={position.current}
                initialOffset={pdfOffset.current}
                onPosition={progress}
                onNavigation={(n) => {
                  pdfNavigation.current = n;
                  setNav({ page: n.page, total: n.total });
                }}
                onCover={(cover) => {
                  if (book.cover !== cover) persist({ cover });
                }}
              />
            ) : (
              <PDFView
                doc={pdf}
                page={nav.page}
                zoom={zoom}
                fitPage={settings.mode === "vertical"}
                onCover={(cover) => {
                  if (book.cover !== cover) persist({ cover });
                }}
              />
            ))}
        </article>
        <button
          className="rd-edge rd-edge-next"
          aria-label={uiText("下一页")}
          disabled={nav.page >= nav.total - 1 || loading}
          onClick={() => turn(1)}
        >
          {settings.mode === "vertical" ? (
            <ChevronDown size={24} strokeWidth={1.2} />
          ) : (
            <ChevronRight size={24} strokeWidth={1.2} />
          )}
        </button>
      </div>
      <footer className="rd-reader-footer">
        <button
          className="rd-quiet"
          aria-label={uiText("书签")}
          onClick={() => setPanel("bookmarks")}
        >
          <Bookmark size={14} />
          <span>
            {uiText("书签")}
            {book.bookmarks.length ? ` ${book.bookmarks.length}` : ""}
          </span>
        </button>
        <div className="rd-pagination">
          <span>
            {nav.page + 1} / {nav.total}
          </span>
          <input
            aria-label={uiText("阅读进度")}
            type="range"
            min={0}
            max={Math.max(1, nav.total - 1)}
            value={nav.page}
            disabled={loading || nav.total <= 1}
            onChange={(e) => {
              const p = +e.target.value;
              if (pdf) go(p);
              else {
                direction.current = p >= nav.page ? 1 : -1;
                navigation.current?.fraction(p / (nav.total - 1));
              }
            }}
          />
          <span>
            {percent(
              settings.mode === "scroll"
                ? book.progress
                : nav.total <= 1
                  ? 1
                  : nav.page / (nav.total - 1),
            )}
            %
          </span>
        </div>
        <button
          className="rd-quiet rd-focus-button"
          onClick={() => setFocus(!focus)}
        >
          {focus ? uiText("显示工具栏") : uiText("专注阅读")}
        </button>
      </footer>
      {panel === "settings" && (
        <ReadingSettings
          value={settings}
          pdf={book.format === "pdf"}
          onChange={onSettings}
          onClose={closePanel}
        />
      )}
      {(panel === "contents" || panel === "bookmarks") && (
        <Dialog
          title={panel === "contents" ? uiText("目录") : uiText("我的书签")}
          onClose={closePanel}
          drawer
        >
          <div className="rd-contents">
            <p className="rd-drawer-book">{book.title}</p>
            {panel === "contents" ? (
              <>
                <button
                  onClick={() => {
                    go(0);
                    closePanel();
                  }}
                >
                  <span>{uiText("从头开始")}</span>
                  <ChevronRight size={15} />
                </button>
                {outline.map((item, i) => (
                  <button
                    key={i}
                    onClick={() => {
                      go(item.position);
                      closePanel();
                    }}
                  >
                    <span>{item.title}</span>
                    <ChevronRight size={15} />
                  </button>
                ))}
                {!outline.length && (
                  <p className="rd-hint">
                    {uiText("这本书没有目录，可以用底部进度条跳转。")}
                  </p>
                )}
              </>
            ) : book.bookmarks.length ? (
              book.bookmarks.map((mark) => (
                <div key={mark.position} className="rd-bookmark-row">
                  <button
                    onClick={() => {
                      go(mark.position, mark.pageOffset);
                      closePanel();
                    }}
                  >
                    <Bookmark size={14} />
                    <span>{mark.label}</span>
                  </button>
                  <IconButton
                    label={uiText("删除书签")}
                    onClick={() =>
                      persist({
                        bookmarks: book.bookmarks.filter(
                          (b) => b.position !== mark.position,
                        ),
                      })
                    }
                  >
                    <X size={15} />
                  </IconButton>
                </div>
              ))
            ) : (
              <div className="rd-panel-empty">
                <Bookmark size={26} />
                <p>{uiText("读到喜欢的地方，点一下顶部的书签。")}</p>
              </div>
            )}
          </div>
        </Dialog>
      )}
      {panel === "search" && (
        <Dialog title={uiText("搜索书中内容")} onClose={closePanel} drawer>
          <div className="rd-find">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void find();
              }}
            >
              <input
                autoFocus
                aria-label={uiText("关键词")}
                placeholder={uiText("输入关键词")}
                value={query}
                onChange={(e) => {
                  search.current?.abort();
                  setSearching(0);
                  setDidSearch(false);
                  setResults([]);
                  setQuery(e.target.value);
                }}
              />
              <button
                className="rd-primary"
                disabled={!query.trim() || !!searching}
              >
                {uiText("搜索")}
              </button>
            </form>
            {!!searching && (
              <p role="status">
                {pdf
                  ? uiText("正在搜索第 {0} 页…", searching)
                  : uiText("正在搜索…")}
              </p>
            )}
            {didSearch && (
              <p className="rd-hint" role="status">
                {results.length
                  ? uiText("找到 {0} 处（最多显示 100 处）", results.length)
                  : uiText("未找到内容。扫描版 PDF 需要先做文字识别。")}
              </p>
            )}
            {results.map((result, i) => (
              <button
                className="rd-search-result"
                key={i}
                onClick={() => {
                  go(result.position);
                  closePanel();
                }}
              >
                <span>{result.excerpt}</span>
                <ChevronRight size={15} />
              </button>
            ))}
          </div>
        </Dialog>
      )}
      {password && (
        <Dialog title={uiText("这本 PDF 需要密码")} onClose={onBack}>
          <form
            className="rd-password"
            onSubmit={(e) => {
              e.preventDefault();
              password.resolve(secret);
              setSecret("");
              setPassword(null);
            }}
          >
            <p>
              {password.retry
                ? uiText("密码不正确，请再试一次。")
                : uiText("密码只用于本次打开，不会保存。")}
            </p>
            <input
              type="password"
              autoFocus
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              aria-label={uiText("PDF 密码")}
            />
            <button className="rd-primary">{uiText("打开书籍")}</button>
          </form>
        </Dialog>
      )}
    </div>
  );
}
