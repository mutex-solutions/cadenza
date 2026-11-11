import { useEffect, useRef } from "react";
import { timeAgo, type PostView, type ReplyView } from "@cadenza/core";
import { renderPost, type Marker, type PostViewer } from "@cadenza/viewer";
import { useServices } from "../app/services.tsx";
import { Html } from "./Html.tsx";
import { ReplyItem } from "./ReplyItem.tsx";

/** Numbers pinned comments on a post 1, 2, 3… in the order they were sent. */
export function markersFor(replies: ReplyView[]): Marker[] {
  return replies.filter((r) => r.kind === "comment" && r.spot).map((r, i) => ({ id: r.id, spot: r.spot!, label: String(i + 1) }));
}

/** One post: its title, summary and attachments (drawn by the shared viewer), then the replies on it. */
export function PostCard({
  post,
  replies,
  onViewer,
}: {
  post: PostView;
  replies: ReplyView[];
  onViewer: (postId: string, v: PostViewer | null) => void;
}) {
  const { resolveFile } = useServices();
  const box = useRef<HTMLDivElement>(null);
  const viewer = useRef<PostViewer | null>(null);
  const markers = markersFor(replies);
  const markerKey = markers.map((m) => m.id).join(",");

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const v = renderPost(el, post.attachments, {
      resolveFile,
      onMarkerClick: (id) => document.getElementById(`reply-${id}`)?.scrollIntoView({ behavior: "smooth", block: "center" }),
    });
    viewer.current = v;
    onViewer(post.id, v);
    return () => {
      onViewer(post.id, null);
      viewer.current = null;
      v.destroy();
    };
    // Posts are final (plan D5): the attachments never change for an id.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [post.id]);

  useEffect(() => {
    viewer.current?.setMarkers(markers);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [markerKey, post.id]);

  const labels = new Map(markers.map((m) => [m.id, m.label]));
  return (
    <article className="post" id={`post-${post.id}`} aria-labelledby={`post-title-${post.id}`}>
      <div className="post-head">
        <div className="round">
          {/^round\b/i.test(post.title.trim()) ? timeAgo(post.created_at) : `Round ${post.seq} · ${timeAgo(post.created_at)}`}
        </div>
        <h2 id={`post-title-${post.id}`}>{post.title}</h2>
      </div>
      {post.summary_html && <Html className="post-summary" html={post.summary_html} />}
      <div className="post-body" ref={box} />
      {replies.length > 0 && (
        <div className="post-replies" aria-label="Your replies on this post">
          {replies.map((r) => (
            <ReplyItem key={r.id} reply={r} post={post} marker={labels.get(r.id)} />
          ))}
        </div>
      )}
    </article>
  );
}
