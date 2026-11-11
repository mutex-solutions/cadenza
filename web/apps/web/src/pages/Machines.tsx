import { useCallback, useEffect, useState, type FormEvent } from "react";
import { agentLabel, timeAgo, type Machine, type Session } from "@cadenza/core";
import { useServices } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { useAction } from "../lib/useAsync.ts";

/** Codes look like "KQ7-4XM"; accept them typed in any case, with or without the dash. */
export function normalizeCode(s: string): string {
  return s.toUpperCase().replace(/[^A-Z0-9-]/g, "");
}

function PairForm({ onPaired }: { onPaired: () => void }) {
  const { client } = useServices();
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [done, setDone] = useState("");
  const pair = useAction(async () => {
    const out = await client.call("pair-approve", { body: { code: normalizeCode(code), name: name.trim() } });
    setDone(`${out.name} is paired. "cadenza login" on that machine finishes by itself.`);
    setCode("");
    setName("");
    onPaired();
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    setDone("");
    void pair.run();
  };
  return (
    <form className="panel" onSubmit={submit} aria-labelledby="pair-title">
      <h2 id="pair-title">Pair a machine</h2>
      <p className="small muted">
        Run <code className="mono">cadenza login</code> on the machine. It shows a code; type it here and give the machine a name.
      </p>
      <div className="row" style={{ alignItems: "flex-end" }}>
        <div style={{ flex: "1 1 140px" }}>
          <label htmlFor="pair-code">Code</label>
          <input id="pair-code" type="text" autoComplete="off" autoCapitalize="characters" value={code} onChange={(e) => setCode(e.target.value)} placeholder="KQ7-4XM" required />
        </div>
        <div style={{ flex: "2 1 180px" }}>
          <label htmlFor="pair-name">Name</label>
          <input id="pair-name" type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="vm-03" required />
        </div>
        <button className="primary" type="submit" disabled={pair.busy || !code.trim() || !name.trim()}>
          Approve
        </button>
      </div>
      <ErrorText error={pair.error} />
      {done && (
        <p className="small" role="status">
          {done}
        </p>
      )}
    </form>
  );
}

function MachineItem({ m, sessions, onChange }: { m: Machine; sessions: Session[]; onChange: () => void }) {
  const { client } = useServices();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(m.name);
  const rename = useAction(async () => {
    await client.call("machine-rename", { params: { id: m.id }, body: { name: name.trim() } });
    setEditing(false);
    onChange();
  });
  const remove = useAction(async () => {
    if (!window.confirm(`Remove ${m.name}? Its keys stop working at once. What it showed stays.`)) return;
    await client.call("machine-remove", { params: { id: m.id } });
    onChange();
  });
  return (
    <li>
      {editing ? (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault();
            void rename.run();
          }}
        >
          <label htmlFor={`name-${m.id}`} className="sr-only">
            New name for {m.name}
          </label>
          <input id={`name-${m.id}`} type="text" value={name} onChange={(e) => setName(e.target.value)} style={{ flex: 1 }} autoFocus />
          <button className="primary" type="submit" disabled={rename.busy || !name.trim()}>
            Save
          </button>
          <button type="button" onClick={() => setEditing(false)}>
            Cancel
          </button>
        </form>
      ) : (
        <div className="row">
          <div style={{ flex: 1, minWidth: 0 }}>
            <strong>{m.name}</strong>
            <div className="small muted">
              Last seen {m.last_seen_at ? timeAgo(m.last_seen_at) : "never"} · {sessions.length} agent {sessions.length === 1 ? "session" : "sessions"}
            </div>
          </div>
          <button onClick={() => setEditing(true)}>Rename</button>
          <button className="bad" onClick={() => void remove.run()} disabled={remove.busy}>
            Remove
          </button>
        </div>
      )}
      <ErrorText error={rename.error ?? remove.error} />
      {sessions.length > 0 && (
        <ul className="list sessions" aria-label={`Agent sessions on ${m.name}`}>
          {sessions.map((s) => (
            <SessionItem key={s.id} s={s} onChange={onChange} />
          ))}
        </ul>
      )}
    </li>
  );
}

/** One live agent session; revoking stops its key without removing the machine. */
function SessionItem({ s, onChange }: { s: Session; onChange: () => void }) {
  const { client } = useServices();
  const revoke = useAction(async () => {
    await client.call("session-revoke", { params: { id: s.id } });
    onChange();
  });
  const where = s.repo || s.cwd;
  return (
    <li>
      <div className="row">
        <div style={{ flex: 1, minWidth: 0 }}>
          <span>{agentLabel(s.agent)}</span>
          {where && <span className="small muted mono"> {where}</span>}
          <div className="small muted">
            Started {timeAgo(s.created_at)} · last used {s.last_used_at ? timeAgo(s.last_used_at) : "never"}
          </div>
        </div>
        <button className="bad" onClick={() => void revoke.run()} disabled={revoke.busy} aria-label={`Revoke ${agentLabel(s.agent)} session${where ? ` in ${where}` : ""}`}>
          Revoke
        </button>
      </div>
      <ErrorText error={revoke.error} />
    </li>
  );
}

/** Paired machines: approve new ones, rename, remove, revoke agent sessions. */
export default function MachinesPage() {
  const { client, onChange } = useServices();
  const [machines, setMachines] = useState<Machine[] | null>(null);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(() => {
    Promise.all([client.call("machines"), client.call("sessions")])
      .then(([m, s]) => {
        setMachines(m.machines);
        setSessions(s.sessions);
        setError(null);
      })
      .catch((e: unknown) => setError(e));
  }, [client]);
  useEffect(() => {
    load();
    return onChange((e) => {
      if (e.kind === "machine.changed") load();
    });
  }, [load, onChange]);

  return (
    <div className="page-narrow stack">
      <h1>Machines</h1>
      <PairForm onPaired={load} />
      <section className="panel" aria-labelledby="machines-title">
        <h2 id="machines-title">Paired</h2>
        <ErrorText error={error} />
        {machines === null && !error && <p className="muted">Loading…</p>}
        {machines?.length === 0 && <p className="muted">No machines yet.</p>}
        {machines && machines.length > 0 && (
          <ul className="list">
            {machines.map((m) => (
              <MachineItem key={m.id} m={m} sessions={sessions.filter((x) => x.machine_id === m.id)} onChange={load} />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
