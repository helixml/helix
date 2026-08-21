import { describe, expect, it } from "vitest";
import {
  loadSpecTaskContentPanelOpen,
  resolveSpecTaskChatDefaultLayout,
  saveSpecTaskContentPanelOpen,
  specTaskContentPanelStorageKey,
} from "./specTaskPanelLayout";

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    values,
  };
}

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

  it("defaults an unseen task to closed and remembers visibility per task", () => {
    const storage = memoryStorage();

    expect(loadSpecTaskContentPanelOpen("spt_one", storage)).toBe(false);

    saveSpecTaskContentPanelOpen("spt_one", true, storage);
    expect(loadSpecTaskContentPanelOpen("spt_one", storage)).toBe(true);
    expect(loadSpecTaskContentPanelOpen("spt_two", storage)).toBe(false);

    saveSpecTaskContentPanelOpen("spt_one", false, storage);
    expect(loadSpecTaskContentPanelOpen("spt_one", storage)).toBe(false);
    expect(storage.values.get(specTaskContentPanelStorageKey("spt_one")))
      .toBe("closed");
  });
});
