import { describe, expect, it } from "vitest";
import { closeWorkspaceTabs } from "./workspaceTabs";

const files = ["first.go", "second.go", "third.go", "fourth.go"];

describe("closeWorkspaceTabs", () => {
  it("selects the next tab when the active tab is closed", () => {
    expect(closeWorkspaceTabs(files, "second.go", "second.go", "close")).toEqual({
      activeFile: "third.go",
      openFiles: ["first.go", "third.go", "fourth.go"],
    });
  });

  it("selects the preceding tab when the last active tab is closed", () => {
    expect(closeWorkspaceTabs(files, "fourth.go", "fourth.go", "close")).toEqual({
      activeFile: "third.go",
      openFiles: ["first.go", "second.go", "third.go"],
    });
  });

  it("keeps the active tab when a background tab is closed", () => {
    expect(closeWorkspaceTabs(files, "first.go", "third.go", "close")).toEqual({
      activeFile: "first.go",
      openFiles: ["first.go", "second.go", "fourth.go"],
    });
  });

  it("keeps the file browser selected when closing a background tab", () => {
    expect(closeWorkspaceTabs(files, null, "third.go", "close")).toEqual({
      activeFile: null,
      openFiles: ["first.go", "second.go", "fourth.go"],
    });
  });

  it("closes every tab other than the target", () => {
    expect(closeWorkspaceTabs(files, "first.go", "third.go", "close_others")).toEqual({
      activeFile: "third.go",
      openFiles: ["third.go"],
    });
  });

  it("closes tabs to the right and selects the target if needed", () => {
    expect(closeWorkspaceTabs(files, "fourth.go", "second.go", "close_right")).toEqual({
      activeFile: "second.go",
      openFiles: ["first.go", "second.go"],
    });
  });

  it("closes all tabs", () => {
    expect(closeWorkspaceTabs(files, "second.go", "second.go", "close_all")).toEqual({
      activeFile: null,
      openFiles: [],
    });
  });
});
