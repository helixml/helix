import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import ReviewActionFooter from "./ReviewActionFooter";

describe("ReviewActionFooter", () => {
  it("shows one persistent approval action", () => {
    render(
      <ReviewActionFooter
        reviewStatus="pending"
        unresolvedCount={0}
        startingImplementation={false}
        implementationStarted={false}
        onApprove={vi.fn()}
        onStartImplementation={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Approve" })).toBeEnabled();
    expect(screen.queryByText("Reject Design")).not.toBeInTheDocument();
    expect(screen.queryByText("Request Changes")).not.toBeInTheDocument();
    expect(screen.queryByText("Next Document")).not.toBeInTheDocument();
  });

  it("blocks approval while comments remain unresolved", () => {
    render(
      <ReviewActionFooter
        reviewStatus="in_review"
        unresolvedCount={2}
        startingImplementation={false}
        implementationStarted={false}
        onApprove={vi.fn()}
        onStartImplementation={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Approve" })).toBeDisabled();
    expect(screen.getByText("2 unresolved comments")).toBeInTheDocument();
  });
});
