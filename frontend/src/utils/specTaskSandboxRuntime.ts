import { TypesSandboxRuntime } from "../api/api";

const STORAGE_PREFIX = "helix_spec_task_sandbox_runtime_";

export const DEFAULT_SPEC_TASK_SANDBOX_RUNTIME =
  TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop;

export function effectiveSpecTaskSandboxRuntime(
  runtime?: TypesSandboxRuntime,
): TypesSandboxRuntime {
  return runtime || DEFAULT_SPEC_TASK_SANDBOX_RUNTIME;
}

export function readSpecTaskSandboxRuntimePreference(
  projectId: string,
): TypesSandboxRuntime | undefined {
  if (!projectId) return undefined;
  const value = localStorage.getItem(`${STORAGE_PREFIX}${projectId}`);
  if (
    value === TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop
    || value === TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu
  ) {
    return value;
  }
  return undefined;
}

export function saveSpecTaskSandboxRuntimePreference(
  projectId: string,
  runtime: TypesSandboxRuntime,
): void {
  if (!projectId) return;
  localStorage.setItem(`${STORAGE_PREFIX}${projectId}`, runtime);
}

export function preferredSpecTaskSandboxRuntime(
  projectId: string,
  projectDefault?: TypesSandboxRuntime,
): TypesSandboxRuntime {
  return readSpecTaskSandboxRuntimePreference(projectId)
    || effectiveSpecTaskSandboxRuntime(projectDefault);
}
