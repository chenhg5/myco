// Composes regional discovery, a reusable audio element and a device-only library.
import "../_lib/sheet.css";
import "./podcast.css";
import { useEffect, useRef, useState } from "react";
import {
  Headphones,
  Heart,
  Play,
  Pause,
  SkipForward,
  Radio,
  Search,
  Bookmark,
  Globe2,
} from "lucide-react";
import { mount, useKit, Toast } from "../_lib/appkit";
import { t as uiText, language } from "../_lib/i18n";
import { network, networkState, networkStateText } from "../_lib/net";
import { fromBase64 } from "../_lib/os";
import { kit, stations } from "./model";
import { Globe } from "./globe";
import { REGIONS } from "./regions";
import {
  searchShows,
  publicURL,
  type DirectoryShow,
  type Market,
} from "./directory";
import { fetchFeed, type Episode } from "./feed";
import { canStream, mediaAddress, releaseAudio } from "./player";
import {
  readListening,
  saveListening,
  listeningKey,
  rememberProgress,
  episodeKey,
  type ListeningState,
} from "./local";
function storage() {
  try {
    return localStorage;
  } catch {
    return null;
  }
}
function time(n: number) {
  if (!Number.isFinite(n) || n < 0) return "0:00";
  return `${Math.floor(n / 60)}:${String(Math.floor(n % 60)).padStart(2, "0")}`;
}
function Cover({ url }: { url: string }) {
  const [src, setSrc] = useState("");
  useEffect(() => {
    let active = true,
      object = "";
    setSrc("");
    if (publicURL(url))
      void network()
        .fetch({
          url,
          response: "base64",
          max_bytes: 1024 * 1024,
          timeout_ms: 10000,
        })
        .then((r) => {
          const type = (r.headers["content-type"] || "").split(";")[0];
          if (
            !active ||
            r.status !== 200 ||
            r.truncated ||
            !["image/jpeg", "image/png", "image/webp", "image/gif"].includes(
              type,
            )
          )
            return;
          object = URL.createObjectURL(
            new Blob([fromBase64(r.body) as BlobPart], { type }),
          );
          setSrc(object);
        })
        .catch(() => {});
    return () => {
      active = false;
      if (object) URL.revokeObjectURL(object);
    };
  }, [url]);
  return (
    <div className="pg-cover">
      {src ? <img src={src} alt="" /> : <Headphones aria-hidden="true" />}
    </div>
  );
}
function App() {
  useKit(kit);
  const key = listeningKey(kit.page, kit.me),
    [library, setLibrary] = useState(() => readListening(storage(), key));
  const libraryRef = useRef(library);
  libraryRef.current = library;
  const [market, setMarket] = useState<Market>(
      language() === "zh" ? "cn" : "us",
    ),
    [topic, setTopic] = useState(0),
    [tab, setTab] = useState("radio");
  const [query, setQuery] = useState(""),
    [shows, setShows] = useState<DirectoryShow[]>([]),
    [episodes, setEpisodes] = useState<Episode[]>([]),
    [episode, setEpisode] = useState<Episode | null>(null);
  const [busy, setBusy] = useState(false),
    [notice, setNotice] = useState(""),
    [show, setShow] = useState<DirectoryShow | null>(null);
  const [playing, setPlaying] = useState(false),
    [position, setPosition] = useState(0),
    [duration, setDuration] = useState(0),
    [speed, setSpeed] = useState(1),
    [sleep, setSleep] = useState(0);
  const audio = useRef<HTMLAudioElement>(null),
    current = useRef<Episode | null>(null),
    sequence = useRef(0),
    abort = useRef<AbortController | null>(null);
  const region = REGIONS.find((r) => r.id === market)!;
  const change = (next: ListeningState) => {
    libraryRef.current = next;
    setLibrary(next);
    if (!saveListening(storage(), key, next))
      setNotice(uiText("此设备暂时无法保存收听记录"));
  };
  const persist = () => {
    const a = audio.current,
      e = current.current;
    if (a && e && a.currentTime > 0)
      change(rememberProgress(libraryRef.current, e, a.currentTime));
  };
  useEffect(() => {
    const element = audio.current;
    return () => {
      abort.current?.abort();
      sequence.current++;
      if (element) releaseAudio(element);
    };
  }, []);
  useEffect(() => {
    window.addEventListener("pagehide", persist);
    return () => window.removeEventListener("pagehide", persist);
  }, [key]);
  useEffect(() => {
    if (!sleep) return;
    const id = window.setTimeout(() => {
      audio.current?.pause();
      setSleep(0);
      setNotice(uiText("睡眠定时已结束"));
    }, sleep * 60000);
    return () => window.clearTimeout(id);
  }, [sleep]);
  const stop = () => {
    persist();
    if (audio.current) releaseAudio(audio.current);
    current.current = null;
    setEpisode(null);
    setPlaying(false);
    setPosition(0);
    setDuration(0);
  };
  const loadEpisode = (e: Episode) => {
    persist();
    const a = audio.current!;
    releaseAudio(a);
    setNotice("");
    current.current = e;
    setEpisode(e);
    setPosition(0);
    setDuration(0);
    setPlaying(false);
    if (!canStream()) {
      setNotice(uiText("当前客户端不支持播客流式播放，请更新 App"));
      return;
    }
    try {
      a.src = mediaAddress(e.audioUrl);
      a.playbackRate = speed;
      a.load();
      void a
        .play()
        .catch(() => setNotice(uiText("节目已准备好，点播放开始收听")));
    } catch {
      setNotice(uiText("这个节目暂时无法播放，请换一期试试"));
    }
  };
  const loadShow = async (s: DirectoryShow) => {
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    const seq = ++sequence.current;
    stop();
    setShow(s);
    setEpisodes([]);
    setBusy(true);
    setNotice("");
    try {
      const feed = await fetchFeed(s.feedUrl, controller.signal);
      if (seq !== sequence.current) return;
      setShow({...s,title:feed.title || s.title,author:feed.author || s.author,artworkUrl:feed.artworkUrl || s.artworkUrl});
      setEpisodes(feed.episodes);
      if (!feed.episodes.length)
        setNotice(uiText("这个节目还没有可播放的公开单集"));
      else loadEpisode(feed.episodes[0]);
    } catch {
      if (seq === sequence.current && !controller.signal.aborted)
        setNotice(uiText("无法读取节目，请稍后重试或换一个节目"));
    } finally {
      if (seq === sequence.current) setBusy(false);
    }
  };
  const discover = async (term: string, country: Market, tune = false) => {
    if(tune){stop();setShow(null);setEpisodes([]);}
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    const seq = ++sequence.current;
    setBusy(true);
    setNotice("");
    setShows([]);
    try {
      const results = await searchShows(term, country, controller.signal);
      if (seq !== sequence.current) return;
      setShows(results);
      if(tune && results.length) void loadShow(results[Math.floor(Math.random()*results.length)]);
      if (!results.length)
        setNotice(uiText("暂时没有找到节目，换个关键词试试"));
    } catch {
      if (seq === sequence.current && !controller.signal.aborted)
        setNotice(uiText("目录暂时连接不上，请稍后重试"));
    } finally {
      if (seq === sequence.current) setBusy(false);
    }
  };
  useEffect(() => {
    if (networkState === "ready") void discover(region.queries[topic], market, true);
  }, [market, topic]);
  const chooseRegion = (id: string) => {
    if (REGIONS.some((r) => r.id === id)) {
      stop();
      setShow(null);
      setEpisodes([]);
      if(id===market)void discover(region.queries[topic],market,true);
      else setMarket(id as Market);
      setQuery("");
      setTab("radio");
    }
  };
  const next = () => {
    if (episode) {
      const i = episodes.findIndex(
        (e) => episodeKey(e) === episodeKey(episode),
      );
      if (i >= 0 && episodes[i + 1]) {
        loadEpisode(episodes[i + 1]);
        return;
      }
    }
    const other = shows.filter((s) => s.feedUrl !== show?.feedUrl);
    if (other.length)
      void loadShow(other[Math.floor(Math.random() * other.length)]);
    else setNotice(uiText("这一轮听完了，换个频道继续探索"));
  };
  const toggle = () => {
    const a = audio.current;
    if (!a || !episode || !canStream()) return;
    if (a.paused)
      void a
        .play()
        .catch(() => setNotice(uiText("这个节目暂时无法播放，请换一期试试")));
    else a.pause();
  };
  const liked =
    episode && library.likes.some((e) => episodeKey(e) === episodeKey(episode));
  const followed =
    show && library.follows.some((s) => s.feedUrl === show.feedUrl);
  const toggleFollow = () => {
    if (show)
      change({
        ...libraryRef.current,
        follows: followed
          ? libraryRef.current.follows.filter((s) => s.feedUrl !== show.feedUrl)
          : [show, ...libraryRef.current.follows].slice(0, 100),
      });
  };
  const toggleLike = () => {
    if (episode)
      change({
        ...libraryRef.current,
        likes: liked
          ? libraryRef.current.likes.filter(
              (e) => episodeKey(e) !== episodeKey(episode),
            )
          : [episode, ...libraryRef.current.likes].slice(0, 100),
      });
  };
  return (
    <div className="pg-app">
      <header className="pg-header">
        <div className="pg-brand">
          <Radio />
          <strong>{uiText("地球播客")}</strong>
          <span>WORLD / VOICES</span>
        </div>
        <nav>
          {[
            ["radio", "探索地球"],
            ["library", "我的收藏"],
          ].map(([id, name]) => (
            <button
              className={tab === id ? "active" : ""}
              key={id}
              onClick={() => setTab(id)}
            >
              {uiText(name)}
            </button>
          ))}
        </nav>
      </header>
      <main className="pg-main">
        <section className="pg-earth">
          <div className="pg-heading">
            <span className="pg-eyebrow">
              {uiText("转动地球，遇见一种声音")}
            </span>
            <h1>{uiText("世界很大，听听别处。")}</h1>
            <p>{uiText("点亮一个地方，让故事带你出发。")}</p>
          </div>
          <Globe
            regions={REGIONS.map((r) => ({ ...r, label: uiText(r.label) }))}
            selected={market}
            onSelect={chooseRegion}
          />
          <p className="pg-map-note">
            <Globe2 size={14} />
            {uiText(
              "光点代表地区频道，不代表节目制作地；频道可能包含多种语言。",
            )}
          </p>
          <div className="pg-tuning">
            <label>
              {uiText("地区")}
              <select
                value={market}
                onChange={(e) => chooseRegion(e.target.value)}
              >
                {REGIONS.map((r) => (
                  <option value={r.id} key={r.id}>
                    {uiText(r.label)}
                  </option>
                ))}
              </select>
            </label>
            <div className="pg-topics">
              {["故事", "科技", "文化", "生活"].map((name, i) => (
                <button
                  key={i}
                  className={topic === i ? "active" : ""}
                  onClick={() => {
                    setTopic(i);
                    setTab("radio");
                  }}
                >
                  {uiText(name)}
                </button>
              ))}
            </div>
          </div>
        </section>
        <section className="pg-listening">
          <div className="pg-player">
            <div className="pg-now">
              <span className="pg-eyebrow">
                {episode
                  ? uiText("此刻，正在别处")
                  : uiText("下一站，听见世界")}
              </span>
              <span className="pg-region">{uiText(region.label)}</span>
            </div>
            <Cover url={episode?.artworkUrl || show?.artworkUrl || ""} />
            <h2>{episode?.title || uiText("给耳朵一次小旅行")}</h2>
            <p className="pg-show-name">
              {episode?.showTitle || uiText("选一个节目，或让好奇心替你选。")}
            </p>
            <div className="pg-actions">
              <button
                className={liked ? "pg-icon active" : "pg-icon"}
                aria-label={uiText("喜欢这一期")}
                aria-pressed={!!liked}
                disabled={!episode}
                onClick={toggleLike}
              >
                <Heart fill={liked ? "currentColor" : "none"} />
              </button>
              <button
                className="pg-play"
                aria-label={playing ? uiText("暂停") : uiText("播放")}
                disabled={busy || !canStream()}
                onClick={() => {
                  if (!episode && shows.length)
                    void loadShow(
                      shows[Math.floor(Math.random() * shows.length)],
                    );
                  else toggle();
                }}
              >
                {playing ? (
                  <Pause fill="currentColor" />
                ) : (
                  <Play fill="currentColor" />
                )}
              </button>
              <button
                className="pg-icon"
                aria-label={uiText("换一期")}
                disabled={busy || (!shows.length && !episodes.length)}
                onClick={next}
              >
                <SkipForward />
              </button>
            </div>
            <div className="pg-progress">
              <input
                type="range"
                aria-label={uiText("收听进度")}
                min="0"
                max={duration || 1}
                step="1"
                value={Math.min(position, duration || 1)}
                disabled={!duration}
                onChange={(e) => {
                  if (audio.current) {
                    audio.current.currentTime = Number(e.target.value);
                    setPosition(Number(e.target.value));
                  }
                }}
              />
              <div>
                <span>{time(position)}</span>
                <span>{time(duration)}</span>
              </div>
            </div>
            <div className="pg-player-options">
              <label>
                {uiText("倍速")}
                <select
                  value={speed}
                  onChange={(e) => {
                    setSpeed(Number(e.target.value));
                    if (audio.current)
                      audio.current.playbackRate = Number(e.target.value);
                  }}
                >
                  {[0.75, 1, 1.25, 1.5, 2].map((v) => (
                    <option key={v} value={v}>
                      {v}×
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {uiText("睡眠定时")}
                <select
                  value={sleep}
                  onChange={(e) => setSleep(Number(e.target.value))}
                >
                  {[0, 15, 30, 60].map((v) => (
                    <option key={v} value={v}>
                      {v ? uiText("{0} 分钟", v) : uiText("关闭")}
                    </option>
                  ))}
                </select>
              </label>
              <button
                onClick={toggleFollow}
                disabled={!show}
                aria-pressed={!!followed}
              >
                <Bookmark size={16} />
                {followed ? uiText("已订阅") : uiText("订阅")}
              </button>
            </div>
            {episode?.link && publicURL(episode.link) && (
              <a
                className="pg-source"
                href={publicURL(episode.link)}
                target="_blank"
                rel="noreferrer"
              >
                {uiText("节目原文与来源")}
              </a>
            )}
            {busy && <p role="status">{uiText("正在寻找声音…")}</p>}
            {notice && (
              <p className="pg-notice" role="status">
                {notice}
              </p>
            )}
            {networkState !== "ready" && (
              <p role="status">{networkStateText()}</p>
            )}
            {!canStream() && !notice && (
              <p className="pg-notice">
                {uiText("当前客户端不支持播客流式播放，请更新 App")}
              </p>
            )}
          </div>
        </section>
        <section className="pg-directory">
          <div className="pg-directory-title">
            <div>
              <span className="pg-eyebrow">
                {tab === "radio"
                  ? uiText("跟着好奇心走")
                  : uiText("把喜欢的声音留下")}
              </span>
              <h2>
                {tab === "radio" ? uiText("从这里开始听") : uiText("我的收藏")}
              </h2>
            </div>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                setTab("radio");
                if (publicURL(query))
                  void loadShow({
                    id: query,
                    title: uiText("公开 RSS"),
                    feedUrl: publicURL(query),
                    author: "",
                    artworkUrl: "",
                    directoryUrl: "",
                    genre: "",
                  });
                else void discover(query || region.queries[topic], market);
              }}
            >
              <Search size={18} />
              <input
                aria-label={uiText("搜索播客")}
                placeholder={uiText("搜索节目或粘贴 RSS 地址")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                maxLength={160}
              />
              <button type="submit">{uiText("搜索")}</button>
            </form>
          </div>
          {tab === "library" ? (
            <>
              <h3>{uiText("订阅的节目")}</h3>
              <div className="pg-show-grid">
                {library.follows.map((s) => (
                  <button key={s.feedUrl} onClick={() => void loadShow(s)}>
                    <span className="pg-tile">
                      <Headphones />
                    </span>
                    <strong>{s.title}</strong>
                    <small>{s.author}</small>
                  </button>
                ))}
              </div>
              <h3>{uiText("喜欢的单集")}</h3>
              {library.likes.map((e) => (
                <button
                  className="pg-episode"
                  key={episodeKey(e)}
                  onClick={() => {
                    setEpisodes(library.likes);
                    setShow(null);
                    loadEpisode(e);
                  }}
                >
                  <Play size={16} />
                  <span>
                    {e.title}
                    <small>{e.showTitle}</small>
                  </span>
                </button>
              ))}
              {!library.follows.length && !library.likes.length && (
                <p className="pg-empty">
                  {uiText("喜欢或订阅后，它们会留在这里。仅保存在此设备。")}
                </p>
              )}
            </>
          ) : (
            <>
              <div className="pg-show-grid">
                {shows.map((s) => (
                  <button key={s.feedUrl} onClick={() => void loadShow(s)}>
                    <span className="pg-tile">
                      <Headphones />
                    </span>
                    <strong>{s.title}</strong>
                    <small>{s.author}</small>
                    <span className="pg-genre">{s.genre}</span>
                  </button>
                ))}
              </div>
              {stations.rows
                .filter(
                  (r) => r.data.region === market && publicURL(r.data.feed_url),
                )
                .map((r) => (
                  <button
                    className="pg-episode"
                    key={r.id}
                    onClick={() =>
                      void loadShow({
                        id: r.id,
                        title: r.data.name,
                        feedUrl: r.data.feed_url,
                        author: "",
                        artworkUrl: "",
                        directoryUrl: "",
                        genre: r.data.topic,
                      })
                    }
                  >
                    {r.data.name}
                  </button>
                ))}
            </>
          )}
          {show && episodes.length > 0 && (
            <details>
              <summary>
                {uiText("节目单集")} · {show.title}
              </summary>
              {episodes.slice(0, 50).map((e) => (
                <button
                  className="pg-episode"
                  key={episodeKey(e)}
                  onClick={() => loadEpisode(e)}
                >
                  <Play size={16} />
                  <span>
                    {e.title}
                    <small>
                      {e.publishedAt ? e.publishedAt.slice(0, 10) : ""}
                    </small>
                  </span>
                  <small>{e.duration ? time(e.duration) : ""}</small>
                </button>
              ))}
              {show.directoryUrl && (
                <a href={show.directoryUrl} target="_blank" rel="noreferrer">
                  Apple Podcasts
                </a>
              )}
            </details>
          )}
        </section>
      </main>
      <footer className="pg-footer">
        {uiText(
          "声音来自发布者公开 RSS，目录由 Apple Podcasts 提供。收藏与进度仅留在此设备。",
        )}
      </footer>
      <Toast kit={kit} />
      <audio
        ref={audio}
        preload="metadata"
        onPlay={() => {
          setPlaying(true);
          setNotice("");
        }}
        onPause={() => {
          setPlaying(false);
          persist();
        }}
        onTimeUpdate={() => setPosition(audio.current?.currentTime || 0)}
        onDurationChange={() =>
          setDuration(
            Number.isFinite(audio.current?.duration)
              ? audio.current!.duration
              : 0,
          )
        }
        onLoadedMetadata={() => {
          const a = audio.current,
            e = current.current;
          if (a && e) {
            const saved =
              libraryRef.current.progress[episodeKey(e)]?.seconds || 0;
            if (saved > 0 && saved < a.duration - 3) a.currentTime = saved;
          }
        }}
        onEnded={next}
        onError={() => {
          setPlaying(false);
          setNotice(uiText("这个节目暂时无法播放，请换一期试试"));
        }}
      />
    </div>
  );
}
void mount(kit, App);
