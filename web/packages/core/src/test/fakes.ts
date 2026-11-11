import type { ThreadSummary, ThreadView } from "../api.gen.ts";

export interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
  credentials?: RequestCredentials;
}

type Handler = (call: Call) => { status?: number; body?: unknown } | Promise<{ status?: number; body?: unknown }>;

/** A fetch that answers from a route table "METHOD /path" and records calls. */
export function fakeFetch(routes: Record<string, Handler>) {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    const method = init.method ?? "GET";
    const headers = (init.headers ?? {}) as Record<string, string>;
    let body: unknown = init.body;
    if (typeof body === "string") {
      try {
        body = JSON.parse(body);
      } catch {
        /* keep text */
      }
    }
    const call: Call = { url, method, headers, body, credentials: init.credentials };
    calls.push(call);
    const path = new URL(url, "http://x").pathname;
    const h = routes[`${method} ${path}`] ?? routes[`${method} ${url}`];
    if (!h) return new Response(JSON.stringify({ error: { kind: "not_found", message: `no route ${method} ${path}` } }), { status: 404 });
    const r = await h(call);
    return new Response(r.body === undefined ? "{}" : JSON.stringify(r.body), { status: r.status ?? 200 });
  };
  return { fetch: fn as typeof fetch, calls };
}

export function thread(id: string, updated = 1000, extra: Partial<ThreadSummary> = {}): ThreadSummary {
  return { id, title: `Thread ${id}`, status: "active", machine: "vm-03", agent: "claude-code", posts: 1, open_questions: 0, updated_at: updated, ...extra };
}

export function threadView(id: string, updated = 1000): ThreadView {
  return { thread: thread(id, updated), posts: [], questions: [], replies: [] };
}
