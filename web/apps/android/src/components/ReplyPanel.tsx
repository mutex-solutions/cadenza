import { useState } from "react";
import {
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { FORM_MAX_WIDTH } from "../lib/layout";
import {
  uploadFiles,
  type QuestionView,
  type ReplyView,
  type Spot,
  type ThreadView,
  type UploadItem,
} from "@cadenza/core";
import { errorText, useApp } from "../context";
import { htmlToText } from "../lib/text";
import { pickPhotos } from "../native/photos";
import { useTheme, type Colors } from "../theme";
import { Button, Message } from "./ui";

interface Props {
  view: ThreadView;
  visible: boolean;
  onClose: () => void;
  /** A spot the owner picked for a pinned comment, if any. */
  spot: { postId: string; spot: Spot } | null;
  onPickSpot: (postId: string) => void;
  onClearSpot: () => void;
  onSent: () => void;
}

function describeSpot(s: Spot): string {
  const parts = [`attachment ${s.attachment + 1}`];
  if (s.block) parts.push(s.block);
  if (s.line) parts.push(`line ${s.line}${s.side ? ` (${s.side})` : ""}`);
  if (s.time_ms !== undefined)
    parts.push(`at ${Math.floor(s.time_ms / 1000)} s`);
  if (s.x !== undefined && s.y !== undefined)
    parts.push(`at ${Math.round(s.x * 100)}%, ${Math.round(s.y * 100)}%`);
  return parts.join(" · ");
}

function replyLabel(r: ReplyView): string {
  switch (r.kind) {
    case "answer":
      return r.choice ? `Answer: ${r.choice}` : "Answer";
    case "verdict":
      return r.verdict === "approve" ? "Approved" : "Requested changes";
    case "comment":
      return "Pinned comment";
    case "sketch":
      return "Sketch";
    default:
      return "Note";
  }
}

/** Questions, the reply form, and replies already sent. */
export function ReplyPanel(props: Props) {
  const { colors } = useTheme();
  const { view } = props;
  const open = view.questions.filter((q) => q.status === "open");
  const lastPost = view.posts[view.posts.length - 1];
  return (
    <Modal
      visible={props.visible}
      animationType="slide"
      onRequestClose={props.onClose}
      supportedOrientations={["portrait", "landscape"]}
    >
      <View style={{ flex: 1, backgroundColor: colors.bg }}>
        {/* On tablets and unfolded foldables the panel is a centred column, not stretched across. */}
        <View
          style={{
            flex: 1,
            width: "100%",
            maxWidth: FORM_MAX_WIDTH + 120,
            alignSelf: "center",
          }}
        >
          <View style={[styles.top, { borderBottomColor: colors.line }]}>
            <Text
              accessibilityRole="header"
              style={[styles.h, { color: colors.text }]}
            >
              Reply
            </Text>
            <Button title="Close" onPress={props.onClose} />
          </View>
          <ScrollView
            contentContainerStyle={{ padding: 14, paddingBottom: 48 }}
            keyboardShouldPersistTaps="handled"
          >
            {open.map((q) => (
              <QuestionCard
                key={q.id}
                q={q}
                colors={colors}
                onSent={props.onSent}
              />
            ))}
            {lastPost ? (
              <ReplyForm
                {...props}
                postId={props.spot?.postId ?? lastPost.id}
                colors={colors}
              />
            ) : null}
            {view.replies.length ? (
              <Text style={[styles.section, { color: colors.muted }]}>
                Your replies
              </Text>
            ) : null}
            {view.replies.map((r) => (
              <View
                key={r.id}
                style={[styles.card, { borderColor: colors.line }]}
              >
                <Text style={{ color: colors.text, fontWeight: "600" }}>
                  {replyLabel(r)}
                </Text>
                {r.question_text ? (
                  <Text style={{ color: colors.muted }}>
                    {htmlToText(r.question_text)}
                  </Text>
                ) : null}
                {r.text ? (
                  <Text style={{ color: colors.text, marginTop: 4 }}>
                    {r.text}
                  </Text>
                ) : null}
                {r.files?.length ? (
                  <Text style={{ color: colors.muted }}>
                    {r.files.length} photo(s)
                  </Text>
                ) : null}
                {r.agent_reply ? (
                  <Text style={{ color: colors.good, marginTop: 6 }}>
                    Agent: {r.agent_reply}
                  </Text>
                ) : null}
                <Text
                  style={{ color: colors.muted, fontSize: 12, marginTop: 4 }}
                >
                  {r.delivered ? "Delivered to the agent" : "Not picked up yet"}
                </Text>
              </View>
            ))}
          </ScrollView>
        </View>
      </View>
    </Modal>
  );
}

function usePhotos() {
  const [photos, setPhotos] = useState<UploadItem[]>([]);
  const [error, setError] = useState("");
  async function add(source: "library" | "camera") {
    try {
      const more = await pickPhotos(source);
      setPhotos((p) => [...p, ...more].slice(0, 10));
      setError("");
    } catch (err) {
      setError(errorText(err));
    }
  }
  return { photos, setPhotos, error, add };
}

function QuestionCard(props: {
  q: QuestionView;
  colors: Colors;
  onSent: () => void;
}) {
  const { client } = useApp();
  const { q, colors } = props;
  const [choice, setChoice] = useState<string | undefined>();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ph = usePhotos();

  async function send() {
    setBusy(true);
    setError("");
    try {
      const files = await uploadFiles(client, ph.photos);
      await client.call("answer", {
        params: { id: q.id },
        body: {
          choice,
          text: text.trim() || undefined,
          files: files.length ? files : undefined,
        },
      });
      setText("");
      ph.setPhotos([]);
      props.onSent();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function dismiss() {
    setBusy(true);
    try {
      await client.call("dismiss", { params: { id: q.id } });
      props.onSent();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={[styles.card, { borderColor: colors.accent }]}>
      <Text style={{ color: colors.muted, fontSize: 12 }}>
        {q.waiting ? "The agent is waiting for this" : "Question"}
      </Text>
      <Text style={{ color: colors.text, fontSize: 16, marginVertical: 6 }}>
        {htmlToText(q.text_html)}
      </Text>
      <View style={styles.wrap}>
        {(q.choices ?? []).map((c) => (
          <Pressable
            key={c}
            accessibilityRole="radio"
            accessibilityState={{ checked: choice === c }}
            onPress={() => setChoice(choice === c ? undefined : c)}
            style={[
              styles.chip,
              {
                borderColor: colors.accent,
                backgroundColor: choice === c ? colors.accent : "transparent",
              },
            ]}
          >
            <Text
              style={{
                color: choice === c ? colors.accentText : colors.accent,
              }}
            >
              {c}
            </Text>
          </Pressable>
        ))}
      </View>
      <TextInput
        accessibilityLabel="Your answer"
        placeholder={
          q.choices?.length ? "Add a note (optional)" : "Your answer"
        }
        placeholderTextColor={colors.muted}
        multiline
        value={text}
        onChangeText={setText}
        style={[styles.input, { color: colors.text, borderColor: colors.line }]}
      />
      <PhotoRow count={ph.photos.length} onAdd={ph.add} colors={colors} />
      {ph.error || error ? (
        <Message kind="error">{ph.error || error}</Message>
      ) : null}
      <View style={styles.row}>
        <Button
          title="Send answer"
          kind="primary"
          onPress={send}
          busy={busy}
          disabled={!choice && !text.trim() && !ph.photos.length}
        />
        <Button title="Dismiss" onPress={dismiss} disabled={busy} />
      </View>
    </View>
  );
}

function PhotoRow(props: {
  count: number;
  onAdd: (s: "library" | "camera") => void;
  colors: Colors;
}) {
  return (
    <View style={styles.row}>
      <Button title="Add photo" onPress={() => props.onAdd("library")} />
      <Button title="Take photo" onPress={() => props.onAdd("camera")} />
      {props.count ? (
        <Text style={{ color: props.colors.muted, alignSelf: "center" }}>
          {props.count} attached
        </Text>
      ) : null}
    </View>
  );
}

function ReplyForm(props: Props & { postId: string; colors: Colors }) {
  const { client } = useApp();
  const { colors } = props;
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ph = usePhotos();

  async function send(
    kind: "note" | "comment" | "verdict",
    verdict?: "approve" | "request_changes",
  ) {
    setBusy(true);
    setError("");
    try {
      const files = await uploadFiles(client, ph.photos);
      await client.call("add-reply", {
        body: {
          post_id: props.postId,
          kind,
          text: text.trim() || undefined,
          verdict,
          spot: kind === "comment" ? props.spot?.spot : undefined,
          files: files.length ? files : undefined,
        },
      });
      setText("");
      ph.setPhotos([]);
      props.onClearSpot();
      props.onSent();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  const hasBody = !!text.trim() || ph.photos.length > 0;
  return (
    <View style={[styles.card, { borderColor: colors.line }]}>
      <Text style={{ color: colors.text, fontWeight: "600" }}>
        About the latest work
      </Text>
      {props.spot ? (
        <View style={styles.row}>
          <Text style={{ color: colors.muted, flex: 1 }}>
            Pinned to {describeSpot(props.spot.spot)}
          </Text>
          <Button title="Remove pin" onPress={props.onClearSpot} />
        </View>
      ) : null}
      <TextInput
        accessibilityLabel="Your note"
        placeholder={props.spot ? "Your comment on this spot" : "Write a note"}
        placeholderTextColor={colors.muted}
        multiline
        value={text}
        onChangeText={setText}
        style={[styles.input, { color: colors.text, borderColor: colors.line }]}
      />
      <PhotoRow count={ph.photos.length} onAdd={ph.add} colors={colors} />
      {ph.error || error ? (
        <Message kind="error">{ph.error || error}</Message>
      ) : null}
      <View style={styles.row}>
        {props.spot ? (
          <Button
            title="Send comment"
            kind="primary"
            onPress={() => send("comment")}
            busy={busy}
            disabled={!text.trim()}
          />
        ) : (
          <>
            <Button
              title="Send note"
              kind="primary"
              onPress={() => send("note")}
              busy={busy}
              disabled={!hasBody}
            />
            <Button
              title="Pin to a spot"
              onPress={() => props.onPickSpot(props.postId)}
              disabled={busy}
            />
          </>
        )}
      </View>
      <View style={styles.row}>
        <Button
          title="Approve"
          onPress={() => send("verdict", "approve")}
          disabled={busy}
        />
        <Button
          title="Request changes"
          kind="danger"
          onPress={() => send("verdict", "request_changes")}
          disabled={busy}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  top: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    padding: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  h: { fontSize: 18, fontWeight: "700", marginLeft: 6 },
  card: {
    borderWidth: 1,
    borderRadius: 12,
    padding: 12,
    marginBottom: 14,
    gap: 4,
  },
  section: { fontSize: 13, marginVertical: 8 },
  wrap: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginBottom: 6 },
  chip: {
    borderWidth: 1,
    borderRadius: 18,
    paddingHorizontal: 14,
    paddingVertical: 8,
    minHeight: 40,
    justifyContent: "center",
  },
  input: {
    borderWidth: 1,
    borderRadius: 10,
    padding: 10,
    minHeight: 64,
    fontSize: 16,
    textAlignVertical: "top",
    marginVertical: 6,
  },
  row: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 6 },
});
