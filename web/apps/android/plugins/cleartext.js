// Expo config plugin: allow plain http:// servers in release builds.
//
// Self-hosters often run Cadenza on a home network or a trial machine without
// HTTPS. Android blocks cleartext traffic in release builds by default, so the
// app could not reach such a server at all. The login screen warns when the
// address is not https://; this plugin makes the connection possible.
const { withAndroidManifest } = require("expo/config-plugins");

/** setCleartext marks the manifest's <application> as allowing cleartext traffic. */
function setCleartext(manifest) {
  const app = manifest.manifest.application?.[0];
  if (!app) throw new Error("AndroidManifest.xml has no <application>");
  app.$ = { ...app.$, "android:usesCleartextTraffic": "true" };
  return manifest;
}

module.exports = (config) =>
  withAndroidManifest(config, (c) => {
    c.modResults = setCleartext(c.modResults);
    return c;
  });
module.exports.setCleartext = setCleartext;
