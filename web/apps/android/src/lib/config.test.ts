import { describe, expect, it } from "vitest";
import appJson from "../../app.json";

// Guards the Expo config that the native manifest is generated from.
const app = appJson as unknown as { expo: { orientation?: string; android: Record<string, unknown>; plugins: unknown[] } };

describe("app.json", () => {
  it("does not lock the orientation, so tablets and foldables can rotate", () => {
    expect(app.expo.orientation).toBe("default");
  });

  it("keeps the activity resizable (split screen, fold and unfold)", () => {
    expect(app.expo.android.resizeableActivity).not.toBe(false);
  });

  it("keeps the large-screens plugin (fold/unfold and rotation keep the user's place)", () => {
    expect(app.expo.plugins).toContain("./plugins/large-screens.js");
  });

  it("keeps the cleartext plugin for http:// servers", () => {
    expect(app.expo.plugins).toContain("./plugins/cleartext.js");
  });
});
