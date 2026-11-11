import { lazy, Suspense, useState } from "react";
import { withExtension, type PostView, type ReplyInput, type Spot } from "@cadenza/core";
import type { PostViewer } from "@cadenza/viewer";
import { useServices } from "../app/services.tsx";
import { describeSpot } from "../lib/spot.ts";
import { useAction } from "../lib/useAsync.ts";
import { ErrorText } from "./ErrorText.tsx";
import { FilePicker } from "./FilePicker.tsx";

const Sketch = lazy(() => import("./Sketch.tsx"));

/**
 * Reply on a post: a note, photos, a comment pinned to a spot, a sketch,
 * or a verdict (approve / request changes).
 */
export function ReplyPanel({
  posts,
  viewerFor,
  onSent,
}: {
  posts: PostView[];
  viewerFor: (postId: string) => PostViewer | undefined;
  onSent: () => void;
}) {
  const { client, upload } = useServices();
  const latest = posts[posts.length - 1];
  const [postId, setPostId] = useState<string | undefined>(undefined);
  const target = posts.find((p) => p.id === postId) ?? latest;
  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [spot, setSpot] = useState<Spot | null>(null);
  const [picking, setPicking] = useState(false);
  const [sketching, setSketching] = useState(false);
  const [sent, setSent] = useState("");

  const reset = (msg: string) => {
    setText("");
    setFiles([]);
    setSpot(null);
    setSent(msg);
    onSent();
  };

  const send = useAction(async (input: Omit<ReplyInput, "post_id" | "files">, msg: string, extraFiles: File[] = files) => {
    if (!target) return;
    const refs = await upload(extraFiles);
    await client.call("add-reply", { body: { post_id: target.id, ...input, files: refs.length ? refs : undefined } });
    reset(msg);
  });

  const pick = async () => {
    const v = target && viewerFor(target.id);
    if (!v) return;
    setPicking(true);
    setSent("");
    document.getElementById(`post-${target.id}`)?.scrollIntoView({ behavior: "smooth", block: "start" });
    try {
      const s = await v.pickSpot();
      if (s) setSpot(s);
    } finally {
      setPicking(false);
    }
  };

  const sendNote = () => {
    const t = text.trim();
    if (spot) void send.run({ kind: "comment", text: t, spot }, "Comment sent.");
    else void send.run({ kind: "note", text: t || undefined }, "Note sent.");
  };
  const verdict = (v: "approve" | "request_changes") =>
    void send.run({ kind: "verdict", verdict: v, text: text.trim() || undefined }, v === "approve" ? "Approved." : "Asked for changes.");

  if (!target) return null;
  const canNote = spot ? !!text.trim() : !!text.trim() || files.length > 0;

  return (
    <section className="panel" aria-labelledby="reply-title">
      <h2 id="reply-title">Reply</h2>
      {posts.length > 1 && (
        <div className="field">
          <label htmlFor="reply-post">On</label>
          <select
            id="reply-post"
            value={target.id}
            onChange={(e) => {
              setPostId(e.target.value);
              setSpot(null);
            }}
          >
            {posts.map((p) => (
              <option key={p.id} value={p.id}>
                Round {p.seq}: {p.title}
              </option>
            ))}
          </select>
        </div>
      )}
      <div className="field">
        <label htmlFor="reply-text">{spot ? "Your comment" : "Note"}</label>
        <textarea id="reply-text" value={text} onChange={(e) => setText(e.target.value)} placeholder={spot ? "What should change here?" : "Anything the agent should know"} />
      </div>
      {spot && (
        <p className="spot-chip">
          Pinned to {describeSpot(spot, target)}
          <button type="button" className="ghost" onClick={() => setSpot(null)} aria-label="Remove pin">
            ×
          </button>
        </p>
      )}
      <div className="row" style={{ marginBottom: 8 }}>
        <FilePicker files={files} onChange={setFiles} />
        <button type="button" onClick={() => void pick()} disabled={picking}>
          {picking ? "Tap a spot in the post…" : "Pin to a spot"}
        </button>
        <button type="button" onClick={() => setSketching(true)}>
          Sketch
        </button>
      </div>
      <div className="row">
        <button type="button" className="primary" onClick={sendNote} disabled={!canNote || send.busy}>
          {send.busy ? "Sending…" : spot ? "Send comment" : "Send note"}
        </button>
        <span className="spacer" />
        <button type="button" className="bad" onClick={() => verdict("request_changes")} disabled={send.busy}>
          Request changes
        </button>
        <button type="button" className="good" onClick={() => verdict("approve")} disabled={send.busy}>
          Approve
        </button>
      </div>
      <ErrorText error={send.error} />
      {sent && (
        <p className="small muted" role="status">
          {sent}
        </p>
      )}
      {sketching && (
        <Suspense fallback={null}>
          <Sketch
            onCancel={() => setSketching(false)}
            onDone={(png, labels) => {
              setSketching(false);
              const file = new File([png], withExtension("sketch", "png"), { type: "image/png" });
              void send.run({ kind: "sketch", text: text.trim() || undefined, extra: { labels } }, "Sketch sent.", [file]);
            }}
          />
        </Suspense>
      )}
    </section>
  );
}
