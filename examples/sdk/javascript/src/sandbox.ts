// Vercel-style Sandbox class for Helix.
//
// Usage:
//
//   const sandbox = await Sandbox.create({
//     baseURL: "https://app.example.com",
//     apiKey: process.env.HELIX_API_KEY!,
//     organizationId: "org_xxx",
//   });
//   const result = await sandbox.runCommand("echo", ["hello"]);
//   console.log(await result.stdout());
//   await sandbox.destroy();

import { HttpClient } from "./http.js";
import type {
  ClientOptions,
  CommandRecord,
  CommandsListResponse,
  CreateSandboxInput,
  ListFilesResponse,
  ListSandboxesResponse,
  RunCommandInput,
  SandboxFileEntry,
  SandboxRecord,
  UpdateSandboxInput,
} from "./types.js";

/** Options for Sandbox.create — combines client config + sandbox spec. */
export type SandboxCreateOptions = ClientOptions & CreateSandboxInput;

/** Options for Sandbox.list / Sandbox.get — just the client config. */
export type SandboxClientOptions = ClientOptions;

/** Result type returned by `runCommand({ detached:false })`. */
export class CommandResult {
  constructor(
    private readonly sandbox: Sandbox,
    public readonly record: CommandRecord,
  ) {}

  get id(): string {
    return this.record.id;
  }
  get cmdId(): string {
    return this.record.id;
  }
  get exitCode(): number | null {
    return this.record.exit_code ?? null;
  }
  get cwd(): string {
    return this.record.cwd ?? "";
  }
  get startedAt(): number {
    return Date.parse(this.record.started_at);
  }

  async stdout(): Promise<string> {
    if (this.record.status === "finished" || this.record.status === "failed" || this.record.status === "killed") {
      return this.record.stdout ?? "";
    }
    const fresh = await this.sandbox.getCommand(this.record.id);
    return fresh.record.stdout ?? "";
  }

  async stderr(): Promise<string> {
    if (this.record.status === "finished" || this.record.status === "failed" || this.record.status === "killed") {
      return this.record.stderr ?? "";
    }
    const fresh = await this.sandbox.getCommand(this.record.id);
    return fresh.record.stderr ?? "";
  }

  async output(stream: "stdout" | "stderr" | "both" = "both"): Promise<string> {
    const o = await this.stdout();
    const e = await this.stderr();
    switch (stream) {
      case "stdout":
        return o;
      case "stderr":
        return e;
      default:
        return `${o}${e}`;
    }
  }

  /** Wait for the command to finish (polling). For non-detached commands the
   *  initial response is already terminal, so this is a no-op. */
  async wait(opts?: { intervalMs?: number; timeoutMs?: number }): Promise<CommandResult> {
    const started = Date.now();
    let rec = this.record;
    const interval = opts?.intervalMs ?? 500;
    const timeout = opts?.timeoutMs ?? 5 * 60_000;
    while (rec.status === "pending" || rec.status === "running") {
      if (Date.now() - started > timeout) break;
      await new Promise((r) => setTimeout(r, interval));
      const fresh = await this.sandbox.getCommand(rec.id);
      rec = fresh.record;
    }
    return new CommandResult(this.sandbox, rec);
  }

  /** Stream live logs via the SSE endpoint. */
  async *logs(opts?: { stream?: "stdout" | "stderr" | "both"; follow?: boolean }): AsyncGenerator<
    { stream: "stdout" | "stderr"; data: string },
    void,
    void
  > {
    yield* this.sandbox.streamLogs(this.record.id, opts);
  }

  /** Send a signal to a running command. Default is TERM. */
  async kill(signal: string = "TERM"): Promise<void> {
    await this.sandbox.killCommand(this.record.id, signal);
  }
}

export class Sandbox {
  readonly http: HttpClient;
  private record: SandboxRecord;

  private constructor(http: HttpClient, record: SandboxRecord) {
    this.http = http;
    this.record = record;
  }

  /** Sandbox id (sbx_...). Persist this if you need to reconnect. */
  get sandboxId(): string {
    return this.record.id;
  }

  get status(): SandboxRecord["status"] {
    return this.record.status;
  }

