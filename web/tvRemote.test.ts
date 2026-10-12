import { afterEach, beforeEach, expect, test } from "vitest";
import { focusRing, installTvRemote } from "./src/app/remote.ts";

// jsdom throws on focus({ focusVisible: true }) and reports an empty box.
const originalFocus = HTMLElement.prototype.focus;
const originalRect = HTMLElement.prototype.getBoundingClientRect;
const originalBack = window.history.back;
const originalState = window.history.state;
const originalInnerWidth = window.innerWidth;
const originalInnerHeight = window.innerHeight;

const placed = new WeakMap<Element, DOMRect>();
const focusCalls: { el: HTMLElement; options?: FocusOptions }[] = [];

let stop = () => {};
let backs = 0;

function place<T extends HTMLElement>(el: T, left: number, top: number, width = 120, height = 40): T {
  placed.set(el, new DOMRect(left, top, width, height));
  return el;
}

function button(label: string, left: number, top: number, width = 120, height = 40) {
  const el = document.createElement("button");
  el.type = "button";
  el.textContent = label;
  return place(el, left, top, width, height);
}

function press(key: string, target: EventTarget, init: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

function listen() {
  stop();
  stop = installTvRemote();
}

function sideBySide() {
  const left = button("Den TV", 40, 40);
  const right = button("City News", 200, 40);
  document.body.append(left, right);
  listen();
  left.focus();
  return { left, right };
}

beforeEach(() => {
  backs = 0;
  focusCalls.length = 0;
  window.innerWidth = 1280;
  window.innerHeight = 720;
  window.history.back = () => {
    backs += 1;
  };
  HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
    return placed.get(this) ?? new DOMRect(0, 0, 0, 0);
  };
  HTMLElement.prototype.focus = function (this: HTMLElement, options?: FocusOptions) {
    focusCalls.push({ el: this, options });
    try {
      originalFocus.call(this, options);
    } catch {
      originalFocus.call(this);
    }
  };
});

afterEach(() => {
  stop();
  stop = () => {};
  document.body.replaceChildren();
  HTMLElement.prototype.focus = originalFocus;
  HTMLElement.prototype.getBoundingClientRect = originalRect;
  window.history.back = originalBack;
  window.history.replaceState(originalState, "");
  window.innerWidth = originalInnerWidth;
  window.innerHeight = originalInnerHeight;
  focusCalls.length = 0;
});

