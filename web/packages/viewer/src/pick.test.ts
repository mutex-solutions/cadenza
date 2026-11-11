import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderPost, setLazyLoaders, type PostViewer } from "./core.ts";
import { BUILTINS, extraBlocks, md, ref, slotAttachment, templateAttachment, toView } from "./test/fixtures.ts";
import { flush, installIO, options } from "./test/setup.ts";
import type { AttachmentView } from "./types.ts";

const never = () => new Promise<never>(() => undefined);

function press(target: Element, x = 0, y = 0, x2 = x, y2 = y) {
  target.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, cancelable: true, clientX: x, clientY: y }));
  target.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, cancelable: true, clientX: x2, clientY: y2 }));
  target.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, clientX: x2, clientY: y2 }));
}

function box(el: Element, left: number, top: number, width: number, height: number) {
  (el as HTMLElement).getBoundingClientRect = () => ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top, toJSON() {} }) as DOMRect;
}

let el: HTMLElement;
function post(atts: AttachmentView[], o = options()): PostViewer {
  el = document.createElement("div");
  document.body.replaceChildren(el);
  return renderPost(el, atts, o);
}

const bugFix = () => templateAttachment(toView(BUILTINS.find((t) => t.name === "bug-fix")!));

beforeEach(() => {
  installIO(true);
  setLazyLoaders({ chart: never, diagram: never });
});

