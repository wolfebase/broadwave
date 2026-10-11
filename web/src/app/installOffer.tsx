import { useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { installDismissKey } from "../lib/install";
import { canonicalKey } from "../lib/tvKey";

type InstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice?: Promise<{ outcome: "accepted" | "dismissed" }>;
};

let root: Root | null = null;
let host: HTMLElement | null = null;
let watch: MutationObserver | null = null;
let onKey: ((event: KeyboardEvent) => void) | null = null;

function rememberDismiss() {
  try {
    localStorage.setItem(installDismissKey, "1");
  } catch {
    // A private window can still close the banner for this visit.
  }
}

function Banner({ event, onDone }: { event: InstallPromptEvent; onDone: () => void }) {
  const [error, setError] = useState("");

  function dismiss() {
    rememberDismiss();
    onDone();
  }

  async function accept() {
    try {
      await event.prompt();
      const choice = event.userChoice ? await event.userChoice : { outcome: "dismissed" as const };
      if (choice.outcome === "accepted") rememberDismiss();
      onDone();
    } catch {
      setError("Couldn't open the install dialog. Try again from the browser menu.");
    }
  }

  return (
    <>
      <p>Install Broadwave on this device. It opens like an app.</p>
      {error ? <p role="alert">{error}</p> : null}
      <button type="button" className="btn small primary" onClick={() => void accept()}>
        Install
      </button>
      <button type="button" className="btn small ghost" data-install-dismiss onClick={dismiss}>
        Not now
      </button>
    </>
  );
}

function covered() {
  return document.querySelector(".shell.immersive") != null;
}

function teardown() {
  if (onKey) window.removeEventListener("keydown", onKey, true);
  onKey = null;
  watch?.disconnect();
  watch = null;
  root?.unmount();
  root = null;
  host?.remove();
  host = null;
}

/** Show the install banner. It stays off the player, and Back on it means Not now. */
export function present(event: Event) {
  const prompt = event as InstallPromptEvent;
  if (typeof prompt.prompt !== "function") return;
  teardown();
  host = document.createElement("div");
  host.className = "install-offer";
  host.setAttribute("role", "region");
  host.setAttribute("aria-label", "Install Broadwave");
  document.body.appendChild(host);
  root = createRoot(host);
  const shell = document.querySelector(".shell");
  const place = () => {
    if (host) host.hidden = covered();
  };
  if (shell) {
    watch = new MutationObserver(place);
    watch.observe(shell, { attributes: true, attributeFilter: ["class"] });
  }
  onKey = (event) => {
    if (!host || host.hidden) return;
    const target = event.target;
    if (!(target instanceof Node) || !host.contains(target)) return;
    const key = canonicalKey(event);
    if (key !== "Escape" && key !== "Backspace") return;
    event.preventDefault();
    event.stopPropagation();
    host.querySelector<HTMLButtonElement>("[data-install-dismiss]")?.click();
  };
  window.addEventListener("keydown", onKey, true);
  root.render(<Banner event={prompt} onDone={teardown} />);
  place();
}
