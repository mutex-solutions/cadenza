import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderAttachment, renderPost, setLazyLoaders, injectStyles, VERSION } from "./core.ts";
import { BUILTINS, extraBlocks, md, ref, slotAttachment, templateAttachment, toView } from "./test/fixtures.ts";
import { flush, installIO, options } from "./test/setup.ts";

const SELECTOR: Record<string, string> = {
  text: ".cz-md",
  callout: ".cz-callout[role=note]",
  checklist: "ul.cz-checklist li",
  list: "ul.cz-list li .cz-md",
  image: ".cz-image img",
  gallery: ".cz-gallery .cz-gallery-item img",
  compare: ".cz-compare .cz-compare-after img",
  video: "video[controls][playsinline]",
  chart: ".cz-chart pre.cz-source",
  table: "table.cz-table tbody tr",
  metrics: ".cz-metrics .cz-metric",
  code: ".cz-code .cz-line[data-line='1']",
  diff: ".cz-diff .cz-line",
  diagram: ".cz-diagram pre.cz-source",
  file: ".cz-file button",
  webpage: ".cz-webpage .cz-frame-wrap",
  links: ".cz-links a[href]",
  choices: ".cz-choice h4",
};

const never = () => new Promise<never>(() => undefined);

beforeEach(() => {
  document.body.innerHTML = "";
  installIO(true);
  setLazyLoaders({ chart: never, diagram: never });
});

describe("built-in templates", () => {
  it("covers all 15 built-ins", () => {
    expect(BUILTINS).toHaveLength(15);
  });

  for (const t of BUILTINS) {
    it(`renders every slot of the ${t.name} example`, async () => {
      const view = toView(t);
      const el = document.createElement("div");
      document.body.append(el);
      renderPost(el, [templateAttachment(view)], options());
      await flush();
      expect(view.slots.length).toBe(Object.keys(t.example).length);
      for (const s of view.slots) {
        const slot = el.querySelector(`.cz-slot[data-slot="${s.name}"][data-block="${s.block}"]`);
        expect(slot, `${t.name}.${s.name}`).not.toBeNull();
        expect(slot!.querySelector("h3")!.textContent).toBe(s.label);
        expect(slot!.querySelector(SELECTOR[s.block as string]!), `${t.name}.${s.name} (${s.block})`).not.toBeNull();
      }
    });
  }

  it("renders compare, webpage and file blocks", async () => {
    const el = document.createElement("div");
    renderPost(el, [templateAttachment(extraBlocks)], options());
    await flush();
    await flush();
    for (const s of extraBlocks.slots) expect(el.querySelector(`.cz-slot[data-slot="${s.name}"] ${SELECTOR[s.block as string]}`)).not.toBeNull();
    expect(el.querySelector(".cz-compare-block figcaption")!.textContent).toBe("Before vs after");
  });

  it("draws block details: callout tone, checklist state, table numbers, metrics deltas, links", async () => {
    const el = document.createElement("div");
    const inc = BUILTINS.find((t) => t.name === "incident")!;
    const exp = BUILTINS.find((t) => t.name === "experiment")!;
    const plan = BUILTINS.find((t) => t.name === "plan")!;
    const dr = BUILTINS.find((t) => t.name === "data-report")!;
    renderPost(el, [toView(inc), toView(exp), toView(plan), toView(dr)].map((v, i) => templateAttachment(v, i)), options());
    expect(el.querySelector(".cz-callout")!.classList.contains("cz-tone-success")).toBe(true);
    const items = el.querySelectorAll('[data-index="2"] .cz-checklist li');
    expect(Array.from(items).map((li) => li.getAttribute("data-done"))).toEqual(["true", "false", "false"]);
    const metric = el.querySelector('[data-index="1"] .cz-metric')!;
    expect(metric.querySelector(".cz-metric-value")!.textContent).toBe("19 ms");
    expect(metric.querySelector(".cz-delta")!.textContent).toBe("−55%");
    expect(metric.querySelector(".cz-metric-change")!.classList.contains("cz-good")).toBe(true);
    const tds = el.querySelectorAll('[data-index="3"] table tbody tr:first-child td');
    expect(Array.from(tds).map((td) => [td.textContent, td.className])).toEqual([
      ["Free", ""],
      ["9,100", "cz-num"],
      ["4.2", "cz-num"],
    ]);
    // "Churn" went from 3.8% to 3.1%, and lower is good.
    const churn = el.querySelectorAll('[data-index="3"] .cz-metric')[1]!;
    expect(churn.querySelector(".cz-metric-change")!.classList.contains("cz-good")).toBe(true);
  });

  it("opens links through openLink", () => {
    const o = options();
    const research = BUILTINS.find((t) => t.name === "research")!;
    const el = renderAttachment(templateAttachment(toView(research)), o);
    const a = el.querySelector<HTMLAnchorElement>(".cz-links a")!;
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
    expect(a.getAttribute("target")).toBe("_blank");
    a.click();
    expect(o.openLink).toHaveBeenCalledWith("https://modelcontextprotocol.io/specification");
  });
});

