// Window size classes and the layout decisions that follow from them.
// Breakpoints follow Material's window size classes, in density-independent
// pixels (dp): compact < 600 ≤ medium < 840 ≤ expanded.
//
// Reference sizes: a phone, or a foldable's cover screen, is about 360–420 dp
// wide (compact). A book-style foldable's inner screen (Galaxy Z Fold) is about
// 700–830 dp in either orientation (medium). Tablets in landscape are
// expanded.

export type WindowClass = "compact" | "medium" | "expanded";

export function windowClass(widthDp: number): WindowClass {
  if (widthDp < 600) return "compact";
  if (widthDp < 840) return "medium";
  return "expanded";
}

/** Two panes need room for a list (≥ 300 dp) and a readable thread (≥ 400 dp). */
export const TWO_PANE_MIN_WIDTH = 700;

/** Widest a form or settings column grows; wider looks stretched. */
export const FORM_MAX_WIDTH = 560;

/** Widest a single column of threads or posts grows. */
export const COLUMN_MAX_WIDTH = 760;

export interface Layout {
  cls: WindowClass;
  /** Show the inbox list and the open thread side by side. */
  twoPane: boolean;
  /** Width of the inbox list in two-pane mode. */
  listWidth: number;
}

export function layoutFor(widthDp: number): Layout {
  const cls = windowClass(widthDp);
  const twoPane = widthDp >= TWO_PANE_MIN_WIDTH;
  const listWidth = twoPane ? Math.round(Math.min(420, Math.max(300, widthDp * 0.36))) : widthDp;
  return { cls, twoPane, listWidth };
}
