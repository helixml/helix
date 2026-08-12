import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TypesInteraction } from "../../api/api";
import ChangedFilesCard from "./ChangedFilesCard";

const setParams = vi.fn();

vi.mock("../../hooks/useRouter", () => ({
  default: () => ({ params: {}, setParams }),
}));

const interaction = {
  id: "int_changed_files",
  code_changes: {
    status: "ready",
    additions: 6_500,
    deletions: 1_300,
    files: [
      { path: "frontend/src/App.tsx", additions: 6_000, deletions: 1_000 },
      { path: "frontend/src/App.test.tsx", additions: 100, deletions: 100 },
      { path: "api/pkg/server.go", additions: 200, deletions: 100 },
      { path: "package.json", additions: 200, deletions: 100 },
    ],
  },
} as TypesInteraction;

describe("ChangedFilesCard", () => {
  beforeEach(() => {
    localStorage.clear();
    setParams.mockClear();
  });

  it("renders the compact T3-style preview for a large latest change", () => {
    const { container } = render(<ChangedFilesCard interaction={interaction} isLatest />);

    expect(container.querySelector('[data-changed-files-state="preview"]')).toBeTruthy();
    expect(screen.getByText("4 changed files")).toBeInTheDocument();
    expect(screen.getByLabelText("6500 additions, 1300 deletions")).toHaveTextContent("+6.5k-1.3k");
    expect(screen.getByText("frontend")).toBeInTheDocument();
    expect(screen.getByText("2 files")).toBeInTheDocument();
    expect(screen.getByText("api")).toBeInTheDocument();
    expect(screen.getByText("root")).toBeInTheDocument();
    expect(screen.getByText("App.tsx")).toBeInTheDocument();
    expect(screen.queryByText("App.test.tsx")).not.toBeInTheDocument();
    expect(screen.getByText("server.go")).toBeInTheDocument();
    expect(screen.getByText("package.json")).toBeInTheDocument();
    expect(container.querySelector('[data-pierre-icon="helix-file-icon-package-json"]')).toBeTruthy();
    expect(screen.getByText("Show all 4 files")).toBeInTheDocument();
  });

  it("says so when a turn's checkpoint capture failed", () => {
    const failed = {
      id: "int_failed",
      code_changes: { status: "error", files: [], error: "container not ready" },
    } as TypesInteraction;

    const { container } = render(<ChangedFilesCard interaction={failed} isLatest />);

    // Rendering nothing here made a broken capture look like a turn that
    // changed no files.
    expect(container.querySelector('[data-changed-files-state="unavailable"]')).toBeTruthy();
    expect(screen.getByText("Changed files unavailable for this turn")).toBeInTheDocument();
  });

  it("renders nothing for a turn that simply changed no files", () => {
    const empty = {
      id: "int_empty",
      code_changes: { status: "ready", files: [] },
    } as TypesInteraction;

    const { container } = render(<ChangedFilesCard interaction={empty} isLatest />);
    expect(container).toBeEmptyDOMElement();
  });

  it("opens large trees collapsed and deep-links file chips", () => {
    const { container } = render(<ChangedFilesCard interaction={interaction} isLatest />);

    fireEvent.click(screen.getByTitle("api/pkg/server.go"));
    expect(setParams).toHaveBeenCalledWith({
      view: "changes",
      interaction: "int_changed_files",
      file: "api/pkg/server.go",
    }, true);

    fireEvent.click(screen.getByText("Show all 4 files"));
    expect(container.querySelector('[data-changed-files-state="expanded"]')).toBeTruthy();
    expect(screen.getByRole("button", { name: "Expand all folders" })).toBeInTheDocument();
    expect(screen.queryByText("App.tsx")).not.toBeInTheDocument();
  });
});
