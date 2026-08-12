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
  return platform.includes("Mac") ? "⌘ Enter" : "Ctrl Enter";
}

export function registerNewTaskShortcut(
  viewMode: SpecTaskViewMode,
  openNewTask: () => void,
): () => void {
  const handleKeyDown = (event: KeyboardEvent) => {
    if (!shouldOpenNewTask(event, viewMode)) return;
    event.preventDefault();
    openNewTask();
  };

  window.addEventListener("keydown", handleKeyDown, { capture: true });
  return () => window.removeEventListener("keydown", handleKeyDown, {
    capture: true,
  });
}
