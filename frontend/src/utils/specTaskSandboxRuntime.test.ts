import { beforeEach, describe, expect, it } from "vitest";
import { TypesSandboxRuntime } from "../api/api";
import {
  preferredSpecTaskSandboxResources,
  preferredSpecTaskSandboxRuntime,
  saveSpecTaskSandboxResourcesPreference,
  saveSpecTaskSandboxRuntimePreference,
  specTaskComputeStorageKey,
} from "./specTaskSandboxRuntime";

describe("spec task compute preferences", () => {
  beforeEach(() => localStorage.clear());

  it("uses the project default until the user chooses a preference", () => {
    expect(preferredSpecTaskSandboxRuntime(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    )).toBe(TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu);
  });

  it("keeps the user's last choice for that project", () => {
    saveSpecTaskSandboxRuntimePreference(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    );

    expect(preferredSpecTaskSandboxRuntime(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
    )).toBe(TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu);
    expect(preferredSpecTaskSandboxRuntime(
      "prj_2",
      TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
    )).toBe(TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop);
  });

  it("uses the project resource default until that project has a preference", () => {
    const projectDefault = { vcpus: 8, memory_mb: 16384 };

    expect(preferredSpecTaskSandboxResources("prj_1", projectDefault))
      .toEqual(projectDefault);
    expect(preferredSpecTaskSandboxResources("prj_1"))
      .toBeUndefined();
  });

  it("remembers size and environment together for only that project", () => {
    saveSpecTaskSandboxResourcesPreference("prj_1", { vcpus: 4, memory_mb: 8192 });
    saveSpecTaskSandboxRuntimePreference(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    );

    expect(JSON.parse(localStorage.getItem(specTaskComputeStorageKey("prj_1")) || ""))
      .toEqual({
        sandbox_resource_overrides: { vcpus: 4, memory_mb: 8192 },
        sandbox_runtime: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
      });
    expect(preferredSpecTaskSandboxResources(
      "prj_1",
      { vcpus: 8, memory_mb: 16384 },
    )).toEqual({ vcpus: 4, memory_mb: 8192 });
    expect(preferredSpecTaskSandboxResources(
      "prj_2",
      { vcpus: 8, memory_mb: 16384 },
    )).toEqual({ vcpus: 8, memory_mb: 16384 });
  });

  it("reads the legacy per-project runtime until the next compute change", () => {
    localStorage.setItem(
      "helix_spec_task_sandbox_runtime_prj_1",
      TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    );

    expect(preferredSpecTaskSandboxRuntime(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
    )).toBe(TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu);

    saveSpecTaskSandboxRuntimePreference(
      "prj_1",
      TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
    );
    expect(localStorage.getItem("helix_spec_task_sandbox_runtime_prj_1"))
      .toBeNull();
  });

  it("ignores malformed and unsupported saved sizes", () => {
    localStorage.setItem(specTaskComputeStorageKey("prj_1"), JSON.stringify({
      sandbox_resource_overrides: { vcpus: 3, memory_mb: 4096 },
    }));

    expect(preferredSpecTaskSandboxResources(
      "prj_1",
      { vcpus: 8, memory_mb: 16384 },
    )).toEqual({ vcpus: 8, memory_mb: 16384 });
  });
});
