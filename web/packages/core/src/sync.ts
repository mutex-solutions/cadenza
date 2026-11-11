import type { Event as ChangeEvent, ThreadSummary, ThreadView } from "./api.gen.ts";
import { ApiError, type Client } from "./client.ts";
import type { KV } from "./storage.ts";

/** The part of EventSource the engine uses; injectable for tests and other platforms. */
export interface EventSourceLike {
  addEventListener(type: string, listener: (e: MessageEvent<string>) => void): void;
  close(): void;
  onerror: ((e: never) => unknown) | null;
  readonly readyState: number;
}
export type EventSourceFactory = (url: string) => EventSourceLike;

/** live: server-sent events; polling: asking every few seconds; offline: can't reach the server. */
export type Connection = "connecting" | "live" | "polling" | "offline";

export interface SyncState {
  /** True once the inbox is known, from the saved copy or the server. */
  ready: boolean;
  /** True while what's shown came from the saved copy and the server hasn't answered yet. */
  fromCache: boolean;
  inbox: ThreadSummary[];
  /** Threads opened recently, by id. */
  threads: Record<string, ThreadView>;
  /** The number of the last change seen (plan D15). */
  cursor: number;
  /** Per thread, the updated_at the owner last saw; drives "unread". */
  seen: Record<string, number>;
  connection: Connection;
  /** The last error, e.g. not logged in. Cleared on the next success. */
  error?: ApiError;
}

export interface SyncOptions {
  client: Client;
  storage: KV;
  /** Makes an EventSource; null or missing uses polling only (e.g. React Native). */
  eventSource?: EventSourceFactory | null;
  /** How often to poll events when there's no live stream. Default 5 s. */
  pollMs?: number;
  /** Changes arriving together are fetched once after this delay. Default 150 ms. */
  debounceMs?: number;
  /** How many threads to keep in the offline copy. Default 30. */
  maxCachedThreads?: number;
  /** Called for every change, e.g. so the machines page can refresh. */
  onEvent?: (e: ChangeEvent) => void;
}

interface Saved {
  v: 1;
  inbox: ThreadSummary[];
  threads: Record<string, ThreadView>;
  order: string[];
  cursor: number;
  seen: Record<string, number>;
}

const CACHE_KEY = "sync:v1";
const PAGE = 500;
const MAX_SSE_ERRORS = 3;
const MAX_BACKOFF_MS = 60_000;

/** Whether the owner hasn't looked at this thread since it last changed. */
export function isUnread(t: ThreadSummary, seen: Record<string, number>): boolean {
  if (t.unread !== undefined) return t.unread;
  const s = seen[t.id];
  return s === undefined || t.updated_at > s;
}

/**
 * Keeps the inbox and open threads up to date: shows the saved copy at once,
 * then follows the numbered change log, live over server-sent events or by
 * polling `events?after=N`. A "reset" (too far behind) reloads everything.
 */
export class SyncEngine {
  private o: Required<Omit<SyncOptions, "eventSource" | "onEvent">> & Pick<SyncOptions, "eventSource" | "onEvent">;
  private state: SyncState = { ready: false, fromCache: false, inbox: [], threads: {}, cursor: 0, seen: {}, connection: "connecting" };
  private order: string[] = [];
  private seededSeen = false;
  private listeners = new Set<(s: SyncState) => void>();
  private es: EventSourceLike | null = null;
  private sseErrors = 0;
  private timers = new Map<string, ReturnType<typeof setTimeout>>();
  private pollTimer: ReturnType<typeof setTimeout> | null = null;
  private backoff = 0;
  private running = false;
  private persistTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(opts: SyncOptions) {
    this.o = { pollMs: 5000, debounceMs: 150, maxCachedThreads: 30, ...opts };
  }

  getState = (): SyncState => this.state;

  subscribe = (fn: (s: SyncState) => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  /** Loads the saved copy, then connects. Resolves once the saved copy is shown. */
  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    const saved = await this.o.storage.get<Saved>(CACHE_KEY).catch(() => undefined);
    if (saved && saved.v === 1) {
      this.order = saved.order ?? [];
      this.seededSeen = true;
      this.set({ ready: true, fromCache: true, inbox: saved.inbox, threads: saved.threads, cursor: saved.cursor, seen: saved.seen });
    }
    this.connect();
  }

