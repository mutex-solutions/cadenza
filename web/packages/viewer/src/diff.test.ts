import { describe, expect, it } from "vitest";
import { parseDiff } from "./diff.ts";

describe("parseDiff", () => {
  it("numbers old and new lines per hunk", () => {
    const rows = parseDiff("--- a/x.go\n+++ b/x.go\n@@ -12,3 +12,3 @@ func f() {\n-\tif len(s) > 32 {\n+\tif n > 32 {\n \t\treturn err\n \t}\n");
    expect(rows.map((r) => [r.kind, r.old, r.new])).toEqual([
      ["file", undefined, undefined],
      ["file", undefined, undefined],
      ["hunk", undefined, undefined],
      ["del", 12, undefined],
      ["add", undefined, 12],
      ["ctx", 13, 13],
      ["ctx", 14, 14],
    ]);
    expect(rows[3]!.text).toBe("\tif len(s) > 32 {");
  });

  it("doesn't take a removed '--' line for a file header", () => {
    const rows = parseDiff("@@ -1,2 +1,1 @@\n--- not a header\n keep\n");
    expect(rows.map((r) => r.kind)).toEqual(["hunk", "del", "ctx"]);
    expect(rows[1]!.text).toBe("-- not a header");
  });

  it("handles several hunks, single-line hunks and no-newline markers", () => {
    const rows = parseDiff("@@ -1 +1 @@\n-a\n+b\n\\ No newline at end of file\n@@ -10,0 +11,2 @@\n+c\n+d");
    expect(rows.map((r) => [r.kind, r.old ?? r.new])).toEqual([
      ["hunk", undefined],
      ["del", 1],
      ["add", 1],
      ["meta", undefined],
      ["hunk", undefined],
      ["add", 11],
      ["add", 12],
    ]);
  });
});