describe("pickSpot", () => {
  it("shows the hint bar while picking", async () => {
    const v = post([bugFix()]);
    const p = v.pickSpot();
    const root = v.element;
    expect(root.classList.contains("cz-picking")).toBe(true);
    expect(root.querySelector(".cz-pickbar")!.textContent).toContain("Tap the spot you want to comment on · Esc to cancel");
    v.cancelPick();
    expect(await p).toBeNull();
    expect(root.classList.contains("cz-picking")).toBe(false);
  });

  it("picks a code line", async () => {
    const v = post([bugFix()]);
    const p = v.pickSpot();
    press(el.querySelector('.cz-slot[data-slot="proof"] .cz-line[data-line="2"] .cz-line-text')!);
    expect(await p).toEqual({ attachment: 0, block: "proof", line: 2, side: "new", quote: "--- PASS: TestValidateNameEmoji (0.00s)" });
  });

  it("picks removed (old) and added (new) diff lines", async () => {
    const v = post([bugFix()]);
    let p = v.pickSpot();
    press(el.querySelector('.cz-slot[data-slot="fix"] .cz-diff-del')!);
    expect(await p).toEqual({ attachment: 0, block: "fix", line: 12, side: "old", quote: "\tif len(s) > 32 {" });
    p = v.pickSpot();
    press(el.querySelector('.cz-slot[data-slot="fix"] .cz-diff-add .cz-sign')!);
    expect(await p).toEqual({ attachment: 0, block: "fix", line: 12, side: "new", quote: "\tif utf8.RuneCountInString(s) > 32 {" });
  });

  it("picks a point on an image as fractions", async () => {
    const ui = templateAttachment(toView(BUILTINS.find((t) => t.name === "ui-change")!), 1);
    const v = post([bugFix(), ui]);
    await flush();
    const area = el.querySelector('[data-index="1"] .cz-slot[data-slot="after"] .cz-pick-area')!;
    box(area, 100, 50, 200, 300);
    const p = v.pickSpot();
    press(area.querySelector("img")!, 150, 150);
    expect(await p).toEqual({ attachment: 1, block: "after", x: 0.25, y: 0.3333 });
  });

  it("picks an area on an image by dragging", async () => {
    const ui = templateAttachment(toView(BUILTINS.find((t) => t.name === "ui-change")!));
    const v = post([ui]);
    const area = el.querySelector('.cz-slot[data-slot="before"] .cz-pick-area')!;
    box(area, 0, 0, 400, 200);
    const p = v.pickSpot();
    press(area.querySelector("img")!, 300, 150, 100, 50);
    expect(await p).toEqual({ attachment: 0, block: "before", x: 0.25, y: 0.25, w: 0.5, h: 0.5 });
  });

  it("does not open the image while picking", async () => {
    const o = options();
    const v = post([{ index: 0, file: ref("./a.png", "image/webp") }], o);
    await flush();
    await flush();
    const area = el.querySelector(".cz-pick-area")!;
    box(area, 0, 0, 100, 100);
    const p = v.pickSpot();
    press(area.querySelector("img")!, 50, 10);
    // A plain file attachment has no block.
    expect(await p).toEqual({ attachment: 0, x: 0.5, y: 0.1 });
    expect(o.openLink).not.toHaveBeenCalled();
  });

  it("picks a gallery image and says which one", async () => {
    const dr = templateAttachment(toView(BUILTINS.find((t) => t.name === "design-review")!));
    const v = post([dr]);
    const area = el.querySelector('.cz-gallery-item[data-item="2"] .cz-pick-area')!;
    box(area, 0, 0, 100, 100);
    const p = v.pickSpot();
    press(area, 10, 90);
    expect(await p).toEqual({ attachment: 0, block: "mockups", x: 0.1, y: 0.9, quote: "Image 2: B: dense list" });
  });

  it("picks a point on a compare slider", async () => {
    const v = post([templateAttachment(extraBlocks)]);
    const area = el.querySelector(".cz-compare")!;
    box(area, 0, 0, 100, 100);
    const p = v.pickSpot();
    press(area.querySelector(".cz-compare-after img")!, 70, 20);
    expect(await p).toEqual({ attachment: 0, block: "slider", x: 0.7, y: 0.2 });
  });

  it("picks a spot on a webpage through the overlay", async () => {
    const v = post([templateAttachment(extraBlocks)]);
    await flush();
    await flush();
    expect(el.querySelector("iframe")).not.toBeNull();
    const overlay = el.querySelector(".cz-frame-wrap .cz-overlay")!;
    box(overlay.parentElement!, 0, 100, 400, 400);
    const p = v.pickSpot();
    press(overlay, 100, 300);
    expect(await p).toEqual({ attachment: 0, block: "page", x: 0.25, y: 0.5 });
  });

  it("picks the current time of a video", async () => {
    const ui = templateAttachment(toView(BUILTINS.find((t) => t.name === "ui-change")!));
    const v = post([ui]);
    const video = el.querySelector<HTMLVideoElement>('.cz-slot[data-slot="demo"] video')!;
    Object.defineProperty(video, "currentTime", { value: 12.3456, configurable: true });
    const p = v.pickSpot();
    press(video);
    expect(await p).toEqual({ attachment: 0, block: "demo", time_ms: 12346 });
  });

  it("quotes the text of other slots", async () => {
    const v = post([bugFix()]);
    const p = v.pickSpot();
    press(el.querySelector('.cz-slot[data-slot="risks"] li')!);
    expect(await p).toEqual({ attachment: 0, block: "risks", quote: "Names stored before the fix are unchanged" });
  });

  it("quotes at most 200 characters", async () => {
    const v = post([slotAttachment("text", md("x".repeat(500)))]);
    const p = v.pickSpot();
    press(el.querySelector(".cz-md")!);
    expect((await p)!.quote).toHaveLength(200);
  });

  it("cancels with Escape or the Cancel button", async () => {
    const v = post([bugFix()]);
    let p = v.pickSpot();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(await p).toBeNull();
    p = v.pickSpot();
    el.querySelector<HTMLButtonElement>(".cz-pickbar button")!.click();
    expect(await p).toBeNull();
  });

  it("a new pick cancels the previous one", async () => {
    const v = post([bugFix()]);
    const first = v.pickSpot();
    const second = v.pickSpot();
    expect(await first).toBeNull();
    press(el.querySelector('.cz-slot[data-slot="symptom"] .cz-md')!);
    expect((await second)!.block).toBe("symptom");
  });

  it("ignores taps when not picking", () => {
    const v = post([bugFix()]);
    const spy = vi.fn();
    v.element.addEventListener("click", spy);
    press(el.querySelector('.cz-slot[data-slot="symptom"] .cz-md')!);
    expect(spy).toHaveBeenCalled();
  });

  it("destroy resolves a pending pick with null", async () => {
    const v = post([bugFix()]);
    const p = v.pickSpot();
    v.destroy();
    expect(await p).toBeNull();
  });
});

