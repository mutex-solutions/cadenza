import { describe, expect, it } from "vitest";
import plugin from "../../plugins/large-screens.js";

const { setLargeScreen, setLegacyPackaging } = plugin;

const manifest = (attrs: Record<string, string>) => ({
  manifest: { application: [{ activity: [{ $: { "android:name": ".SomethingElse" } }, { $: { "android:name": ".MainActivity", ...attrs } }] }] },
});

describe("large-screens config plugin", () => {
  it("handles rotation, fold/unfold and density changes in place, without recreating the activity", () => {
    const out = setLargeScreen(manifest({ "android:configChanges": "keyboard|orientation|assetsPaths", "android:screenOrientation": "portrait" }));
    const main = out.manifest.application?.[0]?.activity?.[1]?.$ ?? {};
    const changes = main["android:configChanges"]!.split("|");
    for (const c of ["orientation", "screenSize", "screenLayout", "smallestScreenSize", "density", "assetsPaths"]) {
      expect(changes).toContain(c);
    }
    expect(new Set(changes).size).toBe(changes.length);
    expect(main["android:resizeableActivity"]).toBe("true");
    expect(main["android:screenOrientation"]).toBeUndefined();
  });

  it("leaves other activities alone and fails loudly without MainActivity", () => {
    const out = setLargeScreen(manifest({}));
    expect(out.manifest.application?.[0]?.activity?.[0]?.$).toEqual({ "android:name": ".SomethingElse" });
    expect(() => setLargeScreen({ manifest: { application: [{ activity: [] }] } })).toThrow(/MainActivity/);
  });
});

describe("legacy packaging", () => {
  it("unpacks native libraries at install, replacing any earlier setting", () => {
    const out = setLegacyPackaging([
      { type: "comment", value: "x" },
      { type: "property", key: "expo.useLegacyPackaging", value: "false" },
      { type: "property", key: "other", value: "1" },
    ]);
    expect(out.filter((p) => p.key === "expo.useLegacyPackaging")).toEqual([{ type: "property", key: "expo.useLegacyPackaging", value: "true" }]);
    expect(out).toContainEqual({ type: "property", key: "other", value: "1" });
    expect(out).toContainEqual({ type: "comment", value: "x" });
  });
});
