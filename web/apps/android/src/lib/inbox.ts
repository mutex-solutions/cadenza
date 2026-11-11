import { agentLabel, isUnread, statusLabel, timeAgo, type Device, type ThreadSummary } from "@cadenza/core";

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
export function inboxRows(threads: ThreadSummary[], now: number = Date.now()): InboxRow[] {
  return [...threads]
    .sort((a, b) => Number(b.open_questions > 0) - Number(a.open_questions > 0) || b.updated_at - a.updated_at)
    .map((t) => ({
      id: t.id,
      title: t.title || "Untitled",
      subtitle: `${agentLabel(t.agent)} · ${t.machine || "unknown machine"}`,
      status: statusLabel(t.status),
      questions: t.open_questions,
      unread: isUnread(t),
      time: timeAgo(t.updated_at, now),
    }));
}

/** The bottom bar label on a thread, e.g. "2 questions · Reply". */
export function replyBarLabel(openQuestions: number): string {
  if (openQuestions <= 0) return "Reply";
  return `${openQuestions} ${openQuestions === 1 ? "question" : "questions"} · Reply`;
}

/**
 * Whether an open thread should be marked read now (thread-read): once when it
 * opens, and again whenever the server says it changed, but only while the app
 * is on screen.
 */
export function shouldMarkRead(o: { loaded: boolean; unread: boolean; active: boolean; markedThisOpen: boolean }): boolean {
  return o.loaded && o.active && (o.unread || !o.markedThisOpen);
}

/** One row in the devices list. */
export interface DeviceRow {
  id: string;
  name: string;
  detail: string;
  thisPhone: boolean;
}

/** The server's devices, with this phone marked (by the id saved when it registered). */
export function deviceRows(devices: Device[], thisId: string | null, now: number = Date.now()): DeviceRow[] {
  return devices.map((d) => ({
    id: d.id,
    name: d.name || d.provider,
    detail: `${d.provider === "expo" ? "Android app" : d.provider === "webpush" ? "Browser" : d.provider} · added ${timeAgo(d.created_at, now)} · ${
      d.last_ok_at ? `last delivered ${timeAgo(d.last_ok_at, now)}` : "nothing delivered yet"
    }`,
    thisPhone: d.id === thisId,
  }));
}
