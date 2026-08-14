export type SpecTaskChatLayout = {
  "spec-task-chat": number;
  "spec-task-content": number;
};

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
