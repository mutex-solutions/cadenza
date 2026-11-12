// Types for app.config.js (plain CommonJS, read by Expo).
type Config = { version?: string; android?: { versionCode?: number; [k: string]: unknown }; [k: string]: unknown };
declare const appConfig: ((arg: { config: Config }) => Config) & {
  withVersion: (config: Config, env: Record<string, string | undefined>) => Config;
};
export = appConfig;
