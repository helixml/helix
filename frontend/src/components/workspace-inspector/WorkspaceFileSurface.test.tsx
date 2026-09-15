import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorkspaceFileSurface from "./WorkspaceFileSurface";

const mocks = vi.hoisted(() => ({ file: vi.fn(), download: vi.fn() }));

vi.mock("../../hooks/useApi", () => ({
  default: () => ({
    getApiClient: () => ({
      v1ExternalAgentsWorkspaceFileDownloadDetail: mocks.download,
    }),
  }),
}));

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
      baseBranch="main"
      pollInterval={3_000}
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
    mocks.download.mockResolvedValue({ data: new Blob(["image"]) });
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => "blob:workspace-image"),
      revokeObjectURL: vi.fn(),
    });
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

  it.each([
    ["logo/icon.png", true],
    ["logo/lockup.svg", false],
  ])("loads %s from the binary-safe download endpoint", async (path, binary) => {
    mocks.file.mockReturnValue({
      ...idle,
      data: { binary, byte_length: 2048, contents: binary ? "" : "<svg />", content_hash: "abc" },
    });

    renderSurface(path);

    expect(await screen.findByRole("img", { name: path.split("/").pop() })).toHaveAttribute(
      "src",
      "blob:workspace-image",
    );
    expect(mocks.download).toHaveBeenCalledWith(
      "ses_1",
      { path, workspace: "primary" },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(screen.queryByTestId("file-view")).not.toBeInTheDocument();
  });

  it("revokes an image object URL when the preview closes", async () => {
    mocks.file.mockReturnValue({ ...idle, data: { binary: true, contents: "" } });
    const view = renderSurface("logo/icon.png");
    await screen.findByRole("img");

    view.unmount();

    await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:workspace-image"));
  });

  it("revokes an image object URL when its request finishes after unmount", async () => {
    let resolveDownload: (response: { data: Blob }) => void = () => {};
    mocks.download.mockReturnValue(
      new Promise((resolve) => {
        resolveDownload = resolve;
      }),
    );
    mocks.file.mockReturnValue({ ...idle, data: { binary: true, contents: "" } });
    const view = renderSurface("logo/icon.png");
    await waitFor(() => expect(mocks.download).toHaveBeenCalled());

    view.unmount();
    resolveDownload({ data: new Blob(["image"]) });

    await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:workspace-image"));
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
