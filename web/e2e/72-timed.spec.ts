import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Pass = { id: number; title: string; kind: string; channelId?: number; timeStart?: string; duration?: number; date?: string; days?: number[] };
type Channel = { id: number; number: string; name: string };

const here = path.dirname(fileURLToPath(import.meta.url));
const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { now: number; channels: Channel[] };

async function passes(page: import("@playwright/test").Page) {
  return ((await (await page.request.get("/api/v1/passes")).json()) as { passes: Pass[] }).passes;
}

// The server reads the date and time on its own clock.
let offset = 0;
function serverParts(ms: number) {
  const d = new Date(ms + offset * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return { date: `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`, clock: `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`, day: d.getUTCDay() };
}

test("record a time: once, then on days, from the schedule and from a channel", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
  offset = ((await (await page.request.post("/api/v1/passes/preview", { data: { title: "x" } })).json()) as { utcOffset: number }).utcOffset;
  const ch = runtime.channels.find((c) => c.number === "4.1") ?? runtime.channels[0];
  await page.goto("/schedule");
  await settle(page);

  await page.getByRole("button", { name: "Record a time" }).click();
  const draft = page.getByRole("form", { name: "Record a time" });
  await expect(draft.getByRole("status")).toHaveCount(0);
  await draft.getByLabel("Channel").selectOption({ label: `${ch.number} ${ch.name}` });
  const at = serverParts(Math.ceil((Date.now() + 3 * 60 * 60_000) / 300_000) * 300_000);
  await draft.getByLabel("Date").fill(at.date);
  await draft.getByLabel("Starts at").fill(at.clock);
  await draft.getByRole("group", { name: "Length" }).getByRole("button", { name: "90 min" }).click();
  await expect(draft.getByRole("status")).toContainText("Records 1 time in the next 2 weeks.");
  await draft.getByRole("button", { name: "Add timed recording" }).click();
  await expect(draft).toHaveCount(0);

  const saved = (await passes(page))[0];
  expect(saved).toMatchObject({ kind: "manual", channelId: ch.id, timeStart: at.clock, duration: 90, date: at.date });
  const row = page.locator(".pass-row").first();
  await expect(row).toContainText("Timed");
  await expect(row).toContainText(`${ch.number} ${ch.name}`);
  // Coming up lists it as a timed recording.
  await expect(page.locator(".sched-row").filter({ hasText: "Timed" }).first()).toBeVisible();

  // On days instead: it repeats, and the plan counts the times.
  await row.getByRole("button", { name: `Edit ${ch.number} ${ch.name}` }).click();
  const edit = page.getByRole("form", { name: `Edit Timed recording on ${ch.number} ${ch.name}` });
  await edit.getByRole("button", { name: "On days" }).click();
  await expect(edit.getByRole("group", { name: "Days" }).getByRole("button", { pressed: true })).toHaveCount(1);
  await expect(edit.getByRole("status")).toContainText(/Records [23] times in the next 2 weeks\./);
  await edit.getByLabel("Name").fill("Late look");
  await edit.getByRole("button", { name: "Save" }).click();
  await expect(edit).toHaveCount(0);
  const repeat = (await passes(page))[0];
  expect(repeat.days).toEqual([at.day]);
  expect(repeat.date).toBeUndefined();
  await expect(page.locator(".pass-row").first()).toContainText("Late look");

  // From a channel: the editor opens with it picked.
  await page.goto(`/schedule?record=${ch.id}`);
  await settle(page);
  const fromGuide = page.getByRole("form", { name: "Record a time" });
  await expect(fromGuide.getByLabel("Channel")).toHaveValue(String(ch.id));
  await fromGuide.getByRole("button", { name: "Cancel" }).click();
  await expect(page).toHaveURL(/\/schedule$/);

  await page.getByRole("button", { name: "Remove Late look" }).click();
  await page.getByRole("button", { name: "Remove Late look", exact: true }).last().click();
  await expect.poll(async () => (await passes(page)).length).toBe(0);
});
