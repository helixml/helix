import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

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
    respondMutate.mockClear();
    cancelMutate.mockClear();
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

    fireEvent.click(screen.getByRole("button", { name: /React/ }));
    expect(screen.getByText("Which database?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Postgres/ }));

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
});
