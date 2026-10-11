import { afterEach, expect, test, vi } from "vitest";
import { playerControls } from "./src/features/player/focusCycle.ts";
import { pageCovered } from "./src/lib/dialogFocus.ts";
import { optionRows } from "./src/lib/optionArrow.ts";

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function withBoxes() {
  vi.spyOn(HTMLElement.prototype, "getClientRects").mockImplementation(function (this: HTMLElement) {
    if (this.dataset.box === "0") return [] as unknown as DOMRectList;
    const list = [{ width: 10, height: 10 } as DOMRect];
    return list as unknown as DOMRectList;
  });
}

test("the player bar skips controls the viewer cannot land on", () => {
  withBoxes();
  document.body.innerHTML = `
    <div id="stage">
      <button type="button" id="play">Play</button>
      <button type="button" hidden>Hidden</button>
      <button type="button" disabled>Off</button>
      <button type="button" aria-hidden="true">Quiet</button>
      <button type="button" tabindex="-1">Skip</button>
      <input type="hidden" />
      <div inert><button type="button" id="inert">Inert</button></div>
      <button type="button" id="gone" data-box="0">Gone</button>
      <button type="button" id="folded" style="display:none">Folded</button>
      <a id="link" href="#den">Den TV</a>
    </div>`;
  const ids = playerControls(document.getElementById("stage")).map((el) => el.id);
  expect(ids).toEqual(["play", "link"]);
  expect(playerControls(null)).toEqual([]);
});

test("the page is covered only while the shell is inert", () => {
  expect(pageCovered()).toBe(false);
  document.body.innerHTML = `<main class="content"></main>`;
  expect(pageCovered()).toBe(false);
  document.querySelector("main")!.setAttribute("inert", "");
  expect(pageCovered()).toBe(true);
});

test("option rows are one group each, and a disabled choice is left out", () => {
  document.body.innerHTML = `
    <div id="sheet">
      <div class="segmented">
        <button type="button" id="fit">Fit</button>
        <button type="button" id="fill" disabled>Fill</button>
      </div>
      <div class="option-actions"><button type="button" id="done">Done</button></div>
      <input id="level" type="range" />
      <div class="marker-list"><ul><li><button type="button" id="mark">Break</button></li></ul></div>
    </div>`;
  const rows = optionRows(document.getElementById("sheet")).map((row) => row.map((el) => el.id));
  expect(rows).toEqual([["fit"], ["done"], ["level"], ["mark"]]);
  expect(optionRows(null)).toEqual([]);
});
