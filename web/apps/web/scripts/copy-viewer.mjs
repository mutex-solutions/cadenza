// Copies the viewer's standalone bundle into dist/viewer/, so the Go server
// serves it at /viewer/viewer.js for the Android app's WebView.
import { cpSync, existsSync, mkdirSync, readdirSync } from "node:fs";

const src = new URL("../../../packages/viewer/dist/", import.meta.url);
const dest = new URL("../dist/viewer/", import.meta.url);
if (!existsSync(src)) {
  console.error("viewer bundle missing: run pnpm --filter @cadenza/viewer build first");
  process.exit(1);
}
mkdirSync(dest, { recursive: true });
let n = 0;
for (const f of readdirSync(src)) {
  if (!f.endsWith(".js")) continue;
  cpSync(new URL(f, src), new URL(f, dest));
  n++;
}
console.log(`copied ${n} viewer files to dist/viewer/`);
