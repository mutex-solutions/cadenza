/** The part of expo-secure-store the app uses; injectable for tests. */
export interface SecureStoreLike {
  getItemAsync(key: string): Promise<string | null>;
  setItemAsync(key: string, value: string): Promise<void>;
  deleteItemAsync(key: string): Promise<void>;
}

export interface Session {
  serverUrl: string;
  token: string;
}

export const TOKEN_KEY = "cadenza.token";
export const SERVER_KEY = "cadenza.server";
export const DEVICE_KEY = "cadenza.device";

/** Keeps the login token, server address and push device id in the phone's secure storage. */
export function createSessionStore(store: SecureStoreLike) {
  return {
    async load(): Promise<Session | null> {
      const [serverUrl, token] = await Promise.all([store.getItemAsync(SERVER_KEY), store.getItemAsync(TOKEN_KEY)]);
      return serverUrl && token ? { serverUrl, token } : null;
    },
    async save(s: Session): Promise<void> {
      await store.setItemAsync(SERVER_KEY, s.serverUrl);
      await store.setItemAsync(TOKEN_KEY, s.token);
    },
    /** Forgets the login and device, keeping the server address for next time. */
    async clear(): Promise<void> {
      await store.deleteItemAsync(TOKEN_KEY);
      await store.deleteItemAsync(DEVICE_KEY);
    },
    lastServer: () => store.getItemAsync(SERVER_KEY),
    deviceId: () => store.getItemAsync(DEVICE_KEY),
    setDeviceId: (id: string) => store.setItemAsync(DEVICE_KEY, id),
    clearDeviceId: () => store.deleteItemAsync(DEVICE_KEY),
  };
}
export type SessionStore = ReturnType<typeof createSessionStore>;
