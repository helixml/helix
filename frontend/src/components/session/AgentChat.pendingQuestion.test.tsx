import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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
  useListInteractions: () => ({
    data: {
      data: {
        totalCount: 2,
        interactions: [
          {
            id: "interaction-1",
            state: "waiting",
            pending_question: pendingQuestion,
          },
        ],
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
  default: React.forwardRef(function EmbeddedSessionView() {
    return <div data-testid="session-view" />;
  }),
}));

vi.mock("./PendingQuestionCard", () => ({
  default: () => <div data-testid="pending-question" />,
}));

vi.mock("./useSessionPromptQueue", () => ({
  useSessionPromptQueue: () => ({
    entries: [],
    remove: vi.fn(),
    restartAgent: vi.fn(),
  }),
}));

describe("AgentChat pending question placement", () => {
  it("attaches the pending question immediately above the composer", () => {
    render(<AgentChat sessionId="session-1" />);

    const question = screen.getByTestId("pending-question");
    const composer = screen.getByTestId("composer");

    expect(question.compareDocumentPosition(composer)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(composer).toHaveAttribute("data-has-attached-header", "true");
  });
});
