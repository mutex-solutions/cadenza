import { timeAgo, type PostView, type ReplyView } from "@cadenza/core";
import { useServices } from "../app/services.tsx";
import { describeSpot } from "../lib/spot.ts";

const KIND: Record<string, string> = {
  note: "Note",
  comment: "Comment",
  sketch: "Sketch",
  answer: "Answer",
};

function verdictText(v?: string): string {
  return v === "approve" ? "Approved" : v === "request_changes" ? "Requested changes" : "Verdict";
}

/** A reply the owner sent, the agent's answer to it, and whether the agent has picked it up. */
export function ReplyItem({ reply: r, post, marker }: { reply: ReplyView; post?: PostView; marker?: string }) {
  const { resolveFile } = useServices();
  const open = async (sha: string) => {
    // Open the tab now (browsers block pop-ups opened after an await), then point it at the signed link.
    const w = window.open("about:blank", "_blank");
    if (w) w.opener = null;
    try {
      const url = await resolveFile(sha);
      if (w) w.location.href = url;
      else window.location.assign(url);
    } catch {
      w?.close();
    }
  };
  const labels = (r.extra as { labels?: unknown } | undefined)?.labels;
  return (
    <div className="reply" id={`reply-${r.id}`}>
      <div className="row" style={{ gap: 6 }}>
        {marker && <span className="marker-label" aria-label={`Pin ${marker}`}>{marker}</span>}
        <span className="reply-kind">{r.kind === "verdict" ? verdictText(r.verdict) : (KIND[r.kind] ?? r.kind)}</span>
        {r.spot && <span className="muted small">on {describeSpot(r.spot, post)}</span>}
        {r.choice && <span className="pill">{r.choice}</span>}
        <span className="spacer" />
        <span className="delivered">
          {timeAgo(r.created_at)} · {r.delivered ? "Agent has it" : "Not picked up yet"}
        </span>
      </div>
      {r.text && <div style={{ whiteSpace: "pre-wrap" }}>{r.text}</div>}
      {Array.isArray(labels) && labels.length > 0 && <div className="muted small">Labels: {labels.map(String).join(", ")}</div>}
      {r.files && r.files.length > 0 && (
        <div className="reply-files">
          {r.files.map((f) => (
            <button key={f.sha256} className="ghost small" onClick={() => void open(f.sha256)}>
              {f.name}
            </button>
          ))}
        </div>
      )}
      {r.agent_reply && (
        <div className="agent-reply">
          <span className="small">Agent replied{r.agent_reply_at ? ` · ${timeAgo(r.agent_reply_at)}` : ""}:</span>
          <div style={{ whiteSpace: "pre-wrap" }}>{r.agent_reply}</div>
        </div>
      )}
    </div>
  );
}
