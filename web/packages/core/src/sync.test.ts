import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Event as ChangeEvent, EventList } from "./api.gen.ts";
import { createClient } from "./client.ts";
import { memoryStorage } from "./storage.ts";
import { SyncEngine, isUnread, type EventSourceLike } from "./sync.ts";
import { fakeFetch, thread, threadView } from "./test/fakes.ts";

class FakeES implements EventSourceLike {
  static last: FakeES | null = null;
  static urls: string[] = [];
  listeners = new Map<string, ((e: MessageEvent<string>) => void)[]>();
  onerror: ((e: unknown) => void) | null = null;
  readyState = 0;
  closed = false;
  constructor(public url: string) {
    FakeES.last = this;
    FakeES.urls.push(url);
  }
  addEventListener(type: string, fn: (e: MessageEvent<string>) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }
  emit(type: string, data: unknown = {}, id?: string) {
    for (const fn of this.listeners.get(type) ?? []) fn({ data: JSON.stringify(data), lastEventId: id ?? "" } as MessageEvent<string>);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
}

const ev = (seq: number, kind: string, thread?: string): ChangeEvent => ({ seq, kind, thread, at: seq });

async function flush(ms = 200) {
  await vi.advanceTimersByTimeAsync(ms);
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeES.last = null;
  FakeES.urls = [];
});
afterEach(() => vi.useRealTimers());

describe("isUnread", () => {
  it("uses the server flag when present, else the seen map", () => {
    expect(isUnread(thread("a", 5, { unread: false }), {})).toBe(false);
    expect(isUnread(thread("a", 5), {})).toBe(true);
    expect(isUnread(thread("a", 5), { a: 5 })).toBe(false);
    expect(isUnread(thread("a", 6), { a: 5 })).toBe(true);
  });
});

