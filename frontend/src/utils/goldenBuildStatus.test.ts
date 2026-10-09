import { describe, expect, it } from "vitest";
import {
  goldenBuildStatusDetail,
  goldenBuildStatusLabel,
  isGoldenBuildActive,
} from "./goldenBuildStatus";

describe("goldenBuildStatus", () => {
  it("shows the attempt while building", () => {
    expect(goldenBuildStatusLabel({ status: "building", attempt: 2, max_attempts: 3 })).toBe(
      "Building (attempt 2 of 3)",
    );
  });

  it("explains a retry after an interruption", () => {
    const state = { status: "retrying", attempt: 1, max_attempts: 3, interrupt_reason: "sandbox restarted" };
    expect(goldenBuildStatusLabel(state)).toBe("Retrying after interruption (attempt 1 of 3 interrupted)");
    expect(goldenBuildStatusDetail(state)).toEqual({
      text: "Retrying after interruption: sandbox restarted",
      severity: "warning",
    });
    expect(isGoldenBuildActive(state)).toBe(true);
  });

  it("reports why a build failed", () => {
    const state = { status: "failed", error: "Startup script exited with code 2" };
    expect(goldenBuildStatusLabel(state)).toBe("Failed");
    expect(goldenBuildStatusDetail(state)).toEqual({
      text: "Failed: Startup script exited with code 2",
      severity: "error",
    });
    expect(isGoldenBuildActive(state)).toBe(false);
  });

  it("names the previous interruption on a later attempt", () => {
    expect(
      goldenBuildStatusDetail({ status: "building", attempt: 2, interrupt_reason: "Hydra restarted" }),
    ).toEqual({ text: "Previous attempt interrupted: Hydra restarted", severity: "warning" });
  });
});
