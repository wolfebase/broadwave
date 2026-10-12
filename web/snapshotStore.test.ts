import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { CACHE_FLAG, loadSnapshot, saveSnapshot, type Snapshot } from "./src/lib/snapshot.ts";
import type { Airing, Channel } from "./src/types.ts";

// jsdom has no indexedDB. open/get/put finish on a microtask because callers
// assign onupgradeneeded, onsuccess, oncomplete, and onerror after they return.
// The first open of a database is version 0 → 1 and runs onupgradeneeded first.

const channel = { id: 4, guideNumber: "4.1", guideName: "Den TV" } as Channel;
const airing = {
  id: 1,
  channelId: 4,
  title: "Night Desk",
  start: "2026-10-11T23:00:00Z",
  end: "2026-10-12T00:00:00Z",
} as Airing;

type Memory = {
  version: number;
  stores: Map<string, Map<unknown, unknown>>;
};

type SeenCall = { name: string; store: string; key?: unknown; mode?: string };

type Listener = ((this: unknown, ev: Event) => void) | null;

type FakeRequest = {
  result?: unknown;
  error: DOMException | null;
  onsuccess: Listener;
  onerror: Listener;
  onupgradeneeded: Listener;
};

const databases = new Map<string, Memory>();
const seen = {
  opens: [] as Array<{ name: string; version: number }>,
  upgrades: [] as Array<{ name: string; from: number; to: number }>,
  created: [] as Array<{ name: string; store: string }>,
  transactions: [] as SeenCall[],
  gets: [] as SeenCall[],
  puts: [] as SeenCall[],
};

let openError: DOMException | null = null;
let transactionError: DOMException | null = null;

function blankRequest(): FakeRequest {
  return { error: null, onsuccess: null, onerror: null, onupgradeneeded: null };
}

function resetMemory() {
  databases.clear();
  seen.opens.length = 0;
  seen.upgrades.length = 0;
  seen.created.length = 0;
  seen.transactions.length = 0;
  seen.gets.length = 0;
  seen.puts.length = 0;
  openError = null;
  transactionError = null;
}

function seedHome(value: unknown) {
  databases.set("broadwave", {
    version: 1,
    stores: new Map([["snapshot", new Map<unknown, unknown>([["home", value]])]]),
  });
}

function installIndexedDB() {
  Object.defineProperty(globalThis, "indexedDB", {
    configurable: true,
    writable: true,
    value: { open: openDatabase },
  });
}

function openDatabase(name: string, version?: number) {
  const requested = version ?? 1;
  seen.opens.push({ name, version: requested });
  const req = blankRequest();
  queueMicrotask(() => finishOpen(req, name, requested));
  return req;
}

function finishOpen(req: FakeRequest, name: string, version: number) {
  if (openError) {
    req.error = openError;
    openError = null;
    req.onerror?.call(req, new Event("error"));
    return;
  }
  let memory = databases.get(name);
  if (!memory) {
    memory = { version: 0, stores: new Map() };
    databases.set(name, memory);
  }
  req.result = connection(name, memory);
  if (memory.version < version) {
    const from = memory.version;
    memory.version = version;
    seen.upgrades.push({ name, from, to: version });
    req.onupgradeneeded?.call(req, new Event("upgradeneeded"));
  }
  req.onsuccess?.call(req, new Event("success"));
}

function connection(name: string, memory: Memory) {
  return {
    name,
    get version() {
      return memory.version;
    },
    objectStoreNames: {
      get length() {
        return memory.stores.size;
      },
      contains(store: string) {
        return memory.stores.has(store);
      },
      item(index: number) {
        return [...memory.stores.keys()][index] ?? null;
      },
    },
    createObjectStore(store: string) {
      if (memory.stores.has(store)) {
        throw new DOMException(`Store ${store} already exists.`, "ConstraintError");
      }
      memory.stores.set(store, new Map());
      seen.created.push({ name, store });
    },
    transaction(store: string, mode: string) {
      return transact(name, memory, store, mode);
    },
    close() {
      // Later opens of this name keep the records. close() must not wipe them.
    },
  };
}

function transact(dbName: string, memory: Memory, store: string, mode: string) {
  seen.transactions.push({ name: dbName, store, mode });
  const tx: {
    error: DOMException | null;
    oncomplete: Listener;
    onerror: Listener;
    objectStore: (next: string) => {
      get: (key: unknown) => FakeRequest;
      put: (value: unknown, key: unknown) => FakeRequest;
    };
  } = {
    error: null,
    oncomplete: null,
    onerror: null,
    objectStore(next: string) {
      return {
        get(key: unknown) {
          seen.gets.push({ name: dbName, store: next, key });
          const req = blankRequest();
          queueMicrotask(() => {
            const records = memory.stores.get(next);
            if (!records) {
              req.error = new DOMException("The snapshot store is missing.", "NotFoundError");
              req.onerror?.call(req, new Event("error"));
              return;
            }
            req.result = records.has(key) ? records.get(key) : undefined;
            req.onsuccess?.call(req, new Event("success"));
          });
          return req;
        },
        put(value: unknown, key: unknown) {
          seen.puts.push({ name: dbName, store: next, key });
          const req = blankRequest();
          queueMicrotask(() => {
            if (transactionError) {
              tx.error = transactionError;
              req.error = transactionError;
              transactionError = null;
              tx.onerror?.call(tx, new Event("error"));
              return;
            }
            const records = memory.stores.get(next);
            if (!records) {
              tx.error = new DOMException("The snapshot store is missing.", "NotFoundError");
              tx.onerror?.call(tx, new Event("error"));
              return;
            }
            records.set(key, value);
            tx.oncomplete?.call(tx, new Event("complete"));
          });
          return req;
        },
      };
    },
  };
  return tx;
}

