// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";

const source = readFileSync(new URL("../public/sw.js", import.meta.url), "utf8");

function load({ fetchImpl, wins = [] } = {}) {
  const handlers = {};
  const store = new Map();
  const cache = { addAll: vi.fn(async () => {}), put: vi.fn(async (k, v) => store.set(typeof k === "string" ? k : k.url, v)) };
  const caches = {
    open: vi.fn(async () => cache),
    keys: vi.fn(async () => ["old", "cadenza-shell-v1"]),
    delete: vi.fn(async () => true),
    match: vi.fn(async (k) => store.get(typeof k === "string" ? new URL(k, "https://cz.test").href.replace("https://cz.test", "") || k : k.url) ?? store.get(typeof k === "string" ? k : k.url)),
  };
  const self = {
    location: { origin: "https://cz.test" },
    addEventListener: (t, f) => (handlers[t] = f),
    skipWaiting: vi.fn(),
    registration: { showNotification: vi.fn(async () => {}) },
    clients: { claim: vi.fn(async () => {}), matchAll: vi.fn(async () => wins), openWindow: vi.fn(async () => {}) },
  };
  new Function("self", "caches", "fetch", "Response", source)(self, caches, fetchImpl ?? vi.fn(), Response);
  return { handlers, self, caches, cache, store };
}

function event(extra) {
  const e = { ...extra, waits: [], responded: undefined };
  e.waitUntil = (p) => e.waits.push(p);
  e.respondWith = (p) => (e.responded = p);
  return e;
}

describe("service worker", () => {
  it("shows the server's notification with its title, body and url", async () => {
    const { handlers, self } = load();
    const e = event({ data: { json: () => ({ title: "Claude Code · vm-03", body: "Keep promo visible?", url: "/t/t_81", thread_id: "t_81" }) } });
    handlers.push(e);
    await Promise.all(e.waits);
    expect(self.registration.showNotification).toHaveBeenCalledWith("Claude Code · vm-03", expect.objectContaining({ body: "Keep promo visible?", tag: "t_81", data: { url: "/t/t_81" } }));
  });

  it("falls back to a plain notification for odd payloads and never uses outside URLs", async () => {
    const { handlers, self } = load();
    const e = event({ data: { json: () => ({ url: "https://evil.example/x" }) } });
    handlers.push(e);
    await Promise.all(e.waits);
    expect(self.registration.showNotification).toHaveBeenCalledWith("Cadenza", expect.objectContaining({ data: { url: "/" } }));
  });

  it("opens the thread on click, reusing an open window", async () => {
    const win = { url: "https://cz.test/", focus: vi.fn(async () => win), navigate: vi.fn(async () => {}) };
    const { handlers, self } = load({ wins: [win] });
    const close = vi.fn();
    const e = event({ notification: { close, data: { url: "/t/t_81" } } });
    handlers.notificationclick(e);
    await Promise.all(e.waits);
    expect(close).toHaveBeenCalled();
    expect(win.navigate).toHaveBeenCalledWith("https://cz.test/t/t_81");
    expect(self.clients.openWindow).not.toHaveBeenCalled();
  });

  it("opens a new window when none is open", async () => {
    const { handlers, self } = load();
    const e = event({ notification: { close: vi.fn(), data: { url: "/t/x" } } });
    handlers.notificationclick(e);
    await Promise.all(e.waits);
    expect(self.clients.openWindow).toHaveBeenCalledWith("https://cz.test/t/x");
  });

  it("never touches API calls or file downloads", () => {
    const { handlers } = load();
    for (const path of ["/v1/threads", "/blob/abc"]) {
      const e = event({ request: { method: "GET", url: `https://cz.test${path}`, mode: "cors" } });
      handlers.fetch(e);
      expect(e.responded).toBeUndefined();
    }
    const post = event({ request: { method: "POST", url: "https://cz.test/", mode: "navigate" } });
    handlers.fetch(post);
    expect(post.responded).toBeUndefined();
  });

  it("serves the saved shell for page loads when offline", async () => {
    const shell = new Response("<html>shell</html>");
    const fetchImpl = vi.fn(async () => {
      throw new TypeError("offline");
    });
    const { handlers, store } = load({ fetchImpl });
    store.set("/", shell);
    const e = event({ request: { method: "GET", url: "https://cz.test/t/t_1", mode: "navigate" } });
    handlers.fetch(e);
    expect(await e.responded).toBe(shell);
  });

  it("saves the shell after a successful page load", async () => {
    const fetchImpl = vi.fn(async () => new Response("<html>new</html>", { status: 200 }));
    const { handlers, cache } = load({ fetchImpl });
    const e = event({ request: { method: "GET", url: "https://cz.test/", mode: "navigate" } });
    handlers.fetch(e);
    await e.responded;
    await new Promise((r) => setTimeout(r, 0));
    expect(cache.put).toHaveBeenCalledWith("/", expect.any(Response));
  });

  it("caches built assets", async () => {
    const fetchImpl = vi.fn(async () => new Response("js", { status: 200 }));
    const { handlers, cache } = load({ fetchImpl });
    const req = { method: "GET", url: "https://cz.test/assets/index-abc.js", mode: "cors" };
    const e = event({ request: req });
    handlers.fetch(e);
    await e.responded;
    await new Promise((r) => setTimeout(r, 0));
    expect(cache.put).toHaveBeenCalledWith(req, expect.any(Response));
  });

  it("drops old caches on activate", async () => {
    const { handlers, caches } = load();
    const e = event({});
    handlers.activate(e);
    await Promise.all(e.waits);
    expect(caches.delete).toHaveBeenCalledWith("old");
    expect(caches.delete).not.toHaveBeenCalledWith("cadenza-shell-v1");
  });
});
