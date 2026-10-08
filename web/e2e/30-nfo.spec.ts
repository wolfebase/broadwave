// A finished recording gets a .nfo beside it when the setting is on.
// The file stays in the recordings folder and leaves with the recording.
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l59");

type Channel = { id: number; number: string; name: string };
type Rec = { id: number; title: string; status: string; bytes?: number };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function newsChannel(): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === "KBWV2");
  if (!found) throw new Error("no news channel");
  return found;
}

function sql(db: string, statement: string) {
  const result = spawnSync("sqlite3", [db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  // busy_timeout prints the value it set. The statement result is the last line.
  return result.stdout.trim().split("\n").at(-1) ?? "";
}

async function recordings(): Promise<Rec[]> {
  const res = await fetch(`${harness().base}/api/v1/recordings`);
  return ((await res.json()) as { recordings?: Rec[] }).recordings ?? [];
}

test("a finished recording writes an nfo and delete removes it", async ({ page }) => {
  test.setTimeout(120_000);
  mkdirSync(evidence, { recursive: true });
  const prepared = await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } });
  expect(prepared.ok()).toBeTruthy();

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/settings");
  await settle(page);
  const control = page.getByLabel("Write .nfo files for Plex, Jellyfin, and Kodi");
  await expect(control).toBeVisible();
  await expect(control).toHaveValue("0");
  await control.selectOption("On");
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/settings");
      return ((await res.json()) as { writeNfo?: string }).writeNfo;
    })
    .toBe("1");
  await control.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(evidence, "settings.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  const started = await page.request.post("/api/v1/recordings", {
    data: { channelId: newsChannel().id, minutes: 2, title: "Evening News" },
  });
  if (!started.ok()) throw new Error(`record ${started.status()} ${await started.text()}`);
  const created = (await started.json()) as { id: number };
  const id = created.id;
  expect(id).toBeGreaterThan(0);

  try {
    await expect.poll(async () => (await recordings()).find((rec) => rec.id === id)?.bytes ?? 0, { timeout: 20_000 }).toBeGreaterThan(10_000);
    const stopped = await page.request.post(`/api/v1/recordings/${id}/stop`);
    expect(stopped.ok()).toBeTruthy();
    await expect.poll(async () => (await recordings()).find((rec) => rec.id === id)?.status, { timeout: 15_000 }).toBe("complete");

    const filePath = sql(harness().db, `SELECT path FROM recordings WHERE id = ${id};`);
    expect(filePath.endsWith(".ts"), filePath).toBe(true);
    // By show, the layout Plex and Jellyfin read: TV/Show/Show - date - episode.
    const showDir = path.join(harness().config, "work", "recordings", "TV", "Evening News");
    expect(path.dirname(filePath), filePath).toBe(showDir);
    expect(path.basename(filePath)).toMatch(/^Evening News - \d{4}-\d{2}-\d{2} - .+\.ts$/);
    const nfoPath = `${filePath.slice(0, -3)}.nfo`;
    let xml = "";
    await expect
      .poll(() => {
        try {
          xml = readFileSync(nfoPath, "utf8");
        } catch {
          xml = "";
        }
        return xml;
      }, { timeout: 15_000 })
      .toContain("<showtitle>Evening News</showtitle>");
    expect(xml).toContain('encoding="UTF-8"');
    expect(xml).toContain("<title>Local headlines</title>");
    expect(xml).toContain("<plot>The evening newscast.</plot>");
    expect(xml).toContain("<genre>News</genre>");
    expect(xml).not.toContain("<season");
    expect(nfoPath.startsWith(path.dirname(filePath)), nfoPath).toBe(true);

    const refresh = await page.request.post("/api/v1/recordings/nfo");
    expect(refresh.ok()).toBeTruthy();
    const written = (await refresh.json()) as { written?: number };
    expect(written.written).toBeGreaterThan(0);
    expect(readFileSync(nfoPath, "utf8")).toContain("<showtitle>Evening News</showtitle>");

    const removed = await page.request.delete(`/api/v1/recordings/${id}`);
    expect(removed.ok()).toBeTruthy();
    expect(() => readFileSync(nfoPath)).toThrow();
    expect(() => readFileSync(filePath)).toThrow();
    // An earlier spec may have left an episode of the same show beside it.
    expect(!existsSync(showDir) || readdirSync(showDir).length > 0, "an emptied show folder goes").toBe(true);
  } finally {
    await page.request.post(`/api/v1/recordings/${id}/stop`).catch(() => undefined);
    await page.request.delete(`/api/v1/recordings/${id}`).catch(() => undefined);
  }
});