describe("setMarkers", () => {
  it("draws pins for lines, image spots, video times and slots; taps call onMarkerClick", async () => {
    const onMarkerClick = vi.fn();
    const ui = templateAttachment(toView(BUILTINS.find((t) => t.name === "ui-change")!), 1);
    const v = post([bugFix(), ui], options({ onMarkerClick }));
    const video = el.querySelector<HTMLVideoElement>("video")!;
    v.setMarkers([
      { id: "r1", label: "1", spot: { attachment: 0, block: "fix", line: 12, side: "old" } },
      { id: "r2", label: "2", spot: { attachment: 0, block: "proof", line: 1, side: "new" } },
      { id: "r3", label: "3", spot: { attachment: 1, block: "after", x: 0.25, y: 0.5, w: 0.1, h: 0.2 } },
      { id: "r4", label: "4", spot: { attachment: 1, block: "demo", time_ms: 12_000 } },
      { id: "r5", label: "5", spot: { attachment: 0, block: "risks", quote: "Names" } },
      { id: "r6", label: "6", spot: { attachment: 1 } },
      { id: "r7", label: "7", spot: { attachment: 9 } },
    ]);
    const m = (id: string) => el.querySelector<HTMLElement>(`[data-marker="${id}"]`);
    expect(m("r1")!.closest(".cz-line")!.classList.contains("cz-diff-del")).toBe(true);
    expect(m("r2")!.closest(".cz-line")!.getAttribute("data-line")).toBe("1");
    const pin = m("r3")!;
    expect(pin.classList.contains("cz-pin")).toBe(true);
    expect([pin.style.left, pin.style.top]).toEqual(["25%", "50%"]);
    expect(pin.closest('.cz-slot[data-slot="after"]')).not.toBeNull();
    const rect = el.querySelector<HTMLElement>(".cz-pin-rect")!;
    expect([rect.style.width, rect.style.height]).toEqual(["10%", "20%"]);
    expect(m("r4")!.textContent).toBe("4 · 0:12");
    m("r4")!.click();
    expect(video.currentTime).toBe(12);
    expect(m("r5")!.parentElement!.tagName).toBe("H3");
    expect(m("r6")!.parentElement!.getAttribute("data-index")).toBe("1");
    expect(m("r7")).toBeNull();
    m("r1")!.click();
    expect(onMarkerClick).toHaveBeenCalledWith("r1");
    expect(onMarkerClick).toHaveBeenCalledWith("r4");

    // Setting markers again replaces them.
    v.setMarkers([{ id: "x", label: "1", spot: { attachment: 0, block: "risks" } }]);
    expect(el.querySelectorAll(".cz-marker")).toHaveLength(1);
    expect(el.querySelectorAll(".cz-pin-rect, .cz-chips")).toHaveLength(0);
  });

  it("puts gallery pins on the right image", () => {
    const dr = templateAttachment(toView(BUILTINS.find((t) => t.name === "design-review")!));
    const v = post([dr]);
    v.setMarkers([{ id: "g", label: "1", spot: { attachment: 0, block: "mockups", x: 0.5, y: 0.5, quote: "Image 2: B: dense list" } }]);
    expect(el.querySelector('[data-marker="g"]')!.closest(".cz-gallery-item")!.getAttribute("data-item")).toBe("2");
  });
});