  /** Stops following changes. The saved copy stays. */
  stop(): void {
    this.running = false;
    this.closeStream();
    if (this.pollTimer) clearTimeout(this.pollTimer);
    this.pollTimer = null;
    for (const t of this.timers.values()) clearTimeout(t);
    this.timers.clear();
  }

  /** Drops the saved copy, e.g. on logout. */
  async clear(): Promise<void> {
    this.stop();
    this.order = [];
    this.seededSeen = false;
    this.state = { ready: false, fromCache: false, inbox: [], threads: {}, cursor: 0, seen: {}, connection: "connecting" };
    await this.o.storage.delete(CACHE_KEY);
    this.emit();
  }

  /** Reconnects now, e.g. when the device comes back online. */
  reconnect(): void {
    if (!this.running) return;
    this.closeStream();
    if (this.pollTimer) clearTimeout(this.pollTimer);
    this.pollTimer = null;
    this.backoff = 0;
    this.connect();
  }

  /** A thread from the saved copy (if any) and a fresh copy from the server. */
  async openThread(id: string): Promise<ThreadView> {
    const cached = this.state.threads[id];
    if (cached) {
      this.touch(id);
      void this.refreshThread(id).catch(() => undefined);
      return cached;
    }
    return this.refreshThread(id);
  }

  /** Fetches one thread now. */
  async refreshThread(id: string): Promise<ThreadView> {
    try {
      const view = await this.o.client.call("thread-view", { params: { id } });
      this.touch(id);
      const threads = { ...this.state.threads, [id]: view };
      for (const old of this.order.slice(this.o.maxCachedThreads)) delete threads[old];
      this.order = this.order.slice(0, this.o.maxCachedThreads);
      const inbox = this.state.inbox.map((t) => (t.id === id ? { ...t, ...view.thread } : t));
      this.set({ threads, inbox, error: undefined });
      this.persist();
      return view;
    } catch (err) {
      this.fail(err);
      throw err;
    }
  }

  /** Fetches the inbox now. */
  async refreshInbox(): Promise<void> {
    try {
      const { threads } = await this.o.client.call("threads");
      let seen = this.state.seen;
      if (!this.seededSeen) {
        // First run: everything already there counts as seen.
        seen = Object.fromEntries(threads.map((t) => [t.id, t.updated_at]));
        this.seededSeen = true;
      }
      this.set({ inbox: threads, seen, ready: true, fromCache: false, error: undefined });
      this.persist();
    } catch (err) {
      this.fail(err);
    }
  }

  /** Records that the owner looked at a thread as it is now. */
  markSeen(id: string): void {
    const t = this.state.threads[id]?.thread ?? this.state.inbox.find((x) => x.id === id);
    if (!t) return;
    if (this.state.seen[id] === t.updated_at) return;
    this.set({ seen: { ...this.state.seen, [id]: t.updated_at } });
    this.persist();
  }

  // --- connection -------------------------------------------------------

  private connect(): void {
    if (!this.running) return;
    if (this.o.eventSource) this.openStream(this.o.eventSource);
    else this.startPolling();
  }

  private openStream(factory: EventSourceFactory): void {
    this.set({ connection: this.state.connection === "live" ? "live" : "connecting" });
    const c = this.state.cursor;
    const url = this.o.client.url("events-stream", undefined, c > 0 ? { after: c } : undefined);
    let es: EventSourceLike;
    try {
      es = factory(url);
    } catch {
      this.startPolling();
      return;
    }
    this.es = es;
    const alive = () => {
      this.sseErrors = 0;
      if (this.state.connection !== "live") this.set({ connection: "live" });
    };
    es.addEventListener("open", () => {
      alive();
      this.debounce("inbox", () => this.reloadAll());
    });
    es.addEventListener("ready", () => {
      alive();
      this.debounce("inbox", () => this.reloadAll());
    });
    es.addEventListener("change", (m) => {
      alive();
      let e: ChangeEvent;
      try {
        e = JSON.parse(m.data) as ChangeEvent;
      } catch {
        return;
      }
      this.apply([e]);
    });
    es.addEventListener("reset", () => {
      this.closeStream();
      void this.reset().then(() => this.connect());
    });
    es.onerror = () => {
      this.sseErrors++;
      // EventSource retries by itself (sending Last-Event-ID). Give up after a few
      // failures in a row, or when it closed for good (e.g. 401), and poll instead.
      if (es.readyState === 2 || this.sseErrors >= MAX_SSE_ERRORS) {
        this.closeStream();
        this.startPolling();
      }
    };
  }

