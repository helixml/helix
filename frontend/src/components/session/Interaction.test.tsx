import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { TypesInteractionState } from "../../api/api";
import { Interaction } from "./Interaction";

vi.mock("./InteractionContainer", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./InteractionInference", () => ({
  default: ({
    enableDebugCopy,
    error,
    errorIsHistorical,
    isFromAssistant,
    message,
    workspaceAttachments,
  }: {
    enableDebugCopy?: boolean;
    error?: string;
    errorIsHistorical?: boolean;
    isFromAssistant?: boolean;
    message?: string;
    workspaceAttachments?: Array<{ name: string }>;
  }) => (
    <div data-testid={isFromAssistant ? "agent-reply" : "user-message"}>
      {message}
      {workspaceAttachments?.map((attachment) => (
        <span key={attachment.name}>{attachment.name}</span>
      ))}
      {error && (
        <span
          data-testid="interaction-error"
          data-historical={errorIsHistorical ? "true" : "false"}
        >
          {error}
        </span>
      )}
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

describe("Interaction", () => {
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

  it("renders workspace attachments without exposing the transport manifest", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_attachment",
          prompt_message: [
            "What is in this screenshot?",
            "",
            "Attachments available in the agent workspace:",
            '- Image: "/home/retro/work/incoming/image.png"',
          ].join("\n"),
        }}
      />,
    );

    expect(screen.getByTestId("user-message")).toHaveTextContent("What is in this screenshot?");
    expect(screen.getByTestId("user-message")).toHaveTextContent("image.png");
    expect(screen.queryByText("Attachments available in the agent workspace:")).not.toBeInTheDocument();
  });

  it("does not offer raw-message editing for structured file comments", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_review",
          prompt_message: [
            "Please explain this.",
            "",
            '<review_comment filePath="README.md" startIndex="22" endIndex="22" rangeLabel="L23">',
            "What does this line do?",
            "```md",
            "- Docker",
            "```",
            "</review_comment>",
          ].join("\n"),
        }}
      />,
    );

    expect(screen.queryByRole("button", { name: "edit" })).not.toBeInTheDocument();
  });

  it("renders an interaction error once below the user message", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_error",
          prompt_message: "Question",
          error: "agent failed",
        }}
      />,
    );

    expect(screen.getByTestId("user-message")).not.toHaveTextContent("agent failed");
    expect(screen.getByTestId("agent-reply")).toHaveTextContent("agent failed");
    expect(screen.getAllByTestId("interaction-error")).toHaveLength(1);
  });

  it("removes the error after the retried prompt completes successfully", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_error",
          prompt_message: "Question",
          error: "agent failed",
        }}
        nextInteraction={{
          id: "int_retry",
          prompt_message: "Question",
          response_message: "Answer",
          state: TypesInteractionState.InteractionStateComplete,
        }}
      />,
    );

    expect(screen.queryByTestId("interaction-error")).not.toBeInTheDocument();
    expect(screen.queryByTestId("agent-reply")).not.toBeInTheDocument();
  });

  // The 2026-08-18 shape: a turn aborted mid-work, then the session went on and
  // answered a DIFFERENT question. The old rule only suppressed the alarm when
  // the very next turn retried the SAME prompt, so a recovered session kept a
  // red error and a Retry button that would have re-sent a stale prompt.
  it("demotes an error once a later turn has succeeded", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_error",
          prompt_message: "Do the work",
          response_message: "partial output before the turn died",
          error: "agent turn aborted",
        }}
        nextInteraction={{
          id: "int_other",
          prompt_message: "whats the status?",
          response_message: "Idle.",
          state: TypesInteractionState.InteractionStateComplete,
        }}
        recoveredLater
      />,
    );

    // Still shown — the turn really did fail and its work was abandoned — but
    // as history rather than as an actionable alarm.
    const shown = screen.getByTestId("interaction-error");
    expect(shown).toHaveTextContent("agent turn aborted");
    expect(shown).toHaveAttribute("data-historical", "true");
  });

  it("keeps an error actionable while nothing has succeeded after it", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_error",
          prompt_message: "Do the work",
          error: "agent turn aborted",
        }}
        nextInteraction={{
          id: "int_later",
          prompt_message: "another go",
          state: TypesInteractionState.InteractionStateError,
          error: "failed again",
        }}
        recoveredLater={false}
      />,
    );

    expect(screen.getByTestId("interaction-error")).toHaveAttribute(
      "data-historical",
      "false",
    );
  });

  it("still removes the error entirely when the same prompt was retried", () => {
    // recoveredLater must not weaken the stronger suppression: a successful
    // retry of the same prompt means the work got done, so nothing is shown.
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_error",
          prompt_message: "Question",
          error: "agent failed",
        }}
        nextInteraction={{
          id: "int_retry",
          prompt_message: "Question",
          response_message: "Answer",
          state: TypesInteractionState.InteractionStateComplete,
        }}
        recoveredLater
      />,
    );

    expect(screen.queryByTestId("interaction-error")).not.toBeInTheDocument();
  });

  it("collapses an agent-switch handoff under its divider", () => {
    const systemPrompt = "[System: The coding agent or model configuration changed for this task.]";
    const agentReply = "Ready to continue with Claude Code.";
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_seed",
          trigger: "fork_seed",
          prompt_message: "Agent switched to claude_code at turn 2",
          response_message: "prior transcript",
        }}
        nextInteraction={{
          id: "int_handoff",
          trigger: "fork_handoff",
          prompt_message: systemPrompt,
          response_entries: [{ type: "text", content: agentReply }] as any,
          state: TypesInteractionState.InteractionStateComplete,
        }}
      />,
    );

    const divider = screen.getByRole("button", {
      name: "Agent switched to claude_code at turn 2",
    });
    expect(divider).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText(systemPrompt)).not.toBeInTheDocument();
    expect(screen.queryByText(agentReply)).not.toBeInTheDocument();
    expect(screen.queryByText(/Show transcript/)).not.toBeInTheDocument();

    fireEvent.click(divider);

    expect(divider).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(systemPrompt)).toBeInTheDocument();
    expect(screen.getByText(agentReply)).toBeInTheDocument();
  });

  it("does not render a handoff as a normal conversation turn", () => {
    render(
      <Interaction
        {...baseProps}
        interaction={{
          id: "int_handoff",
          trigger: "fork_handoff",
          prompt_message: "[System: hidden handoff]",
          response_message: "Hidden agent reply",
        }}
      />,
    );

    expect(screen.queryByText("[System: hidden handoff]")).not.toBeInTheDocument();
    expect(screen.queryByText("Hidden agent reply")).not.toBeInTheDocument();
  });
});
