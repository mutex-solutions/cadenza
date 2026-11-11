// Types for the large-screens config plugin (plain CommonJS, run by Expo at prebuild).
type Activity = { $: Record<string, string> };
type Manifest = { manifest: { application?: Array<{ activity?: Activity[] }> } };
declare const plugin: ((config: unknown) => unknown) & { setLargeScreen: (m: Manifest) => Manifest };
export = plugin;
