import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { Recording } from "./src/types.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const api = vi.hoisted(() => ({
  moveRecording: vi.fn<(id: number, name: string) => Promise<void>>(),
  refresh: vi.fn<(what?: string[]) => Promise<void>>(),
}));

vi.mock("./src/api.ts", () => ({
  moveRecording: api.moveRecording,
}));

vi.mock("./src/app/data.tsx", () => ({
  useData: () => ({ refresh: api.refresh }),
}));

vi.mock("./src/app/remote.ts", () => ({
  focusRing(el: HTMLElement | null | undefined) {
    el?.focus();
  },
}));

const { MoveFile } = await import("./src/features/library/MoveFile.tsx");

function rec(extra: Partial<Recording> = {}): Recording {
  return {
    id: 7,
    channelId: 4,
    guideNumber: "4.1",
    title: "Night Desk",
    subtitle: "The Quiet Hour",
    status: "complete",
    startedAt: "2026-10-11T02:00:00Z",
    season: 1,
    episode: 2,
    file: "Night Desk/S01E02.mkv",
    ...extra,
  };
}

let root: Root | null = null;

function mount(item = rec()) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  act(() => {
    next.render(<MoveFile rec={item} />);
  });
  return el;
}

function click(el: Element) {
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

beforeEach(() => {
  api.moveRecording.mockReset();
  api.refresh.mockReset();
  api.moveRecording.mockResolvedValue(undefined);
  api.refresh.mockResolvedValue(undefined);
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

test("rename starts from the file name without its extension", () => {
  const el = mount();
  const open = el.querySelector("button")!;
  expect(open.getAttribute("aria-label")).toBe("Rename file S1 E2, Night Desk, The Quiet Hour");
  click(open);
  const input = el.querySelector("input")!;
  expect(input.value).toBe("Night Desk/S01E02");
  expect(document.activeElement).toBe(input);
  expect(el.querySelector("form")?.getAttribute("aria-label")).toBe("Rename The Quiet Hour");
});

test("a name with no dot, and a file with no name, open as they are stored", () => {
  const bare = mount(rec({ file: "Night Desk", subtitle: undefined, season: undefined, episode: undefined }));
  expect(bare.querySelector("button")?.getAttribute("aria-label")).toBe("Rename file Night Desk");
  click(bare.querySelector("button")!);
  expect(bare.querySelector("input")!.value).toBe("Night Desk");
  expect(bare.querySelector("form")?.getAttribute("aria-label")).toBe("Rename Night Desk");

  act(() => {
    root?.unmount();
  });
  root = null;
  const missing = mount(rec({ file: undefined }));
  click(missing.querySelector("button")!);
  expect(missing.querySelector("input")!.value).toBe("");
  expect((missing.querySelector('button[type="submit"]') as HTMLButtonElement).disabled).toBe(true);
});

test("only the last extension comes off", () => {
  const el = mount(rec({ file: "Night Desk/episode.tar.gz" }));
  click(el.querySelector("button")!);
  expect(el.querySelector("input")!.value).toBe("Night Desk/episode.tar");
});

test("saving the same name closes and does not move the file", async () => {
  const el = mount();
  click(el.querySelector("button")!);
  const input = el.querySelector("input")!;
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, "  Night Desk/S01E02  ");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  expect(input.value).toBe("  Night Desk/S01E02  ");
  await act(async () => {
    el.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect(api.moveRecording).not.toHaveBeenCalled();
  expect(el.querySelector("form")).toBeNull();
  expect(el.querySelector("button")?.textContent).toBe("Rename file");
});

test("a new name is sent as typed, then the library reloads", async () => {
  const el = mount();
  click(el.querySelector("button")!);
  const input = el.querySelector("input")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, "City News/S02E01");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  expect(input.value).toBe("City News/S02E01");
  await act(async () => {
    el.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect(api.moveRecording).toHaveBeenCalledWith(7, "City News/S02E01");
  expect(api.refresh).toHaveBeenCalledWith(["recordings"]);
  expect(el.querySelector("form")).toBeNull();
});

test("a move that fails stays open and says why", async () => {
  api.moveRecording.mockRejectedValue(new Error("That folder is not there."));
  const el = mount();
  click(el.querySelector("button")!);
  const input = el.querySelector("input")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, "Missing/S01E02");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    el.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe("That folder is not there.");
  expect(el.querySelector("form")).not.toBeNull();
  expect((el.querySelector("input") as HTMLInputElement).disabled).toBe(false);

  api.moveRecording.mockRejectedValue("nope");
  await act(async () => {
    el.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect(el.querySelector('[role="alert"]')?.textContent).toBe("The file could not be moved.");
});

test("the buttons stay disabled while the move is in flight", async () => {
  let release: (() => void) | undefined;
  api.moveRecording.mockImplementation(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const el = mount();
  click(el.querySelector("button")!);
  const input = el.querySelector("input")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, "City News/S02E01");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  act(() => {
    el.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect((input as HTMLInputElement).disabled).toBe(true);
  expect((el.querySelector('button[type="submit"]') as HTMLButtonElement).disabled).toBe(true);
  expect((el.querySelectorAll("button")[1] as HTMLButtonElement).disabled).toBe(true);
  await act(async () => {
    release?.();
  });
  expect(el.querySelector("form")).toBeNull();
});

test("escape closes, backspace in the name does not, and cancel puts the old name back", async () => {
  const el = mount();
  click(el.querySelector("button")!);
  const input = el.querySelector("input")!;
  const back = new KeyboardEvent("keydown", { key: "Backspace", bubbles: true, cancelable: true });
  act(() => {
    input.dispatchEvent(back);
  });
  expect(back.defaultPrevented).toBe(false);
  expect(el.querySelector("form")).not.toBeNull();

  const escape = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  act(() => {
    input.dispatchEvent(escape);
  });
  expect(escape.defaultPrevented).toBe(true);
  expect(el.querySelector("form")).toBeNull();
  expect(document.activeElement).toBe(el.querySelector("button"));

  click(el.querySelector("button")!);
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(el.querySelector("input")!, "City News/S02E01");
    el.querySelector("input")!.dispatchEvent(new Event("input", { bubbles: true }));
  });
  click([...el.querySelectorAll("button")].find((button) => button.textContent === "Cancel")!);
  expect(el.querySelector("form")).toBeNull();
  click(el.querySelector("button")!);
  expect(el.querySelector("input")!.value).toBe("Night Desk/S01E02");
});
