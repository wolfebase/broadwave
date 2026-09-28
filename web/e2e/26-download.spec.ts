// A finished recording can be saved from the library row and from its page.
// The link is a plain anchor. The file is not read into the page.
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l55");

type Rec = { id: number; title: string; status: string; bytes?: number };
type Channel = { id: number; number: string; name: string };

const sizes = [
  { name: "390", width: 390, height: 844 },
  { name: "1440", width: 1440, height: 900 },
  { name: "1920", width: 1920, height: 1080 },
] as const;

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

function libraryRow(page: Page, id: number) {
  // An earlier spec can replace the listing this recording was named from.
  // The poster is this recording either way.
  return page.locator(".media-card", { has: page.locator(`img.poster[src="/media/poster/${id}"]`) });
}

async function wake(page: Page) {
  const stage = page.getByRole("region", { name: "Player" });
  const box = await stage.boundingBox();
  if (!box) return;
  await page.mouse.move(box.x + 40, box.y + 40);
  await page.mouse.move(box.x + box.width / 2, box.y + 80);
}

async function tabTo(page: Page, link: ReturnType<Page["getByRole"]>) {
  for (let i = 0; i < 40; i++) {
    if (await link.evaluate((el) => el === document.activeElement).catch(() => false)) return;
    await page.keyboard.press("Tab");
  }
}

test("a finished recording can be downloaded", async ({ page }) => {
  test.setTimeout(150_000);
  mkdirSync(evidence, { recursive: true });
  const prepared = await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } });
  expect(prepared.ok()).toBeTruthy();

  const started = await page.request.post("/api/v1/recordings", {
    data: { channelId: newsChannel().id, minutes: 2, title: "Evening News" },
  });
  if (!started.ok()) throw new Error(`record ${started.status()} ${await started.text()}`);
  const created = (await started.json()) as { id: number; title: string };
  const id = created.id;
  const title = created.title;
  expect(id).toBeGreaterThan(0);
  expect(title).toBeTruthy();

  try {
    await expect.poll(async () => (await recordings()).find((rec) => rec.id === id)?.bytes ?? 0, { timeout: 20_000 }).toBeGreaterThan(10_000);

    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/recordings");
    await settle(page);
    const recording = libraryRow(page, id);
    await expect(recording.getByRole("button", { name: "Stop recording" })).toBeVisible();
    await expect(recording.getByRole("link", { name: "Download" })).toHaveCount(0);

    await recording.getByRole("button", { name: "Stop recording" }).click();
    await expect(page.getByText(`Stopped ${title}. What it recorded is kept.`)).toBeVisible();
    const link = recording.getByRole("link", { name: "Download" });
    await expect(link).toHaveAttribute("href", `/api/v1/recordings/${id}/file`);
    await expect(link).toHaveAttribute("download", "");

    await recording.getByRole("button", { name: "Play" }).focus();
    await tabTo(page, link);
    await expect(link).toBeFocused();

    const file = `/api/v1/recordings/${id}/file`;
    const head = await page.request.fetch(file, { method: "HEAD" });
    expect(head.status()).toBe(200);
    const disposition = head.headers()["content-disposition"] ?? "";
    // A plain name is a token. A name that needs quotes or percent-encoding
    // still has to yield the base name, with no extra header field.
    const quoted = disposition.match(/filename="((?:\\.|[^"])*)"/);
    const plain = disposition.match(/filename=([^;\s]+)/);
    const filename = (quoted?.[1] ?? plain?.[1] ?? "").replace(/\\"/g, '"');
    expect(disposition.startsWith("attachment;"), disposition).toBeTruthy();
    expect(filename.endsWith(".ts"), disposition).toBeTruthy();
    expect(filename.includes("/") || filename.includes("\\"), disposition).toBeFalsy();
    expect(disposition).not.toMatch(/[\r\n]/);
    expect(head.headers()["content-type"] ?? "").toContain("video/mp2t");

    const ranged = await page.request.fetch(file, { headers: { Range: "bytes=0-15" } });
    expect(ranged.status()).toBe(206);
    expect(ranged.headers()["content-disposition"] ?? "").toBe(disposition);
    expect(ranged.headers()["content-range"] ?? "").toMatch(/^bytes 0-15\//);
    const bytes = await ranged.body();
    expect(bytes.length).toBe(16);
    expect(bytes[0]).toBe(0x47);

    for (const size of sizes) {
      await page.setViewportSize({ width: size.width, height: size.height });
      await link.scrollIntoViewIfNeeded();
      await expect(link).toBeVisible();
      await recording.screenshot({ path: path.join(evidence, `library-${size.name}.jpg`), type: "jpeg", quality: 70, animations: "disabled" });
    }

    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`/play?recording=${id}`);
    await settle(page);
    const player = page.getByRole("region", { name: "Player" });
    await expect(player).toBeVisible();
    await expect(player.getByRole("heading", { name: new RegExp(title.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")) })).toBeVisible();
    await wake(page);
    await page.getByRole("button", { name: "Options" }).click();
    const onPage = player.getByRole("link", { name: "Download" });
    await expect(onPage).toHaveAttribute("href", `/api/v1/recordings/${id}/file`);
    await expect(onPage).toHaveAttribute("download", "");
    await page.getByRole("button", { name: "Start over" }).focus();
    await page.keyboard.press("Tab");
    await expect(onPage).toBeFocused();

    for (const size of sizes) {
      await page.setViewportSize({ width: size.width, height: size.height });
      await wake(page);
      if ((await page.getByRole("button", { name: "Options" }).getAttribute("aria-expanded")) !== "true") {
        await page.getByRole("button", { name: "Options" }).click();
      }
      await onPage.scrollIntoViewIfNeeded();
      await expect(onPage).toBeVisible();
      await page.screenshot({ path: path.join(evidence, `page-${size.name}.jpg`), type: "jpeg", quality: 70, animations: "disabled" });
    }
  } finally {
    await page.request.post(`/api/v1/recordings/${id}/stop`).catch(() => undefined);
  }
});
