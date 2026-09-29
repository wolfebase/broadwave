import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import "./theme/tokens.css";
import "./theme/base.css";
import "./theme/brand.css";
import { App } from "./app/App";
import { chooseGlass } from "./lib/glass";

chooseGlass();

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root");
}
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// The production build only. Vite's dev server is not the shell this caches.
if (import.meta.env.PROD && "serviceWorker" in navigator) {
  void navigator.serviceWorker.register("/sw.js");
}
