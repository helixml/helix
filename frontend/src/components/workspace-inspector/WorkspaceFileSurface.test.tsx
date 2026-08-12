import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorkspaceFileSurface from "./WorkspaceFileSurface";

const mocks = vi.hoisted(() => ({ file: vi.fn() }));

vi.mock("../../hooks/useLightTheme", () => ({
  default: () => ({ isLight: false }),
}));
vi.mock("./WorkspaceFileTree", () => ({ default: () => <div data-testid="tree" /> }));
vi.mock("./WorkspaceEditableFile", () => ({
  default: ({ initialContents }: { initialContents: string }) => (
    <pre data-testid="file-view">{initialContents}</pre>
  ),
}));
vi.mock("./workspaceReviewService", () => ({
  useWorkspaceFile: (...args: unknown[]) => mocks.file(...args),
}));

const idle = { data: undefined, isLoading: false, isError: false };

const renderSurface = (path: string | null = "src/app.ts") =>
  render(
    <WorkspaceFileSurface
      sessionId="ses_1"
      workspace="primary"
      workspacePath="/home/retro/work/primary"
      path={path}
      revealPath={null}
      onOpenFile={vi.fn()}
      comments={[]}
      onUpsertComment={vi.fn()}
      onRemoveComment={vi.fn()}
    />,
  );

describe("WorkspaceFileSurface", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.file.mockReturnValue(idle);
  });

  it("prompts for a selection before any file is opened", () => {
    renderSurface(null);
    expect(screen.getByText("Choose a file from the browser.")).toBeInTheDocument();
    expect(screen.queryByTestId("file-view")).not.toBeInTheDocument();
  });

  it("renders file contents once loaded", () => {
    mocks.file.mockReturnValue({
      ...idle,
      data: { contents: "export const a = 1\n", byte_length: 19, content_hash: "abc" },
    });

    renderSurface();

    expect(screen.getByTestId("file-view")).toHaveTextContent("export const a = 1");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("says a truncated file is partial rather than showing the prefix silently", () => {
    mocks.file.mockReturnValue({
      ...idle,
      data: { contents: "prefix", byte_length: 5_242_880, truncated: true, content_hash: "abc" },
    });

    renderSurface();

    expect(screen.getByText(/larger than the preview limit/i)).toBeInTheDocument();
    expect(screen.getByText(/5,242,880/)).toBeInTheDocument();
  });

  it("represents a binary file explicitly instead of rendering bytes", () => {
    mocks.file.mockReturnValue({
      ...idle,
      data: { binary: true, byte_length: 2048, contents: "" },
    });

    renderSurface();

    expect(screen.getByText("Binary file")).toBeInTheDocument();
    expect(screen.getByText("2,048 bytes")).toBeInTheDocument();
    expect(screen.queryByTestId("file-view")).not.toBeInTheDocument();
  });

  it("surfaces a failed read as an error, never as an empty file", () => {
    mocks.file.mockReturnValue({ ...idle, isError: true });

    renderSurface();

    expect(screen.getByText("Could not read src/app.ts.")).toBeInTheDocument();
    expect(screen.queryByTestId("file-view")).not.toBeInTheDocument();
  });

  it("keeps the browser tree available alongside the viewer", () => {
    mocks.file.mockReturnValue({ ...idle, data: { contents: "x", content_hash: "h" } });
    renderSurface();
    expect(screen.getByTestId("tree")).toBeInTheDocument();
  });
});
