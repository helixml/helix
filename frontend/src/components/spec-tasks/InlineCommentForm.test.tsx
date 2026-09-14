import { fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import InlineCommentForm from "./InlineCommentForm";

describe("InlineCommentForm", () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("keeps a narrow comment form inside the plan panel", () => {
    const onCreate = vi.fn();
    const { container } = render(
      <InlineCommentForm
        show
        yPos={100}
        selectedText="Selected plan text"
        commentText="Please clarify"
        onCommentChange={vi.fn()}
        onCreate={onCreate}
        onCancel={vi.fn()}
        isNarrowViewport
        submitLabel="Add to chat"
      />,
    );

    expect(container.querySelector(".MuiPaper-root")).toHaveStyle({
      position: "absolute",
      top: "132px",
    });
    fireEvent.click(screen.getByRole("button", { name: "Add to chat" }));
    expect(onCreate).toHaveBeenCalledOnce();
  });
});
