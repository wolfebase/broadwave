import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";
import { PageBoundary } from "./src/app/PageBoundary.tsx";
import { isChunkError } from "./src/lib/chunkError.ts";

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

test("the browsers' words for a page file that could not be fetched", () => {
  expect(isChunkError(away)).toBe(true);
  expect(isChunkError(new TypeError("Importing a module script failed."))).toBe(true);
  expect(isChunkError(new Error("Unable to preload CSS for /assets/Stage-1.css"))).toBe(true);
  expect(isChunkError(new TypeError("error loading dynamically imported module"))).toBe(true);
  expect(isChunkError(new TypeError("Cannot read properties of undefined (reading 'id')"))).toBe(false);
  expect(isChunkError(undefined)).toBe(false);
});

test("a page that fails keeps the app on screen and says why", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const out = render(
    <PageBoundary path="/sports" online={false}>
      <Throws error={away} />
    </PageBoundary>,
  );
  expect(out.querySelector("[role=alert] h1")?.textContent).toBe("This page didn't load");
  expect(out.textContent).toContain("Can't reach the server.");
  expect(out.querySelector("button")?.textContent).toBe("Reload");

  const bug = render(
    <PageBoundary path="/settings" online>
      <Throws error={new TypeError("x is undefined")} />
    </PageBoundary>,
  );
  expect(bug.textContent).toContain("Something went wrong on this page.");
});

test("going to another page clears the failure", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  render(
    <PageBoundary path="/sports" online={false}>
      <Throws error={away} />
    </PageBoundary>,
  );
  const out = render(
    <PageBoundary path="/guide" online={false}>
      <p>Guide</p>
    </PageBoundary>,
  );
  expect(out.textContent).toBe("Guide");
});

test("beside the page, a failure is left out quietly", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const out = render(
    <PageBoundary path="/" online quiet>
      <Throws error={away} />
    </PageBoundary>,
  );
  expect(out.innerHTML).toBe("");
});

test("the server coming back reloads a page whose file could not be fetched", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const reload = vi.fn();
  vi.spyOn(window, "location", "get").mockReturnValue({ ...window.location, reload } as Location);
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
