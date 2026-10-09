import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Pass = { id: number; title: string; kind: string; matchKind?: string; priority?: number; days?: number[]; timeStart?: string; timeEnd?: string; keepMode?: string; keepCount?: number };

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/l100");
const seeded = (JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { now: number }).now;

async function passes(page: import("@playwright/test").Page) {
  return ((await (await page.request.get("/api/v1/passes")).json()) as { passes: Pass[] }).passes;
}

// The server reads days and times on its own clock; the image runs in another zone than the runner.
let offset = 0;
function serverDate(ms: number) {
  return new Date(ms + offset * 1000);
}
function clock(ms: number) {
  const d = serverDate(ms);
  return `${String(d.getUTCHours()).padStart(2, "0")}:${String(d.getUTCMinutes()).padStart(2, "0")}`;
}

test("passes take keyword, category, day, and time rules, and keep an order", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
  offset = ((await (await page.request.post("/api/v1/passes/preview", { data: { title: "x" } })).json()) as { utcOffset: number }).utcOffset;
  await page.goto("/schedule");
  await settle(page);

  // A category pass held to the hour of the basketball game: the game already on stays out.
  await page.getByRole("button", { name: "New pass" }).click();
  const draft = page.getByRole("form", { name: "New pass" });
  await draft.getByRole("combobox", { name: "Match" }).selectOption("category");
  await draft.getByLabel("Category", { exact: true }).fill("Sports");
  await expect(draft.getByRole("status")).toContainText("NFL: Bears at Bills");
  const nba = seeded + 3 * 60 * 60_000;
  await draft.getByLabel("From", { exact: true }).fill(clock(nba - 10 * 60_000));
  await draft.getByLabel("Until", { exact: true }).fill(clock(nba + 10 * 60_000));
  await expect(draft.getByRole("status")).toContainText("Records 1 airing in the next 2 weeks.");
  await expect(draft.getByRole("status")).toContainText("NBA: Lakers at Celtics");
  await expect(draft.getByRole("status")).not.toContainText("NFL");
  await draft.getByRole("button", { name: "Add pass" }).click();
  await expect(page.getByText("Sports (category)")).toBeVisible();

  // Words in a title, on a day it doesn't air, then on its day.
  const late = serverDate(seeded + 100 * 60_000).getUTCDay();
  const other = (late + 3) % 7;
  const names = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  await page.getByRole("button", { name: "New pass" }).click();
  await draft.getByRole("combobox", { name: "Match" }).selectOption("contains");
  await draft.getByLabel("Words", { exact: true }).fill("late local");
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
  // Titles sort without case until there is an order.
  await expect(rows.nth(0)).toContainText("Titles with “late local”");
  await page.getByRole("button", { name: "Move Sports up" }).click();
  await expect(rows.nth(0)).toContainText("Sports (category)");
  await expect.poll(async () => (await passes(page)).map((p) => p.title)).toEqual(["Sports", "late local"]);
  await expect(page.getByRole("button", { name: "Move Sports up" })).toBeDisabled();
  // At the top Up turns off, so focus moves to Down.
  await expect(page.getByRole("button", { name: "Move Sports down" })).toBeFocused();

  // Dragging works too.
  await rows.nth(1).dragTo(rows.nth(0));
  await expect(rows.nth(0)).toContainText("Titles with “late local”");
  await expect.poll(async () => (await passes(page)).map((p) => `${p.title}:${p.priority}`)).toEqual(["late local:2", "Sports:1"]);

  // Edit keep rules.
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: /^Edit / }).click();
  const edit = page.getByRole("form", { name: "Edit late local" });
  await edit.getByRole("combobox", { name: "Keep" }).selectOption("last");
  await edit.getByLabel("How many", { exact: true }).fill("3");
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
  await rows.filter({ hasText: "Sports" }).getByRole("button", { name: /^Edit / }).click();
  const sportsEdit = page.getByRole("form", { name: "Edit Sports" });
  await expect(sportsEdit.getByRole("status")).toContainText("Matches 1 airing in the next 2 weeks, but none will record.");
  await expect(sportsEdit.getByRole("status")).toContainText("Skipped: another pass has the tuner");
  await expect(sportsEdit.getByRole("status")).toContainText("This pass would stop these from recording:");
  await expect(sportsEdit.getByRole("status")).toContainText("Late Local News");
  await page.unroute("**/api/v1/passes/preview");
  await sportsEdit.getByRole("button", { name: "Cancel" }).click();

  // A rename changes the words, and the list follows.
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: /^Edit / }).click();
  const rename = page.getByRole("form", { name: "Edit late local" });
  await rename.getByLabel("Words", { exact: true }).fill("late local news");
  await rename.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Titles with “late local news”")).toBeVisible();
  expect((await passes(page)).find((p) => p.id === words.id)?.title).toBe("late local news");

  // Fits a TV.
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/schedule?layout=tv");
  await settle(page);
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: /^Edit / }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
  await page.getByRole("form", { name: "Edit late local news" }).getByRole("button", { name: "Cancel" }).click();

  // Fits a phone.
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/schedule");
  await settle(page);
  await rows.filter({ hasText: "late local" }).getByRole("button", { name: /^Edit / }).click();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await page.getByRole("form", { name: "Edit late local news" }).getByRole("button", { name: "Cancel" }).click();

  // Removing asks first. Escape and Cancel keep the pass; Remove deletes it.
  mkdirSync(evidence, { recursive: true });
  const ask = page.getByText("Remove Titles with “late local news”?");
  const stay = page.getByText("Recordings it made stay.");

  async function openAsk() {
    await page.getByRole("button", { name: "Remove late local news" }).click();
    await expect(ask).toBeVisible();
    await expect(stay).toBeVisible();
    await expect(page.getByRole("button", { name: "Cancel" })).toBeFocused();
  }

  await openAsk();
  await page.screenshot({ path: path.join(evidence, "confirm-390.jpg"), type: "jpeg", quality: 60 });
  await page.keyboard.press("Escape");
  await expect(ask).toHaveCount(0);
  expect((await passes(page)).map((p) => p.title).sort()).toEqual(["Sports", "late local news"]);

  await openAsk();
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(ask).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Remove late local news" })).toBeFocused();
  expect((await passes(page)).find((p) => p.id === words.id)?.title).toBe("late local news");

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/schedule");
  await settle(page);
  await openAsk();
  await page.screenshot({ path: path.join(evidence, "confirm-1440.jpg"), type: "jpeg", quality: 60 });
  await page.getByRole("button", { name: "Cancel" }).click();

  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/schedule?layout=tv");
  await settle(page);
  await openAsk();
  await page.screenshot({ path: path.join(evidence, "confirm-1920.jpg"), type: "jpeg", quality: 60 });

  await page.route("**/api/v1/passes/*", (route) => {
    if (route.request().method() !== "DELETE") return route.continue();
    return route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ code: "not_found", message: "pass not found" }) });
  });
  await page.getByRole("button", { name: "Remove Titles with “late local news”", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("pass not found");
  expect((await passes(page)).find((p) => p.id === words.id)?.title).toBe("late local news");
  await page.unroute("**/api/v1/passes/*");

  await page.getByRole("button", { name: "Remove Titles with “late local news”", exact: true }).click();
  await expect(page.getByText("Titles with “late local news”")).toHaveCount(0);
  expect((await passes(page)).map((p) => p.title)).toEqual(["Sports"]);

  await page.getByRole("button", { name: "Remove Sports" }).click();
  await expect(page.getByText("Remove Sports (category)?")).toBeVisible();
  await page.getByRole("button", { name: "Remove Sports (category)", exact: true }).click();
  await expect(page.getByText(/A pass records every airing that matches/)).toBeVisible();
  expect(await passes(page)).toEqual([]);
});
