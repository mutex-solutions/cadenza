/** The only things an agent's webpage may use: inline code and styles, and data/blob media. No network. */
export const WEBPAGE_CSP =
  "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:";

const META = `<meta http-equiv="Content-Security-Policy" content="${WEBPAGE_CSP}">`;

// Whitespace and comments the parser skips before the first real tag.
const SKIP = String.raw`(?:\s|<!--[\s\S]*?-->)*`;
const DOCTYPE = new RegExp(String.raw`^\uFEFF?${SKIP}<!doctype[^>]*>`, "i");
const HTML_TAG = String.raw`<html(?=[\s>/])[^>]*>`;
const HEAD_TAG = String.raw`<head(?=[\s>/])[^>]*>`;
const HTML_HEAD = new RegExp(String.raw`^${SKIP}(?:${HTML_TAG}${SKIP})?${HEAD_TAG}`, "i");
const HTML_ONLY = new RegExp(String.raw`^${SKIP}${HTML_TAG}`, "i");

/**
 * Puts the CSP meta tag first in the page's <head>, adding <head> when the
 * page has none. Tags are only matched at the very start of the page (after
 * the doctype, whitespace and comments), so a "<head>" inside a comment or a
 * script further down can't trick it. A second CSP in the page can only
 * tighten the rules, never loosen them.
 */
export function sandboxSrcdoc(html: string): string {
  let prefix = "";
  let rest = html;
  const d = DOCTYPE.exec(rest);
  if (d) {
    prefix = d[0];
    rest = rest.slice(d[0].length);
  }
  const head = HTML_HEAD.exec(rest);
  if (head) return prefix + head[0] + META + rest.slice(head[0].length);
  const tag = HTML_ONLY.exec(rest);
  if (tag) return prefix + tag[0] + `<head>${META}</head>` + rest.slice(tag[0].length);
  // A fragment: the parser moves later <title>, <style> and <script> into this head.
  return prefix + `<head>${META}</head>` + rest;
}
