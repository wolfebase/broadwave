import type { Airing, Channel, Recording } from "../types";

const DB = "broadwave";
const STORE = "snapshot";
const KEY = "home";
export const CACHE_FLAG = "wg.cached";

export type Snapshot = {
  channels: Channel[];
  allChannels: Channel[];
  airings: Airing[];
  recordings: Recording[];
  savedAt: number;
};

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

export async function loadSnapshot(): Promise<Snapshot | null> {
  try {
    const db = await open();
    const snap = await new Promise<Snapshot | null>((resolve, reject) => {
      const tx = db.transaction(STORE, "readonly");
      const req = tx.objectStore(STORE).get(KEY);
      req.onsuccess = () => resolve((req.result as Snapshot | undefined) ?? null);
      req.onerror = () => reject(req.error);
    });
    db.close();
    if (!snap || !Array.isArray(snap.channels) || !Array.isArray(snap.airings)) return null;
    return snap;
  } catch {
    return null;
  }
}

export async function saveSnapshot(snap: Snapshot): Promise<void> {
  try {
    const db = await open();
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).put(snap, KEY);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
    db.close();
    localStorage.setItem(CACHE_FLAG, "1");
  } catch {
    // A private window can refuse storage. The live fetch still paints.
  }
}

export function hasSnapshotFlag(): boolean {
  try {
    return localStorage.getItem(CACHE_FLAG) === "1";
  } catch {
    return false;
  }
}
