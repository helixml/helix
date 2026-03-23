/**
 * Client-side navigation history stored in localStorage.
 * No imports from router.tsx — keeps the dependency graph acyclic.
 */

const STORAGE_KEY = 'helix_nav_history'
const MAX_ENTRIES = 30

export interface NavHistoryEntry {
  url: string
  routeName: string
  params: Record<string, string>
  title: string
  timestamp: number
}

function deriveTitle(routeName: string, params: Record<string, string>): string {
  const shortId = (id: string | undefined) => id ? id.slice(-6) : ''
  switch (routeName) {
    case 'org_project-task-detail':
      return `Task · ${shortId(params.taskId)}`
    case 'org_project-task-review':
      return `Review · ${shortId(params.taskId)}`
    case 'org_project-specs':
      return 'Project board'
    case 'org_spec-tasks':
      return 'All tasks'
    case 'org_project-design-doc':
      return `Design doc · ${shortId(params.taskId || params.id)}`
    default:
      return routeName.replace(/^org_/, '').replace(/-/g, ' ')
  }
}

export function loadNavHistory(): NavHistoryEntry[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

export function recordNavRoute(routeName: string, params: Record<string, string>): void {
  try {
    const url = window.location.pathname + window.location.search
    const title = deriveTitle(routeName, params)
    const entry: NavHistoryEntry = { url, routeName, params, title, timestamp: Date.now() }
    const deduped = loadNavHistory().filter(e => e.url !== entry.url)
    const updated = [entry, ...deduped].slice(0, MAX_ENTRIES)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(updated))
  } catch {
    // ignore storage errors
  }
}
