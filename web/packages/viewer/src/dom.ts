import type { Ctx } from "./types.ts";

interface Props {
  class?: string;
  text?: string;
  attrs?: Record<string, string>;
}

/** Makes an element. Text always goes in as textContent, never as HTML. */
export function h<K extends keyof HTMLElementTagNameMap>(
  doc: Document,
  tag: K,
  props: Props = {},
  ...children: (Node | null | undefined | false)[]
): HTMLElementTagNameMap[K] {
  const el = doc.createElement(tag);
  if (props.class) el.className = props.class;
  if (props.text !== undefined) el.textContent = props.text;
  if (props.attrs) for (const [k, v] of Object.entries(props.attrs)) el.setAttribute(k, v);
  for (const c of children) if (c) el.appendChild(c);
  return el;
}

/** True for absolute http(s) URLs, the only links Cadenza opens. */
export function isWebUrl(url: unknown): url is string {
  if (typeof url !== "string") return false;
  try {
    const u = new URL(url);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

export function openLink(ctx: Ctx, url: string): void {
  if (ctx.opts.openLink) ctx.opts.openLink(url);
  else ctx.doc.defaultView?.open(url, "_blank", "noopener,noreferrer");
}

/** A link that opens in a new tab through openLink; non-web URLs become plain text. */
export function link(ctx: Ctx, url: unknown, text: string): HTMLElement {
  if (!isWebUrl(url)) return h(ctx.doc, "span", { class: "cz-link-text", text });
  const a = h(ctx.doc, "a", { text, attrs: { href: url, target: "_blank", rel: "noopener noreferrer" } });
  a.addEventListener("click", (e) => {
    e.preventDefault();
    openLink(ctx, url);
  });
  return a;
}

/** Is this value a {md, html} markdown field? */
export function isMarkdown(v: unknown): v is { md: string; html: string } {
  return typeof v === "object" && v !== null && typeof (v as { html?: unknown }).html === "string";
}

/**
 * Markdown HTML from the server (already sanitized there) goes in as HTML.
 * Links inside it still only open web addresses, in a new tab.
 */
export function markdown(ctx: Ctx, v: unknown, cls = "cz-md"): HTMLElement {
  const el = h(ctx.doc, "div", { class: cls });
  if (isMarkdown(v)) {
    el.innerHTML = v.html;
    for (const a of Array.from(el.querySelectorAll("a"))) {
      const href: string | null = a.getAttribute("href");
      if (!isWebUrl(href)) {
        if (!(href ?? "").startsWith("mailto:")) a.removeAttribute("href");
        continue;
      }
      a.setAttribute("target", "_blank");
      a.setAttribute("rel", "noopener noreferrer");
      a.addEventListener("click", (e) => {
        e.preventDefault();
        openLink(ctx, href);
      });
    }
  } else if (typeof v === "string") {
    el.textContent = v;
  } else if (v && typeof (v as { md?: unknown }).md === "string") {
    el.textContent = (v as { md: string }).md;
  }
  return el;
}

/** Plain text for any scalar a template may hold (table cells, metric values). */
export function plain(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "number") return Number.isInteger(v) ? v.toLocaleString("en-US") : String(v);
  if (typeof v === "string") return v;
  if (typeof v === "boolean") return v ? "yes" : "no";
  return JSON.stringify(v);
}

/**
 * Runs `f` once the element is near the screen (IntersectionObserver), or at
 * once where there's no IntersectionObserver.
 */
export function whenVisible(ctx: Ctx, el: Element, f: () => void): void {
  const IO = (ctx.doc.defaultView as (Window & typeof globalThis) | null)?.IntersectionObserver ?? globalThis.IntersectionObserver;
  if (typeof IO !== "function") {
    f();
    return;
  }
  let done = false;
  const io = new IO(
    (entries) => {
      if (done || !entries.some((e) => e.isIntersecting)) return;
      done = true;
      io.disconnect();
      f();
    },
    { rootMargin: "300px" },
  );
  io.observe(el);
  ctx.cleanups.push(() => io.disconnect());
}
