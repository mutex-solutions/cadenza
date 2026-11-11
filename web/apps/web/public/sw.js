/* Cadenza service worker: shows notifications, and keeps the app shell so the
   installed app opens offline. API calls (/v1/) and files (/blob/) are never cached. */
const SHELL = "cadenza-shell-v1";

self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(
    caches
      .open(SHELL)
      .then((c) => c.addAll(["/", "/manifest.webmanifest", "/icon.svg", "/icon-192.png"]))
      .catch(() => undefined),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/v1/") || url.pathname.startsWith("/blob/")) return;
  if (req.mode === "navigate") {
    // Network first, so a new version shows at once; the saved shell when offline.
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            caches.open(SHELL).then((c) => c.put("/", copy)).catch(() => undefined);
          }
          return res;
        })
        .catch(() => caches.match("/").then((hit) => hit || Response.error())),
    );
    return;
  }
  if (url.pathname.startsWith("/assets/") || url.pathname.startsWith("/viewer/")) {
    // Built files have content hashes in their names, so a saved copy is always right.
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ||
          fetch(req).then((res) => {
            if (res.ok && url.pathname.startsWith("/assets/")) {
              const copy = res.clone();
              caches.open(SHELL).then((c) => c.put(req, copy)).catch(() => undefined);
            }
            return res;
          }),
      ),
    );
  }
});

/** Reads the server's message: {title, body, url, thread_id}. */
function parsePush(data) {
  let msg = {};
  try {
    msg = data ? data.json() : {};
  } catch {
    msg = { body: data ? data.text() : "" };
  }
  return {
    title: typeof msg.title === "string" && msg.title ? msg.title : "Cadenza",
    body: typeof msg.body === "string" ? msg.body : "",
    url: typeof msg.url === "string" && msg.url.startsWith("/") ? msg.url : "/",
    tag: typeof msg.thread_id === "string" ? msg.thread_id : undefined,
  };
}

self.addEventListener("push", (event) => {
  const m = parsePush(event.data);
  event.waitUntil(
    self.registration.showNotification(m.title, {
      body: m.body,
      tag: m.tag,
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      data: { url: m.url },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const path = (event.notification.data && event.notification.data.url) || "/";
  const target = new URL(path, self.location.origin);
  if (target.origin !== self.location.origin) return;
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((wins) => {
      const win = wins.find((w) => new URL(w.url).origin === self.location.origin);
      if (win) {
        return win.focus().then((w) => (w || win).navigate(target.href));
      }
      return self.clients.openWindow(target.href);
    }),
  );
});
