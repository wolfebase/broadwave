import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const SCOREBOARD = "/api/v1/sports/scoreboard";
const noon = Date.parse("2026-10-11T18:00:00.000Z");

type Scores = typeof import("./src/features/sports/scores.ts");

type Side = {
  name: string;
  abbr?: string;
  score?: string;
  home?: boolean;
};

type Game = {
  id: string;
  state?: string;
  teams?: Side[];
};

type Held = {
  input: RequestInfo | URL;
  init?: RequestInit;
  resolve: (response: Response) => void;
  reject: (reason: unknown) => void;
};

const held: Held[] = [];
let root: Root | null = null;

const kept: Game = {
  id: "city-kept",
  state: "in",
  teams: [
    { name: "City News", abbr: "CNO", score: "2" },
    { name: "Den TV", abbr: "DEN", home: true, score: "1" },
  ],
};

const earlyGame: Game = {
  id: "city-early",
  state: "in",
  teams: [
    { name: "City News", abbr: "CNO", score: "1" },
    { name: "Den TV", abbr: "DEN", home: true, score: "0" },
  ],
};

const lateGame: Game = {
  id: "city-late",
  state: "post",
  teams: [
    { name: "City News", abbr: "CNO", score: "3" },
    { name: "Den TV", abbr: "DEN", home: true, score: "7" },
  ],
};

// Home is listed first on purpose: the line still names the visitors first.
const liveTie: Game = {
  id: "city-den",
  state: "in",
  teams: [
    { name: "Den TV", abbr: "DEN", home: true, score: "0" },
    { name: "City News", abbr: "CNO", score: "0" },
  ],
};

const mixed: Game[] = [
  {
    id: "not-started",
    state: "pre",
    teams: [
      { name: "City News", abbr: "CNO", score: "0" },
      { name: "Den TV", abbr: "DEN", home: true, score: "0" },
    ],
  },
  liveTie,
  {
    id: "one-side",
    state: "in",
    teams: [{ name: "Den TV", abbr: "DEN", home: true, score: "4" }],
  },
  {
    id: "",
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO", score: "1" },
      { name: "Den TV", abbr: "DEN", home: true, score: "2" },
    ],
  },
  {
    id: "no-away-score",
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO" },
      { name: "Den TV", abbr: "DEN", home: true, score: "4" },
    ],
  },
  {
    id: "blank-score",
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO", score: "" },
      { name: "Den TV", abbr: "DEN", home: true, score: "0" },
    ],
  },
  {
    id: "no-home",
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO", score: "1" },
      { name: "Den TV", abbr: "DEN", score: "2" },
    ],
  },
  {
    id: "city-final",
    state: "post",
    teams: [
      { name: "City News", abbr: "CNO", score: "3" },
      { name: "Den TV", abbr: "DEN", home: true, score: "7" },
    ],
  },
];

const mixedIds = "not-started,city-den,one-side,(none),no-away-score,blank-score,no-home,city-final";
const mixedLines = "city-den=CNO 0 · DEN 0|city-final=CNO 3 · DEN 7";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function expectOnlyScoreboard() {
  expect(held.length).toBeGreaterThan(0);
  for (const call of held) expect(call.input).toBe(SCOREBOARD);
}

// Fake only Date. Faking promises or timers stalls the fetch microtasks.
function useClock(ms: number) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(ms);
}

// The score module keeps one cache. Import it again so each test starts empty.
async function freshScores(): Promise<Scores> {
  vi.resetModules();
  return import("./src/features/sports/scores.ts");
}

function mount(node: ReactNode) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const next = createRoot(el);
  root = next;
  act(() => {
    next.render(node);
  });
  return {
    el,
    unmount() {
      act(() => {
        next.unmount();
      });
      if (root === next) root = null;
      el.remove();
    },
  };
}

async function settle(apply?: () => void) {
  await act(async () => {
    apply?.();
    for (let step = 0; step < 12; step += 1) await Promise.resolve();
  });
}

