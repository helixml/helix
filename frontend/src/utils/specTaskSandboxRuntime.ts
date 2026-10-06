import {
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
} from "../api/api";
import { SANDBOX_PRESETS } from "../constants/sandboxPresets";

const COMPUTE_STORAGE_PREFIX = "helix_spec_task_compute_";
const LEGACY_RUNTIME_STORAGE_PREFIX = "helix_spec_task_sandbox_runtime_";

interface SpecTaskComputePreference {
  sandbox_resource_overrides?: TypesSandboxResourceOverrides;
  sandbox_runtime?: TypesSandboxRuntime;
}

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
  const preference = readSpecTaskComputePreference(projectId);
  if (preference?.sandbox_runtime) return preference.sandbox_runtime;

  const value = localStorage.getItem(`${LEGACY_RUNTIME_STORAGE_PREFIX}${projectId}`);
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
  saveSpecTaskComputePreference(projectId, { sandbox_runtime: runtime });
  localStorage.removeItem(`${LEGACY_RUNTIME_STORAGE_PREFIX}${projectId}`);
}

export function preferredSpecTaskSandboxRuntime(
  projectId: string,
  projectDefault?: TypesSandboxRuntime,
): TypesSandboxRuntime {
  return readSpecTaskSandboxRuntimePreference(projectId)
    || effectiveSpecTaskSandboxRuntime(projectDefault);
}

export function specTaskComputeStorageKey(projectId: string): string {
  return `${COMPUTE_STORAGE_PREFIX}${projectId}`;
}

export function readSpecTaskSandboxResourcesPreference(
  projectId: string,
): TypesSandboxResourceOverrides | undefined {
  return readSpecTaskComputePreference(projectId)?.sandbox_resource_overrides;
}

export function saveSpecTaskSandboxResourcesPreference(
  projectId: string,
  resources: TypesSandboxResourceOverrides,
): void {
  if (!projectId || !validSandboxResources(resources)) return;
  saveSpecTaskComputePreference(projectId, { sandbox_resource_overrides: resources });
}

export function preferredSpecTaskSandboxResources(
  projectId: string,
  projectDefault?: TypesSandboxResourceOverrides,
): TypesSandboxResourceOverrides | undefined {
  return readSpecTaskSandboxResourcesPreference(projectId) || projectDefault;
}

function readSpecTaskComputePreference(
  projectId: string,
): SpecTaskComputePreference | undefined {
  if (!projectId) return undefined;
  const value = localStorage.getItem(specTaskComputeStorageKey(projectId));
  if (!value) return undefined;

  try {
    const stored = JSON.parse(value) as SpecTaskComputePreference;
    const resources = validSandboxResources(stored.sandbox_resource_overrides)
      ? stored.sandbox_resource_overrides
      : undefined;
    const runtime = validSandboxRuntime(stored.sandbox_runtime)
      ? stored.sandbox_runtime
      : undefined;
    return resources || runtime
      ? { sandbox_resource_overrides: resources, sandbox_runtime: runtime }
      : undefined;
  } catch {
    return undefined;
  }
}

function saveSpecTaskComputePreference(
  projectId: string,
  update: SpecTaskComputePreference,
): void {
  const current = readSpecTaskComputePreference(projectId) || {};
  localStorage.setItem(
    specTaskComputeStorageKey(projectId),
    JSON.stringify({ ...current, ...update }),
  );
}

function validSandboxRuntime(
  runtime?: TypesSandboxRuntime,
): runtime is TypesSandboxRuntime {
  return runtime === TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop
    || runtime === TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu;
}

function validSandboxResources(
  resources?: TypesSandboxResourceOverrides,
): resources is TypesSandboxResourceOverrides {
  return !!resources && SANDBOX_PRESETS.some(
    (preset) => preset.vcpus === resources.vcpus
      && preset.memory_mb === resources.memory_mb,
  );
}
