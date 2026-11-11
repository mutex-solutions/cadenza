const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " ", "#39": "'" };

/**
 * Turns sanitized server HTML (question text, summaries) into plain text for
 * native Text views: block ends become line breaks, tags are dropped and
 * entities decoded. Output is only ever shown as text, never as HTML.
 */
export function htmlToText(html: string): string {
  return html
    .replace(/<\s*(script|style)[^>]*>[\s\S]*?<\s*\/\s*\1\s*>/gi, "")
    .replace(/<\s*br\s*\/?>/gi, "\n")
    .replace(/<\s*li[^>]*>/gi, "• ")
    .replace(/<\s*\/\s*(p|div|li|h[1-6]|pre|blockquote|tr)\s*>/gi, "\n")
    .replace(/<[^>]*>/g, "")
    .replace(/&(#x[0-9a-f]+|#\d+|[a-z0-9]+);/gi, (m, e: string) => {
      const k = e.toLowerCase();
      if (k.startsWith("#x")) return safeChar(parseInt(k.slice(2), 16), m);
      if (k.startsWith("#") && k !== "#39") return safeChar(parseInt(k.slice(1), 10), m);
      return ENTITIES[k] ?? m;
    })
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function safeChar(code: number, fallback: string): string {
  if (!Number.isFinite(code) || code <= 0 || code > 0x10ffff) return fallback;
  return String.fromCodePoint(code);
}
