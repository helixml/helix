import { beforeEach, describe, expect, it } from "vitest";
import { TypesSandboxRuntime } from "../api/api";
import {
  preferredSpecTaskSandboxRuntime,
  saveSpecTaskSandboxRuntimePreference,
} from "./specTaskSandboxRuntime";

describe("spec task sandbox runtime preferences", () => {
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
});
