import { useEffect, useState } from "react";
import { agentLabel, isUnread, statusLabel, timeAgo, type ThreadSummary } from "@cadenza/core";
import { Link, threadPath } from "../app/router.tsx";
import { useServices, useSync } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";

export function ThreadRow({ t, unread }: { t: ThreadSummary; unread: boolean }) {
  return (
    <li>
      <Link to={threadPath(t.id)} className="thread-row" data-unread={unread}>
        <span className={unread ? "dot unread" : "dot"} aria-hidden="true" />
        <span className="thread-title">
          {unread && <span className="sr-only">Unread: </span>}
          {t.title}
        </span>
        <span className="time">{timeAgo(t.updated_at)}</span>
        <span className="thread-meta">
          <span>
            {agentLabel(t.agent)} · {t.machine}
          </span>
          <span className="pill" data-status={t.status}>
            {statusLabel(t.status)}
          </span>
          {t.open_questions > 0 && (
            <span className="badge">
              {t.open_questions} {t.open_questions === 1 ? "question" : "questions"}
            </span>
          )}
        </span>
      </Link>
    </li>
  );
}

/** Every thread from every machine, newest change first. */
export function Inbox() {
  const s = useSync();
  const open = s.inbox.reduce((n, t) => n + t.open_questions, 0);
  useEffect(() => {
    document.title = open > 0 ? `(${open}) Cadenza` : "Cadenza";
  }, [open]);

  if (!s.ready) {
    if (s.error) return <ErrorText error={s.error} />;
    return <p className="muted">Loading…</p>;
  }
  return (
    <div className="page-narrow">
      <div className="inbox-head">
        <h1>Inbox</h1>
        {open > 0 && <span className="badge">{open} open</span>}
        <span className="spacer" />
        <Link to="/archived" className="small">
          Archived
        </Link>
      </div>
      {s.error && !s.error.unauthorized && <ErrorText error={s.error} />}
      {s.inbox.length === 0 ? (
        <div className="empty">
          <p>Nothing here yet.</p>
          <p className="small">
            Pair a machine on the <Link to="/machines">Machines</Link> page, then agents there can show you their work.
          </p>
        </div>
      ) : (
        <ul className="threads" aria-label="Threads">
          {s.inbox.map((t) => (
            <ThreadRow key={t.id} t={t} unread={isUnread(t, s.seen)} />
          ))}
        </ul>
      )}
    </div>
  );
}

/** Archived threads, fetched on demand (they aren't kept offline). */
export function Archived() {
  const { client } = useServices();
  const [threads, setThreads] = useState<ThreadSummary[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let live = true;
    client
      .call("threads", { query: { archived: 1 } })
      .then((r) => live && setThreads(r.threads.filter((t) => t.status === "archived")))
      .catch((e: unknown) => live && setError(e));
    return () => {
      live = false;
    };
  }, [client]);
  return (
    <div className="page-narrow">
      <div className="inbox-head">
        <h1>Archived</h1>
        <span className="spacer" />
        <Link to="/" className="small">
          Back to inbox
        </Link>
      </div>
      <ErrorText error={error} />
      {threads === null && !error && <p className="muted">Loading…</p>}
      {threads?.length === 0 && <p className="empty">No archived threads.</p>}
      {threads && threads.length > 0 && (
        <ul className="threads" aria-label="Archived threads">
          {threads.map((t) => (
            <ThreadRow key={t.id} t={t} unread={false} />
          ))}
        </ul>
      )}
    </div>
  );
}
