// Thin HTTP wrapper used by Sandbox/Client. Centralizes auth header injection
// and error normalization.

import type { ClientOptions } from "./types.js";

export class HelixApiError extends Error {
  status: number;
  body: string;

  constructor(status: number, body: string, message?: string) {
    super(message ?? `Helix API error (${status}): ${body}`);
    this.status = status;
    this.body = body;
  }
}

export class HttpClient {
  readonly baseURL: string;
  readonly apiKey: string;
  readonly organizationId: string;
  readonly fetchImpl: typeof fetch;
  readonly timeoutMs: number;

  constructor(opts: ClientOptions) {
    if (!opts.baseURL) throw new Error("baseURL is required");
    if (!opts.apiKey) throw new Error("apiKey is required");
    if (!opts.organizationId) throw new Error("organizationId is required");
    this.baseURL = opts.baseURL.replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
    this.organizationId = opts.organizationId;
    this.fetchImpl = opts.fetch ?? fetch;
    this.timeoutMs = opts.timeoutMs ?? 60_000;
  }

  /** Build an absolute URL for a Sandboxes API path. The path argument is
   *  appended after `/api/v1/organizations/{org}` so callers don't need to
   *  spell it out every time. */
  url(path: string, query?: Record<string, string | undefined>): string {
    const cleaned = path.startsWith("/") ? path : `/${path}`;
    let url = `${this.baseURL}/api/v1/organizations/${encodeURIComponent(
      this.organizationId,
    )}${cleaned}`;
    if (query) {
      const params = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined) params.set(k, v);
      }
      const s = params.toString();
      if (s) url += `?${s}`;
    }
    return url;
  }

  authHeaders(extra?: Record<string, string>): Record<string, string> {
    return {
      Authorization: `Bearer ${this.apiKey}`,
      ...extra,
    };
  }

  /** Execute a JSON request. Throws HelixApiError on non-2xx. */
  async json<T>(
    method: string,
    path: string,
    init?: {
      body?: unknown;
      query?: Record<string, string | undefined>;
      headers?: Record<string, string>;
    },
  ): Promise<T> {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), this.timeoutMs);
    try {
      const res = await this.fetchImpl(this.url(path, init?.query), {
        method,
        headers: this.authHeaders({
          ...(init?.body !== undefined ? { "Content-Type": "application/json" } : {}),
          Accept: "application/json",
          ...(init?.headers ?? {}),
        }),
        body: init?.body !== undefined ? JSON.stringify(init.body) : undefined,
        signal: ctrl.signal,
      });
      const text = await res.text();
      if (!res.ok) throw new HelixApiError(res.status, text);
      if (!text) return undefined as unknown as T;
      try {
        return JSON.parse(text) as T;
      } catch {
        return text as unknown as T;
      }
    } finally {
      clearTimeout(timer);
    }
  }

  /** Raw byte request used for file read/write. */
  async raw(
    method: string,
    path: string,
    init?: {
      body?: ArrayBuffer | Uint8Array;
      query?: Record<string, string | undefined>;
      headers?: Record<string, string>;
    },
  ): Promise<Uint8Array> {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), this.timeoutMs);
    try {
      const res = await this.fetchImpl(this.url(path, init?.query), {
        method,
        headers: this.authHeaders(init?.headers),
        body: init?.body as BodyInit | undefined,
        signal: ctrl.signal,
      });
      const buf = new Uint8Array(await res.arrayBuffer());
      if (!res.ok) throw new HelixApiError(res.status, new TextDecoder().decode(buf));
      return buf;
    } finally {
      clearTimeout(timer);
    }
  }

  /** Stream the response body as an async iterable (line-buffered). Used for
   *  the SSE log endpoint. */
  async *stream(
    method: string,
    path: string,
    init?: { query?: Record<string, string | undefined> },
  ): AsyncGenerator<string, void, void> {
    const res = await this.fetchImpl(this.url(path, init?.query), {
      method,
      headers: this.authHeaders({ Accept: "text/event-stream" }),
    });
    if (!res.ok || !res.body) {
      const body = res.body ? await res.text() : "no body";
      throw new HelixApiError(res.status, body);
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buffer.indexOf("\n")) !== -1) {
        const line = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 1);
        yield line;
      }
    }
    if (buffer.length > 0) yield buffer;
  }

  /** Return a websocket-ready URL for the terminal endpoint. */
  wsUrl(path: string, query?: Record<string, string | undefined>): string {
    const url = new URL(this.url(path, query));
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return url.toString();
  }
}
