import { describe, expect, it } from "vitest";
import { COLUMN_MAX_WIDTH, FORM_MAX_WIDTH, layoutFor, windowClass } from "./layout";

describe("window size classes", () => {
  it("uses Material breakpoints", () => {
    expect(windowClass(360)).toBe("compact");
    expect(windowClass(599)).toBe("compact");
    expect(windowClass(600)).toBe("medium");
    expect(windowClass(839)).toBe("medium");
    expect(windowClass(840)).toBe("expanded");
    expect(windowClass(1280)).toBe("expanded");
  });
});

describe("layoutFor", () => {
  it("keeps phones and a foldable's cover screen in one pane", () => {
    for (const w of [360, 393, 412, 599, 699]) {
      const l = layoutFor(w);
      expect(l.twoPane).toBe(false);
      expect(l.listWidth).toBe(w);
    }
  });

  it("uses two panes on a Fold's inner screen in both orientations", () => {
    // Galaxy Z Fold inner display: about 707 x 830 dp.
    for (const w of [707, 830]) {
      const l = layoutFor(w);
      expect(l.cls).toBe("medium");
      expect(l.twoPane).toBe(true);
      expect(l.listWidth).toBeGreaterThanOrEqual(300);
      expect(w - l.listWidth).toBeGreaterThanOrEqual(400); // room for the thread
    }
  });

  it("caps the list width on big tablets", () => {
    const l = layoutFor(1280);
    expect(l.cls).toBe("expanded");
    expect(l.listWidth).toBe(420);
  });

  it("has sensible content widths", () => {
    expect(FORM_MAX_WIDTH).toBeLessThan(COLUMN_MAX_WIDTH);
    expect(COLUMN_MAX_WIDTH).toBeLessThanOrEqual(840);
  });
});
