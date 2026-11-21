// Cinema composition: catalog browsing, cross-source search, shared watchlist
// and personal resume history. No source is selected or contacted on first use.
import "../_lib/sheet.css";
import "./cinema.css";
import { useEffect, useRef, useState } from "react";
import { Film, Search, Settings2, Star, X } from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { networkState, networkStateText } from "../_lib/net";
import { t as uiText } from "../_lib/i18n";
import { kit, sources, sourceName, watchlist } from "./model";
import { catalog } from "./network";
import { FilmCard, Modal, Sources } from "./panels";
import { Player } from "./Player";
import { forget, history, remember } from "./local";
import { removeFavorite, toggleFavorite } from "./edit";
import {
  clock,
  matchEpisode,
  movieKey,
  savedMovie,
  type Catalog,
  type Movie,
  type Progress,
  type SavedMovie,
} from "./catalog";
function App() {
  useKit(kit);
  const enabled = sources.rows.filter((r) => r.data.enabled !== false),
    stamp = JSON.stringify(sources.rows.map((r) => [r.id, r.data]));
  const [tab, setTab] = useState("discover"),
    [source, setSource] = useState(""),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [category, setCategory] = useState(""),
    [page, setPage] = useState(1),
    [settings, setSettings] = useState(false);
  const active =
    source === "all"
      ? "all"
      : enabled.some((r) => r.id === source)
        ? source
        : enabled[0]?.id || "";
  const [result, setResult] = useState<Catalog>({
      movies: [],
      categories: [],
      page: 1,
      pages: 1,
    }),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [partial, setPartial] = useState(""),
    [refresh, setRefresh] = useState(0);
  const [selected, setSelected] = useState<Movie | null>(null),
    [opening, setOpening] = useState<SavedMovie | null>(null),
    [openError, setOpenError] = useState("");
  const [line, setLine] = useState(0),
    [episode, setEpisode] = useState(0),
    [resume, setResume] = useState(0),
    [intro, setIntro] = useState(0),
    [outro, setOutro] = useState(0),
    [recent, setRecent] = useState<Progress[]>(history);
  const openingGeneration = useRef(0),
    currentProgress = useRef({ seconds: 0, duration: 0 });
  useEffect(() => {
    if (
      tab !== "discover" ||
      !active ||
      networkState !== "ready" ||
      (active === "all" && !search)
    ) {
      setBusy(false);
      setResult({ movies: [], categories: [], page: 1, pages: 1 });
      return;
    }
    let live = true;
    setBusy(true);
    setError("");
    setPartial("");
    setResult((r) => ({ ...r, movies: [] }));
    const targets = enabled.filter((r) => active === "all" || r.id === active);
    // Three concurrent source requests at most, even with a long source list.
    const batches: PromiseSettledResult<Catalog>[] = [];
    void (async () => {
      for (let i = 0; i < targets.length && live; i += 3) {
        const batch = await Promise.allSettled(
          targets.slice(i, i + 3).map((r) =>
            catalog(r.id, r.data, {
              query: search,
              category: search ? "" : category,
              page,
            }),
          ),
        );
        batches.push(...batch);
      }
      if (!live) return;
      const good = batches.flatMap((r) =>
          r.status === "fulfilled" ? [r.value] : [],
        ),
        failed = batches.filter((r) => r.status === "rejected").length;
      if (!good.length) {
        setError(uiText("影视源连接失败，请检查地址后重试"));
        setResult({ movies: [], categories: [], page: 1, pages: 1 });
      } else {
        setResult({
          movies: good.flatMap((r) => r.movies),
          categories: active === "all" ? [] : good[0].categories,
          page,
          pages: Math.max(...good.map((r) => r.pages)),
        });
        if (failed)
          setPartial(
            uiText("{0} 个影视源暂时无法连接，已显示其余结果", failed),
          );
      }
      setBusy(false);
    })();
    return () => {
      live = false;
    };
  }, [tab, active, search, category, page, stamp, refresh]);
  useEffect(() => {
    // Editing a source invalidates detail requests and stops its old stream.
    openingGeneration.current++;
    setSelected(null);
    setOpening(null);
    setOpenError("");
  }, [stamp]);
  useEffect(
    () => () => {
      openingGeneration.current++;
    },
    [],
  );
  const closeMovie = () => {
    openingGeneration.current++;
    setSelected(null);
    setOpening(null);
    setOpenError("");
    setRecent(history());
  };
  const open = async (movie: SavedMovie) => {
    const n = ++openingGeneration.current;
    setOpening(movie);
    setSelected(null);
    setOpenError("");
    const source = sources.get(movie.source);
    if (!source || source.data.enabled === false) {
      setOpenError(
        uiText("这个影视源已移除或停用，请在影视源设置中重新添加或启用"),
      );
      return;
    }
    try {
      const data = await catalog(source.id, source.data, {
        id: movie.movie_id,
      });
      if (n !== openingGeneration.current) return;
      const detail = data.movies.find((m) => m.movie_id === movie.movie_id);
      if (!detail)
        throw new Error(uiText("影视源已找不到这部影片，请重新搜索"));
      const progress = history().find(
        (h) => movieKey(h.movie) === movieKey(movie),
      );
      const li = Math.max(
        0,
        detail.lines.findIndex((l) => l.name === progress?.line),
      );
      const matched = progress
        ? matchEpisode(detail.lines[li]?.episodes || [], progress.episode)
        : -1;
      setLine(li);
      setEpisode(matched >= 0 ? matched : 0);
      setResume(matched >= 0 ? progress!.seconds : 0);
      setIntro(progress?.intro || 0);
      setOutro(progress?.outro || 0);
      currentProgress.current = { seconds: 0, duration: 0 };
      setSelected(detail);
    } catch {
      if (n === openingGeneration.current)
        setOpenError(uiText("影片载入失败，请重试或换个影视源"));
    }
  };
  const currentLine = selected?.lines[line],
    current = currentLine?.episodes[episode];
  const saveProgress = (
    seconds: number,
    duration: number,
    skipIntro = intro,
    skipOutro = outro,
  ) => {
    if (!selected || !current || !currentLine) return;
    currentProgress.current = { seconds, duration };
    remember({
      movie: savedMovie(selected),
      line: currentLine.name,
      episode: current.name,
      index: episode,
      seconds,
      duration,
      at: Date.now(),
      intro: skipIntro,
      outro: skipOutro,
    });
  };
  const pickEpisode = (index: number) => {
    setEpisode(index);
    setResume(0);
    currentProgress.current = { seconds: 0, duration: 0 };
  };
  const favorite =
    selected &&
    watchlist.rows.some((r) => movieKey(r.data) === movieKey(selected));
  const library =
    tab === "watchlist"
      ? watchlist.rows
          .slice()
          .sort((a, b) => b.data.order - a.data.order)
          .map((r) => ({
            movie: r.data as SavedMovie,
            remove: () => removeFavorite(r.id),
            progress: "",
          }))
      : recent.map((p) => ({
          movie: p.movie,
          remove: () => {
            forget(p);
            setRecent(history());
            kit.flash(uiText("已删除播放记录"), () => {
              remember(p);
              setRecent(history());
            });
          },
          progress: `${p.episode} · ${clock(p.seconds)} / ${clock(p.duration)}`,
        }));
  return (
    <div className="ci-app">
      <header className="top">
        <h1 className="ci-title">{uiText("影视")}</h1>
        <span className="ci-tagline">{uiText("你的片库，自己的影视源")}</span>
        <span className="top-gap" />
        <button className="btn" onClick={() => setSettings(true)}>
          <Settings2 className="ico" />
          {uiText("影视源")}
          {sources.rows.length > 0 && (
            <span className="faint">{sources.rows.length}</span>
          )}
        </button>
      </header>
      <div className="ci-navigation">
        <div className="ci-tabs" role="tablist" aria-label={uiText("影视内容")}>
          {[
            ["discover", uiText("发现")],
            ["watchlist", uiText("待看")],
            ["history", uiText("最近播放")],
          ].map(([id, name]) => (
            <button
              role="tab"
              aria-selected={tab === id}
              className={tab === id ? "is-on" : ""}
              key={id}
              onClick={() => {
                setTab(id);
                setRecent(history());
              }}
            >
              {name}
              {id === "watchlist" && watchlist.rows.length > 0 && (
                <span>{watchlist.rows.length}</span>
              )}
            </button>
          ))}
        </div>
        <span className="ci-hint ci-private">
          {uiText("播放进度仅保存在这台电脑")}
        </span>
      </div>
      {networkState !== "ready" && (
        <div className="ci-notice" role="status">
          {networkStateText()}
        </div>
      )}
      <main className="ci-content">
        {tab === "discover" && (
          <>
            {enabled.length > 0 && (
              <form
                className="ci-filters"
                onSubmit={(e) => {
                  e.preventDefault();
                  setSearch(query.trim());
                  setPage(1);
                  setCategory("");
                  setRefresh((n) => n + 1);
                }}
              >
                <select
                  className="sel ci-source-select"
                  aria-label={uiText("选择影视源")}
                  value={active}
                  onChange={(e) => {
                    setSource(e.target.value);
                    setCategory("");
                    setPage(1);
                  }}
                >
                  <option value="all">{uiText("所有影视源")}</option>
                  {enabled.map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.data.name}
                    </option>
                  ))}
                </select>
                <div className="ci-search">
                  <Search className="ico" />
                  <input
                    className="inp"
                    aria-label={uiText("搜索影片")}
                    placeholder={uiText("搜索电影、剧集")}
                    maxLength={100}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                  {(query || search) && (
                    <button
                      className="icon-btn"
                      type="button"
                      title={uiText("清除搜索")}
                      aria-label={uiText("清除搜索")}
                      onClick={() => {
                        setQuery("");
                        setSearch("");
                        setPage(1);
                      }}
                    >
                      <X className="ico" />
                    </button>
                  )}
                </div>
                <button
                  className="btn"
                  type="submit"
                  disabled={networkState !== "ready" || busy}
                >
                  {uiText("搜索")}
                </button>
                {active !== "all" &&
                  result.categories.length > 0 &&
                  !search && (
                    <select
                      className="sel"
                      aria-label={uiText("影片分类")}
                      value={category}
                      onChange={(e) => {
                        setCategory(e.target.value);
                        setPage(1);
                      }}
                    >
                      <option value="">{uiText("全部分类")}</option>
                      {result.categories.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  )}
              </form>
            )}
            {!enabled.length ? (
              <div className="ci-empty">
                <Film aria-hidden="true" />
                <h2>
                  {sources.rows.length
                    ? uiText("还没有启用的影视源")
                    : uiText("先添加你的影视源")}
                </h2>
                <p>
                  {uiText(
                    "接入自己的片库后，就能搜索影片、选择集数并继续观看。",
                  )}
                </p>
                <button
                  className="btn btn-primary"
                  onClick={() => setSettings(true)}
                >
                  {sources.rows.length
                    ? uiText("管理影视源")
                    : uiText("添加影视源")}
                </button>
                <span className="ci-hint">
                  {uiText("支持 MacCMS JSON 接口，不内置影视源")}
                </span>
              </div>
            ) : busy ? (
              <div className="ci-empty" role="status">
                <p>
                  {search ? uiText("正在搜索影片…") : uiText("正在载入片库…")}
                </p>
                <button
                  className="btn"
                  onClick={() => {
                    setTab("watchlist");
                  }}
                >
                  {uiText("先看待看清单")}
                </button>
              </div>
            ) : error ? (
              <div className="ci-empty" role="alert">
                <h2>{uiText("暂时连不上影视源")}</h2>
                <p>{error}</p>
                <div className="ci-actions">
                  <button className="btn" onClick={() => setSettings(true)}>
                    {uiText("管理影视源")}
                  </button>
                  <button
                    className="btn btn-primary"
                    onClick={() => setRefresh((n) => n + 1)}
                  >
                    {uiText("重试")}
                  </button>
                </div>
              </div>
            ) : (
              <>
                {partial && (
                  <p className="ci-notice" role="status">
                    {partial}
                  </p>
                )}
                {result.movies.length ? (
                  <>
                    <div className="ci-section-head">
                      <h2>
                        {search
                          ? uiText("「{0}」的搜索结果", search)
                          : uiText("片库更新")}
                      </h2>
                      <span className="ci-hint">
                        {uiText("第 {0} 页", page)}
                      </span>
                    </div>
                    <div className="ci-grid">
                      {result.movies.map((m) => (
                        <FilmCard
                          key={movieKey(m)}
                          movie={m}
                          onOpen={() => void open(m)}
                        />
                      ))}
                    </div>
                    <div className="ci-pagination">
                      <button
                        className="btn"
                        disabled={page <= 1}
                        onClick={() => setPage((p) => p - 1)}
                      >
                        {uiText("上一页")}
                      </button>
                      <span>
                        {uiText("第 {0} / {1} 页", page, result.pages)}
                      </span>
                      <button
                        className="btn"
                        disabled={page >= result.pages}
                        onClick={() => setPage((p) => p + 1)}
                      >
                        {uiText("下一页")}
                      </button>
                    </div>
                  </>
                ) : (
                  <div className="ci-empty">
                    <Search aria-hidden="true" />
                    <h2>
                      {active === "all" && !search
                        ? uiText("搜索所有影视源")
                        : uiText("没有找到影片")}
                    </h2>
                    <p>
                      {active === "all" && !search
                        ? uiText("输入片名，在已启用的影视源里一起搜索。")
                        : uiText("换个关键词、分类或影视源再试试。")}
                    </p>
                  </div>
                )}
              </>
            )}
          </>
        )}
        {tab !== "discover" && (
          <>
            <div className="ci-section-head">
              <h2>
                {tab === "watchlist"
                  ? uiText("留给下一次的好片")
                  : uiText("接着上次看")}
              </h2>
              <span className="ci-hint">
                {tab === "watchlist"
                  ? uiText("与此页面共享")
                  : uiText("仅保存在这台电脑")}
              </span>
            </div>
            {library.length ? (
              <div className="ci-grid">
                {library.map(({ movie, remove, progress }) => (
                  <FilmCard
                    key={movieKey(movie)}
                    movie={movie}
                    progress={progress}
                    onOpen={() => void open(movie)}
                    onRemove={remove}
                  />
                ))}
              </div>
            ) : (
              <div className="ci-empty">
                <Film aria-hidden="true" />
                <h2>
                  {tab === "watchlist"
                    ? uiText("待看清单还是空的")
                    : uiText("还没有播放记录")}
                </h2>
                <p>
                  {tab === "watchlist"
                    ? uiText("打开一部影片，点「加入待看」留到下次。")
                    : uiText("开始观看后，会在这里记住集数和进度。")}
                </p>
                <button className="btn" onClick={() => setTab("discover")}>
                  {uiText("去发现影片")}
                </button>
              </div>
            )}
          </>
        )}
      </main>
      {settings && <Sources onClose={() => setSettings(false)} />}
      {opening && !settings && (
        <Modal
          title={selected?.title || opening.title}
          onClose={closeMovie}
          wide
        >
          {!selected ? (
            <div className="ci-empty" role={openError ? "alert" : "status"}>
              <p>{openError || uiText("正在载入影片…")}</p>
              {openError && (
                <button className="btn" onClick={() => void open(opening)}>
                  {uiText("重试")}
                </button>
              )}
            </div>
          ) : (
            <>
              <div className="ci-detail-meta">
                <span>
                  {[
                    sourceName(selected.source),
                    selected.year,
                    selected.area,
                    selected.category,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
                <button
                  className="btn"
                  aria-pressed={!!favorite}
                  onClick={() => toggleFavorite(selected)}
                >
                  <Star
                    className="ico"
                    fill={favorite ? "currentColor" : "none"}
                  />
                  {favorite ? uiText("已加入待看") : uiText("加入待看")}
                </button>
              </div>
              <div className="ci-detail-grid">
                <div>
                  {current ? (
                    <Player
                      key={`${line}:${episode}`}
                      url={current.url}
                      resume={resume}
                      intro={intro}
                      outro={outro}
                      previous={
                        episode > 0 ? () => pickEpisode(episode - 1) : undefined
                      }
                      next={
                        episode < (currentLine?.episodes.length || 0) - 1
                          ? () => pickEpisode(episode + 1)
                          : undefined
                      }
                      onProgress={saveProgress}
                      onSkip={(a, b) => {
                        setIntro(a);
                        setOutro(b);
                        const p = currentProgress.current;
                        saveProgress(p.seconds, p.duration, a, b);
                      }}
                    />
                  ) : (
                    <div className="ci-empty">
                      <p>
                        {uiText(
                          "这部影片还没有可用的播放线路，请换个影视源搜索。",
                        )}
                      </p>
                    </div>
                  )}
                  <div className="ci-description">
                    <h3>{uiText("剧情简介")}</h3>
                    <p>{selected.description || uiText("暂无简介")}</p>
                    {selected.actors && (
                      <p className="ci-hint">
                        {uiText("主演：{0}", selected.actors)}
                      </p>
                    )}
                    {selected.director && (
                      <p className="ci-hint">
                        {uiText("导演：{0}", selected.director)}
                      </p>
                    )}
                  </div>
                </div>
                <aside className="ci-episodes">
                  <label className="ci-line-label">
                    {uiText("播放线路")}
                    <select
                      className="sel"
                      aria-label={uiText("播放线路")}
                      value={line}
                      onChange={(e) => {
                        const li = Number(e.target.value),
                          matched = matchEpisode(
                            selected.lines[li]?.episodes || [],
                            current?.name || "",
                          );
                        if (matched < 0)
                          kit.flash(uiText("新线路没有同名集数，已选第一集"));
                        setLine(li);
                        setEpisode(Math.max(0, matched));
                        setResume(
                          matched >= 0 ? currentProgress.current.seconds : 0,
                        );
                      }}
                    >
                      {selected.lines.map((l, i) => (
                        <option key={i} value={i}>
                          {l.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <div className="ci-section-head">
                    <h3>{uiText("选集")}</h3>
                    <span className="ci-hint">
                      {uiText("共 {0} 集", currentLine?.episodes.length || 0)}
                    </span>
                  </div>
                  <div className="ci-episode-list">
                    {currentLine?.episodes.map((e, i) => (
                      <button
                        className={`btn${i === episode ? " is-current" : ""}`}
                        key={i}
                        aria-pressed={i === episode}
                        title={e.name}
                        onClick={() => pickEpisode(i)}
                      >
                        {e.name}
                      </button>
                    ))}
                  </div>
                  <p className="ci-hint">
                    {uiText("优先选择 HLS 线路；空格暂停，左右键快进或快退。")}
                  </p>
                </aside>
              </div>
            </>
          )}
        </Modal>
      )}
      <Toast kit={kit} />
    </div>
  );
}
mount(kit, App);
