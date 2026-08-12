import React from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import {
  desktopPollInterval,
  desktopQueryRetry,
  isDesktopUnavailableError,
  useUpdateWorkspaceFile,
} from "./workspaceReviewService";

const mocks = vi.hoisted(() => ({ updateFile: vi.fn() }));

vi.mock("../../hooks/useApi", () => ({
  default: () => ({
    getApiClient: () => ({
      v1ExternalAgentsWorkspaceFileUpdate: mocks.updateFile,
    }),
  }),
}));

const stopped = { response: { status: 503 } };
const broken = { response: { status: 500 } };

describe("workspace query behaviour against a stopped sandbox", () => {
  it("recognises only 503 as a stopped desktop", () => {
    expect(isDesktopUnavailableError(stopped)).toBe(true);
    expect(isDesktopUnavailableError(broken)).toBe(false);
    expect(isDesktopUnavailableError(new Error("network"))).toBe(false);
    expect(isDesktopUnavailableError(undefined)).toBe(false);
    expect(isDesktopUnavailableError(null)).toBe(false);
  });

  it("does not retry a stopped desktop", () => {
    // Every retry re-dials RevDial and logs "container not ready" server-side
    // for a container we already know is gone.
    expect(desktopQueryRetry(0, stopped)).toBe(false);
  });

  it("still retries a genuine failure, up to a bound", () => {
    expect(desktopQueryRetry(0, broken)).toBe(true);
    expect(desktopQueryRetry(1, broken)).toBe(true);
    expect(desktopQueryRetry(2, broken)).toBe(false);
  });

  it("stops the poll once the sandbox has answered 503", () => {
    const interval = desktopPollInterval(3000);
    expect(interval({ state: { error: undefined } })).toBe(3000);
    expect(interval({ state: { error: broken } })).toBe(3000);
    expect(interval({ state: { error: stopped } })).toBe(false);
  });
});

describe("workspace file updates", () => {
  it("uses the generated client with a content precondition and invalidates workspace data", async () => {
    mocks.updateFile.mockResolvedValue({
      data: { path: "src/app.ts", contents: "updated", content_hash: "next" },
    });
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) => React.createElement(
      QueryClientProvider,
      { client: queryClient },
      children,
    );
    const { result } = renderHook(
      () => useUpdateWorkspaceFile("ses_1", "primary", "src/app.ts"),
      { wrapper },
    );

    await act(async () => {
      await result.current.mutateAsync({ contents: "updated", expectedContentHash: "opened" });
    });

    expect(mocks.updateFile).toHaveBeenCalledWith("ses_1", {
      workspace: "primary",
      path: "src/app.ts",
      contents: "updated",
      expected_content_hash: "opened",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["workspace-review", "ses_1"] });
  });
});
