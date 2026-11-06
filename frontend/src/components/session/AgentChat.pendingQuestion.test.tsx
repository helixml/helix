import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { TypesInteractionState, type TypesInteraction } from "../../api/api";
import AgentChat from "./AgentChat";

const pendingQuestion = {
  request_id: "request-1",
  questions: [
    {
      id: "framework",
      header: "Framework",
      question: "Which framework?",
      options: [{ label: "React" }],
    },
  ],
};
const { latestInteractions, newInference, pendingQuestionInteractions } = vi.hoisted(() => ({
  latestInteractions: {
    current: {
      totalCount: 12,
      interactions: [{ id: "interaction-new", state: "complete" }],
    },
  },
  newInference: vi.fn(),
  pendingQuestionInteractions: { current: [] as TypesInteraction[] },
}));

vi.mock("../../contexts/streaming", () => ({
  useStreaming: () => ({ NewInference: newInference }),
}));

vi.mock("../../hooks/useApi", () => ({
  default: () => ({ getApiClient: () => ({}) }),
}));

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ error: vi.fn(), info: vi.fn() }),
}));

vi.mock("../../services/sessionService", () => ({
  useListInteractions: (
    _sessionId: string,
    _page: number,
    _perPage: number,
    _order: string,
    options?: { pendingQuestionOnly?: boolean },
  ) => ({
    data: {
      data: options?.pendingQuestionOnly
        ? {
            totalCount: pendingQuestionInteractions.current.length,
            interactions: pendingQuestionInteractions.current,
          }
        : {
            ...latestInteractions.current,
          },
    },
    refetch: vi.fn(),
  }),
}));

vi.mock("../../services/specTaskService", () => ({
  useRefreshSpecTaskStatus: () => vi.fn(),
}));

vi.mock("../common/RobustPromptInput", () => ({
  default: ({ hasAttachedHeader, onSend }: {
    hasAttachedHeader?: boolean;
    onSend: (message: string) => Promise<void>;
  }) => (
    <button
      data-testid="composer"
      data-has-attached-header={String(hasAttachedHeader)}
      onClick={() => void onSend("hello")}
    >
      Send
    </button>
  ),
}));

vi.mock("./EmbeddedSessionView", () => ({
  default: React.forwardRef(function EmbeddedSessionView(
    { trailingContent }: { trailingContent?: React.ReactNode },
    _ref,
  ) {
    return <div data-testid="session-view">{trailingContent}</div>;
  }),
}));

vi.mock("./PendingQuestionCard", () => ({
  default: ({ interactionId }: { interactionId: string }) => (
    <div data-testid="pending-question" data-interaction-id={interactionId} />
  ),
}));

vi.mock("./useSessionPromptQueue", () => ({
  useSessionPromptQueue: () => ({
    entries: [],
    remove: vi.fn(),
    restartAgent: vi.fn(),
  }),
}));

describe("AgentChat pending question placement", () => {
  beforeEach(() => {
    latestInteractions.current = {
      totalCount: 12,
      interactions: [{ id: "interaction-new", state: "complete" }],
    };
    newInference.mockReset();
    newInference.mockResolvedValue(undefined);
    pendingQuestionInteractions.current = [{
      id: "interaction-old",
      state: TypesInteractionState.InteractionStateWaiting,
      pending_question: pendingQuestion,
    }];
  });

  it("keeps the welcome composer mounted until the first send succeeds", async () => {
    latestInteractions.current = { totalCount: 0, interactions: [] };
    pendingQuestionInteractions.current = [];
    let finishSend: () => void = () => {};
    newInference.mockReturnValueOnce(new Promise<void>((resolve) => {
      finishSend = resolve;
    }));
    render(<AgentChat sessionId="session-1" minimal />);

    fireEvent.click(screen.getByTestId("composer"));

    expect(screen.getByText("What would you like to know?")).toBeInTheDocument();
    await act(async () => finishSend());
    await waitFor(() => {
      expect(screen.queryByText("What would you like to know?")).not.toBeInTheDocument();
    });
  });

  it("puts a question older than the loaded transcript after newer replies and action cards", () => {
    render(
      <AgentChat
        sessionId="session-1"
        actionCards={<div data-testid="pr-proposal" />}
      />,
    );

    const card = screen.getByTestId("pr-proposal");
    expect(screen.getByTestId("session-view")).toContainElement(card);
    const question = screen.getByTestId("pending-question");
    expect(question).toHaveAttribute("data-interaction-id", "interaction-old");
    expect(card.compareDocumentPosition(question)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(screen.getByTestId("composer")).toHaveAttribute(
      "data-has-attached-header",
      "false",
    );
  });

  it("removes a question when the pending-question query refreshes", () => {
    const view = render(<AgentChat sessionId="session-1" />);
    expect(screen.getByTestId("pending-question")).toBeInTheDocument();

    pendingQuestionInteractions.current = [];
    view.rerender(<AgentChat sessionId="session-1" />);

    expect(screen.queryByTestId("pending-question")).not.toBeInTheDocument();
  });
});
