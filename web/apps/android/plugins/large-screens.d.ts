// Types for the large-screens config plugin (plain CommonJS, run by Expo at prebuild).
type Activity = { $: Record<string, string> };
type Manifest = { manifest: { application?: Array<{ activity?: Activity[] }> } };
type Prop = { type: string; key?: string; value?: string };
declare const plugin: ((config: unknown) => unknown) & {
  setLargeScreen: (m: Manifest) => Manifest;
  setLegacyPackaging: (props: Prop[]) => Prop[];
};
export = plugin;
