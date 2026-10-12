import { act, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { Channel } from "./src/types.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Catalog = {
  channels: Channel[];
  allChannels: Channel[];
  ready: boolean;
  channelsReady: boolean;
  needsPair: boolean;
};

const catalog = vi.hoisted<Catalog>(() => ({
  channels: [],
  allChannels: [],
  ready: true,
  channelsReady: true,
  needsPair: false,
}));

type PlayerProps = {
  channel: Channel;
  mode: "full" | "mini";
  notice?: string;
  onChannel: (channel: Channel) => void;
  onMinimize: () => void;
  onClose: () => void;
  onExpand: () => void;
};

const shown = vi.hoisted(() => ({ props: null as PlayerProps | null }));

vi.mock("./src/app/data.tsx", () => ({
  useData: () => catalog,
}));

vi.mock("./src/features/player/LivePlayer.tsx", () => ({
  LivePlayer: (props: PlayerProps) => {
    shown.props = props;
    return <div data-testid="live" />;
  },
}));

const { PlayerProvider, usePlayer } = await import("./src/app/player.tsx");
const { inAppDepth, navigate } = await import("./src/app/router.ts");
const { encryptedBroadcast } = await import("./src/strings/notice.ts");

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

const den = channel({ id: 4, guideName: "Den TV", guideNumber: "4.1", displayNumber: "4.1" });
const city = channel({ id: 5, guideName: "City News", guideNumber: "5.1", displayNumber: "5.1" });
const encrypted = channel({ id: 9, guideName: "City News 3.0", guideNumber: "9.1", displayNumber: "9.1", playsAs: 4, protected: true });
const hiddenTwin = channel({ id: 8, guideName: "Den TV extra", guideNumber: "4.2", displayNumber: "4.2", twinId: 4, hidden: true });
const hiddenOnly = channel({ id: 12, guideName: "Night Desk", guideNumber: "12.1", displayNumber: "12.1", hidden: true });

let root: Root | null = null;
let host: HTMLDivElement | null = null;

function resetCatalog(channels: Channel[], allChannels = channels) {
  catalog.channels = channels;
  catalog.allChannels = allChannels;
  catalog.ready = true;
  catalog.channelsReady = true;
  catalog.needsPair = false;
}

beforeEach(() => {
  shown.props = null;
  resetCatalog([den, city], [den, city, encrypted, hiddenTwin, hiddenOnly]);
  window.history.replaceState(null, "", "/");
});

afterEach(async () => {
  act(() => root?.unmount());
  root = null;
  host?.remove();
  host = null;
  shown.props = null;
  vi.useRealTimers();
  window.history.replaceState(null, "", "/");
  await new Promise((resolve) => setImmediate(resolve));
  await new Promise((resolve) => setImmediate(resolve));
});

async function flush() {
  await act(async () => {
    for (let step = 0; step < 8; step += 1) await Promise.resolve();
  });
}

// jsdom runs history.back on a later turn, and that turn queues one more.
async function historyTurn() {
  await act(async () => {
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
  });
}

function mount(node: ReactNode) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => {
    root?.render(node);
  });
  return host;
}

function Shell({ tick = 0 }: { tick?: number }) {
  const player = usePlayer();
  return (
    <p>
      {player.mode} {player.channel?.guideName ?? "none"} {tick}
    </p>
  );
}

function Controls() {
  const player = usePlayer();
  return (
    <div>
      <button type="button" onClick={() => player.open(den)}>
        open den
      </button>
      <button type="button" onClick={() => player.open(city)}>
        open city
      </button>
      <button type="button" onClick={() => player.open(encrypted)}>
        open encrypted
      </button>
      <button type="button" onClick={() => player.close()}>
        close
      </button>
    </div>
  );
}

function Page() {
  const [tick, setTick] = useState(0);
  return (
    <PlayerProvider>
      <button type="button" onClick={() => setTick((n) => n + 1)}>
        refresh
      </button>
      <Controls />
      <Shell tick={tick} />
    </PlayerProvider>
  );
}

function text(): string {
  return host?.querySelector("p")?.textContent ?? "";
}

function click(label: string) {
  const button = [...(host?.querySelectorAll("button") ?? [])].find((el) => el.textContent === label);
  if (!button) throw new Error(`missing ${label}`);
  act(() => {
    button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function here(): string {
  return window.location.pathname + window.location.search;
}

test("usePlayer throws outside the player", () => {
  function Boom() {
    usePlayer();
    return null;
  }
  expect(() => mount(<Boom />)).toThrow(/usePlayer outside PlayerProvider/);
});

test("a listed channel plays full screen", async () => {
  window.history.replaceState(null, "", "/watch?channel=4");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/watch?channel=4");
  expect(text()).toContain("full Den TV");
  expect(shown.props?.mode).toBe("full");
  expect(shown.props?.channel.guideName).toBe("Den TV");
  expect(shown.props?.notice).toBeUndefined();
  expect(host?.querySelector("[data-testid=live]")).not.toBeNull();
});

test("an encrypted station plays its clear twin and the note leaves after ten seconds", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  window.history.replaceState(null, "", "/watch?channel=9");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/watch?channel=4&from=9");
  expect(text()).toContain("full Den TV");
  expect(shown.props?.notice).toBe(encryptedBroadcast());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(9_999);
  });
  expect(shown.props?.notice).toBe(encryptedBroadcast());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(shown.props?.notice).toBeUndefined();
  expect(text()).toContain("full Den TV");
});

