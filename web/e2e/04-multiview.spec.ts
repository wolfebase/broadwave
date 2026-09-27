import { expect, holdClock, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

test("multiview adds a channel and swaps the one with sound", async ({ page }, info) => {
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await page.getByRole("region", { name: "Player" }).press("m");

  await expect(page).toHaveURL(/\/multiview/);
  await expect(page.getByRole("listbox", { name: "Add a channel" })).toBeVisible();
  await page.getByRole("option", { name: /5\.1\s*KCTV/ }).click();
  await expect(page.getByRole("group", { name: "5.1 KCTV, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "4.1 WDAF", exact: true })).toBeVisible();

  await page.getByRole("group", { name: "4.1 WDAF", exact: true }).click();
  await expect(page.getByRole("group", { name: "4.1 WDAF, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "5.1 KCTV", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Quad", exact: true }).click();
  await expect(page.getByRole("region", { name: "Quad" })).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `multiview-${size.name}`, info);
  }
});

test("moving the sound between equal tiles restarts neither", async ({ page }) => {
  await page.goto("/watch?channel=1");
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await expect(setup.or(player)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(player).toBeVisible();
  }
  await page.goto("/multiview?ch=1,3&layout=2up&focus=1");
  const playing = () =>
    page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) =>
      videos.map((video) => ({ channel: video.dataset.channel, muted: video.muted, moving: video.videoWidth > 0 && !video.paused && video.currentTime > 0.5 })),
    );
  await expect.poll(async () => (await playing()).filter((tile) => tile.moving).length, { timeout: 45_000 }).toBe(2);

  const watches: number[] = [];
  page.on("request", (req) => {
    if (req.method() === "POST" && req.url().endsWith("/api/v1/watch")) watches.push((JSON.parse(req.postData() || "{}") as { channelId: number }).channelId);
  });
  await page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) => {
    const w = window as unknown as { __emptied: number };
    w.__emptied = 0;
    for (const video of videos) video.addEventListener("emptied", () => w.__emptied++);
  });

  for (const name of ["5.1 KCTV", "4.1 WDAF", "5.1 KCTV"]) {
    await page.getByRole("group", { name, exact: true }).click();
    await expect(page.getByRole("group", { name: `${name}, sound on` })).toBeVisible();
    await page.waitForTimeout(1500);
  }
  expect(watches, "a sound swap started a new watch").toEqual([]);
  expect(await page.evaluate(() => (window as unknown as { __emptied: number }).__emptied), "a sound swap emptied a tile").toBe(0);
  const tiles = await playing();
  expect(tiles.filter((tile) => tile.moving)).toHaveLength(2);
  expect(tiles.find((tile) => tile.channel === "3")?.muted).toBe(false);
  expect(tiles.find((tile) => tile.channel === "1")?.muted).toBe(true);
});