  private closeStream(): void {
    if (this.es) {
      this.es.onerror = null;
      this.es.close();
      this.es = null;
    }
  }

  private startPolling(): void {
    if (!this.running) return;
    if (this.pollTimer) clearTimeout(this.pollTimer);
    this.pollTimer = null;
    void this.poll();
  }

  private async poll(): Promise<void> {
    if (!this.running) return;
    try {
      if (this.state.cursor === 0) {
        // Find the newest change number, then load everything fresh.
        let after = 0;
        for (;;) {
          const page = await this.o.client.call("events", { query: { after } });
          after = page.last;
          if (page.events.length < PAGE) break;
        }
        this.set({ cursor: after });
        await this.reloadAll();
      } else {
        for (;;) {
          const page = await this.o.client.call("events", { query: { after: this.state.cursor } });
          if (page.reset) {
            await this.reset();
            break;
          }
          if (page.events.length) this.apply(page.events);
          if (page.last > this.state.cursor) this.set({ cursor: page.last });
          if (page.events.length < PAGE) break;
        }
      }
      this.backoff = 0;
      this.set({ connection: "polling", error: undefined });
      this.persist();
    } catch (err) {
      this.fail(err);
      if (err instanceof ApiError && err.unauthorized) return; // the app shows the login screen
      this.backoff = Math.min(this.backoff ? this.backoff * 2 : this.o.pollMs * 2, MAX_BACKOFF_MS);
    }
    if (!this.running) return;
    this.pollTimer = setTimeout(() => void this.poll(), this.backoff || this.o.pollMs);
  }

  // --- changes ------------------------------------------------------------

  private apply(events: ChangeEvent[]): void {
    let cursor = this.state.cursor;
    for (const e of events) {
      if (e.seq > cursor) cursor = e.seq;
      this.o.onEvent?.(e);
      if (e.thread && this.state.threads[e.thread]) {
        const id = e.thread;
        this.debounce(`thread:${id}`, () => this.refreshThread(id).then(() => undefined, () => undefined));
      }
    }
    if (cursor !== this.state.cursor) this.set({ cursor });
    this.debounce("inbox", () => this.refreshInbox());
    this.persist();
  }

  /** Too far behind: forget the cursor and reload everything. */
  private async reset(): Promise<void> {
    this.set({ cursor: 0 });
    if (!this.o.eventSource) return; // polling finds the newest number itself
    await this.reloadAll();
  }

  private async reloadAll(): Promise<void> {
    await this.refreshInbox();
    await Promise.all(this.order.map((id) => this.refreshThread(id).catch(() => undefined)));
  }

  private debounce(key: string, f: () => unknown): void {
    const old = this.timers.get(key);
    if (old) clearTimeout(old);
    this.timers.set(
      key,
      setTimeout(() => {
        this.timers.delete(key);
        if (this.running) void f();
      }, this.o.debounceMs),
    );
  }

  private touch(id: string): void {
    this.order = [id, ...this.order.filter((x) => x !== id)];
  }

  private fail(err: unknown): void {
    if (err instanceof ApiError) {
      this.set({ error: err, connection: err.offline ? "offline" : this.state.connection });
    }
  }

  private persist(): void {
    if (this.persistTimer) return;
    this.persistTimer = setTimeout(() => {
      this.persistTimer = null;
      const s = this.state;
      const saved: Saved = { v: 1, inbox: s.inbox, threads: s.threads, order: this.order, cursor: s.cursor, seen: s.seen };
      void this.o.storage.set(CACHE_KEY, saved).catch(() => undefined);
    }, 0);
  }

  private set(patch: Partial<SyncState>): void {
    this.state = { ...this.state, ...patch };
    this.emit();
  }

  private emit(): void {
    for (const fn of this.listeners) fn(this.state);
  }
}
