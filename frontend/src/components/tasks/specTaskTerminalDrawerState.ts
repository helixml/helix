export const DEFAULT_SPEC_TASK_TERMINAL_HEIGHT = 320
export const MIN_SPEC_TASK_TERMINAL_HEIGHT = 180
const MAX_SPEC_TASK_TERMINAL_HEIGHT_RATIO = 0.7

export interface SpecTaskTerminalDrawerState {
  open: boolean
  height: number
}

interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export const specTaskTerminalDrawerStorageKey = (taskId: string) =>
  `helix.specTask.${taskId}.terminalDrawer`

export function clampSpecTaskTerminalHeight(
  height: number,
  viewportHeight = typeof window === 'undefined' ? 800 : window.innerHeight,
): number {
  const maximum = Math.max(
    MIN_SPEC_TASK_TERMINAL_HEIGHT,
    Math.floor(viewportHeight * MAX_SPEC_TASK_TERMINAL_HEIGHT_RATIO),
  )
  const safeHeight = Number.isFinite(height)
    ? height
    : DEFAULT_SPEC_TASK_TERMINAL_HEIGHT
  return Math.min(
    Math.max(Math.round(safeHeight), MIN_SPEC_TASK_TERMINAL_HEIGHT),
    maximum,
  )
}

export function loadSpecTaskTerminalDrawerState(
  taskId: string,
  storage: StorageLike = window.localStorage,
): SpecTaskTerminalDrawerState {
  try {
    const stored = JSON.parse(
      storage.getItem(specTaskTerminalDrawerStorageKey(taskId)) ?? 'null',
    ) as Partial<SpecTaskTerminalDrawerState> | null
    return {
      open: stored?.open === true,
      height: clampSpecTaskTerminalHeight(
        stored?.height ?? DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
      ),
    }
  } catch {
    return { open: false, height: DEFAULT_SPEC_TASK_TERMINAL_HEIGHT }
  }
}

export function saveSpecTaskTerminalDrawerState(
  taskId: string,
  state: SpecTaskTerminalDrawerState,
  storage: StorageLike = window.localStorage,
): void {
  try {
    storage.setItem(
      specTaskTerminalDrawerStorageKey(taskId),
      JSON.stringify({
        open: state.open,
        height: clampSpecTaskTerminalHeight(state.height),
      }),
    )
  } catch {
    // Storage is optional; the terminal remains usable for this page load.
  }
}

export function isSpecTaskTerminalToggleShortcut(event: {
  key: string
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
  shiftKey: boolean
}): boolean {
  return (
    (event.ctrlKey || event.metaKey) &&
    !event.altKey &&
    !event.shiftKey &&
    event.key.toLowerCase() === 'j'
  )
}
