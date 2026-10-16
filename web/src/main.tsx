import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import "./theme/tokens.css";
import "./theme/base.css";
import "./theme/brand.css";
import "./theme/light.css";
import { App } from "./app/App";
import { shellMessages } from "./i18n/messages/shell";
import { bind, setLocaleMode } from "./i18n/runtime";
import { chooseGlass } from "./lib/glass";
import { installLook } from "./theme/look";
import { installBlocked, isAppleTouch, offerHomeScreenHint } from "./lib/install";

const shell = bind(shellMessages);
const asked = new URLSearchParams(location.search).get("locale");
if (asked === "pseudo") setLocaleMode("pseudo");
document.title = shell("shell.brand");

installLook();
chooseGlass();

// Registered before the first paint. The browser fires this once, and a listener
// added after React commits can miss it.
window.addEventListener("beforeinstallprompt", (event) => {
  if (installBlocked()) return;
  event.preventDefault();
  void import("./app/installOffer").then((mod) => mod.present(event));
});

// Safari on iPhone and iPad has no install prompt. A short hint comes once the page has settled.
if (offerHomeScreenHint({ appleTouch: isAppleTouch(navigator.userAgent, navigator.maxTouchPoints ?? 0), secure: location.protocol === "https:", standalone: false, dismissed: installBlocked() })) {
  window.setTimeout(() => void import("./app/installOffer").then((mod) => mod.presentHomeScreenHint()), 4000);
}

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
