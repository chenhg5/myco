// Bookshelf presentation. All file writes and library mutation are owned by
// the parent; this view only filters, sorts and opens explicit user actions.
import { useState } from "react";
import {
  ArrowRight,
  BookOpen,
  Check,
  ChevronDown,
  Grid2X2,
  List,
  MoreHorizontal,
  Plus,
  Search,
  Upload,
  X,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { bookColor, percent, type Book } from "./model";
import { IconButton } from "./ui";
export function Cover({
  book,
  large = false,
}: {
  book: Book;
  large?: boolean;
}) {
  return (
    <div
      className={`rd-cover rd-cover-${bookColor(book.id)} ${large ? "is-large" : ""}`}
      aria-hidden="true"
    >
      {book.cover ? (
        <img src={book.cover} alt="" />
      ) : (
        <>
          <span className="rd-cover-spine" />
          <span className="rd-cover-format">{book.format.toUpperCase()}</span>
          <strong>{book.title}</strong>
          <span className="rd-cover-art">
            <i />
            <i />
            <i />
          </span>
          <span className="rd-cover-edition">MY LIBRARY</span>
        </>
      )}
    </div>
  );
}
export function Shelf({
  books,
  busy,
  onImport,
  onOpen,
  onManage,
  onDemo,
  notice,
}: {
  books: Book[];
  busy: string;
  onImport: (files: FileList | File[]) => void;
  onOpen: (b: Book) => void;
  onManage: (b: Book) => void;
  onDemo: () => void;
  notice: string;
}) {
  const [query, setQuery] = useState(""),
    [filter, setFilter] = useState("all"),
    [sort, setSort] = useState("recent"),
    [list, setList] = useState(false),
    [dragging, setDragging] = useState(false);
  const recent = books.find((b) => b.opened && b.progress < 1);
  const shown = books
    .filter(
      (b) =>
        (!query ||
          (b.title + b.name)
            .toLocaleLowerCase()
            .includes(query.toLocaleLowerCase())) &&
        (filter === "all" ||
          (filter === "reading" && b.opened && b.progress < 1) ||
          (filter === "finished" && b.progress >= 1)),
    )
    .sort((a, b) =>
      sort === "title"
        ? a.title.localeCompare(b.title)
        : sort === "added"
          ? b.added - a.added
          : (b.opened || b.added) - (a.opened || a.added),
    );
  return (
    <div
      className={`rd-shelf ${dragging ? "is-dragging" : ""}`}
      onDragOver={(e) => {
        e.preventDefault();
        if (!busy) setDragging(true);
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node))
          setDragging(false);
      }}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        if (!busy) onImport([...e.dataTransfer.files]);
      }}
    >
      <header className="rd-shelf-header">
        <div className="rd-brand">
          <BookOpen size={24} strokeWidth={1.6} />
          <span>{uiText("阅读")}</span>
          <small>READER</small>
        </div>
        <label className={`rd-primary rd-import ${busy ? "is-disabled" : ""}`}>
          <Plus size={17} />
          {uiText("导入书籍")}
          <input
            data-testid="import-books"
            type="file"
            multiple
            accept=".pdf,.txt,application/pdf,text/plain"
            disabled={!!busy}
            onChange={(e) => {
              if (e.target.files) onImport(e.target.files);
              e.target.value = "";
            }}
          />
        </label>
      </header>
      <main className="rd-shelf-main">
        <section className="rd-shelf-heading">
          <div>
            <span className="rd-eyebrow">
              {uiText("给自己，一点阅读的时间")}
            </span>
            <h1>
              {uiText("我的书架")}
              <sup>{books.length.toString().padStart(2, "0")}</sup>
            </h1>
            <p>{uiText("书页之间，世界慢一点。")}</p>
          </div>
          <div className="rd-library-mark" aria-hidden="true">
            <i />
            <i />
            <i />
            <i />
          </div>
        </section>
        {recent && (
          <button className="rd-continue" onClick={() => onOpen(recent)}>
            <Cover book={recent} />
            <span className="rd-continue-copy">
              <small>{uiText("接着上次读")}</small>
              <strong>{recent.title}</strong>
              <span>
                {uiText("已读 {0}%", percent(recent.progress))}
                <span className="rd-dot">·</span>
                {recent.format.toUpperCase()}
              </span>
              <span className="rd-continue-track">
                <i style={{ width: `${percent(recent.progress)}%` }} />
              </span>
            </span>
            <span className="rd-continue-action">
              {uiText("继续阅读")}
              <ArrowRight size={18} />
            </span>
          </button>
        )}
        <div className="rd-shelf-tools">
          <nav aria-label={uiText("书架分类")}>
            {[
              ["all", uiText("全部书籍")],
              ["reading", uiText("正在读")],
              ["finished", uiText("已读完")],
            ].map(([id, label]) => (
              <button
                key={id}
                aria-pressed={filter === id}
                onClick={() => setFilter(id)}
              >
                {label}
              </button>
            ))}
          </nav>
          <div className="rd-shelf-controls">
            <div className="rd-search">
              <Search size={16} />
              <input
                aria-label={uiText("搜索书架")}
                placeholder={uiText("搜索书名")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              {query && (
                <IconButton
                  label={uiText("清除搜索")}
                  onClick={() => setQuery("")}
                >
                  <X size={14} />
                </IconButton>
              )}
            </div>
            <label className="rd-sort">
              <select
                aria-label={uiText("排列顺序")}
                value={sort}
                onChange={(e) => setSort(e.target.value)}
              >
                <option value="recent">{uiText("最近阅读")}</option>
                <option value="added">{uiText("导入时间")}</option>
                <option value="title">{uiText("书名")}</option>
              </select>
              <ChevronDown size={13} />
            </label>
            <IconButton
              label={list ? uiText("封面视图") : uiText("列表视图")}
              onClick={() => setList(!list)}
            >
              {list ? <Grid2X2 size={17} /> : <List size={18} />}
            </IconButton>
          </div>
        </div>
        {busy && (
          <div className="rd-import-state" role="status">
            <span className="rd-spinner" />
            {busy}
          </div>
        )}
        {notice && (
          <div role="status" className="rd-inline-notice">
            {notice}
          </div>
        )}
        {!books.length && !busy ? (
          <section className="rd-empty">
            <div className="rd-empty-book" aria-hidden="true">
              <BookOpen size={48} strokeWidth={0.9} />
            </div>
            <h2>{uiText("把喜欢的书，放在这里")}</h2>
            <p>{uiText("拖入 PDF 或 TXT，开启一段安静的阅读。")}</p>
            <label className="rd-primary rd-import">
              <Upload size={16} />
              {uiText("选择书籍")}
              <input
                type="file"
                multiple
                accept=".pdf,.txt,application/pdf,text/plain"
                onChange={(e) => {
                  if (e.target.files) onImport(e.target.files);
                  e.target.value = "";
                }}
              />
            </label>
            <button className="rd-quiet" onClick={onDemo}>
              {uiText("先读一篇示例")}
              <ArrowRight size={14} />
            </button>
            <small>{uiText("PDF ≤ 100 MB · TXT ≤ 5 MB")}</small>
          </section>
        ) : (
          <section
            className={`rd-book-grid ${list ? "is-list" : ""}`}
            aria-label={uiText("书籍")}
          >
            {shown.map((book) => (
              <article className="rd-book" key={book.id} data-book={book.id}>
                <button
                  className="rd-book-open"
                  aria-label={uiText("阅读《{0}》", book.title)}
                  onClick={() => onOpen(book)}
                >
                  <Cover book={book} large />
                  <span className="rd-book-text">
                    <strong>{book.title}</strong>
                    <span>
                      {book.format.toUpperCase()}
                      <span className="rd-dot">·</span>
                      {book.opened
                        ? book.progress >= 1
                          ? uiText("已读完")
                          : uiText("已读 {0}%", percent(book.progress))
                        : uiText("未开始")}
                    </span>
                    <span className="rd-book-track">
                      <i style={{ width: `${percent(book.progress)}%` }} />
                    </span>
                  </span>
                </button>
                <IconButton
                  label={uiText("管理《{0}》", book.title)}
                  onClick={() => onManage(book)}
                >
                  <MoreHorizontal size={18} />
                </IconButton>
                {book.progress >= 1 && (
                  <span className="rd-finished">
                    <Check size={12} />
                  </span>
                )}
              </article>
            ))}
            {!shown.length && !busy && (
              <p className="rd-no-results">
                {uiText("没有找到书籍，试试其他关键词或分类。")}
              </p>
            )}
            {!!books.length && filter === "all" && !query && (
              <label className="rd-add-tile rd-import">
                <Plus size={27} strokeWidth={1.2} />
                <span>{uiText("再添一本")}</span>
                <input
                  disabled={!!busy}
                  type="file"
                  multiple
                  accept=".pdf,.txt"
                  onChange={(e) => {
                    if (e.target.files) onImport(e.target.files);
                    e.target.value = "";
                  }}
                />
              </label>
            )}
          </section>
        )}
      </main>
      <footer className="rd-shelf-footer">
        <span className="rd-local-dot" />
        {uiText("原书与进度仅保存在本机，不会上传或跨设备同步。请保留原文件。")}
        {!!books.length && (
          <button className="rd-quiet" disabled={!!busy} onClick={onDemo}>
            {uiText("示例书架")}
          </button>
        )}
      </footer>
      {dragging && (
        <div className="rd-drop-overlay">
          <Upload size={38} />
          <strong>{uiText("松手，加入书架")}</strong>
          <span>PDF / TXT</span>
        </div>
      )}
    </div>
  );
}
