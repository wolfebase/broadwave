import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { chromium, type Page } from "@playwright/test";
import { createServer, type ViteDevServer } from "vite";
import { appPages } from "./src/i18n/pages.ts";

const here = path.dirname(fileURLToPath(import.meta.url));
const fixtures = path.resolve(here, "../api/fixtures");
const lanePorts = [19140, 19141, 19142, 19143];

const files: Record<string, string> = {
  "/api/v1/settings": "settings.json",
  "/api/v1/server": "server.json",
  "/api/v1/channels": "channels.json",
  "/api/v1/airings": "airings.json",
  "/api/v1/recordings": "recordings.json",
  "/api/v1/passes": "passes.json",
  "/api/v1/schedule": "schedule.json",
  "/api/v1/virtuals": "virtuals.json",
  "/api/v1/devices": "devices.json",
  "/api/v1/storage": "storage.json",
  "/api/v1/events": "events.json",
  "/api/v1/sources": "sources.json",
  "/api/v1/tuners": "tuners.json",
  "/api/v1/signals": "signals.json",
  "/api/v1/groups": "groups.json",
  "/api/v1/diagnostics": "diagnostics.json",
  "/api/v1/health": "health.json",
  "/api/v1/backups": "backup-restore.json",
};

function bodyFor(pathname: string): string {
  if (pathname === "/api/v1/teams" || pathname.startsWith("/api/v1/teams/")) return '{"teams":[]}';
  if (pathname === "/api/v1/search") return '{"airings":[],"recordings":[],"channels":[]}';
  if (pathname === "/api/v1/clients/me" || pathname === "/api/v1/client/me") return '{"auth":"","device":null}';
  if (pathname === "/api/v1/devices/health") return '{"devices":[]}';
  if (pathname === "/api/v1/sources/discover") return fs.readFileSync(path.join(fixtures, "devices.json"), "utf8");
  if (pathname === "/api/v1/sources/look") return '{"found":[]}';
  if (pathname === "/api/v1/sources/free") return '{"found":[],"guide":""}';
  if (pathname === "/api/v1/setup/finish") return '{"running":false,"ready":"1","steps":[]}';
  if (pathname === "/api/v1/frames") return '{"frames":[]}';
  if (pathname === "/api/v1/playback") return '{"pictureMode":"broadcast","autoplay":"1","layout":"auto"}';
  // A play or plan body of {} crashes the page after it is ready, so the overflow check never sees it.
  if (/^\/api\/v1\/recordings\/\d+\/play$/.test(pathname)) return '{"markers":[],"growing":false,"position":0,"playlist":""}';
  if (pathname === "/api/v1/multiview/plan") return '{"blocked":[],"offers":[],"stops":[]}';
  const name = files[pathname];
  if (!name) return "{}";
  return fs.readFileSync(path.join(fixtures, name), "utf8");
}

async function measure(page: Page) {
  return page.evaluate(() => {
    const root = document.documentElement;
    const overflowX = root.scrollWidth - root.clientWidth;
    const skip = /nc-title|nc-name|nc-num|cell-title|hero-title|hero-sub|sc-title|sc-sub|ch-name|guide-name|program-title|rc-title|rc-sub/;
    const clipped: { cls: string; text: string }[] = [];
    for (const el of document.querySelectorAll<HTMLElement>("h1, h2, h3, button, a, label, .tab-label, .hint, p, legend")) {
      const style = getComputedStyle(el);
      const clips = style.textOverflow === "ellipsis" || (style.overflowX === "hidden" && style.whiteSpace === "nowrap");
      if (!clips || el.scrollWidth <= el.clientWidth + 1) continue;
      // A 1px clip is how screen-reader-only text is hidden. It is not a layout overflow.
      if (el.clientWidth <= 2 || el.closest(".sr-only")) continue;
      const cls = String(el.className);
      if (skip.test(cls) || el.closest(".guide-canvas, .now-card, .sport-card, .rec-card")) continue;
      const text = (el.textContent || "").replace(/\s+/g, " ").trim();
      if (text) clipped.push({ cls: cls.slice(0, 80), text: text.slice(0, 80) });
    }
    return {
      overflowX,
      locale: root.dataset.locale || "",
      clipped: clipped.slice(0, 8),
      ready: root.querySelector("[data-ready]")?.getAttribute("data-ready") || "",
    };
  });
}

