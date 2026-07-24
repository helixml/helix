import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { Interaction } from "./Interaction";

vi.mock("./InteractionContainer", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./InteractionInference", () => ({
  default: ({
    enableDebugCopy,
    isFromAssistant,
  }: {
    enableDebugCopy?: boolean;
    isFromAssistant?: boolean;
  }) => (
    <div data-testid={isFromAssistant ? "agent-reply" : "user-message"}>
      {enableDebugCopy && <button aria-label="agent debug copy" />}
    </div>
  ),
}));

vi.mock("./InteractionDebugCopyButton", () => ({
  default: () => <button aria-label="user debug copy" />,
}));

const baseProps = {
  serverConfig: { filestore_prefix: "/api/v1/filestore" },
  session: { id: "ses_1" },
  highlightAllFiles: false,
  onReloadSession: vi.fn(),
  isLastInteraction: true,
  isOwner: true,
  isAdmin: true,
  session_id: "ses_1",
  onRegenerate: vi.fn(),
  enableDebugCopy: true,
};

describe("Interaction debug copy placement", () => {
  it("shows debug copy only on the agent side after a reply", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_1",
          prompt_message: "Question",
          response_message: "Answer",
        }}
      />,
    );

    expect(screen.getByRole("button", { name: "agent debug copy" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "user debug copy" })).not.toBeInTheDocument();
  });

  it("keeps debug copy on the user side when there is no agent reply", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{ id: "int_1", prompt_message: "Question" }}
      />,
    );

    expect(screen.getByRole("button", { name: "user debug copy" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "agent debug copy" })).not.toBeInTheDocument();
  });
});
