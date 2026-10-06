import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorkspaceInspector from "./WorkspaceInspector";
import { DESKTOP_RECONNECT_GRACE_MS } from "./workspaceReviewService";

const mocks = vi.hoisted(() => ({
  mergeParams: vi.fn(),
  removeParams: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  workspaces: vi.fn(),
  workspacesArgs: [] as unknown[],
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

vi.mock("./workspaceReviewService", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./workspaceReviewService")>()),
  useWorkspaces: (...args: unknown[]) => {
    mocks.workspacesArgs = args;
    return mocks.workspaces();
  },
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
    mocks.workspaces.mockReturnValue({
      data: { workspaces: [{ is_primary: true, name: "primary", path: "/workspace" }] },
    });
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

describe("WorkspaceInspector when the sandbox is stopped", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.workspaces.mockReturnValue({
      data: undefined,
      error: { response: { status: 503 } },
    });
  });

  it("offers to start the desktop instead of reporting a failure", () => {
    const onStartDesktop = vi.fn();
    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="changes"
        desktopRunning={false}
        onStartDesktop={onStartDesktop}
      />,
    );

    expect(screen.getByText("Desktop not running")).toBeInTheDocument();
    expect(screen.getByText(/sandbox is stopped/i)).toBeInTheDocument();
    // The old copy read as a fault; it must not come back.
    expect(screen.queryByText(/could not load workspace changes/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Start desktop" }));
    expect(onStartDesktop).toHaveBeenCalledTimes(1);
  });

  it("lets the caller name the state, so a merged task says so", () => {
    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="changes"
        desktopRunning={false}
        desktopUnavailableTitle="Task finished"
        desktopUnavailableDescription="This task has been merged to the default branch. Start the desktop to review its workspace."
        onStartDesktop={vi.fn()}
      />,
    );

    expect(screen.getByText("Task finished")).toBeInTheDocument();
    expect(screen.getByText(/merged to the default branch/i)).toBeInTheDocument();
  });

  it("disables the action while a start is already in flight", () => {
    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="files"
        desktopRunning={false}
        onStartDesktop={vi.fn()}
        isDesktopStarting
      />,
    );

    expect(screen.getByText(/browse its workspace files/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Starting/ })).toBeDisabled();
  });

  it("issues no workspace request at all when the sandbox is known to be stopped", () => {
    // The task page already tracks sandbox state, so the inspector should not
    // have to discover the same 503 on every query to find out.
    mocks.workspaces.mockReturnValue({ data: undefined, error: undefined });

    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="changes"
        desktopRunning={false}
        onStartDesktop={vi.fn()}
      />,
    );

    expect(screen.getByText("Desktop not running")).toBeInTheDocument();
    // useWorkspaces is called (hooks must be unconditional) but disabled.
    expect(mocks.workspacesArgs).toEqual(["session-id", false]);
  });

  it("says the sandbox is still connecting while the 503 is fresh", () => {
    // The bug this covers: a headless task that had just started showed
    // "This task's sandbox is stopped" — for a sandbox whose bridge came up a
    // second later — and stayed there until the page was reloaded.
    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="changes"
        onStartDesktop={vi.fn()}
      />,
    );

    expect(screen.getByText("Connecting to the sandbox")).toBeInTheDocument();
    expect(screen.queryByText(/sandbox is stopped/i)).not.toBeInTheDocument();
    // Restarting a task that is already running must not be offered here.
    expect(screen.queryByRole("button", { name: "Start desktop" })).not.toBeInTheDocument();
  });

  it("still reports a stopped sandbox once the 503s have persisted", () => {
    // A session row can claim to be running while its container is really
    // gone. The optimistic state is bounded so the start action comes back.
    vi.useFakeTimers();
    try {
      render(
        <WorkspaceInspector
          sessionId="session-id"
          primarySurface="changes"
          onStartDesktop={vi.fn()}
        />,
      );

      expect(screen.getByText("Connecting to the sandbox")).toBeInTheDocument();

      act(() => {
        vi.advanceTimersByTime(DESKTOP_RECONNECT_GRACE_MS + 1);
      });

      expect(screen.getByText("Desktop not running")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Start desktop" })).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("offers the subscription login when that is what is blocking the desktop", () => {
    const onConnectSubscription = vi.fn();
    render(
      <WorkspaceInspector
        sessionId="session-id"
        primarySurface="changes"
        desktopRunning={false}
        onStartDesktop={vi.fn()}
        onConnectSubscription={onConnectSubscription}
        connectSubscriptionLabel="Connect Claude"
        desktopUnavailableDetail="This agent signs in with a Claude subscription, and none is connected for this organisation."
      />,
    );

    expect(screen.getByText(/none is connected for this organisation/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Connect Claude" }));
    expect(onConnectSubscription).toHaveBeenCalledTimes(1);
  });
});
