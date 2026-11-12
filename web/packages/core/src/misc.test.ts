import "fake-indexeddb/auto";
import { describe, expect, it, vi } from "vitest";
import { agentLabel, clock, fileSize, postLabel, statusLabel, timeAgo } from "./format.ts";
import { browserImageConverter, fitWithin, withExtension } from "./image.ts";
import { indexedDBStorage, memoryStorage } from "./storage.ts";

describe("format", () => {
  it("labels statuses and agents in plain words", () => {
    expect(statusLabel("changes_requested")).toBe("Changes requested");
    expect(statusLabel("ready")).toBe("Ready to review");
    expect(statusLabel("weird")).toBe("weird");
    expect(agentLabel("claude-code")).toBe("Claude Code");
    expect(agentLabel("")).toBe("Agent");
    expect(agentLabel("aider")).toBe("aider");
  });
  it("formats times and sizes", () => {
    const now = 10_000_000_000;
    expect(timeAgo(now - 10_000, now)).toBe("now");
    expect(timeAgo(now - 5 * 60_000, now)).toBe("5 min");
    expect(timeAgo(now - 3 * 3600_000, now)).toBe("3 h");
    expect(timeAgo(now - 2 * 86400_000, now)).toBe("2 d");
    expect(timeAgo(0, now)).toBe("1970-01-01");
    expect(fileSize(500)).toBe("500 B");
    expect(fileSize(1536)).toBe("1.5 KB");
    expect(fileSize(50 * 1024 * 1024)).toBe("50 MB");
    expect(clock(65_400)).toBe("1:05");
  });
  it("names posts without doubling Round", () => {
    expect(postLabel(2, "layout")).toBe("Round 2 · layout");
    expect(postLabel(2, "Round 2: layout")).toBe("Round 2: layout");
    expect(postLabel(3, "round 3 fixes")).toBe("round 3 fixes");
    expect(postLabel(4, "Roundup of changes")).toBe("Round 4 · Roundup of changes");
    expect(postLabel(5, "  ")).toBe("Round 5");
  });
});

describe("image", () => {
  it("fits sizes without scaling up", () => {
    expect(fitWithin(5120, 2560, 2560)).toEqual({ width: 2560, height: 1280 });
    expect(fitWithin(100, 50, 640)).toEqual({ width: 100, height: 50 });
    expect(fitWithin(1000, 4000, 640)).toEqual({ width: 160, height: 640 });
  });
  it("swaps extensions", () => {
    expect(withExtension("IMG_1.HEIC", "webp")).toBe("IMG_1.webp");
    expect(withExtension("noext", "webp")).toBe("noext.webp");
    expect(withExtension(".jpg", "webp")).toBe("photo.webp");
  });
  it("browser converter makes a 2560 px image and a 640 px preview at quality 0.8", async () => {
    const close = vi.fn();
    const encode = vi.fn(async (_s: unknown, w: number, h: number, type: string) => new Blob([`${w}x${h}`], { type }));
    const conv = browserImageConverter({ decode: async () => ({ width: 4000, height: 3000, close }), encode });
    const out = await conv.toWebP(new Blob(["jpeg"]));
    expect(out.type).toBe("image/webp");
    expect([out.width, out.height]).toEqual([2560, 1920]);
    expect(await out.full.text()).toBe("2560x1920");
    expect(await out.preview.text()).toBe("640x480");
    expect(encode.mock.calls[0]!.slice(1)).toEqual([2560, 1920, "image/webp", 0.8]);
    expect(close).toHaveBeenCalled();
  });
  it("makes JPEG instead when the browser can't make WebP (it returns PNG)", async () => {
    // Like older Safari: asked for WebP, it encodes PNG; JPEG works.
    const encode = vi.fn(async (_s: unknown, w: number, h: number, type: string, _q: number) =>
      new Blob([`${w}x${h}`], { type: type === "image/webp" ? "image/png" : type }),
    );
    const conv = browserImageConverter({ decode: async () => ({ width: 4000, height: 3000 }), encode });
    const out = await conv.toWebP(new Blob(["x"]));
    expect(out.type).toBe("image/jpeg");
    expect(out.full.type).toBe("image/jpeg");
    expect(out.preview.type).toBe("image/jpeg");
    expect(await out.preview.text()).toBe("640x480");
    expect(encode.mock.calls.map((c) => c[3])).toEqual(["image/webp", "image/jpeg", "image/jpeg"]);
    expect(encode.mock.calls[1]![4]).toBe(0.85);
  });
});

describe("storage", () => {
  it("memory storage copies values", async () => {
    const s = memoryStorage();
    const v = { a: [1] };
    await s.set("k", v);
    v.a.push(2);
    expect(await s.get("k")).toEqual({ a: [1] });
    await s.delete("k");
    expect(await s.get("k")).toBeUndefined();
  });
  it("IndexedDB storage round-trips", async () => {
    const s = indexedDBStorage("test-db");
    await s.set("x", { n: 1 });
    expect(await s.get("x")).toEqual({ n: 1 });
    // A second instance sees the same data.
    expect(await indexedDBStorage("test-db").get("x")).toEqual({ n: 1 });
    await s.delete("x");
    expect(await s.get("x")).toBeUndefined();
  });
  it("falls back to memory without IndexedDB", async () => {
    const s = indexedDBStorage("none", null);
    await s.set("x", 1);
    expect(await s.get("x")).toBe(1);
  });
});
