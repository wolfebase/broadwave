import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useDiagnostics, type Diagnostics } from "./src/features/setup/useDiagnostics.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Held = {
  url: string;
  init?: RequestInit;
  resolve: (response: Response) => void;
  reject: (reason: unknown) => void;
};

const held: Held[] = [];
let seen: Diagnostics | null = null;
let root: Root | null = null;

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function requestURL(input: RequestInfo | URL): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  return input.url;
}

function authorization(init: RequestInit | undefined): string | undefined {
  const value = new Headers(init?.headers).get("Authorization");
  return value ?? undefined;
}

function mount(node: ReactNode) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  act(() => {
    next.render(node);
  });
  return {
    render(nextNode: ReactNode) {
      act(() => {
        next.render(nextNode);
      });
    },
    unmount() {
      act(() => {
        next.unmount();
      });
      if (root === next) root = null;
      el.remove();
    },
  };
}

function Show({ probeKey, every }: { probeKey?: unknown; every?: number }) {
  const diag = useDiagnostics(probeKey, every);
  seen = diag;
  return <p>{diag?.version ?? "none"}</p>;
}

async function settle(apply?: () => void) {
  await act(async () => {
    apply?.();
    for (let step = 0; step < 8; step += 1) await Promise.resolve();
    await vi.advanceTimersByTimeAsync(0);
  });
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  held.length = 0;
  seen = null;
  localStorage.clear();
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "setTimeout", "clearTimeout"] });
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestURL(input);
    if (url !== "/api/v1/diagnostics") throw new Error(`unexpected ${url}`);
    return new Promise<Response>((resolve, reject) => {
      held.push({ url, init, resolve, reject });
    });
  });
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  held.length = 0;
  seen = null;
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

test("the first value is null until the diagnostics body arrives", async () => {
  localStorage.setItem("bw.deviceToken", "den-token");
  const view = mount(<Show />);
  expect(seen).toBeNull();
  expect(held).toHaveLength(1);
  expect(held[0].url).toBe("/api/v1/diagnostics");
  expect(authorization(held[0].init)).toBe("Bearer den-token");

  await settle(() => {
    held[0].resolve(json({ version: "1" }));
  });
  expect(seen).toEqual({ version: "1" });
  view.unmount();
});

test("a missing device token sends no Authorization header", async () => {
  const view = mount(<Show />);
  expect(held).toHaveLength(1);
  expect(held[0].url).toBe("/api/v1/diagnostics");
  expect(authorization(held[0].init)).toBeUndefined();
  view.unmount();
});

test("the default poll waits 5000 ms, then the newer body replaces the first", async () => {
  const view = mount(<Show />);
  await settle(() => {
    held[0].resolve(json({ version: "1" }));
  });
  expect(seen).toEqual({ version: "1" });

  await advance(4999);
  expect(held).toHaveLength(1);
  await advance(1);
  expect(held).toHaveLength(2);
  expect(held[1].url).toBe("/api/v1/diagnostics");
  expect(authorization(held[1].init)).toBeUndefined();

  await settle(() => {
    held[1].resolve(json({ version: "2" }));
  });
  expect(seen).toEqual({ version: "2" });
  view.unmount();
});

test("an older response that finishes second does not replace the newer body", async () => {
  const view = mount(<Show />);
  expect(seen).toBeNull();
  expect(held[0].url).toBe("/api/v1/diagnostics");
  await advance(5000);
  expect(held).toHaveLength(2);
  expect(held[1].url).toBe("/api/v1/diagnostics");

  await settle(() => {
    held[1].resolve(json({ version: "2" }));
  });
  expect(seen).toEqual({ version: "2" });
  await settle(() => {
    held[0].resolve(json({ version: "old" }));
  });
  expect(seen).toEqual({ version: "2" });
  view.unmount();
});

test("a rejected fetch keeps the body already on screen", async () => {
  const view = mount(<Show />);
  await settle(() => {
    held[0].resolve(json({ version: "1" }));
  });
  expect(seen).toEqual({ version: "1" });
  await advance(5000);
  expect(held).toHaveLength(2);

  await settle(() => {
    held[1].reject(new Error("offline"));
  });
  expect(seen).toEqual({ version: "1" });
  view.unmount();
});

test("unmount clears the interval", async () => {
  const view = mount(<Show />);
  await settle(() => {
    held[0].resolve(json({ version: "1" }));
  });
  expect(held).toHaveLength(1);
  view.unmount();
  await advance(20_000);
  expect(held).toHaveLength(1);
  expect(held[0].url).toBe("/api/v1/diagnostics");
});

test("a new key ignores the response still in flight from the old effect", async () => {
  const view = mount(<Show probeKey="a" />);
  view.render(<Show probeKey="b" />);
  expect(held.map((call) => call.url)).toEqual(["/api/v1/diagnostics", "/api/v1/diagnostics"]);

  await settle(() => {
    held[0].resolve(json({ version: "old" }));
  });
  expect(seen).toBeNull();
  await settle(() => {
    held[1].resolve(json({ version: "2" }));
  });
  expect(seen).toEqual({ version: "2" });

  await advance(5000);
  expect(held).toHaveLength(3);
  expect(held[2].url).toBe("/api/v1/diagnostics");
  view.unmount();
});

test("a new interval ignores the response still in flight from the old effect", async () => {
  const view = mount(<Show />);
  view.render(<Show every={8000} />);
  expect(held.map((call) => call.url)).toEqual(["/api/v1/diagnostics", "/api/v1/diagnostics"]);

  await settle(() => {
    held[0].resolve(json({ version: "old" }));
  });
  expect(seen).toBeNull();
  await settle(() => {
    held[1].resolve(json({ version: "2" }));
  });
  expect(seen).toEqual({ version: "2" });

  await advance(5000);
  expect(held).toHaveLength(2);
  await advance(3000);
  expect(held).toHaveLength(3);
  expect(held[2].url).toBe("/api/v1/diagnostics");
  view.unmount();
});
