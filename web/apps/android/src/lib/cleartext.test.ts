import { describe, expect, it } from "vitest";
import plugin from "../../plugins/cleartext.js";

const { setCleartext } = plugin;

describe("cleartext config plugin", () => {
  it("allows http:// servers in release builds, keeping other attributes", () => {
    const out = setCleartext({ manifest: { application: [{ $: { "android:name": ".MainApplication" } }] } });
    expect(out.manifest.application?.[0]?.$).toEqual({
      "android:name": ".MainApplication",
      "android:usesCleartextTraffic": "true",
    });
  });

  it("fails loudly when the manifest has no application", () => {
    expect(() => setCleartext({ manifest: {} })).toThrow(/no <application>/);
  });
});
