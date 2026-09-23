import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import "./theme/tokens.css";
import "./theme/base.css";
import { App } from "./app/App";

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root");
}
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
