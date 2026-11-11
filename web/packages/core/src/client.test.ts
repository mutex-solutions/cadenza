import { describe, expect, it, vi } from "vitest";
import { ApiError, createClient, fillPath } from "./client.ts";
import { fakeFetch } from "./test/fakes.ts";

describe("fillPath", () => {
  it("fills and encodes params", () => {
    expect(fillPath("/v1/threads/{id}", { id: "t 1/2" })).toBe("/v1/threads/t%201%2F2");
  });
  it("throws on a missing param", () => {
    expect(() => fillPath("/v1/threads/{id}", {})).toThrow(/missing path parameter "id"/);
  });
});

describe("createClient", () => {
  it("GETs with cookies (web mode) and parses JSON", async () => {
    const f = fakeFetch({ "GET /v1/threads": () => ({ body: { threads: [] } }) });
    const c = createClient({ fetch: f.fetch });
    const out = await c.call("threads", { query: { archived: 1 } });
    expect(out).toEqual({ threads: [] });
    expect(f.calls[0]!.url).toBe("/v1/threads?archived=1");
    expect(f.calls[0]!.credentials).toBe("same-origin");
    expect(f.calls[0]!.headers.Authorization).toBeUndefined();
  });

  it("sends a Bearer token and JSON body (Android mode)", async () => {
    const f = fakeFetch({ "POST /v1/questions/q_1/answer": () => ({ body: { reply: "r_1" } }) });
    const c = createClient({ baseUrl: "https://cz.example.com/", token: () => "tok", fetch: f.fetch });
    const out = await c.call("answer", { params: { id: "q_1" }, body: { choice: "yes" } });
    expect(out).toEqual({ reply: "r_1" });
    const call = f.calls[0]!;
    expect(call.url).toBe("https://cz.example.com/v1/questions/q_1/answer");
    expect(call.method).toBe("POST");
    expect(call.headers.Authorization).toBe("Bearer tok");
    expect(call.headers["Content-Type"]).toBe("application/json");
    expect(call.body).toEqual({ choice: "yes" });
    expect(call.credentials).toBe("omit");
  });

  it("sends DELETE without a body", async () => {
    const f = fakeFetch({ "DELETE /v1/machines/m_1": () => ({ body: {} }) });
    const c = createClient({ fetch: f.fetch });
    await c.call("machine-remove", { params: { id: "m_1" } });
    expect(f.calls[0]!.method).toBe("DELETE");
    expect(f.calls[0]!.headers["Content-Type"]).toBeUndefined();
  });

  it("turns error bodies into ApiError with kind, message and fix", async () => {
    const f = fakeFetch({
      "POST /v1/auth/login": () => ({ status: 401, body: { error: { kind: "unauthorized", message: "wrong password", fix: "try again" } } }),
    });
    const onUnauthorized = vi.fn();
    const c = createClient({ fetch: f.fetch, onUnauthorized });
    const err = await c.call("login", { body: { password: "x" } }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const e = err as ApiError;
    expect([e.kind, e.message, e.fix, e.status, e.unauthorized]).toEqual(["unauthorized", "wrong password", "try again", 401, true]);
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it("handles error responses without a JSON body", async () => {
    const c = createClient({ fetch: (async () => new Response("oops", { status: 502 })) as typeof fetch });
    const e = (await c.call("me").catch((x: unknown) => x)) as ApiError;
    expect(e.status).toBe(502);
    expect(e.message).toMatch(/502/);
  });

  it("reports network failures as offline", async () => {
    const c = createClient({
      fetch: (async () => {
        throw new TypeError("Failed to fetch");
      }) as typeof fetch,
    });
    const e = (await c.call("version").catch((x: unknown) => x)) as ApiError;
    expect(e.offline).toBe(true);
    expect(e.kind).toBe("offline");
  });

  it("builds URLs and auth headers", () => {
    const c = createClient({ baseUrl: "https://a.b", token: () => "t" });
    expect(c.url("file-url", { sha256: "abc" })).toBe("https://a.b/v1/files/abc");
    expect(c.url("events", undefined, { after: 5 })).toBe("https://a.b/v1/events?after=5");
    expect(c.authHeaders()).toEqual({ Authorization: "Bearer t" });
    expect(createClient().authHeaders()).toEqual({});
  });

  it("returns undefined for an empty body", async () => {
    const c = createClient({ fetch: (async () => new Response("", { status: 200 })) as typeof fetch });
    await expect(c.call("logout")).resolves.toBeUndefined();
  });
});
