import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import PlanningDocumentsPlaceholder from "./PlanningDocumentsPlaceholder";

describe("PlanningDocumentsPlaceholder", () => {
  it("explains where the planning documents will appear", () => {
    render(<PlanningDocumentsPlaceholder />);

    expect(screen.getByText("Planning in progress")).toBeInTheDocument();
    expect(screen.getByText(/requirements\.md, design\.md, and tasks\.md/)).toBeInTheDocument();
  });

  it("surfaces a failed plan push and its recovery step", () => {
    render(
      <PlanningDocumentsPlaceholder
        pushError={{
          cause: "@planner cannot access helixml/private-repo.",
          next_step: "Switch to a VCS account with access, then retry.",
        }}
      />,
    );

    expect(screen.getByText("Plan documents were not published")).toBeInTheDocument();
    expect(screen.getByText("@planner cannot access helixml/private-repo.")).toBeInTheDocument();
    expect(screen.getByText("Switch to a VCS account with access, then retry.")).toBeInTheDocument();
  });
});
