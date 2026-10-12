import { expect, test } from "./fixture";
import { settle } from "./snap";

const namespace = "urn:x-cast:app.broadwave.watch";

test("Cast stays hidden until the server has an application id", async ({ page }) => {
  const google: string[] = [];
  page.on("request", (req) => {
    if (new URL(req.url()).hostname.endsWith("gstatic.com")) google.push(req.url());
  });
  await page.goto("/");
  await settle(page);
  const watch = page.waitForResponse((res) => res.url().includes("/api/v1/watch") && res.request().method() === "POST" && res.status() === 200);
  const media = page.waitForRequest((req) => req.url().includes("/media/live/"));
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await watch;
  await media;
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Cast" })).toHaveCount(0);
  expect(google).toEqual([]);
});

test("Cast tells the television which channel to play", async ({ page }) => {
  await page.route("**/api/v1/server", async (route) => {
    const res = await route.fetch();
    const body = (await res.json()) as { castAppId?: string };
    body.castAppId = "4F8C2A1B";
    await route.fulfill({ status: res.status(), contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.addInitScript(() => {
    const host = window as Window & {
      __castSends?: { namespace: string; data: unknown }[];
      __castEnded?: boolean;
      __castOptions?: { receiverApplicationId?: string };
      chrome?: { cast?: { AutoJoinPolicy?: { ORIGIN_SCOPED?: string } } };
      cast?: unknown;
    };
    host.__castSends = [];
    host.__castEnded = false;
    host.chrome = host.chrome ?? {};
    host.chrome.cast = { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } };
    let asked = false;
    const session = {
      sendMessage(namespace: string, data: unknown) {
        host.__castSends?.push({ namespace, data });
        return Promise.resolve(null);
      },
      endSession(stop: boolean) {
        host.__castEnded = stop;
      },
    };
    host.cast = {
      framework: {
        CastContext: {
          getInstance: () => ({
            setOptions(next: { receiverApplicationId?: string }) {
              host.__castOptions = next;
            },
            requestSession: () => {
              asked = true;
              return Promise.resolve(null);
            },
            getCurrentSession: () => (asked ? session : null),
          }),
        },
      },
    };
  });
  await page.route("https://www.gstatic.com/**", (route) => route.abort());
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("button", { name: "Cast", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Cast", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __castSends?: { namespace: string; data: { type?: string; channelId?: number; playlist?: string; room?: string; latency?: string } }[] }).__castSends ?? []))
    .toHaveLength(1);
  const sent = await page.evaluate(() => (window as unknown as { __castSends: { namespace: string; data: { type?: string; channelId?: number; playlist?: string; room?: string; latency?: string } }[]; __castOptions?: { receiverApplicationId?: string } }).__castSends[0]);
  const options = await page.evaluate(() => (window as unknown as { __castOptions?: { receiverApplicationId?: string } }).__castOptions);
  expect(sent.namespace).toBe(namespace);
  expect(sent.data.type).toBe("load");
  expect(sent.data.channelId).toBeGreaterThan(0);
  expect(sent.data.playlist?.startsWith("/media/")).toBe(true);
  expect(sent.data.playlist).not.toContain("access_token");
  expect(sent.data.room).toBe(`channel:${sent.data.channelId}`);
  expect(["lowest", "balanced", "stable"]).toContain(sent.data.latency);
  expect(options?.receiverApplicationId).toBe("4F8C2A1B");
  await page.getByRole("button", { name: "Stop casting", exact: true }).click();
  await expect.poll(() => page.evaluate(() => (window as unknown as { __castEnded?: boolean }).__castEnded)).toBe(true);
  const stop = await page.evaluate(() => (window as unknown as { __castSends: { namespace: string; data: { type?: string } }[] }).__castSends.at(-1));
  expect(stop?.namespace).toBe(namespace);
  expect(stop?.data.type).toBe("stop");
});

test("the receiver joins the channel room", async ({ page }) => {
  const frames: string[] = [];
  page.on("websocket", (socket) => {
    socket.on("framesent", (frame) => {
      if (typeof frame.payload === "string") frames.push(frame.payload);
    });
  });
  await page.addInitScript(() => {
    const host = window as Window & { cast?: unknown };
    class CastReceiverOptions {}
    const context = {
      start() {},
      addCustomMessageListener(_namespace: string, listener: (event: { data: unknown }) => void) {
        queueMicrotask(() => {
          listener({
            data: {
              type: "load",
              channelId: 1,
              playlist: "/media/live/1/index.m3u8",
              room: "channel:1",
              latency: "balanced",
              title: "Evening news",
            },
          });
        });
      },
    };
    host.cast = {
      framework: {
        CastReceiverContext: { getInstance: () => context },
        CastReceiverOptions,
        system: { MessageType: { JSON: "JSON" } },
      },
    };
  });
  await page.route("https://www.gstatic.com/**", (route) => route.abort());
  await page.goto("/cast");
  await expect.poll(() => frames.some((frame) => frame.includes('"sync.join"'))).toBe(true);
  const join = frames.map((frame) => JSON.parse(frame) as { type?: string; data?: { room?: string; channelId?: number } }).find((message) => message.type === "sync.join");
  expect(join?.data?.room).toBe("channel:1");
  expect(join?.data?.channelId).toBe(1);
  expect(frames.some((frame) => frame.includes('"here"'))).toBe(true);
  expect(frames.some((frame) => frame.includes('"clock"'))).toBe(true);
  await expect(page.locator("#cast-idle")).toBeHidden();
  await expect(page.locator("#cast-title")).toHaveText("Evening news");
  await expect.poll(() => page.locator("html").getAttribute("data-cast-state")).toBe("buffering");
});
