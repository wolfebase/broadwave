import { act, useRef, type ReactNode, type RefObject } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";
import { Stage } from "./src/features/player/Stage.tsx";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock("./src/app/remote.ts", () => ({
  focusRing(el: HTMLElement | null | undefined) {
    el?.focus();
  },
}));

let root: Root | null = null;

type Extra = {
  mode?: "full" | "mini";
  score?: string;
  liveLabel?: string;
  position?: number;
  duration?: number;
  markers?: { id: number; start: number; end: number }[];
  error?: string;
  errorAction?: ReactNode;
  note?: string;
  noteAction?: ReactNode;
  loading?: ReactNode;
  hold?: boolean;
  more?: ReactNode;
  moreAside?: boolean;
  onTogglePlay?: () => void;
};

function mount(extra: Extra = {}) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  const calls = {
    back: vi.fn(),
    live: vi.fn(),
    seek: vi.fn(),
    jump: vi.fn(),
    expand: vi.fn(),
    close: vi.fn(),
    sound: vi.fn(),
    key: vi.fn(),
  };
  function Page() {
    const videoRef = useRef<HTMLVideoElement>(null);
    return (
      <Stage
        videoRef={videoRef as RefObject<HTMLVideoElement | null>}
        videoClass="picture"
        title="Night Desk"
        eyebrow="4.1"
        score={extra.score}
        onBack={calls.back}
        backLabel="Guide"
        liveLabel={extra.liveLabel}
        onLive={calls.live}
        position={extra.position ?? 90}
        duration={extra.duration ?? 100}
        onSeek={calls.seek}
        markers={extra.markers}
        onJump={calls.jump}
        error={extra.error}
        errorAction={extra.errorAction}
        note={extra.note}
        noteAction={extra.noteAction}
        more={extra.more}
        moreAside={extra.moreAside}
        mode={extra.mode}
        onExpand={calls.expand}
        onClose={calls.close}
        onTogglePlay={extra.onTogglePlay}
        loading={extra.loading}
        onSound={calls.sound}
        hold={extra.hold}
        onKeyDown={calls.key}
      />
    );
  }
  act(() => {
    next.render(<Page />);
  });
  return { el, calls };
}

function click(el: Element) {
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function press(el: Element, key: string, init: KeyboardEventInit = {}) {
  let event!: KeyboardEvent;
  act(() => {
    event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
    el.dispatchEvent(event);
  });
  return event;
}

function setRange(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
    root = null;
  }
  document.body.replaceChildren();
  vi.useRealTimers();
});

test("the docked bar names the show and does not draw the full controls", () => {
  const { el, calls } = mount({ mode: "mini" });
  const stage = el.querySelector("section")!;
  expect(stage.className).toContain("mini");
  expect(stage.getAttribute("aria-label")).toBe("Now playing: Night Desk");
  expect(stage.tabIndex).toBe(-1);
  expect(stage.querySelector(".stage-hud")).toBeNull();
  click(stage.querySelector("video")!);
  click(stage.querySelector('[aria-label="Open player"]')!);
  expect(calls.expand).toHaveBeenCalledTimes(2);
  click(stage.querySelector('[aria-label="Stop watching"]')!);
  expect(calls.close).toHaveBeenCalledTimes(1);
});

test("a paused picture offers play, and a loading picture does not", () => {
  const toggle = vi.fn();
  const paused = mount({ onTogglePlay: toggle });
  expect(paused.el.querySelector(".stage-fab")?.getAttribute("aria-label")).toBe("Play");
  click(paused.el.querySelector(".stage-fab")!);
  expect(toggle).toHaveBeenCalledTimes(1);

  act(() => {
    root?.unmount();
  });
  root = null;
  const loading = mount({ loading: <p>Starting…</p>, onTogglePlay: toggle });
  expect(loading.el.textContent).toContain("Starting…");
  expect(loading.el.querySelector(".stage-fab")).toBeNull();
});

test("the scrubber clamps the playhead and draws a break", () => {
  const { el, calls } = mount({
    position: -4,
    duration: 100,
    markers: [
      { id: 1, start: 10, end: 40 },
      { id: 2, start: 80, end: 80 },
    ],
  });
  const scrub = el.querySelector<HTMLInputElement>('[aria-label="Playback position"]')!;
  expect(scrub.value).toBe("0");
  expect(scrub.getAttribute("aria-valuetext")).toBe("0:00 of 1:40");
  const marks = [...el.querySelectorAll(".scrub-marks span")];
  expect(marks[0].getAttribute("style")).toContain("left: 10%");
  expect(marks[0].getAttribute("style")).toContain("width: 30%");
  expect(marks[1].getAttribute("style")).toContain("width: 0.4%");
  setRange(scrub, "40");
  expect(calls.seek).toHaveBeenCalledWith(40);

  click(el.querySelector('[aria-label="Back 15 seconds"]')!);
  click(el.querySelector('[aria-label="Forward 30 seconds"]')!);
  expect(calls.jump).toHaveBeenNthCalledWith(1, -15);
  expect(calls.jump).toHaveBeenNthCalledWith(2, 30);
  click(el.querySelector('[aria-label="Guide"]')!);
  expect(calls.back).toHaveBeenCalledTimes(1);
});

test("an hour-long recording reads with hours, and a playhead past the end stays at the end", () => {
  const { el } = mount({ position: 9000, duration: 3661 });
  const scrub = el.querySelector<HTMLInputElement>('[aria-label="Playback position"]')!;
  expect(scrub.getAttribute("aria-valuetext")).toBe("1:01:01 of 1:01:01");
  expect(el.querySelector(".time-read")?.textContent).toBe("1:01:01 / 1:01:01");
});

