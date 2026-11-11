// Expo config plugin: keep the app running, and the user's place, when the
// screen changes under it: rotation, folding or unfolding a foldable (the
// inner and cover screens can have different sizes and densities), resizing
// in split screen or desktop windows, or a display-size setting change.
//
// The activity handles these changes itself (React Native re-lays out from
// useWindowDimensions) instead of Android destroying and recreating it, which
// would reset the app to the inbox.
const { withAndroidManifest, withGradleProperties } = require("expo/config-plugins");

const HANDLED = ["keyboard", "keyboardHidden", "orientation", "screenSize", "screenLayout", "smallestScreenSize", "density", "uiMode"];

/** setLargeScreen updates the main activity: handled config changes, resizable, no orientation lock. */
function setLargeScreen(manifest) {
  const app = manifest.manifest.application?.[0];
  const main = app?.activity?.find((a) => a.$?.["android:name"] === ".MainActivity");
  if (!main) throw new Error("AndroidManifest.xml has no .MainActivity");
  const have = new Set((main.$["android:configChanges"] ?? "").split("|").filter(Boolean));
  for (const c of HANDLED) have.add(c);
  main.$["android:configChanges"] = [...have].join("|");
  main.$["android:resizeableActivity"] = "true";
  delete main.$["android:screenOrientation"];
  return manifest;
}

/**
 * setLegacyPackaging stores native libraries compressed and unpacks them at
 * install. Without it the app loads them straight from the APK, which fails
 * on x86_64 emulators running the arm64 APK through translation (the app
 * crashed at start there). It also makes the download smaller.
 */
function setLegacyPackaging(props) {
  const out = props.filter((p) => !(p.type === "property" && p.key === "expo.useLegacyPackaging"));
  out.push({ type: "property", key: "expo.useLegacyPackaging", value: "true" });
  return out;
}

module.exports = (config) =>
  withGradleProperties(
    withAndroidManifest(config, (c) => {
      c.modResults = setLargeScreen(c.modResults);
      return c;
    }),
    (c) => {
      c.modResults = setLegacyPackaging(c.modResults);
      return c;
    },
  );
module.exports.setLargeScreen = setLargeScreen;
module.exports.setLegacyPackaging = setLegacyPackaging;
