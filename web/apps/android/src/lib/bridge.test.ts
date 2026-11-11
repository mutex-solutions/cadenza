import { describe, expect, it } from "vitest";
import { MAX_MESSAGE, jsCall, parseBridgeMessage, parseSpot, safeJson } from "./bridge";

const SHA = "a".repeat(64);

describe("parseBridgeMessage", () => {
  it("accepts each known message", () => {
    expect(parseBridgeMessage('{"type":"ready"}')).toEqual({ type: "ready" });
    expect(parseBridgeMessage(JSON.stringify({ type: "resolve", id: 3, sha256: SHA }))).toEqual({ type: "resolve", id: 3, sha256: SHA });
    expect(parseBridgeMessage(JSON.stringify({ type: "open", url: "https://go.dev" }))).toEqual({ type: "open", url: "https://go.dev" });
    expect(parseBridgeMessage(JSON.stringify({ type: "marker", id: "r_1" }))).toEqual({ type: "marker", id: "r_1" });
    expect(parseBridgeMessage(JSON.stringify({ type: "spot", postId: "p_1", spot: null }))).toEqual({ type: "spot", postId: "p_1", spot: null });
    expect(parseBridgeMessage(JSON.stringify({ type: "spot", postId: "p_1", spot: { attachment: 0, block: "fix", line: 12, side: "new", quote: "x" } }))).toEqual({
      type: "spot",
      postId: "p_1",
      spot: { attachment: 0, block: "fix", line: 12, side: "new", quote: "x" },
    });
    expect(parseBridgeMessage(JSON.stringify({ type: "error", message: "m" }))).toEqual({ type: "error", message: "m" });
  });

  it("drops extra fields", () => {
    expect(parseBridgeMessage(JSON.stringify({ type: "marker", id: "r_1", evil: "x" }))).toEqual({ type: "marker", id: "r_1" });
  });

  it("rejects unknown types, bad shapes, bad JSON and oversize messages", () => {
    for (const raw of [
      "not json",
      "[]",
      "null",
      '{"type":"eval","code":"x"}',
      JSON.stringify({ type: "resolve", id: -1, sha256: SHA }),
      JSON.stringify({ type: "resolve", id: 1, sha256: "../etc" }),
      JSON.stringify({ type: "resolve", id: 1.5, sha256: SHA }),
      JSON.stringify({ type: "open", url: "javascript:alert(1)" }),
      JSON.stringify({ type: "open", url: "intent://x" }),
      JSON.stringify({ type: "marker", id: "<script>" }),
      JSON.stringify({ type: "spot", postId: "p 1", spot: null }),
      JSON.stringify({ type: "spot", postId: "p_1", spot: { attachment: "0" } }),
      JSON.stringify({ type: "error", message: 5 }),
      JSON.stringify({ type: "ready", pad: "x".repeat(MAX_MESSAGE) }),
    ]) {
      expect(parseBridgeMessage(raw), raw.slice(0, 60)).toBeNull();
    }
  });
});

describe("parseSpot", () => {
  it("accepts fractions, time and clips quotes", () => {
    expect(parseSpot({ attachment: 1, x: 0.5, y: 0.25, w: 0.1, h: 0.2 })).toEqual({ attachment: 1, x: 0.5, y: 0.25, w: 0.1, h: 0.2 });
    expect(parseSpot({ attachment: 0, time_ms: 1500 })).toEqual({ attachment: 0, time_ms: 1500 });
    expect(parseSpot({ attachment: 0, quote: "q".repeat(5000) })!.quote).toHaveLength(1000);
  });
  it("rejects out-of-range values", () => {
    expect(parseSpot({ attachment: 0, x: 1.5 })).toBeNull();
    expect(parseSpot({ attachment: 0, line: 0 })).toBeNull();
    expect(parseSpot({ attachment: 0, side: "left" })).toBeNull();
    expect(parseSpot({ attachment: 0, time_ms: -1 })).toBeNull();
    expect(parseSpot({ attachment: 0, x: Number.NaN })).toBeNull();
    expect(parseSpot({})).toBeNull();
  });
});

describe("safeJson and jsCall", () => {
  it("escapes characters that could end a script or a line", () => {
    const s = safeJson({ t: "</script><b>&", l: "a\u2028b\u2029c" });
    expect(s).not.toMatch(/[<>&\u2028\u2029]/);
    expect(JSON.parse(s)).toEqual({ t: "</script><b>&", l: "a\u2028b\u2029c" });
  });
  it("builds guarded calls that evaluate to true", () => {
    expect(jsCall("resolved", 3, null)).toBe("window.__cz&&window.__cz.resolved(3,null);true;");
    expect(jsCall("cancelPick")).toBe("window.__cz&&window.__cz.cancelPick();true;");
    const code = jsCall("render", [{ id: "p", title: "'); alert(1); ('" }]);
    // The argument is a JSON literal, so quotes in data can't break out.
    const calls: unknown[] = [];
    new Function("window", code)({ __cz: { render: (x: unknown) => calls.push(x) } });
    expect(calls).toEqual([[{ id: "p", title: "'); alert(1); ('" }]]);
  });
});
