export type TerminalSplitDirection = 'horizontal' | 'vertical'

export type TerminalLayoutNode =
  | {
      type: 'pane'
      sessionName: string
    }
  | {
      type: 'split'
      direction: TerminalSplitDirection
      children: TerminalLayoutNode[]
    }

export interface TerminalGroup {
  id: string
  root: TerminalLayoutNode
}

export interface TerminalLayoutState {
  version: 2
  groups: TerminalGroup[]
  activeGroupId: string | null
  activePaneName: string | null
}

export const MAX_TERMINAL_PANES_PER_GROUP = 4

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null

const isSessionName = (value: unknown): value is string =>
  typeof value === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(value)

const isSplitDirection = (value: unknown): value is TerminalSplitDirection =>
  value === 'horizontal' || value === 'vertical'

const createPane = (sessionName: string): TerminalLayoutNode => ({
  type: 'pane',
  sessionName,
})

export const getTerminalPaneNames = (node: TerminalLayoutNode): string[] => {
  if (node.type === 'pane') return [node.sessionName]
  return node.children.flatMap(getTerminalPaneNames)
}

export const getFirstTerminalPaneName = (
  node: TerminalLayoutNode,
): string => node.type === 'pane'
  ? node.sessionName
  : getFirstTerminalPaneName(node.children[0])

export const createTerminalLayout = (sessionName: string): TerminalLayoutState => ({
  version: 2,
  groups: [{
    id: `group-${sessionName}`,
    root: createPane(sessionName),
  }],
  activeGroupId: `group-${sessionName}`,
  activePaneName: sessionName,
})

const parseTerminalNode = (
  value: unknown,
  sessionNames: Set<string>,
  depth = 0,
): TerminalLayoutNode | null => {
  if (!isRecord(value) || depth >= MAX_TERMINAL_PANES_PER_GROUP) return null

  if (value.type === 'pane') {
    if (!isSessionName(value.sessionName) || sessionNames.has(value.sessionName)) {
      return null
    }
    sessionNames.add(value.sessionName)
    return createPane(value.sessionName)
  }

  if (
    value.type !== 'split'
    || !isSplitDirection(value.direction)
    || !Array.isArray(value.children)
    || value.children.length < 2
    || value.children.length > MAX_TERMINAL_PANES_PER_GROUP
  ) {
    return null
  }

  const children: TerminalLayoutNode[] = []
  for (const child of value.children) {
    const parsed = parseTerminalNode(child, sessionNames, depth + 1)
    if (!parsed || sessionNames.size > MAX_TERMINAL_PANES_PER_GROUP) return null
    children.push(parsed)
  }
  return { type: 'split', direction: value.direction, children }
}

const parseGroupID = (value: unknown): string | null => {
  if (typeof value !== 'string' || !value.startsWith('group-')) return null
  return isSessionName(value.slice('group-'.length)) ? value : null
}

const parseCurrentGroups = (value: unknown): TerminalGroup[] => {
  if (!Array.isArray(value)) return []
  return value.flatMap((candidate): TerminalGroup[] => {
    if (!isRecord(candidate)) return []
    const id = parseGroupID(candidate.id)
    const root = parseTerminalNode(candidate.root, new Set<string>())
    return id && root ? [{ id, root }] : []
  })
}

const migrateLegacyGroups = (value: unknown): TerminalGroup[] => {
  if (!Array.isArray(value)) return []
  return value.flatMap((candidate): TerminalGroup[] => {
    if (!isRecord(candidate)) return []
    const id = parseGroupID(candidate.id)
    const paneNames = Array.isArray(candidate.paneNames)
      && candidate.paneNames.every(isSessionName)
      ? candidate.paneNames
      : []
    if (
      !id
      || paneNames.length === 0
      || paneNames.length > MAX_TERMINAL_PANES_PER_GROUP
      || new Set(paneNames).size !== paneNames.length
      || !isSplitDirection(candidate.direction)
    ) {
      return []
    }
    const panes = paneNames.map(createPane)
    return [{
      id,
      root: panes.length === 1
        ? panes[0]
        : { type: 'split', direction: candidate.direction, children: panes },
    }]
  })
}

