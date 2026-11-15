import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";
import { PageBoundary } from "./src/app/PageBoundary.tsx";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | null = null;
let el: HTMLDivElement | null = null;

function render(node: ReactNode) {
  if (!el) {
    el = document.createElement("div");
    document.body.appendChild(el);
    root = createRoot(el);
  }
  act(() => root!.render(node));
  return el;
}

afterEach(() => {
  act(() => root?.unmount());
  el?.remove();
  root = null;
  el = null;
  vi.restoreAllMocks();
});

function Throws({ error }: { error: Error }): ReactNode {
  throw error;
}

const away = new TypeError("Failed to fetch dynamically imported module: http://tv.local/assets/Sports-x.js");

function at(path: string) {
  window.history.replaceState(null, "", path);
}

test("a page that fails keeps the app on screen and offers a reload", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  at("/sports");
  const out = render(
    <PageBoundary path="/sports">
      <Throws error={away} />
    </PageBoundary>,
  );
  expect(out.querySelector("[role=alert] p")?.textContent).toBe("This page didn't load.");
  expect(out.querySelector("button")?.textContent).toBe("Reload");
});

test("going to another page renders it", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  at("/sports");
  render(
    <PageBoundary path="/sports">
      <Throws error={away} />
    </PageBoundary>,
  );
  at("/guide");
  const out = render(
    <PageBoundary path="/guide">
      <p>Guide</p>
    </PageBoundary>,
  );
  expect(out.textContent).toBe("Guide");
});

test("beside the page, a failure is left out quietly", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  at("/");
  const out = render(
    <PageBoundary path="/" quiet>
      <Throws error={away} />
    </PageBoundary>,
  );
  expect(out.innerHTML).toBe("");
});

test("the server coming back reloads a page that failed", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const reload = vi.fn();
  vi.spyOn(window, "location", "get").mockReturnValue({ ...window.location, pathname: "/sports", reload } as Location);
  render(
    <PageBoundary path="/sports" online={false}>
      <Throws error={away} />
    </PageBoundary>,
  );
  render(
    <PageBoundary path="/sports" online>
      <Throws error={away} />
    </PageBoundary>,
  );
  expect(reload).toHaveBeenCalledTimes(1);
});
