import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import WorkspaceFileComment, { type WorkspaceFileCommentEntry } from "./WorkspaceFileComment";

const draft: WorkspaceFileCommentEntry = {
  id: "comment-1",
  kind: "draft",
  startLine: 12,
  endLine: 13,
  text: "",
};

describe("WorkspaceFileComment", () => {
  it("keeps the same focused input while the draft changes", () => {
    const onParentKeyDown = vi.fn();
    const onSubmit = vi.fn();
    render(
      <div onKeyDown={onParentKeyDown}>
        <WorkspaceFileComment entry={draft} onCancel={vi.fn()} onSubmit={onSubmit} />
      </div>,
    );

    const input = screen.getByRole("textbox", { name: "Comment on lines 12 to 13" });
    input.focus();

    fireEvent.keyDown(input, { key: "f" });
    fireEvent.change(input, { target: { value: "f" } });
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input).toHaveFocus();
    expect(onParentKeyDown).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: "fix this\nwithout editing the file" } });
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input).toHaveFocus();

    fireEvent.click(screen.getByRole("button", { name: "Add comment" }));
    expect(onSubmit).toHaveBeenCalledWith(draft, "fix this\nwithout editing the file");
  });

  it("submits with Ctrl+Enter without bubbling into the file editor", () => {
    const onParentKeyDown = vi.fn();
    const onSubmit = vi.fn();
    const populatedDraft = { ...draft, text: "Review this" };
    render(
      <div onKeyDown={onParentKeyDown}>
        <WorkspaceFileComment entry={populatedDraft} onCancel={vi.fn()} onSubmit={onSubmit} />
      </div>,
    );

    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter", ctrlKey: true });

    expect(onSubmit).toHaveBeenCalledWith(populatedDraft, "Review this");
    expect(onParentKeyDown).not.toHaveBeenCalled();
  });

  it("renders a committed comment with its line range and delete action", () => {
    render(
      <WorkspaceFileComment
        entry={{ ...draft, kind: "comment", text: "Keep this focused." }}
        onCancel={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.getByText("File comment")).toBeInTheDocument();
    expect(screen.getByText("L12–13")).toBeInTheDocument();
    expect(screen.getByText("Keep this focused.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete comment" })).toBeInTheDocument();
  });
});
