import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { Channel } from "./src/types.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const shell = vi.hoisted(() => ({
  channels: [] as Channel[],
  allChannels: [] as Channel[],
  ready: true,
  settings: { needsSetup: "0" },
  path: "/",
  open: vi.fn(),
}));

const bus = vi.hoisted(() => {
  const handlers = new Map<string, Set<(data: unknown) => void>>();
  return {
    handlers,
    events() {
      return {
        on(type: string, fn: (data: unknown) => void) {
          let set = handlers.get(type);
          if (!set) {
            set = new Set();
            handlers.set(type, set);
          }
          set.add(fn);
          return () => {
            set.delete(fn);
          };
        },
      };
    },
    emit(type: string, data: unknown) {
      handlers.get(type)?.forEach((fn) => fn(data));
    },
  };
});

vi.mock("./src/lib/events.ts", () => ({ events: bus.events }));
vi.mock("./src/app/data.tsx", () => ({
  useData: () => ({
    channels: shell.channels,
    allChannels: shell.allChannels,
    ready: shell.ready,
    settings: shell.settings,
  }),
}));
vi.mock("./src/app/player.tsx", () => ({
  usePlayer: () => ({ open: shell.open }),
}));
vi.mock("./src/app/router.ts", () => ({
  useRoute: () => ({ path: shell.path, params: new URLSearchParams() }),
}));

const { ScreenMoves } = await import("./src/features/player/ScreenMoves.tsx");
const { movedTo, onSentHere } = await import("./src/features/player/moved.ts");

function channel(partial: Partial<Channel> & Pick<Channel, "id" | "guideName">): Channel {
  return {
    deviceId: "lab-tuner",
    guideNumber: `${partial.id}.1`,
    displayNumber: `${partial.id}.1`,
    displayName: partial.guideName,
    hd: true,
    favorite: false,
    enabled: true,
    hidden: false,
    present: true,
    ...partial,
  };
}

const den = channel({ id: 4, guideName: "Den TV" });
const encrypted = channel({
  id: 9,
  guideName: "City News 3.0",
  playsAs: 4,
  protected: true,
});
const night = channel({ id: 12, guideName: "Night Desk", hidden: true });

let root: Root | null = null;

function mount() {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  function Page() {
    const [, setTick] = useState(0);
    return (
      <div>
        <button type="button" onClick={() => setTick((n) => n + 1)}>
          refresh
        </button>
        <ScreenMoves />
      </div>
    );
  }
  act(() => {
    next.render(<Page />);
  });
  return el;
}

function refresh(el: HTMLElement) {
  act(() => {
    el.querySelector("button")!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function watch(data: unknown) {
  act(() => {
    bus.emit("screen.watch", data);
  });
}

function hear() {
  const notes: { id: number; text: string }[] = [];
  const stop = onSentHere((note) => notes.push({ id: note.id, text: note.text }));
  return { notes, stop };
}

beforeEach(() => {
  shell.channels = [den];
  shell.allChannels = [den];
  shell.ready = true;
  shell.settings = { needsSetup: "0" };
  shell.path = "/";
  shell.open.mockReset();
  bus.handlers.clear();
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
});

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
    root = null;
  }
  document.body.replaceChildren();
  // Drops a note nobody heard, so the next test starts clean.
  onSentHere(() => undefined)();
  vi.useRealTimers();
});

test("a send opens the channel and names who sent it", () => {
  mount();
  const heard = hear();
  watch({ channelId: 4, from: "  City screen  " });
  expect(shell.open).toHaveBeenCalledTimes(1);
  expect(shell.open).toHaveBeenCalledWith(den);
  expect(heard.notes).toEqual([{ id: 4, text: "From City screen" }]);
  heard.stop();
});

test("an encrypted channel's note follows the clear picture", () => {
  shell.channels = [encrypted];
  shell.allChannels = [den, encrypted];
  mount();
  const heard = hear();
  watch({ channelId: 9, from: "Den screen" });
  expect(shell.open).toHaveBeenCalledWith(encrypted);
  expect(heard.notes).toEqual([{ id: 4, text: "From Den screen" }]);
  heard.stop();
});

test("a clear twin that is not in the lineup keeps the channel that was sent", () => {
  shell.channels = [encrypted];
  shell.allChannels = [encrypted];
  mount();
  const heard = hear();
  watch({ channelId: 9, from: "Den screen" });
  expect(heard.notes).toEqual([{ id: 9, text: "From Den screen" }]);
  heard.stop();
});

test("a blank name still opens and does not leave a note", () => {
  mount();
  const heard = hear();
  watch({ channelId: 4, from: "   " });
  expect(shell.open).toHaveBeenCalledWith(den);
  expect(heard.notes).toEqual([]);
  watch({ channelId: 4 });
  expect(shell.open).toHaveBeenCalledTimes(2);
  expect(heard.notes).toEqual([]);
  heard.stop();
});

test("a channel only on the full lineup still plays", () => {
  shell.channels = [den];
  shell.allChannels = [den, night];
  mount();
  watch({ channelId: 12, from: "City screen" });
  expect(shell.open).toHaveBeenCalledWith(night);
});

test("an unknown channel does nothing", () => {
  mount();
  const heard = hear();
  watch({ channelId: 30, from: "City screen" });
  watch(null);
  expect(shell.open).not.toHaveBeenCalled();
  expect(heard.notes).toEqual([]);
  heard.stop();
});

test("setup ignores a send, and leaving setup plays the next one", () => {
  shell.path = "/setup";
  const el = mount();
  watch({ channelId: 4, from: "City screen" });
  expect(shell.open).not.toHaveBeenCalled();

  shell.path = "/guide";
  refresh(el);
  watch({ channelId: 4, from: "City screen" });
  expect(shell.open).toHaveBeenCalledWith(den);
});

test("a server that still needs setup ignores a send", () => {
  shell.settings = { needsSetup: "1" };
  mount();
  watch({ channelId: 4, from: "City screen" });
  expect(shell.open).not.toHaveBeenCalled();
});

test("a watch during boot still plays", () => {
  shell.ready = false;
  shell.settings = { needsSetup: "1" };
  mount();
  watch({ channelId: 4, from: "City screen" });
  expect(shell.open).toHaveBeenCalledWith(den);
});

test("leaving the page stops taking sends", () => {
  const el = mount();
  act(() => {
    root?.unmount();
  });
  root = null;
  el.remove();
  watch({ channelId: 4, from: "City screen" });
  expect(shell.open).not.toHaveBeenCalled();
  expect(bus.handlers.get("screen.watch")?.size ?? 0).toBe(0);
});

test("sending the picture away says where it went, then the line leaves", async () => {
  const el = mount();
  act(() => {
    movedTo("Den screen");
  });
  expect(el.querySelector("p")?.textContent).toBe("Playing on Den screen");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(4_999);
  });
  expect(el.querySelector("p")?.textContent).toBe("Playing on Den screen");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(el.querySelector('[role="status"]')).toBeNull();
});

test("a second send keeps the line up for another five seconds", async () => {
  const el = mount();
  act(() => {
    movedTo("Den screen");
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4_000);
  });
  act(() => {
    movedTo("City screen");
  });
  expect(el.querySelector("p")?.textContent).toBe("Playing on City screen");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(4_000);
  });
  expect(el.querySelector("p")?.textContent).toBe("Playing on City screen");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1_000);
  });
  expect(el.querySelector('[role="status"]')).toBeNull();
});
