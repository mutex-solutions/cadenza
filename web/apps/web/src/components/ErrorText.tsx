import { ApiError } from "@cadenza/core";

/** Shows an error in plain words, with the server's fix when it gives one. */
export function ErrorText({ error }: { error: unknown }) {
  if (!error) return null;
  const msg = error instanceof Error ? error.message : String(error);
  const fix = error instanceof ApiError ? error.fix : undefined;
  return (
    <p className="error" role="alert">
      {msg}
      {fix && <span className="fix">{fix}</span>}
    </p>
  );
}
