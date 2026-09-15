import { fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import InlineCommentForm from "./InlineCommentForm";

describe("InlineCommentForm", () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("keeps a narrow comment form inside the plan panel", () => {
    const onCreate = vi.fn();
    const onSend = vi.fn();
    const { container } = render(
      <InlineCommentForm
        show
        yPos={100}
        selectedText="Selected plan text"
        commentText="Please clarify"
        onCommentChange={vi.fn()}
        onCreate={onCreate}
        onSend={onSend}
        onCancel={vi.fn()}
        isNarrowViewport
        submitLabel="Add to chat"
      />,
    );

    expect(container.querySelector("[data-plan-comment-draft]")).toHaveStyle({
      position: "absolute",
      top: "108px",
      maxWidth: "480px",
    });
    expect(screen.queryByText("Add Comment")).not.toBeInTheDocument();
    expect(screen.queryByText(/Selected plan text/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add to chat" }));
    expect(onCreate).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSend).toHaveBeenCalledOnce();
  });

  it("provides keyboard shortcuts for send and add to chat", () => {
    const onCreate = vi.fn();
    const onSend = vi.fn();
    render(
      <InlineCommentForm
        show
        yPos={100}
        selectedText="Selected plan text"
        commentText="Please clarify"
        onCommentChange={vi.fn()}
        onCreate={onCreate}
        onSend={onSend}
        onCancel={vi.fn()}
        submitLabel="Add to chat"
      />,
    );

    const input = screen.getByRole("textbox", { name: "Comment on selected text" });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    expect(onSend).toHaveBeenCalledOnce();
    expect(onCreate).not.toHaveBeenCalled();

    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, shiftKey: true });
    expect(onCreate).toHaveBeenCalledOnce();
    expect(
      screen.getByText("⌘/Ctrl Enter to send · Shift+⌘/Ctrl Enter to add to chat"),
    ).toBeInTheDocument();
  });
});
