/**
 * Client-side navigation history stored in localStorage.
 * No imports from router.tsx — keeps the dependency graph acyclic.
 */

const STORAGE_KEY = 'helix_nav_history'
const NAMES_KEY = 'helix_task_name_cache'
const MAX_ENTRIES = 30

export interface NavHistoryEntry {
  url: string
  routeName: string
  params: Record<string, string>
  title: string
  timestamp: number
}

// ---------- Task name cache ----------

function loadNameCache(): Record<string, string> {
  try {
    const raw = localStorage.getItem(NAMES_KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

/** Call this from any page that knows a task's name (e.g. SpecTaskDetailPage). */
export function cacheTaskName(taskId: string, name: string): void {
  try {
    const cache = loadNameCache()
    if (cache[taskId] === name) return // no change, skip write
    cache[taskId] = name
    localStorage.setItem(NAMES_KEY, JSON.stringify(cache))
  } catch {
    // ignore storage errors
  }
}

// ---------- Title derivation ----------

function deriveTitle(routeName: string, params: Record<string, string>, nameCache: Record<string, string>): string {
  const taskName = params.taskId ? nameCache[params.taskId] : undefined
  const truncate = (s: string, max = 40) => s.length > max ? s.slice(0, max).trimEnd() + '…' : s

  switch (routeName) {
    case 'org_project-task-detail':
      return taskName ? truncate(taskName) : 'Task'
    case 'org_project-task-review':
      return taskName ? `Review: ${truncate(taskName, 34)}` : 'Spec review'
    case 'org_project-specs':
      return 'Project board'
    case 'org_spec-tasks':
      return 'All tasks'
    case 'org_project-design-doc':
      return taskName ? `Design doc: ${truncate(taskName, 30)}` : 'Design doc'
    default:
      return routeName.replace(/^org_/, '').replace(/-/g, ' ')
  }
}

// ---------- History ----------

export function loadNavHistory(): NavHistoryEntry[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    const entries: NavHistoryEntry[] = raw ? JSON.parse(raw) : []
    // Enrich titles with any cached names (retroactively improves old entries)
    const nameCache = loadNameCache()
    return entries.map(e => ({
      ...e,
      title: deriveTitle(e.routeName, e.params, nameCache),
    }))
  } catch {
    return []
  }
}

export function recordNavRoute(routeName: string, params: Record<string, string>): void {
  try {
    const url = window.location.pathname + window.location.search
    const nameCache = loadNameCache()
    const title = deriveTitle(routeName, params, nameCache)
    const entry: NavHistoryEntry = { url, routeName, params, title, timestamp: Date.now() }
    const raw = localStorage.getItem(STORAGE_KEY)
    const existing: NavHistoryEntry[] = raw ? JSON.parse(raw) : []
    const deduped = existing.filter(e => e.url !== entry.url)
    localStorage.setItem(STORAGE_KEY, JSON.stringify([entry, ...deduped].slice(0, MAX_ENTRIES)))
  } catch {
    // ignore storage errors
  }
}