test("a missing duration still has a scrubber, held at the start", () => {
  const { el } = mount({ position: 5, duration: 0 });
  const scrub = el.querySelector<HTMLInputElement>('[aria-label="Playback position"]')!;
  expect(Number(scrub.max)).toBeCloseTo(0.1);
  expect(Number(scrub.value)).toBeCloseTo(0.1);
  expect(scrub.getAttribute("aria-valuetext")).toBe("0:00 of 0:00");
});

test("live is a pill, and a delay says how far behind", () => {
  const live = mount({ liveLabel: "Live", score: "CNO 0 · DEN 0" });
  const pill = live.el.querySelector(".live-pill")!;
  expect(pill.className).toContain("on");
  expect(pill.getAttribute("aria-label")).toBeNull();
  expect(live.el.querySelector(".stage-score")?.textContent).toBe("CNO 0 · DEN 0");
  click(pill);
  expect(live.calls.live).toHaveBeenCalledTimes(1);

  act(() => {
    root?.unmount();
  });
  root = null;
  const behind = mount({ liveLabel: "1m 30s behind" });
  const late = behind.el.querySelector(".live-pill")!;
  expect(late.className).not.toContain("on");
  expect(late.getAttribute("aria-label")).toBe("Go to live, 1m 30s behind");
});

test("an error replaces the note, and mute tells the player", () => {
  const { el, calls } = mount({
    error: "This channel isn't coming in.",
    errorAction: <button type="button">Try again</button>,
    note: "On in a moment",
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe("This channel isn't coming in.");
  expect(el.textContent).not.toContain("On in a moment");
  const video = el.querySelector("video")!;
  expect(video.muted).toBe(false);
  click(el.querySelector('[aria-label="Mute"]')!);
  expect(video.muted).toBe(true);
  expect(el.querySelector('[aria-label="Unmute"]')).not.toBeNull();
  expect(calls.sound).toHaveBeenCalledTimes(1);
});

test("options close on escape unless a dialog or a typed channel is up", () => {
  const sheet = mount({ more: <button type="button">Smooth</button>, note: "Night Desk is on" });
  const stage = sheet.el.querySelector("section")!;
  click(sheet.el.querySelector('[data-options]')!);
  expect(sheet.el.querySelector("#player-options")?.textContent).toBe("Smooth");
  expect(sheet.el.querySelector('[data-options]')?.getAttribute("aria-expanded")).toBe("true");
  const escape = press(stage, "Escape");
  expect(escape.defaultPrevented).toBe(true);
  expect(sheet.el.querySelector("#player-options")).toBeNull();
  expect(document.activeElement).toBe(sheet.el.querySelector('[data-options]'));

  act(() => {
    root?.unmount();
  });
  root = null;
  const covered = mount({
    more: (
      <div role="dialog">
        <button type="button">Close</button>
      </div>
    ),
  });
  click(covered.el.querySelector('[data-options]')!);
  press(covered.el.querySelector("section")!, "Escape");
  expect(covered.el.querySelector("#player-options")).not.toBeNull();

  act(() => {
    root?.unmount();
  });
  root = null;
  const typed = mount({
    more: (
      <div>
        <span className="typed-number">4</span>
        <button type="button">Smooth</button>
      </div>
    ),
  });
  click(typed.el.querySelector('[data-options]')!);
  const back = press(typed.el.querySelector("section")!, "Backspace");
  expect(back.defaultPrevented).toBe(false);
  expect(typed.el.querySelector("#player-options")).not.toBeNull();
});

test("a panel that steps aside does not name the options region", () => {
  const { el } = mount({ more: <p>Smooth</p>, moreAside: true });
  click(el.querySelector('[data-options]')!);
  expect(el.querySelector("#player-options")).toBeNull();
  expect(el.querySelector('[data-options]')?.getAttribute("aria-controls")).toBeNull();
});

test("the bar fades after a few quiet seconds and a move brings it back", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const { el } = mount();
  const video = el.querySelector("video")!;
  Object.defineProperty(video, "paused", { configurable: true, get: () => false });
  await act(async () => {
    video.dispatchEvent(new Event("play"));
  });
  const stage = el.querySelector("section")!;
  expect(stage.className).not.toContain("idle");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3199);
  });
  expect(stage.className).not.toContain("idle");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(stage.className).toContain("idle");
  expect(el.querySelector(".sr-only")).toBeNull();

  await act(async () => {
    window.dispatchEvent(new MouseEvent("mousemove"));
  });
  expect(stage.className).not.toContain("idle");
});

test("a held note stays up, and the idle note is read aloud once the bar fades", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const held = mount({ hold: true, note: "Keep watching?" });
  const video = held.el.querySelector("video")!;
  Object.defineProperty(video, "paused", { configurable: true, get: () => false });
  await act(async () => {
    video.dispatchEvent(new Event("play"));
    await vi.advanceTimersByTimeAsync(4000);
  });
  expect(held.el.querySelector("section")!.className).not.toContain("idle");
  expect(held.el.querySelector(".player-note")?.textContent).toContain("Keep watching?");

  act(() => {
    root?.unmount();
  });
  root = null;
  const quiet = mount({ note: "On in a moment" });
  const playing = quiet.el.querySelector("video")!;
  Object.defineProperty(playing, "paused", { configurable: true, get: () => false });
  await act(async () => {
    playing.dispatchEvent(new Event("play"));
    await vi.advanceTimersByTimeAsync(3200);
  });
  expect(quiet.el.querySelector(".sr-only")?.textContent).toBe("On in a moment");
});
