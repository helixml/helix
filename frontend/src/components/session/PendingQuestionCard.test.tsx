import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import PendingQuestionCard from "./PendingQuestionCard";

const respondMutate = vi.fn();
const cancelMutate = vi.fn();

vi.mock("../../services/agentQuestionService", () => ({
  useRespondToAgentQuestion: () => ({
    mutate: respondMutate,
    isPending: false,
  }),
  useCancelAgentQuestion: () => ({ mutate: cancelMutate, isPending: false }),
}));

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ error: vi.fn() }),
}));

describe("PendingQuestionCard", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    respondMutate.mockClear();
    cancelMutate.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("advances through single-select questions and submits all answers", () => {
    render(
      <PendingQuestionCard
        interactionId="interaction-1"
        pendingQuestion={{
          request_id: "question-1",
          questions: [
            {
              id: "framework",
              header: "Framework",
              question: "Which framework?",
              options: [{ label: "React" }],
            },
            {
              id: "database",
              header: "Database",
              question: "Which database?",
              options: [{ label: "Postgres" }],
            },
          ],
        }}
      />,
    );

    fireEvent.keyDown(document, { key: "1" });
    act(() => vi.advanceTimersByTime(200));
    expect(screen.getByText("Which database?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Postgres/ }));
    act(() => vi.advanceTimersByTime(200));

    expect(respondMutate).toHaveBeenCalledWith(
      {
        interactionId: "interaction-1",
        requestId: "question-1",
        answers: { framework: "React", database: "Postgres" },
      },
      expect.any(Object),
    );
  });

  it("submits multiple selected options in wire order", () => {
    render(
      <PendingQuestionCard
        interactionId="interaction-1"
        pendingQuestion={{
          request_id: "question-2",
          questions: [
            {
              id: "features",
              header: "Features",
              question: "Which features?",
              multi_select: true,
              options: [{ label: "Auth" }, { label: "Billing" }],
            },
          ],
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /Auth/ }));
    fireEvent.click(screen.getByRole("button", { name: /Billing/ }));
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(respondMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        answers: { features: "Auth\nBilling" },
      }),
      expect.any(Object),
    );
  });

  it("does not submit an auto-advancing option after dismissal", () => {
    render(
      <PendingQuestionCard
        interactionId="interaction-1"
        pendingQuestion={{
          request_id: "question-3",
          questions: [
            {
              id: "framework",
              header: "Framework",
              question: "Which framework?",
              options: [{ label: "React" }],
            },
          ],
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /React/ }));
    fireEvent.click(
      screen.getByRole("button", {
        name: "Dismiss question without answering",
      }),
    );
    act(() => vi.advanceTimersByTime(200));

    expect(cancelMutate).toHaveBeenCalledWith(
      { interactionId: "interaction-1", requestId: "question-3" },
      expect.any(Object),
    );
    expect(respondMutate).not.toHaveBeenCalled();
  });
});
