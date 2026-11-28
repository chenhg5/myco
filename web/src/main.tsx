import React from "react";
import ReactDOM from "react-dom/client";
import { initializePreferences } from "@/lib/localPreferences";
import "./index.css";
import { applyUiStyle } from "@/lib/uiStyle";


// Scrollbars: a Mac shows its own thin ones only while scrolling; elsewhere
// (Windows) the system's are always there, wide and with arrows, so they
// are drawn thin and only on hover or while scrolling (app.css, os-other).
if (!/Mac|iPhone|iPad/.test(navigator.platform || "")) {
  document.documentElement.classList.add("os-other");
  const timers = new WeakMap<Element, number>();
  document.addEventListener("scroll", (e) => {
    const el = e.target instanceof Element ? e.target : document.scrollingElement;
    if (!el) return;
    el.classList.add("is-scrolling");
    window.clearTimeout(timers.get(el));
    timers.set(el, window.setTimeout(() => el.classList.remove("is-scrolling"), 900));
  }, { capture: true, passive: true });
}
// In the desktop app (Wails), JS the app sends a page (opening a chat from
// the quick panel or the notch, the menu's commands) waits until the page
// says it is ready, which only Wails' own npm runtime would say: we do not
// load it, so say it when Wails' core arrives in the page.
const wailsReady = () => (window as unknown as { _wails?: { invoke?: (m: string) => void } })._wails?.invoke?.("wails:runtime:ready");
window.addEventListener("wails:runtime-config-ready", wailsReady);
wailsReady();
async function start() {
  await initializePreferences();
  await (await import("@/lib/i18n")).initializeLanguage();
  applyUiStyle();
  const root = ReactDOM.createRoot(document.getElementById("root")!);
  // The Mac app's notch island is its own little window with its own page;
  // loaded apart, so its styles (a transparent page) stay out of the app's.
  if (new URLSearchParams(window.location.search).has("island")) {
    void import("./island/Island").then(({ Island }) => root.render(<Island />));
  } else if (new URLSearchParams(window.location.search).has("pagewin")) {
    // A page (an app) in a window of its own (the desktop App).
    const [hubId, id] = (new URLSearchParams(window.location.search).get("pagewin") || "").split("/");
    void import("./app/PageWindow").then(({ PageWindow }) => root.render(<PageWindow hubId={hubId || ""} id={id || ""} />));
  } else if (new URLSearchParams(window.location.search).has("quick")) {
    // The ⌘⇧Space panel (Mac app): its own small window too.
    void import("./quick/Quick").then(({ Quick }) => root.render(<Quick />));
  } else {
    const { default: App } = await import("./App");
    root.render(
      <React.StrictMode>
        <App />
      </React.StrictMode>,
    );
  }
}
void start();
