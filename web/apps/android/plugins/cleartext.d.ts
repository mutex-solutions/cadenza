// Types for the cleartext config plugin (plain CommonJS, run by Expo at prebuild).
type Manifest = { manifest: { application?: Array<{ $?: Record<string, string> }> } };
declare const plugin: ((config: unknown) => unknown) & { setCleartext: (m: Manifest) => Manifest };
export = plugin;
