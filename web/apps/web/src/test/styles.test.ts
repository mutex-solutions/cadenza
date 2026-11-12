import { describe, expect, it } from "vitest";
import css from "../styles.css?raw";

// The top bar (brand + Inbox, Machines, Settings) is 358 px wide at normal
// spacing; a foldable's cover screen is about 344 px. A headless-browser check
// found the page scrolling sideways there, so the narrow rule must stay.
describe("styles", () => {
  it("tightens the top bar on screens under 380 px so it never scrolls sideways", () => {
    const narrow = /@media \(max-width: 379px\) \{([\s\S]*?)\n\}/.exec(css);
    expect(narrow).not.toBeNull();
    expect(narrow![1]).toMatch(/\.nav a \{[^}]*padding: 6px 7px/);
    expect(narrow![1]).toMatch(/\.topbar-inner \{[^}]*padding: 8px 10px/);
  });
});
