// Page animation, keyboard navigation and wheel paging share one direction and
// mode. Native continuous scrolling is deliberately not intercepted by wheel.
import {
  useEffect,
  useRef,
  type RefObject,
  type MutableRefObject,
} from "react";
import type { Settings } from "./model";
export function useReadingNavigation({
  settings,
  page,
  direction,
  stage,
  shell,
  zoom,
  isPDF,
  loading,
  error,
  turn,
  setFocus,
}: {
  settings: Settings;
  page: number;
  direction: MutableRefObject<number>;
  stage: RefObject<HTMLDivElement>;
  shell: RefObject<HTMLDivElement>;
  zoom: number;
  isPDF: boolean;
  loading: boolean;
  error: string;
  turn: (delta: number) => void;
  setFocus: (focus: boolean) => void;
}) {
  const wheel = useRef({ sum: 0, lastEvent: 0, lastFlip: 0 });
  useEffect(() => {
    if (
      !shell.current ||
      settings.mode === "scroll" ||
      settings.effect === "none" ||
      matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const d = direction.current,
      vertical = settings.mode === "vertical",
      axis = vertical ? "Y" : "X",
      rotation = vertical ? "X" : "Y",
      effects = {
        slide: [
          { opacity: 0.45, transform: `translate${axis}(${d * 22}px)` },
          { opacity: 1, transform: `translate${axis}(0)` },
        ],
        fade: [{ opacity: 0.2 }, { opacity: 1 }],
        leaf: [
          {
            opacity: 0.55,
            transform: `perspective(1400px) rotate${rotation}(${d * -7}deg) translate${axis}(${d * 12}px)`,
            filter: "brightness(.91)",
          },
          {
            opacity: 1,
            transform: `perspective(1400px) rotate${rotation}(0deg) translate${axis}(0)`,
            filter: "brightness(1)",
          },
        ],
      };
    const animation = shell.current.animate(effects[settings.effect], {
      duration: settings.effect === "leaf" ? 330 : 210,
      easing: "cubic-bezier(.2,.7,.3,1)",
    });
    return () => animation.cancel();
  }, [page, settings.effect, settings.mode]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (
        document.querySelector("dialog[open]") ||
        /INPUT|TEXTAREA|SELECT/.test((e.target as HTMLElement).tagName) ||
        e.ctrlKey ||
        e.metaKey ||
        e.altKey
      )
        return;
      if (
        settings.mode === "scroll" &&
        (e.key === "ArrowDown" || e.key === "ArrowUp")
      ) {
        e.preventDefault();
        turn(
          ((e.key === "ArrowDown" ? 1 : -1) * 48) /
            Math.max(1, (stage.current?.clientHeight || 600) * 0.9),
        );
        return;
      }
      if (
        e.key === "ArrowRight" ||
        e.key === "ArrowDown" ||
        e.key === "PageDown" ||
        (e.key === " " && (e.target as HTMLElement).tagName !== "BUTTON")
      ) {
        e.preventDefault();
        turn(1);
      }
      if (e.key === "ArrowLeft" || e.key === "ArrowUp" || e.key === "PageUp") {
        e.preventDefault();
        turn(-1);
      }
      if (e.key === "Escape") setFocus(false);
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });
  useEffect(() => {
    const el = stage.current;
    if (!el || settings.mode !== "vertical") return;
    const handler = (event: WheelEvent) => {
      if (
        zoom > 1 ||
        event.ctrlKey ||
        Math.abs(event.deltaX) > Math.abs(event.deltaY)
      )
        return;
      event.preventDefault();
      const now = performance.now(),
        state = wheel.current;
      if (now - state.lastEvent > 180) state.sum = 0;
      state.lastEvent = now;
      if (state.lastFlip && now - state.lastFlip < 350) return;
      state.sum +=
        event.deltaY *
        (event.deltaMode === 1
          ? 16
          : event.deltaMode === 2
            ? el.clientHeight
            : 1);
      if (Math.abs(state.sum) >= 60) {
        state.lastFlip = now;
        turn(state.sum > 0 ? 1 : -1);
        state.sum = 0;
      }
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, [settings.mode, page, isPDF, zoom, loading, error]);
}
