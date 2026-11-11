// @vitest-environment node
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { describe, expect, it } from "vitest";
import { BUDGET, initialScripts, measure } from "../scripts/check-size.mjs";
import { drawIcon } from "../scripts/make-icons.mjs";

describe("check-size", () => {
  it("finds entry scripts and module preloads, not lazy chunks", () => {
    const html = `<script type="module" crossorigin src="/assets/index-a.js"></script>
      <link rel="modulepreload" crossorigin href="/assets/react-b.js">
      <link rel="stylesheet" href="/assets/index.css">`;
    expect(initialScripts(html)).toEqual(["assets/index-a.js", "assets/react-b.js"]);
  });

  it("measures gzip size of the initial scripts", () => {
    const dir = mkdtempSync(join(tmpdir(), "cz-size-"));
    mkdirSync(join(dir, "assets"));
    writeFileSync(join(dir, "index.html"), `<script type="module" src="/assets/a.js"></script>`);
    writeFileSync(join(dir, "assets", "a.js"), "console.log(1);".repeat(1000));
    writeFileSync(join(dir, "assets", "lazy.js"), "x".repeat(500_000));
    const { files, total } = measure(pathToFileURL(dir + "/"));
    expect(files.map((f) => f.file)).toEqual(["assets/a.js"]);
    expect(total).toBeGreaterThan(0);
    expect(total).toBeLessThan(BUDGET);
  });
});

describe("make-icons", () => {
  it("writes a valid PNG of the right size", () => {
    const png = drawIcon(48);
    expect(png.subarray(1, 4).toString()).toBe("PNG");
    expect(png.readUInt32BE(16)).toBe(48);
    expect(png.readUInt32BE(20)).toBe(48);
  });
});
