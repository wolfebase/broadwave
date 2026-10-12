import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { live, stops, frames, pending } = vi.hoisted(() => {
  const live = new Set<() => void>();
  const stops: Array<() => void> = [];
  const pending: Array<(body: { channels: number[] } | null) => void> = [];
  const frames = vi.fn(
    () =>
      new Promise<{ channels: number[] }>((resolve, reject) => {
        pending.push((body) => (body ? resolve({ channels: body }) : reject(new Error("down"))));
      }),
  );
  return { live, stops, frames, pending };
});

vi.mock("./src/lib/events.ts", () => ({
  events: () => ({
    on: (_type: string, fn: () => void) => {
      live.add(fn);
      const stop = () => live.delete(fn);
      stops.push(stop);
      return stop;
    },
  }),
}));

vi.mock("./src/api.ts", () => ({
  getFrames: () => frames(),
}));

const { useHasFrame } = await import("./src/ui/frames.ts");

let root: Root | null = null;
let host: HTMLElement | null = null;
let visibility = "visible";

function mount(node: ReactNode) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => {
    root?.render(node);
  });
}

async function answer(channels: number[] | null) {
  const waiting = pending.splice(0);
  if (waiting.length === 0) throw new Error(`no frame read waiting, calls ${frames.mock.calls.length}`);
  await act(async () => {
    for (const resolve of waiting) resolve(channels);
    await Promise.resolve();
    await Promise.resolve();
  });
}

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  host?.remove();
  host = null;
  pending.length = 0;
  live.clear();
  stops.length = 0;
  visibility = "visible";
  vi.useRealTimers();
});

test("a card learns which channels have a picture, and a repeat does not redraw it", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "setTimeout", "clearTimeout"] });
  let renders = 0;
  function Show({ id }: { id: number }) {
    renders += 1;
    return <p>{useHasFrame(id) ? "yes" : "no"}</p>;
  }
  mount(
    <>
      <Show id={4} />
      <Show id={9} />
    </>,
  );
  expect(document.body.textContent).toBe("nono");
  await answer([4, 7]);
  expect(document.body.textContent).toBe("yesno");
  const afterFirst = renders;
  for (const fn of live) fn();
  await answer([4, 7]);
  expect(renders).toBe(afterFirst);
  for (const fn of live) fn();
  await answer([4, 9]);
  expect(document.body.textContent).toBe("yesyes");
  const shown = renders;
  for (const fn of live) fn();
  await answer(null);
  expect(document.body.textContent).toBe("yesyes");
  expect(renders).toBe(shown);

  const before = frames.mock.calls.length;
  for (const fn of live) fn();
  await answer([9]);
  expect(frames.mock.calls.length).toBeGreaterThan(before);
  expect(document.body.textContent).toBe("noyes");

  const polls = frames.mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(59_000);
  });
  expect(frames.mock.calls.length).toBe(polls);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1_000);
  });
  expect(frames.mock.calls.length).toBe(polls + 1);
});

test("a hidden page does not poll, and the last card stops the list", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "setTimeout", "clearTimeout"] });
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => visibility });
  function Show() {
    return <p>{useHasFrame(4) ? "yes" : "no"}</p>;
  }
  mount(<Show />);
  await answer([4]);
  expect(document.body.textContent).toBe("yes");
  const armed = frames.mock.calls.length;
  visibility = "hidden";
  document.dispatchEvent(new Event("visibilitychange"));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(120_000);
  });
  expect(frames.mock.calls.length).toBe(armed);

  visibility = "visible";
  document.dispatchEvent(new Event("visibilitychange"));
  await answer([4]);
  expect(frames.mock.calls.length).toBe(armed + 1);

  const stop = stops.at(-1);
  act(() => root?.unmount());
  root = null;
  expect(live.size).toBe(0);
  expect(stop).toBeTypeOf("function");
  const after = frames.mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(120_000);
  });
  expect(frames.mock.calls.length).toBe(after);
});
