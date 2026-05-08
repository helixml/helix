// Wire types mirroring the Helix Sandboxes REST API. Kept in one file so
// callers can import them alongside the Sandbox class for type-safe use.

export type SandboxStatus =
  | "pending"
  | "running"
  | "stopping"
  | "stopped"
  | "failed";

export type SandboxRuntime = "ubuntu-desktop";

export interface SandboxRecord {
  id: string;
  name?: string;
  organization_id: string;
  owner: string;
  runtime: SandboxRuntime | string;
  image?: string;
  status: SandboxStatus;
  status_message?: string;
  vcpus?: number;
  memory_mb?: number;
  host_device_id?: string;
  container_id?: string;
  display_width?: number;
  display_height?: number;
  display_fps?: number;
  env?: Record<string, string>;
  tags?: Record<string, string>;
  timeout_seconds?: number;
  created_at?: string;
  updated_at?: string;
  started_at?: string;
  stopped_at?: string;
  expires_at?: string;
}

export interface CreateSandboxInput {
  name?: string;
  runtime?: SandboxRuntime;
  env?: Record<string, string>;
  tags?: Record<string, string>;
  timeout_seconds?: number;
  display_width?: number;
  display_height?: number;
  display_fps?: number;
}

export interface UpdateSandboxInput {
  name?: string;
  timeout_seconds?: number;
  tags?: Record<string, string>;
}

export interface RunCommandInput {
  cmd: string;
  args?: string[];
  cwd?: string;
  env?: Record<string, string>;
  sudo?: boolean;
  detached?: boolean;
  timeout_seconds?: number;
}

export interface CommandRecord {
  id: string;
  sandbox_id: string;
  cmd: string;
  args?: string[];
  cwd?: string;
  sudo?: boolean;
  detached?: boolean;
  status: "pending" | "running" | "finished" | "failed" | "killed";
  exit_code?: number;
  started_at: string;
  finished_at?: string;
  stdout?: string;
  stderr?: string;
}

export interface CommandsListResponse {
  commands: CommandRecord[];
}

export interface SandboxFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mode: string;
  mod_time: string;
}

export interface ListFilesResponse {
  path: string;
  entries: SandboxFileEntry[];
}

export interface ListSandboxesResponse {
  sandboxes: SandboxRecord[];
  total: number;
}

export interface ClientOptions {
  /** Helix base URL, e.g. https://app.tryhelix.ai */
  baseURL: string;
  /** Helix API key (`hlx_...`). */
  apiKey: string;
  /** Organization id (org_...). All operations are scoped to this org. */
  organizationId: string;
  /** Optional fetch implementation override (for tests). */
  fetch?: typeof fetch;
  /** Default request timeout in milliseconds. */
  timeoutMs?: number;
}
