// The library groups a show by season, keeps what was started in Continue watching,
// sorts and filters, and marks or deletes many recordings at once.
import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/e3");

type Rec = { id: number; title: string; watched?: number; season?: number; episode?: number; progressAt?: string };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

function stamp(ms: number) {
  return new Date(ms).toISOString().replace(/\.\d+Z$/, "Z");
}

async function recordings(): Promise<Rec[]> {
  return ((await (await fetch(`${harness().base}/api/v1/recordings`)).json()) as { recordings: Rec[] }).recordings;
}

// Real files, so each has a size; the length is stored so nothing probes them.
function plant() {
  const dir = path.join(harness().config, "work", "recordings");
  mkdirSync(dir, { recursive: true });
  const sample = path.join(here, ".run", "sample.ts");
  const rows: [string, string, string, number, number, number][] = [
    ["Mystery Hour", "The Locked Room", "Series", 1, 1, 9],
    ["Mystery Hour", "The Second Key", "Series", 1, 2, 8],
    ["Mystery Hour", "A New Town", "Series", 2, 1, 7],
    ["Mystery Hour", "The Lighthouse", "Series", 2, 2, 6],
    ["Mystery Hour", "Low Tide", "Series", 2, 3, 5],
    ["A Western", "", "Movie", 0, 0, 4],
    ["NFL: Rivertown at Lakeside", "", "Sports", 0, 0, 3],
  ];
  const ids: Record<string, number> = {};
  for (const [title, subtitle, category, season, episode, daysAgo] of rows) {
    const file = path.join(dir, `e3-${title.replace(/\W+/g, "-")}-${season}-${episode}.ts`);
    if (!existsSync(file)) copyFileSync(sample, file);
    ids[subtitle || title] = Number(
      sql(
        `INSERT INTO recordings (channel_id, guide_number, title, subtitle, category, path, status, started_at, duration_sec, season, episode) VALUES (1, '4.1', '${title}', '${subtitle}', '${category}', '${file}', 'complete', '${stamp(Date.now() - daysAgo * 86_400_000)}', 1800, ${season}, ${episode}); SELECT last_insert_rowid();`,
      ),
    );
  }
  sql(`UPDATE recordings SET watched = 1 WHERE id = ${ids["The Locked Room"]};`);
  sql(`INSERT INTO progress (recording_id, position_sec, updated_at) VALUES (${ids["The Second Key"]}, 600, '${stamp(Date.now() - 60_000)}');`);
  sql(`INSERT INTO progress (recording_id, position_sec, updated_at) VALUES (${ids["A Western"]}, 900, '${stamp(Date.now() - 3_600_000)}');`);
  return ids;
}

