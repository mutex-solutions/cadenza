// Expo config plugin: sign release builds with your Play upload key when it is
// provided, so Google Play accepts the bundle.
//
// The key comes from environment variables set by CI (never from the repo):
//   CADENZA_UPLOAD_STORE_FILE      path to the upload keystore (.jks)
//   CADENZA_UPLOAD_STORE_PASSWORD  keystore password
//   CADENZA_UPLOAD_KEY_ALIAS       key alias
//   CADENZA_UPLOAD_KEY_PASSWORD    key password
// Without them, release builds keep the debug key (for sideloaded test APKs).
const { withAppBuildGradle } = require("expo/config-plugins");

const RELEASE_CONFIG = `
        release {
            // Play upload key, from CI environment variables (see plugins/release-signing.js).
            if (System.getenv('CADENZA_UPLOAD_STORE_FILE')) {
                storeFile file(System.getenv('CADENZA_UPLOAD_STORE_FILE'))
                storePassword System.getenv('CADENZA_UPLOAD_STORE_PASSWORD')
                keyAlias System.getenv('CADENZA_UPLOAD_KEY_ALIAS')
                keyPassword System.getenv('CADENZA_UPLOAD_KEY_PASSWORD')
            }
        }`;

const MARK = "// cadenza: release signing";

/** setReleaseSigning adds a release signing config and uses it when the upload key is set. */
function setReleaseSigning(gradle) {
  if (gradle.includes(MARK)) return gradle;
  const configs = /signingConfigs \{\n/;
  if (!configs.test(gradle)) throw new Error("app/build.gradle has no signingConfigs block");
  let out = gradle.replace(configs, (m) => m + `        ${MARK}${RELEASE_CONFIG}\n`);
  const release = /(buildTypes \{[\s\S]*?release \{[\s\S]*?)signingConfig signingConfigs\.debug/;
  if (!release.test(out)) throw new Error("app/build.gradle has no release build type using the debug key");
  out = out.replace(release, "$1signingConfig System.getenv('CADENZA_UPLOAD_STORE_FILE') ? signingConfigs.release : signingConfigs.debug");
  return out;
}

module.exports = (config) =>
  withAppBuildGradle(config, (c) => {
    c.modResults.contents = setReleaseSigning(c.modResults.contents);
    return c;
  });
module.exports.setReleaseSigning = setReleaseSigning;
