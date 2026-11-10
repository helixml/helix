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
const { latestInteractions, newInference, pendingQuestionInteractions, sessionQueueEntries } = vi.hoisted(() => ({
  latestInteractions: {
    current: {
      totalCount: 12,
      interactions: [{ id: "interaction-new", state: "complete" }],
    },
  },
  newInference: vi.fn(),
  pendingQuestionInteractions: { current: [] as TypesInteraction[] },
  sessionQueueEntries: { current: [] as Array<{ id: string }> },
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
  default: ({ hasAttachedHeader, onSend, onWillSend, sendMode }: {
    hasAttachedHeader?: boolean;
    onSend: (message: string) => Promise<void>;
    onWillSend?: () => void;
    sendMode: "queued" | "direct";
  }) => (
    <button
      data-testid="composer"
      data-has-attached-header={String(hasAttachedHeader)}
      onClick={() => {
        onWillSend?.();
        if (sendMode === "direct") void onSend("hello");
      }}
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
    entries: sessionQueueEntries.current,
    remove: vi.fn(),
    restartAgent: vi.fn(),
  }),
}));

vi.mock("./SessionPromptQueue", () => ({
  default: () => <div data-testid="session-prompt-queue" />,
}));

describe("AgentChat pending question placement", () => {
  beforeEach(() => {
    latestInteractions.current = {
      totalCount: 12,
      interactions: [{ id: "interaction-new", state: "complete" }],
    };
    newInference.mockReset();
    newInference.mockResolvedValue(undefined);
    sessionQueueEntries.current = [];
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

  it("fills the composer from a starter prompt in a full empty bot chat", () => {
    latestInteractions.current = { totalCount: 0, interactions: [] };
    pendingQuestionInteractions.current = [];
    const onSelectStarter = vi.fn();

    render(
      <AgentChat
        sessionId="session-1"
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
        onSelectStarter={onSelectStarter}
      />,
    );

    expect(screen.getByText("How can I help?")).toBeInTheDocument();
    expect(screen.getByTestId("composer")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /help me set up my team of ai bots/i }));
    expect(onSelectStarter).toHaveBeenCalledWith("Help me set up my team of AI bots");
    expect(newInference).not.toHaveBeenCalled();
  });

  it("shows starters after the Chief's org hire briefing", () => {
    latestInteractions.current = {
      totalCount: 1,
      interactions: [{ id: "interaction-new", state: "complete", trigger: "org_hire" }],
    } as any;
    pendingQuestionInteractions.current = [];

    render(
      <AgentChat
        sessionId="session-1"
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
        onSelectStarter={vi.fn()}
      />,
    );

    expect(screen.getByText("How can I help?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /help me set up my team of ai bots/i })).toBeInTheDocument();
  });

  it.each([
    [TypesInteractionState.InteractionStateError, "setup failed"],
    [TypesInteractionState.InteractionStateWaiting, undefined],
  ])("keeps a %s org hire interaction visible", (state, error) => {
    latestInteractions.current = {
      totalCount: 1,
      interactions: [{ id: "interaction-new", state, trigger: "org_hire", error }],
    } as any;
    pendingQuestionInteractions.current = [];

    render(
      <AgentChat
        sessionId="session-1"
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
        onSelectStarter={vi.fn()}
      />,
    );

    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
  });

  it("does not replace a single ordinary bot interaction with starters", () => {
    latestInteractions.current = {
      totalCount: 1,
      interactions: [{ id: "interaction-new", state: "complete" }],
    };
    pendingQuestionInteractions.current = [];

    render(
      <AgentChat
        sessionId="session-1"
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
      />,
    );

    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
  });

  it("does not replace multiple bot interactions with starters", () => {
    latestInteractions.current = {
      totalCount: 2,
      interactions: [{ id: "interaction-new", state: "complete", trigger: "org_hire" }],
    } as any;
    pendingQuestionInteractions.current = [];

    render(
      <AgentChat
        sessionId="session-1"
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
        onSelectStarter={vi.fn()}
      />,
    );

    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
  });

  it("shows a queued prompt instead of empty-chat starters", () => {
    latestInteractions.current = { totalCount: 0, interactions: [] };
    pendingQuestionInteractions.current = [];
    sessionQueueEntries.current = [{ id: "queued-prompt" }];

    render(
      <AgentChat
        sessionId="session-1"
        showSessionPromptQueue
        welcomeWhenEmpty
        welcomeHeading="How can I help?"
        starterPrompts={["Help me set up my team of AI bots"]}
        onSelectStarter={vi.fn()}
      />,
    );

    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
    expect(screen.getByTestId("session-prompt-queue")).toBeInTheDocument();
  });

  it("does not restore the welcome after a queued prompt leaves the queue", () => {
    latestInteractions.current = {
      totalCount: 1,
      interactions: [{ id: "interaction-new", state: "complete", trigger: "org_hire" }],
    } as any;
    pendingQuestionInteractions.current = [];
    const onWillSend = vi.fn();
    const props = {
      sessionId: "session-1",
      showSessionPromptQueue: true,
      welcomeWhenEmpty: true,
      welcomeHeading: "How can I help?",
      starterPrompts: ["Help me set up my team of AI bots"],
      onSelectStarter: vi.fn(),
      onWillSend,
    };
    const view = render(<AgentChat {...props} />);

    expect(screen.getByText("How can I help?")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("composer"));
    expect(onWillSend).toHaveBeenCalledTimes(1);
    expect(newInference).not.toHaveBeenCalled();

    sessionQueueEntries.current = [{ id: "queued-prompt" }];
    view.rerender(<AgentChat {...props} />);
    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-prompt-queue")).toBeInTheDocument();

    sessionQueueEntries.current = [];
    view.rerender(<AgentChat {...props} />);
    expect(screen.queryByText("How can I help?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
  });

  it("leaves an empty standard task chat on the existing thread surface", () => {
    latestInteractions.current = { totalCount: 0, interactions: [] };
    pendingQuestionInteractions.current = [];

    render(<AgentChat sessionId="session-1" />);

    expect(screen.queryByText("What would you like to know?")).not.toBeInTheDocument();
    expect(screen.getByTestId("session-view")).toBeInTheDocument();
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
