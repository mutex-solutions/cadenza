import { useEffect, useState } from "react";
import { KeyboardAvoidingView, ScrollView, StyleSheet, Text } from "react-native";
import { FORM_MAX_WIDTH } from "../lib/layout";
import { createClient } from "@cadenza/core";
import { Button, Field, Message } from "../components/ui";
import { errorText } from "../context";
import { checkServerUrl } from "../lib/url";
import type { Session, SessionStore } from "../lib/session";
import { useTheme } from "../theme";

/** First screen: the server address and the owner password. */
export function ConnectScreen(props: { session: SessionStore; onConnected: (s: Session) => void }) {
  const { colors } = useTheme();
  const [url, setUrl] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");

  useEffect(() => {
    void props.session.lastServer().then((s) => s && setUrl(s));
  }, [props.session]);

  async function connect() {
    setError("");
    setInfo("");
    const check = checkServerUrl(url);
    if (!check.ok) return setError(check.error);
    if (check.warning) setInfo(check.warning);
    setBusy(true);
    try {
      const client = createClient({ baseUrl: check.url, token: () => null });
      const v = await client.call("version");
      if (v.setup_needed) {
        setError(`This server has no password yet. Open ${check.url} in a browser to set one, then log in here.`);
        return;
      }
      const out = await client.call("login", { body: { password } });
      const s = { serverUrl: check.url, token: out.token };
      await props.session.save(s);
      setPassword("");
      props.onConnected(s);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView behavior="height" style={{ flex: 1, backgroundColor: colors.bg }}>
      <ScrollView contentContainerStyle={styles.wrap} keyboardShouldPersistTaps="handled">
        <Text accessibilityRole="header" style={[styles.h1, { color: colors.text }]}>
          Cadenza
        </Text>
        <Text style={[styles.lead, { color: colors.muted }]}>See what your coding agents made, and answer their questions.</Text>
        <Field label="Server address" value={url} onChangeText={setUrl} placeholder="https://cadenza.example.com" autoCapitalize="none" autoCorrect={false} keyboardType="url" inputMode="url" />
        <Field label="Password" value={password} onChangeText={setPassword} secureTextEntry autoCapitalize="none" onSubmitEditing={connect} />
        {info ? <Message>{info}</Message> : null}
        {error ? <Message kind="error">{error}</Message> : null}
        <Button title="Log in" kind="primary" onPress={connect} busy={busy} disabled={!url || !password} />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  wrap: { padding: 20, paddingTop: 64, width: "100%", maxWidth: FORM_MAX_WIDTH, alignSelf: "center" },
  h1: { fontSize: 30, fontWeight: "800", marginBottom: 6 },
  lead: { fontSize: 16, marginBottom: 28 },
});
