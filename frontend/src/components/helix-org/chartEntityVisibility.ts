export type ChartEntityKind = 'agents' | 'processors' | 'assets'

export type HiddenChartEntityIDs = Record<ChartEntityKind, string[]>

export const EMPTY_HIDDEN_CHART_ENTITY_IDS: HiddenChartEntityIDs = {
  agents: [],
  processors: [],
  assets: [],
}

const storageKey = (userID: string, orgID: string): string =>
  `helix.orgChart.entityVisibility.${userID}.${orgID}`

const isStringArray = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === 'string')

export const loadHiddenChartEntityIDs = (
  userID: string,
  orgID: string,
): HiddenChartEntityIDs | null => {
  if (!userID || !orgID) return null
  try {
    const raw = window.localStorage.getItem(storageKey(userID, orgID))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return null
    const value = parsed as Partial<HiddenChartEntityIDs>
    if (!isStringArray(value.agents) || !isStringArray(value.processors) || !isStringArray(value.assets)) return null
    return {
      agents: [...new Set(value.agents)],
      processors: [...new Set(value.processors)],
      assets: [...new Set(value.assets)],
    }
  } catch {
    return null
  }
}

export const saveHiddenChartEntityIDs = (
  userID: string,
  orgID: string,
  hiddenIDs: HiddenChartEntityIDs,
): void => {
  if (!userID || !orgID) return
  try {
    window.localStorage.setItem(storageKey(userID, orgID), JSON.stringify(hiddenIDs))
  } catch {
    // The chart remains usable when storage is unavailable.
  }
}
