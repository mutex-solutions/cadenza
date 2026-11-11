import { clock, type PostView, type Spot } from "@cadenza/core";

function pct(n: number): string {
  return `${Math.round(n * 100)}%`;
}

/** Where a pinned comment points, in plain words, e.g. "line 12 of Fix". */
export function describeSpot(spot: Spot, post?: PostView): string {
  const att = post?.attachments.find((a) => a.index === spot.attachment);
  const slotLabel = (() => {
    if (!spot.block) return att?.file?.name ?? `attachment ${spot.attachment + 1}`;
    const slots = (att?.data as { slots?: { name: string; label?: string }[] } | undefined)?.slots;
    return slots?.find((s) => s.name === spot.block)?.label || spot.block;
  })();
  if (spot.line !== undefined) {
    const side = spot.side === "old" ? " (old)" : "";
    return `line ${spot.line}${side} of ${slotLabel}`;
  }
  if (spot.time_ms !== undefined) return `${clock(spot.time_ms)} in ${slotLabel}`;
  if (spot.x !== undefined && spot.y !== undefined) {
    const area = spot.w !== undefined && spot.h !== undefined ? "area" : "spot";
    return `${area} at ${pct(spot.x)} across, ${pct(spot.y)} down in ${slotLabel}`;
  }
  if (spot.quote) {
    const q = spot.quote.length > 40 ? `${spot.quote.slice(0, 40)}…` : spot.quote;
    return `“${q}” in ${slotLabel}`;
  }
  return slotLabel;
}
