import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type List = {
  matches: boolean;
  addEventListener: (type: string, fn: () => void) => void;
  removeEventListener: (type: string, fn: () => void) => void;
  dispatch: () => void;
  listeners: Set<() => void>;
};

const lists: List[] = [];
window.matchMedia = ((query: string) => {
  const listeners = new Set<() => void>();
  const list = {
    media: query,
    matches: false,
    listeners,
    onchange: null,
    addEventListener: (_type: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_type: string, fn: () => void) => listeners.delete(fn),
    dispatch() {
      for (const fn of listeners) fn();
    },
    addListener() {},
    removeListener() {},
    dispatchEvent() {
      return true;
    },
  };
  lists.push(list);
  return list as unknown as MediaQueryList;
}) as typeof window.matchMedia;

const settings = { layout: "auto" };
vi.mock("./src/app/data.tsx", () => ({
  useData: () => ({ settings }),
}));

const { useLayout } = await import("./src/app/layout.ts");

let root: Root | null = null;

function mount(node: ReactNode) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => {
    root?.render(node);
  });
  return host;
}

function Show() {
  return <p>{useLayout()}</p>;
}

function rerender() {
  act(() => {
    root?.render(<Show />);
  });
}

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  document.body.replaceChildren();
  document.documentElement.className = "";
  delete document.documentElement.dataset.layout;
  settings.layout = "auto";
  window.history.replaceState(null, "", "/");
  for (const list of lists) list.matches = false;
});

test("a saved layout wins, and auto follows the window and the address", () => {
  const phone = lists[0];
  const tv = lists[1];
  settings.layout = "tv";
  phone.matches = true;
  window.history.replaceState(null, "", "/?layout=phone");
  mount(<Show />);
  expect(document.body.textContent).toBe("tv");
  expect(document.documentElement.dataset.layout).toBe("tv");
  expect(document.documentElement.classList.contains("layout-tv")).toBe(true);
  expect(document.documentElement.classList.contains("layout-phone")).toBe(false);

  settings.layout = "auto";
  rerender();
  expect(document.body.textContent).toBe("phone");
  expect(document.documentElement.classList.contains("layout-phone")).toBe(true);
  expect(document.documentElement.classList.contains("layout-tv")).toBe(false);

  window.history.replaceState(null, "", "/?layout=desktop");
  phone.matches = false;
  tv.matches = true;
  rerender();
  expect(document.body.textContent).toBe("desktop");

  window.history.replaceState(null, "", "/?layout=nope");
  rerender();
  expect(document.body.textContent).toBe("tv");

  phone.matches = true;
  tv.matches = false;
  act(() => phone.dispatch());
  expect(document.body.textContent).toBe("phone");

  settings.layout = "desktop";
  rerender();
  act(() => phone.dispatch());
  expect(document.body.textContent).toBe("desktop");

  const listening = phone.listeners.size;
  act(() => root?.unmount());
  root = null;
  expect(phone.listeners.size).toBeLessThan(listening);
  expect(() => phone.dispatch()).not.toThrow();
});
