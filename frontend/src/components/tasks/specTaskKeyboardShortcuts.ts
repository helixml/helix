export type SpecTaskViewMode = "kanban" | "workspace" | "audit";

const EDITABLE_TARGET_SELECTOR = [
  "input",
  "textarea",
  "select",
  '[role="textbox"]',
  '[role="searchbox"]',
  '[role="combobox"]',
  '[contenteditable]:not([contenteditable="false"])',
].join(",");

function hasEditableTarget(event: KeyboardEvent): boolean {
  return event.composedPath().some((target) =>
    target instanceof Element &&
    (target.matches(EDITABLE_TARGET_SELECTOR) ||
      (target instanceof HTMLElement && target.isContentEditable)),
  );
}

export function shouldOpenNewTask(
  event: KeyboardEvent,
  viewMode: SpecTaskViewMode,
): boolean {
  return viewMode === "kanban" &&
    event.key === "Enter" &&
    (event.metaKey || event.ctrlKey) &&
    !event.altKey &&
    !event.shiftKey &&
    !event.repeat &&
    !hasEditableTarget(event);
}

export function getNewTaskShortcutLabel(platform = navigator.platform): string {
  return platform.includes("Mac") ? "⌘↵" : "Ctrl+↵";
}
