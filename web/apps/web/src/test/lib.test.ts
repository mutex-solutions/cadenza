import { describe, expect, it, vi } from "vitest";
import { createClient, type PostView } from "@cadenza/core";
import { match, threadPath } from "../app/router.tsx";
import { markersFor } from "../components/PostCard.tsx";
import { withSafeLinks } from "../components/Html.tsx";
import { parseLabels } from "../components/Sketch.tsx";
import { fileResolver, toUploadItems } from "../lib/files.ts";
import { deviceName, pushSupport, urlBase64ToUint8Array } from "../lib/push.ts";
import { describeSpot } from "../lib/spot.ts";
import { fakeConverter, fakeFetch } from "./helpers.tsx";

describe("router", () => {
  it("matches every screen", () => {
    expect(match("/")).toEqual({ name: "inbox" });
    expect(match("/archived/")).toEqual({ name: "archived" });
    expect(match("/t/t_81")).toEqual({ name: "thread", id: "t_81" });
    expect(match("/t/a%20b")).toEqual({ name: "thread", id: "a b" });
    expect(match("/t/%E0%A4%A")).toEqual({ name: "notfound", path: "/t/%E0%A4%A" });
    expect(match("/machines")).toEqual({ name: "machines" });
    expect(match("/settings")).toEqual({ name: "settings" });
    expect(match("/t/a/b")).toEqual({ name: "notfound", path: "/t/a/b" });
    expect(threadPath("a b")).toBe("/t/a%20b");
  });
});

describe("fileResolver", () => {
  it("reuses signed links until shortly before they expire, and shares in-flight requests", async () => {
    let now = 1_000_000;
    let n = 0;
    const f = fakeFetch({ "GET /v1/files/abc": () => ({ body: { url: `https://s3/abc?sig=${++n}`, expires: now + 60_000 } }) });
    const resolve = fileResolver(createClient({ fetch: f.fetch }), () => now);
    const [a, b] = await Promise.all([resolve("abc"), resolve("abc")]);
    expect(a).toBe("https://s3/abc?sig=1");
    expect(b).toBe(a);
    expect(await resolve("abc")).toBe(a);
    now += 45_000; // within 30 s of expiry
    expect(await resolve("abc")).toBe("https://s3/abc?sig=2");
    expect(f.calls).toHaveLength(2);
  });
});

describe("toUploadItems", () => {
  it("converts photos to WebP with a preview and keeps other allowed files", async () => {
    const items = await toUploadItems(
      [new File(["j"], "IMG_1.HEIC", { type: "image/heic" }), new File(["p"], "spec.pdf", { type: "application/pdf" }), new File(["z"], "logs.zip", { type: "application/zip" })],
      fakeConverter,
    );
    expect(items.map((i) => [i.name, i.type, !!i.preview])).toEqual([
      ["IMG_1.webp", "image/webp", true],
      ["spec.pdf", "application/pdf", false],
      ["logs.zip", "application/octet-stream", false],
    ]);
  });
  it("sends JPEG with a preview when the browser can't make WebP", async () => {
    const jpeg = { toWebP: vi.fn(async (b: Blob) => ({ full: b, preview: b, type: "image/jpeg", width: 1, height: 1 })) };
    const items = await toUploadItems([new File(["x"], "IMG_2.HEIC", { type: "image/heic" })], jpeg);
    expect(items.map((i) => [i.name, i.type, !!i.preview])).toEqual([["IMG_2.jpg", "image/jpeg", true]]);
  });
  it("sends the original photo as an image when conversion fails", async () => {
    const broken = { toWebP: vi.fn(async () => Promise.reject(new Error("decode"))) };
    expect((await toUploadItems([new File(["x"], "b.jpg", { type: "image/jpeg" })], broken))[0]!.type).toBe("image/jpeg");
    expect((await toUploadItems([new File(["x"], "c.png", { type: "image/png" })], broken))[0]!.type).toBe("image/png");
    expect((await toUploadItems([new File(["x"], "d.heic", { type: "image/heic" })], broken))[0]!.type).toBe("application/octet-stream");
  });
});

describe("push helpers", () => {
  it("decodes base64url keys", () => {
    expect(Array.from(urlBase64ToUint8Array("BAEC"))).toEqual([4, 1, 2]);
    expect(Array.from(urlBase64ToUint8Array("-_8"))).toEqual([251, 255]);
  });
  it("names devices plainly", () => {
    expect(deviceName("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Version/17.0 Mobile/15E148 Safari/604.1")).toBe("Safari on iPhone");
    expect(deviceName("Mozilla/5.0 (Windows NT 10.0) Chrome/130 Safari/537 Edg/130")).toBe("Edge on Windows");
    expect(deviceName("curl")).toBe("Browser");
  });
  it("tells iPhone users to install first", () => {
    const nav = { userAgent: "iPhone", platform: "iPhone", maxTouchPoints: 5 } as unknown as Navigator;
    const win = { matchMedia: () => ({ matches: false }) } as unknown as Window;
    expect(pushSupport(nav, win)).toBe("ios-needs-install");
    const plain = { userAgent: "Firefox", platform: "Linux", maxTouchPoints: 0 } as unknown as Navigator;
    expect(pushSupport(plain, {} as Window)).toBe("unsupported");
  });
});

describe("spots and markers", () => {
  const post = {
    id: "p",
    seq: 1,
    title: "x",
    created_at: 0,
    attachments: [
      { index: 0, template: "bug-fix", data: { slots: [{ name: "fix", label: "Fix" }, { name: "demo", label: "Demo" }] } },
      { index: 1, file: { sha256: "a", size: 1, type: "image/webp", name: "shot.webp" } },
    ],
  } as PostView;
  it("describes spots in plain words", () => {
    expect(describeSpot({ attachment: 0, block: "fix", line: 12, side: "old" }, post)).toBe("line 12 (old) of Fix");
    expect(describeSpot({ attachment: 0, block: "demo", time_ms: 12_000 }, post)).toBe("0:12 in Demo");
    expect(describeSpot({ attachment: 1, x: 0.25, y: 0.5 }, post)).toBe("spot at 25% across, 50% down in shot.webp");
    expect(describeSpot({ attachment: 1, x: 0.25, y: 0.5, w: 0.1, h: 0.1 }, post)).toBe("area at 25% across, 50% down in shot.webp");
    expect(describeSpot({ attachment: 0, block: "other", quote: "a".repeat(50) })).toBe(`“${"a".repeat(40)}…” in other`);
    expect(describeSpot({ attachment: 3 })).toBe("attachment 4");
  });
  it("numbers pinned comments in order", () => {
    const base = { thread: "t", created_at: 0, delivered: false };
    const m = markersFor([
      { ...base, id: "a", kind: "note" },
      { ...base, id: "b", kind: "comment", spot: { attachment: 0 } },
      { ...base, id: "c", kind: "comment", spot: { attachment: 1 } },
    ]);
    expect(m.map((x) => [x.id, x.label])).toEqual([
      ["b", "1"],
      ["c", "2"],
    ]);
  });
  it("opens markdown links in a new tab without the opener", () => {
    expect(withSafeLinks('<p><a href="https://x">x</a></p>')).toBe('<p><a target="_blank" rel="noopener noreferrer" href="https://x">x</a></p>');
  });
  it("parses sketch labels", () => {
    expect(parseLabels(" login button, error text\n\n,total ")).toEqual(["login button", "error text", "total"]);
  });
});