  get name(): string | undefined {
    return this.record.name;
  }

  get runtime(): string {
    return this.record.runtime;
  }

  get organizationId(): string {
    return this.record.organization_id;
  }

  /** Last fetched record. Use refresh() to re-pull. */
  get details(): SandboxRecord {
    return this.record;
  }

  /** Create a new sandbox. Returns the Sandbox once the row exists; the
   *  desktop container provisions asynchronously, call waitForRunning() to
   *  block until status="running". */
  static async create(opts: SandboxCreateOptions): Promise<Sandbox> {
    const { baseURL, apiKey, organizationId, fetch: fetchImpl, timeoutMs, ...payload } = opts;
    const http = new HttpClient({ baseURL, apiKey, organizationId, fetch: fetchImpl, timeoutMs });
    const record = await http.json<SandboxRecord>("POST", "/sandboxes", { body: payload });
    return new Sandbox(http, record);
  }

  /** Reconnect to an existing sandbox by id. */
  static async get(opts: ClientOptions & { sandboxId: string }): Promise<Sandbox> {
    const { sandboxId, ...rest } = opts;
    const http = new HttpClient(rest);
    const record = await http.json<SandboxRecord>("GET", `/sandboxes/${encodeURIComponent(sandboxId)}`);
    return new Sandbox(http, record);
  }

  /** List sandboxes in the organization. */
  static async list(opts: ClientOptions): Promise<Sandbox[]> {
    const http = new HttpClient(opts);
    const result = await http.json<ListSandboxesResponse>("GET", "/sandboxes");
    return (result.sandboxes ?? []).map((rec) => new Sandbox(http, rec));
  }

  /** Re-pull the row from the server. */
  async refresh(): Promise<SandboxRecord> {
    this.record = await this.http.json<SandboxRecord>(
      "GET",
      `/sandboxes/${encodeURIComponent(this.record.id)}`,
    );
    return this.record;
  }

  /** Block until the sandbox transitions to status="running" (or fails). */
  async waitForRunning(opts?: { intervalMs?: number; timeoutMs?: number }): Promise<SandboxRecord> {
    const interval = opts?.intervalMs ?? 1000;
    const timeout = opts?.timeoutMs ?? 5 * 60_000;
    const started = Date.now();
    while (this.record.status === "pending") {
      if (Date.now() - started > timeout) {
        throw new Error(`sandbox ${this.record.id} did not become running within ${timeout}ms (status=${this.record.status})`);
      }
      await new Promise((r) => setTimeout(r, interval));
      await this.refresh();
    }
    if (this.record.status !== "running") {
      throw new Error(`sandbox ${this.record.id} entered terminal status=${this.record.status}: ${this.record.status_message ?? ""}`);
    }
    return this.record;
  }

  /** Update name/tags/ttl. */
  async update(input: UpdateSandboxInput): Promise<SandboxRecord> {
    this.record = await this.http.json<SandboxRecord>(
      "PATCH",
      `/sandboxes/${encodeURIComponent(this.record.id)}`,
      { body: input },
    );
    return this.record;
  }

  /** Delete the sandbox. The container is torn down and all in-memory state
   *  on hydra is forgotten. */
  async destroy(): Promise<void> {
    await this.http.json<void>("DELETE", `/sandboxes/${encodeURIComponent(this.record.id)}`);
  }

  // ----- Commands -----

  /** Run a command. By default returns a CommandResult after the command
   *  finishes. Pass `{ detached: true }` to start in the background. */
  async runCommand(
    cmdOrInput: string | RunCommandInput,
    args?: string[],
    extra?: Omit<RunCommandInput, "cmd" | "args">,
  ): Promise<CommandResult> {
    const payload: RunCommandInput =
      typeof cmdOrInput === "string"
        ? { cmd: cmdOrInput, args, ...(extra ?? {}) }
        : cmdOrInput;
    const record = await this.http.json<CommandRecord>(
      "POST",
      `/sandboxes/${encodeURIComponent(this.record.id)}/commands`,
      { body: payload },
    );
    return new CommandResult(this, record);
  }

