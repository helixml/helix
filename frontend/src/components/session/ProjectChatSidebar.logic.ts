import type { TypesOrganizationMembership, TypesProject, TypesSessionSummary } from '../../api/api'
import type { BotDTO } from '../../services/helixOrgService'
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
  createdAt?: string
  updatedAt?: string
  projectId?: string
  /** Set on cross-project lists (a person's work) so the row can say where it lives. */
  projectName?: string
  session?: TypesSessionSummary
  task?: SpecTask
  pinnedAt?: string
}

export type SidebarGroup = {
  id: string
  name: string
  items: SidebarItem[]
}

export type SidebarProjectSortOrder = 'updated_at' | 'created_at' | 'manual'
export type SidebarThreadSortOrder = 'updated_at' | 'created_at'

export type ProjectChatSidebarPreferences = {
  projectSortOrder: SidebarProjectSortOrder
  threadSortOrder: SidebarThreadSortOrder
  visibleThreadCount: number
  manualProjectOrder: string[]
}

export const MIN_VISIBLE_THREAD_COUNT = 1
export const MAX_VISIBLE_THREAD_COUNT = 15
export const DEFAULT_VISIBLE_THREAD_COUNT = 6

export const DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES: ProjectChatSidebarPreferences = {
  projectSortOrder: 'updated_at',
  threadSortOrder: 'updated_at',
  visibleThreadCount: DEFAULT_VISIBLE_THREAD_COUNT,
  manualProjectOrder: [],
}

export const sidebarPreferencesStorageKey = (orgId: string): string => (
  `helix:project-chat-sidebar:preferences:${orgId}`
)

export const sidebarPeopleFilterStorageKey = (
  userId: string,
  orgId: string,
  projectId: string,
): string => (
  `helix:project-chat-sidebar:people:${userId}:${orgId}:${projectId}`
)

export const ALL_PROJECTS_FILTER = 'all-projects'

export const sidebarProjectFilterStorageKey = (orgId: string): string => (
  `helix:project-chat-sidebar:project-filter:${orgId}`
)

export const parseSidebarProjectFilter = (storedValue: string | null): string => (
  storedValue?.trim() || ALL_PROJECTS_FILTER
)

export const resolveSidebarProjectFilter = (
  projectId: string,
  projects: TypesProject[],
): string => (
  projectId === ALL_PROJECTS_FILTER || projects.some((project) => project.id === projectId)
    ? projectId
    : ALL_PROJECTS_FILTER
)

export const parseSidebarParticipantIds = (storedValue: string | null): string[] | null => {
  if (storedValue === null) return null
  try {
    const value = JSON.parse(storedValue)
    if (!Array.isArray(value)) return null
    return [...new Set(value.filter((id): id is string => typeof id === 'string' && !!id))]
  } catch {
    return null
  }
}

export const serializeSidebarParticipantIds = (userIds: string[]): string => (
  JSON.stringify([...new Set(userIds.filter(Boolean))])
)

export const filterSidebarMembers = (
  members: TypesOrganizationMembership[],
  query: string,
): TypesOrganizationMembership[] => members.filter((member) => {
  if (!member.user_id || !member.user) return false
  return matchesAllTokens(query,
    member.user.full_name,
    member.user.username,
    member.user.email,
  )
})

export const getSidebarMemberResults = (
  members: TypesOrganizationMembership[],
  query: string,
  currentUserId: string,
  selectedUserIds: string[],
  limit = 10,
): { members: TypesOrganizationMembership[]; total: number } => {
  const selectedUserIdSet = new Set(selectedUserIds)
  const orderedMembers = [...members].sort((left, right) => {
    if (left.user_id === currentUserId) return -1
    if (right.user_id === currentUserId) return 1
    const leftSelected = !!left.user_id && selectedUserIdSet.has(left.user_id)
    const rightSelected = !!right.user_id && selectedUserIdSet.has(right.user_id)
    if (leftSelected !== rightSelected) return leftSelected ? -1 : 1
    return 0
  })
  const filteredMembers = filterSidebarMembers(orderedMembers, query)
  return { members: filteredMembers.slice(0, limit), total: filteredMembers.length }
}

const isProjectSortOrder = (value: unknown): value is SidebarProjectSortOrder => (
  value === 'updated_at' || value === 'created_at' || value === 'manual'
)

const isThreadSortOrder = (value: unknown): value is SidebarThreadSortOrder => (
  value === 'updated_at' || value === 'created_at'
)

export const clampVisibleThreadCount = (value: number): number => (
  Math.min(MAX_VISIBLE_THREAD_COUNT, Math.max(MIN_VISIBLE_THREAD_COUNT, Math.round(value)))
)

