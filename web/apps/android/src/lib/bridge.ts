// The message bridge between the viewer WebView and the app. It is the only
// channel between them, and it carries only these few message shapes. Agent
// webpages never reach it: they run in a sandboxed iframe inside the viewer.
import type { AttachmentView, Spot } from "@cadenza/core";

/** Messages from the WebView to the app. */
export type BridgeMessage =
  | { type: "ready" }
  | { type: "resolve"; id: number; sha256: string }
  | { type: "open"; url: string }
  | { type: "spot"; postId: string; spot: Spot | null }
  | { type: "marker"; id: string }
  | { type: "error"; message: string };

/** Largest message the app accepts, in characters. */
export const MAX_MESSAGE = 64 * 1024;

const SHA = /^[0-9a-f]{64}$/;
const ID = /^[A-Za-z0-9_\-:.]{1,128}$/;

function isObj(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isInt(v: unknown, min = 0): v is number {
  return typeof v === "number" && Number.isInteger(v) && v >= min;
}

function isFraction(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 1;
}

/** Checks a spot from the viewer; returns a clean copy or null. */
export function parseSpot(v: unknown): Spot | null {
  if (!isObj(v) || !isInt(v.attachment) || v.attachment > 1000) return null;
  const s: Spot = { attachment: v.attachment };
  if (v.block !== undefined) {
    if (typeof v.block !== "string" || v.block.length > 100) return null;
    s.block = v.block;
  }
  if (v.line !== undefined) {
    if (!isInt(v.line, 1)) return null;
    s.line = v.line;
  }
  if (v.side !== undefined) {
    if (v.side !== "old" && v.side !== "new") return null;
    s.side = v.side;
  }
  for (const k of ["x", "y", "w", "h"] as const) {
    if (v[k] !== undefined) {
      if (!isFraction(v[k])) return null;
      s[k] = v[k];
    }
  }
  if (v.time_ms !== undefined) {
    if (!isInt(v.time_ms)) return null;
    s.time_ms = v.time_ms;
  }
  if (v.quote !== undefined) {
    if (typeof v.quote !== "string") return null;
    s.quote = v.quote.slice(0, 1000);
  }
  return s;
}

/** Parses and validates one raw message; anything unexpected gives null. */
export function parseBridgeMessage(raw: string): BridgeMessage | null {
  if (typeof raw !== "string" || raw.length > MAX_MESSAGE) return null;
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!isObj(v)) return null;
  switch (v.type) {
    case "ready":
      return { type: "ready" };
    case "resolve":
      if (!isInt(v.id) || typeof v.sha256 !== "string" || !SHA.test(v.sha256)) return null;
      return { type: "resolve", id: v.id, sha256: v.sha256 };
    case "open":
      if (typeof v.url !== "string" || v.url.length > 4096 || !/^https?:\/\//i.test(v.url)) return null;
      return { type: "open", url: v.url };
    case "spot": {
      if (typeof v.postId !== "string" || !ID.test(v.postId)) return null;
      if (v.spot === null) return { type: "spot", postId: v.postId, spot: null };
      const spot = parseSpot(v.spot);
      return spot ? { type: "spot", postId: v.postId, spot } : null;
    }
    case "marker":
      if (typeof v.id !== "string" || !ID.test(v.id)) return null;
      return { type: "marker", id: v.id };
    case "error":
      if (typeof v.message !== "string") return null;
      return { type: "error", message: v.message.slice(0, 500) };
    default:
      return null;
  }
}

/**
 * JSON that is safe to place inside JavaScript source: no "<" (so no
 * "</script"), and no U+2028/U+2029, which end lines in older JS engines.
 */
export function safeJson(v: unknown): string {
  return JSON.stringify(v ?? null)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

/** Builds a call to one of the host page's window.__cz functions, for injectJavaScript. */
export function jsCall(fn: "render" | "resolved" | "pick" | "cancelPick" | "markers", ...args: unknown[]): string {
  return `window.__cz&&window.__cz.${fn}(${args.map(safeJson).join(",")});true;`;
}

/** What the host page needs to draw one post. */
export interface HostPost {
  id: string;
  title: string;
  summary_html?: string;
  attachments: AttachmentView[];
}

/** A comment marker drawn on a post. */
export interface HostMarker {
  id: string;
  spot: Spot;
  label: string;
}
