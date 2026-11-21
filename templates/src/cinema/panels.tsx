// Catalog cards and source settings. Posters are requested only near the
// viewport and become local blobs; source changes go through edit.ts.
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Film, MoreHorizontal, Pencil, Plus, Trash2, X } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { Popover, MenuItem, rectOf } from "../_lib/ui";
import { useKit } from "../_lib/appkit";
import { sourceURL, type SavedMovie, type Source } from "./catalog";
import { sources, kit, sourceName } from "./model";
import { removeSource, saveSource } from "./edit";
import { catalog, posterURL } from "./network";
export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null),
    close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const el = ref.current!,
      before = document.activeElement as HTMLElement;
    el.showModal();
    return () => {
      el.close();
      before?.focus?.();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={`ci-modal${wide ? " ci-wide" : ""}`}
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        close.current();
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          const r = e.currentTarget.getBoundingClientRect();
          if (
            e.clientX < r.left ||
            e.clientX > r.right ||
            e.clientY < r.top ||
            e.clientY > r.bottom
          )
            close.current();
        }
      }}
    >
      <header className="ci-modal-head">
        <h2>{title}</h2>
        <span className="top-gap" />
        <button
          className="icon-btn"
          title={uiText("关闭")}
          aria-label={uiText("关闭")}
          onClick={onClose}
        >
          <X className="ico" />
        </button>
      </header>
      {children}
    </dialog>
  );
}
let activePosters = 0;
const posterQueue: (() => void)[] = [];
async function queuedPoster(url: string) {
  if (activePosters >= 3) await new Promise<void>((r) => posterQueue.push(r));
  activePosters++;
  try {
    return await posterURL(url);
  } finally {
    activePosters--;
    posterQueue.shift()?.();
  }
}
export function Poster({ movie }: { movie: SavedMovie }) {
  const ref = useRef<HTMLDivElement>(null),
    [image, setImage] = useState("");
  useEffect(() => {
    let live = true,
      blob = "",
      requested = false;
    setImage("");
    const load = () => {
      if (!live || requested || !movie.poster) return;
      requested = true;
      void queuedPoster(movie.poster)
        .then((url) => {
          blob = url;
          if (live) setImage(url);
          else URL.revokeObjectURL(url);
        })
        .catch(() => {});
    };
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          load();
          observer.disconnect();
        }
      },
      { rootMargin: "100px" },
    );
    if (ref.current) observer.observe(ref.current);
    return () => {
      live = false;
      observer.disconnect();
      if (blob) URL.revokeObjectURL(blob);
    };
  }, [movie.poster]);
  return (
    <div className="ci-poster" ref={ref}>
      {image ? <img src={image} alt="" /> : <Film aria-hidden="true" />}
      <span>{movie.remarks}</span>
    </div>
  );
}
export function FilmCard({
  movie,
  onOpen,
  onRemove,
  progress,
}: {
  movie: SavedMovie;
  onOpen: () => void;
  onRemove?: () => void;
  progress?: string;
}) {
  const [menu, setMenu] = useState<ReturnType<typeof rectOf> | null>(null);
  return (
    <article
      className="ci-film"
      onContextMenu={(e) => {
        e.preventDefault();
        setMenu({ x: e.clientX, y: e.clientY, w: 0, h: 0 });
      }}
    >
      <button className="ci-film-open" onClick={onOpen}>
        <Poster movie={movie} />
        <strong>{movie.title}</strong>
        <span className="ci-film-meta">
          {[movie.year, movie.category].filter(Boolean).join(" · ") || "—"}
        </span>
        <span className="ci-film-meta">
          {progress || sourceName(movie.source) || uiText("影视源已移除")}
        </span>
      </button>
      <button
        className="icon-btn ci-card-menu"
        title={uiText("更多操作")}
        aria-label={uiText("更多操作")}
        onClick={(e) => setMenu(rectOf(e.currentTarget))}
      >
        <MoreHorizontal className="ico" />
      </button>
      {menu && (
        <Popover at={menu} onClose={() => setMenu(null)}>
          <div className="menu">
            <MenuItem
              onClick={() => {
                setMenu(null);
                onOpen();
              }}
            >
              {uiText("打开影片")}
            </MenuItem>
            {onRemove && (
              <MenuItem
                icon={<Trash2 className="ico" />}
                onClick={() => {
                  setMenu(null);
                  onRemove();
                }}
              >
                {uiText("删除")}
              </MenuItem>
            )}
          </div>
        </Popover>
      )}
    </article>
  );
}
export function Sources({ onClose }: { onClose: () => void }) {
  useKit(kit);
  const [editing, setEditing] = useState<string | null>(
      sources.rows.length ? null : "",
    ),
    [name, setName] = useState(""),
    [endpoint, setEndpoint] = useState(""),
    [enabled, setEnabled] = useState(true),
    [error, setError] = useState(""),
    [testing, setTesting] = useState(false),
    [tested, setTested] = useState(false),
    [menu, setMenu] = useState<{
      id: string;
      at: ReturnType<typeof rectOf>;
    } | null>(null);
  const generation = useRef(0);
  useEffect(
    () => () => {
      generation.current++;
    },
    [],
  );
  const edit = (id: string) => {
    generation.current++;
    const data = sources.get(id)?.data;
    setEditing(id);
    setName(data?.name || "");
    setEndpoint(data?.endpoint || "");
    setEnabled(data?.enabled !== false);
    setError("");
    setTested(false);
    setTesting(false);
  };
  const data = (): Source => ({
    name: name.trim(),
    endpoint: sourceURL(endpoint),
    enabled,
    order: sources.get(editing || "")?.data.order || Date.now(),
  });
  const test = async () => {
    const n = ++generation.current;
    setTesting(true);
    setTested(false);
    setError("");
    try {
      const source = data();
      await catalog("test", source);
      if (generation.current === n) setTested(true);
    } catch (e) {
      if (generation.current === n)
        setError(
          e instanceof Error
            ? e.message
            : uiText("影视源连接失败，请检查地址后重试"),
        );
    } finally {
      if (generation.current === n) setTesting(false);
    }
  };
  return (
    <Modal title={uiText("影视源")} onClose={onClose}>
      <div className="ci-source-body">
        <p className="ci-hint">
          {uiText(
            "添加自己的 MacCMS JSON 接口，不预设任何影视源。影视源与待看清单由此页面共享，请勿填写密钥。",
          )}
        </p>
        <div className="ci-source-list">
          {sources.rows.map((r) => (
            <div
              className="ci-source-row"
              key={r.id}
              onContextMenu={(e) => {
                e.preventDefault();
                setMenu({
                  id: r.id,
                  at: { x: e.clientX, y: e.clientY, w: 0, h: 0 },
                });
              }}
            >
              <div>
                <strong>{r.data.name}</strong>
                <span>
                  {r.data.enabled === false
                    ? uiText("已停用")
                    : uiText("已启用")}{" "}
                  ·{" "}
                  {(() => {
                    try {
                      return new URL(r.data.endpoint).hostname;
                    } catch {
                      return uiText("接口地址无效");
                    }
                  })()}
                </span>
              </div>
              <button
                className="icon-btn"
                title={uiText("编辑影视源")}
                aria-label={uiText("编辑影视源")}
                onClick={() => edit(r.id)}
              >
                <Pencil className="ico" />
              </button>
              <button
                className="icon-btn"
                title={uiText("更多操作")}
                aria-label={uiText("更多操作")}
                onClick={(e) =>
                  setMenu({ id: r.id, at: rectOf(e.currentTarget) })
                }
              >
                <MoreHorizontal className="ico" />
              </button>
              {menu?.id === r.id && (
                <div className="menu ci-source-menu" role="menu">
                  <MenuItem
                    onClick={() => {
                      edit(r.id);
                      setMenu(null);
                    }}
                  >
                    {uiText("编辑")}
                  </MenuItem>
                  <MenuItem
                    onClick={() => {
                      removeSource(r.id);
                      if (editing === r.id) setEditing(null);
                      setMenu(null);
                    }}
                  >
                    {uiText("删除")}
                  </MenuItem>
                  <MenuItem onClick={() => setMenu(null)}>
                    {uiText("关闭")}
                  </MenuItem>
                </div>
              )}
            </div>
          ))}
        </div>
        {editing === null ? (
          <button className="btn btn-primary" onClick={() => edit("")}>
            <Plus className="ico" />
            {uiText("添加影视源")}
          </button>
        ) : (
          <form
            className="ci-source-form"
            onSubmit={(e) => {
              e.preventDefault();
              try {
                saveSource(data(), editing || undefined);
                generation.current++;
                setEditing(null);
              } catch (err) {
                setError(
                  err instanceof Error ? err.message : uiText("没保存上"),
                );
              }
            }}
          >
            <h3>{editing ? uiText("编辑影视源") : uiText("添加影视源")}</h3>
            <label>
              {uiText("名称")}
              <input
                className="inp"
                value={name}
                maxLength={80}
                required
                autoFocus
                aria-label={uiText("影视源名称")}
                onChange={(e) => setName(e.target.value)}
                placeholder={uiText("例如：我的片库")}
              />
            </label>
            <label>
              {uiText("接口地址")}
              <input
                className="inp"
                type="url"
                value={endpoint}
                maxLength={2000}
                required
                aria-label={uiText("接口地址")}
                onChange={(e) => {
                  generation.current++;
                  setTesting(false);
                  setEndpoint(e.target.value);
                  setTested(false);
                  setError("");
                }}
                placeholder="https://example.com/api.php/provide/vod/"
              />
            </label>
            <p className="ci-hint">
              {uiText(
                "支持 MacCMS 兼容的 JSON 接口和 HLS 播放线路，不是网站首页地址。",
              )}
            </p>
            <label className="ci-check">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              {uiText("启用这个影视源")}
            </label>
            {error && (
              <p className="ci-error" role="alert">
                {error}
              </p>
            )}
            <div className="ci-actions">
              <span className="ci-hint" role="status">
                {testing
                  ? uiText("正在测试连接…")
                  : tested
                    ? uiText("接口可用")
                    : ""}
              </span>
              <span className="top-gap" />
              <button
                className="btn"
                type="button"
                onClick={() => {
                  generation.current++;
                  setEditing(null);
                }}
              >
                {uiText("取消")}
              </button>
              <button
                className="btn"
                type="button"
                disabled={!endpoint.trim() || testing}
                onClick={() => void test()}
              >
                {uiText("测试连接")}
              </button>
              <button
                className="btn btn-primary"
                type="submit"
                disabled={!name.trim() || !endpoint.trim() || testing}
              >
                {uiText("保存影视源")}
              </button>
            </div>
          </form>
        )}
      </div>
    </Modal>
  );
}
