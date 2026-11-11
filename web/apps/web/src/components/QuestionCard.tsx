import { useState, type FormEvent } from "react";
import { timeAgo, type PostView, type QuestionView, type ReplyView } from "@cadenza/core";
import { useServices } from "../app/services.tsx";
import { useAction } from "../lib/useAsync.ts";
import { ErrorText } from "./ErrorText.tsx";
import { FilePicker } from "./FilePicker.tsx";
import { Html } from "./Html.tsx";

const STATUS: Record<string, string> = { answered: "Answered", dismissed: "Dismissed", replaced: "Replaced by a newer question" };

/** A question from the agent: pick a choice, write an answer, attach photos. */
export function QuestionCard({
  q,
  post,
  answer,
  onDone,
}: {
  q: QuestionView;
  post?: PostView;
  answer?: ReplyView;
  onDone: () => void;
}) {
  const { client, upload } = useServices();
  const [choice, setChoice] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const isOpen = q.status === "open";

  const send = useAction(async () => {
    const refs = await upload(files);
    await client.call("answer", {
      params: { id: q.id },
      body: { choice: choice ?? undefined, text: text.trim() || undefined, files: refs.length ? refs : undefined },
    });
    setChoice(null);
    setText("");
    setFiles([]);
    onDone();
  });
  const dismiss = useAction(async () => {
    await client.call("dismiss", { params: { id: q.id } });
    onDone();
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void send.run();
  };
  const canSend = !!choice || !!text.trim() || files.length > 0;

  return (
    <section className={isOpen ? "question open" : "question"} aria-label="Question from the agent">
      <div className="row small muted" style={{ marginBottom: 6 }}>
        {q.waiting && isOpen && <span className="badge waiting">Agent is waiting</span>}
        {post && <span>About round {post.seq}</span>}
        <span className="spacer" />
        <span>{timeAgo(q.created_at)}</span>
      </div>
      <Html className="question-text" html={q.text_html} />
      {isOpen ? (
        <form onSubmit={submit}>
          {q.choices && q.choices.length > 0 && (
            <div className="choices" role="group" aria-label="Choices">
              {q.choices.map((c) => (
                <button type="button" key={c} className="choice" aria-pressed={choice === c} onClick={() => setChoice(choice === c ? null : c)}>
                  {c}
                </button>
              ))}
            </div>
          )}
          <div className="field">
            <label htmlFor={`a-${q.id}`}>{q.choices?.length ? "Add a note (optional)" : "Your answer"}</label>
            <textarea id={`a-${q.id}`} value={text} onChange={(e) => setText(e.target.value)} />
          </div>
          <div className="row">
            <FilePicker files={files} onChange={setFiles} />
            <span className="spacer" />
            <button type="button" className="ghost" onClick={() => void dismiss.run()} disabled={dismiss.busy || send.busy}>
              Dismiss
            </button>
            <button type="submit" className="primary" disabled={!canSend || send.busy}>
              {send.busy ? "Sending…" : "Send answer"}
            </button>
          </div>
          <ErrorText error={send.error ?? dismiss.error} />
        </form>
      ) : (
        <div className="small">
          <span className="muted">{STATUS[q.status] ?? q.status}</span>
          {answer && (
            <div style={{ marginTop: 4 }}>
              {answer.choice && <span className="pill">{answer.choice}</span>} {answer.text}
              <div className="delivered">{answer.delivered ? "Agent has it" : "Not picked up yet"}</div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
