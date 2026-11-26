import { useEffect, useState } from "react";

/*
 * 界面风格: standard, or clean (清爽) without the decoration (faces in the
 * lists and titles, chat backgrounds, the walking mushroom, intro cards).
 * One for the whole App, kept on this computer like the theme; a change
 * applies at once, in every window of the App (a class on <html>).
 */
export type UiStyle = "standard" | "clean";

const KEY = "myco-ui-style";
const EVENT = "myco-ui-style";

export function storedUiStyle(): UiStyle {
  try { return localStorage.getItem(KEY) === "clean" ? "clean" : "standard"; } catch { return "standard"; }
}

/** Puts the stored style on the page (once, at start). */
export function applyUiStyle(style: UiStyle = storedUiStyle()) {
  document.documentElement.classList.toggle("ui-clean", style === "clean");
}

export function isClean(): boolean {
  return document.documentElement.classList.contains("ui-clean");
}

export function useUiStyle() {
  const [style, setStyle] = useState<UiStyle>(storedUiStyle);
  useEffect(() => {
    const on = () => setStyle(storedUiStyle());
    window.addEventListener(EVENT, on);
    window.addEventListener("storage", on);
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener("storage", on); };
  }, []);
  useEffect(() => { applyUiStyle(style); }, [style]);
  const change = (next: UiStyle) => {
    try { localStorage.setItem(KEY, next); } catch { /* not kept */ }
    applyUiStyle(next);
    setStyle(next);
    window.dispatchEvent(new Event(EVENT));
  };
  return [style, change] as const;
}
