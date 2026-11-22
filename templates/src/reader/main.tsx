// Assemble the local reader. The bridge supplies page/member identity and file
// downloads; importing a book never writes its contents to the workspace.
import "./reader.css";
import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Download, Trash2 } from "lucide-react";
import { language, t as uiText } from "../_lib/i18n";
import { myco } from "../_lib/myco";
import { Library } from "./library";
import { settingsOf, type Book, type Settings } from "./model";
import { demoFile } from "./demo";
import { Reader } from "./reader";
import { Shelf } from "./shelf";
import { Dialog } from "./ui";

function friendlyError(error: unknown) {
  const code = error instanceof Error ? error.message : "";
  if (code === "file_type") return uiText("请选择 PDF 或 TXT 文件。");
  if (code === "file_size")
    return uiText("文件为空或过大：PDF 最多 100 MB，TXT 最多 5 MB。");
  if (code === "invalid_text" || error instanceof TypeError)
    return uiText("无法读取这个文本。支持 UTF-8、UTF-16 和 GB18030 编码。");
  if (code === "invalid_pdf")
    return uiText("这个文件不是有效的 PDF，请检查原文件。");
  if (code === "missing_book")
    return uiText("找不到这本书的本机文件，请重新导入。");
  return uiText(
    "操作未能保存。请检查本机剩余空间，并确保允许保存网站数据。请勿清除书籍原文件。",
  );
}
function App({ library }: { library: Library }) {
  const [books, setBooks] = useState<Book[]>([]),
    [book, setBook] = useState<Book | null>(null),
    [settings, setSettings] = useState(settingsOf(null)),
    [ready, setReady] = useState(false),
    [error, setError] = useState("");
  const [busy, setBusy] = useState(""),
    [notice, setNotice] = useState(""),
    [manage, setManage] = useState<Book | null>(null),
    [title, setTitle] = useState(""),
    [deleting, setDeleting] = useState(false);
  const [, refreshLabels] = useState(0);
  useEffect(
    () => myco?.on?.("preferences", () => refreshLabels((n) => n + 1)),
    [],
  );
  const importing = useRef(false),
    alive = useRef(true);
  const report = (e: unknown) => setError(friendlyError(e));
  const refresh = () =>
    library.list().then((b) => {
      if (alive.current) setBooks(b);
    });
  useEffect(() => {
    alive.current = true;
    Promise.all([
      refresh(),
      library.settings().then((s) => setSettings(settingsOf(s || null))),
    ])
      .then(() => setReady(true))
      .catch(report);
    return () => {
      alive.current = false;
    };
  }, [library]);
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 5000);
    return () => clearTimeout(timer);
  }, [notice]);
  async function importBooks(files: FileList | File[]) {
    if (importing.current) return;
    importing.current = true;
    setNotice("");
    const items = [...files];
    let imported = 0;
    const failures: string[] = [];
    try {
      for (const [i, file] of items.entries()) {
        setBusy(
          uiText("正在导入 {0} / {1}：{2}", i + 1, items.length, file.name),
        );
        try {
          await library.import(file);
          imported++;
        } catch (e) {
          failures.push(`${file.name}：${friendlyError(e)}`);
        }
      }
      await refresh();
      if (imported)
        setNotice(
          uiText("已加入书架，共 {0} 本。重复文件会自动合并。", imported),
        );
      if (failures.length) setError(failures.join("\n"));
    } catch (e) {
      report(e);
    } finally {
      setBusy("");
      importing.current = false;
    }
  }
  const changeSettings = (s: Settings) => {
    const next = settingsOf(s);
    setSettings(next);
    void library.saveSettings(next).catch(report);
  };
  if (book)
    return (
      <>
        <Reader
          key={book.id}
          initial={book}
          library={library}
          settings={settings}
          onSettings={changeSettings}
          onBack={() => {
            setBook(null);
            void refresh().catch(report);
          }}
          onError={report}
        />
        {error && (
          <Dialog title={uiText("没有完成")} onClose={() => setError("")}>
            <p className="rd-error-message">{error}</p>
          </Dialog>
        )}
      </>
    );
  return (
    <>
      {ready ? (
        <Shelf
          books={books}
          busy={busy}
          notice={notice}
          onImport={(files) => {
            void importBooks(files);
          }}
          onOpen={setBook}
          onManage={(b) => {
            setManage(b);
            setTitle(b.title);
            setDeleting(false);
          }}
          onDemo={() => {
            void importBooks(
              [0, 1, 2].map((i) => demoFile(i, language() === "en")),
            );
          }}
        />
      ) : (
        <div className="rd-loading">
          <span className="rd-spinner" />
          {uiText("正在打开书架…")}
        </div>
      )}
      {error && (
        <Dialog title={uiText("没有完成")} onClose={() => setError("")}>
          <p className="rd-error-message">{error}</p>
          <div className="rd-dialog-actions">
            <button
              className="rd-primary"
              onClick={() => {
                setError("");
                if (!ready) location.reload();
              }}
            >
              {uiText("知道了")}
            </button>
          </div>
        </Dialog>
      )}
      {manage && (
        <Dialog title={uiText("管理书籍")} onClose={() => setManage(null)}>
          <div className="rd-manage">
            <label>
              {uiText("书名")}
              <input
                value={title}
                maxLength={180}
                onChange={(e) => setTitle(e.target.value)}
              />
            </label>
            <p className="rd-hint">
              {manage.name}
              <br />
              {(manage.size / 1024 / 1024).toFixed(2)} MB ·{" "}
              {manage.format.toUpperCase()}
            </p>
            <button
              className="rd-primary"
              disabled={!title.trim()}
              onClick={() => {
                void library
                  .patch(manage.id, { title: title.trim() })
                  .then(refresh)
                  .then(() => setManage(null))
                  .catch(report);
              }}
            >
              {uiText("保存书名")}
            </button>
            <button
              className="rd-secondary"
              disabled={manage.size > 20 * 1024 * 1024}
              onClick={() => {
                void library
                  .get(manage.id)
                  .then(async (stored) => {
                    if (!stored) throw new Error("missing_book");
                    await myco!.download(
                      stored.name,
                      stored.file,
                      stored.format === "pdf"
                        ? "application/pdf"
                        : "text/plain",
                    );
                  })
                  .catch(report);
              }}
            >
              <Download size={16} />
              {uiText("保存原文件")}
            </button>
            {manage.size > 20 * 1024 * 1024 && (
              <p className="rd-hint">
                {uiText(
                  "此文件超过应用的 20 MB 保存上限，请自行保留导入时的原文件。",
                )}
              </p>
            )}
            <div className="rd-delete-section">
              {deleting ? (
                <>
                  <p>
                    {uiText(
                      "从本机书架移除这本书和它的进度、书签？原始文件不会被删除。",
                    )}
                  </p>
                  <div className="rd-dialog-actions">
                    <button
                      className="rd-secondary"
                      onClick={() => setDeleting(false)}
                    >
                      {uiText("取消")}
                    </button>
                    <button
                      className="rd-danger"
                      onClick={() => {
                        void library
                          .remove(manage.id)
                          .then(refresh)
                          .then(() => setManage(null))
                          .catch(report);
                      }}
                    >
                      {uiText("确认移除")}
                    </button>
                  </div>
                </>
              ) : (
                <button
                  className="rd-quiet rd-danger-text"
                  onClick={() => setDeleting(true)}
                >
                  <Trash2 size={16} />
                  {uiText("从书架移除")}
                </button>
              )}
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
async function start() {
  const root = createRoot(document.getElementById("root")!);
  try {
    if (!myco) throw new Error("bridge");
    const ctx = await myco.ready;
    const scope = JSON.stringify([ctx.page.id, ctx.me.name]);
    root.render(<App library={new Library(scope)} />);
  } catch {
    root.render(
      <div className="rd-loading">
        {uiText("请在 myco App 中打开阅读应用。")}
      </div>,
    );
  }
}
void start();
