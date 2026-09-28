// The installed app shell. The document is read from the network first,
// so a rebuild's new bundle name shows up on reload. Live video is not cached.
import { copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, "../..");
const built = path.join(repo, "server/cmd/broadwave/assets/web");
const evidence = path.resolve(here, "../../.evidence/lane/l58");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { control: string };
}

async function control(action: "stop" | "start") {
  const res = await fetch(`${harness().control}/${action}`, { method: "POST" });
  if (!res.ok) throw new Error(`${action} ${res.status} ${await res.text()}`);
}

function goBuild() {
  const result = spawnSync("go", ["build", "-o", path.join(here, ".run/broadwave"), "./server/cmd/broadwave"], {
    cwd: repo,
    encoding: "utf8",
  });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || "go build failed");
}

async function moduleSrc(page: Page) {
  const srcs = await page.locator('script[type="module"]').evaluateAll((els) => els.map((el) => el.getAttribute("src") || ""));
  const src = srcs.find((item) => item.includes("/assets/index-"));
  if (!src) throw new Error(`no bundle script in ${srcs.join(", ")}`);
  return src;
}

async function iconSize(page: Page, src: string) {
  return page.evaluate(async (url) => {
    const res = await fetch(url);
    const blob = await res.blob();
    const bitmap = await createImageBitmap(blob);
    return { status: res.status, type: blob.type, width: bitmap.width, height: bitmap.height };
  }, src);
}

test("the manifest installs and a channel plays with the worker on", async ({ page }) => {
  await page.goto("/");
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller !== null)).toBe(true);
  await page.reload();
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller?.scriptURL ?? "")).toContain("/sw.js");

  const shell = await page.request.get("/");
  expect(shell.headers()["cache-control"]).toContain("no-cache");
  const manifestRes = await page.request.get("/manifest.webmanifest");
  expect(manifestRes.status()).toBe(200);
  expect(manifestRes.headers()["content-type"]).toContain("application/manifest+json");
  const manifest = await manifestRes.json();
  expect(manifest.name).toBe("Broadwave");
  expect(manifest.display).toBe("standalone");
  expect(manifest.background_color).toBe("#070A10");
  expect(manifest.theme_color).toBe("#070A10");
  expect(manifest.start_url).toBe("/");
  const icons = manifest.icons as { src: string; sizes: string; purpose: string }[];
  expect(icons).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ src: "/icons/icon-192.png", sizes: "192x192", purpose: "any" }),
      expect.objectContaining({ src: "/icons/icon-512.png", sizes: "512x512", purpose: "any" }),
      expect.objectContaining({ src: "/icons/maskable-512.png", sizes: "512x512", purpose: "maskable" }),
    ]),
  );

  const icon192 = await iconSize(page, "/icons/icon-192.png");
  const icon512 = await iconSize(page, "/icons/icon-512.png");
  const maskable = await iconSize(page, "/icons/maskable-512.png");
  expect(icon192).toMatchObject({ status: 200, type: "image/png", width: 192, height: 192 });
  expect(icon512).toMatchObject({ status: 200, type: "image/png", width: 512, height: 512 });
  expect(maskable).toMatchObject({ status: 200, type: "image/png", width: 512, height: 512 });

  await page.getByRole("button", { name: "Watch", exact: true }).click();
  const video = page.locator("video.stage-video");
  await expect.poll(async () => video.evaluate((el: HTMLVideoElement) => (el.currentTime > 0.2 && !el.paused ? el.currentTime : 0)), { timeout: 30_000 }).toBeGreaterThan(0.2);
  const started = await video.evaluate((el: HTMLVideoElement) => el.currentTime);
  await expect.poll(async () => video.evaluate((el: HTMLVideoElement) => el.currentTime), { timeout: 15_000 }).toBeGreaterThan(started + 0.3);
  expect(await page.evaluate(() => navigator.serviceWorker.controller !== null)).toBe(true);

  const cached = await page.evaluate(async () => {
    const names = await caches.keys();
    const urls: string[] = [];
    for (const name of names) {
      const cache = await caches.open(name);
      for (const req of await cache.keys()) urls.push(new URL(req.url).pathname);
    }
    return urls;
  });
  expect(cached.some((url) => url.startsWith("/assets/"))).toBe(true);
  for (const url of cached) {
    expect(url.startsWith("/api/"), url).toBe(false);
    expect(url.startsWith("/media/"), url).toBe(false);
    expect(url.includes("/frame"), url).toBe(false);
    expect(/\.m3u8$|\.m4s$|\.ts$/.test(url), url).toBe(false);
  }
});

test("a reload after a rebuild uses the new bundle", async ({ page }) => {
  test.setTimeout(240_000);
  await page.goto("/");
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller !== null)).toBe(true);
  await page.reload();
  const before = await moduleSrc(page);
  const file = path.posix.basename(before);
  const next = file.replace(/\.js$/, "pwa.js");
  expect(next).not.toBe(file);

  const indexPath = path.join(built, "index.html");
  const saved = {
    html: readFileSync(indexPath),
    gz: existsSync(indexPath + ".gz") ? readFileSync(indexPath + ".gz") : null,
    br: existsSync(indexPath + ".br") ? readFileSync(indexPath + ".br") : null,
  };
  const copies = [path.join(built, "assets", next)];
  for (const ext of [".gz", ".br"]) {
    if (existsSync(path.join(built, "assets", file + ext))) copies.push(path.join(built, "assets", next + ext));
  }

  const restore = async () => {
    writeFileSync(indexPath, saved.html);
    if (saved.gz) writeFileSync(indexPath + ".gz", saved.gz);
    if (saved.br) writeFileSync(indexPath + ".br", saved.br);
    for (const copy of copies) rmSync(copy, { force: true });
    await control("stop");
    goBuild();
    await control("start");
  };

  try {
    await control("stop");
    copyFileSync(path.join(built, "assets", file), path.join(built, "assets", next));
    for (const ext of [".gz", ".br"]) {
      const from = path.join(built, "assets", file + ext);
      if (existsSync(from)) copyFileSync(from, path.join(built, "assets", next + ext));
    }
    const html = saved.html.toString("utf8").split(file).join(next);
    expect(html).toContain(next);
    writeFileSync(indexPath, html);
    writeFileSync(indexPath + ".gz", gzipSync(html, { level: 9 }));
    writeFileSync(indexPath + ".br", brotliCompressSync(Buffer.from(html), { params: { [constants.BROTLI_PARAM_QUALITY]: 6 } }));
    goBuild();
    await control("start");

    await page.reload();
    await expect.poll(() => moduleSrc(page)).toContain(next);
    await expect(page.getByRole("button", { name: "Watch", exact: true })).toBeVisible();
    await expect.poll(async () => page.evaluate(async (name) => {
      const cache = await caches.open("broadwave-shell");
      const hit = await cache.match("/index.html");
      return hit ? await hit.text() : "";
    }, next)).toContain(next);
    await expect.poll(async () => page.evaluate(async () => {
      const cache = await caches.open("broadwave-shell");
      return (await cache.keys()).map((req) => new URL(req.url).pathname);
    })).not.toContain(before);

    mkdirSync(evidence, { recursive: true });
    writeFileSync(path.join(evidence, "reload.json"), JSON.stringify({ before, after: `/${path.posix.join("assets", next)}` }, null, 2));
  } finally {
    await restore();
  }
});