describe("SyncEngine with server-sent events", () => {
  it("loads the inbox on ready, follows changes and refreshes open threads", async () => {
    let inbox = [thread("t1", 1)];
    let view = threadView("t1", 1);
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: inbox } }),
      "GET /v1/threads/t1": () => ({ body: view }),
    });
    const storage = memoryStorage();
    const onEvent = vi.fn();
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage, eventSource: (u) => new FakeES(u), onEvent });
    await sync.start();
    expect(FakeES.urls).toEqual(["/v1/events/stream"]);
    expect(sync.getState().connection).toBe("connecting");

    FakeES.last!.emit("ready");
    await flush();
    let s = sync.getState();
    expect(s.connection).toBe("live");
    expect(s.ready).toBe(true);
    expect(s.inbox.map((t) => t.id)).toEqual(["t1"]);
    // First run: existing threads count as seen.
    expect(isUnread(s.inbox[0]!, s.seen)).toBe(false);

    await sync.openThread("t1");
    inbox = [thread("t1", 2, { open_questions: 1 }), thread("t2", 2)];
    view = threadView("t1", 2);
    FakeES.last!.emit("change", ev(7, "question.asked", "t1"), "7");
    FakeES.last!.emit("change", ev(8, "thread.created", "t2"), "8");
    await flush();
    s = sync.getState();
    expect(s.cursor).toBe(8);
    expect(s.inbox.map((t) => t.id)).toEqual(["t1", "t2"]);
    expect(s.threads.t1!.thread.updated_at).toBe(2);
    expect(isUnread(s.inbox[0]!, s.seen)).toBe(true);
    expect(isUnread(s.inbox[1]!, s.seen)).toBe(true);
    expect(onEvent).toHaveBeenCalledTimes(2);
    // Changes in a burst are fetched once.
    expect(f.calls.filter((c) => c.url === "/v1/threads/t1")).toHaveLength(2);

    sync.markSeen("t1");
    expect(isUnread(sync.getState().inbox[0]!, sync.getState().seen)).toBe(false);

    // The copy is saved for next time.
    await flush(10);
    const saved = storage.dump()["sync:v1"] as { cursor: number; threads: Record<string, unknown> };
    expect(saved.cursor).toBe(8);
    expect(Object.keys(saved.threads)).toEqual(["t1"]);
    sync.stop();
    expect(FakeES.last!.closed).toBe(true);
  });

  it("opens instantly from the saved copy and resumes after its cursor", async () => {
    const storage = memoryStorage({
      "sync:v1": { v: 1, inbox: [thread("old", 3)], threads: { old: threadView("old", 3) }, order: ["old"], cursor: 42, seen: { old: 3 } },
    });
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: [thread("old", 3)] } }),
      "GET /v1/threads/old": () => ({ body: threadView("old", 3) }),
    });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage, eventSource: (u) => new FakeES(u) });
    await sync.start();
    const s = sync.getState();
    expect(s.ready && s.fromCache).toBe(true);
    expect(s.inbox[0]!.id).toBe("old");
    expect(f.calls).toHaveLength(0);
    expect(FakeES.urls).toEqual(["/v1/events/stream?after=42"]);
    // A cached thread is returned at once and refreshed in the background.
    const v = await sync.openThread("old");
    expect(v.thread.id).toBe("old");
    FakeES.last!.emit("open");
    await flush();
    expect(sync.getState().fromCache).toBe(false);
    sync.stop();
  });

  it("reloads everything on reset", async () => {
    const storage = memoryStorage({ "sync:v1": { v: 1, inbox: [], threads: {}, order: [], cursor: 5, seen: {} } });
    const f = fakeFetch({ "GET /v1/threads": () => ({ body: { threads: [thread("n", 9)] } }) });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage, eventSource: (u) => new FakeES(u) });
    await sync.start();
    const first = FakeES.last!;
    first.emit("reset");
    await flush();
    expect(first.closed).toBe(true);
    expect(sync.getState().cursor).toBe(0);
    expect(sync.getState().inbox[0]!.id).toBe("n");
    expect(FakeES.urls.at(-1)).toBe("/v1/events/stream");
    sync.stop();
  });

  it("falls back to polling after repeated stream errors", async () => {
    const pages: EventList[] = [{ events: [], last: 12 }];
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: [] } }),
      "GET /v1/events": () => ({ body: pages.shift() ?? { events: [], last: 12 } }),
    });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage: memoryStorage(), eventSource: (u) => new FakeES(u), pollMs: 1000 });
    await sync.start();
    const es = FakeES.last!;
    es.onerror?.({} as never);
    es.onerror?.({} as never);
    expect(sync.getState().connection).toBe("connecting");
    es.onerror?.({} as never);
    expect(es.closed).toBe(true);
    await flush(10);
    expect(sync.getState().connection).toBe("polling");
    expect(sync.getState().cursor).toBe(12);
    sync.stop();
  });
});

