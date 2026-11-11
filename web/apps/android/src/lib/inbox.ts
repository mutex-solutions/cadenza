import { agentLabel, isUnread, statusLabel, timeAgo, type ThreadSummary } from "@cadenza/core";

/** One inbox row, ready to show. */
export interface InboxRow {
  id: string;
  title: string;
  subtitle: string;
  status: string;
  questions: number;
  unread: boolean;
  time: string;
}

/** Inbox rows: threads with open questions first, then newest change first. */
export function inboxRows(threads: ThreadSummary[], seen: Record<string, number>, now: number = Date.now()): InboxRow[] {
  return [...threads]
    .sort((a, b) => Number(b.open_questions > 0) - Number(a.open_questions > 0) || b.updated_at - a.updated_at)
    .map((t) => ({
      id: t.id,
      title: t.title || "Untitled",
      subtitle: `${agentLabel(t.agent)} · ${t.machine || "unknown machine"}`,
      status: statusLabel(t.status),
      questions: t.open_questions,
      unread: isUnread(t, seen),
      time: timeAgo(t.updated_at, now),
    }));
}

/** The bottom bar label on a thread, e.g. "2 questions · Reply". */
export function replyBarLabel(openQuestions: number): string {
  if (openQuestions <= 0) return "Reply";
  return `${openQuestions} ${openQuestions === 1 ? "question" : "questions"} · Reply`;
}
