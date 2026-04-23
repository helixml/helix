import { useRoute } from 'react-router5'
import { loadNavHistory, NavHistoryEntry } from '../lib/navHistory'

export type { NavHistoryEntry }

/**
 * Returns the current navigation history from localStorage.
 * Re-renders the caller on every route change so the list stays fresh.
 * Recording is handled globally in router.tsx's subscriber.
 */
export function useNavigationHistory(): NavHistoryEntry[] {
  useRoute() // causes re-render on every route change
  return loadNavHistory()
}
