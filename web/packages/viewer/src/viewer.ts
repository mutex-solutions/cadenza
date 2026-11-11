import { clock } from "@cadenza/core";
import { renderBlock } from "./blocks.ts";
import { h } from "./dom.ts";
import { plainFile } from "./media.ts";
import { injectStyles } from "./styles.ts";
import type { AttachmentView, Ctx, Marker, PostViewer, Spot, View, ViewerOptions, ViewSlot } from "./types.ts";

export const VERSION = "0.1.0";

function isView(v: unknown): v is View {
  return typeof v === "object" && v !== null && Array.isArray((v as View).slots);
}

function drawAttachment(ctx: Ctx, att: AttachmentView): HTMLElement {
  const d = ctx.doc;
  const section = h(d, "section", { class: "cz-attachment", attrs: { "data-index": String(att.index ?? ctx.index) } });
  if (att.file) {
    section.dataset.file = att.file.type;
    section.append(plainFile(ctx, att.file));
    return section;
  }
  const view = att.data;
  if (!isView(view)) {
    section.append(h(d, "p", { class: "cz-unavailable", text: "This attachment can't be shown." }));
    return section;
  }
  section.dataset.template = view.template;
  for (const slot of view.slots as ViewSlot[]) {
    const el = h(d, "div", { class: "cz-slot", attrs: { "data-slot": slot.name, "data-block": String(slot.block) } });
    el.append(h(d, "h3", { text: slot.label || slot.name }), renderBlock(ctx, slot));
    section.append(el);
  }
  return section;
}

/** Draws one attachment on its own (no carousel, no spot picking). */
export function renderAttachment(att: AttachmentView, opts: ViewerOptions): HTMLElement {
  const doc = opts.document ?? document;
  const ctx: Ctx = { opts, doc, index: att.index ?? 0, cleanups: [] };
  const el = drawAttachment(ctx, att);
  el.classList.add("cz-root");
  return el;
}

const round4 = (n: number) => Math.round(Math.min(1, Math.max(0, n)) * 10000) / 10000;

/**
 * Draws a post's attachments into `container`: stacked on wide screens, a
 * swipeable carousel on narrow ones. Also picks spots for pinned comments and
 * draws their markers.
 */
