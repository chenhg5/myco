// Player view and per-film resume/skip settings. The transport itself lives in
// player.ts; progress is saved locally on pause, episode changes and page exit.
import { useEffect, useRef, useState } from "react";
import { Maximize, SkipBack, SkipForward } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { clock, positive } from "./catalog";
import { playStream } from "./player";
export function Player({
  url,
  resume,
  intro,
  outro,
  previous,
  next,
  onProgress,
  onSkip,
}: {
  url: string;
  resume: number;
  intro: number;
  outro: number;
  previous?: () => void;
  next?: () => void;
  onProgress: (seconds: number, duration: number) => void;
  onSkip: (intro: number, outro: number) => void;
}) {
  const ref = useRef<HTMLVideoElement>(null),
    wrap = useRef<HTMLDivElement>(null),
    callbacks = useRef({ onProgress, next, intro, outro });
  callbacks.current = { onProgress, next, intro, outro };
  const speedRef = useRef(1);
  const [error, setError] = useState(""),
    [ready, setReady] = useState(false),
    [retry, setRetry] = useState(0),
    [speed, setSpeed] = useState(1),
    [settings, setSettings] = useState(false);
  useEffect(() => {
    const video = ref.current!;
    let started = false,
      jumped = false,
      last = 0,
      advanced = false;
    setError("");
    setReady(false);
    const persist = () => {
      if (started && video.duration > 0 && Number.isFinite(video.duration))
        callbacks.current.onProgress(video.currentTime, video.duration);
    };
    const loaded = () => {
      setReady(true);
      video.playbackRate = speedRef.current;
      const start = Math.max(resume, callbacks.current.intro);
      if (
        !jumped &&
        Number.isFinite(video.duration) &&
        start < video.duration - 3
      ) {
        jumped = true;
        video.currentTime = start;
      }
    };
    const play = () => {
      started = true;
    };
    const advance = () => {
      if (advanced) return;
      advanced = true;
      persist();
      callbacks.current.next?.();
    };
    const time = () => {
      if (Date.now() - last > 5000) {
        last = Date.now();
        persist();
      }
      const end = callbacks.current.outro;
      if (
        started &&
        !video.paused &&
        end > 0 &&
        video.duration > callbacks.current.intro + end + 5 &&
        video.currentTime >= video.duration - end &&
        callbacks.current.next
      )
        advance();
    };
    const failed = () => setError(uiText("这个视频无法解码，请切换播放线路"));
    video.addEventListener("loadedmetadata", loaded);
    video.addEventListener("canplay", loaded);
    video.addEventListener("play", play);
    video.addEventListener("timeupdate", time);
    video.addEventListener("pause", persist);
    video.addEventListener("ended", advance);
    video.addEventListener("error", failed);
    window.addEventListener("pagehide", persist);
    const stop = playStream(video, url, setError);
    return () => {
      persist();
      video.removeEventListener("loadedmetadata", loaded);
      video.removeEventListener("canplay", loaded);
      video.removeEventListener("play", play);
      video.removeEventListener("timeupdate", time);
      video.removeEventListener("pause", persist);
      video.removeEventListener("ended", advance);
      video.removeEventListener("error", failed);
      window.removeEventListener("pagehide", persist);
      stop();
    };
    // Resume is captured once per stream; local history writes must not reload it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, retry]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (
        e.defaultPrevented ||
        e.ctrlKey ||
        e.metaKey ||
        e.altKey ||
        (e.target as Element)?.closest(
          "input,textarea,select,button,[contenteditable]",
        )
      )
        return;
      const v = ref.current;
      if (!v) return;
      if (e.code === "Space") {
        e.preventDefault();
        if (v.paused) void v.play().catch(() => {});
        else v.pause();
      }
      if (
        ["ArrowLeft", "ArrowRight"].includes(e.key) &&
        Number.isFinite(v.duration)
      ) {
        e.preventDefault();
        v.currentTime = Math.max(
          0,
          Math.min(
            v.duration,
            v.currentTime + (e.key === "ArrowLeft" ? -5 : 5),
          ),
        );
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  return (
    <div className="ci-playback">
      <div className="ci-screen" ref={wrap}>
        <video
          ref={ref}
          controls
          playsInline
          preload="auto"
          aria-label={uiText("影视播放器")}
        />
        {(!ready || error) && (
          <div className="ci-player-message" role={error ? "alert" : "status"}>
            <p>{error || uiText("正在载入视频…")}</p>
            {error && (
              <button className="btn" onClick={() => setRetry((r) => r + 1)}>
                {uiText("重试播放")}
              </button>
            )}
          </div>
        )}
      </div>
      <div className="ci-player-tools">
        <button className="tool" disabled={!previous} onClick={previous}>
          <SkipBack className="ico" />
          {uiText("上一集")}
        </button>
        <button className="tool" disabled={!next} onClick={next}>
          <SkipForward className="ico" />
          {uiText("下一集")}
        </button>
        <span className="top-gap" />
        <select
          className="sel"
          value={speed}
          aria-label={uiText("播放速度")}
          onChange={(e) => {
            const n = Number(e.target.value);
            setSpeed(n);
            speedRef.current = n;
            if (ref.current) ref.current.playbackRate = n;
          }}
        >
          {[0.5, 0.75, 1, 1.25, 1.5, 2].map((n) => (
            <option key={n} value={n}>
              {n}×
            </option>
          ))}
        </select>
        <button
          className="tool"
          aria-expanded={settings}
          onClick={() => setSettings((v) => !v)}
        >
          {uiText("跳过片头片尾")}
        </button>
        <button
          className="icon-btn"
          title={uiText("全屏")}
          aria-label={uiText("全屏")}
          onClick={() => {
            const v = ref.current as
              | (HTMLVideoElement & { webkitEnterFullscreen?: () => void })
              | null;
            if (wrap.current?.requestFullscreen)
              void wrap.current
                .requestFullscreen()
                .catch(() => v?.webkitEnterFullscreen?.());
            else v?.webkitEnterFullscreen?.();
          }}
        >
          <Maximize className="ico" />
        </button>
      </div>
      {settings && (
        <div className="ci-skip">
          <label>
            {uiText("片头（秒）")}
            <input
              className="inp"
              type="number"
              min="0"
              max="600"
              value={intro}
              onChange={(e) =>
                onSkip(Math.min(600, positive(e.target.value)), outro)
              }
            />
          </label>
          <label>
            {uiText("片尾（秒）")}
            <input
              className="inp"
              type="number"
              min="0"
              max="600"
              value={outro}
              onChange={(e) =>
                onSkip(intro, Math.min(600, positive(e.target.value)))
              }
            />
          </label>
          <span className="faint">
            {uiText("填 0 关闭；片尾自动进入下一集。仅保存在这台电脑。")}
          </span>
        </div>
      )}
      {resume > 5 && (
        <p className="ci-hint">{uiText("从 {0} 继续", clock(resume))}</p>
      )}
    </div>
  );
}
