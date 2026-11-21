import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@mui/material/styles";

import CompletionRequestCard from "./CompletionRequestCard";
import { useNotifyUserMessage, ActionCardCollapseScope } from "../session/actionCardCollapse";

vi.mock("../../services/specTaskCompletionService", () => ({
  useDecideCompletion: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}));

function SendButton() {
  const notify = useNotifyUserMessage();
  return <button onClick={notify}>send</button>;
}

const renderCard = () =>
  render(
    <ThemeProvider theme={createTheme()}>
      <ActionCardCollapseScope>
        <CompletionRequestCard specTaskId="spt_1" summary="Shipped the fix" openPullRequests={0} />
        <SendButton />
      </ActionCardCollapseScope>
    </ThemeProvider>,
  );

const header = () => screen.getByRole("button", { name: /Agent says the task is finished/ });

describe("CompletionRequestCard", () => {
  it("starts open and collapses to a one-line summary when the header is clicked", () => {
    renderCard();
    expect(header()).toHaveAttribute("aria-expanded", "true");

    fireEvent.click(header());
    expect(header()).toHaveAttribute("aria-expanded", "false");
    expect(header()).toHaveTextContent("Shipped the fix");

    fireEvent.click(header());
    expect(header()).toHaveAttribute("aria-expanded", "true");
  });

  it("collapses when the user sends a message", () => {
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "send" }));
    expect(header()).toHaveAttribute("aria-expanded", "false");
  });
});
