import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@mui/material/styles";

import PRProposalCard from "./PRProposalCard";
import { useNotifyUserMessage, ActionCardCollapseScope } from "../session/actionCardCollapse";
import { TypesSpecTaskPRProposalStatus } from "../../api/api";

vi.mock("../../services/specTaskPRProposalService", () => ({
  useDecidePRProposalFor: () => ({ mutate: vi.fn(), isPending: false }),
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

function SendButton() {
  const notify = useNotifyUserMessage();
  return <button onClick={notify}>send</button>;
}

const renderCard = () =>
  render(
    <ThemeProvider theme={createTheme()}>
      <ActionCardCollapseScope>
        <PRProposalCard owner={{ kind: "task", specTaskId: "spt_1" }} proposal={proposal} />
        <SendButton />
      </ActionCardCollapseScope>
    </ThemeProvider>,
  );

const header = () =>
  screen.getByRole("button", { name: /Agent wants to open a pull request/ });
const send = () => fireEvent.click(screen.getByRole("button", { name: "send" }));

describe("PRProposalCard", () => {
  it("opens a pending proposal and collapses it when the user sends a message", () => {
    renderCard();
    expect(header()).toHaveAttribute("aria-expanded", "true");

    send();
    expect(header()).toHaveAttribute("aria-expanded", "false");
    expect(header()).toHaveTextContent("feature/x → main · feat: x");
  });

  it("can be reopened by hand after auto-collapsing", () => {
    renderCard();
    send();
    fireEvent.click(header());
    expect(header()).toHaveAttribute("aria-expanded", "true");
  });
});
