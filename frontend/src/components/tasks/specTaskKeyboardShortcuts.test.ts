import { describe, expect, it } from "vitest";
import {
  getNewTaskShortcutLabel,
  shouldOpenNewTask,
  type SpecTaskViewMode,
} from "./specTaskKeyboardShortcuts";

function keyboardEvent(
  overrides: Partial<KeyboardEvent> = {},
  path: EventTarget[] = [],
): KeyboardEvent {
  return {
    key: "Enter",
    metaKey: true,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    repeat: false,
    composedPath: () => path,
    ...overrides,
  } as KeyboardEvent;
}

describe("project board new-task shortcut", () => {
  it.each<[SpecTaskViewMode, boolean]>([
    ["kanban", true],
    ["workspace", false],
    ["audit", false],
  ])("is scoped to the %s view", (viewMode, expected) => {
    expect(shouldOpenNewTask(keyboardEvent(), viewMode)).toBe(expected);
  });

  it("accepts Command or Control plus Enter without extra modifiers", () => {
    expect(shouldOpenNewTask(keyboardEvent(), "kanban")).toBe(true);
    expect(
      shouldOpenNewTask(
        keyboardEvent({ metaKey: false, ctrlKey: true }),
        "kanban",
      ),
    ).toBe(true);
    expect(shouldOpenNewTask(keyboardEvent({ metaKey: false }), "kanban")).toBe(false);
    expect(shouldOpenNewTask(keyboardEvent({ shiftKey: true }), "kanban")).toBe(false);
    expect(shouldOpenNewTask(keyboardEvent({ altKey: true }), "kanban")).toBe(false);
    expect(shouldOpenNewTask(keyboardEvent({ repeat: true }), "kanban")).toBe(false);
  });

  it("does not fire while typing, including from a retargeted shadow DOM event", () => {
    const input = document.createElement("input");
    const editor = document.createElement("div");
    editor.setAttribute("contenteditable", "true");

    expect(
      shouldOpenNewTask(
        keyboardEvent({}, [input, document.body, window]),
        "kanban",
      ),
    ).toBe(false);
    expect(
      shouldOpenNewTask(
        keyboardEvent({}, [editor, document.body, window]),
        "kanban",
      ),
    ).toBe(false);
  });

  it("formats the shortcut for the active platform", () => {
    expect(getNewTaskShortcutLabel("MacIntel")).toBe("⌘↵");
    expect(getNewTaskShortcutLabel("Linux x86_64")).toBe("Ctrl+↵");
  });
});
