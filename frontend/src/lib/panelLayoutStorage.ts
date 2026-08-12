export type PanelLayout = Record<string, number>

const isValidLayout = (value: unknown, panelIds: readonly string[]): value is PanelLayout => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false

  const layout = value as Record<string, unknown>
  const keys = Object.keys(layout)
  if (keys.length !== panelIds.length || panelIds.some((id) => !keys.includes(id))) return false

  const sizes = panelIds.map((id) => layout[id] as number)
  if (sizes.some((size) => typeof size !== 'number' || !Number.isFinite(size) || size <= 0 || size > 100)) {
    return false
  }

  const total = sizes.reduce((sum, size) => sum + size, 0)
  return Math.abs(total - 100) < 0.1
}

export const loadPanelLayout = (
  storageKey: string,
  panelIds: readonly string[],
): PanelLayout | null => {
  if (!storageKey || panelIds.length === 0) return null

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    return isValidLayout(parsed, panelIds) ? parsed : null
  } catch {
    return null
  }
}

export const savePanelLayout = (
  storageKey: string,
  layout: PanelLayout,
  panelIds: readonly string[],
): void => {
  if (!storageKey || !isValidLayout(layout, panelIds)) return

  try {
    window.localStorage.setItem(storageKey, JSON.stringify(layout))
  } catch {
    // Private mode / quota — the current layout still works in memory.
  }
}