function BoardText({ useScoreboard }: Pick<Scores, "useScoreboard">) {
  const games = useScoreboard();
  return createElement("p", null, games.map((game) => game.id).join(","));
}

function MapText({ useScoreMap }: Pick<Scores, "useScoreMap">) {
  const lines = useScoreMap();
  const text = [...lines.entries()].map(([id, line]) => `${id}=${line}`).join("|");
  return createElement("p", null, text);
}

function Both({ useScoreboard, useScoreMap }: Pick<Scores, "useScoreboard" | "useScoreMap">) {
  const games = useScoreboard();
  const lines = useScoreMap();
  const ids = games.map((game) => (game.id ? game.id : "(none)")).join(",");
  const mapped = [...lines.entries()].map(([id, line]) => `${id}=${line}`).join("|");
  return createElement("p", null, `${ids}#${mapped}`);
}

beforeEach(() => {
  held.length = 0;
  localStorage.clear();
  vi.useRealTimers();
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url !== SCOREBOARD) throw new Error(`unexpected ${url}`);
    return new Promise<Response>((resolve, reject) => {
      held.push({ input, init, resolve, reject });
    });
  });
});

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
    root = null;
  }
  held.length = 0;
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

test("scoreboard requests send the device token and omit Authorization without one", async () => {
  const { refreshScores, loadScores } = await freshScores();

  const missing = refreshScores();
  expect(held[0].input).toBe(SCOREBOARD);
  expect(held[0].init?.headers).toEqual({});
  held[0].reject(new TypeError("Failed to fetch"));
  await missing;

  localStorage.setItem("bw.deviceToken", "");
  const blank = loadScores();
  expect(held[1].input).toBe(SCOREBOARD);
  expect(held[1].init?.headers).toEqual({});
  held[1].reject(new TypeError("Failed to fetch"));
  await blank;

  localStorage.setItem("bw.deviceToken", "den-token");
  const authed = refreshScores();
  expect(held[2].input).toBe(SCOREBOARD);
  expect(held[2].init?.headers).toEqual({ Authorization: "Bearer den-token" });
  held[2].resolve(json({ games: [kept] }));
  const board = await authed;
  expect(await loadScores()).toBe(board);
  expect(held).toHaveLength(3);

  localStorage.removeItem("bw.deviceToken");
  const dropped = refreshScores();
  expect(dropped).not.toBe(authed);
  expect(held).toHaveLength(4);
  expect(held[3].input).toBe(SCOREBOARD);
  expect(held[3].init?.headers).toEqual({});
  held[3].resolve(json({ games: [lateGame] }));
  expect(await dropped).toEqual([lateGame]);
  expectOnlyScoreboard();
});

test("a 200 scoreboard replaces the cache, and a missing games list is empty", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const first = refreshScores();
  held[0].resolve(json({ games: [kept, lateGame] }));
  const seeded = await first;
  expect(seeded).toEqual([kept, lateGame]);
  expect(await loadScores()).toBe(seeded);
  expect(held).toHaveLength(1);

  const bodies = [json({}), json({ games: null }), json(null), json({ games: [] })];
  for (const body of bodies) {
    const restore = refreshScores();
    held[held.length - 1].resolve(json({ games: [kept] }));
    expect(await restore).toEqual([kept]);

    const cleared = refreshScores();
    held[held.length - 1].resolve(body);
    const empty = await cleared;
    expect(empty).toEqual([]);
    expect(await loadScores()).toBe(empty);
  }

  expect(held).toHaveLength(1 + bodies.length * 2);
  expectOnlyScoreboard();
});

test("a bad response keeps the board that was already there", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const seeded = refreshScores();
  held[0].resolve(json({ games: [kept] }));
  const board = await seeded;

  const rejected = refreshScores();
  held[1].reject(new TypeError("Failed to fetch"));
  expect(await rejected).toBe(board);

  const denied = refreshScores();
  held[2].resolve(json({ games: [earlyGame] }, 503));
  expect(await denied).toBe(board);

  const garbled = refreshScores();
  held[3].resolve(new Response("nope", { status: 200, headers: { "content-type": "application/json" } }));
  expect(await garbled).toBe(board);

  const emptyBody = refreshScores();
  held[4].resolve(new Response(null, { status: 204 }));
  expect(await emptyBody).toBe(board);

  expect(await loadScores()).toBe(board);
  expect(held).toHaveLength(5);
  expectOnlyScoreboard();
});

