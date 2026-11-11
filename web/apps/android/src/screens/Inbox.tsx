import { useCallback, useEffect, useMemo, useState } from "react";
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from "react-native";
import type { ThreadSummary } from "@cadenza/core";
import { Button, Header, Message } from "../components/ui";
import { errorText, useApp, useSync } from "../context";
import { inboxRows, type InboxRow } from "../lib/inbox";
import { COLUMN_MAX_WIDTH } from "../lib/layout";
import { useTheme } from "../theme";

const CONNECTION: Record<string, string> = { connecting: "Connecting…", live: "Up to date", polling: "Up to date", offline: "Offline — showing saved copy" };

/** The thread list. selectedId highlights the thread open beside it (two-pane layout). */
export function InboxScreen(props: { selectedId: string | null }) {
  const { colors } = useTheme();
  const { client, sync, nav } = useApp();
  const state = useSync();
  const [archived, setArchived] = useState(false);
  const [archivedList, setArchivedList] = useState<ThreadSummary[] | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  const loadArchived = useCallback(async () => {
    try {
      const out = await client.call("threads", { query: { archived: 1 } });
      setArchivedList(out.threads.filter((t) => t.status === "archived"));
      setError("");
    } catch (err) {
      setError(errorText(err));
    }
  }, [client]);

  useEffect(() => {
    if (archived) void loadArchived();
  }, [archived, loadArchived]);

  const rows = useMemo(() => inboxRows(archived ? (archivedList ?? []) : state.inbox), [archived, archivedList, state.inbox]);

  async function refresh() {
    setRefreshing(true);
    if (archived) await loadArchived();
    else await sync.refreshInbox();
    setRefreshing(false);
  }

  const renderItem = ({ item }: { item: InboxRow }) => (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${item.title}. ${item.subtitle}. ${item.status}.${item.questions ? ` ${item.questions} open questions.` : ""}${item.unread ? " Unread." : ""}`}
      accessibilityState={{ selected: item.id === props.selectedId }}
      onPress={() => nav({ type: "openThread", id: item.id })}
      style={({ pressed }) => [
        styles.row,
        { borderBottomColor: colors.line, backgroundColor: pressed || item.id === props.selectedId ? colors.surface : colors.bg },
      ]}
    >
      <View style={[styles.dot, { backgroundColor: item.unread ? colors.accent : "transparent" }]} />
      <View style={{ flex: 1 }}>
        <Text numberOfLines={1} style={[styles.title, { color: colors.text, fontWeight: item.unread ? "700" : "500" }]}>
          {item.title}
        </Text>
        <Text numberOfLines={1} style={[styles.sub, { color: colors.muted }]}>
          {item.subtitle} · {item.status}
        </Text>
      </View>
      <View style={{ alignItems: "flex-end", gap: 4 }}>
        <Text style={[styles.sub, { color: colors.muted }]}>{item.time}</Text>
        {item.questions > 0 ? (
          <Text style={[styles.badge, { backgroundColor: colors.accent, color: colors.accentText }]}>
            {item.questions} ?
          </Text>
        ) : null}
      </View>
    </Pressable>
  );

  return (
    <View style={{ flex: 1, backgroundColor: colors.bg }}>
      <Header
        title={archived ? "Archived" : "Inbox"}
        left={<Button title={archived ? "Inbox" : "Archived"} onPress={() => setArchived(!archived)} />}
        right={<Button title="Settings" onPress={() => nav({ type: "push", screen: { name: "settings" } })} />}
      />
      <Text style={[styles.status, { color: colors.muted }]}>{CONNECTION[state.connection] ?? ""}</Text>
      {error || state.error ? <Message kind="error">{error || errorText(state.error)}</Message> : null}
      <FlatList
        style={styles.list}
        data={rows}
        keyExtractor={(r) => r.id}
        renderItem={renderItem}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={refresh} />}
        ListEmptyComponent={
          <Text style={[styles.empty, { color: colors.muted }]}>
            {!state.ready && !archived ? "Loading…" : archived ? "No archived threads." : "Nothing yet. When an agent shows work or asks a question, it appears here."}
          </Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 14, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, minHeight: 64 },
  dot: { width: 8, height: 8, borderRadius: 4 },
  title: { fontSize: 16 },
  sub: { fontSize: 13, marginTop: 2 },
  badge: { fontSize: 12, fontWeight: "700", paddingHorizontal: 8, paddingVertical: 2, borderRadius: 10, overflow: "hidden" },
  status: { fontSize: 12, textAlign: "center", paddingVertical: 4 },
  empty: { padding: 24, textAlign: "center", fontSize: 15 },
  // One column of threads never grows wider than this (medium screens in one pane).
  list: { width: "100%", maxWidth: COLUMN_MAX_WIDTH, alignSelf: "center" },
});
