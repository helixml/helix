import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SpecTaskViewToolbar from "./SpecTaskViewToolbar";

vi.mock("../../hooks/useElementWidth", () => ({
  useElementWidth: () => [{ current: null }, 800],
}));

describe("SpecTaskViewToolbar", () => {
  it("hides the desktop view for a headless task and keeps panel collapse available", () => {
    const collapse = vi.fn();
    render(
      <SpecTaskViewToolbar
        currentView="changes"
        onViewChange={vi.fn()}
        hasSession
        showDesktop={false}
        onCollapsePanel={collapse}
      />,
    );

    expect(screen.queryByRole("button", { name: "Desktop view" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Collapse task panel" }));
    expect(collapse).toHaveBeenCalledOnce();
  });
});
