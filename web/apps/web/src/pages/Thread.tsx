import { useCallback, useEffect, useRef, useState } from "react";
import { agentLabel, statusLabel, type QuestionView, type ReplyView, type ThreadView } from "@cadenza/core";
import type { PostViewer } from "@cadenza/viewer";
import { Link } from "../app/router.tsx";
import { useServices, useSync } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { PostCard } from "../components/PostCard.tsx";
import { QuestionCard } from "../components/QuestionCard.tsx";
import { ReplyPanel } from "../components/ReplyPanel.tsx";
import { useAction } from "../lib/useAsync.ts";

/** Open questions first; the ones the agent is blocked on at the top. */
export function sortQuestions(qs: QuestionView[]): { open: QuestionView[]; closed: QuestionView[] } {
  const open = qs.filter((q) => q.status === "open").sort((a, b) => Number(b.waiting) - Number(a.waiting) || a.created_at - b.created_at);
  const closed = qs.filter((q) => q.status !== "open").sort((a, b) => b.created_at - a.created_at);
  return { open, closed };
}

/** One thread: the agent's posts top to bottom, its questions, and your replies. */
export default function ThreadPage({ id }: { id: string }) {
  const { sync, client } = useServices();
  const s = useSync();
  const view: ThreadView | undefined = s.threads[id];
  const [error, setError] = useState<unknown>(null);
  const viewers = useRef(new Map<string, PostViewer>());

  useEffect(() => {
    setError(null);
    sync.openThread(id).catch((e: unknown) => setError(e));
  }, [id, sync]);

  // Looking at the thread marks it read, including changes that arrive while it's open.
  const updated = view?.thread.updated_at;
  useEffect(() => {
    if (updated !== undefined) sync.markSeen(id);
  }, [id, updated, sync]);

  useEffect(() => {
    if (view) document.title = `${view.thread.title} · Cadenza`;
  }, [view]);

  const refresh = useCallback(() => void sync.refreshThread(id).catch(() => undefined), [id, sync]);
  const onViewer = useCallback((postId: string, v: PostViewer | null) => {
    if (v) viewers.current.set(postId, v);
    else viewers.current.delete(postId);
  }, []);

  const setStatus = useAction(async (status: "archived" | "active") => {
    await client.call("thread-status", { params: { id }, body: { status } });
    refresh();
  });

  if (!view) {
    return error ? (
      <div className="page-narrow">
        <ErrorText error={error} />
        <Link to="/">Back to the inbox</Link>
      </div>
    ) : (
      <p className="muted">Loading…</p>
    );
  }

  const { thread, posts, questions, replies } = view;
  const { open, closed } = sortQuestions(questions);
  const repliesByPost = new Map<string, ReplyView[]>();
  const answers = new Map<string, ReplyView>();
  for (const r of replies) {
    if (r.question) answers.set(r.question, r);
    else if (r.post) repliesByPost.set(r.post, [...(repliesByPost.get(r.post) ?? []), r]);
  }
  const postById = new Map(posts.map((p) => [p.id, p]));
  const archived = thread.status === "archived";

  return (
    <div>
      <div className="thread-head">
        <Link to="/" className="btn ghost" aria-label="Back to the inbox">
          ←
        </Link>
        <div style={{ flex: 1, minWidth: 0 }}>
          <h1>{thread.title}</h1>
          <div className="meta">
            <span>
              {agentLabel(thread.agent)} · {thread.machine}
            </span>
            <span className="pill" data-status={thread.status}>
              {statusLabel(thread.status)}
            </span>
          </div>
        </div>
        <button className="ghost" onClick={() => void setStatus.run(archived ? "active" : "archived")} disabled={setStatus.busy}>
          {archived ? "Reopen" : "Archive"}
        </button>
      </div>
      <ErrorText error={setStatus.error} />
      <div className="thread-layout">
        <div>
          {posts.length === 0 && <p className="empty">The agent hasn't shown anything in this thread yet.</p>}
          {posts.map((p) => (
            <PostCard key={p.id} post={p} replies={repliesByPost.get(p.id) ?? []} onViewer={onViewer} />
          ))}
        </div>
        <aside className="side" id="respond" aria-label="Questions and replies">
          {open.length > 0 && (
            <section className="panel" aria-labelledby="q-title">
              <h2 id="q-title">
                {open.length === 1 ? "1 question" : `${open.length} questions`}
              </h2>
              {open.map((q) => (
                <QuestionCard key={q.id} q={q} post={q.post_id ? postById.get(q.post_id) : undefined} onDone={refresh} />
              ))}
            </section>
          )}
          <ReplyPanel posts={posts} viewerFor={(pid) => viewers.current.get(pid)} onSent={refresh} />
          {closed.length > 0 && (
            <details className="panel">
              <summary>Earlier questions ({closed.length})</summary>
              <div style={{ marginTop: 10 }}>
                {closed.map((q) => (
                  <QuestionCard key={q.id} q={q} post={q.post_id ? postById.get(q.post_id) : undefined} answer={answers.get(q.id)} onDone={refresh} />
                ))}
              </div>
            </details>
          )}
        </aside>
      </div>
      <div className="jump-bar">
        <button className={open.length ? "primary" : ""} onClick={() => document.getElementById("respond")?.scrollIntoView({ behavior: "smooth" })}>
          {open.length ? `${open.length} ${open.length === 1 ? "question" : "questions"} · Reply` : "Reply"}
        </button>
      </div>
    </div>
  );
}
