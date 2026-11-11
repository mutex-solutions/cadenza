import { describe, expect, it, vi } from "vitest";
import { FileUrlCache, URL_MARGIN_MS } from "./fileUrls";
import { inboxRows, replyBarLabel } from "./inbox";
import { navReducer, threadFromNotification, type Screen } from "./nav";
import { photoName, photoSizes, preparePhoto } from "./photo";
import { createSessionStore, DEVICE_KEY, SERVER_KEY, TOKEN_KEY, type SecureStoreLike } from "./session";
import { htmlToText } from "./text";
import { checkServerUrl, isPrivateHost } from "./url";

describe("checkServerUrl", () => {
  it("accepts https and adds it when missing", () => {
    expect(checkServerUrl(" cz.example.com/ ")).toEqual({ ok: true, url: "https://cz.example.com" });
    expect(checkServerUrl("https://cz.example.com:8443/base/")).toEqual({ ok: true, url: "https://cz.example.com:8443/base" });
  });
  it("allows http only on private networks, with a warning", () => {
    for (const u of ["http://localhost:8080", "http://192.168.1.5:8080", "http://10.0.0.2", "http://100.101.1.2", "http://box.tail1234.ts.net", "http://172.20.0.1"]) {
      const r = checkServerUrl(u);
      expect(r.ok, u).toBe(true);
      expect(r.ok && r.warning).toMatch(/isn't encrypted/);
    }
    const bad = checkServerUrl("http://cz.example.com");
    expect(bad.ok).toBe(false);
  });
  it("rejects junk", () => {
    expect(checkServerUrl("").ok).toBe(false);
    expect(checkServerUrl("ftp://x").ok).toBe(false);
    expect(checkServerUrl("https://").ok).toBe(false);
  });
  it("knows private hosts", () => {
    expect(isPrivateHost("100.63.0.1")).toBe(false);
    expect(isPrivateHost("172.32.0.1")).toBe(false);
    expect(isPrivateHost("8.8.8.8")).toBe(false);
    expect(isPrivateHost("[::1]")).toBe(true);
  });
});

describe("FileUrlCache", () => {
  it("reuses links until shortly before they expire and shares requests in flight", async () => {
    let now = 0;
    const fetchUrl = vi.fn(async (sha: string) => ({ url: `https://s3/${sha}?n=${fetchUrl.mock.calls.length}`, expires: now + 10 * 60_000 }));
    const c = new FileUrlCache(fetchUrl, () => now);
    const [a, b] = await Promise.all([c.get("x"), c.get("x")]);
    expect(a).toBe(b);
    expect(fetchUrl).toHaveBeenCalledTimes(1);
    now = 10 * 60_000 - URL_MARGIN_MS - 1;
    expect(await c.get("x")).toBe(a);
    now += 2;
    expect(await c.get("x")).not.toBe(a);
    expect(fetchUrl).toHaveBeenCalledTimes(2);
  });
  it("does not cache failures", async () => {
    const fetchUrl = vi.fn().mockRejectedValueOnce(new Error("down")).mockResolvedValueOnce({ url: "u", expires: Date.now() + 3600_000 });
    const c = new FileUrlCache(fetchUrl);
    await expect(c.get("x")).rejects.toThrow("down");
    expect(await c.get("x")).toBe("u");
  });
});

describe("htmlToText", () => {
  it("keeps text, breaks blocks and decodes entities", () => {
    expect(htmlToText("<p>Keep <strong>promo</strong> &amp; total?</p><ul><li>a</li><li>b</li></ul>")).toBe("Keep promo & total?\n• a\n• b");
    expect(htmlToText("x<br>y &#128512; &#x41; &lt;b&gt;")).toBe("x\ny 😀 A <b>");
  });
  it("drops scripts and styles and leaves unknown entities alone", () => {
    expect(htmlToText("<script>alert(1)</script><style>p{}</style>ok &bogus; &#0;")).toBe("ok &bogus; &#0;");
  });
});

describe("photo", () => {
  it("sizes full images to 2560 px and previews to 640 px", () => {
    expect(photoSizes(4032, 3024)).toEqual({ full: { width: 2560, height: 1920 }, preview: { width: 640, height: 480 } });
    expect(photoSizes(500, 400)).toEqual({ full: { width: 500, height: 400 }, preview: { width: 500, height: 400 } });
  });
  it("names photos .webp and strips unsafe characters", () => {
    expect(photoName({ uri: "u", width: 1, height: 1, fileName: "IMG_2001.JPG" }, 0)).toBe("IMG_2001.webp");
    expect(photoName({ uri: "u", width: 1, height: 1, fileName: "a/b:c.png" }, 0)).toBe("a_b_c.webp");
    expect(photoName({ uri: "u", width: 1, height: 1 }, 1, 42)).toBe("photo-42-2.webp");
  });
  it("converts to WebP at quality 0.8 with a preview, ready for upload", async () => {
    const toWebP = vi.fn(async (uri: string, width: number, height: number) => ({ uri: `${uri}-${width}`, width, height }));
    const readBlob = vi.fn(async (uri: string) => new Blob([uri]));
    const item = await preparePhoto({ uri: "file:///p.jpg", width: 3000, height: 3000, fileName: "p.jpg" }, 0, { toWebP, readBlob });
    expect(toWebP.mock.calls).toEqual([
      ["file:///p.jpg", 2560, 2560, 0.8],
      ["file:///p.jpg", 640, 640, 0.8],
    ]);
    expect(item.name).toBe("p.webp");
    expect(item.type).toBe("image/webp");
    expect(await item.data.text()).toBe("file:///p.jpg-2560");
    expect(await item.preview!.text()).toBe("file:///p.jpg-640");
  });
});

describe("navigation", () => {
  const inbox: Screen = { name: "inbox" };
  it("pushes, pops (never the last screen) and resets", () => {
    let s = navReducer([inbox], { type: "push", screen: { name: "settings" } });
    expect(s).toEqual([inbox, { name: "settings" }]);
    s = navReducer(s, { type: "pop" });
    expect(s).toEqual([inbox]);
    expect(navReducer(s, { type: "pop" })).toBe(s);
    expect(navReducer(s, { type: "reset", screen: { name: "connect" } })).toEqual([{ name: "connect" }]);
  });
  it("opens a thread from a notification over the inbox, once", () => {
    const s = navReducer([inbox, { name: "settings" }], { type: "openThread", id: "t_1" });
    expect(s).toEqual([inbox, { name: "thread", id: "t_1" }]);
    expect(navReducer(s, { type: "openThread", id: "t_1" })).toBe(s);
  });
  it("finds the thread in notification data", () => {
    expect(threadFromNotification({ url: "/t/t_81" })).toBe("t_81");
    expect(threadFromNotification({ url: "https://cz.example.com/t/t_81?x=1" })).toBe("t_81");
    expect(threadFromNotification({ thread_id: "t_9" })).toBe("t_9");
    expect(threadFromNotification({ url: "/machines" })).toBeNull();
    expect(threadFromNotification({ thread_id: "../x" })).toBeNull();
    expect(threadFromNotification(null)).toBeNull();
    expect(threadFromNotification("t_1")).toBeNull();
  });
});

describe("session store", () => {
  function fake(): SecureStoreLike & { m: Map<string, string> } {
    const m = new Map<string, string>();
    return {
      m,
      getItemAsync: async (k) => m.get(k) ?? null,
      setItemAsync: async (k, v) => void m.set(k, v),
      deleteItemAsync: async (k) => void m.delete(k),
    };
  }
  it("saves, loads and clears the login but keeps the server address", async () => {
    const f = fake();
    const s = createSessionStore(f);
    expect(await s.load()).toBeNull();
    await s.save({ serverUrl: "https://a", token: "tok" });
    await s.setDeviceId("d_1");
    expect(f.m.get(TOKEN_KEY)).toBe("tok");
    expect(TOKEN_KEY).toBe("cadenza.token");
    expect(await s.load()).toEqual({ serverUrl: "https://a", token: "tok" });
    expect(await s.deviceId()).toBe("d_1");
    await s.clear();
    expect(await s.load()).toBeNull();
    expect(f.m.has(DEVICE_KEY)).toBe(false);
    expect(f.m.get(SERVER_KEY)).toBe("https://a");
    expect(await s.lastServer()).toBe("https://a");
  });
});

describe("inbox rows", () => {
  const t = (id: string, updated: number, q = 0) => ({ id, title: id, status: "ready", machine: "vm-03", agent: "claude-code", posts: 1, open_questions: q, updated_at: updated });
  it("puts open questions first, then newest, with labels", () => {
    const rows = inboxRows([t("old", 1), t("new", 5), t("ask", 2, 2)], { new: 5 }, 10_000);
    expect(rows.map((r) => r.id)).toEqual(["ask", "new", "old"]);
    expect(rows[0]).toMatchObject({ subtitle: "Claude Code · vm-03", status: "Ready to review", questions: 2, unread: true, time: "now" });
    expect(rows[1]!.unread).toBe(false);
  });
  it("labels the reply bar", () => {
    expect(replyBarLabel(0)).toBe("Reply");
    expect(replyBarLabel(1)).toBe("1 question · Reply");
    expect(replyBarLabel(3)).toBe("3 questions · Reply");
  });
});