test("a failed loadScores does not become a fresh empty board", async () => {
  const { loadScores } = await freshScores();
  const first = loadScores();
  held[0].resolve(json({ games: [kept] }, 401));
  expect(await first).toEqual([]);

  const second = loadScores();
  expect(second).not.toBe(first);
  expect(held).toHaveLength(2);
  held[1].reject(new TypeError("Failed to fetch"));
  expect(await second).toEqual([]);

  const third = loadScores();
  expect(held).toHaveLength(3);
  expect(held[2].input).toBe(SCOREBOARD);
  held[2].resolve(new Response("[]", { status: 200, headers: { "content-type": "application/json" } }));
  const empty = await third;
  expect(empty).toEqual([]);

  expect(await loadScores()).toBe(empty);
  expect(held).toHaveLength(3);
  expectOnlyScoreboard();
});

test("a failed refresh keeps the previous games and does not move the freshness window", async () => {
  useClock(noon);
  const { refreshScores, loadScores } = await freshScores();
  const seeded = refreshScores();
  held[0].resolve(json({ games: [kept] }));
  const board = await seeded;

  vi.setSystemTime(noon + 20_000);
  const rejected = refreshScores();
  held[1].reject(new TypeError("Failed to fetch"));
  expect(await rejected).toBe(board);

  vi.setSystemTime(noon + 29_999);
  expect(await loadScores()).toBe(board);
  expect(held).toHaveLength(2);

  vi.setSystemTime(noon + 30_000);
  const again = loadScores();
  expect(held).toHaveLength(3);
  expect(held[2].input).toBe(SCOREBOARD);
  held[2].resolve(json({ games: [earlyGame] }, 404));
  expect(await again).toBe(board);

  const follow = loadScores();
  expect(follow).not.toBe(again);
  expect(held).toHaveLength(4);
  held[3].resolve(json({ games: [lateGame] }));
  expect(await follow).toEqual([lateGame]);
  expectOnlyScoreboard();
});

test("an empty 200 starts a new freshness window", async () => {
  useClock(noon);
  const { refreshScores, loadScores } = await freshScores();
  const seeded = refreshScores();
  held[0].resolve(json({ games: [kept] }));
  await seeded;

  vi.setSystemTime(noon + 20_000);
  const cleared = refreshScores();
  held[1].resolve(json({}));
  const empty = await cleared;
  expect(empty).toEqual([]);

  vi.setSystemTime(noon + 49_999);
  expect(await loadScores()).toBe(empty);
  expect(held).toHaveLength(2);

  vi.setSystemTime(noon + 50_000);
  const again = loadScores();
  expect(held).toHaveLength(3);
  expect(held[2].input).toBe(SCOREBOARD);
  held[2].resolve(json({ games: [lateGame] }));
  expect(await again).toEqual([lateGame]);
  expectOnlyScoreboard();
});

test("loadScores uses the cache for just under 30 seconds, then fetches again", async () => {
  useClock(noon);
  const { loadScores } = await freshScores();
  const first = loadScores();
  expect(held[0].input).toBe(SCOREBOARD);
  held[0].resolve(json({ games: [kept] }));
  const board = await first;

  vi.setSystemTime(noon + 29_999);
  expect(await loadScores()).toBe(board);
  expect(loadScores()).not.toBe(first);
  expect(held).toHaveLength(1);

  vi.setSystemTime(noon + 30_000);
  const second = loadScores();
  expect(second).not.toBe(first);
  expect(held).toHaveLength(2);
  expect(held[1].input).toBe(SCOREBOARD);
  held[1].resolve(json({ games: [lateGame] }));
  expect(await second).toEqual([lateGame]);
  expectOnlyScoreboard();
});