export const parseSidebarPreferences = (
  storedValue: string | null,
): ProjectChatSidebarPreferences => {
  if (!storedValue) return DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES

  try {
    const value = JSON.parse(storedValue) as Partial<ProjectChatSidebarPreferences>
    const manualProjectOrder = Array.isArray(value.manualProjectOrder)
      ? [...new Set(value.manualProjectOrder.filter((id): id is string => typeof id === 'string' && !!id))]
      : []
    return {
      projectSortOrder: isProjectSortOrder(value.projectSortOrder)
        ? value.projectSortOrder
        : DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES.projectSortOrder,
      threadSortOrder: isThreadSortOrder(value.threadSortOrder)
        ? value.threadSortOrder
        : DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES.threadSortOrder,
      visibleThreadCount: typeof value.visibleThreadCount === 'number' && Number.isFinite(value.visibleThreadCount)
        ? clampVisibleThreadCount(value.visibleThreadCount)
        : DEFAULT_VISIBLE_THREAD_COUNT,
      manualProjectOrder,
    }
  } catch {
    return DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES
  }
}

export const serializeSidebarPreferences = (
  preferences: ProjectChatSidebarPreferences,
): string => JSON.stringify(preferences)

const sortableTimestamp = (value?: string): number => {
  if (!value) return Number.NEGATIVE_INFINITY
  const timestamp = Date.parse(value)
  return Number.isNaN(timestamp) ? Number.NEGATIVE_INFINITY : timestamp
}

export const sortSidebarProjects = (
  projects: TypesProject[],
  preferences: ProjectChatSidebarPreferences,
): TypesProject[] => {
  if (preferences.projectSortOrder === 'manual') {
    const projectById = new Map(projects.flatMap((project) => project.id ? [[project.id, project]] : []))
    const ordered = preferences.manualProjectOrder.flatMap((id) => {
      const project = projectById.get(id)
      if (!project) return []
      projectById.delete(id)
      return [project]
    })
    return [...ordered, ...projects.filter((project) => !!project.id && projectById.has(project.id))]
  }

  return [...projects].sort((left, right) => {
    const leftTimestamp = preferences.projectSortOrder === 'created_at'
      ? sortableTimestamp(left.created_at)
      : sortableTimestamp(left.last_activity_at || left.updated_at || left.created_at)
    const rightTimestamp = preferences.projectSortOrder === 'created_at'
      ? sortableTimestamp(right.created_at)
      : sortableTimestamp(right.last_activity_at || right.updated_at || right.created_at)
    return rightTimestamp - leftTimestamp
      || (left.name || '').localeCompare(right.name || '')
      || (left.id || '').localeCompare(right.id || '')
  })
}

export const reorderProjectIds = (
  currentOrder: string[],
  activeId: string,
  overId: string,
): string[] => {
  const activeIndex = currentOrder.indexOf(activeId)
  const overIndex = currentOrder.indexOf(overId)
  if (activeIndex < 0 || overIndex < 0 || activeIndex === overIndex) return currentOrder

  const next = [...currentOrder]
  const [active] = next.splice(activeIndex, 1)
  next.splice(overIndex, 0, active)
  return next
}

export const isTaskCompletedOrMerged = (task?: SpecTask): boolean => (
  task?.status === 'done' || task?.merged_to_main === true
)

export const isOrgAgentSession = (
  item: SidebarItem,
  orgAgentAppIds: ReadonlySet<string>,
): boolean => (
  item.kind === 'session'
  && (
    !!item.session?.metadata?.org_worker_id
    || (!!item.session?.app_id && orgAgentAppIds.has(item.session.app_id))
  )
)

// Mirrors the server: archiveSession only calls StopDesktop when the session is
// an external-agent session (session_handlers.go). A plain model chat has no
// agent to stop, so it must not be warned about as if it did.
export const isExternalAgentSession = (item: SidebarItem): boolean => (
  item.kind === 'session' && item.session?.metadata?.agent_type === 'zed_external'
)

// Archiving is reversible (see the Archived view), so the confirmation exists
// only to warn about the irreversible side effect: stopping a running agent.
// Confirm when archiving would stop something —
//   - a spec task we cannot prove is terminal with an already-absent sandbox
//   - an external-agent chat that isn't a shared org-agent sandbox
// Everything else (plain chats, org-agent chats, finished tasks) archives
// straight away. Unarchiving never stops anything, so it never confirms.
export const shouldConfirmArchive = (
  item: SidebarItem,
  orgAgentAppIds: ReadonlySet<string> = new Set(),
  archived = false,
): boolean => {
  if (archived) return false
  if (isOrgAgentSession(item, orgAgentAppIds)) return false
  if (item.kind === 'session') return isExternalAgentSession(item)
  return !item.task
    || item.task.sandbox_state !== 'absent'
    || !isTaskCompletedOrMerged(item.task)
}

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

