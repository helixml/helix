import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useApproveImplementation } from "./specTaskWorkflowService";

const mocks = vi.hoisted(() => ({
  approve: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  success: vi.fn(),
}));

vi.mock("../hooks/useApi", () => ({
  default: () => ({
    getApiClient: () => ({
      v1SpecTasksApproveImplementationCreate: mocks.approve,
    }),
  }),
}));

vi.mock("../hooks/useSnackbar", () => ({
  default: () => ({
    error: mocks.error,
    info: mocks.info,
    success: mocks.success,
  }),
}));

describe("useApproveImplementation", () => {
  beforeEach(() => vi.clearAllMocks());

  it("reports a 202 as a request to the agent, not an opened PR", async () => {
    mocks.approve.mockResolvedValue({
      status: 202,
      data: {
        status: "done",
        repo_pull_requests: [{ repository_id: "repo-1", pr_state: "merged", pr_url: "https://example/pull/1" }],
      },
    });
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useApproveImplementation("spt-1"), {
      wrapper,
    });

    await act(async () => result.current.mutateAsync());

    expect(mocks.success).toHaveBeenCalledWith(
      "Asked the agent to push and propose its pull request(s). Approve each proposal here when it arrives.",
    );
    expect(mocks.success).not.toHaveBeenCalledWith(
      "Implementation approved and merged!",
    );
    expect(mocks.info).not.toHaveBeenCalled();
  });

  it("shows the server message when the request fails", async () => {
    mocks.approve.mockRejectedValue({
      response: {
        data: {
          error: "no_follow_up_changes",
          message: "no new changes are available to open a pull request",
        },
      },
    });
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useApproveImplementation("spt-1"), {
      wrapper,
    });

    await expect(act(async () => result.current.mutateAsync())).rejects.toBeDefined();

    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledWith(
        "no new changes are available to open a pull request",
      ),
    );
  });
});