test("refreshScores fetches even when the board is still fresh", async () => {
  useClock(noon);
  const { loadScores, refreshScores } = await freshScores();
  const first = loadScores();
  held[0].resolve(json({ games: [kept] }));
  const board = await first;

  vi.setSystemTime(noon + 1_000);
  expect(await loadScores()).toBe(board);
  expect(held).toHaveLength(1);

  const forced = refreshScores();
  expect(held).toHaveLength(2);
  expect(held[1].input).toBe(SCOREBOARD);
  expect(held[1].init?.headers).toEqual({});
  held[1].resolve(json({ games: [lateGame] }));
  const next = await forced;
  expect(next).toEqual([lateGame]);
  expect(await loadScores()).toBe(next);
  expect(held).toHaveLength(2);
  expectOnlyScoreboard();
});

test("overlapping loadScores calls share one fetch, and refreshScores does not join it", async () => {
  localStorage.setItem("bw.deviceToken", "den-token");
  const { loadScores, refreshScores } = await freshScores();
  const first = loadScores();
  await Promise.resolve();
  localStorage.setItem("bw.deviceToken", "other-token");
  const second = loadScores();
  expect(second).toBe(first);
  expect(held).toHaveLength(1);
  expect(held[0].input).toBe(SCOREBOARD);
  expect(held[0].init?.headers).toEqual({ Authorization: "Bearer den-token" });

  const refreshed = refreshScores();
  expect(loadScores()).toBe(first);
  expect(refreshed).not.toBe(first);
  expect(held.map((call) => call.input)).toEqual([SCOREBOARD, SCOREBOARD]);
  expect(held[1].init?.headers).toEqual({ Authorization: "Bearer other-token" });

  // The load finishes while a newer refresh is still open, so it keeps the
  // board already cached (nothing yet), not the body from this response.
  held[0].resolve(json({ games: [earlyGame] }));
  expect(await first).toEqual([]);
  expect(await second).toEqual([]);
  held[1].resolve(json({ games: [lateGame] }));
  expect(await refreshed).toEqual([lateGame]);
  expect(await loadScores()).toEqual([lateGame]);
  expect(held).toHaveLength(2);
  expectOnlyScoreboard();
});

test("an older refresh keeps the newer board and resolves to that cache", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const early = refreshScores();
  const late = refreshScores();
  expect(early).not.toBe(late);
  expect(held.map((call) => call.input)).toEqual([SCOREBOARD, SCOREBOARD]);

  held[1].resolve(json({ games: [lateGame] }));
  const lateGames = await late;
  expect(lateGames).toEqual([lateGame]);

  held[0].resolve(json({ games: [earlyGame] }));
  const earlyGames = await early;
  expect(earlyGames).toBe(lateGames);
  expect(earlyGames).toEqual([lateGame]);
  expect(earlyGames).not.toEqual([earlyGame]);

  expect(await loadScores()).toBe(lateGames);
  expect(held).toHaveLength(2);
  expectOnlyScoreboard();
});

test("an earlier response that finishes first still does not write over a newer request", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const seeded = refreshScores();
  held[0].resolve(json({ games: [kept] }));
  const board = await seeded;

  const early = refreshScores();
  const late = refreshScores();
  held[1].resolve(json({ games: [earlyGame] }));
  expect(await early).toBe(board);
  expect(await early).not.toEqual([earlyGame]);

  held[2].resolve(json({ games: [lateGame] }));
  const lateGames = await late;
  expect(lateGames).toEqual([lateGame]);
  expect(await loadScores()).toBe(lateGames);
  expect(held).toHaveLength(3);
  expectOnlyScoreboard();
});

test("a newer failure leaves the board, and the older success cannot replace it", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const seeded = refreshScores();
  held[0].resolve(json({ games: [kept] }));
  const board = await seeded;

  const early = refreshScores();
  const late = refreshScores();
  held[2].reject(new TypeError("Failed to fetch"));
  expect(await late).toBe(board);
  held[1].resolve(json({ games: [earlyGame] }));
  expect(await early).toBe(board);
  expect(await early).not.toEqual([earlyGame]);
  expect(await loadScores()).toBe(board);
  expect(held).toHaveLength(3);
  expectOnlyScoreboard();
});

