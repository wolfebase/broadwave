import { act, useRef, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test } from "vitest";
import { useDialogFocus } from "./src/ui/useDialogFocus.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const originalFocus = HTMLElement.prototype.focus;
const focused: HTMLElement[] = [];

HTMLElement.prototype.focus = function (this: HTMLElement, options?: FocusOptions) {
  focused.push(this);
  try {
    originalFocus.call(this, options);
  } catch {
    originalFocus.call(this);
  }
};

function mount(node: ReactNode) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const root: Root = createRoot(el);
  act(() => {
    root.render(node);
  });
  return {
    unmount() {
      act(() => root.unmount());
      el.remove();
    },
  };
}

function press(key: string, target: EventTarget = window, init: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

afterEach(() => {
  document.body.replaceChildren();
  focused.length = 0;
});

test("a dialog focuses its primary control and wraps Tab at the ends", () => {
  function Box() {
    const ref = useRef<HTMLDivElement>(null);
    useDialogFocus(ref);
    return (
      <div ref={ref} role="dialog">
        <button type="button">Back</button>
        <button type="button" className="primary">
          Close
        </button>
        <button type="button">More</button>
      </div>
    );
  }
  const view = mount(<Box />);
  const buttons = [...document.querySelectorAll("button")];
  expect(document.activeElement).toBe(buttons[1]);

  buttons[2].focus();
  const tab = press("Tab");
  expect(tab.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons[0]);

  const shift = press("Tab", window, { shiftKey: true });
  expect(shift.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons[2]);

  buttons[1].focus();
  const middle = press("Tab");
  expect(middle.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(buttons[1]);

  const outside = document.createElement("button");
  outside.textContent = "Guide";
  document.body.appendChild(outside);
  outside.focus();
  const fromOutside = press("Tab", outside);
  expect(fromOutside.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons[0]);
  outside.focus();
  const fromOutsideBack = press("Tab", outside, { shiftKey: true });
  expect(fromOutsideBack.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(buttons[2]);

  for (const extra of [{ ctrlKey: true }, { altKey: true }, { metaKey: true }] as KeyboardEventInit[]) {
    const held = press("Tab", window, extra);
    expect(held.defaultPrevented).toBe(false);
  }
  view.unmount();
});

test("escape and back close the dialog, except backspace in a text field", () => {
  const escapes: string[] = [];
  function Box() {
    const ref = useRef<HTMLDivElement>(null);
    const [label, setLabel] = useState("first");
    useDialogFocus(ref, () => escapes.push(label));
    return (
      <div ref={ref} role="dialog">
        <button type="button" className="primary" onClick={() => setLabel("second")}>
          Close
        </button>
        <input aria-label="Name" defaultValue="Den TV" />
        <input type="checkbox" aria-label="Keep" />
        <textarea aria-label="Note" defaultValue="hi" />
      </div>
    );
  }
  const view = mount(<Box />);
  const button = document.querySelector("button")!;
  act(() => {
    button.click();
  });
  press("Escape");
  expect(escapes).toEqual(["second"]);

  const field = document.querySelector("input")!;
  const typing = press("Backspace", field);
  expect(typing.defaultPrevented).toBe(false);
  expect(escapes).toEqual(["second"]);

  const area = document.querySelector("textarea")!;
  expect(press("Backspace", area).defaultPrevented).toBe(false);

  const box = document.querySelector("input[type='checkbox']")!;
  const back = press("Backspace", box);
  expect(back.defaultPrevented).toBe(true);
  expect(escapes).toEqual(["second", "second"]);
  view.unmount();
});

test("hidden, disabled, and inert controls are skipped", () => {
  function Box() {
    const ref = useRef<HTMLDivElement>(null);
    useDialogFocus(ref);
    return (
      <div ref={ref} role="dialog">
        <button type="button" hidden>
          Hidden
        </button>
        <button type="button" disabled>
          Off
        </button>
        <button type="button" aria-hidden="true">
          Quiet
        </button>
        <button type="button" tabIndex={-1}>
          Skip
        </button>
        <input type="hidden" />
        <div inert>
          <button type="button">Inside</button>
        </div>
        <a href="#den">Den TV</a>
      </div>
    );
  }
  mount(<Box />);
  expect(document.activeElement).toBe(document.querySelector("a"));
});

test("an empty dialog focuses its root", () => {
  function Box() {
    const ref = useRef<HTMLDivElement>(null);
    useDialogFocus(ref);
    return <div ref={ref} role="dialog" tabIndex={-1} />;
  }
  mount(<Box />);
  expect(document.activeElement).toBe(document.querySelector("[role='dialog']"));
  const tab = press("Tab");
  expect(tab.defaultPrevented).toBe(true);
  expect(document.activeElement?.getAttribute("role")).toBe("dialog");
});

test("closing puts focus back, unless the opener is inert", () => {
  const opener = document.createElement("button");
  opener.textContent = "Guide";
  document.body.appendChild(opener);
  opener.focus();

  function Dialog({ onClose }: { onClose: () => void }) {
    const ref = useRef<HTMLDivElement>(null);
    useDialogFocus(ref);
    return (
      <div ref={ref} role="dialog">
        <button type="button" className="primary" onClick={onClose}>
          Close
        </button>
      </div>
    );
  }
  function App() {
    const [show, setShow] = useState(true);
    return show ? <Dialog onClose={() => setShow(false)} /> : null;
  }
  const view = mount(<App />);
  expect(document.activeElement?.textContent).toBe("Close");
  act(() => {
    (document.activeElement as HTMLButtonElement).click();
  });
  expect(document.activeElement).toBe(opener);
  view.unmount();

  const stage = document.createElement("div");
  stage.className = "stage";
  stage.tabIndex = -1;
  document.body.appendChild(stage);
  const inert = document.createElement("div");
  inert.setAttribute("inert", "");
  inert.appendChild(opener);
  document.body.appendChild(inert);
  opener.focus();
  function Again() {
    const ref = useRef<HTMLDivElement>(null);
    useDialogFocus(ref);
    return (
      <div ref={ref} role="dialog">
        <button type="button" className="primary">
          Close
        </button>
      </div>
    );
  }
  const second = mount(<Again />);
  second.unmount();
  expect(document.activeElement).toBe(stage);
});
