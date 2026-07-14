// Per-user, per-org pan/zoom for the org chart. Node positions are shared
// (server-side); the camera is personal and lives in localStorage.

export type ChartViewport = {
  x: number
  y: number
  zoom: number
}

const keyFor = (userId: string, orgId: string): string =>
  `helix.orgChart.viewport.${userId}.${orgId}`

const isViewport = (v: unknown): v is ChartViewport => {
  if (!v || typeof v !== 'object') return false
  const o = v as Record<string, unknown>
  return (
    typeof o.x === 'number' &&
    Number.isFinite(o.x) &&
    typeof o.y === 'number' &&
    Number.isFinite(o.y) &&
    typeof o.zoom === 'number' &&
    Number.isFinite(o.zoom) &&
    o.zoom > 0
  )
}

export const loadChartViewport = (
  userId: string,
  orgId: string,
): ChartViewport | null => {
  if (!userId || !orgId) return null
  try {
    const raw = window.localStorage.getItem(keyFor(userId, orgId))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    return isViewport(parsed) ? parsed : null
  } catch {
    return null
  }
}

export const saveChartViewport = (
  userId: string,
  orgId: string,
  viewport: ChartViewport,
): void => {
  if (!userId || !orgId || !isViewport(viewport)) return
  try {
    window.localStorage.setItem(
      keyFor(userId, orgId),
      JSON.stringify({ x: viewport.x, y: viewport.y, zoom: viewport.zoom }),
    )
  } catch {
    // Quota / private mode — ignore; chart still works without persistence.
  }
}
