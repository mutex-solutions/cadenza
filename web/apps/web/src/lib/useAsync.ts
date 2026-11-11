import { useCallback, useState } from "react";

/** Runs an action, tracking busy state and the last error. */
export function useAction<A extends unknown[], R>(f: (...args: A) => Promise<R>) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const run = useCallback(
    async (...args: A): Promise<R | undefined> => {
      setBusy(true);
      setError(null);
      try {
        return await f(...args);
      } catch (e) {
        setError(e);
        return undefined;
      } finally {
        setBusy(false);
      }
    },
    [f],
  );
  return { run, busy, error, setError };
}
