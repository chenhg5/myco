import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

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
