// Expo reads app.json, then this file. CI sets the version for each build:
//   CADENZA_VERSION_NAME  shown to users, e.g. "0.1.5" (from the git tag)
//   CADENZA_VERSION_CODE  a whole number that must grow with every upload to Google Play
// Local builds keep the values in app.json.
function withVersion(config, env) {
  const name = env.CADENZA_VERSION_NAME;
  const code = env.CADENZA_VERSION_CODE;
  if (code !== undefined && !/^[1-9][0-9]{0,9}$/.test(code)) {
    throw new Error(`CADENZA_VERSION_CODE must be a positive whole number, got "${code}"`);
  }
  return {
    ...config,
    version: name || config.version,
    android: { ...config.android, versionCode: code ? Number(code) : (config.android?.versionCode ?? 1) },
  };
}

module.exports = ({ config }) => withVersion(config, process.env);
module.exports.withVersion = withVersion;
