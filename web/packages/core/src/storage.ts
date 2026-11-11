/** A tiny async key-value store. The sync engine keeps its offline copy here. */
export interface KV {
  get<T>(key: string): Promise<T | undefined>;
  set<T>(key: string, value: T): Promise<void>;
  delete(key: string): Promise<void>;
}

/** In-memory store, for tests and platforms without persistence. */
export function memoryStorage(initial: Record<string, unknown> = {}): KV & { dump(): Record<string, unknown> } {
  const m = new Map<string, unknown>(Object.entries(initial));
  return {
    async get<T>(key: string) {
      return structuredClone(m.get(key)) as T | undefined;
    },
    async set<T>(key: string, value: T) {
      m.set(key, structuredClone(value));
    },
    async delete(key: string) {
      m.delete(key);
    },
    dump: () => Object.fromEntries(m),
  };
}

function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error);
  });
}

/**
 * IndexedDB store (browser). Falls back to memory when IndexedDB is missing or
 * blocked (private windows), so the app still works, just without the copy.
 */
export function indexedDBStorage(dbName = "cadenza", factory: IDBFactory | null = globalThis.indexedDB ?? null): KV {
  const STORE = "kv";
  let dbp: Promise<IDBDatabase> | undefined;
  const fallback = memoryStorage();
  const open = (): Promise<IDBDatabase> => {
    if (!factory) return Promise.reject(new Error("no IndexedDB"));
    dbp ??= new Promise<IDBDatabase>((resolve, reject) => {
      const r = factory.open(dbName, 1);
      r.onupgradeneeded = () => r.result.createObjectStore(STORE);
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => reject(r.error);
      r.onblocked = () => reject(new Error("IndexedDB blocked"));
    });
    return dbp;
  };
  const withStore = async <T>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> => {
    const db = await open();
    return req(f(db.transaction(STORE, mode).objectStore(STORE)));
  };
  return {
    async get<T>(key: string) {
      try {
        return (await withStore("readonly", (s) => s.get(key))) as T | undefined;
      } catch {
        return fallback.get<T>(key);
      }
    },
    async set<T>(key: string, value: T) {
      try {
        await withStore("readwrite", (s) => s.put(value, key));
      } catch {
        await fallback.set(key, value);
      }
    },
    async delete(key: string) {
      try {
        await withStore("readwrite", (s) => s.delete(key));
      } catch {
        await fallback.delete(key);
      }
    },
  };
}
