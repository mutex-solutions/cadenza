import { useCallback, useEffect, useMemo, useReducer, useState } from "react";
import { AppState, BackHandler, StyleSheet, Text, View, useWindowDimensions } from "react-native";
import { SafeAreaProvider, SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import { SyncEngine, createClient, memoryStorage } from "@cadenza/core";
import { AppContext, type AppContextValue } from "./src/context";
import { FileUrlCache } from "./src/lib/fileUrls";
import { layoutFor } from "./src/lib/layout";
import { navReducer, threadFromNotification } from "./src/lib/nav";
import { createSessionStore, type Session } from "./src/lib/session";
import { disableNotifications, onNotificationTap } from "./src/native/notifications";
import { secureStore } from "./src/native/secureStore";
import { ConnectScreen } from "./src/screens/Connect";
import { InboxScreen } from "./src/screens/Inbox";
import { SettingsScreen } from "./src/screens/Settings";
import { ThreadScreen } from "./src/screens/Thread";
import { useTheme } from "./src/theme";

const sessionStore = createSessionStore(secureStore);

export default function App() {
  const { colors, scheme } = useTheme();
  // Re-renders on rotation and fold/unfold; the activity is never restarted
  // (configChanges in the manifest), so all state below survives.
  const { width } = useWindowDimensions();
  const layout = layoutFor(width);
  const [session, setSession] = useState<Session | null | undefined>(undefined);
  const [stack, nav] = useReducer(navReducer, [{ name: "inbox" }]);

  useEffect(() => {
    void sessionStore.load().then(setSession, () => setSession(null));
  }, []);

  const logoutLocal = useCallback(async () => {
    await sessionStore.clear();
    setSession(null);
  }, []);

  const ctx = useMemo<Omit<AppContextValue, "nav" | "logout"> | null>(() => {
    if (!session) return null;
    const client = createClient({ baseUrl: session.serverUrl, token: () => session.token, onUnauthorized: () => void logoutLocal() });
    // React Native has no EventSource: follow changes by polling while the app is open.
    const sync = new SyncEngine({ client, storage: memoryStorage(), eventSource: null, pollMs: 5000 });
    const files = new FileUrlCache((sha256) => client.call("file-url", { params: { sha256 } }));
    return { client, sync, files, session: sessionStore, serverUrl: session.serverUrl };
  }, [session, logoutLocal]);

  useEffect(() => {
    if (!ctx) return;
    void ctx.sync.start();
    // Poll only while the app is on screen; push notifications cover the rest.
    const sub = AppState.addEventListener("change", (s) => (s === "active" ? ctx.sync.reconnect() : ctx.sync.stop()));
    return () => {
      sub.remove();
      ctx.sync.stop();
    };
  }, [ctx]);

  useEffect(() => {
    if (!session) return;
    return onNotificationTap((data) => {
      const id = threadFromNotification(data);
      if (id) nav({ type: "openThread", id });
    });
  }, [session]);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      if (stack.length > 1) {
        nav({ type: "pop" });
        return true;
      }
      return false;
    });
    return () => sub.remove();
  }, [stack.length]);

  const value = useMemo<AppContextValue | null>(() => {
    if (!ctx) return null;
    return {
      ...ctx,
      nav,
      logout: async () => {
        await disableNotifications(ctx.client, sessionStore).catch(() => undefined);
        await ctx.client.call("logout").catch(() => undefined);
        await ctx.sync.clear();
        await logoutLocal();
        nav({ type: "reset", screen: { name: "inbox" } });
      },
    };
  }, [ctx, logoutLocal]);

  const top = stack[stack.length - 1]!;
  let screen = null;
  if (session === null) screen = <ConnectScreen session={sessionStore} onConnected={setSession} />;
  else if (value) {
    if (top.name === "settings") screen = <SettingsScreen />;
    else if (layout.twoPane) {
      // Tablets and a foldable's inner screen: the inbox and the open thread side by side.
      const open = top.name === "thread" ? top.id : null;
      screen = (
        <View style={styles.row}>
          <View style={[styles.list, { width: layout.listWidth, borderRightColor: colors.line }]}>
            <InboxScreen selectedId={open} />
          </View>
          <View style={styles.detail}>
            {open ? (
              <ThreadScreen key={open} id={open} embedded />
            ) : (
              <View style={styles.empty}>
                <Text style={{ color: colors.muted, fontSize: 16, textAlign: "center" }}>Pick a thread to see the work and answer questions.</Text>
              </View>
            )}
          </View>
        </View>
      );
    } else if (top.name === "thread") screen = <ThreadScreen key={top.id} id={top.id} />;
    else screen = <InboxScreen selectedId={null} />;
  }

  return (
    <SafeAreaProvider>
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.bg }}>
        <StatusBar style={scheme === "dark" ? "light" : "dark"} />
        {value ? <AppContext.Provider value={value}>{screen}</AppContext.Provider> : <View style={{ flex: 1 }}>{screen}</View>}
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  row: { flex: 1, flexDirection: "row" },
  list: { borderRightWidth: StyleSheet.hairlineWidth },
  detail: { flex: 1 },
  empty: { flex: 1, alignItems: "center", justifyContent: "center", padding: 32 },
});
