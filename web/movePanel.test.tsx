import { act, StrictMode, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { unreadBody } from "./src/api.ts";
import type { Channel, Screen } from "./src/types.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const shell = vi.hoisted(() => ({
  channels: [] as Channel[],
  allChannels: [] as Channel[],
}));

const api = vi.hoisted(() => {
  const loads: {
    resolve: (value: { screens: Screen[] }) => void;
    reject: (reason: unknown) => void;
  }[] = [];
  return {
    loads,
    getScreens: () =>
      new Promise<{ screens: Screen[] }>((resolve, reject) => {
        loads.push({ resolve, reject });
      }),
    sendToScreen: vi.fn<(id: string, channelId: number, from: string) => Promise<void>>(),
    startedOn: vi.fn<(id: string, channelId: number, open: () => boolean) => Promise<boolean>>(),
  };
});

vi.mock("./src/app/data.tsx", () => ({
  useData: () => ({ channels: shell.channels, allChannels: shell.allChannels }),
}));

vi.mock("./src/app/remote.ts", () => ({
  focusRing(el: HTMLElement | null | undefined) {
    el?.focus();
  },
}));

vi.mock("./src/lib/events.ts", () => ({
  browserName: () => "City screen",
  screenId: () => "this-screen",
}));

vi.mock("./src/features/player/screens.ts", () => ({
  getScreens: api.getScreens,
  sendToScreen: api.sendToScreen,
  startedOn: api.startedOn,
}));

const { MovePanel } = await import("./src/features/player/MovePanel.tsx");

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
const night = channel({ id: 12, guideName: "Night Desk", hidden: true });

function screen(partial: Partial<Screen> & Pick<Screen, "id" | "name">): Screen {
  return { kind: "tv", ...partial };
}

const denScreen = screen({ id: "den-screen", name: "Den screen", channelId: 4 });
const cityScreen = screen({ id: "city-screen", name: "City screen" });
const nightScreen = screen({ id: "night-screen", name: "Night screen", channelId: 12 });
const self = screen({ id: "this-screen", name: "This screen", channelId: 4 });
const unknown = screen({ id: "other-screen", name: "Other screen", channelId: 30 });

let root: Root | null = null;

function mount(node: ReactNode = <MovePanel channel={den} onMoved={() => undefined} onClose={() => undefined} />) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  act(() => {
    next.render(node);
  });
  return el;
}

function click(el: Element) {
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function key(el: Element, name: string) {
  let event!: KeyboardEvent;
  act(() => {
    event = new KeyboardEvent("keydown", { key: name, bubbles: true, cancelable: true });
    el.dispatchEvent(event);
  });
  return event;
}

beforeEach(() => {
  shell.channels = [den];
  shell.allChannels = [den, night];
  api.loads.length = 0;
  api.sendToScreen.mockReset();
  api.startedOn.mockReset();
  api.sendToScreen.mockResolvedValue(undefined);
  api.startedOn.mockResolvedValue(false);
});

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
    root = null;
  }
  document.body.replaceChildren();
});

test("the list hides this screen and says what the others are watching", async () => {
  const el = mount();
  expect(el.textContent).toContain("Looking for screens…");
  expect(api.loads).toHaveLength(1);

  await act(async () => {
    api.loads[0].resolve({ screens: [self, denScreen, cityScreen, nightScreen, unknown] });
  });

  expect(el.textContent).not.toContain("This screen");
  expect(el.textContent).toContain("Den screen");
  expect(el.textContent).toContain("Watching 4.1");
  expect(el.textContent).toContain("City screen");
  expect(el.textContent).toContain("Night screen");
  expect(el.textContent).toContain("Watching 12.1");
  expect(el.textContent).not.toContain("Watching 30");
  expect(document.activeElement).toBe(el.querySelector("li button"));
});

test("no other screen explains how one shows up", async () => {
  const el = mount();
  await act(async () => {
    api.loads[0].resolve({ screens: [self] });
  });
  expect(el.textContent).toContain("Open Broadwave on another screen and it shows up here.");
});

test("a list that fails says why, and a blank error uses the shared sentence", async () => {
  const el = mount();
  await act(async () => {
    api.loads[0].reject(new Error("The screen list did not load."));
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe("The screen list did not load.");

  await act(async () => {
    root?.unmount();
  });
  root = null;
  const again = mount();
  await act(async () => {
    api.loads[api.loads.length - 1].reject(new Error(""));
  });
  expect(again.querySelector('[role="alert"]')?.textContent).toBe(unreadBody);
});

test("a screen that starts the channel reports the move", async () => {
  const moved: string[] = [];
  api.startedOn.mockResolvedValue(true);
  const el = mount(<MovePanel channel={den} onMoved={(name) => moved.push(name)} onClose={() => undefined} />);
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen] });
  });

  await act(async () => {
    click(el.querySelector("li button")!);
  });

  expect(api.sendToScreen).toHaveBeenCalledWith("den-screen", 4, "City screen");
  expect(api.startedOn).toHaveBeenCalledWith("den-screen", 4, expect.any(Function));
  expect(moved).toEqual(["Den screen"]);
  expect(el.textContent).toContain("Starting on Den screen…");
  expect(el.querySelector('[role="alert"]')).toBeNull();
  expect(api.loads).toHaveLength(1);
});