// Cmd/Ctrl+Shift+O rather than Cmd/Ctrl+N: browsers reserve N for "new window"
// and ignore preventDefault on it, so binding N would open a browser window AND
// navigate. Shift+O is unreserved and matches the convention other chat UIs use.
export const isNewThreadShortcut = (
  event: Pick<KeyboardEvent, 'altKey' | 'ctrlKey' | 'key' | 'metaKey' | 'shiftKey'>,
): boolean => (
  (event.metaKey || event.ctrlKey)
  && event.shiftKey
  && !event.altKey
  && event.key.toLowerCase() === 'o'
)

export const isChatShortcutModifier = (
  event: Pick<KeyboardEvent, 'ctrlKey' | 'metaKey'>,
  isMac: boolean,
): boolean => isMac ? event.metaKey : event.ctrlKey

export const getChatShortcutNumber = (
  event: Pick<KeyboardEvent, 'altKey' | 'code' | 'ctrlKey' | 'key' | 'metaKey' | 'shiftKey'>,
  isMac: boolean,
  modifierHeld = false,
): number | null => {
  if ((!modifierHeld && !isChatShortcutModifier(event, isMac)) || event.altKey || event.shiftKey) return null
  if (/^[1-9]$/.test(event.key)) return Number(event.key)
  const codeMatch = /^Digit([1-9])$/.exec(event.code)
  return codeMatch ? Number(codeMatch[1]) : null
}

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

// Both server lists paginate on their computed last_message_at value. Keeping
// the same key here makes the top-N merge correct even when a recently active
// task would otherwise have been outside the task page.
export const specTaskSortKey = (
  task: SpecTask,
  sortOrder: SidebarThreadSortOrder = 'updated_at',
): string | undefined => (
  sortOrder === 'created_at'
    ? task.created_at
    : task.last_message_at || task.created_at
)

