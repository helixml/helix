export type TerminalSplitDirection = 'horizontal' | 'vertical'

export interface TerminalGroup {
  id: string
  paneNames: string[]
  direction: TerminalSplitDirection
}

export interface TerminalLayoutState {
  version: 1
  groups: TerminalGroup[]
  activeGroupId: string | null
  activePaneName: string | null
}

const isSessionName = (value: unknown): value is string =>
  typeof value === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(value)

export const createTerminalLayout = (sessionName: string): TerminalLayoutState => ({
  version: 1,
  groups: [{
    id: `group-${sessionName}`,
    paneNames: [sessionName],
    direction: 'horizontal',
  }],
  activeGroupId: `group-${sessionName}`,
  activePaneName: sessionName,
})

export const readTerminalLayout = (
  stored: string | null,
  fallbackSessionName: string,
): TerminalLayoutState => {
  if (!stored) return createTerminalLayout(fallbackSessionName)

  if (isSessionName(stored)) return createTerminalLayout(stored)

  try {
    const parsed = JSON.parse(stored) as Partial<TerminalLayoutState>
    if (parsed.version !== 1 || !Array.isArray(parsed.groups)) {
      return createTerminalLayout(fallbackSessionName)
    }
    const groups = parsed.groups.filter((group): group is TerminalGroup => {
      return !!group
        && typeof group.id === 'string'
        && group.id.startsWith('group-')
        && isSessionName(group.id.slice('group-'.length))
        && Array.isArray(group.paneNames)
        && group.paneNames.length > 0
        && group.paneNames.every(isSessionName)
        && (group.direction === 'horizontal' || group.direction === 'vertical')
    })
    if (groups.length === 0) {
      return createTerminalLayout(fallbackSessionName)
    }
    const activeGroup = groups.find((group) => group.id === parsed.activeGroupId) ?? groups[0]
    const activePaneName = activeGroup.paneNames.includes(parsed.activePaneName ?? '')
      ? parsed.activePaneName as string
      : activeGroup.paneNames[0]
    return {
      version: 1,
      groups,
      activeGroupId: activeGroup.id,
      activePaneName,
    }
  } catch {
    return createTerminalLayout(fallbackSessionName)
  }
}

export const addTerminalGroup = (
  state: TerminalLayoutState,
  sessionName: string,
): TerminalLayoutState => {
  const group: TerminalGroup = {
    id: `group-${sessionName}`,
    paneNames: [sessionName],
    direction: 'horizontal',
  }
  return {
    ...state,
    groups: [...state.groups, group],
    activeGroupId: group.id,
    activePaneName: sessionName,
  }
}

export const splitActiveTerminal = (
  state: TerminalLayoutState,
  sessionName: string,
  direction: TerminalSplitDirection,
): TerminalLayoutState => {
  if (!state.activeGroupId) return addTerminalGroup(state, sessionName)
  return {
    ...state,
    groups: state.groups.map((group) => group.id === state.activeGroupId
      ? { ...group, paneNames: [...group.paneNames, sessionName], direction }
      : group),
    activePaneName: sessionName,
  }
}

export const removeTerminalPane = (
  state: TerminalLayoutState,
  sessionName: string,
): TerminalLayoutState => {
  const groupIndex = state.groups.findIndex((group) => group.paneNames.includes(sessionName))
  if (groupIndex === -1) return state

  const group = state.groups[groupIndex]
  const remainingPanes = group.paneNames.filter((name) => name !== sessionName)
  if (remainingPanes.length > 0) {
    return {
      ...state,
      groups: state.groups.map((candidate) => candidate.id === group.id
        ? { ...candidate, paneNames: remainingPanes }
        : candidate),
      activeGroupId: group.id,
      activePaneName: remainingPanes[0],
    }
  }

  const groups = state.groups.filter((candidate) => candidate.id !== group.id)
  const nextGroup = groups[Math.min(groupIndex, groups.length - 1)]
  return {
    ...state,
    groups,
    activeGroupId: nextGroup?.id ?? null,
    activePaneName: nextGroup?.paneNames[0] ?? null,
  }
}