test("the library groups seasons, resumes, filters, and changes many at once", async ({ page }) => {
  test.setTimeout(90_000);
  mkdirSync(evidence, { recursive: true });
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  const ids = plant();
  const listed = await recordings();
  const second = listed.find((rec) => rec.id === ids["The Second Key"]);
  expect(second?.season).toBe(1);
  expect(second?.episode).toBe(2);
  expect(second?.progressAt).toBeTruthy();

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/recordings");
  await settle(page);

  // Last played first; a watched one is not there.
  const resume = page.locator(".shelf").filter({ has: page.getByRole("heading", { name: "Continue watching" }) });
  await expect(resume.locator(".rec-card")).toHaveCount(2);
  await expect(resume.locator(".rec-card").first()).toContainText("The Second Key");
  await expect(resume.locator(".rec-card").nth(1)).toContainText("A Western");

  const show = page.locator(".show-grid > section").filter({ has: page.getByRole("heading", { name: "Mystery Hour", exact: true }) });
  await expect(show.locator(":scope > .ch-tags")).toContainText("5 recordings · 4 unwatched");
  await expect(show.locator(".media-card")).toHaveCount(4);
  // Newest first: the latest season's last episode leads.
  await expect(show.locator(".media-card").first()).toContainText("S2 E3");
  await page.screenshot({ path: path.join(evidence, "desktop.png") });

  const kind = page.getByRole("combobox", { name: "Show" });
  await kind.selectOption("movies");
  await expect(page.getByRole("heading", { name: "Movies" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Mystery Hour", exact: true })).toHaveCount(0);
  await kind.selectOption("sports");
  await expect(page.getByRole("heading", { name: "NFL: Rivertown at Lakeside" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Movies" })).toHaveCount(0);
  await kind.selectOption("all");
  await page.getByRole("combobox", { name: "Sort" }).selectOption("title");
  await expect(page.locator(".show-title").first()).toHaveText("Mystery Hour");

  // A show's own page lists every episode by season, first episode first.
  await page.getByRole("button", { name: "All 5 of Mystery Hour" }).click();
  await expect(page).toHaveURL(/\/recordings\?show=Mystery(%20|\+)Hour/);
  await expect(page.getByRole("heading", { name: "Season 1" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Season 2" })).toBeVisible();
  await expect(page.locator(".show-title").first()).toHaveText("Season 1");
  await expect(page.locator(".media-card").first()).toContainText("S1 E1");
  await expect(page.locator(".media-card")).toHaveCount(5);

  // Mark two watched in one go, by keyboard.
  await page.getByRole("button", { name: "Select" }).click();
  const pickA = page.getByRole("checkbox", { name: "Select S2 E1 A New Town" });
  await pickA.focus();
  await page.keyboard.press("Space");
  await page.getByRole("checkbox", { name: "Select S2 E2 The Lighthouse" }).check();
  await expect(page.getByRole("toolbar", { name: "Selected recordings" })).toContainText("2 selected");
  await page.screenshot({ path: path.join(evidence, "desktop-select.png") });
  await page.getByRole("toolbar", { name: "Selected recordings" }).getByRole("button", { name: "Mark watched" }).click();
  await expect(page.locator(".page .lede")).toHaveText("Marked 2 recordings watched.");
  await expect.poll(async () => (await recordings()).filter((rec) => [ids["A New Town"], ids["The Lighthouse"]].includes(rec.id)).map((rec) => rec.watched)).toEqual([1, 1]);
  await expect(page.getByRole("checkbox")).toHaveCount(0);

  // Delete asks in place, then removes every file of the show.
  await page.getByRole("button", { name: "Select" }).click();
  const bar = page.getByRole("toolbar", { name: "Selected recordings" });
  await bar.getByRole("button", { name: "Select all" }).click();
  await expect(bar).toContainText("5 selected");
  await bar.getByRole("button", { name: "Delete", exact: true }).click();
  await bar.getByRole("button", { name: "Keep them" }).click();
  await expect(bar.getByRole("button", { name: "Delete", exact: true })).toBeVisible();
  await bar.getByRole("button", { name: "Delete", exact: true }).click();
  await bar.getByRole("button", { name: "Delete 5 files" }).click();
  await expect(page.locator(".page .lede")).toHaveText("Deleted 5 recordings.");
  await expect.poll(async () => (await recordings()).filter((rec) => rec.title === "Mystery Hour").length).toBe(0);

  await page.getByRole("button", { name: "All recordings" }).click();
  await expect(page).toHaveURL(/\/recordings$/);
  await expect(page.getByRole("heading", { name: "Movies" })).toBeVisible();
});

test("the library fits a phone and a TV", async ({ page }) => {
  plant();
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  for (const [name, width, height, url] of [
    ["phone", 390, 844, "/recordings"],
    ["tv", 1920, 1080, "/recordings?layout=tv"],
  ] as const) {
    await page.setViewportSize({ width, height });
    await page.goto(url);
    await settle(page);
    await expect(page.getByRole("heading", { name: "Continue watching" })).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, name).toBeLessThanOrEqual(0);
    // The page can clip what is wider than the screen, so check the pieces themselves.
    const past = await page.evaluate(() =>
      [...document.querySelectorAll(".library-tools > *, .media-card, .page-head")].filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1).map((el) => el.className || el.tagName),
    );
    expect(past, name).toEqual([]);
    await page.screenshot({ path: path.join(evidence, `${name}.png`) });
  }
  // Remove what this test planted so later specs see the library they expect.
  sql(`DELETE FROM progress WHERE recording_id IN (SELECT id FROM recordings WHERE path LIKE '%/e3-%'); DELETE FROM recordings WHERE path LIKE '%/e3-%';`);
});
