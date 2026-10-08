// A multiview added to other apps is one channel in the exports, its stream
// is one MPEG-TS picture, and Settings can take it off the list again.
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

type Channel = { id: number; number: string; name: string };

function channel(name: string): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

test("a multiview added to other apps is one channel in the exports", async ({ page, baseURL }) => {
  test.setTimeout(120_000);
  const a = channel("KBWV");
  const b = channel("WTST");
  const key = `${b.id}-${a.id}`;
  await fetch(`${baseURL}/api/v1/settings`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ exportMosaics: "" }) });
  await page.goto(`/multiview?ch=${a.id},${b.id}&layout=2up&focus=${b.id}`);
  const add = page.getByRole("button", { name: "Add to other apps" });
  await add.click();
  await expect(page.getByRole("status").filter({ hasText: "Other apps list this as channel 990.1." })).toBeVisible();
  await expect(page.getByRole("button", { name: "In other apps" })).toBeDisabled();

  // The sound tile comes first in the key, so 5.1's sound plays.
  const settings = (await (await fetch(`${baseURL}/api/v1/settings`)).json()) as { exportMosaics?: string };
  expect(settings.exportMosaics).toBe(key);
  const m3u = await (await fetch(`${baseURL}/export/lineup.m3u`)).text();
  expect(m3u).toContain(`tvg-chno="990.1" tvg-name="Multiview: WTST + KBWV"`);
  expect(m3u).toContain(`${baseURL}/export/mosaic/${key}\n`);

  // What Plex or Jellyfin would read: one 1920x1080 picture with sound.
  const probe = spawnSync("ffprobe", ["-v", "error", "-rw_timeout", "30000000", "-show_entries", "stream=codec_type,width", "-of", "csv=p=0", `${baseURL}/export/mosaic/${key}`], {
    encoding: "utf8",
    timeout: 60_000,
  });
  expect(probe.stdout).toContain("video,1920");
  expect(probe.stdout).toContain("audio");

  await page.goto("/settings");
  const row = page.getByText("990.1 · Multiview: WTST + KBWV");
  await expect(row).toBeVisible();
  await page.getByRole("button", { name: "Remove Multiview: WTST + KBWV" }).click();
  await expect(row).toBeHidden();
  await expect(page.getByText("Open a multiview and choose Add to other apps.")).toBeVisible();
  const after = await (await fetch(`${baseURL}/export/lineup.m3u`)).text();
  expect(after).not.toContain("990.1");
});
