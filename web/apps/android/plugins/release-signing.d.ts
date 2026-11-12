// Types for the release-signing config plugin (plain CommonJS, run by Expo at prebuild).
declare const plugin: ((config: unknown) => unknown) & { setReleaseSigning: (gradle: string) => string };
export = plugin;
