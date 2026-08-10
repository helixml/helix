import { afterEach, describe, expect, it, vi } from "vitest";
import { copyTextToClipboard, workspaceFilePath } from "./clipboard";

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalSecureContext = Object.getOwnPropertyDescriptor(window, "isSecureContext");

function restoreProperty(object: object, property: string, descriptor?: PropertyDescriptor) {
  if (descriptor) {
    Object.defineProperty(object, property, descriptor);
  } else {
    Reflect.deleteProperty(object, property);
  }
}

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
    restoreProperty(navigator, "clipboard", originalClipboard);
    restoreProperty(window, "isSecureContext", originalSecureContext);
  });

  it("uses the clipboard API when it is available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: true,
    });
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
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: true,
    });
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
    const clipboardGetter = vi.fn(() => {
      throw new Error("Safari must not evaluate navigator.clipboard on HTTP");
    });
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: false,
    });
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      get: clipboardGetter,
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    await copyTextToClipboard("https://share.example.test");

    expect(clipboardGetter).not.toHaveBeenCalled();
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(document.querySelector("textarea")).toBeNull();
  });
});