test("a hidden half plays the row on the guide without the encrypted note", async () => {
  window.history.replaceState(null, "", "/watch?channel=8");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/watch?channel=4");
  expect(shown.props?.notice).toBeUndefined();
  expect(shown.props?.channel.id).toBe(4);
  expect(text()).toContain("full Den TV");
});

test("a channel the lineup does not have goes to the guide", async () => {
  window.history.replaceState(null, "", "/watch?channel=99");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/guide");
  expect(text()).toContain("mini none");
  expect(host?.querySelector("[data-testid=live]")).toBeNull();
});

test("a watch link waits until the lineup is ready", async () => {
  catalog.ready = false;
  catalog.channelsReady = false;
  window.history.replaceState(null, "", "/watch?channel=4");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/watch?channel=4");
  expect(text()).toContain("mini none");
  expect(host?.querySelector("[data-testid=live]")).toBeNull();
});

test("a hidden channel with no twin plays as itself", async () => {
  window.history.replaceState(null, "", "/watch?channel=12");
  mount(<Page />);
  await flush();
  expect(here()).toBe("/watch?channel=12");
  expect(text()).toContain("full Night Desk");
  expect(shown.props?.channel.id).toBe(12);
  expect(shown.props?.notice).toBeUndefined();
});

test("pairing and multiview keep the picture off the page", async () => {
  catalog.needsPair = true;
  window.history.replaceState(null, "", "/watch?channel=4");
  mount(<Page />);
  await flush();
  expect(text()).toContain("full Den TV");
  expect(host?.querySelector("[data-testid=live]")).toBeNull();

  catalog.needsPair = false;
  click("refresh");
  await flush();
  expect(host?.querySelector("[data-testid=live]")).not.toBeNull();

  navigate("/multiview");
  await flush();
  expect(here()).toBe("/multiview");
  expect(text()).toContain("mini none");
  expect(host?.querySelector("[data-testid=live]")).toBeNull();
});

test("opening a channel watches it, and an encrypted one names the stand-in", async () => {
  navigate("/guide");
  mount(<Page />);
  await flush();
  expect(inAppDepth()).toBe(1);

  click("open den");
  await flush();
  expect(here()).toBe("/watch?channel=4");
  expect(text()).toContain("full Den TV");
  const depth = inAppDepth();

  click("open city");
  await flush();
  expect(here()).toBe("/watch?channel=5");
  expect(inAppDepth()).toBe(depth);
  expect(text()).toContain("full City News");

  click("open encrypted");
  await flush();
  expect(here()).toBe("/watch?channel=4&from=9");
  expect(shown.props?.notice).toBe(encryptedBroadcast());
  expect(text()).toContain("full Den TV");
});

test("close returns to the page under the player, and setup is not that page", async () => {
  navigate("/sports?channel=7");
  mount(<Page />);
  await flush();
  navigate("/setup");
  await flush();
  click("open den");
  await flush();
  expect(here()).toBe("/watch?channel=4");
  click("close");
  await flush();
  expect(here()).toBe("/sports?channel=7");
  expect(text()).toContain("mini none");
  expect(host?.querySelector("[data-testid=live]")).toBeNull();
});

test("minimize steps back when the app pushed a page, and expand returns to the picture", async () => {
  navigate("/guide");
  mount(<Page />);
  await flush();
  click("open den");
  await flush();
  expect(inAppDepth()).toBe(2);
  act(() => {
    shown.props?.onMinimize();
  });
  await historyTurn();
  expect(here()).toBe("/guide");
  expect(text()).toContain("mini Den TV");
  expect(shown.props?.mode).toBe("mini");
  act(() => {
    shown.props?.onExpand();
  });
  await flush();
  expect(here()).toBe("/watch?channel=4");
  expect(text()).toContain("full Den TV");
});

test("minimize from a page opened directly returns to the guide", async () => {
  window.history.replaceState(null, "", "/watch?channel=4");
  mount(<Page />);
  await flush();
  expect(inAppDepth()).toBe(0);
  act(() => {
    shown.props?.onMinimize();
  });
  await historyTurn();
  expect(here()).toBe("/guide");
  expect(text()).toContain("mini Den TV");
});