export const buildProjectChatGroups = (
  projects: TypesProject[],
  specTasks: SpecTask[],
  sessions: TypesSessionSummary[],
  sortOrder: SidebarThreadSortOrder = 'updated_at',
  pinnedAtByItemKey: ReadonlyMap<string, string> = new Map(),
): SidebarGroup[] => {
  const defaultGroup: SidebarGroup = { id: 'default', name: 'No project', items: [] }
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
  const taskItemsById = new Map<string, SidebarItem>()
  specTasks.forEach((task) => {
    if (!task.id || !task.project_id) return
    const group = groupsByProjectId.get(task.project_id)
    if (!group) return
    taskIds.add(task.id)
    const item: SidebarItem = {
      id: task.id,
      kind: 'spec-task',
      title: task.user_short_title || task.short_title || task.name || 'Untitled task',
      createdAt: task.created_at,
      updatedAt: specTaskSortKey(task, sortOrder),
      projectId: task.project_id,
      task,
      pinnedAt: pinnedAtByItemKey.get(`spec-task:${task.id}`),
    }
    group.items.push(item)
    taskItemsById.set(task.id, item)
  })

  sessions.forEach((session) => {
    if (!session.session_id) return
    const metadata = session.metadata
    if (metadata?.spec_task_id && taskIds.has(metadata.spec_task_id)) {
      const taskItem = taskItemsById.get(metadata.spec_task_id)
      if (taskItem) taskItem.session = session
      return
    }

    if (metadata?.spec_task_id && metadata.project_id && groupsByProjectId.has(metadata.project_id)) {
      groupsByProjectId.get(metadata.project_id)?.items.push({
        id: metadata.spec_task_id,
        kind: 'spec-task',
        title: session.name || 'Untitled task',
        createdAt: session.created,
        updatedAt: session.last_message_at || session.created,
        projectId: metadata.project_id,
      })
      taskIds.add(metadata.spec_task_id)
      return
    }

    const projectGroup = metadata?.project_id
      ? groupsByProjectId.get(metadata.project_id)
      : undefined
    const group = projectGroup || defaultGroup
    group.items.push({
      id: session.session_id,
      kind: 'session',
      title: session.name || session.summary || 'Untitled chat',
      createdAt: session.created,
      updatedAt: session.last_message_at || session.created,
      projectId: projectGroup?.id,
      session,
      pinnedAt: pinnedAtByItemKey.get(`session:${session.session_id}`),
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
      items: [...group.items].sort((left, right) => (
        sortableTimestamp(right.pinnedAt) - sortableTimestamp(left.pinnedAt)
        || (right.pinnedAt ? 1 : 0) - (left.pinnedAt ? 1 : 0)
        ||
        sortableTimestamp(sortOrder === 'created_at' ? right.createdAt : right.updatedAt)
        - sortableTimestamp(sortOrder === 'created_at' ? left.createdAt : left.updatedAt)
      )),
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

// ---------------------------------------------------------------------------
// Bots
// ---------------------------------------------------------------------------

export type SidebarBot = {
  id: string
  name: string
  running: boolean
  restartRequired: boolean
  agentAppId?: string
  projectId?: string
  sessionId?: string
}

// Every helix-org agent owns a Helix project whose exploratory session is the
// agent's chat. The sidebar lists the agent itself, so that project must not
// also appear as an ordinary project group.
export const toSidebarBots = (bots: BotDTO[]): SidebarBot[] => (
  bots
    .filter((bot) => bot.kind !== 'human' && !!bot.id)
    .map((bot) => ({
      id: bot.id!,
      name: bot.name || bot.id!,
      running: bot.agent_status === 'running',
      restartRequired: !!bot.restart_required,
      agentAppId: bot.agent_id || bot.agent_app_id || undefined,
      projectId: bot.project_id || undefined,
      sessionId: bot.session_id || undefined,
    }))
    .sort((left, right) => (
      Number(right.running) - Number(left.running) || left.name.localeCompare(right.name)
    ))
)

export const botHomeProjectIds = (bots: SidebarBot[]): Set<string> => (
  new Set(bots.flatMap((bot) => bot.projectId ? [bot.projectId] : []))
)

export const withoutBotProjects = (projects: TypesProject[], bots: SidebarBot[]): TypesProject[] => {
  const hidden = botHomeProjectIds(bots)
  return projects.filter((project) => !project.id || !hidden.has(project.id))
}

export const filterSidebarBots = (bots: SidebarBot[], query: string): SidebarBot[] => (
  query.trim() ? bots.filter((bot) => matchesAllTokens(query, bot.name, bot.id)) : bots
)

// ---------------------------------------------------------------------------
// People
// ---------------------------------------------------------------------------

export type SidebarMember = {
  userId: string
  user: NonNullable<TypesOrganizationMembership['user']>
  online: boolean
}

const memberDisplayName = (user: SidebarMember['user']): string => (
  user.full_name || user.username || user.email || ''
)

// The current user's own work is already grouped by project above, and
// pending invitations (oin_… placeholders) have no sessions to show.
export const toSidebarMembers = (
  members: TypesOrganizationMembership[],
  currentUserId: string,
): SidebarMember[] => (
  members
    .filter((member): member is TypesOrganizationMembership & { user_id: string; user: SidebarMember['user'] } => (
      !!member.user_id && !!member.user && member.user_id !== currentUserId && !member.user_id.startsWith('oin_')
    ))
    .map((member) => ({ userId: member.user_id, user: member.user, online: !!member.online }))
    .sort((left, right) => (
      Number(right.online) - Number(left.online)
      || memberDisplayName(left.user).localeCompare(memberDisplayName(right.user))
      || left.userId.localeCompare(right.userId)
    ))
)

export const DEFAULT_VISIBLE_OFFLINE_MEMBERS = 5

// Online members and anyone whose work is expanded always show; the offline
// remainder is capped so a large org does not turn the sidebar into a
// directory. Searching lifts the cap and matches by name or email.
export const visibleSidebarMembers = (
  members: SidebarMember[],
  selectedUserIds: ReadonlySet<string>,
  query: string,
  showAll: boolean,
  offlineLimit = DEFAULT_VISIBLE_OFFLINE_MEMBERS,
): { members: SidebarMember[]; hiddenCount: number } => {
  if (query.trim()) {
    const matching = members.filter((member) => (
      matchesAllTokens(query, member.user.full_name, member.user.username, member.user.email)
    ))
    return { members: matching, hiddenCount: 0 }
  }
  if (showAll) return { members, hiddenCount: 0 }
  let offlineShown = 0
  const visible = members.filter((member) => {
    if (member.online || selectedUserIds.has(member.userId)) return true
    if (offlineShown < offlineLimit) {
      offlineShown += 1
      return true
    }
    return false
  })
  return { members: visible, hiddenCount: members.length - visible.length }
}

// A person's work is one flat, most-recent-first list across every project
// the viewer can see. Grouping by project first would hide what they are
// doing right now behind a fold per project.
export const buildPersonChatItems = (
  projects: TypesProject[],
  specTasks: SpecTask[],
  sessions: TypesSessionSummary[],
  sortOrder: SidebarThreadSortOrder = 'updated_at',
): SidebarItem[] => {
  const groups = buildProjectChatGroups(projects, specTasks, sessions, sortOrder)
  const items = groups.flatMap((group) => group.items.map((item) => ({
    ...item,
    projectName: group.id === 'default' ? undefined : group.name,
  })))
  return items.sort((left, right) => (
    sortableTimestamp(sortOrder === 'created_at' ? right.createdAt : right.updatedAt)
    - sortableTimestamp(sortOrder === 'created_at' ? left.createdAt : left.updatedAt)
  ))
}