describe("XSS safety", () => {
  const evil = "<img src=x onerror=alert(1)><script>alert(2)</script>";
  const noHtml = (el: Element) => {
    expect(el.querySelector("img[onerror], script")).toBeNull();
  };

  const cases: [string, unknown][] = [
    ["gallery", [{ file: ref("./a.png", "image/webp"), caption: evil }]],
    ["compare", { before: { file: ref("./a.png", "image/webp") }, after: { file: ref("./b.png", "image/webp") }, caption: evil }],
    ["table", { columns: [evil], rows: [[evil, 1, null, true]] }],
    ["code", { code: evil, filename: evil, language: evil }],
    ["diff", { diff: `--- ${evil}\n+++ b\n@@ -1 +1 @@\n-${evil}\n+${evil}\n`, filename: evil }],
    ["links", [{ title: evil, url: "https://ok.test" }]],
    ["metrics", [{ label: evil, value: evil, unit: evil, before: evil }]],
    ["choices", [{ title: evil, pros: [evil], cons: [evil] }, { title: "b" }]],
    ["file", { file: { ...ref("./x.bin", "application/octet-stream"), name: evil } }],
    ["chart", { mark: evil }],
    ["diagram", { mermaid: evil }],
    ["unknown-block", { anything: evil }],
  ];
  for (const [block, value] of cases) {
    it(`keeps ${block} text as text`, async () => {
      const el = renderAttachment(slotAttachment(block, value, "s", evil), options());
      document.body.append(el);
      await flush();
      noHtml(el);
      expect(el.textContent).toContain("<img src=x onerror=alert(1)>");
      expect(el.querySelector("h3")!.textContent).toBe(evil);
    });
  }

  it("does not link javascript: or other non-web URLs", () => {
    const el = renderAttachment(slotAttachment("links", [{ title: "bad", url: "javascript:alert(1)" }, { title: "data", url: "data:text/html,x" }]), options());
    expect(el.querySelectorAll("a")).toHaveLength(0);
    expect(el.textContent).toContain("bad");
  });

  it("renders markdown HTML as HTML and locks its links", () => {
    const o = options();
    const el = renderAttachment(
      slotAttachment("text", { md: "x", html: '<p>Hi <strong>there</strong> <a href="https://a.test">a</a> <a href="javascript:alert(1)">b</a></p>' }),
      o,
    );
    expect(el.querySelector(".cz-md strong")!.textContent).toBe("there");
    const [good, bad] = Array.from(el.querySelectorAll<HTMLAnchorElement>(".cz-md a"));
    expect(good!.getAttribute("rel")).toBe("noopener noreferrer");
    expect(good!.getAttribute("target")).toBe("_blank");
    expect(bad!.hasAttribute("href")).toBe(false);
    good!.click();
    expect(o.openLink).toHaveBeenCalledWith("https://a.test");
  });

  it("renders markdown in callouts, lists, checklists and choices", () => {
    const html = { md: "**b**", html: "<p><strong>b</strong></p>" };
    for (const [block, value] of [
      ["callout", { tone: "warning", text: html }],
      ["list", [html]],
      ["checklist", [{ text: html, done: false }]],
      ["choices", [{ title: "a", text: html }, { title: "c" }]],
    ] as [string, unknown][]) {
      const el = renderAttachment(slotAttachment(block, value), options());
      expect(el.querySelector(".cz-md strong"), block).not.toBeNull();
    }
  });

  it("falls back to text when a markdown field has no html", () => {
    const el = renderAttachment(slotAttachment("text", "<b>raw</b>"), options());
    expect(el.querySelector("b")).toBeNull();
    expect(el.textContent).toContain("<b>raw</b>");
  });

  it("ignores an unknown callout tone", () => {
    const el = renderAttachment(slotAttachment("callout", { tone: "x onclick=1", text: md("t") }), options());
    expect(el.querySelector(".cz-callout")!.className).toBe("cz-md cz-callout cz-tone-info");
  });
});

