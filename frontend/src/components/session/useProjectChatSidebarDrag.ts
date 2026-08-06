import { useCallback, useRef } from 'react'
import {
  DragCancelEvent,
  DragEndEvent,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'

import type { TypesProject } from '../../api/api'
import { reorderProjectIds } from './ProjectChatSidebar.logic'
import type { SidebarProjectSortOrder } from './ProjectChatSidebar.logic'

const useProjectChatSidebarDrag = (
  projectSortOrder: SidebarProjectSortOrder,
  query: string,
  sortedProjects: TypesProject[],
  setManualProjectOrder: (order: string[]) => void,
) => {
  const dragInProgressRef = useRef(false)
  const suppressClickAfterDragRef = useRef(false)
  const sensors = useSensors(useSensor(PointerSensor, {
    activationConstraint: { distance: 6 },
  }))

  const onDragStart = useCallback((_event: DragStartEvent) => {
    if (projectSortOrder !== 'manual' || query) return
    dragInProgressRef.current = true
    suppressClickAfterDragRef.current = true
  }, [projectSortOrder, query])

  const onDragCancel = useCallback((_event: DragCancelEvent) => {
    dragInProgressRef.current = false
  }, [])

  const onDragEnd = useCallback((event: DragEndEvent) => {
    dragInProgressRef.current = false
    if (projectSortOrder !== 'manual' || query || !event.over) return
    const activeId = String(event.active.id)
    const overId = String(event.over.id)
    if (activeId === overId) return

    const currentOrder = sortedProjects.flatMap((project) => project.id ? [project.id] : [])
    setManualProjectOrder(reorderProjectIds(currentOrder, activeId, overId))
  }, [projectSortOrder, query, setManualProjectOrder, sortedProjects])

  return {
    dragInProgressRef,
    suppressClickAfterDragRef,
    sensors,
    onDragStart,
    onDragCancel,
    onDragEnd,
  }
}

export default useProjectChatSidebarDrag
