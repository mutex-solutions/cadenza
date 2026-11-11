import { vi } from "vitest";
import type { ViewerOptions } from "../types.ts";

/** A controllable IntersectionObserver. `auto` makes every observed element visible at once. */
export class FakeIO {
  static all: FakeIO[] = [];
  static auto = true;
  els: Element[] = [];
  disconnected = false;
  constructor(public cb: IntersectionObserverCallback) {
    FakeIO.all.push(this);
  }
  observe(el: Element) {
    this.els.push(el);
    if (FakeIO.auto) queueMicrotask(() => this.fire());
  }
  disconnect() {
    this.disconnected = true;
  }
  unobserve() {}
  takeRecords() {
    return [];
  }
  fire() {
    if (this.disconnected) return;
    this.cb(this.els.map((target) => ({ target, isIntersecting: true }) as IntersectionObserverEntry), this as unknown as IntersectionObserver);
  }
  static fireAll() {
    for (const io of FakeIO.all) io.fire();
  }
}

export function installIO(auto = true) {
  FakeIO.all = [];
  FakeIO.auto = auto;
  vi.stubGlobal("IntersectionObserver", FakeIO);
  (window as unknown as { IntersectionObserver: unknown }).IntersectionObserver = FakeIO;
}

export function options(over: Partial<ViewerOptions> = {}): ViewerOptions & { openLink: ReturnType<typeof vi.fn>; resolveFile: ReturnType<typeof vi.fn> } {
  return {
    resolveFile: vi.fn(async (sha: string) => `https://files.test/${sha}`),
    fetchText: vi.fn(async () => "<html><head><title>x</title></head><body>hi</body></html>"),
    openLink: vi.fn(),
    ...over,
  } as never;
}

export const flush = () => new Promise((r) => setTimeout(r, 0));
