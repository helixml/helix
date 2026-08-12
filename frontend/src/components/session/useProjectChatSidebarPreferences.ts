import { useCallback, useEffect, useMemo, useState } from 'react'

import type { TypesProject } from '../../api/api'
import {
  clampVisibleThreadCount,
  parseSidebarPreferences,
  serializeSidebarPreferences,
  sortSidebarProjects,
} from './ProjectChatSidebar.logic'
import type {
  ProjectChatSidebarPreferences,
  SidebarProjectSortOrder,
  SidebarThreadSortOrder,
} from './ProjectChatSidebar.logic'

const readPreferences = (storageKey: string): ProjectChatSidebarPreferences => {
  try {
    return parseSidebarPreferences(window.localStorage.getItem(storageKey))
  } catch {
    return parseSidebarPreferences(null)
  }
}

const useProjectChatSidebarPreferences = (storageKey: string, projects: TypesProject[]) => {
  const [preferences, setPreferences] = useState<ProjectChatSidebarPreferences>(() => (
    readPreferences(storageKey)
  ))

  useEffect(() => {
    setPreferences(readPreferences(storageKey))
  }, [storageKey])

  const updatePreferences = useCallback((
    update: (current: ProjectChatSidebarPreferences) => ProjectChatSidebarPreferences,
  ) => {
    setPreferences((current) => {
      const next = update(current)
      try {
        window.localStorage.setItem(storageKey, serializeSidebarPreferences(next))
      } catch {
        // Persistence is optional when browser storage is unavailable.
      }
      return next
    })
  }, [storageKey])

  const sortedProjects = useMemo(
    () => sortSidebarProjects(projects, preferences),
    [preferences, projects],
  )

  const setProjectSortOrder = useCallback((projectSortOrder: SidebarProjectSortOrder) => {
    updatePreferences((current) => ({
      ...current,
      projectSortOrder,
      manualProjectOrder: projectSortOrder === 'manual' && current.manualProjectOrder.length === 0
        ? projects.flatMap((project) => project.id ? [project.id] : [])
        : current.manualProjectOrder,
    }))
  }, [projects, updatePreferences])

  const setThreadSortOrder = useCallback((threadSortOrder: SidebarThreadSortOrder) => {
    updatePreferences((current) => ({ ...current, threadSortOrder }))
  }, [updatePreferences])

  const setVisibleThreadCount = useCallback((visibleThreadCount: number) => {
    updatePreferences((current) => ({
      ...current,
      visibleThreadCount: clampVisibleThreadCount(visibleThreadCount),
    }))
  }, [updatePreferences])

  const setManualProjectOrder = useCallback((manualProjectOrder: string[]) => {
    updatePreferences((current) => ({ ...current, manualProjectOrder }))
  }, [updatePreferences])

  return {
    preferences,
    sortedProjects,
    setProjectSortOrder,
    setThreadSortOrder,
    setVisibleThreadCount,
    setManualProjectOrder,
  }
}

export default useProjectChatSidebarPreferences
