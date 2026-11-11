import { useState, type FormEvent } from "react";
import { useServices } from "../app/services.tsx";
import { ErrorText } from "../components/ErrorText.tsx";
import { useAction } from "../lib/useAsync.ts";

/** First run (set the owner password) or log in. */
export function Login({ setup, onDone }: { setup: boolean; onDone: () => void }) {
  const { client } = useServices();
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [mismatch, setMismatch] = useState(false);
  const action = useAction(async () => {
    if (setup) await client.call("setup", { body: { password } });
    else await client.call("login", { body: { password } });
    onDone();
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (setup && password !== again) {
      setMismatch(true);
      return;
    }
    setMismatch(false);
    void action.run();
  };

  return (
    <div className="auth">
      <span className="brand">
        <span className="brand-mark" aria-hidden="true" />
        Cadenza
      </span>
      <h1>{setup ? "Set your password" : "Log in"}</h1>
      <p className="muted">
        {setup
          ? "This is a new Cadenza server. Choose the password you'll use to log in. Only you can see what your agents show."
          : "Enter your Cadenza password."}
      </p>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="pw">Password</label>
          <input
            id="pw"
            type="password"
            autoComplete={setup ? "new-password" : "current-password"}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoFocus
          />
        </div>
        {setup && (
          <div className="field">
            <label htmlFor="pw2">Type it again</label>
            <input id="pw2" type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} required />
          </div>
        )}
        {mismatch && <p className="error">The two passwords are different.</p>}
        <ErrorText error={action.error} />
        <button className="primary" type="submit" disabled={action.busy || !password} style={{ width: "100%" }}>
          {action.busy ? "Please wait…" : setup ? "Save password" : "Log in"}
        </button>
      </form>
    </div>
  );
}
