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
  default: ({ text }: { text: string }) => <p data-testid="agent-chat-markdown">{text}</p>,
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
vi.mock("./InlineCommentForm", () => ({
  default: (props: {
    show: boolean;
    commentText: string;
    onCommentChange: (text: string) => void;
    onCreate: () => void;
    onSend?: () => void;
    submitLabel?: string;
  }) => props.show ? (
    <div>
      <textarea
        aria-label="Plan comment"
        value={props.commentText}
        onChange={(event) => props.onCommentChange(event.target.value)}
      />
      <button onClick={props.onCreate}>{props.submitLabel}</button>
      {props.onSend && <button onClick={props.onSend}>Send</button>}
    </div>
  ) : null,
}));
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
    expect(screen.getByRole("button", { name: "Preview" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    fireEvent.click(screen.getByRole("button", { name: "Markdown" }));
    const editor = screen.getByRole("textbox", { name: "Requirements Specification markdown source" });
    fireEvent.change(editor, { target: { value: "# Requirements\n\nReviewer edit" } });

    expect(screen.getByText("Unsaved")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByTestId("agent-chat-markdown")).toHaveTextContent("Reviewer edit");

    fireEvent.click(screen.getByRole("button", { name: "Save document" }));
    await waitFor(() => expect(updateDocument).toHaveBeenCalledWith({
      document_type: "requirements",
      content: "# Requirements\n\nReviewer edit",
      original_content: "# Requirements\n\nOriginal text",
    }));
    expect(snackbarSuccess).toHaveBeenCalledWith("Requirements Specification saved");
  });

  it("does not intercept typing keys as global review shortcuts", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <DesignReviewContent specTaskId="task-1" reviewId="review-1" onClose={vi.fn()} />
      </QueryClientProvider>,
    );

    for (const key of ["c", "2"]) {
      const event = new KeyboardEvent("keydown", { key, cancelable: true });
      window.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(false);
    }
    expect(screen.getByRole("tab", { name: "Requirements" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("offers add-to-chat and immediate send from a narrow plan pane", async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const onQueueComment = vi.fn();
    const onSendComment = vi.fn().mockResolvedValue(undefined);
    render(
      <QueryClientProvider client={queryClient}>
        <DesignReviewContent
          specTaskId="task-1"
          reviewId="review-1"
          onClose={vi.fn()}
          onQueueComment={onQueueComment}
          onSendComment={onSendComment}
        />
      </QueryClientProvider>,
    );

    fireEvent.mouseMove(screen.getByTestId("agent-chat-markdown"));
    fireEvent.click(screen.getByRole("button", { name: "Add comment" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Plan comment" }), {
      target: { value: "Address this now" },
    });

    expect(screen.getByRole("button", { name: "Add to chat" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(onSendComment).toHaveBeenCalledWith(expect.objectContaining({
      sectionTitle: "Plan comment",
      text: "Address this now",
      contents: expect.stringContaining("Original text"),
    })));
    expect(onQueueComment).not.toHaveBeenCalled();
  });
});