test("an older failure does not clear a newer board", async () => {
  const { refreshScores, loadScores } = await freshScores();
  const early = refreshScores();
  const late = refreshScores();
  held[1].resolve(json({ games: [lateGame] }));
  const lateGames = await late;
  held[0].reject(new TypeError("Failed to fetch"));
  expect(await early).toBe(lateGames);
  expect(await loadScores()).toBe(lateGames);
  expect(held).toHaveLength(2);
  expectOnlyScoreboard();
});

test("useScoreboard lists every game id after the scoreboard loads", async () => {
  localStorage.setItem("bw.deviceToken", "den-token");
  const { useScoreboard } = await freshScores();
  const view = mount(createElement(BoardText, { useScoreboard }));
  expect(view.el.textContent).toBe("");
  expect(held).toHaveLength(1);
  expect(held[0].input).toBe(SCOREBOARD);
  expect(held[0].init?.headers).toEqual({ Authorization: "Bearer den-token" });

  await settle(() => {
    held[0].resolve(json({ games: [kept, lateGame] }));
  });
  expect(view.el.textContent).toBe("city-kept,city-late");
  view.unmount();
  expectOnlyScoreboard();
});

test("a scoreboard unmounted before the response does not throw, and the next mount shows the newest board", async () => {
  localStorage.setItem("bw.deviceToken", "den-token");
  const { useScoreboard, refreshScores } = await freshScores();
  const first = mount(createElement(BoardText, { useScoreboard }));
  expect(first.el.textContent).toBe("");
  const late = held[0];
  expect(late.input).toBe(SCOREBOARD);
  expect(late.init?.headers).toEqual({ Authorization: "Bearer den-token" });
  first.unmount();

  const second = mount(createElement(BoardText, { useScoreboard }));
  expect(second.el.textContent).toBe("");
  expect(held).toHaveLength(1);

  const newer = refreshScores();
  expect(held).toHaveLength(2);
  expect(held[1].input).toBe(SCOREBOARD);
  await settle(() => {
    held[1].resolve(json({ games: [lateGame] }));
  });
  expect(await newer).toEqual([lateGame]);

  await settle(() => {
    late.resolve(json({ games: [earlyGame] }));
  });
  expect(second.el.textContent).toBe("city-late");
  second.unmount();
  expectOnlyScoreboard();
});

test("useScoreMap keeps an in-progress zero and drops a pregame, a missing side, and a blank id", async () => {
  const { useScoreboard, useScoreMap } = await freshScores();
  const view = mount(createElement(Both, { useScoreboard, useScoreMap }));
  expect(view.el.textContent).toBe("#");
  expect(held).toHaveLength(1);
  expect(held[0].input).toBe(SCOREBOARD);
  expect(held[0].init?.headers).toEqual({});

  await settle(() => {
    held[0].resolve(json({ games: mixed }));
  });
  expect(view.el.textContent).toBe(`${mixedIds}#${mixedLines}`);
  view.unmount();
  expectOnlyScoreboard();
});

test("a score map unmounted before the response does not throw, and the next mount reads the cache", async () => {
  const { useScoreMap } = await freshScores();
  const first = mount(createElement(MapText, { useScoreMap }));
  expect(first.el.textContent).toBe("");
  expect(held).toHaveLength(1);
  const late = held[0];
  expect(late.input).toBe(SCOREBOARD);
  first.unmount();

  await settle(() => {
    late.resolve(json({ games: mixed }));
  });
  expect(held).toHaveLength(1);

  const second = mount(createElement(MapText, { useScoreMap }));
  expect(held).toHaveLength(1);
  await settle();
  expect(second.el.textContent).toBe(mixedLines);
  second.unmount();
  expectOnlyScoreboard();
});