async function openLane(): Promise<{ vite: ViteDevServer; port: number }> {
  let last: unknown;
  for (const candidate of lanePorts) {
    const vite = await createServer({
      configFile: path.join(here, "vite.config.ts"),
      server: { host: "127.0.0.1", port: candidate, strictPort: true, hmr: false },
      logLevel: "error",
    });
    try {
      await vite.listen();
      return { vite, port: candidate };
    } catch (err) {
      await vite.close();
      last = err;
      const message = err instanceof Error ? err.message : String(err);
      if (!message.includes("already in use")) throw err;
    }
  }
  throw last;
}

test("every page renders in the pseudo locale without overflow", { timeout: 180_000 }, async () => {
  const { vite, port } = await openLane();
  let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
  const problems: string[] = [];
  try {
    browser = await chromium.launch({ channel: "chrome", headless: true });
    const page = await browser.newPage();
    const crashes: string[] = [];
    page.on("pageerror", (err) => crashes.push(err.message));
    await page.route("**/api/**", (route) => {
      const url = new URL(route.request().url());
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: bodyFor(url.pathname),
      });
    });
    await page.route("**/media/**", (route) => route.fulfill({ status: 200, contentType: "image/gif", body: Buffer.from("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7", "base64") }));
    const layouts = [
      { name: "desktop", width: 1280, height: 800, query: "layout=desktop" },
      { name: "phone", width: 390, height: 844, query: "layout=phone" },
      { name: "tv", width: 1440, height: 810, query: "layout=tv" },
    ];
    for (const layout of layouts) {
      await page.setViewportSize({ width: layout.width, height: layout.height });
      for (const item of appPages) {
        const join = item.path.includes("?") ? "&" : "?";
        const url = `http://127.0.0.1:${port}${item.path}${join}locale=pseudo&${layout.query}`;
        const seen = crashes.length;
        await page.goto(url, { waitUntil: "domcontentloaded", timeout: 15_000 });
        try {
          await page.waitForSelector("[data-ready='1']", { timeout: 20_000, state: "attached" });
        } catch (err) {
          const text = await page.locator("body").innerText().catch(() => "");
          problems.push(`${item.name} ${layout.name}: did not become ready (${crashes.at(-1) || (err instanceof Error ? err.message : "timeout")}) ${text.slice(0, 160)}`);
          continue;
        }
        // English already fills the TV top bar, so its tabs need a design before a longer
        // language ships. Wrapping them here keeps the rest of each page under test.
        await page.addStyleTag({ content: ".tabs { flex-wrap: wrap; min-width: 0; }" });
        // Play and multiview fetch after ready. Give that render a moment to throw.
        await page.waitForTimeout(150);
        const box = await measure(page);
        const where = `${item.name} ${layout.name}`;
        if (crashes.length > seen) problems.push(`${where}: ${crashes.slice(seen).join(" | ")}`);
        if (box.locale !== "pseudo") problems.push(`${where}: locale is ${box.locale || "unset"}`);
        if (box.overflowX > 1) problems.push(`${where}: page is ${box.overflowX}px wider than the window`);
        for (const clip of box.clipped) problems.push(`${where}: clipped "${clip.text}" (${clip.cls})`);
        const immersive = ["/play", "/multiview", "/setup"].includes(item.path.split("?")[0]) || item.path.includes("virtual=");
        if (!immersive) {
          const home = page.getByRole("link", { name: "HomeHome", exact: true });
          if ((await home.count()) !== 1) problems.push(`${where}: the Home tab is not doubled${crashes.length ? ` (${crashes.at(-1)})` : ""}`);
        }
      }
    }
  } finally {
    await browser?.close();
    await vite.close();
  }
  assert.deepEqual(problems, []);
});
