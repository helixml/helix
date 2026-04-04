import { useEffect } from 'react'

const MAX_TITLE_LENGTH = 60
const SUFFIX = ' - Helix'

/**
 * Sets the browser tab title based on breadcrumb titles.
 * Format: "Last Crumb - Middle - First - Helix"
 */
export default function useDocumentTitle(titles: string[]) {
  useEffect(() => {
    if (titles.length === 0) {
      document.title = 'Helix'
      return
    }

    // Reverse order: most specific first (task > project > etc)
    const reversed = [...titles].reverse()
    let combined = reversed.join(' - ') + SUFFIX

    // Truncate if too long
    if (combined.length > MAX_TITLE_LENGTH) {
      // Keep first item (most specific) and last item (Helix suffix is auto-added)
      const first = reversed[0]
      const truncatedFirst = first.length > 40 ? first.slice(0, 37) + '...' : first
      combined = truncatedFirst + SUFFIX
    }

    document.title = combined
  }, [titles.join('|')]) // stable dependency
}
