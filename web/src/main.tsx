import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import "./theme/tokens.css";
import "./theme/base.css";
import "./theme/brand.css";
import "./theme/light.css";
import { App } from "./app/App";
import { chooseGlass } from "./lib/glass";
import { installLook } from "./theme/look";
import { installBlocked } from "./lib/install";

installLook();
chooseGlass();

// Registered before the first paint. The browser fires this once, and a listener
// added after React commits can miss it.
window.addEventListener("beforeinstallprompt", (event) => {
  if (installBlocked()) return;
  event.preventDefault();
  void import("./app/installOffer").then((mod) => mod.present(event));
});

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
