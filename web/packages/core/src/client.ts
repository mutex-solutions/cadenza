import { endpoints, type EndpointName, type EndpointTypes, type ErrorBody } from "./api.gen.ts";

/** Names of the `{param}` parts of a path, e.g. "/v1/threads/{id}" -> "id". */
type ParamNames<P extends string> = P extends `${string}{${infer Name}}${infer Rest}`
  ? Name | ParamNames<Rest>
  : never;

type PathOf<N extends EndpointName> = (typeof endpoints)[N]["path"];
export type Input<N extends EndpointName> = EndpointTypes[N]["input"];
export type Output<N extends EndpointName> = EndpointTypes[N]["output"];

type ParamsArg<N extends EndpointName> = [ParamNames<PathOf<N>>] extends [never]
  ? { params?: undefined }
  : { params: Record<ParamNames<PathOf<N>>, string> };

type BodyArg<N extends EndpointName> = [Input<N>] extends [void] ? { body?: undefined } : { body: Input<N> };

/** Options for one call: path params, query string and JSON body, as the endpoint needs. */
export type CallOptions<N extends EndpointName> = ParamsArg<N> &
  BodyArg<N> & {
    query?: Record<string, string | number | boolean | undefined>;
    signal?: AbortSignal;
  };

/** Every option is optional for endpoints with no params and no body. */
type CallArgs<N extends EndpointName> = [ParamNames<PathOf<N>>] extends [never]
  ? [Input<N>] extends [void]
    ? [opts?: CallOptions<N>]
    : [opts: CallOptions<N>]
  : [opts: CallOptions<N>];

/** An error from the server ({error: {kind, message, fix}}) or from the network. */
export class ApiError extends Error {
  readonly kind: string;
  readonly fix: string | undefined;
  /** HTTP status; 0 when the server could not be reached. */
  readonly status: number;

  constructor(kind: string, message: string, status: number, fix?: string) {
    super(message);
    this.name = "ApiError";
    this.kind = kind;
    this.status = status;
    this.fix = fix;
  }

  /** True when the caller is not logged in, or the login has expired. */
  get unauthorized(): boolean {
    return this.status === 401 || this.kind === "unauthorized";
  }

  /** True when the server could not be reached at all. */
  get offline(): boolean {
    return this.status === 0;
  }
}

export interface ClientOptions {
  /** Server address, e.g. "https://cadenza.example.com". Empty means the same origin (web app). */
  baseUrl?: string;
  /**
   * Bearer mode (Android): returns the login token. Leave it out for cookie
   * mode (web), where the browser sends the cadenza_session cookie.
   */
  token?: () => string | null | undefined;
  /** fetch to use; defaults to the global fetch. */
  fetch?: typeof fetch;
  /** Called after any 401, e.g. to show the login screen. */
  onUnauthorized?: () => void;
}

export interface Client {
  readonly baseUrl: string;
  /** Calls one endpoint by name, with typed params, body and result. */
  call<N extends EndpointName>(name: N, ...args: CallArgs<N>): Promise<Output<N>>;
  /** The full URL of an endpoint, with params and query filled in. */
  url<N extends EndpointName>(name: N, params?: Record<string, string>, query?: CallOptions<N>["query"]): string;
  /** Headers that authenticate a request (Bearer mode only; empty for cookies). */
  authHeaders(): Record<string, string>;
}

/** Fills `{name}` parts of a path; values are URL-encoded. */
export function fillPath(path: string, params: Record<string, string> = {}): string {
  return path.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    if (v === undefined || v === "") throw new Error(`missing path parameter "${name}" for ${path}`);
    return encodeURIComponent(v);
  });
}

function queryString(query?: Record<string, string | number | boolean | undefined>): string {
  if (!query) return "";
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined) qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

/** Turns a non-2xx response into an ApiError, using the server's error body when there is one. */
export async function errorFromResponse(res: globalThis.Response): Promise<ApiError> {
  let body: { error?: ErrorBody } | undefined;
  try {
    body = (await res.json()) as { error?: ErrorBody };
  } catch {
    body = undefined;
  }
  const e = body?.error;
  if (e && typeof e.message === "string") {
    return new ApiError(e.kind || "error", e.message, res.status, e.fix || undefined);
  }
  return new ApiError(res.status === 401 ? "unauthorized" : "http", `The server answered ${res.status}`, res.status);
}

export function createClient(opts: ClientOptions = {}): Client {
  const baseUrl = (opts.baseUrl ?? "").replace(/\/+$/, "");
  const doFetch = opts.fetch ?? ((input: RequestInfo | URL, init?: RequestInit) => globalThis.fetch(input, init));

  const authHeaders = (): Record<string, string> => {
    const t = opts.token?.();
    return t ? { Authorization: `Bearer ${t}` } : {};
  };

  const url = (name: EndpointName, params?: Record<string, string>, query?: Record<string, string | number | boolean | undefined>) =>
    baseUrl + fillPath(endpoints[name].path, params) + queryString(query);

  async function call<N extends EndpointName>(name: N, ...args: CallArgs<N>): Promise<Output<N>> {
    const o = (args[0] ?? {}) as {
      params?: Record<string, string>;
      body?: unknown;
      query?: Record<string, string | number | boolean | undefined>;
      signal?: AbortSignal;
    };
    const ep = endpoints[name];
    const headers: Record<string, string> = { Accept: "application/json", ...authHeaders() };
    let body: string | undefined;
    if (ep.method !== "GET" && o.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(o.body);
    }
    let res: globalThis.Response;
    try {
      res = await doFetch(url(name, o.params, o.query), {
        method: ep.method,
        headers,
        body,
        signal: o.signal,
        credentials: opts.token ? "omit" : "same-origin",
      });
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") throw err;
      throw new ApiError("offline", "Can't reach the Cadenza server. Check your connection.", 0);
    }
    if (!res.ok) {
      const err = await errorFromResponse(res);
      if (err.unauthorized) opts.onUnauthorized?.();
      throw err;
    }
    const text = await res.text();
    if (!text) return undefined as Output<N>;
    return JSON.parse(text) as Output<N>;
  }

  return { baseUrl, call, url: url as Client["url"], authHeaders };
}