describe("SyncEngine polling", () => {
  it("finds the newest change number, then polls after it", async () => {
    const full = Array.from({ length: 500 }, (_, i) => ev(i + 1, "post.added", "t1"));
    const responses: EventList[] = [
      { events: full, last: 500 },
      { events: [ev(501, "post.added", "t1")], last: 501 },
      { events: [ev(502, "thread.created", "t2")], last: 502 },
    ];
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: [thread("t1", 1)] } }),
      "GET /v1/events": () => ({ body: responses.shift() ?? { events: [], last: 502 } }),
    });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage: memoryStorage(), eventSource: null, pollMs: 1000 });
    await sync.start();
    await flush(10);
    const eventCalls = () => f.calls.filter((c) => c.url.startsWith("/v1/events")).map((c) => c.url);
    expect(eventCalls()).toEqual(["/v1/events?after=0", "/v1/events?after=500"]);
    expect(sync.getState().cursor).toBe(501);
    expect(sync.getState().connection).toBe("polling");
    await flush(1000);
    expect(eventCalls().at(-1)).toBe("/v1/events?after=501");
    expect(sync.getState().cursor).toBe(502);
    sync.stop();
  });

  it("handles reset while polling by starting over", async () => {
    const responses: EventList[] = [{ events: [], reset: true, last: 900 }, { events: [], last: 900 }];
    const storage = memoryStorage({ "sync:v1": { v: 1, inbox: [], threads: {}, order: [], cursor: 3, seen: {} } });
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: [] } }),
      "GET /v1/events": () => ({ body: responses.shift() ?? { events: [], last: 900 } }),
    });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage, eventSource: null, pollMs: 1000 });
    await sync.start();
    await flush(10);
    expect(sync.getState().cursor).toBe(0);
    await flush(1000);
    expect(sync.getState().cursor).toBe(900);
    sync.stop();
  });

  it("keeps the saved copy and backs off while offline", async () => {
    let down = true;
    const f = fakeFetch({
      "GET /v1/threads": () => ({ body: { threads: [] } }),
      "GET /v1/events": () => ({ body: { events: [], last: 1 } }),
    });
    const fetchFn = (async (i: RequestInfo | URL, init?: RequestInit) => {
      if (down) throw new TypeError("offline");
      return f.fetch(i, init);
    }) as typeof fetch;
    const storage = memoryStorage({ "sync:v1": { v: 1, inbox: [thread("c", 1)], threads: {}, order: [], cursor: 1, seen: {} } });
    const sync = new SyncEngine({ client: createClient({ fetch: fetchFn }), storage, eventSource: null, pollMs: 1000 });
    await sync.start();
    await flush(10);
    expect(sync.getState().connection).toBe("offline");
    expect(sync.getState().inbox[0]!.id).toBe("c");
    down = false;
    await flush(1000);
    expect(sync.getState().connection).toBe("offline"); // still waiting: backoff is 2 s
    await flush(1000);
    expect(sync.getState().connection).toBe("polling");
    expect(sync.getState().error).toBeUndefined();
    sync.stop();
  });

  it("stops polling when not logged in", async () => {
    const f = fakeFetch({ "GET /v1/events": () => ({ status: 401, body: { error: { kind: "unauthorized", message: "log in" } } }) });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage: memoryStorage(), eventSource: null, pollMs: 1000 });
    await sync.start();
    await flush(10);
    expect(sync.getState().error?.unauthorized).toBe(true);
    const n = f.calls.length;
    await flush(10_000);
    expect(f.calls.length).toBe(n);
    sync.stop();
  });

  it("clear() drops the saved copy", async () => {
    const storage = memoryStorage({ "sync:v1": { v: 1, inbox: [thread("x")], threads: {}, order: [], cursor: 1, seen: {} } });
    const f = fakeFetch({ "GET /v1/events": () => ({ body: { events: [], last: 1 } }), "GET /v1/threads": () => ({ body: { threads: [] } }) });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage, eventSource: null });
    await sync.start();
    await sync.clear();
    expect(storage.dump()["sync:v1"]).toBeUndefined();
    expect(sync.getState().inbox).toEqual([]);
  });

  it("keeps only the most recent threads in the copy", async () => {
    const f = fakeFetch({
      "GET /v1/events": () => ({ body: { events: [], last: 1 } }),
      "GET /v1/threads": () => ({ body: { threads: [] } }),
      "GET /v1/threads/a": () => ({ body: threadView("a") }),
      "GET /v1/threads/b": () => ({ body: threadView("b") }),
      "GET /v1/threads/c": () => ({ body: threadView("c") }),
    });
    const sync = new SyncEngine({ client: createClient({ fetch: f.fetch }), storage: memoryStorage(), eventSource: null, maxCachedThreads: 2 });
    await sync.refreshThread("a");
    await sync.refreshThread("b");
    await sync.refreshThread("c");
    expect(Object.keys(sync.getState().threads).sort()).toEqual(["b", "c"]);
  });
});
