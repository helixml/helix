import type { TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import { matchesAllTokens } from '../../utils/searchUtils'

export type SidebarStatus = {
  label: string
  color: string
}

export type SidebarItem = {
  id: string
  kind: 'session' | 'spec-task'
  title: string
  updatedAt?: string
  projectId?: string
  session?: TypesSessionSummary
  task?: SpecTask
}

export type SidebarGroup = {
  id: string
  name: string
  items: SidebarItem[]
}

export const getSidebarTaskStatus = (task?: SpecTask): SidebarStatus | null => {
  switch (task?.status) {
    case 'queued_spec_generation':
    case 'queued_implementation':
    case 'implementation_queued':
      return { label: 'Queued', color: '#60a5fa' }
    case 'spec_generation':
    case 'spec_revision':
      return { label: 'Planning', color: '#38bdf8' }
    case 'spec_review':
      return { label: 'Plan review', color: '#fbbf24' }
    case 'spec_approved':
      return { label: 'Approved', color: '#34d399' }
    case 'implementation':
      return { label: 'Implementation', color: '#a78bfa' }
    case 'implementation_review':
      return { label: 'Review', color: '#fb923c' }
    case 'pull_request':
      return { label: 'Pull request', color: '#22d3ee' }
    case 'done':
      return { label: 'Completed', color: '#34d399' }
    case 'spec_failed':
    case 'implementation_failed':
      return { label: 'Failed', color: '#f87171' }
    case 'backlog':
      return { label: 'Backlog', color: '#94a3b8' }
    default:
      if (task?.agent_work_state === 'working') {
        return { label: 'Working', color: '#38bdf8' }
      }
      return null
  }
}

const timestamp = (value?: string): number => {
  if (!value) return 0
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

const itemTimestamp = (item: SidebarItem): number => timestamp(item.updatedAt)

export const compactRelativeTime = (value?: string, now = Date.now()): string => {
  const valueMs = timestamp(value)
  if (!valueMs) return ''
  const elapsedSeconds = Math.max(0, Math.floor((now - valueMs) / 1000))
  if (elapsedSeconds < 60) return 'now'
  const minutes = Math.floor(elapsedSeconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d`
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(valueMs)
}

export const dedupeSessions = (sessions: TypesSessionSummary[]): TypesSessionSummary[] => {
  const seen = new Set<string>()
  return sessions.filter((session) => {
    if (!session.session_id) return true
    if (seen.has(session.session_id)) return false
    seen.add(session.session_id)
    return true
  })
}

export const buildProjectChatGroups = (
  projects: TypesProject[],
  specTasks: SpecTask[],
  sessions: TypesSessionSummary[],
): SidebarGroup[] => {
  const defaultGroup: SidebarGroup = { id: 'default', name: 'Default', items: [] }
  const groupsByProjectId = new Map<string, SidebarGroup>()
  projects.forEach((project) => {
    if (!project.id) return
    groupsByProjectId.set(project.id, {
      id: project.id,
      name: project.name || 'Untitled project',
      items: [],
    })
  })

  const taskIds = new Set<string>()
  specTasks.forEach((task) => {
    if (!task.id || !task.project_id) return
    const group = groupsByProjectId.get(task.project_id)
    if (!group) return
    taskIds.add(task.id)
    group.items.push({
      id: task.id,
      kind: 'spec-task',
      title: task.user_short_title || task.short_title || task.name || 'Untitled task',
      updatedAt: task.session_updated_at || task.updated_at || task.status_updated_at || task.created_at,
      projectId: task.project_id,
      task,
    })
  })

  sessions.forEach((session) => {
    if (!session.session_id) return
    const metadata = session.metadata
    if (metadata?.spec_task_id && taskIds.has(metadata.spec_task_id)) return

    if (metadata?.spec_task_id && metadata.project_id && groupsByProjectId.has(metadata.project_id)) {
      groupsByProjectId.get(metadata.project_id)?.items.push({
        id: metadata.spec_task_id,
        kind: 'spec-task',
        title: session.name || 'Untitled task',
        updatedAt: session.updated || session.created,
        projectId: metadata.project_id,
      })
      taskIds.add(metadata.spec_task_id)
      return
    }

    const projectGroup = !metadata?.org_worker_id && metadata?.project_id
      ? groupsByProjectId.get(metadata.project_id)
      : undefined
    const group = projectGroup || defaultGroup
    group.items.push({
      id: session.session_id,
      kind: 'session',
      title: session.name || session.summary || 'Untitled chat',
      updatedAt: session.updated || session.created,
      projectId: projectGroup?.id,
      session,
    })
  })

  const projectGroups = [...groupsByProjectId.values()]
    .filter((group) => group.items.length > 0)
    .sort((left, right) => {
      const rightActivity = Math.max(...right.items.map(itemTimestamp), 0)
      const leftActivity = Math.max(...left.items.map(itemTimestamp), 0)
      return rightActivity - leftActivity || left.name.localeCompare(right.name)
    })

  return [defaultGroup, ...projectGroups]
    .filter((group) => group.items.length > 0)
    .map((group) => ({
      ...group,
      items: [...group.items].sort((left, right) => itemTimestamp(right) - itemTimestamp(left)),
    }))
}

export const filterProjectChatGroups = (groups: SidebarGroup[], query: string): SidebarGroup[] => (
  groups.flatMap((group) => {
    if (!query.trim()) return [group]
    if (matchesAllTokens(query, group.name)) return [group]
    const items = group.items.filter((item) => {
      const status = item.kind === 'spec-task' ? getSidebarTaskStatus(item.task)?.label : undefined
      return matchesAllTokens(query, item.title, group.name, status)
    })
    return items.length > 0 ? [{ ...group, items }] : []
  })
)
