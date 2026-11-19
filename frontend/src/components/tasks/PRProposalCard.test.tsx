import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@mui/material/styles";

import PRProposalCard from "./PRProposalCard";
import { ActionCardCollapseContext } from "../session/actionCardCollapse";
import { TypesSpecTaskPRProposalStatus } from "../../api/api";

vi.mock("../../services/specTaskPRProposalService", () => ({
  useDecidePRProposal: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}));

const proposal = {
  id: "prp_1",
  status: TypesSpecTaskPRProposalStatus.PRProposalStatusPending,
  head_branch: "feature/x",
  base_branch: "main",
  title: "feat: x",
  body: "body",
  repository_name: "helix",
};

const card = (userMessages: number) => (
  <ThemeProvider theme={createTheme()}>
    <ActionCardCollapseContext.Provider value={userMessages}>
      <PRProposalCard specTaskId="spt_1" proposal={proposal} />
    </ActionCardCollapseContext.Provider>
  </ThemeProvider>
);

const header = () =>
  screen.getByRole("button", { name: /Agent wants to open a pull request/ });

describe("PRProposalCard", () => {
  it("opens a pending proposal and collapses it when the user sends a message", () => {
    const { rerender } = render(card(3));
    expect(header()).toHaveAttribute("aria-expanded", "true");

    rerender(card(3));
    expect(header()).toHaveAttribute("aria-expanded", "true");

    rerender(card(4));
    expect(header()).toHaveAttribute("aria-expanded", "false");
    expect(header()).toHaveTextContent("feature/x → main · feat: x");
  });

  it("can be reopened by hand after auto-collapsing", () => {
    const { rerender } = render(card(0));
    rerender(card(1));
    fireEvent.click(header());
    expect(header()).toHaveAttribute("aria-expanded", "true");
  });
});
