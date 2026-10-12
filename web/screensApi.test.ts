import { afterEach, describe, expect, test, vi } from "vitest";
import { onUnauthorized } from "./src/lib/deviceToken.ts";
import { getScreens, sendToScreen, startedOn } from "./src/features/player/screens.ts";
import {
  approvePair,
  claimPair,
  createPairCode,
  listClients,
  pollPair,
  revokeClient,
  startPair,
} from "./src/features/settings/pairApi.ts";

const stops: Array<() => void> = [];
let endPoll = () => {};

afterEach(() => {
  endPoll();
  endPoll = () => {};
  for (const stop of stops) stop();
  stops.length = 0;
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string, init: RequestInit | undefined) => Response | Promise<Response>) {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    return Promise.resolve().then(() => handler(String(input), init));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function callAt(mock: { mock: { calls: unknown[][] } }, index = 0) {
  const call = mock.mock.calls[index];
  if (!call) throw new Error(`missing fetch call ${index}`);
  return { url: String(call[0]), init: call[1] as RequestInit | undefined };
}

function headerMap(init?: RequestInit): Record<string, string> {
  const headers = init?.headers;
  if (!headers || headers instanceof Headers || Array.isArray(headers)) throw new Error("expected a header record");
  return headers;
}

function jsonBody(init?: RequestInit): unknown {
  if (typeof init?.body !== "string") throw new Error("expected a JSON string body");
  return JSON.parse(init.body);
}

function methodOf(init?: RequestInit): string {
  return init?.method ?? "GET";
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

async function failed(promise: Promise<unknown>): Promise<Error & { status?: number; code?: string }> {
  try {
    await promise;
  } catch (err) {
    return err as Error & { status?: number; code?: string };
  }
  throw new Error("expected the call to fail");
}

function watchUnauthorized() {
  const opens: number[] = [];
  stops.push(
    onUnauthorized(() => {
      opens.push(opens.length + 1);
    }),
  );
  return opens;
}

const den = {
  id: "den-tv",
  name: "Den TV",
  kind: "tv",
  scopes: ["watch"],
  createdAt: "2026-10-11T12:00:00Z",
};

function screen(id: string, channelId: number, name = id === "news-4" ? "News 4" : "Den TV") {
  return { id, name, kind: "tv", channelId };
}

describe("sendToScreen", () => {
  for (const status of [202, 204]) {
    test(`posts the channel to the encoded screen and accepts an empty ${status}`, async () => {
      const fetchMock = stubFetch(() => new Response(null, { status }));
      await expect(sendToScreen("den/tv", 4, "Den TV")).resolves.toBeUndefined();
      const { url, init } = callAt(fetchMock);
      expect(url).toBe("/api/v1/screens/den%2Ftv/watch");
      expect(methodOf(init)).toBe("POST");
      expect(headerMap(init)["Content-Type"]).toBe("application/json");
      expect(headerMap(init).Authorization).toBeUndefined();
      expect(jsonBody(init)).toEqual({ channelId: 4, from: "Den TV" });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
  }

  test("sends the bearer token stored for this browser", async () => {
    localStorage.setItem("bw.deviceToken", "den-token");
    const fetchMock = stubFetch(() => new Response(null, { status: 202 }));
    await expect(sendToScreen("news-4", 4, "Den TV")).resolves.toBeUndefined();
    expect(headerMap(callAt(fetchMock).init).Authorization).toBe("Bearer den-token");
  });

  test("a JSON error keeps the message, status, and code", async () => {
    const opens = watchUnauthorized();
    stubFetch(() => jsonResponse({ message: "Den TV is off.", code: "asleep" }, 409));
    const err = await failed(sendToScreen("den/tv", 4, "Den TV"));
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("Den TV is off.");
    expect(err.status).toBe(409);
    expect(err.code).toBe("asleep");
    expect(opens).toEqual([]);
  });

  test("a non-JSON error uses the fallback line", async () => {
    stubFetch(() => new Response("nope", { status: 500 }));
    const err = await failed(sendToScreen("den/tv", 4, "Den TV"));
    expect(err.message).toBe("That did not work. Try again.");
    expect(err.status).toBe(500);
    expect(err.code).toBeUndefined();
  });

  test("a 401 tells the app to pair again and still throws", async () => {
    const opens = watchUnauthorized();
    stubFetch(() => jsonResponse({ message: "Sign in again.", code: "unauthorized" }, 401));
    const err = await failed(sendToScreen("den/tv", 4, "Den TV"));
    expect(opens).toEqual([1]);
    expect(err.message).toBe("Sign in again.");
    expect(err.status).toBe(401);
    expect(err.code).toBe("unauthorized");
  });
});

describe("getScreens", () => {
  test("gets the screen list and returns the screens", async () => {
    const screens = [screen("den-tv", 4), screen("news-4", 7)];
    const fetchMock = stubFetch(() => jsonResponse({ screens }));
    await expect(getScreens()).resolves.toEqual({ screens });
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/screens");
    expect(methodOf(init)).toBe("GET");
  });
});

describe("startedOn", () => {
  function openFlag(initial: boolean) {
    let open = initial;
    endPoll = () => {
      open = false;
    };
    return {
      isOpen: () => open,
      close: () => {
        open = false;
      },
    };
  }

  test("returns false immediately when the panel is closed", async () => {
    const fetchMock = stubFetch(() => {
      throw new Error("fetch should not run");
    });
    const panel = openFlag(false);
    await expect(startedOn("den-tv", 4, panel.isOpen)).resolves.toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  test("returns true when a poll sees that screen on the channel", async () => {
    vi.useFakeTimers();
    const panel = openFlag(true);
    const fetchMock = stubFetch(() => jsonResponse({ screens: [screen("den-tv", 4), screen("news-4", 7)] }));
    const pending = startedOn("den-tv", 4, panel.isOpen);
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1_000);
    await expect(pending).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(callAt(fetchMock).url).toBe("/api/v1/screens");
    expect(methodOf(callAt(fetchMock).init)).toBe("GET");
  });

  test("ignores the screen when it is on a different channel", async () => {
    vi.useFakeTimers();
    const panel = openFlag(true);
    const responses = [
      { screens: [screen("news-4", 4), screen("den-tv", 9)] },
      { screens: [screen("den-tv", 4)] },
    ];
    const fetchMock = stubFetch(() => jsonResponse(responses[fetchMock.mock.calls.length - 1] ?? responses[1]));
    let settled = false;
    const pending = startedOn("den-tv", 4, panel.isOpen).then((value) => {
      settled = true;
      return value;
    });
    await vi.advanceTimersByTimeAsync(1_000);
    expect(settled).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1_000);
    await expect(pending).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  test("swallows a failed poll and can succeed on the next one", async () => {
    vi.useFakeTimers();
    const panel = openFlag(true);
    let attempt = 0;
    const fetchMock = stubFetch(() => {
      attempt += 1;
      if (attempt === 1) return Promise.reject(new Error("Den TV dropped"));
      return jsonResponse({ screens: [screen("den-tv", 4)] });
    });
    const pending = startedOn("den-tv", 4, panel.isOpen);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1_000);
    await expect(pending).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  test("returns false once 12 seconds have passed", async () => {
    vi.useFakeTimers();
    const panel = openFlag(true);
    const fetchMock = stubFetch(() => jsonResponse({ screens: [screen("den-tv", 9)] }));
    let settled = false;
    const pending = startedOn("den-tv", 4, panel.isOpen).then((value) => {
      settled = true;
      return value;
    });
    for (let second = 1; second <= 11; second += 1) {
      await vi.advanceTimersByTimeAsync(1_000);
      expect(settled, `settled during second ${second}`).toBe(false);
      expect(fetchMock, `polls during second ${second}`).toHaveBeenCalledTimes(second);
    }
    await vi.advanceTimersByTimeAsync(1_000);
    await expect(pending).resolves.toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(12);
    await vi.advanceTimersByTimeAsync(3_000);
    expect(fetchMock).toHaveBeenCalledTimes(12);
  });

  test("stops as soon as the panel closes on a later loop", async () => {
    vi.useFakeTimers();
    const panel = openFlag(true);
    const fetchMock = stubFetch(() => {
      panel.close();
      return jsonResponse({ screens: [screen("den-tv", 9)] });
    });
    const pending = startedOn("den-tv", 4, panel.isOpen);
    await vi.advanceTimersByTimeAsync(1_000);
    await expect(pending).resolves.toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("pair API", () => {
  test("pollPair encodes an id and secret that contain spaces and &", async () => {
    const id = "den tv&room";
    const secret = "s & x";
    const fetchMock = stubFetch(() => jsonResponse({ state: "waiting" }));
    await expect(pollPair(id, secret)).resolves.toEqual({ state: "waiting" });
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/pair/den%20tv%26room?secret=s%20%26%20x");
    expect(methodOf(init)).toBe("GET");
  });

  test("revokeClient encodes an id with a slash", async () => {
    const fetchMock = stubFetch(() => jsonResponse({ devices: [] }));
    await expect(revokeClient("den/tv")).resolves.toEqual({ devices: [] });
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/clients/den%2Ftv");
    expect(methodOf(init)).toBe("DELETE");
  });

  test("startPair posts the JSON body to /api/v1/pair", async () => {
    const body = { name: "Den TV", kind: "tv", scopes: ["watch"] };
    const ticket = { id: "pair-1", code: "482913", expiresAt: "2026-10-11T12:05:00Z", pollSecret: "den-secret" };
    const fetchMock = stubFetch(() => jsonResponse(ticket));
    await expect(startPair(body)).resolves.toEqual(ticket);
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/pair");
    expect(methodOf(init)).toBe("POST");
    expect(jsonBody(init)).toEqual(body);
  });

  test("createPairCode posts to /api/v1/pair/code", async () => {
    const body = { kind: "tv", scopes: ["watch", "record"] };
    const ticket = { id: "pair-2", code: "193847", expiresAt: "2026-10-11T12:05:00Z" };
    const fetchMock = stubFetch(() => jsonResponse(ticket));
    await expect(createPairCode(body)).resolves.toEqual(ticket);
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/pair/code");
    expect(methodOf(init)).toBe("POST");
    expect(jsonBody(init)).toEqual(body);
  });

  test("approvePair and claimPair hit their paths", async () => {
    const fetchMock = stubFetch((url) => {
      if (url === "/api/v1/pair/approve") return jsonResponse({ device: den });
      return jsonResponse({ token: "den-token", device: den });
    });
    const approveBody = { code: "482913", scopes: ["watch"] };
    const claimBody = { code: "482913", name: "Den TV", kind: "tv" };
    await expect(approvePair(approveBody)).resolves.toEqual({ device: den });
    await expect(claimPair(claimBody)).resolves.toEqual({ token: "den-token", device: den });
    const approve = callAt(fetchMock, 0);
    const claim = callAt(fetchMock, 1);
    expect(approve.url).toBe("/api/v1/pair/approve");
    expect(methodOf(approve.init)).toBe("POST");
    expect(jsonBody(approve.init)).toEqual(approveBody);
    expect(claim.url).toBe("/api/v1/pair/claim");
    expect(methodOf(claim.init)).toBe("POST");
    expect(jsonBody(claim.init)).toEqual(claimBody);
  });

  test("listClients gets /api/v1/clients", async () => {
    const fetchMock = stubFetch(() => jsonResponse({ devices: [den] }));
    await expect(listClients()).resolves.toEqual({ devices: [den] });
    const { url, init } = callAt(fetchMock);
    expect(url).toBe("/api/v1/clients");
    expect(methodOf(init)).toBe("GET");
  });
});
