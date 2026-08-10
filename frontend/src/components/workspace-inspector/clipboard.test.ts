import { afterEach, describe, expect, it, vi } from "vitest";
import { copyTextToClipboard, workspaceFilePath } from "./clipboard";

describe("workspaceFilePath", () => {
  it("joins a sandbox workspace root and repository-relative path", () => {
    expect(workspaceFilePath("/home/retro/work/keel/", "/extension/aws.go")).toBe(
      "/home/retro/work/keel/extension/aws.go",
    );
  });
});

describe("copyTextToClipboard", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("uses the clipboard API when it is available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    await copyTextToClipboard("src/index.ts");

    expect(writeText).toHaveBeenCalledWith("src/index.ts");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("falls back to document copy when the clipboard API is blocked", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("blocked"));
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    await copyTextToClipboard("file contents");

    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("falls back when the clipboard API is unavailable on an HTTP origin", async () => {
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: undefined,
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    await copyTextToClipboard("https://share.example.test");

    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(document.querySelector("textarea")).toBeNull();
  });
});
