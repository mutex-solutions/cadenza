// Hinge-aware layout for foldables. The native module (modules/cadenza-fold)
// reports each fold in window pixels; this file turns that into dp and
// decides where the two panes split. Pure, so it is unit-tested.
import { layoutFor } from "./layout";

/** A fold as the native module reports it: bounds in window pixels. */
export interface RawFold {
  left: number;
  top: number;
  right: number;
  bottom: number;
  orientation: "vertical" | "horizontal";
  state: "flat" | "half_opened";
  separating: boolean;
  occluding: boolean;
}

/** A fold in window dp. */
export type Fold = RawFold;

export function toDp(folds: RawFold[], pixelRatio: number): Fold[] {
  const r = pixelRatio > 0 ? pixelRatio : 1;
  return folds.map((f) => ({ ...f, left: f.left / r, top: f.top / r, right: f.right / r, bottom: f.bottom / r }));
}

/** Smallest side of a split: about a phone's width. */
export const MIN_PANE_WIDTH = 280;

export interface PaneSplit {
  twoPane: boolean;
  /** Width of the inbox list. */
  listWidth: number;
  /** Space left empty over the hinge (0 for a seamless fold). */
  gap: number;
  /** True when the split follows a fold rather than the width rules. */
  atHinge: boolean;
}

/**
 * splitPanes decides the inbox/thread split for a content area that starts
 * contentLeft dp from the window's left edge and is contentWidth dp wide.
 *
 * A vertical fold that is half-opened (the "book" posture) or that separates
 * the screen puts the split exactly on the hinge, as long as both sides are
 * at least phone-sized. That also gives two panes on a foldable's inner
 * screen that is narrower than the usual two-pane width. Otherwise the
 * width rules from layout.ts apply.
 */
export function splitPanes(contentLeft: number, contentWidth: number, folds: Fold[]): PaneSplit {
  const hinge = folds.find((f) => f.orientation === "vertical" && (f.separating || f.state === "half_opened"));
  if (hinge) {
    const left = hinge.left - contentLeft;
    const right = hinge.right - contentLeft;
    if (left >= MIN_PANE_WIDTH && contentWidth - right >= MIN_PANE_WIDTH) {
      return { twoPane: true, listWidth: Math.round(left), gap: Math.max(0, Math.round(right - left)), atHinge: true };
    }
  }
  const l = layoutFor(contentWidth);
  return { twoPane: l.twoPane, listWidth: l.listWidth, gap: 0, atHinge: false };
}
