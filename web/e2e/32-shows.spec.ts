// Finished recordings of one title show up together under Settings, with a link to that show.
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l64");

type Channel = { id: number; number: string; name: string };
type Rec = { id: number; title: string; status: string; bytes?: number };
type Show = { title: string; count: number; bytes: number };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string };
}

function newsChannel(): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === "KBWV2");
  if (!found) throw new Error("no news channel");
  return found;
}

async function recordings(): Promise<Rec[]> {
  const res = await fetch(`${harness().base}/api/v1/recordings`);
  return ((await res.json()) as { recordings?: Rec[] }).recordings ?? [];
}

async function shows(): Promise<Show[]> {
  const res = await fetch(`${harness().base}/api/v1/storage/shows`);
  if (!res.ok) throw new Error(`shows ${res.status}`);
  return ((await res.json()) as { shows?: Show[] }).shows ?? [];
}

function countOf(list: Show[], title: string) {
  return list.find((row) => row.title.toLocaleLowerCase() === title.toLocaleLowerCase())?.count ?? 0;
}

async function recordOne(page: import("@playwright/test").Page, channelId: number) {
  let started = await page.request.post("/api/v1/recordings", {
    data: { channelId, minutes: 2, title: "Evening News" },
  });
  for (let i = 0; i < 5 && !started.ok(); i++) {
    await page.waitForTimeout(1000);
    started = await page.request.post("/api/v1/recordings", {
      data: { channelId, minutes: 2, title: "Evening News" },
    });
  }
  if (!started.ok()) throw new Error(`record ${started.status()} ${await started.text()}`);
  const created = (await started.json()) as { id: number; title: string };
  await expect
    .poll(async () => (await recordings()).find((rec) => rec.id === created.id)?.bytes ?? 0, { timeout: 20_000 })
    .toBeGreaterThan(1000);
  const stopped = await page.request.post(`/api/v1/recordings/${created.id}/stop`);
  if (!stopped.ok()) throw new Error(`stop ${stopped.status()}`);
  await expect.poll(async () => (await recordings()).find((rec) => rec.id === created.id)?.status, { timeout: 20_000 }).toBe("complete");
  return created;
}

test("two recordings of one show are one row", async ({ page }) => {
  test.setTimeout(120_000);
  mkdirSync(evidence, { recursive: true });
  const prepared = await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } });
  expect(prepared.ok()).toBeTruthy();

  const before = await shows();
  const made: { id: number; title: string }[] = [];
  try {
    const first = await recordOne(page, newsChannel().id);
    made.push(first);
    const second = await recordOne(page, newsChannel().id);
    made.push(second);
    expect(first.title).toBe(second.title);
    const title = first.title;

    const after = await shows();
    const row = after.find((item) => item.title.toLocaleLowerCase() === title.toLocaleLowerCase());
    expect(row, title).toBeTruthy();
    expect(row?.count).toBe(countOf(before, title) + 2);
    expect(row?.bytes ?? 0).toBeGreaterThan(0);

    const space = page.getByRole("region", { name: "Space by show" });
    for (const size of [
      { name: "390", width: 390, height: 844 },
      { name: "1440", width: 1440, height: 900 },
    ] as const) {
      await page.setViewportSize({ width: size.width, height: size.height });
      await page.goto("/settings");
      await settle(page);
      await expect(space.getByRole("heading", { name: "Space by show" })).toBeVisible();
      const link = space.getByRole("link", { name: row?.title ?? title });
      await link.scrollIntoViewIfNeeded();
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", `/recordings?show=${encodeURIComponent(row?.title ?? title)}`);
      await expect(space.getByText(`${row?.count} recording${row?.count === 1 ? "" : "s"}`, { exact: true })).toBeVisible();
      await space.screenshot({ path: path.join(evidence, `settings-${size.name}.jpg`), type: "jpeg", quality: 70, animations: "disabled" });
    }

    await page.setViewportSize({ width: 1440, height: 900 });
    await space.getByRole("link", { name: row?.title ?? title }).click();
    await expect(page).toHaveURL(/\/recordings\?show=/);
    await settle(page);
    await expect(page.getByRole("heading", { name: title, level: 3 })).toBeVisible();
    await expect(page.locator(`img.poster[src="/media/poster/${first.id}"]`)).toBeVisible();
    await expect(page.locator(`img.poster[src="/media/poster/${second.id}"]`)).toBeVisible();
  } finally {
    for (const rec of made) {
      await page.request.post(`/api/v1/recordings/${rec.id}/stop`).catch(() => undefined);
      await page.request.delete(`/api/v1/recordings/${rec.id}`).catch(() => undefined);
    }
  }
});