export function renderPost(container: HTMLElement, attachments: AttachmentView[], opts: ViewerOptions): PostViewer {
  const doc = opts.document ?? container.ownerDocument;
  injectStyles(doc);
  const cleanups: (() => void)[] = [];
  const root = h(doc, "div", { class: "cz-root cz-post", attrs: { "data-count": String(attachments.length) } });

  // Spot picking bar.
  const cancelBtn = h(doc, "button", { text: "Cancel", attrs: { type: "button" } });
  const pickbar = h(doc, "div", { class: "cz-pickbar", attrs: { role: "status" } }, h(doc, "span", { text: "Tap the spot you want to comment on · Esc to cancel" }), cancelBtn);

  // Carousel controls (shown on narrow screens only, by CSS).
  const prev = h(doc, "button", { text: "‹", attrs: { type: "button", "aria-label": "Previous attachment" } });
  const next = h(doc, "button", { text: "›", attrs: { type: "button", "aria-label": "Next attachment" } });
  const counter = h(doc, "span", { class: "cz-counter", attrs: { "aria-live": "polite" } });
  const nav = h(doc, "div", { class: "cz-nav" }, prev, counter, next);
  const track = h(doc, "div", { class: "cz-track" });
  root.append(pickbar, nav, track);

  const sections = attachments.map((att, i) => {
    const ctx: Ctx = { opts, doc, index: att.index ?? i, cleanups };
    const s = drawAttachment(ctx, att);
    s.setAttribute("aria-label", `Attachment ${i + 1} of ${attachments.length}`);
    track.append(s);
    return s;
  });

  let current = 0;
  const setCurrent = (i: number) => {
    current = Math.max(0, Math.min(sections.length - 1, i));
    counter.textContent = `${current + 1} / ${sections.length}`;
    prev.disabled = current === 0;
    next.disabled = current >= sections.length - 1;
  };
  setCurrent(0);
  const show = (i: number) => {
    setCurrent(i);
    const s = sections[current];
    if (s && typeof s.scrollIntoView === "function") s.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "start" });
  };
  prev.addEventListener("click", () => show(current - 1));
  next.addEventListener("click", () => show(current + 1));
  const onScroll = () => {
    const w = track.clientWidth;
    if (w > 0 && track.scrollWidth > w) setCurrent(Math.round(track.scrollLeft / w));
  };
  track.addEventListener("scroll", onScroll, { passive: true });

  container.replaceChildren(root);

  // --- spot picking -------------------------------------------------------
  let pending: ((s: Spot | null) => void) | null = null;
  let start: { x: number; y: number; target: Element } | null = null;
  // The click that follows the pointerup that finished a pick must not act (e.g. open the image).
  let swallowClick = false;

  const finish = (s: Spot | null) => {
    const p = pending;
    pending = null;
    start = null;
    root.classList.remove("cz-picking");
    p?.(s);
  };

  const attachmentOf = (el: Element): number | undefined => {
    const a = el.closest<HTMLElement>(".cz-attachment");
    return a ? Number(a.dataset.index) : undefined;
  };

  const spotFor = (target: Element, x: number, y: number, s: { x: number; y: number }): Spot | null => {
    const attachment = attachmentOf(target);
    if (attachment === undefined) return null;
    const spot: Spot = { attachment };
    const slot = target.closest<HTMLElement>(".cz-slot");
    if (slot?.dataset.slot) spot.block = slot.dataset.slot;
    const scope: Element = slot ?? target.closest(".cz-attachment")!;

    const line = target.closest<HTMLElement>(".cz-line");
    if (line?.dataset.line && line.dataset.line !== "undefined") {
      spot.line = Number(line.dataset.line);
      spot.side = line.dataset.side ?? "new";
      spot.quote = (line.querySelector(".cz-line-text")?.textContent ?? "").slice(0, 200);
      return spot;
    }
    const area = target.closest<HTMLElement>(".cz-pick-area");
    if (area) {
      const r = area.getBoundingClientRect();
      if (r.width > 0 && r.height > 0) {
        const fx = (n: number) => (n - r.left) / r.width;
        const fy = (n: number) => (n - r.top) / r.height;
        if (Math.hypot(x - s.x, y - s.y) > 8) {
          const x0 = round4(fx(Math.min(x, s.x)));
          const y0 = round4(fy(Math.min(y, s.y)));
          spot.x = x0;
          spot.y = y0;
          spot.w = round4(fx(Math.max(x, s.x)) - x0);
          spot.h = round4(fy(Math.max(y, s.y)) - y0);
        } else {
          spot.x = round4(fx(x));
          spot.y = round4(fy(y));
        }
        const item = area.closest<HTMLElement>(".cz-gallery-item");
        if (item) {
          const cap = item.querySelector("figcaption")?.textContent;
          spot.quote = `Image ${item.dataset.item}${cap ? `: ${cap}` : ""}`;
        }
        return spot;
      }
    }
    const v = scope.querySelector("video");
    if (v) {
      spot.time_ms = Math.round((v.currentTime || 0) * 1000);
      return spot;
    }
    if (slot) {
      const sel = doc.getSelection?.();
      const selected = sel && !sel.isCollapsed && sel.anchorNode && slot.contains(sel.anchorNode) ? sel.toString().trim() : "";
      const text = selected || (slot.querySelector(":scope > :not(h3)")?.textContent ?? "").replace(/\s+/g, " ").trim();
      if (text) spot.quote = text.slice(0, 200);
    }
    return spot;
  };

  const point = (e: Event) => {
    const m = e as MouseEvent;
    return { x: m.clientX ?? 0, y: m.clientY ?? 0 };
  };
  const onDown = (e: Event) => {
    swallowClick = false;
    if (!pending || pickbar.contains(e.target as Node)) return;
    const p = point(e);
    start = { ...p, target: e.target as Element };
    e.preventDefault();
  };
  const onUp = (e: Event) => {
    if (!pending || !start || pickbar.contains(e.target as Node)) return;
    e.preventDefault();
    e.stopPropagation();
    const p = point(e);
    const spot = spotFor(start.target, p.x, p.y, start);
    if (spot) {
      swallowClick = true;
      finish(spot);
    }
    else start = null;
  };
  const swallow = (e: Event) => {
    if ((pending || swallowClick) && !pickbar.contains(e.target as Node)) {
      swallowClick = false;
      e.preventDefault();
      e.stopPropagation();
    }
  };
  const onKey = (e: KeyboardEvent) => {
    if (pending && e.key === "Escape") finish(null);
  };
  root.addEventListener("pointerdown", onDown, true);
  root.addEventListener("pointerup", onUp, true);
  root.addEventListener("click", swallow, true);
  doc.addEventListener("keydown", onKey);
  cancelBtn.addEventListener("click", () => finish(null));

  // --- markers --------------------------------------------------------------
  const markerEl = (m: Marker, extra = "") => {
    const b = h(doc, "button", { class: `cz-marker ${extra}`.trim(), text: m.label, attrs: { type: "button", "aria-label": `Comment ${m.label}`, "data-marker": m.id } });
    b.addEventListener("click", (e) => {
      e.stopPropagation();
      opts.onMarkerClick?.(m.id);
    });
    return b;
  };

  const setMarkers = (markers: Marker[]) => {
    for (const el of Array.from(root.querySelectorAll(".cz-marker, .cz-pin-rect, .cz-chips"))) el.remove();
    for (const m of markers) {
      const s = m.spot;
      const section = sections.find((x) => Number(x.dataset.index) === s.attachment);
      if (!section) continue;
      const slot = s.block ? section.querySelector<HTMLElement>(`.cz-slot[data-slot="${cssEscape(s.block)}"]`) : null;
      const scope: HTMLElement = slot ?? section;
      if (s.line !== undefined) {
        const row =
          scope.querySelector(`.cz-line[data-line="${s.line}"][data-side="${s.side === "old" ? "old" : "new"}"]`) ?? scope.querySelector(`.cz-line[data-line="${s.line}"]`);
        const cell = row?.querySelector(".cz-line-text");
        if (cell) {
          cell.append(markerEl(m));
          continue;
        }
      }
      if (s.x !== undefined && s.y !== undefined) {
        let area = scope.querySelector<HTMLElement>(".cz-pick-area");
        const item = /^Image (\d+)/.exec(s.quote ?? "");
        if (item) area = scope.querySelector<HTMLElement>(`.cz-gallery-item[data-item="${item[1]}"] .cz-pick-area`) ?? area;
        if (area) {
          if (s.w !== undefined && s.h !== undefined) {
            const rect = h(doc, "div", { class: "cz-pin-rect" });
            Object.assign(rect.style, { left: `${s.x * 100}%`, top: `${s.y * 100}%`, width: `${s.w * 100}%`, height: `${s.h * 100}%` });
            area.append(rect);
          }
          const pin = markerEl(m, "cz-pin");
          Object.assign(pin.style, { left: `${s.x * 100}%`, top: `${s.y * 100}%` });
          area.append(pin);
          continue;
        }
      }
      if (s.time_ms !== undefined) {
        const v = scope.querySelector("video");
        const wrap = v?.closest(".cz-video-wrap");
        if (v && wrap) {
          let chips = wrap.nextElementSibling;
          if (!chips?.classList.contains("cz-chips")) {
            chips = h(doc, "div", { class: "cz-chips" });
            wrap.after(chips);
          }
          const chip = markerEl(m);
          chip.textContent = `${m.label} · ${clock(s.time_ms)}`;
          chip.addEventListener("click", () => {
            v.currentTime = (s.time_ms ?? 0) / 1000;
          });
          chips.append(chip);
          continue;
        }
      }
      const label = slot?.querySelector(":scope > h3");
      if (label) label.append(markerEl(m));
      else section.prepend(markerEl(m));
    }
  };

  return {
    element: root,
    pickSpot() {
      if (pending) finish(null);
      root.classList.add("cz-picking");
      return new Promise<Spot | null>((resolve) => {
        pending = resolve;
      });
    },
    cancelPick() {
      if (pending) finish(null);
    },
    setMarkers,
    show,
    destroy() {
      if (pending) finish(null);
      for (const c of cleanups) c();
      cleanups.length = 0;
      track.removeEventListener("scroll", onScroll);
      root.removeEventListener("pointerdown", onDown, true);
      root.removeEventListener("pointerup", onUp, true);
      root.removeEventListener("click", swallow, true);
      doc.removeEventListener("keydown", onKey);
      container.replaceChildren();
    },
  };
}

function cssEscape(s: string): string {
  return s.replace(/["\\]/g, "\\$&");
}

export { injectStyles };