function home(savedAt: number, title = airing.title): Snapshot {
  return {
    channels: [{ ...channel }],
    allChannels: [{ ...channel }],
    airings: [{ ...airing, title }],
    recordings: [],
    savedAt,
  };
}

const notSnapshots: ReadonlyArray<readonly [string, unknown]> = [
  ["a string", "Night Desk"],
  ["a number", 4],
  ["null", null],
  ["an array", [channel]],
  ["channels that are not an array", { channels: "4.1", airings: [] }],
  ["airings that are not an array", { channels: [], airings: "Night Desk" }],
  ["missing channels", { airings: [] }],
  ["missing airings", { channels: [] }],
];

beforeEach(() => {
  resetMemory();
  installIndexedDB();
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  installIndexedDB();
});

test("loadSnapshot returns null when the database refuses to open", async () => {
  openError = new DOMException("The database is blocked.", "UnknownError");
  expect(await loadSnapshot()).toBeNull();
  expect(seen.opens).toEqual([{ name: "broadwave", version: 1 }]);
  expect(seen.upgrades).toEqual([]);
});

test("loadSnapshot returns null when indexedDB is missing", async () => {
  expect(Reflect.deleteProperty(globalThis, "indexedDB")).toBe(true);
  expect(await loadSnapshot()).toBeNull();
  expect(seen.opens).toEqual([]);
});

test("loadSnapshot returns null for an empty store and upgrades broadwave only once", async () => {
  expect(await loadSnapshot()).toBeNull();
  expect(await loadSnapshot()).toBeNull();
  expect(seen.opens).toEqual([
    { name: "broadwave", version: 1 },
    { name: "broadwave", version: 1 },
  ]);
  expect(seen.upgrades).toEqual([{ name: "broadwave", from: 0, to: 1 }]);
  expect(seen.created).toEqual([{ name: "broadwave", store: "snapshot" }]);
  expect(seen.gets).toEqual([
    { name: "broadwave", store: "snapshot", key: "home" },
    { name: "broadwave", store: "snapshot", key: "home" },
  ]);
});

test.each(notSnapshots)("loadSnapshot returns null when the stored home value is %s", async (_label, value) => {
  seedHome(value);
  expect(await loadSnapshot()).toBeNull();
});

test("loadSnapshot returns the stored object when channels and airings are arrays", async () => {
  const empty = { channels: [], airings: [] };
  seedHome(empty);
  expect(await loadSnapshot()).toBe(empty);

  const partial = {
    channels: [channel],
    airings: [airing],
    savedAt: Date.parse("2026-10-11T23:00:00Z"),
  };
  seedHome(partial);
  expect(await loadSnapshot()).toBe(partial);
  expect(partial).not.toHaveProperty("recordings");
});

test("saveSnapshot stores home, sets the cache flag, and replaces it on the next save", async () => {
  expect(localStorage.getItem(CACHE_FLAG)).toBeNull();

  const first = home(Date.parse("2026-10-11T23:00:00Z"));
  const second = home(Date.parse("2026-10-12T00:30:00Z"), "Late Desk");
  await saveSnapshot(first);
  expect(localStorage.getItem(CACHE_FLAG)).toBe("1");
  expect(await loadSnapshot()).toEqual(first);

  await saveSnapshot(second);
  const loaded = await loadSnapshot();
  expect(loaded).toEqual(second);
  expect(loaded).not.toEqual(first);
  expect(loaded?.channels).toEqual(second.channels);
  expect(loaded?.airings).toEqual(second.airings);
  expect(loaded?.savedAt).toBe(second.savedAt);

  expect(seen.opens).toEqual([
    { name: "broadwave", version: 1 },
    { name: "broadwave", version: 1 },
    { name: "broadwave", version: 1 },
    { name: "broadwave", version: 1 },
  ]);
  expect(seen.upgrades).toEqual([{ name: "broadwave", from: 0, to: 1 }]);
  expect(seen.created).toEqual([{ name: "broadwave", store: "snapshot" }]);
  expect(seen.transactions).toEqual([
    { name: "broadwave", store: "snapshot", mode: "readwrite" },
    { name: "broadwave", store: "snapshot", mode: "readonly" },
    { name: "broadwave", store: "snapshot", mode: "readwrite" },
    { name: "broadwave", store: "snapshot", mode: "readonly" },
  ]);
  expect(seen.puts).toEqual([
    { name: "broadwave", store: "snapshot", key: "home" },
    { name: "broadwave", store: "snapshot", key: "home" },
  ]);
});

test("saveSnapshot does not throw or set the cache flag when the write fails", async () => {
  expect(localStorage.getItem(CACHE_FLAG)).toBeNull();
  transactionError = new DOMException("The snapshot could not be saved.", "AbortError");
  await expect(saveSnapshot(home(Date.parse("2026-10-11T23:00:00Z")))).resolves.toBeUndefined();
  expect(localStorage.getItem(CACHE_FLAG)).toBeNull();
  expect(seen.puts).toEqual([{ name: "broadwave", store: "snapshot", key: "home" }]);
  expect(await loadSnapshot()).toBeNull();
});

test("saveSnapshot does not throw when localStorage rejects the cache flag", async () => {
  const saved = home(Date.parse("2026-10-11T23:00:00Z"));
  const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("storage is blocked");
  });
  await expect(saveSnapshot(saved)).resolves.toBeUndefined();
  expect(setItem).toHaveBeenCalledWith(CACHE_FLAG, "1");
  vi.restoreAllMocks();
  expect(await loadSnapshot()).toEqual(saved);
});
