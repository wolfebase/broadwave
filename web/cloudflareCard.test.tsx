import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { CfStatus } from "./src/features/settings/cloudflareFlow.ts";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const api = vi.hoisted(() => ({
  get: vi.fn<() => Promise<unknown>>(),
  login: vi.fn<(purpose?: string) => Promise<unknown>>(),
  cancel: vi.fn<() => Promise<unknown>>(),
  setup: vi.fn<(body: { label: string; overwrite?: boolean }) => Promise<unknown>>(),
  off: vi.fn<() => Promise<unknown>>(),
  handlers: new Map<string, () => void>(),
}));

vi.mock("./src/features/settings/cloudflareApi.ts", () => ({
  getCloudflare: api.get,
  startCloudflareLogin: api.login,
  cancelCloudflareLogin: api.cancel,
  setupCloudflare: api.setup,
  turnOffCloudflare: api.off,
}));

vi.mock("./src/lib/events.ts", () => ({
  events: () => ({
    on(type: string, fn: () => void) {
      api.handlers.set(type, fn);
      return () => api.handlers.delete(type);
    },
  }),
}));

const { CloudflareCard } = await import("./src/features/settings/CloudflareCard.tsx");

const tunnel = { state: "off", connections: 0 };

function st(extra: Partial<CfStatus>): CfStatus {
  return { state: "idle", managed: false, tunnel, ...extra };
}

let root: Root | null = null;
let host: HTMLDivElement;

async function mount() {
  host = document.createElement("div");
  document.body.appendChild(host);
  const next = createRoot(host);
  root = next;
  await act(async () => {
    next.render(<CloudflareCard remote={null} onRemote={() => undefined} />);
  });
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
  });
}

function button(name: string): HTMLButtonElement {
  const found = [...host.querySelectorAll("button")].find((b) => b.textContent?.includes(name));
  if (!found) throw new Error(`no button ${name} in ${host.textContent}`);
  return found;
}

beforeEach(() => {
  api.get.mockReset();
  api.login.mockReset();
  api.cancel.mockReset();
  api.setup.mockReset();
  api.off.mockReset();
  api.handlers.clear();
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  host.remove();
  vi.restoreAllMocks();
});

test("sign in opens Cloudflare in the tab it opened inside the click", async () => {
  api.get.mockResolvedValue(st({}));
  await mount();
  expect(host.textContent).toContain("Watch from anywhere");
  // Opening Settings does not pull focus down to this card.
  expect(document.activeElement).toBe(document.body);
  const tab = { opener: {} as unknown, location: { href: "" }, close: vi.fn() };
  const open = vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window);
  const waiting = st({ state: "waiting", purpose: "setup", loginUrl: "https://dash.cloudflare.com/argotunnel?aud=&callback=x", expiresAt: new Date(Date.now() + 540_000).toISOString() });
  api.login.mockResolvedValue(waiting);
  await act(async () => {
    button("Sign in with Cloudflare").click();
  });
  await flush();
  expect(open).toHaveBeenCalledWith("", "_blank");
  expect(api.login).toHaveBeenCalledWith("setup");
  expect(tab.location.href).toBe(waiting.loginUrl);
  expect(tab.opener).toBeNull();
  expect(host.textContent).toContain("Waiting for Cloudflare…");
  expect(host.textContent).toMatch(/[89]:\d\d left/);
  expect(host.querySelector<HTMLAnchorElement>("a.btn")?.href).toBe(waiting.loginUrl);
});

test("a blocked tab still leaves a way to open Cloudflare", async () => {
  api.get.mockResolvedValue(st({}));
  await mount();
  vi.spyOn(window, "open").mockReturnValue(null);
  api.login.mockResolvedValue(st({ state: "waiting", purpose: "setup", loginUrl: "https://dash.cloudflare.com/x", expiresAt: new Date(Date.now() + 60_000).toISOString() }));
  await act(async () => {
    button("Sign in with Cloudflare").click();
  });
  await flush();
  expect(host.textContent).toContain("The browser kept the tab from opening");
  expect(host.querySelector<HTMLAnchorElement>("a.btn")?.textContent).toContain("Open Cloudflare");
});

