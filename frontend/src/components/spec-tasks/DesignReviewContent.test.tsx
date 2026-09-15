import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import DesignReviewContent from "./DesignReviewContent";

const updateDocument = vi.fn();
const snackbarSuccess = vi.fn();

vi.mock("../../services/designReviewService", () => ({
  designReviewKeys: {
    detail: (taskId: string, reviewId: string) => ["design-reviews", "detail", taskId, reviewId],
    comments: (taskId: string, reviewId: string) => ["design-reviews", "detail", taskId, reviewId, "comments"],
  },
  getUnresolvedCount: () => 0,
  useDesignReview: () => ({
    data: {
      review: {
        id: "review-1",
        spec_task_id: "task-1",
        status: "in_review",
        git_commit_hash: "abcdef123456",
        git_branch: "helix-specs",
        git_pushed_at: "2026-09-14T10:00:00Z",
        requirements_spec: "# Requirements\n\nOriginal text",
        technical_design: "# Technical design",
        implementation_plan: "# Implementation plan",
      },
    },
    isLoading: false,
  }),
  useDesignReviewComments: () => ({ data: { comments: [] }, isLoading: false }),
  useCommentQueueStatus: () => ({ data: undefined }),
  useSubmitReview: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateDesignReviewDocument: () => ({ mutateAsync: updateDocument, isPending: false }),
  useCreateComment: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResolveComment: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("../../services/specTaskService", () => ({
  useSpecTask: () => ({ data: { id: "task-1", status: "spec_review" } }),
}));
vi.mock("../session/Markdown", () => ({
  default: ({ text }: { text: string }) => <div data-testid="agent-chat-markdown">{text}</div>,
}));
vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({
    success: snackbarSuccess,
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
}));
vi.mock("../../hooks/useApi", () => ({ default: () => ({ getApiClient: () => ({}) }) }));
vi.mock("../../hooks/useAccount", () => ({ default: () => ({ user: null }) }));
vi.mock("../../hooks/useOAuthFlow", () => ({ useOAuthFlow: () => ({ startOAuthFlow: vi.fn() }) }));
vi.mock("../../services/oauthProvidersService", () => ({ useListOAuthProviders: () => ({ data: [] }) }));
vi.mock("../../utils/oauthProviders", () => ({
  findOAuthProviderForType: () => undefined,
  vcsScopesForProvider: () => [],
}));
vi.mock("./InlineCommentBubble", () => ({ default: () => null }));
vi.mock("./InlineCommentForm", () => ({ default: () => null }));
vi.mock("./CommentLogSidebar", () => ({ default: () => null }));
vi.mock("./ReviewActionFooter", () => ({ default: () => null }));
vi.mock("./ReviewSubmitDialog", () => ({ default: () => null }));

class ResizeObserverStub {
  observe() {}
  disconnect() {}
}

describe("DesignReviewContent document editing", () => {
  beforeEach(() => {
    updateDocument.mockReset();
    updateDocument.mockResolvedValue({});
    snackbarSuccess.mockReset();
    vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  });

  it("uses AgentChat markdown and previews a source edit before saving it", async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <DesignReviewContent specTaskId="task-1" reviewId="review-1" onClose={vi.fn()} />
      </QueryClientProvider>,
    );

    for (const label of ["Requirements", "Design", "Plan"]) {
      expect(screen.getByRole("tab", { name: label })).toBeInTheDocument();
    }
    expect(screen.queryByRole("tab", { name: "Requirements Specification" }))
      .not.toBeInTheDocument();
    expect(screen.getByTestId("agent-chat-markdown")).toHaveTextContent("Original text");

    fireEvent.click(screen.getByRole("button", { name: "Edit markdown source" }));
    const editor = screen.getByRole("textbox", { name: "Requirements Specification markdown source" });
    fireEvent.change(editor, { target: { value: "# Requirements\n\nReviewer edit" } });

    expect(screen.getByText("Unsaved")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Preview rendered markdown" }));
    expect(screen.getByTestId("agent-chat-markdown")).toHaveTextContent("Reviewer edit");

    fireEvent.click(screen.getByRole("button", { name: "Save document" }));
    await waitFor(() => expect(updateDocument).toHaveBeenCalledWith({
      document_type: "requirements",
      content: "# Requirements\n\nReviewer edit",
      original_content: "# Requirements\n\nOriginal text",
    }));
    expect(snackbarSuccess).toHaveBeenCalledWith("Requirements Specification saved");
  });
});
