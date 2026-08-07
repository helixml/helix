import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorkspaceInspector from "./WorkspaceInspector";

const mocks = vi.hoisted(() => ({
  mergeParams: vi.fn(),
  removeParams: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("../../hooks/useLightTheme", () => ({
  default: () => ({
    backgroundColor: "#111",
    border: "#333",
    textColor: "#fff",
    textColorFaded: "#aaa",
  }),
}));

vi.mock("../../hooks/useRouter", () => ({
  default: () => ({
    mergeParams: mocks.mergeParams,
    params: { preview: "first.go" },
    removeParams: mocks.removeParams,
  }),
}));

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: mocks.success, error: mocks.error }),
}));

vi.mock("./workspaceReviewService", () => ({
  useWorkspaces: () => ({
    data: { workspaces: [{ is_primary: true, name: "primary", path: "/workspace" }] },
  }),
}));

vi.mock("./WorkspaceDiffSurface", () => ({ default: () => null }));
vi.mock("./WorkspaceFileSurface", () => ({
  default: ({ onOpenFile, path }: { onOpenFile: (path: string) => void; path: string | null }) => (
    <div>
      <span>Selected: {path || "none"}</span>
      <button onClick={() => onOpenFile("second.go")}>Open second</button>
      <button onClick={() => onOpenFile("third.go")}>Open third</button>
    </div>
  ),
}));

describe("WorkspaceInspector tab menu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("MutationObserver", class {
      disconnect() {}
      observe() {}
      takeRecords() { return []; }
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("offers standard close actions and closes tabs to the right", async () => {
    render(<WorkspaceInspector sessionId="session-id" primarySurface="files" />);
    fireEvent.click(screen.getByRole("button", { name: "Open second" }));
    expect(await screen.findByRole("tab", { name: /second\.go/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open third" }));
    expect(await screen.findByRole("tab", { name: /third\.go/ })).toBeInTheDocument();

    fireEvent.contextMenu(screen.getByRole("tab", { name: /second\.go/ }));

    expect(await screen.findByRole("menuitem", { name: "Copy full path" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Close" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Close others" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Close to the right" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Close all" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("menuitem", { name: "Close to the right" }));

    expect(screen.queryByRole("tab", { name: /third\.go/ })).not.toBeInTheDocument();
    expect(screen.getByText("Selected: second.go")).toBeInTheDocument();
    expect(mocks.mergeParams).toHaveBeenLastCalledWith({ view: "files", preview: "second.go" });
  });
});
