// In-memory store for kanban board scroll positions, scoped per projectId.
// Lives at module scope so it survives SpecTaskKanbanBoard unmount when the
// user navigates to a task detail page and returns.

export type KanbanScrollState = {
  horizontal: number
  columns: Record<string, number>
}

const store = new Map<string, KanbanScrollState>()

const ensure = (projectId: string): KanbanScrollState => {
  let state = store.get(projectId)
  if (!state) {
    state = { horizontal: 0, columns: {} }
    store.set(projectId, state)
  }
  return state
}

export const getKanbanScrollState = (
  projectId: string,
): KanbanScrollState | undefined => store.get(projectId)

export const saveKanbanHorizontalScroll = (
  projectId: string,
  scrollLeft: number,
): void => {
  ensure(projectId).horizontal = scrollLeft
}

export const saveKanbanColumnScroll = (
  projectId: string,
  columnId: string,
  scrollTop: number,
): void => {
  ensure(projectId).columns[columnId] = scrollTop
}

export const clearKanbanScrollState = (projectId: string): void => {
  store.delete(projectId)
}
