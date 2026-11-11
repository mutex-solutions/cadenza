import type { AttachmentView, FileRef } from "@cadenza/core";
import type { View, ViewSlot } from "../types.ts";
import { BUILTINS, type BuiltinTemplate } from "./builtins.ts";

export { BUILTINS };

/** A fake 64-hex sha256 derived from a string. */
export function fakeSha(s: string): string {
  let hash = 0;
  for (const c of s) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
  return (hash.toString(16).padStart(8, "0").repeat(8)).slice(0, 64);
}

export function ref(path: string, type: string): FileRef {
  const name = path.split("/").pop()!.replace(/\.(png|jpe?g|gif)$/i, ".webp");
  const r: FileRef = { sha256: fakeSha(path), size: 1234, type, name };
  if (type === "image/webp") r.preview_sha256 = fakeSha(`${path}#preview`);
  return r;
}

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
export const md = (s: string) => ({ md: s, html: `<p>${esc(s)}</p>` });

const typeFor = (block: string, path: string) => {
  if (block === "image" || block === "gallery" || block === "compare") return "image/webp";
  if (block === "video") return "video/mp4";
  if (block === "webpage") return "text/html";
  return path.endsWith(".pdf") ? "application/pdf" : "application/octet-stream";
};

/** Turns example data (local paths, plain markdown) into what the server sends (FileRefs, {md, html}, defaults). */
export function toValue(block: string, v: unknown): unknown {
  const o = v as Record<string, unknown>;
  const file = (x: unknown) => {
    const f = (x as { file: string }).file;
    return { file: ref(f, typeFor(block, f)) };
  };
  switch (block) {
    case "text":
      return md(v as string);
    case "callout":
      return { tone: (o.tone as string) ?? "info", text: md(o.text as string) };
    case "checklist":
      return (v as Record<string, unknown>[]).map((it) => ({ done: it.done ?? false, text: md(it.text as string) }));
    case "list":
      return (v as string[]).map(md);
    case "image":
    case "video":
    case "webpage":
    case "file":
      return file(v);
    case "gallery":
      return (v as Record<string, unknown>[]).map((it) => ({ ...it, file: ref(it.file as string, "image/webp") }));
    case "compare":
      return { ...o, before: file(o.before), after: file(o.after) };
    case "choices":
      return (v as Record<string, unknown>[]).map((it) => (it.text === undefined ? it : { ...it, text: md(it.text as string) }));
    default:
      return v;
  }
}

export function toView(t: BuiltinTemplate): View {
  const slots: ViewSlot[] = [];
  for (const s of t.slots) {
    if (t.example[s.name] === undefined) continue;
    slots.push({ name: s.name, block: s.block, label: s.label, value: toValue(s.block, t.example[s.name]) });
  }
  return { template: t.name, version: 1, slots };
}

export function templateAttachment(view: View, index = 0): AttachmentView {
  return { index, template: view.template, version: view.version ?? 1, data: view };
}

export function slotAttachment(block: string, value: unknown, name = "s", label = "Label", index = 0): AttachmentView {
  return templateAttachment({ template: "custom", version: 1, slots: [{ name, block, label, value }] }, index);
}

/** One custom template using the blocks no built-in uses: compare, webpage, file. */
export const extraBlocks: View = {
  template: "extras",
  version: 1,
  slots: [
    { name: "slider", block: "compare", label: "Slider", value: toValue("compare", { before: { file: "./a.png" }, after: { file: "./b.png" }, caption: "Before vs after" }) },
    { name: "page", block: "webpage", label: "Page", value: toValue("webpage", { file: "./report.html" }) },
    { name: "doc", block: "file", label: "Doc", value: toValue("file", { file: "./spec.pdf" }) },
  ],
};
