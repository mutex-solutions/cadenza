import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { timeAgo, type NotificationEntry } from "@cadenza/core";
import { Button, Header, Message } from "../components/ui";
import { errorText, useApp } from "../context";
import { disableNotifications, enableNotifications } from "../native/notifications";
import { useTheme } from "../theme";

export function SettingsScreen() {
  const { colors } = useTheme();
  const { client, session, nav, logout, serverUrl } = useApp();
  const [deviceId, setDeviceId] = useState<string | null>(null);
  const [log, setLog] = useState<NotificationEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");

  const load = useCallback(async () => {
    setDeviceId(await session.deviceId());
    try {
      setLog((await client.call("notifications")).items);
    } catch (err) {
      setError(errorText(err));
    }
  }, [client, session]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(f: () => Promise<void>, done?: string) {
    setBusy(true);
    setError("");
    setInfo("");
    try {
      await f();
      if (done) setInfo(done);
      await load();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={{ flex: 1, backgroundColor: colors.bg }}>
      <Header title="Settings" left={<Button title="Back" onPress={() => nav({ type: "pop" })} />} />
      <ScrollView contentContainerStyle={{ padding: 16, gap: 10 }}>
        <Text style={[styles.h, { color: colors.text }]}>Notifications</Text>
        <Text style={{ color: colors.muted }}>
          {deviceId ? "This phone gets a notification when an agent asks a question or has work ready." : "Notifications are off on this phone."}
        </Text>
        <View style={styles.row}>
          {deviceId ? (
            <Button title="Turn off" onPress={() => run(() => disableNotifications(client, session), "Notifications are off.")} busy={busy} />
          ) : (
            <Button title="Turn on" kind="primary" onPress={() => run(() => enableNotifications(client, session), "Notifications are on.")} busy={busy} />
          )}
          <Button title="Send a test" onPress={() => run(async () => void (await client.call("notification-test")), "Test sent. It should arrive in a few seconds.")} disabled={busy} />
        </View>
        {info ? <Message>{info}</Message> : null}
        {error ? <Message kind="error">{error}</Message> : null}
        <Text style={[styles.h, { color: colors.text }]}>Recent notifications</Text>
        {log.length === 0 ? <Text style={{ color: colors.muted }}>None sent yet.</Text> : null}
        {log.slice(0, 30).map((n) => (
          <View key={n.id} style={[styles.item, { borderColor: colors.line }]}>
            <Text style={{ color: colors.text, fontWeight: "600" }}>{n.title}</Text>
            {n.body ? <Text style={{ color: colors.muted }}>{n.body}</Text> : null}
            <Text style={{ color: n.status === "failed" ? colors.danger : colors.muted, fontSize: 12 }}>
              {n.provider} · {n.status === "failed" ? `failed: ${n.error ?? "unknown error"}` : n.status} · {timeAgo(n.created_at)}
            </Text>
          </View>
        ))}
        <Text style={[styles.h, { color: colors.text }]}>Account</Text>
        <Text style={{ color: colors.muted }}>Connected to {serverUrl}</Text>
        <View style={styles.row}>
          <Button title="Log out" kind="danger" onPress={() => void logout()} />
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  h: { fontSize: 17, fontWeight: "700", marginTop: 12 },
  row: { flexDirection: "row", gap: 8, flexWrap: "wrap" },
  item: { borderWidth: 1, borderRadius: 10, padding: 10 },
});
