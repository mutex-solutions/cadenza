import { beforeEach, describe, expect, it } from "vitest";
import { WEBPAGE_CSP, renderAttachment, sandboxSrcdoc } from "./core.ts";
import { ref } from "./test/fixtures.ts";
import { flush, installIO, options } from "./test/setup.ts";

const META = `<meta http-equiv="Content-Security-Policy" content="${WEBPAGE_CSP}">`;

beforeEach(() => installIO(true));

describe("WEBPAGE_CSP", () => {
  it("allows no network at all", () => {
    expect(WEBPAGE_CSP).toBe(
      "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:",
    );
  });
});

describe("sandboxSrcdoc", () => {
  const firstInHead = (html: string) => {
    const doc = new DOMParser().parseFromString(sandboxSrcdoc(html), "text/html");
    const first = doc.head.firstElementChild!;
    expect(first.tagName).toBe("META");
    expect(first.getAttribute("http-equiv")).toBe("Content-Security-Policy");
    expect(first.getAttribute("content")).toBe(WEBPAGE_CSP);
    return doc;
  };

  it("puts the CSP first for a full document", () => {
    const html = "<!DOCTYPE html><html><head><title>t</title><script>fetch('https://x')</script></head><body>b</body></html>";
    const out = sandboxSrcdoc(html);
    expect(out.startsWith(`<!DOCTYPE html><html><head>${META}<title>`)).toBe(true);
    const doc = firstInHead(html);
    expect(doc.title).toBe("t");
  });

  it("handles an uppercase <HEAD> with attributes", () => {
    const html = '<HTML lang="en"><HEAD lang=x><TITLE>u</TITLE></HEAD><BODY>b</BODY></HTML>';
    expect(sandboxSrcdoc(html)).toBe(`<HTML lang="en"><HEAD lang=x>${META}<TITLE>u</TITLE></HEAD><BODY>b</BODY></HTML>`);
    const doc = firstInHead(html);
    expect(doc.body.textContent).toBe("b");
  });

  it("wraps a fragment with no html or head", () => {
    const html = "<h1>Report</h1><p>text</p>";
    expect(sandboxSrcdoc(html)).toBe(`<head>${META}</head>` + html);
    const doc = firstInHead(html);
    expect(doc.body.querySelector("h1")!.textContent).toBe("Report");
  });

  it("keeps a doctype (with comments before it) first", () => {
    const html = "<!-- built by agent -->\n<!doctype html>\n<p>x</p>";
    expect(sandboxSrcdoc(html)).toBe(`<!-- built by agent -->\n<!doctype html><head>${META}</head>\n<p>x</p>`);
    firstInHead(html);
  });

  it("can't be fooled by a <head> inside a comment", () => {
    const html = "<html><body><!-- <head> --><script>'<head>'</script></body></html>";
    expect(sandboxSrcdoc(html)).toBe(`<html><head>${META}</head><body><!-- <head> --><script>'<head>'</script></body></html>`);
    firstInHead(html);
  });

  it("puts the CSP first even when a <head> follows a comment, and ignores <header>", () => {
    expect(sandboxSrcdoc("<!-- x --><head><title>t</title></head>")).toBe(`<!-- x --><head>${META}<title>t</title></head>`);
    expect(sandboxSrcdoc("<header>h</header>")).toBe(`<head>${META}</head><header>h</header>`);
  });
});

describe("webpage attachments", () => {
  it("render in a locked sandboxed iframe via srcdoc", async () => {
    const o = options();
    const f = ref("./report.html", "text/html");
    const el = renderAttachment({ index: 0, file: f }, o);
    await flush();
    await flush();
    const frame = el.querySelector("iframe")!;
    expect(frame).not.toBeNull();
    expect(frame.getAttribute("sandbox")).toBe("allow-scripts");
    expect(frame.getAttribute("sandbox")).not.toContain("allow-same-origin");
    expect(frame.getAttribute("referrerpolicy")).toBe("no-referrer");
    expect(frame.hasAttribute("src")).toBe(false);
    expect(frame.getAttribute("title")).toBe("report.html");
    expect(frame.getAttribute("srcdoc")!.startsWith(`<html><head>${META}<title>`)).toBe(true);
    expect(frame.getAttribute("srcdoc")).toContain("<body>hi</body>");
    expect(o.fetchText).toHaveBeenCalledWith(`https://files.test/${f.sha256}`);
  });

  it("show 'unavailable' when the page can't be fetched", async () => {
    const el = renderAttachment({ index: 0, file: ref("./r.html", "text/html") }, options({ fetchText: async () => Promise.reject(new Error("x")) }));
    await flush();
    await flush();
    expect(el.querySelector("iframe")).toBeNull();
    expect(el.querySelector(".cz-unavailable")!.textContent).toMatch(/unavailable/);
  });
});
