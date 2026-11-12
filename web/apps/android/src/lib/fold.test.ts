import { describe, expect, it } from "vitest";
import { splitPanes, toDp, type Fold } from "./fold";

const fold = (o: Partial<Fold>): Fold => ({
  left: 420, right: 420, top: 0, bottom: 800, orientation: "vertical", state: "half_opened", separating: false, occluding: false, ...o,
});

describe("toDp", () => {
  it("converts window pixels to dp", () => {
    const [f] = toDp([fold({ left: 1104, right: 1104, bottom: 1768 })], 2.625);
    expect(f!.left).toBeCloseTo(420.57, 1);
    expect(f!.bottom).toBeCloseTo(673.5, 1);
    expect(toDp([fold({ left: 10 })], 0)[0]!.left).toBe(10);
  });
});

describe("splitPanes", () => {
  it("splits on the hinge in the half-opened book posture", () => {
    // 7.6" foldable in landscape: 841 dp wide, hinge in the middle, 42 dp inset on the left.
    const s = splitPanes(42, 799, [fold({ left: 420.5, right: 420.5 })]);
    expect(s).toEqual({ twoPane: true, listWidth: 379, gap: 0, atHinge: true });
  });

  it("leaves the hinge area empty when the fold has width (a physical gap)", () => {
    const s = splitPanes(0, 840, [fold({ left: 410, right: 430, state: "flat", separating: true })]);
    expect(s).toMatchObject({ twoPane: true, listWidth: 410, gap: 20, atHinge: true });
  });

  it("gives two panes on an inner screen narrower than the usual two-pane width", () => {
    // 673 dp wide in portrait, half-opened: each side is about 336 dp.
    const s = splitPanes(0, 673, [fold({ left: 336.5, right: 336.5 })]);
    expect(s.twoPane).toBe(true);
    expect(s.atHinge).toBe(true);
    // The same screen lying flat uses the width rules: one pane below 700 dp.
    expect(splitPanes(0, 673, [fold({ left: 336.5, right: 336.5, state: "flat" })]).twoPane).toBe(false);
  });

  it("ignores horizontal folds (tabletop) and folds too close to an edge", () => {
    expect(splitPanes(0, 841, [fold({ orientation: "horizontal" })]).atHinge).toBe(false);
    expect(splitPanes(0, 841, [fold({ left: 200, right: 200 })]).atHinge).toBe(false);
    expect(splitPanes(0, 841, [fold({ left: 700, right: 700 })]).atHinge).toBe(false);
  });

  it("falls back to the width rules without a fold", () => {
    expect(splitPanes(0, 411, [])).toEqual({ twoPane: false, listWidth: 411, gap: 0, atHinge: false });
    expect(splitPanes(0, 1280, [])).toMatchObject({ twoPane: true, listWidth: 420, atHinge: false });
  });
});
