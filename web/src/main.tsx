import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

const root = ReactDOM.createRoot(document.getElementById("root")!);
// The Mac app's notch island is its own little window with its own page;
// loaded apart, so its styles (a transparent page) stay out of the app's.
if (new URLSearchParams(window.location.search).has("island")) {
  void import("./island/Island").then(({ Island }) => root.render(<Island />));
} else {
  root.render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}
