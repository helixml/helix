import { describe, expect, it } from "vitest";
import {
  subscriptionRequirementFromTask,
  subscriptionRequirementMessage,
} from "./taskLaunchFailure";

describe("subscriptionRequirementFromTask", () => {
  it("maps the backend's provider code to a user-facing label", () => {
    expect(subscriptionRequirementFromTask({
      error_code: "subscription_required",
      error_provider: "claude",
    })).toEqual({ provider: "claude", label: "Claude" });

    expect(subscriptionRequirementFromTask({
      error_code: "subscription_required",
      error_provider: "codex",
    })).toEqual({ provider: "codex", label: "ChatGPT" });
  });

  it("ignores failures that connecting a subscription would not fix", () => {
    // Only this code means "the user must log in"; every other failure is
    // retryable and must not send them to the providers page.
    expect(subscriptionRequirementFromTask({
      error: "Failed to sync base branch from upstream",
    })).toBeUndefined();
    expect(subscriptionRequirementFromTask({
      error_code: "something_else",
      error_provider: "claude",
    })).toBeUndefined();
    expect(subscriptionRequirementFromTask(undefined)).toBeUndefined();
    expect(subscriptionRequirementFromTask({})).toBeUndefined();
  });

  it("falls back to the raw provider when the label is unknown", () => {
    expect(subscriptionRequirementFromTask({
      error_code: "subscription_required",
      error_provider: "gemini",
    })).toEqual({ provider: "gemini", label: "gemini" });
  });

  it("requires a provider to act on", () => {
    expect(subscriptionRequirementFromTask({
      error_code: "subscription_required",
    })).toBeUndefined();
  });

  it("explains the requirement without leaking ids into the UI", () => {
    const message = subscriptionRequirementMessage({ provider: "claude", label: "Claude" });
    expect(message).toContain("Claude subscription");
    expect(message).not.toMatch(/usr_|org_/);
  });
});
