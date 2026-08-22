import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SpecTaskViewToolbar from "./SpecTaskViewToolbar";

vi.mock("../../hooks/useElementWidth", () => ({
  useElementWidth: () => [{ current: null }, 800],
}));

let isPhone = false;
vi.mock("../../hooks/useIsPhone", () => ({ default: () => isPhone }));

describe("SpecTaskViewToolbar", () => {
  beforeEach(() => {
    isPhone = false;
  });

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

  it("keeps every view inline on a wide screen", () => {
    render(
      <SpecTaskViewToolbar currentView="chat" onViewChange={vi.fn()} hasSession showChatTab />,
    );

    for (const label of ["Chat", "Desktop", "Browser", "Diff", "Files", "Details"]) {
      expect(screen.getByRole("button", { name: `${label} view` })).toBeInTheDocument();
    }
  });

  it("folds the deliberate views into the menu on a phone", () => {
    isPhone = true;
    const onViewChange = vi.fn();
    render(
      <SpecTaskViewToolbar
        currentView="chat"
        onViewChange={onViewChange}
        hasSession
        showChatTab
      />,
    );

    // Only the views you flick between stay inline.
    for (const label of ["Chat", "Browser", "Diff"]) {
      expect(screen.getByRole("button", { name: `${label} view` })).toBeInTheDocument();
    }
    for (const label of ["Desktop", "Files", "Details"]) {
      expect(screen.queryByRole("button", { name: `${label} view` })).not.toBeInTheDocument();
    }

    fireEvent.click(screen.getByRole("button", { name: "More actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Files" }));

    expect(onViewChange).toHaveBeenCalledWith("files");
  });

  it("drops the close button on a phone, where the panel is the whole screen", () => {
    isPhone = true;
    render(
      <SpecTaskViewToolbar
        currentView="chat"
        onViewChange={vi.fn()}
        hasSession
        onClosePanel={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Close" })).not.toBeInTheDocument();
  });
});