describe("files", () => {
  it("shows the preview first, then the full image; a tap opens the full image", async () => {
    const o = options();
    const f = ref("./a.png", "image/webp");
    const el = renderAttachment({ index: 0, file: f }, o);
    await flush();
    await flush();
    const img = el.querySelector("img")!;
    expect(o.resolveFile).toHaveBeenCalledWith(f.preview_sha256);
    expect(o.resolveFile).toHaveBeenCalledWith(f.sha256);
    expect(img.getAttribute("src")).toBe(`https://files.test/${f.preview_sha256}`);
    expect(img.dataset.full).toBe(`https://files.test/${f.sha256}`);
    expect(img.getAttribute("loading")).toBe("lazy");
    img.click();
    expect(o.openLink).toHaveBeenCalledWith(`https://files.test/${f.sha256}`);
  });

  it("uses the full image when there's no preview", async () => {
    const f = { ...ref("./a.png", "image/webp"), preview_sha256: undefined };
    const el = renderAttachment({ index: 0, file: f }, options());
    await flush();
    expect(el.querySelector("img")!.getAttribute("src")).toBe(`https://files.test/${f.sha256}`);
  });

  it("waits until a file is near the screen before resolving it", async () => {
    installIO(false);
    const o = options();
    const el = renderAttachment({ index: 0, file: ref("./v.mp4", "video/mp4") }, o);
    await flush();
    expect(o.resolveFile).not.toHaveBeenCalled();
    const { FakeIO } = await import("./test/setup.ts");
    FakeIO.fireAll();
    await flush();
    expect(el.querySelector("video")!.getAttribute("src")).toMatch(/^https:\/\/files.test\//);
  });

  it("shows 'unavailable' when a file can't be resolved", async () => {
    const o = options({ resolveFile: vi.fn(async () => Promise.reject(new Error("down"))) });
    for (const f of [ref("./a.png", "image/webp"), ref("./v.mp4", "video/mp4"), ref("./p.html", "text/html")]) {
      const el = renderAttachment({ index: 0, file: f }, o);
      await flush();
      await flush();
      expect(el.querySelector(".cz-unavailable"), f.type).not.toBeNull();
      expect(el.textContent).toContain("unavailable");
    }
  });

  it("file cards open or download through openLink, or say the file is unavailable", async () => {
    const o = options();
    const pdf = renderAttachment({ index: 0, file: ref("./spec.pdf", "application/pdf") }, o);
    const btn = pdf.querySelector("button")!;
    expect(btn.textContent).toBe("Open PDF");
    expect(pdf.textContent).toContain("1.2 KB");
    btn.click();
    await flush();
    expect(o.openLink).toHaveBeenCalledWith(expect.stringMatching(/^https:\/\/files.test\//));
    const other = renderAttachment({ index: 0, file: ref("./x.zip", "application/octet-stream") }, options({ resolveFile: vi.fn(async () => Promise.reject(new Error())) }));
    expect(other.querySelector("button")!.textContent).toBe("Download");
    other.querySelector("button")!.click();
    await flush();
    expect(other.querySelector(".cz-file-status")!.textContent).toMatch(/unavailable/);
  });

  it("says when an attachment has no usable data", () => {
    const el = renderAttachment({ index: 0, template: "x", data: { nope: 1 } }, options());
    expect(el.textContent).toContain("can't be shown");
  });
});

describe("post layout", () => {
  it("shows a counter and moves between attachments", () => {
    const el = document.createElement("div");
    const atts = [0, 1, 2].map((i) => slotAttachment("text", md(`a${i}`), "s", "L", i));
    const scrolled: string[] = [];
    (HTMLElement.prototype as unknown as { scrollIntoView: () => void }).scrollIntoView = function (this: HTMLElement) {
      scrolled.push(this.dataset.index!);
    };
    const v = renderPost(el, atts, options());
    const counter = el.querySelector(".cz-counter")!;
    const [prev, next] = Array.from(el.querySelectorAll<HTMLButtonElement>(".cz-nav button"));
    expect(counter.textContent).toBe("1 / 3");
    expect(prev!.disabled).toBe(true);
    expect(prev!.getAttribute("aria-label")).toBe("Previous attachment");
    next!.click();
    expect(counter.textContent).toBe("2 / 3");
    v.show(2);
    expect(counter.textContent).toBe("3 / 3");
    expect(next!.disabled).toBe(true);
    v.show(99);
    expect(counter.textContent).toBe("3 / 3");
    expect(scrolled).toEqual(["1", "2", "2"]);
    expect(el.querySelector(".cz-post")!.getAttribute("data-count")).toBe("3");
    expect(el.querySelectorAll(".cz-attachment")).toHaveLength(3);
  });

  it("injects styles once and cleans up on destroy", () => {
    const el = document.createElement("div");
    const v = renderPost(el, [slotAttachment("text", md("x"))], options());
    renderPost(document.createElement("div"), [], options());
    injectStyles(document);
    expect(document.querySelectorAll("#cz-viewer-styles")).toHaveLength(1);
    expect(v.element.classList.contains("cz-post")).toBe(true);
    v.destroy();
    expect(el.children).toHaveLength(0);
    expect(VERSION).toMatch(/^\d+\.\d+\.\d+$/);
  });

  it("lets webpages go full screen", async () => {
    const el = renderAttachment({ index: 0, file: ref("./p.html", "text/html") }, options());
    const btn = el.querySelector<HTMLButtonElement>(".cz-fullscreen-btn")!;
    btn.click();
    expect(el.querySelector(".cz-webpage")!.classList.contains("cz-fullscreen")).toBe(true);
    expect(btn.getAttribute("aria-pressed")).toBe("true");
    btn.click();
    expect(el.querySelector(".cz-webpage")!.classList.contains("cz-fullscreen")).toBe(false);
  });

  it("moves the compare slider", () => {
    const el = renderAttachment(templateAttachment(extraBlocks), options());
    const range = el.querySelector<HTMLInputElement>(".cz-compare-range")!;
    expect(range.getAttribute("aria-label")).toMatch(/slide to compare/);
    range.value = "20";
    range.dispatchEvent(new Event("input"));
    expect(el.querySelector<HTMLElement>(".cz-compare-after")!.style.clipPath).toBe("inset(0 0 0 20%)");
  });
});
