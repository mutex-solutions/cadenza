// Fails the build if the JavaScript loaded before a post is opened is over
// budget (plan §12: under 100 KB gzip). "Loaded first" means the scripts and
// module preloads index.html references; lazy chunks don't count.
import { readFileSync } from "node:fs";
import { gzipSync } from "node:zlib";

export const BUDGET = 100 * 1024;

/** The JS files index.html loads at startup, as paths relative to dist/. */
export function initialScripts(html) {
  const out = new Set();
  for (const m of html.matchAll(/<script\b[^>]*\bsrc="([^"]+\.js)"/g)) out.add(m[1]);
  for (const m of html.matchAll(/<link\b[^>]*\brel="modulepreload"[^>]*\bhref="([^"]+\.js)"/g)) out.add(m[1]);
  for (const m of html.matchAll(/<link\b[^>]*\bhref="([^"]+\.js)"[^>]*\brel="modulepreload"/g)) out.add(m[1]);
  return [...out].map((p) => p.replace(/^\//, ""));
}

export function measure(distUrl) {
  const html = readFileSync(new URL("index.html", distUrl), "utf8");
  const files = initialScripts(html).map((f) => {
    const buf = readFileSync(new URL(f, distUrl));
    return { file: f, raw: buf.length, gzip: gzipSync(buf, { level: 9 }).length };
  });
  return { files, total: files.reduce((n, f) => n + f.gzip, 0) };
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const { files, total } = measure(new URL("../dist/", import.meta.url));
  for (const f of files) console.log(`${f.file}  ${(f.raw / 1024).toFixed(1)} KB raw, ${(f.gzip / 1024).toFixed(1)} KB gzip`);
  const line = `initial JS: ${(total / 1024).toFixed(1)} KB gzip (budget ${BUDGET / 1024} KB)`;
  if (files.length === 0) {
    console.error("check-size: no scripts found in dist/index.html");
    process.exit(1);
  }
  if (total > BUDGET) {
    console.error(`${line} — OVER BUDGET`);
    process.exit(1);
  }
  console.log(line);
}
