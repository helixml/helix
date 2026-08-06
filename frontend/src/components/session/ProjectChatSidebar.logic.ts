import type { TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import { matchesAllTokens } from '../../utils/searchUtils'

export type SidebarStatus = {
  label: string
  color: string
  tooltip?: string
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

export const isTaskCompletedOrMerged = (task?: SpecTask): boolean => (
  task?.status === 'done' || task?.merged_to_main === true
)

// The archive endpoint stops task agents. Skip the confirmation only when the
// list has positively established that this task is terminal and its sandbox
// is already absent. Unknown state must remain confirm-first.
export const shouldConfirmTaskArchive = (item: SidebarItem): boolean => (
  item.kind !== 'spec-task'
  || !item.task
  || item.task.sandbox_state !== 'absent'
  || !isTaskCompletedOrMerged(item.task)
)

export const collapsedGroupsStorageKey = (orgId: string): string => (
  `helix:project-chat-sidebar:collapsed:${orgId}`
)

export const parseCollapsedGroupIds = (storedValue: string | null): Set<string> => {
  if (!storedValue) return new Set()
  try {
    const value = JSON.parse(storedValue)
    if (!Array.isArray(value)) return new Set()
    return new Set(value.filter((id): id is string => typeof id === 'string'))
  } catch {
    return new Set()
  }
}

export const serializeCollapsedGroupIds = (groupIds: Set<string>): string => (
  JSON.stringify([...groupIds].sort())
)

export const isNewThreadShortcut = (
  event: Pick<KeyboardEvent, 'altKey' | 'ctrlKey' | 'key' | 'metaKey' | 'shiftKey'>,
): boolean => (
  (event.metaKey || event.ctrlKey)
  && !event.altKey
  && !event.shiftKey
  && event.key.toLowerCase() === 'n'
)

const getSidebarWorkflowStatus = (task?: SpecTask): SidebarStatus | null => {
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
      return { label: 'Implementation', color: '#34d399' }
    case 'implementation_review':
      return { label: 'Review', color: '#fb923c' }
    case 'pull_request':
      return { label: 'Pull request', color: '#22d3ee' }
    case 'done':
      return { label: 'Completed', color: '#a78bfa' }
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

export const getSidebarTaskStatus = (task?: SpecTask): SidebarStatus | null => {
  const workflowStatus = getSidebarWorkflowStatus(task)

  if (task?.sandbox_state === 'absent') {
    return {
      ...(workflowStatus || {}),
      label: workflowStatus?.label || 'Offline',
      color: '#a1a1aa',
      tooltip: 'Sandbox and agent are offline',
    }
  }

  if (
    task?.sandbox_state === 'running'
    && (task.agent_work_state === 'idle' || task.agent_work_state === 'done')
  ) {
    return { label: 'Idle', color: '#fbbf24' }
  }

  return workflowStatus
}

const PULL_REQUEST_ICON_COLORS: Record<string, string> = {
  open: '#10b981',
  closed: '#ef4444',
  merged: '#8b5cf6',
}

export type SidebarPullRequestIcon = {
  color: string
  tooltip: string
  url?: string
}

const normalizePullRequestState = (state?: string): 'open' | 'closed' | 'merged' => {
  const normalized = state?.toLowerCase()
  return normalized === 'closed' || normalized === 'merged' ? normalized : 'open'
}

export const getSidebarPullRequestIcon = (task?: SpecTask): SidebarPullRequestIcon => {
  const pullRequests = task?.repo_pull_requests || []
  if (pullRequests.length === 0) {
    return task?.merged_to_main
      ? { color: PULL_REQUEST_ICON_COLORS.merged, tooltip: 'Pull request is merged' }
      : { color: '#a1a1aa', tooltip: 'No pull request yet' }
  }

  if (task?.merged_to_main) {
    const pullRequest = pullRequests.find((candidate) => candidate.pr_state?.toLowerCase() === 'merged')
      || pullRequests[0]
    return {
      color: PULL_REQUEST_ICON_COLORS.merged,
      tooltip: 'Pull request is merged',
      url: pullRequest.pr_url,
    }
  }

  const pullRequest = pullRequests.find((candidate) => normalizePullRequestState(candidate.pr_state) === 'open')
    || pullRequests.find((candidate) => normalizePullRequestState(candidate.pr_state) === 'closed')
    || pullRequests[0]
  const state = normalizePullRequestState(pullRequest.pr_state)

  return {
    color: PULL_REQUEST_ICON_COLORS[state],
    tooltip: `Pull request is ${state}`,
    url: pullRequest.pr_url,
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
  const defaultGroup: SidebarGroup = { id: 'default', name: 'None', items: [] }
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
