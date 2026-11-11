import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { memoryStorage, type ImageConverter, type ThreadSummary, type ThreadView } from "@cadenza/core";
import { createServices, ServicesProvider, type Services } from "../app/services.tsx";

export interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
}

type Reply = { status?: number; body?: unknown };
export type Routes = Record<string, (call: Call) => Reply | Promise<Reply>>;

/** A fetch answering "METHOD /path" routes (or full URLs), recording every call. */
export function fakeFetch(routes: Routes) {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    const method = init.method ?? "GET";
    let body: unknown = init.body;
    if (typeof body === "string") {
      try {
        body = JSON.parse(body);
      } catch {
        /* text */
      }
    }
    const call = { url, method, headers: (init.headers ?? {}) as Record<string, string>, body };
    calls.push(call);
    const path = url.startsWith("http") ? url : new URL(url, "http://x").pathname;
    const h = routes[`${method} ${url}`] ?? routes[`${method} ${path}`] ?? routes[`${method} ${new URL(url, "http://x").pathname}`];
    if (!h) return new Response(JSON.stringify({ error: { kind: "not_found", message: `no route ${method} ${url}` } }), { status: 404 });
    const r = await h(call);
    return new Response(r.body === undefined ? "{}" : JSON.stringify(r.body), { status: r.status ?? 200 });
  };
  return { fetch: fn as typeof fetch, calls, of: (method: string, path: string) => calls.filter((c) => c.method === method && c.url.split("?")[0] === path) };
}

export const fakeConverter: ImageConverter = {
  async toWebP(input) {
    const bytes = await input.arrayBuffer();
    return {
      full: new Blob([new Uint8Array(bytes), "-full"], { type: "image/webp" }),
      preview: new Blob(["preview-of-", new Uint8Array(bytes)], { type: "image/webp" }),
      type: "image/webp",
      width: 100,
      height: 100,
    };
  },
};

export function makeServices(routes: Routes, extra: Partial<Parameters<typeof createServices>[0]> = {}) {
  const f = fakeFetch({ "GET /v1/events": () => ({ body: { events: [], last: 1 } }), ...routes });
  const services = createServices({ fetch: f.fetch, storage: memoryStorage(), eventSource: null, converter: fakeConverter, pollMs: 60_000, ...extra });
  return { services, f };
}

export function renderWith(ui: ReactNode, services: Services) {
  return render(<ServicesProvider value={services}>{ui}</ServicesProvider>);
}

export function thread(id: string, extra: Partial<ThreadSummary> = {}): ThreadSummary {
  return { id, title: `Thread ${id}`, status: "active", machine: "vm-03", agent: "claude-code", posts: 1, open_questions: 0, updated_at: Date.now() - 60_000, ...extra };
}

export function threadView(id: string, extra: Partial<ThreadView> = {}): ThreadView {
  return {
    thread: thread(id),
    posts: [
      {
        id: "p_1",
        seq: 1,
        title: "Checkout redesign",
        summary_html: "<p>Promo field <strong>below</strong> total</p>",
        attachments: [{ index: 0, template: "ui-change", version: 1, data: { template: "ui-change", version: 1, slots: [{ name: "summary", block: "text", label: "Summary", value: { md: "x", html: "<p>x</p>" } }] } }],
        created_at: Date.now() - 120_000,
      },
    ],
    questions: [],
    replies: [],
    ...extra,
  };
}
