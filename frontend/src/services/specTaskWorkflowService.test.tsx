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

  it("reports a URL-less follow-up placeholder as still being created", async () => {
    mocks.approve.mockResolvedValue({
      data: {
        status: "pull_request",
        repo_pull_requests: [{ repository_id: "repo-1", pr_state: "unknown" }],
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

    expect(mocks.info).toHaveBeenCalledWith(
      "Creating pull request. Waiting for its URL...",
    );
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("shows the server message when there are no follow-up changes", async () => {
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
