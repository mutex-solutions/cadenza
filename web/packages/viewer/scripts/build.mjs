// Builds the viewer for the Android WebView (the web app imports the source
// through Vite instead):
//   dist/viewer.js          one IIFE file, global CadenzaViewer, no chart/diagram code
//   dist/viewer-chart.js    Vega-Lite charts, loaded by viewer.js when needed
//   dist/viewer-diagram.js  Mermaid diagrams, loaded by viewer.js when needed
import { build } from "esbuild";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { gzipSync } from "node:zlib";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "dist");
const MAX_CORE_BYTES = 150 * 1024;

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });

const common = {
  bundle: true,
  minify: true,
  format: "iife",
  target: "es2020",
  platform: "browser",
  legalComments: "none",
  logLevel: "warning",
  define: { "process.env.NODE_ENV": '"production"' },
};

await Promise.all([
  build({ ...common, entryPoints: [join(root, "src/iife.ts")], outfile: join(dist, "viewer.js") }),
  build({ ...common, entryPoints: [join(root, "src/lazy/chart-iife.ts")], outfile: join(dist, "viewer-chart.js") }),
  build({ ...common, entryPoints: [join(root, "src/lazy/diagram-iife.ts")], outfile: join(dist, "viewer-diagram.js") }),
]);

let failed = false;
for (const name of ["viewer.js", "viewer-chart.js", "viewer-diagram.js"]) {
  const buf = readFileSync(join(dist, name));
  const kb = (n) => `${(n / 1024).toFixed(1)} KB`;
  console.log(`${name.padEnd(20)} ${kb(buf.length).padStart(10)}  gzip ${kb(gzipSync(buf).length)}`);
  if (name === "viewer.js" && buf.length > MAX_CORE_BYTES) {
    console.error(`viewer.js is ${kb(buf.length)}; the limit is ${kb(MAX_CORE_BYTES)}. Chart or diagram code probably got bundled into it.`);
    failed = true;
  }
}
if (failed) process.exit(1);
