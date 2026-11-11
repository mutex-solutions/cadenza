import { clock, fileSize, type FileRef } from "@cadenza/core";
import { h, openLink, whenVisible } from "./dom.ts";
import { sandboxSrcdoc } from "./sandbox.ts";
import type { Ctx } from "./types.ts";

/** Resolves a file's signed URL once it's near the screen; shows "unavailable" if that fails. */
function resolveWhenVisible(ctx: Ctx, el: HTMLElement, sha: string, what: string, use: (url: string) => void | Promise<void>): void {
  whenVisible(ctx, el, () => {
    ctx.opts
      .resolveFile(sha)
      .then(use)
      .catch(() => unavailable(ctx, el, what));
  });
}

export function unavailable(ctx: Ctx, el: HTMLElement, what: string): void {
  el.replaceChildren(h(ctx.doc, "p", { class: "cz-unavailable", text: `${what} unavailable. The file store can't be reached right now.` }));
}

export function isFileRef(v: unknown): v is FileRef {
  return typeof v === "object" && v !== null && typeof (v as FileRef).sha256 === "string";
}

/** An image, preview first when there is one; a tap opens the full size. */
export function image(ctx: Ctx, f: FileRef, alt: string): HTMLElement {
  const area = h(ctx.doc, "div", { class: "cz-pick-area cz-image" });
  const img = h(ctx.doc, "img", { attrs: { alt: alt || f.name, loading: "lazy", decoding: "async" } });
  area.appendChild(img);
  let full = "";
  img.addEventListener("click", () => {
    if (full && !area.closest(".cz-picking")) openLink(ctx, full);
  });
  whenVisible(ctx, area, () => {
    const fail = () => unavailable(ctx, area, "Image");
    if (f.preview_sha256) {
      ctx.opts
        .resolveFile(f.preview_sha256)
        .then((u) => {
          if (!full) img.src = u;
        })
        .catch(() => undefined);
    }
    ctx.opts
      .resolveFile(f.sha256)
      .then((u) => {
        full = u;
        img.dataset.full = u;
        if (!f.preview_sha256 || !img.getAttribute("src")) {
          img.src = u;
          return;
        }
        // Swap to the full image once it has loaded, so the preview stays until then.
        const pre = new (ctx.doc.defaultView?.Image ?? Image)();
        pre.onload = () => {
          img.src = u;
        };
        pre.src = u;
      })
      .catch(fail);
  });
  return area;
}

export function video(ctx: Ctx, f: FileRef): HTMLElement {
  const wrap = h(ctx.doc, "div", { class: "cz-video-wrap" });
  const v = h(ctx.doc, "video", { attrs: { controls: "", playsinline: "", preload: "metadata" } });
  v.setAttribute("aria-label", f.name);
  wrap.appendChild(v);
  resolveWhenVisible(ctx, wrap, f.sha256, "Video", (u) => {
    v.src = u;
  });
  return wrap;
}

/** A card with the file's name and size and a button to open or download it. */
export function fileCard(ctx: Ctx, f: FileRef): HTMLElement {
  const isPdf = f.type === "application/pdf";
  const btn = h(ctx.doc, "button", { class: "cz-button", text: isPdf ? "Open PDF" : "Download", attrs: { type: "button" } });
  const status = h(ctx.doc, "span", { class: "cz-file-status", attrs: { role: "status" } });
  btn.addEventListener("click", () => {
    status.textContent = "";
    ctx.opts.resolveFile(f.sha256).then(
      (u) => openLink(ctx, u),
      () => {
        status.textContent = "File unavailable. Try again later.";
      },
    );
  });
  return h(
    ctx.doc,
    "div",
    { class: "cz-file" },
    h(ctx.doc, "span", { class: "cz-file-icon", text: isPdf ? "PDF" : "FILE", attrs: { "aria-hidden": "true" } }),
    h(ctx.doc, "span", { class: "cz-file-info" }, h(ctx.doc, "strong", { class: "cz-file-name", text: f.name }), h(ctx.doc, "span", { class: "cz-file-meta", text: `${fileSize(f.size)} · ${f.type}` })),
    btn,
    status,
  );
}

/**
 * An agent's webpage in a locked box: sandbox="allow-scripts" without
 * allow-same-origin (no access to the app or its login), a CSP that blocks
 * all network use, and no referrer.
 */
export function webpage(ctx: Ctx, f: FileRef, title: string): HTMLElement {
  const wrap = h(ctx.doc, "div", { class: "cz-frame-wrap cz-pick-area" });
  const loading = h(ctx.doc, "p", { class: "cz-loading", text: "Loading page…" });
  const overlay = h(ctx.doc, "div", { class: "cz-overlay", attrs: { "aria-hidden": "true" } });
  wrap.append(loading, overlay);
  const full = h(ctx.doc, "button", { class: "cz-button cz-fullscreen-btn", text: "Full screen", attrs: { type: "button", "aria-pressed": "false" } });
  full.addEventListener("click", () => {
    const on = !box.classList.contains("cz-fullscreen");
    box.classList.toggle("cz-fullscreen", on);
    full.textContent = on ? "Exit full screen" : "Full screen";
    full.setAttribute("aria-pressed", String(on));
  });
  const box = h(ctx.doc, "div", { class: "cz-webpage" }, h(ctx.doc, "div", { class: "cz-toolbar" }, full), wrap);
  const fetchText = ctx.opts.fetchText ?? ((u: string) => fetch(u).then((r) => (r.ok ? r.text() : Promise.reject(new Error(String(r.status))))));
  whenVisible(ctx, wrap, () => {
    ctx.opts
      .resolveFile(f.sha256)
      .then(fetchText)
      .then((html) => {
        const frame = ctx.doc.createElement("iframe");
        // Attributes are set before the frame is in the page or has content.
        frame.setAttribute("sandbox", "allow-scripts");
        frame.setAttribute("referrerpolicy", "no-referrer");
        frame.setAttribute("loading", "lazy");
        frame.setAttribute("title", title || f.name);
        frame.setAttribute("srcdoc", sandboxSrcdoc(html));
        loading.replaceWith(frame);
      })
      .catch(() => {
        loading.className = "cz-unavailable";
        loading.textContent = "Page unavailable. The file store can't be reached right now.";
      });
  });
  return box;
}

/** A plain file attachment, drawn by its type. */
export function plainFile(ctx: Ctx, f: FileRef): HTMLElement {
  switch (f.type) {
    case "image/webp":
      return image(ctx, f, f.name);
    case "video/mp4":
      return video(ctx, f);
    case "text/html":
      return webpage(ctx, f, f.name);
    default:
      return fileCard(ctx, f);
  }
}

export { clock };
