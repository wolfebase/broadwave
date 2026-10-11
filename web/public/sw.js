// The shell only. Hashed files under /assets/ stay in the cache.
// The document is always read from the network first, so a new release
// cannot be stuck behind this worker. Live video, the API, and previews
// are not cached. When the document itself cannot be loaded, the offline
// page is the last resort. It is not the app.

const CACHE = "broadwave-shell";

self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(
    Promise.all([
      fillShell().catch(() => {}),
      cacheOffline().catch(() => {}),
    ]),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      await cacheOffline().catch(() => {});
      await self.clients.claim();
    })(),
  );
});

function leaveAlone(url) {
  const path = url.pathname;
  if (path === "/api" || path.startsWith("/api/")) return true;
  if (path.startsWith("/media/")) return true;
  if (path.includes("/frame")) return true;
  if (/\.(?:m3u8|m4s|ts|mp4)(?:$|\?)/i.test(path)) return true;
  return false;
}

function isDocument(request, url) {
  if (request.mode === "navigate") return true;
  return url.pathname === "/" || url.pathname === "/index.html" || url.pathname === "/offline.html";
}

function isOfflinePage(url) {
  return url.pathname === "/offline.html";
}

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (leaveAlone(url)) return;
  if (isOfflinePage(url)) {
    event.respondWith(offlineDocument(request));
    return;
  }
  if (isDocument(request, url)) {
    event.respondWith(networkFirst(request));
    return;
  }
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(cacheFirst(request));
  }
});

async function fillShell() {
  const fresh = await fetch(new Request("/", { cache: "no-store" }));
  if (!fresh.ok) return;
  const html = await fresh.clone().text();
  await remember(fresh, html);
}

async function cacheOffline() {
  const fresh = await fetch(new Request("/offline.html", { cache: "no-store" }));
  if (!fresh.ok) return;
  if (!(fresh.headers.get("content-type") || "").includes("text/html")) return;
  const cache = await caches.open(CACHE);
  await cache.put("/offline.html", fresh);
}

async function offlineDocument(request) {
  const cache = await caches.open(CACHE);
  try {
    const fresh = await fetch(request, { cache: "no-store" });
    if (fresh.ok && (fresh.headers.get("content-type") || "").includes("text/html")) {
      await cache.put("/offline.html", fresh.clone());
      return fresh;
    }
  } catch {
    // The cached page is the answer.
  }
  const hit = await cache.match("/offline.html");
  if (hit) return hit;
  return new Response("Can't reach the server.", {
    status: 503,
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}

async function networkFirst(request) {
  try {
    const fresh = await fetch(request, { cache: "no-store" });
    if (fresh.ok && (fresh.headers.get("content-type") || "").includes("text/html")) {
      const html = await fresh.clone().text();
      // A cache that will not take the page must not hide the one the network just sent.
      try {
        await remember(fresh.clone(), html);
      } catch {
        // The document is still returned below.
      }
    }
    return fresh;
  } catch {
    const hit = await caches.match("/index.html");
    if (hit) return hit;
    const offline = await caches.match("/offline.html");
    if (offline) return offline;
    throw new Error("offline");
  }
}

async function remember(response, html) {
  const cache = await caches.open(CACHE);
  await cache.put(new Request(new URL("/index.html", self.location.origin).href), response);
  const assets = [...html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)].map((match) => match[1]);
  const added = await Promise.all(assets.map((path) => cacheAsset(cache, path)));
  // A page naming a file the cache lacks is a new release: the old release's
  // files, lazy chunks included, would otherwise stay forever.
  if (added.some(Boolean)) await prune(cache, new Set(assets));
}

async function cacheAsset(cache, path) {
  if (await cache.match(path)) return false;
  try {
    const res = await fetch(path, { cache: "no-store" });
    if (!res.ok) return false;
    await cache.put(path, res);
    return true;
  } catch {
    // The page still asks for the file itself.
    return false;
  }
}

async function prune(cache, keep) {
  for (const request of await cache.keys()) {
    const path = new URL(request.url).pathname;
    if (path.startsWith("/assets/") && !keep.has(path)) await cache.delete(request);
  }
}

async function cacheFirst(request) {
  const cache = await caches.open(CACHE);
  const hit = await cache.match(request);
  if (hit) return hit;
  const fresh = await fetch(request, { cache: "no-store" });
  if (fresh.ok) await cache.put(request, fresh.clone());
  return fresh;
}
