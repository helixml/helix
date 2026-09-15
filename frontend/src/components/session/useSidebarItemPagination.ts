import { useState } from 'react'

import type { SidebarItem } from './ProjectChatSidebar.logic'

const SHOW_MORE_COUNT = 20

export type SidebarItemPagination = {
  /** How many items to render; the server page is one larger to detect more. */
  visibleCount: number
  requestCount: number
  canShowLess: boolean
  showMore: () => void
  showLess: () => void
}

// Every sidebar group pages the same way: a preference-sized window over its
// filtered items, one extra fetched so "Show more" knows whether to appear.
// The window is state the queries need before their items exist, so this hook
// owns only the window; windowSidebarItems applies it to the loaded items.
export const useSidebarItemPagination = (visibleThreadCount: number): SidebarItemPagination => {
  const [additionalVisibleCount, setAdditionalVisibleCount] = useState(0)
  const visibleCount = visibleThreadCount + additionalVisibleCount
  return {
    visibleCount,
    requestCount: visibleCount + 1,
    canShowLess: additionalVisibleCount > 0,
    showMore: () => setAdditionalVisibleCount((count) => count + SHOW_MORE_COUNT),
    showLess: () => setAdditionalVisibleCount(0),
  }
}

// The active item is always rendered, even when it falls outside the window,
// so the row you are looking at never disappears from the list.
export const windowSidebarItems = (
  filteredItems: SidebarItem[],
  activeItemId: string,
  visibleCount: number,
): SidebarItem[] => {
  const previewItems = filteredItems.slice(0, visibleCount)
  const activeHiddenItem = filteredItems.slice(visibleCount).find((item) => item.id === activeItemId)
  return activeHiddenItem ? [...previewItems, activeHiddenItem] : previewItems
}