  async listCommands(): Promise<CommandRecord[]> {
    const result = await this.http.json<CommandsListResponse>(
      "GET",
      `/sandboxes/${encodeURIComponent(this.record.id)}/commands`,
    );
    return result.commands ?? [];
  }

  async getCommand(cmdId: string): Promise<CommandResult> {
    const record = await this.http.json<CommandRecord>(
      "GET",
      `/sandboxes/${encodeURIComponent(this.record.id)}/commands/${encodeURIComponent(cmdId)}`,
    );
    return new CommandResult(this, record);
  }

  async killCommand(cmdId: string, signal: string = "TERM"): Promise<void> {
    await this.http.json<void>(
      "POST",
      `/sandboxes/${encodeURIComponent(this.record.id)}/commands/${encodeURIComponent(cmdId)}/kill`,
      { query: { signal } },
    );
  }

  /** Stream stdout/stderr from a command. Stops when the SSE `end` event
   *  arrives or when the iterator is broken. */
  async *streamLogs(
    cmdId: string,
    opts?: { stream?: "stdout" | "stderr" | "both"; follow?: boolean },
  ): AsyncGenerator<{ stream: "stdout" | "stderr"; data: string }, void, void> {
    const path = `/sandboxes/${encodeURIComponent(this.record.id)}/commands/${encodeURIComponent(cmdId)}/logs`;
    const query: Record<string, string> = { stream: opts?.stream ?? "both" };
    if (opts?.follow) query.follow = "1";

    let event: string | undefined;
    for await (const line of this.http.stream("GET", path, { query })) {
      if (line.startsWith("event: ")) {
        event = line.slice("event: ".length).trim();
        continue;
      }
      if (line.startsWith("data: ")) {
        const data = line.slice("data: ".length);
        if (event === "end") return;
        if (event === "stdout" || event === "stderr") {
          let text: string;
          try {
            text = JSON.parse(data) as string;
          } catch {
            text = data;
          }
          yield { stream: event, data: text };
        }
        event = undefined;
      }
    }
  }

  // ----- Files -----

  /** Read a file from the sandbox as raw bytes. */
  async readFile(path: string): Promise<Uint8Array> {
    return this.http.raw("GET", `/sandboxes/${encodeURIComponent(this.record.id)}/files`, {
      query: { path },
    });
  }

  /** Read a file as UTF-8 string. */
  async readFileText(path: string): Promise<string> {
    const buf = await this.readFile(path);
    return new TextDecoder().decode(buf);
  }

  /** Write a file (creates parent directories). Mode is octal — pass `0o755`
   *  for executable scripts. */
  async writeFile(
    path: string,
    body: string | Uint8Array | ArrayBuffer,
    opts?: { mode?: number },
  ): Promise<void> {
    const bytes =
      typeof body === "string"
        ? new TextEncoder().encode(body)
        : body instanceof Uint8Array
        ? body
        : new Uint8Array(body);
    await this.http.raw("PUT", `/sandboxes/${encodeURIComponent(this.record.id)}/files`, {
      body: bytes,
      query: {
        path,
        mode: opts?.mode !== undefined ? opts.mode.toString(8) : undefined,
      },
      headers: { "Content-Type": "application/octet-stream" },
    });
  }

  async deleteFile(path: string, opts?: { recursive?: boolean }): Promise<void> {
    await this.http.raw("DELETE", `/sandboxes/${encodeURIComponent(this.record.id)}/files`, {
      query: { path, recursive: opts?.recursive ? "1" : undefined },
    });
  }

  async listDirectory(path: string = "/root"): Promise<SandboxFileEntry[]> {
    const result = await this.http.json<ListFilesResponse>(
      "GET",
      `/sandboxes/${encodeURIComponent(this.record.id)}/files/list`,
      { query: { path } },
    );
    return result.entries ?? [];
  }

  // ----- Terminal -----

  /** Build a websocket URL for the sandbox terminal. The browser can use
   *  this directly with native WebSocket; Node callers should use the
   *  `ws` package. */
  terminalURL(opts?: { shell?: string }): string {
    return this.http.wsUrl(
      `/sandboxes/${encodeURIComponent(this.record.id)}/terminal`,
      opts?.shell ? { shell: opts.shell } : undefined,
    );
  }
}
