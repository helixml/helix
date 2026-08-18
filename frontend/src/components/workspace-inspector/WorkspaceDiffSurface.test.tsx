import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorkspaceDiffSurface from "./WorkspaceDiffSurface";
import { DESKTOP_RECONNECT_GRACE_MS } from "./workspaceReviewService";

const mocks = vi.hoisted(() => ({
  live: vi.fn(),
  turn: vi.fn(),
  refetch: vi.fn(),
}));

vi.mock("../../hooks/useLightTheme", () => ({
  default: () => ({ isLight: false, backgroundColor: "#111", border: "#333", textColor: "#fff", textColorFaded: "#aaa" }),
}));
vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}));
vi.mock("@pierre/diffs/react", async () => {
  const { forwardRef } = await import("react");
  return {
    CodeView: forwardRef(({ items }: { items: { id: string }[] }, _ref) => (
      <div data-testid="code-view">{items.map((item) => item.id).join(",")}</div>
    )),
  };
});
vi.mock("./workspaceReviewService", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./workspaceReviewService")>()),
  useWorkspaceReview: (...args: unknown[]) => mocks.live(...args),
  useTurnWorkspaceReview: (...args: unknown[]) => mocks.turn(...args),
}));

const PATCH = `diff --git a/src/app.ts b/src/app.ts
--- a/src/app.ts
+++ b/src/app.ts
@@ -1,1 +1,2 @@
 first
+second
`;

const source = (id: string, overrides = {}) => ({
  id,
  title: id === "all" ? "All task changes" : "Working tree",
  patch: PATCH,
  files: [{ path: "src/app.ts", additions: 1, deletions: 0 }],
  total_additions: 1,
  total_deletions: 0,
  ...overrides,
});

const renderSurface = (props = {}) =>
  render(
    <WorkspaceDiffSurface
      sessionId="ses_1"
      workspace="primary"
      workspacePath="/home/retro/work/primary"
      baseBranch="main"
      pollInterval={3000}
      onOpenFile={vi.fn()}
      onExitTurn={vi.fn()}
      onWorkspaceResolved={vi.fn()}
      {...props}
    />,
  );

const idleQuery = { data: undefined, isLoading: false, isError: false, isFetching: false, failureCount: 0, refetch: mocks.refetch };

describe("WorkspaceDiffSurface", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    mocks.live.mockReturnValue(idleQuery);
    mocks.turn.mockReturnValue(idleQuery);
  });

  afterEach(() => localStorage.clear());

  it("keeps the last loaded review visible when a background poll fails", () => {
    mocks.live.mockReturnValue({
      ...idleQuery,
      data: { workspace: "primary", sources: [source("all")] },
      isError: true,
    });

    renderSurface();

    // The rendered diff survives; the failure is a non-blocking notice.
    expect(screen.getByTestId("code-view")).toBeInTheDocument();
    expect(screen.getByText(/last loaded changes/i)).toBeInTheDocument();
    expect(screen.queryByText("Could not load workspace changes.")).not.toBeInTheDocument();
  });

  it("shows a blocking error only when no review has ever loaded", () => {
    mocks.live.mockReturnValue({ ...idleQuery, isError: true });

    renderSurface();

    expect(screen.getByText("Could not load workspace changes.")).toBeInTheDocument();
    expect(screen.queryByTestId("code-view")).not.toBeInTheDocument();
  });

  it("distinguishes an empty selection from an unavailable one", () => {
    mocks.live.mockReturnValue({
      ...idleQuery,
      data: { workspace: "primary", sources: [source("working_tree", { patch: "" })] },
    });

    renderSurface();

    // The persisted default scope is "all", which this response does not carry.
    expect(screen.getByText(/not available for this workspace/i)).toBeInTheDocument();
  });

  it("persists layout and wrap preferences across remounts", () => {
    mocks.live.mockReturnValue({
      ...idleQuery,
      data: { workspace: "primary", sources: [source("all")] },
    });

    const first = renderSurface();
    fireEvent.click(screen.getByRole("button", { name: "Split diff" }));
    fireEvent.click(screen.getByRole("button", { name: "Wrap long lines" }));
    first.unmount();

    renderSurface();
    expect(screen.getByRole("button", { name: "Split diff" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Disable line wrapping" })).toHaveAttribute("aria-pressed", "true");
  });

  it("stops polling the live review while an immutable turn diff is shown", () => {
    mocks.turn.mockReturnValue({ ...idleQuery, data: source("turn:int_1") });

    renderSurface({ interactionId: "int_1" });

    // Sixth argument is the query's enabled flag.
    expect(mocks.live).toHaveBeenCalled();
    expect(mocks.live.mock.calls[0][5]).toBe(false);
    expect(screen.getByTestId("code-view")).toBeInTheDocument();
  });

  it("reads a fresh 503 as a sandbox that is still coming up", () => {
    // A task launched seconds ago answers 503 until its container registers
    // the bridge. Telling the reviewer it is stopped — and offering to start
    // it — is wrong for a task that is already working.
    mocks.live.mockReturnValue({
      ...idleQuery,
      isError: true,
      error: { response: { status: 503 } },
    });

    renderSurface({ onStartDesktop: vi.fn() });

    expect(screen.getByText("Connecting to the sandbox")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start desktop" })).not.toBeInTheDocument();
  });

  it("offers to start the desktop once the 503s have persisted", () => {
    mocks.live.mockReturnValue({
      ...idleQuery,
      isError: true,
      error: { response: { status: 503 } },
    });

    vi.useFakeTimers();
    try {
      renderSurface({ onStartDesktop: vi.fn() });
      act(() => {
        vi.advanceTimersByTime(DESKTOP_RECONNECT_GRACE_MS + 1);
      });

      expect(screen.getByText("Desktop not running")).toBeInTheDocument();
      expect(screen.queryByText("Could not load workspace changes.")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("still reports a genuine failure as an error", () => {
    mocks.live.mockReturnValue({
      ...idleQuery,
      isError: true,
      error: { response: { status: 500 } },
    });

    renderSurface();

    expect(screen.getByText("Could not load workspace changes.")).toBeInTheDocument();
    expect(screen.queryByText("Desktop not running")).not.toBeInTheDocument();
  });

});
