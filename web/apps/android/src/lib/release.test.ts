import { describe, expect, it } from "vitest";
import appConfig from "../../app.config.js";
import signing from "../../plugins/release-signing.js";

const GRADLE = `android {
    signingConfigs {
        debug {
            storeFile file('debug.keystore')
        }
    }
    buildTypes {
        debug {
            signingConfig signingConfigs.debug
        }
        release {
            // Caution!
            signingConfig signingConfigs.debug
            minifyEnabled false
        }
    }
}`;

describe("release signing plugin", () => {
  it("adds a release config from CI variables and uses it only when the upload key is set", () => {
    const out = signing.setReleaseSigning(GRADLE);
    expect(out).toContain("System.getenv('CADENZA_UPLOAD_STORE_FILE')");
    expect(out).toContain("keyAlias System.getenv('CADENZA_UPLOAD_KEY_ALIAS')");
    expect(out).toContain("signingConfig System.getenv('CADENZA_UPLOAD_STORE_FILE') ? signingConfigs.release : signingConfigs.debug");
    // The debug build type keeps the debug key.
    expect(out).toMatch(/debug \{\n\s+signingConfig signingConfigs\.debug/);
    expect(signing.setReleaseSigning(out)).toBe(out); // running prebuild twice changes nothing
  });

  it("fails loudly on an unexpected build.gradle", () => {
    expect(() => signing.setReleaseSigning("android {}")).toThrow(/signingConfigs/);
    expect(() => signing.setReleaseSigning("signingConfigs {\n}")).toThrow(/release build type/);
  });
});

describe("app.config version", () => {
  const base = { version: "0.1.0", android: { package: "dev.cadenza.app" } };
  it("keeps app.json values for local builds", () => {
    expect(appConfig.withVersion(base, {})).toMatchObject({ version: "0.1.0", android: { versionCode: 1, package: "dev.cadenza.app" } });
  });
  it("takes the version name and code from CI", () => {
    const c = appConfig.withVersion(base, { CADENZA_VERSION_NAME: "0.1.5", CADENZA_VERSION_CODE: "1000042" });
    expect(c).toMatchObject({ version: "0.1.5", android: { versionCode: 1000042, package: "dev.cadenza.app" } });
  });
  it("refuses a version code that is not a positive whole number", () => {
    for (const bad of ["0", "-1", "1.5", "abc", "12345678901"]) {
      expect(() => appConfig.withVersion(base, { CADENZA_VERSION_CODE: bad })).toThrow(/positive whole number/);
    }
  });
});