test("ArrowRight moves across a row, and ArrowLeft at the end does not", () => {
  const { left, right } = sideBySide();

  const stayed = press("ArrowLeft", left);
  expect(stayed.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(left);

  const moved = press("ArrowRight", left);
  expect(moved.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(right);
});

test("modified and already handled keys do nothing", () => {
  const { left } = sideBySide();

  for (const init of [{ metaKey: true }, { ctrlKey: true }, { altKey: true }] as KeyboardEventInit[]) {
    const held = press("ArrowRight", left, init);
    expect(held.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(left);
  }

  const handled = new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true, cancelable: true });
  handled.preventDefault();
  left.dispatchEvent(handled);
  expect(document.activeElement).toBe(left);
});

test("a text field keeps left and right, and down leaves it", () => {
  const field = document.createElement("input");
  field.type = "text";
  field.setAttribute("aria-label", "Find");
  field.value = "Den TV";
  place(field, 40, 40, 200, 40);
  const beside = button("Night Desk", 280, 40);
  const below = button("City News", 40, 120);
  document.body.append(field, beside, below);
  listen();
  field.focus();

  const right = press("ArrowRight", field);
  expect(right.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(field);

  const left = press("ArrowLeft", field);
  expect(left.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(field);

  const down = press("ArrowDown", field);
  expect(down.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(below);
});

test("a slider keeps left and right", () => {
  const slider = document.createElement("input");
  slider.type = "range";
  slider.setAttribute("aria-label", "Level");
  place(slider, 40, 40, 200, 40);
  const beside = button("City News", 280, 40);
  document.body.append(slider, beside);
  listen();
  slider.focus();

  const right = press("ArrowRight", slider);
  expect(right.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(slider);

  const left = press("ArrowLeft", slider);
  expect(left.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(slider);
});

test("a closed menu steps with right, and down leaves it", () => {
  const menu = document.createElement("select");
  menu.setAttribute("aria-label", "Station");
  for (const name of ["Den TV", "City News"]) {
    const option = document.createElement("option");
    option.textContent = name;
    menu.append(option);
  }
  place(menu, 40, 40, 200, 40);
  const beside = button("Night Desk", 280, 40);
  const below = button("Morning Show", 40, 120);
  const wrap = document.createElement("div");
  wrap.append(menu);
  document.body.append(wrap, beside, below);
  listen();
  menu.focus();

  const changes: Event[] = [];
  const onChange = (event: Event) => changes.push(event);
  document.addEventListener("change", onChange);

  const right = press("ArrowRight", menu);
  expect(right.defaultPrevented).toBe(true);
  expect(menu.selectedIndex).toBe(1);
  expect(document.activeElement).toBe(menu);
  expect(changes).toHaveLength(1);
  expect(changes[0]?.target).toBe(menu);
  expect(changes[0]?.bubbles).toBe(true);

  const last = press("ArrowRight", menu);
  expect(last.defaultPrevented).toBe(true);
  expect(menu.selectedIndex).toBe(1);
  expect(document.activeElement).toBe(menu);
  expect(changes).toHaveLength(1);

  const down = press("ArrowDown", menu);
  expect(down.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(below);
  document.removeEventListener("change", onChange);
});

test("escape blurs a text field, and backspace does not", () => {
  window.history.replaceState({ depth: 1 }, "");
  const field = document.createElement("input");
  field.type = "text";
  field.setAttribute("aria-label", "Find");
  field.value = "Den TV";
  place(field, 40, 40, 220, 40);
  document.body.append(field);
  listen();
  field.focus();

  const escape = press("Escape", field);
  expect(escape.defaultPrevented).toBe(true);
  expect(document.activeElement).not.toBe(field);
  expect(backs).toBe(0);

  field.focus();
  const backspace = press("Backspace", field);
  expect(backspace.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(field);
  expect(backs).toBe(0);
});

test("escape does not leave while a dialog is open", () => {
  window.history.replaceState({ depth: 1 }, "");
  const dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  const close = button("Close", 80, 80);
  dialog.append(close);
  document.body.append(dialog);
  listen();
  close.focus();

  const escape = press("Escape", close);
  expect(backs).toBe(0);
  expect(escape.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(close);
});

test("escape goes back one level when the app pushed it", () => {
  window.history.replaceState({ depth: 1 }, "");
  const home = button("Den TV", 40, 40);
  document.body.append(home);
  listen();
  home.focus();

  const escape = press("Escape", home);
  expect(escape.defaultPrevented).toBe(true);
  expect(backs).toBe(1);
});

test("arrows skip controls a remote cannot land on", () => {
  const shows = button("Shows", 20, 80, 100, 40);
  const off = button("Off", 140, 80, 100, 40);
  off.disabled = true;
  const skip = button("Skip", 260, 80, 100, 40);
  skip.tabIndex = -1;
  const inert = document.createElement("div");
  inert.setAttribute("inert", "");
  const inside = button("Inside", 380, 80, 100, 40);
  inert.append(inside);
  const hidden = document.createElement("div");
  hidden.setAttribute("aria-hidden", "true");
  const quiet = button("Quiet", 500, 80, 100, 40);
  hidden.append(quiet);
  const dot = button("Dot", 620, 80, 1, 40);
  const den = button("Den TV", 760, 80, 120, 40);
  document.body.append(shows, off, skip, inert, hidden, dot, den);
  listen();
  shows.focus();

  const right = press("ArrowRight", shows);
  expect(right.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(den);
});

test("down from the bar enters the first row of the page, and sideways stays on the bar", () => {
  // Night Desk is closer under the tab. Find sits between the tabs. Weather sits
  // just off the bar. Morning Show is the first row in main.
  const bar = document.createElement("div");
  bar.className = "topbar";
  const den = button("Den TV", 40, 8, 120, 36);
  const city = button("City News", 700, 8, 140, 36);
  bar.append(den, city);

  const find = document.createElement("input");
  find.type = "text";
  find.setAttribute("aria-label", "Find");
  place(find, 200, 12, 160, 32);
  const night = button("Night Desk", 700, 52, 140, 36);
  const weather = button("Weather", 900, 8, 120, 36);

  const main = document.createElement("main");
  const morning = button("Morning Show", 40, 140, 160, 44);
  const late = button("Late Edition", 700, 220, 140, 44);
  main.append(morning, late);
  document.body.append(bar, find, night, weather, main);
  listen();

  city.focus();
  const down = press("ArrowDown", city);
  expect(down.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(morning);

  den.focus();
  const right = press("ArrowRight", den);
  expect(right.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(city);

  const left = press("ArrowLeft", city);
  expect(left.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(den);

  city.focus();
  const edge = press("ArrowRight", city);
  expect(edge.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(city);
});

test("focusRing draws a visible ring and accepts an empty target", () => {
  expect(() => focusRing(null)).not.toThrow();
  expect(() => focusRing(undefined)).not.toThrow();
  expect(focusCalls).toEqual([]);

  const den = button("Den TV", 40, 40);
  document.body.append(den);
  focusRing(den);
  expect(focusCalls).toEqual([{ el: den, options: { focusVisible: true } }]);
  expect(document.activeElement).toBe(den);
});

test("a removed remote ignores arrows", () => {
  const { left } = sideBySide();
  stop();
  stop = () => {};

  const event = press("ArrowRight", left);
  expect(event.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(left);
});
