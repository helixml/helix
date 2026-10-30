import React from "react";
import { render, screen } from "@testing-library/react";
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
const { pendingQuestionInteractions } = vi.hoisted(() => ({
  pendingQuestionInteractions: { current: [] as TypesInteraction[] },
}));

vi.mock("../../contexts/streaming", () => ({
  useStreaming: () => ({ NewInference: vi.fn() }),
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
            totalCount: 12,
            interactions: [{ id: "interaction-new", state: "complete" }],
          },
    },
    refetch: vi.fn(),
  }),
}));

vi.mock("../../services/specTaskService", () => ({
  useRefreshSpecTaskStatus: () => vi.fn(),
}));

vi.mock("../common/RobustPromptInput", () => ({
  default: ({ hasAttachedHeader }: { hasAttachedHeader?: boolean }) => (
    <div
      data-testid="composer"
      data-has-attached-header={String(hasAttachedHeader)}
    />
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
    pendingQuestionInteractions.current = [{
      id: "interaction-old",
      state: TypesInteractionState.InteractionStateWaiting,
      pending_question: pendingQuestion,
    }];
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