const finalizeParsedLayout = (
  parsed: Record<string, unknown>,
  groups: TerminalGroup[],
  fallbackSessionName: string,
): TerminalLayoutState => {
  if (groups.length === 0) return createTerminalLayout(fallbackSessionName)

  const activeGroup = groups.find((group) => group.id === parsed.activeGroupId) ?? groups[0]
  const paneNames = getTerminalPaneNames(activeGroup.root)
  const activePaneName = paneNames.includes(String(parsed.activePaneName ?? ''))
    ? String(parsed.activePaneName)
    : paneNames[0]
  return {
    version: 2,
    groups,
    activeGroupId: activeGroup.id,
    activePaneName,
  }
}

export const readTerminalLayout = (
  stored: string | null,
  fallbackSessionName: string,
): TerminalLayoutState => {
  if (!stored) return createTerminalLayout(fallbackSessionName)
  if (isSessionName(stored)) return createTerminalLayout(stored)

  try {
    const parsed: unknown = JSON.parse(stored)
    if (!isRecord(parsed)) return createTerminalLayout(fallbackSessionName)
    if (parsed.version === 2) {
      return finalizeParsedLayout(
        parsed,
        parseCurrentGroups(parsed.groups),
        fallbackSessionName,
      )
    }
    if (parsed.version === 1) {
      return finalizeParsedLayout(
        parsed,
        migrateLegacyGroups(parsed.groups),
        fallbackSessionName,
      )
    }
    return createTerminalLayout(fallbackSessionName)
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
    root: createPane(sessionName),
  }
  return {
    ...state,
    groups: [...state.groups, group],
    activeGroupId: group.id,
    activePaneName: sessionName,
  }
}

const splitPane = (
  node: TerminalLayoutNode,
  targetSessionName: string,
  newSessionName: string,
  direction: TerminalSplitDirection,
): TerminalLayoutNode => {
  if (node.type === 'pane') {
    if (node.sessionName !== targetSessionName) return node
    return {
      type: 'split',
      direction,
      children: [node, createPane(newSessionName)],
    }
  }
  return {
    ...node,
    children: node.children.map((child) =>
      splitPane(child, targetSessionName, newSessionName, direction)),
  }
}

export const splitActiveTerminal = (
  state: TerminalLayoutState,
  sessionName: string,
  direction: TerminalSplitDirection,
): TerminalLayoutState => {
  const activeGroup = state.groups.find((group) => group.id === state.activeGroupId)
  const activePaneName = state.activePaneName
  if (!activeGroup || !activePaneName) return addTerminalGroup(state, sessionName)
  if (
    getTerminalPaneNames(activeGroup.root).length
    >= MAX_TERMINAL_PANES_PER_GROUP
  ) {
    return state
  }

  return {
    ...state,
    groups: state.groups.map((group) => group.id === activeGroup.id
      ? {
          ...group,
          root: splitPane(group.root, activePaneName, sessionName, direction),
        }
      : group),
    activePaneName: sessionName,
  }
}

const removePane = (
  node: TerminalLayoutNode,
  sessionName: string,
): TerminalLayoutNode | null => {
  if (node.type === 'pane') {
    return node.sessionName === sessionName ? null : node
  }

  const children = node.children.flatMap((child): TerminalLayoutNode[] => {
    const remaining = removePane(child, sessionName)
    return remaining ? [remaining] : []
  })
  if (children.length === 0) return null
  if (children.length === 1) return children[0]
  return { ...node, children }
}

export const removeTerminalPane = (
  state: TerminalLayoutState,
  sessionName: string,
): TerminalLayoutState => {
  const groupIndex = state.groups.findIndex((group) =>
    getTerminalPaneNames(group.root).includes(sessionName))
  if (groupIndex === -1) return state

  const group = state.groups[groupIndex]
  const root = removePane(group.root, sessionName)
  if (root) {
    const paneNames = getTerminalPaneNames(root)
    return {
      ...state,
      groups: state.groups.map((candidate) => candidate.id === group.id
        ? { ...candidate, root }
        : candidate),
      activeGroupId: group.id,
      activePaneName: state.activePaneName
        && paneNames.includes(state.activePaneName)
        ? state.activePaneName
        : getFirstTerminalPaneName(root),
    }
  }

  const groups = state.groups.filter((candidate) => candidate.id !== group.id)
  const nextGroup = groups[Math.min(groupIndex, groups.length - 1)]
  return {
    ...state,
    groups,
    activeGroupId: nextGroup?.id ?? null,
    activePaneName: nextGroup
      ? getFirstTerminalPaneName(nextGroup.root)
      : null,
  }
}