test("a screen that does not start says it may be asleep and looks again", async () => {
  const el = mount();
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen, cityScreen] });
  });
  await act(async () => {
    click(el.querySelector("li button")!);
  });

  expect(el.querySelector('[role="alert"]')?.textContent).toBe("Den screen didn't start it. It may be asleep.");
  expect(el.textContent).not.toContain("Starting on");
  expect((el.querySelector("li button") as HTMLButtonElement).disabled).toBe(false);
  expect(api.loads).toHaveLength(2);

  await act(async () => {
    api.loads[1].resolve({ screens: [cityScreen] });
  });
  expect([...el.querySelectorAll(".mg-title")].map((node) => node.textContent)).toEqual(["City screen"]);
});

test("a send that fails shows the error and does not ask twice while one is going out", async () => {
  let release: (() => void) | undefined;
  api.sendToScreen.mockImplementation(
    () =>
      new Promise((resolve, reject) => {
        release = () => reject(new Error("That screen is gone."));
      }),
  );
  const el = mount();
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen, cityScreen] });
  });
  const buttons = () => [...el.querySelectorAll("li button")];

  act(() => {
    buttons()[0].dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  expect(buttons()[0].disabled).toBe(true);
  expect(buttons()[1].disabled).toBe(true);
  expect(buttons()[0].getAttribute("aria-busy")).toBe("true");

  click(buttons()[1]);
  expect(api.sendToScreen).toHaveBeenCalledTimes(1);

  await act(async () => {
    release?.();
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe("That screen is gone.");
  expect(api.loads).toHaveLength(2);
});

test("a send that throws something other than an error uses the shared sentence", async () => {
  api.sendToScreen.mockRejectedValue("nope");
  const el = mount();
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen] });
  });
  await act(async () => {
    click(el.querySelector("li button")!);
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe(unreadBody);
});

test("closing the panel while it is still starting does not show an error", async () => {
  let finish: ((ok: boolean) => void) | undefined;
  let stillOpen: (() => boolean) | undefined;
  api.startedOn.mockImplementation((_id, _channel, open) => {
    stillOpen = open;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const moved: string[] = [];
  const el = mount(<MovePanel channel={den} onMoved={(name) => moved.push(name)} onClose={() => undefined} />);
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen] });
  });
  await act(async () => {
    el.querySelector("li button")!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  expect(stillOpen?.()).toBe(true);

  act(() => {
    root?.unmount();
  });
  root = null;
  expect(stillOpen?.()).toBe(false);

  await act(async () => {
    finish?.(false);
  });
  expect(moved).toEqual([]);
  expect(document.body.textContent).not.toContain("asleep");
});

test("a start that finishes after the panel closes still reports the move", async () => {
  let finish: ((ok: boolean) => void) | undefined;
  api.startedOn.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const moved: string[] = [];
  const el = mount(<MovePanel channel={den} onMoved={(name) => moved.push(name)} onClose={() => undefined} />);
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen] });
  });
  await act(async () => {
    el.querySelector("li button")!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  act(() => {
    root?.unmount();
  });
  root = null;
  await act(async () => {
    finish?.(true);
  });
  expect(moved).toEqual(["Den screen"]);
});

test("cancel, backspace, and escape close the panel", async () => {
  const closed: string[] = [];
  const el = mount(<MovePanel channel={den} onMoved={() => undefined} onClose={() => closed.push("close")} />);
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen] });
  });
  const dialog = el.querySelector('[role="dialog"]')!;
  const escape = key(dialog, "Escape");
  expect(escape.defaultPrevented).toBe(true);
  const back = key(dialog, "Backspace");
  expect(back.defaultPrevented).toBe(true);
  click(el.querySelector(".option-actions button")!);
  expect(closed).toEqual(["close", "close", "close"]);
});

test("up and down walk the screens and left and right stay put", async () => {
  const el = mount();
  await act(async () => {
    api.loads[0].resolve({ screens: [denScreen, cityScreen] });
  });
  const dialog = el.querySelector('[role="dialog"]')!;
  const buttons = () => [...el.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
  expect(document.activeElement).toBe(buttons()[0]);

  const down = key(dialog, "ArrowDown");
  expect(down.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons()[1]);

  key(dialog, "ArrowDown");
  expect(document.activeElement).toBe(buttons()[2]);
  key(dialog, "ArrowDown");
  expect(document.activeElement).toBe(buttons()[2]);

  key(dialog, "ArrowUp");
  expect(document.activeElement).toBe(buttons()[1]);

  const left = key(dialog, "ArrowLeft");
  expect(left.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons()[1]);
});

test("a strict-mode remount keeps the newer screen list and ignores the older error", async () => {
  const el = mount(
    <StrictMode>
      <MovePanel channel={den} onMoved={() => undefined} onClose={() => undefined} />
    </StrictMode>,
  );
  expect(api.loads.length).toBeGreaterThanOrEqual(2);
  const first = api.loads[0];
  const latest = api.loads[api.loads.length - 1];

  await act(async () => {
    latest.resolve({ screens: [cityScreen] });
  });
  expect(el.textContent).toContain("City screen");

  await act(async () => {
    first.reject(new Error("The first look failed."));
  });
  expect(el.textContent).toContain("City screen");
  expect(el.textContent).not.toContain("Den screen");
  expect(el.textContent).not.toContain("The first look failed.");
});
