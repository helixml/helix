import { describe, expect, it } from "vitest";
import { resolveSpecTaskChatDefaultLayout } from "./specTaskPanelLayout";

describe("resolveSpecTaskChatDefaultLayout", () => {
  it("starts headless content collapsed without an imperative panel call", () => {
    expect(resolveSpecTaskChatDefaultLayout({
      "spec-task-chat": 40,
      "spec-task-content": 60,
    }, true)).toEqual({
      "spec-task-chat": 100,
      "spec-task-content": 0,
    });
  });

  it("restores the saved expanded split otherwise", () => {
    expect(resolveSpecTaskChatDefaultLayout({
      "spec-task-chat": 40,
      "spec-task-content": 60,
    }, false)).toEqual({
      "spec-task-chat": 40,
      "spec-task-content": 60,
    });
  });
});