test("a socket event moves the card to signed in, and Turn on sends the name", async () => {
  api.get.mockResolvedValue(st({ state: "waiting", purpose: "setup", loginUrl: "https://x", expiresAt: new Date(Date.now() + 60_000).toISOString() }));
  await mount();
  api.get.mockResolvedValue(st({ state: "authorized", purpose: "setup", zone: { id: "z", name: "example.com" } }));
  await act(async () => {
    api.handlers.get("remote.changed")?.();
  });
  await flush();
  expect(host.textContent).toContain("Signed in · example.com");
  const input = host.querySelector("input")!;
  expect(document.activeElement).toBe(input);
  expect(host.textContent).toContain(".example.com");
  expect(host.textContent).toContain("Family will open https://tv.example.com");
  api.setup.mockResolvedValue(st({ state: "provisioning", step: "tunnel", hostname: "tv.example.com", zone: { id: "z", name: "example.com" } }));
  await act(async () => {
    button("Turn on").click();
  });
  await flush();
  expect(api.setup).toHaveBeenCalledWith({ label: "tv", overwrite: false });
  expect(host.textContent).toContain("Setting up tv.example.com");
  const steps = [...host.querySelectorAll(".cf-steps li")].map((li) => li.getAttribute("data-mark"));
  expect(steps).toEqual(["now", "next", "next"]);
});

test("a taken name offers to replace it", async () => {
  api.get.mockResolvedValue(st({ state: "authorized", purpose: "setup", zone: { id: "z", name: "example.com" }, hostname: "tv.example.com", error: { code: "name_taken", message: "taken" } }));
  await mount();
  expect(host.textContent).toContain("tv.example.com is already in use.");
  api.setup.mockResolvedValue(st({ state: "provisioning", step: "tunnel", hostname: "tv.example.com" }));
  await act(async () => {
    button("Replace it").click();
  });
  expect(api.setup).toHaveBeenCalledWith({ label: "tv", overwrite: true });
});

test("a domain the sign-in cannot read asks for the whole name", async () => {
  api.get.mockResolvedValue(st({ state: "authorized", purpose: "setup", zone: { id: "z" } }));
  await mount();
  expect(host.textContent).toContain("Type the whole name");
  expect(button("Turn on").disabled).toBe(true);
});

test("connected shows the address, and Turn off asks first", async () => {
  api.get.mockResolvedValue(st({ state: "connected", managed: true, hostname: "tv.example.com", tunnel: { state: "connected" } }));
  await mount();
  expect(host.querySelector(".cf-url")?.textContent).toBe("https://tv.example.com");
  expect(host.querySelector(".cf-live")?.getAttribute("data-state")).toBe("connected");
  await act(async () => {
    button("Turn off").click();
  });
  expect(host.textContent).toContain("Turn off away from home?");
  api.off.mockResolvedValue(st({ state: "idle", dnsLeft: "tv.example.com" }));
  await act(async () => {
    [...host.querySelectorAll(".cf-confirm button")][0]!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await flush();
  expect(api.off).toHaveBeenCalled();
  expect(host.textContent).toContain("tv.example.com is still in your Cloudflare DNS");
});

test("no domain names the way out, and outside the house says to go home", async () => {
  api.get.mockResolvedValue(st({ state: "error", error: { code: "no_zone", message: "server words" } }));
  await mount();
  expect(host.textContent).toContain("That Cloudflare account has no domain yet.");
  expect(host.textContent).toContain("Add a domain to Cloudflare");
  expect(host.textContent).toContain("Use Tailscale");
  act(() => root?.unmount());
  host.remove();
  const failure = Object.assign(new Error("Set this up from the home network."), { status: 403, code: "not_home" });
  api.get.mockRejectedValue(failure);
  await mount();
  await flush();
  expect(host.textContent).toContain("Set this up from the home network.");
});
