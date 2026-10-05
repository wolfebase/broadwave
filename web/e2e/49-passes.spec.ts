import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Pass = { id: number; title: string; kind: string; matchKind?: string; priority?: number; days?: number[]; timeStart?: string; timeEnd?: string; keepMode?: string; keepCount?: number };

const here = path.dirname(fileURLToPath(import.meta.url));
const seeded = (JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { now: number }).now;

async function passes(page: import("@playwright/test").Page) {
  return ((await (await page.request.get("/api/v1/passes")).json()) as { passes: Pass[] }).passes;
}

// The server reads pass windows in its own zone, which is this process's.
function clock(ms: number) {
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

test("passes take keyword, category, day, and time rules, and keep an order", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
  await page.goto("/schedule");
  await settle(page);

  // A category pass held to the hour of the basketball game: the game already on stays out.
  await page.getByRole("button", { name: "New pass" }).click();
  const draft = page.getByRole("form", { name: "New pass" });
  await draft.getByLabel("Match").selectOption("category");
  await draft.getByLabel("Category").fill("Sports");
  await expect(draft.getByRole("status")).toContainText("NFL: Bears at Bills");
  const nba = seeded + 3 * 60 * 60_000;
  await draft.getByLabel("Starts after").fill(clock(nba - 10 * 60_000));
  await draft.getByLabel("Starts before").fill(clock(nba + 10 * 60_000));
  await expect(draft.getByRole("status")).toContainText("Records 1 airing in the next 2 weeks.");
  await expect(draft.getByRole("status")).toContainText("NBA: Lakers at Celtics");
  await expect(draft.getByRole("status")).not.toContainText("NFL");
  await draft.getByRole("button", { name: "Add pass" }).click();
  await expect(page.getByText("Sports (category)")).toBeVisible();

  // Words in a title, on a day it doesn't air, then on its day.
  const late = new Date(seeded + 100 * 60_000).getDay();
  const other = (late + 3) % 7;
  const names = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  await page.getByRole("button", { name: "New pass" }).click();
  await draft.getByLabel("Match").selectOption("contains");
  await draft.getByLabel("Words in the title").fill("late local");
  await draft.getByRole("button", { name: names[other], exact: true }).click();
  await expect(draft.getByRole("button", { name: names[other], exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(draft.getByRole("status")).toContainText("Nothing in the guide matches in the next 2 weeks.");
  await draft.getByRole("button", { name: names[late], exact: true }).click();
  await expect(draft.getByRole("status")).toContainText("Records 1 airing in the next 2 weeks.");
  await expect(draft.getByRole("status")).toContainText("Late Local News");
  await draft.getByRole("button", { name: "Add pass" }).click();
  await expect(page.getByText("Titles with “late local”")).toBeVisible();

  let saved = await passes(page);
  const sports = saved.find((p) => p.matchKind === "category");
  const words = saved.find((p) => p.matchKind === "contains");
  expect(sports?.timeStart).toBe(clock(nba - 10 * 60_000));
  expect(words?.days?.slice().sort()).toEqual([late, other].sort());
  if (!sports || !words) throw new Error("passes not saved");

  // Up and Down save the order; the first pass gets the tuner.
  const rows = page.locator(".pass-row");
  await expect(rows.nth(0)).toContainText("Sports (category)");
  await page.getByRole("button", { name: "Move late local up" }).click();
  await expect(rows.nth(0)).toContainText("Titles with “late local”");
  await expect.poll(async () => (await passes(page)).map((p) => p.title)).toEqual(["late local", "Sports"]);
  await expect(page.getByRole("button", { name: "Move late local up" })).toBeDisabled();

  // Dragging works too.
  await rows.nth(1).dragTo(rows.nth(0));
  await expect(rows.nth(0)).toContainText("Sports (category)");
  await expect.poll(async () => (await passes(page)).map((p) => `${p.title}:${p.priority}`)).toEqual(["Sports:2", "late local:1"]);

  // Edit keep rules.
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: "Edit" }).click();
  const edit = page.getByRole("form", { name: "Edit late local" });
  await edit.getByLabel("Keep").selectOption("last");
  await edit.getByLabel("Recordings to keep").fill("3");
  await edit.getByRole("button", { name: "Save" }).click();
  await expect(rows.filter({ hasText: "late local" })).toContainText("Keeps the newest 3");
  saved = await passes(page);
  expect(saved.find((p) => p.id === words.id)).toMatchObject({ keepMode: "last", keepCount: 3 });

  // The preview names what a pass would push out.
  await page.route("**/api/v1/passes/preview", (route) =>
    route.fulfill({
      json: {
        tunerCount: 2,
        items: [{ passId: sports.id, airing: { id: 1, channelId: 0, title: "NBA: Lakers at Celtics", start: new Date(nba).toISOString(), end: new Date(nba + 3600_000).toISOString() }, priority: 0, padBefore: 1, padAfter: 2, conflict: true, skipped: true }],
        bumps: [{ passId: words.id, airing: { id: 2, channelId: 0, title: "Late Local News", start: new Date(nba).toISOString(), end: new Date(nba + 3600_000).toISOString() }, priority: 0, padBefore: 1, padAfter: 2, conflict: true, skipped: true }],
      },
    }),
  );
  await rows.filter({ hasText: "Sports" }).getByRole("button", { name: "Edit" }).click();
  const sportsEdit = page.getByRole("form", { name: "Edit Sports" });
  await expect(sportsEdit.getByRole("status")).toContainText("Matches 1 airing in the next 2 weeks, but none will record.");
  await expect(sportsEdit.getByRole("status")).toContainText("Skipped: a higher pass has the tuner");
  await expect(sportsEdit.getByRole("status")).toContainText("This pass would stop these from recording:");
  await expect(sportsEdit.getByRole("status")).toContainText("Late Local News");
  await page.unroute("**/api/v1/passes/preview");
  await sportsEdit.getByRole("button", { name: "Cancel" }).click();

  // Fits a phone.
  await page.setViewportSize({ width: 390, height: 844 });
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: "Edit" }).click();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await page.getByRole("form", { name: "Edit late local" }).getByRole("button", { name: "Cancel" }).click();

  await page.getByRole("button", { name: "Remove late local" }).click();
  await page.getByRole("button", { name: "Remove Sports" }).click();
  await expect(page.getByText(/A pass records every airing that matches/)).toBeVisible();
});
