/** The app's screens. */
export type Screen =
  | { name: "connect" }
  | { name: "inbox" }
  | { name: "thread"; id: string }
  | { name: "settings" };

export type NavAction =
  | { type: "push"; screen: Screen }
  | { type: "pop" }
  | { type: "reset"; screen: Screen }
  | { type: "openThread"; id: string };

/** A tiny screen stack. The bottom screen is never popped. */
export function navReducer(stack: Screen[], a: NavAction): Screen[] {
  switch (a.type) {
    case "push":
      return [...stack, a.screen];
    case "pop":
      return stack.length > 1 ? stack.slice(0, -1) : stack;
    case "reset":
      return [a.screen];
    case "openThread": {
      // From a notification: inbox underneath, the thread on top, no duplicates.
      const top = stack[stack.length - 1];
      if (top?.name === "thread" && top.id === a.id) return stack;
      return [{ name: "inbox" }, { name: "thread", id: a.id }];
    }
  }
}

/** Finds the thread a notification points at: data.url "/t/<id>" or data.thread_id. */
export function threadFromNotification(data: unknown): string | null {
  if (typeof data !== "object" || data === null) return null;
  const d = data as Record<string, unknown>;
  const ok = (s: unknown): s is string => typeof s === "string" && /^[A-Za-z0-9_\-]{1,128}$/.test(s);
  if (typeof d.url === "string") {
    const m = /^(?:https?:\/\/[^/]+)?\/t\/([^/?#]+)/.exec(d.url);
    if (m && ok(decodeURIComponent(m[1]!))) return decodeURIComponent(m[1]!);
  }
  if (ok(d.thread_id)) return d.thread_id;
  return null;
}
