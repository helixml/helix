export type SpecTaskChatLayout = {
  "spec-task-chat": number;
  "spec-task-content": number;
};

interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const DEFAULT_LAYOUT: SpecTaskChatLayout = {
  "spec-task-chat": 50,
  "spec-task-content": 50,
};

const COLLAPSED_CONTENT_LAYOUT: SpecTaskChatLayout = {
  "spec-task-chat": 100,
  "spec-task-content": 0,
};

export function resolveSpecTaskChatDefaultLayout(
  savedLayout: Record<string, number> | null,
  collapseContent: boolean,
): Record<string, number> {
  if (collapseContent) return COLLAPSED_CONTENT_LAYOUT;
  return savedLayout || DEFAULT_LAYOUT;
}

export const specTaskContentPanelStorageKey = (taskId: string): string =>
  `helix.specTask.${taskId}.contentPanel`;

export function loadSpecTaskContentPanelOpen(
  taskId: string,
  storage: StorageLike = window.localStorage,
): boolean {
  try {
    return storage.getItem(specTaskContentPanelStorageKey(taskId)) === "open";
  } catch {
    return false;
  }
}

export function saveSpecTaskContentPanelOpen(
  taskId: string,
  open: boolean,
  storage: StorageLike = window.localStorage,
): void {
  try {
    storage.setItem(
      specTaskContentPanelStorageKey(taskId),
      open ? "open" : "closed",
    );
  } catch {
    // Storage is optional; the panel remains usable for this page load.
  }
}
