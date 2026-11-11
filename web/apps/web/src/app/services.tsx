import { createContext, useContext, useSyncExternalStore } from "react";
import {
  browserImageConverter,
  createClient,
  indexedDBStorage,
  SyncEngine,
  uploadFiles,
  type Client,
  type Event as ChangeEvent,
  type FileRef,
  type ImageConverter,
  type KV,
  type SyncState,
} from "@cadenza/core";
import { fileResolver, toUploadItems } from "../lib/files.ts";

/** Everything the screens need from the outside world; tests pass fakes. */
export interface Services {
  client: Client;
  sync: SyncEngine;
  converter: ImageConverter;
  resolveFile: (sha256: string) => Promise<string>;
  /** Converts picked files (photos to WebP) and stores them; returns refs to send. */
  upload: (files: File[]) => Promise<FileRef[]>;
  /** Subscribe to every change from the server (e.g. machines page). */
  onChange: (f: (e: ChangeEvent) => void) => () => void;
  /** Set by the app shell: called when the server says we're not logged in. */
  unauthorized: { handler: () => void };
}

export interface ServiceOptions {
  fetch?: typeof fetch;
  storage?: KV;
  eventSource?: ((url: string) => EventSource) | null;
  converter?: ImageConverter;
  pollMs?: number;
}

export function createServices(o: ServiceOptions = {}): Services {
  const unauthorized = { handler: () => {} };
  const client = createClient({ fetch: o.fetch, onUnauthorized: () => unauthorized.handler() });
  const changeListeners = new Set<(e: ChangeEvent) => void>();
  const es =
    o.eventSource !== undefined
      ? o.eventSource
      : typeof EventSource === "function"
        ? (url: string) => new EventSource(url, { withCredentials: true })
        : null;
  const sync = new SyncEngine({
    client,
    storage: o.storage ?? indexedDBStorage("cadenza"),
    eventSource: es,
    pollMs: o.pollMs,
    onEvent: (e) => {
      for (const f of changeListeners) f(e);
    },
  });
  const converter = o.converter ?? browserImageConverter();
  return {
    client,
    sync,
    converter,
    upload: async (files) => (files.length ? uploadFiles(client, await toUploadItems(files, converter), { fetch: o.fetch }) : []),
    resolveFile: fileResolver(client),
    onChange: (f) => {
      changeListeners.add(f);
      return () => changeListeners.delete(f);
    },
    unauthorized,
  };
}

const Ctx = createContext<Services | null>(null);
export const ServicesProvider = Ctx.Provider;

export function useServices(): Services {
  const s = useContext(Ctx);
  if (!s) throw new Error("ServicesProvider is missing");
  return s;
}

export function useSync(): SyncState {
  const { sync } = useServices();
  return useSyncExternalStore(sync.subscribe, sync.getState);
}
