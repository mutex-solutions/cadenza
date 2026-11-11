import { createContext, useContext, useSyncExternalStore } from "react";
import type { Client, SyncEngine, SyncState } from "@cadenza/core";
import type { FileUrlCache } from "./lib/fileUrls";
import type { NavAction } from "./lib/nav";
import type { SessionStore } from "./lib/session";

export interface AppContextValue {
  client: Client;
  sync: SyncEngine;
  session: SessionStore;
  serverUrl: string;
  files: FileUrlCache;
  nav: (a: NavAction) => void;
  logout: () => Promise<void>;
}

export const AppContext = createContext<AppContextValue | null>(null);

export function useApp(): AppContextValue {
  const v = useContext(AppContext);
  if (!v) throw new Error("useApp outside AppContext");
  return v;
}

export function useSync(): SyncState {
  const { sync } = useApp();
  return useSyncExternalStore(sync.subscribe, sync.getState);
}

/** A readable message for any error. */
export function errorText(err: unknown): string {
  if (err && typeof err === "object" && "message" in err) {
    const e = err as { message: string; fix?: string };
    return e.fix ? `${e.message}. ${e.fix}` : e.message;
  }
  return String(err);
}
