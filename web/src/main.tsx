import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

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
const root = ReactDOM.createRoot(document.getElementById("root")!);
// The Mac app's notch island is its own little window with its own page;
// loaded apart, so its styles (a transparent page) stay out of the app's.
if (new URLSearchParams(window.location.search).has("island")) {
  void import("./island/Island").then(({ Island }) => root.render(<Island />));
} else if (new URLSearchParams(window.location.search).has("quick")) {
  // The ⌘⇧Space panel (Mac app): its own small window too.
  void import("./quick/Quick").then(({ Quick }) => root.render(<Quick />));
} else {
  root.render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}
