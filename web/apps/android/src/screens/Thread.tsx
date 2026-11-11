import { useEffect, useMemo, useRef, useState } from "react";
import { AppState, Pressable, StyleSheet, Text, View } from "react-native";
import { agentLabel, statusLabel, type Spot, type ThreadView } from "@cadenza/core";
import { PostWebView, type PostWebViewHandle } from "../components/PostWebView";
import { ReplyPanel } from "../components/ReplyPanel";
import { Button, Header, Message } from "../components/ui";
import { errorText, useApp, useSync } from "../context";
import type { HostMarker, HostPost } from "../lib/bridge";
import { replyBarLabel, shouldMarkRead } from "../lib/inbox";
import { useTheme } from "../theme";

export function ThreadScreen(props: { id: string }) {
  const { colors } = useTheme();
  const { client, sync, nav, serverUrl, files } = useApp();
  const state = useSync();
  const view: ThreadView | undefined = state.threads[props.id];
  const [error, setError] = useState("");
  const [panel, setPanel] = useState(false);
  const [picking, setPicking] = useState(false);
  const [spot, setSpot] = useState<{ postId: string; spot: Spot } | null>(null);
  const web = useRef<PostWebViewHandle>(null);

  useEffect(() => {
    sync.openThread(props.id).catch((err: unknown) => setError(errorText(err)));
  }, [sync, props.id]);

  // Mark the thread read on the server when it opens, and again when changes
  // arrive while it's on screen (or when the app comes back to the front).
  const loaded = view !== undefined;
  const unread = view?.thread.unread === true;
  const markedThisOpen = useRef(false);
  useEffect(() => {
    const check = () => {
      const t = sync.getState().threads[props.id]?.thread;
      if (shouldMarkRead({ loaded: !!t, unread: t?.unread === true, active: AppState.currentState === "active", markedThisOpen: markedThisOpen.current })) {
        markedThisOpen.current = true;
        void sync.markRead(props.id);
      }
    };
    check();
    const sub = AppState.addEventListener("change", (s) => s === "active" && check());
    return () => sub.remove();
  }, [sync, props.id, loaded, unread]);

  const posts: HostPost[] = useMemo(
    () => (view?.posts ?? []).map((p) => ({ id: p.id, title: p.title, summary_html: p.summary_html, attachments: p.attachments })),
    [view?.posts],
  );
  const markers = useMemo(() => {
    const out: Record<string, HostMarker[]> = {};
    let n = 0;
    for (const r of view?.replies ?? []) {
      if (r.kind !== "comment" || !r.spot || !r.post) continue;
      (out[r.post] ??= []).push({ id: r.id, spot: r.spot, label: String(++n) });
    }
    return out;
  }, [view?.replies]);

  if (!view) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.bg }}>
        <Header title="Thread" left={<Button title="Back" onPress={() => nav({ type: "pop" })} />} />
        <Message kind={error ? "error" : "info"}>{error || "Loading…"}</Message>
      </View>
    );
  }

  const t = view.thread;
  const archived = t.status === "archived";
  async function setStatus(status: string) {
    try {
      await client.call("thread-status", { params: { id: t.id }, body: { status } });
      await sync.refreshThread(t.id);
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <View style={{ flex: 1, backgroundColor: colors.bg }}>
      <Header
        title={t.title}
        left={<Button title="Back" onPress={() => nav({ type: "pop" })} />}
        right={<Button title={archived ? "Reopen" : "Archive"} onPress={() => setStatus(archived ? "active" : "archived")} />}
      />
      <Text style={[styles.meta, { color: colors.muted }]}>
        {agentLabel(t.agent)} · {t.machine} · {statusLabel(t.status)}
      </Text>
      {error ? <Message kind="error">{error}</Message> : null}
      <PostWebView
        ref={web}
        serverUrl={serverUrl}
        posts={posts}
        markers={markers}
        resolveFile={(sha) => files.get(sha)}
        onSpot={(postId, s) => {
          setPicking(false);
          if (s) setSpot({ postId, spot: s });
          setPanel(true);
        }}
      />
      {picking ? (
        <View style={[styles.bar, { backgroundColor: colors.surface, borderTopColor: colors.line }]}>
          <Text style={{ color: colors.text, flex: 1 }}>Tap the spot you want to comment on.</Text>
          <Button
            title="Cancel"
            onPress={() => {
              web.current?.cancelPick();
              setPicking(false);
              setPanel(true);
            }}
          />
        </View>
      ) : (
        <Pressable
          accessibilityRole="button"
          onPress={() => setPanel(true)}
          style={[styles.bar, { backgroundColor: t.open_questions ? colors.accent : colors.surface, borderTopColor: colors.line }]}
        >
          <Text style={{ color: t.open_questions ? colors.accentText : colors.text, fontWeight: "700", fontSize: 16, textAlign: "center", flex: 1 }}>
            ▲ {replyBarLabel(view.questions.filter((q) => q.status === "open").length)}
          </Text>
        </Pressable>
      )}
      <ReplyPanel
        view={view}
        visible={panel}
        onClose={() => setPanel(false)}
        spot={spot}
        onPickSpot={(postId) => {
          setPanel(false);
          setPicking(true);
          web.current?.pick(postId);
        }}
        onClearSpot={() => setSpot(null)}
        onSent={() => void sync.refreshThread(t.id).catch(() => undefined)}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  meta: { fontSize: 13, textAlign: "center", paddingVertical: 4 },
  bar: { flexDirection: "row", alignItems: "center", gap: 8, minHeight: 56, paddingHorizontal: 14, borderTopWidth: StyleSheet.hairlineWidth },
});
