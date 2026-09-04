import { FC, MouseEvent, MutableRefObject, ReactElement, useEffect, useState } from 'react'
import { useQueries } from '@tanstack/react-query'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { keyframes } from '@mui/material/styles'
import {
  Archive,
  ArchiveRestore,
  ChevronDown,
  ChevronRight,
  Folder,
  GitPullRequest,
  Pin,
  Plus,
} from 'lucide-react'

import type { TypesOrganizationMembership, TypesProject, TypesSessionSummary, TypesUser } from '../../api/api'
import type { TypesPinnedChat } from '../../api/api'
import useApps from '../../hooks/useApps'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import useApi from '../../hooks/useApi'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasks } from '../../services/specTaskService'
import type { SpecTask } from '../../services/specTaskService'
import { useGetProjectRepositories } from '../../services/projectService'
import OrganizationUserAvatar, { resolveOrganizationUser } from '../widgets/OrganizationUserAvatar'
import {
  buildProjectChatGroups,
  compactRelativeTime,
  filterProjectChatGroups,
  getSidebarPullRequestIcon,
  getSidebarTaskStatus,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'
import type { SidebarThreadSortOrder } from './ProjectChatSidebar.logic'
import type { SortableProjectHandleProps } from './SortableProject'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'
import AgentHarness from '../agent/AgentHarness'
import { GitBranch } from 'lucide-react'
import { getProjectChatItemDetails, resolveProjectChatItemBranch } from './projectChatItemDetails'

const SHOW_MORE_COUNT = 20

type GroupVisibility = 'unknown' | 'visible' | 'empty'

const activeStatusDotPulse = keyframes`
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
`

type ProjectChatGroupProps = {
  orgId: string
  project?: TypesProject
  collapsed: boolean
  query: string
  activeItemId: string
  relativeTimeNow: number
  enabled: boolean
  threadSortOrder?: SidebarThreadSortOrder
  visibleThreadCount?: number
  participantIds: string[]
  organizationMembers: TypesOrganizationMembership[]
  currentUser?: TypesUser
  showTaskAvatars?: boolean
  archived?: boolean
  pinnedChats?: TypesPinnedChat[]
  archivingItemId: string | null
  onToggle: () => void
  onNewTask?: () => void
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onOpenProjectContextMenu?: (event: MouseEvent<HTMLElement>, project: TypesProject) => void
  onArchiveItem: (item: SidebarItem) => void
  manualSorting?: boolean
  dragHandleProps?: SortableProjectHandleProps
  dragInProgressRef?: MutableRefObject<boolean>
  suppressClickAfterDragRef?: MutableRefObject<boolean>
}

const ProjectChatGroup: FC<ProjectChatGroupProps> = ({
  orgId,
  project,
  collapsed,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  threadSortOrder = 'updated_at',
  visibleThreadCount = 6,
  participantIds,
  organizationMembers,
  currentUser,
  showTaskAvatars = false,
  archived = false,
  pinnedChats = [],
  archivingItemId,
  onToggle,
  onNewTask,
  onOpenItem,
  onOpenItemContextMenu,
  onOpenProjectContextMenu,
  onArchiveItem,
  manualSorting = false,
  dragHandleProps,
  dragInProgressRef,
  suppressClickAfterDragRef,
}) => {
  const api = useApi()
  const lightTheme = useLightTheme()
  const { apps } = useApps()
  // No hover on a phone, so the facts the tooltip carries have to live on the
  // row itself. That makes the row two lines, and taller.
  const isPhone = useIsPhone()
  const [additionalVisibleCount, setAdditionalVisibleCount] = useState(0)
  const [visibility, setVisibility] = useState<GroupVisibility>('unknown')
  const visibleCount = visibleThreadCount + additionalVisibleCount
  const projectId = project?.id
  const groupId = projectId || 'default'
  const groupName = project?.name || 'No project'
  const requestCount = visibleCount + 1
  // A collapsed group probes once to determine whether the current participant
  // filter can see anything. Visible collapsed groups then stop querying; empty
  // ones keep watching so a newly assigned task can make the project reappear.
  const queriesEnabled = enabled && (!collapsed || visibility !== 'visible')

  const sessionsQuery = useListSessions(
    orgId,
    undefined,
    projectId,
    0,
    requestCount,
    {
      enabled: queriesEnabled,
      includeExternalAgents: true,
      projectScope: projectId ? 'project' : 'none',
      sort: threadSortOrder === 'created_at' ? 'created' : 'last_message',
      archived,
    },
  )
  const tasksQuery = useSpecTasks({
    projectId,
    limit: requestCount,
    offset: 0,
    sort: threadSortOrder === 'created_at' ? 'created' : 'last_message',
    archivedOnly: archived,
    participantIds,
    enabled: queriesEnabled && !!projectId,
    refetchInterval: archived ? false : 10000,
  })
  const repositoriesQuery = useGetProjectRepositories(projectId || '', queriesEnabled && !!projectId)

  const sessionsPage = sessionsQuery.data?.data
  const pagedSessions = sessionsPage?.sessions || []
  const pagedTasks = projectId ? tasksQuery.data || [] : []
  const groupPins = pinnedChats.filter((pin) => (pin.project_id || undefined) === projectId)
  const pinnedQueries = useQueries({
    queries: groupPins.map((pin) => ({
      queryKey: ['pinned-chat-detail', pin.kind, pin.id],
      queryFn: async () => {
        if (pin.kind === 'spec-task') return (await api.getApiClient().v1SpecTasksDetail(pin.id!)).data
        const session = (await api.getApiClient().v1SessionsDetail(pin.id!)).data
        return {
          session_id: session.id,
          name: session.name,
          created: session.created,
          updated: session.updated,
          metadata: session.config,
          archived: session.archived,
        } satisfies TypesSessionSummary
      },
      enabled: queriesEnabled && !!pin.id,
      staleTime: 10000,
    })),
  })
  const pinnedSessions = pinnedQueries.flatMap((query, index) => (
    groupPins[index]?.kind === 'session'
      && query.data
      && !!(query.data as TypesSessionSummary).archived === archived
      ? [query.data as TypesSessionSummary]
      : []
  ))
  const pinnedTasks = pinnedQueries.flatMap((query, index) => (
    groupPins[index]?.kind === 'spec-task'
      && query.data
      && !!(query.data as SpecTask).archived === archived
      ? [query.data as SpecTask]
      : []
  ))
  const sessions = [...pinnedSessions, ...pagedSessions.filter((session) => (
    !pinnedSessions.some((pinned) => pinned.session_id === session.session_id)
  ))]
  const tasks = [...pinnedTasks, ...pagedTasks.filter((task) => (
    !pinnedTasks.some((pinned) => pinned.id === task.id)
  ))]
  const repositories = repositoriesQuery.data || []
  const primaryRepository = repositories.find((repository) => repository.id === project?.default_repo_id)
    || repositories[0]
  const repositoryName = primaryRepository?.name
  const pinnedAtByItemKey = new Map(pinnedChats.map((pin) => [`${pin.kind}:${pin.id}`, pin.pinned_at || '']))
  const group = buildProjectChatGroups(project ? [project] : [], tasks, sessions, threadSortOrder, pinnedAtByItemKey)
    .find((candidate) => candidate.id === groupId)
  const items = group?.items || []
  const filteredItems = filterProjectChatGroups([{ id: groupId, name: groupName, items }], query)[0]?.items || []
  const previewItems = filteredItems.slice(0, visibleCount)
  const activeHiddenItem = filteredItems.slice(visibleCount).find((item) => item.id === activeItemId)
  const renderedItems = activeHiddenItem ? [...previewItems, activeHiddenItem] : previewItems
  const sessionsHaveMore = (sessionsPage?.totalCount || 0) > sessions.length
  const tasksMayHaveMore = !!projectId && tasks.length === requestCount
  const hasMore = filteredItems.length > visibleCount || sessionsHaveMore || tasksMayHaveMore
  const canShowLess = additionalVisibleCount > 0
  const isLoading = queriesEnabled && (
    sessionsQuery.isLoading
    || (!!projectId && tasksQuery.isLoading)
    || pinnedQueries.some((pinnedQuery) => pinnedQuery.isLoading)
  )
  const isFetchingMore = sessionsQuery.isFetching || tasksQuery.isFetching
  const hasError = sessionsQuery.isError || tasksQuery.isError
  const archiveVerb = archived ? 'Unarchive' : 'Archive'
  // Archived groups have no "new task", so there the name keeps its old job.
  const activateGroup = onNewTask || onToggle
  const paginationButtonSx = {
    appearance: 'none',
    border: 0,
    height: 30,
    px: 1,
    backgroundColor: 'transparent',
    color: lightTheme.isLight ? 'rgba(113,113,122,0.75)' : 'rgba(163,163,163,0.75)',
    cursor: isFetchingMore ? 'default' : 'pointer',
    font: 'inherit',
    fontSize: '12px',
    '&:hover': {
      color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
      backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
    },
  }

  const participantScope = participantIds.join('\u0000')
  useEffect(() => {
    setVisibility('unknown')
  }, [archived, participantScope])

  const hasVisibleItems = items.length > 0 || hasMore
  useEffect(() => {
    if (!projectId || isLoading || hasError) return
    setVisibility(hasVisibleItems ? 'visible' : 'empty')
  }, [hasError, hasVisibleItems, isLoading, projectId])

  if (projectId && archived && !isLoading && !hasError && !hasVisibleItems) {
    return null
  }

  if (query && !isLoading && !hasError && !hasMore && filteredItems.length === 0) {
    return null
  }

  return (
    <Box sx={{ mb: 0.5 }}>
      <Box
        role="button"
        tabIndex={0}
        ref={manualSorting ? dragHandleProps?.setActivatorNodeRef : undefined}
        {...(manualSorting ? dragHandleProps?.attributes : {})}
        {...(manualSorting ? dragHandleProps?.listeners : {})}
        onPointerDownCapture={() => {
          if (suppressClickAfterDragRef) suppressClickAfterDragRef.current = false
        }}
        onClick={(event) => {
          if (dragInProgressRef?.current || suppressClickAfterDragRef?.current) {
            if (suppressClickAfterDragRef) suppressClickAfterDragRef.current = false
            event.preventDefault()
            event.stopPropagation()
            return
          }
          // The name is the project's primary action: start work here. Only the
          // chevron collapses, so reaching a project no longer costs a round
          // trip through a collapse you did not want.
          activateGroup()
        }}
        onContextMenu={(event) => {
          if (!project || !onOpenProjectContextMenu) return
          event.preventDefault()
          event.stopPropagation()
          onOpenProjectContextMenu(event, project)
        }}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            activateGroup()
          }
        }}
        aria-label={onNewTask ? `New task in ${groupName}` : groupName}
        sx={{
          width: '100%',
          height: 32,
          px: 0.75,
          display: 'flex',
          alignItems: 'center',
          gap: 0.65,
          borderRadius: '6px',
          backgroundColor: 'transparent',
          color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
          cursor: manualSorting ? 'grab' : 'pointer',
          '&:active': manualSorting ? { cursor: 'grabbing' } : undefined,
          textAlign: 'left',
          font: 'inherit',
          outline: 'none',
          '&:hover': {
            backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
          },
        }}
      >
        <Box
          component="button"
          type="button"
          aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${groupName}`}
          aria-expanded={!collapsed}
          // Must not reach the row's click (new chat) or the drag sensor.
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => {
            event.stopPropagation()
            onToggle()
          }}
          sx={{
            appearance: 'none',
            border: 0,
            p: 0,
            m: 0,
            backgroundColor: 'transparent',
            color: 'inherit',
            cursor: 'pointer',
            flexShrink: 0,
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            // A 13px glyph is not a touch target; the box around it is.
            width: isPhone ? 28 : 16,
            height: isPhone ? 28 : 16,
            borderRadius: '4px',
            '&:hover': {
              backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(241,243,247,0.12)',
            },
          }}
        >
          {collapsed ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
        </Box>
        <Folder size={14} style={{ opacity: 0.72 }} />
        <Typography
          component="span"
          sx={{
            minWidth: 0,
            flex: 1,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontFamily: 'inherit',
            fontSize: '14px',
            lineHeight: '20px',
            fontWeight: 500,
          }}
        >
          {groupName}
        </Typography>
        {/* No item count here: each group only ever holds the page it fetched
            (visibleCount + 1), so any number rendered would be the page size
            rather than the project's real total. */}
        {isLoading ? (
          <CircularProgress size={11} color="inherit" />
        ) : onNewTask ? (
          <Box
            className="sidebar-group-new"
            component="button"
            type="button"
            // The row around it already exposes this exact action, so to a
            // screen reader this is a duplicate control with the same name —
            // it stays as the visual hint and keeps out of the a11y tree.
            aria-hidden="true"
            tabIndex={-1}
            onClick={(event) => {
              event.stopPropagation()
              onNewTask()
            }}
            sx={{
              appearance: 'none',
              height: 26,
              flexShrink: 0,
              border: 0,
              p: 0,
              backgroundColor: 'transparent',
              color: lightTheme.isLight ? '#52525b' : 'rgba(212,212,216,0.82)',
              cursor: 'pointer',
              font: 'inherit',
              fontSize: '10px',
              fontWeight: 500,
              opacity: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'flex-end',
              gap: 0.25,
              '&:hover': { color: lightTheme.isLight ? '#18181b' : '#ffffff' },
            }}
          >
            <Plus size={11} strokeWidth={1.8} />
            New
          </Box>
        ) : null}
      </Box>

      {!collapsed && (
        <Box sx={{ pl: 1.15 }}>
          {hasError && (
            <Typography color="error" sx={{ px: 1, py: 0.75, fontSize: '0.7rem' }}>
              Failed to load chats
            </Typography>
          )}
          {renderedItems.map((item) => {
            const active = item.id === activeItemId
            const status = item.kind === 'spec-task' ? getSidebarTaskStatus(item.task) : null
            const isAgentWorking = item.kind === 'spec-task' && item.task?.agent_work_state === 'working'
            const pullRequestIcon = item.kind === 'spec-task'
              ? getSidebarPullRequestIcon(item.task)
              : undefined
            const isArchiving = archivingItemId === item.id
            const taskPersonId = item.task?.assignee_id || item.task?.created_by || ''
            const taskPerson = resolveOrganizationUser(taskPersonId, organizationMembers, currentUser)
            const taskPersonRole = item.task?.assignee_id ? 'Assigned to' : 'Created by'
            const branch = resolveProjectChatItemBranch(item, project?.default_branch)
            // Only resolved for the phone layout — on desktop the tooltip does
            // its own lookup when it actually opens.
            const details = isPhone
              ? getProjectChatItemDetails({ item, apps, repository: repositoryName, branch })
              : undefined
            const subLine = details
              ? [
                  details.branch && { key: 'branch', icon: <GitBranch size={11} />, value: details.branch },
                  // Icon only — the mark identifies the harness, the name just
                  // ate horizontal space on a phone.
                  details.harness && {
                    key: 'harness',
                    icon: <AgentHarness runtime={details.runtime || ''} variant="short" size={11} />,
                  },
                ].filter(Boolean) as Array<{ key: string; icon: ReactElement; value?: string }>
              : []
            return (
              <ProjectChatItemTooltip
                key={`${item.kind}:${item.id}`}
                item={item}
                repository={repositoryName}
                branch={branch}
                disabled={isPhone}
              >
              <Box
                className="project-chat-item"
                role="button"
                tabIndex={0}
                onClick={() => onOpenItem(item)}
                onContextMenu={(event) => onOpenItemContextMenu(event, item)}
                onKeyDown={(event) => {
                  if (event.target !== event.currentTarget) return
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    onOpenItem(item)
                  }
                }}
                sx={{
                  width: '100%',
                  minWidth: 0,
                  // Two lines and a 48px touch target on a phone; the dense
                  // single-line row everywhere else.
                  ...(isPhone
                    ? { minHeight: 52, py: 0.75, flexDirection: 'column', alignItems: 'stretch', gap: 0.25 }
                    : { height: 32, flexDirection: 'row', alignItems: 'center', gap: 0.75 }),
                  px: 1,
                  borderRadius: '6px',
                  display: 'flex',
                  color: active
                    ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
                    : (lightTheme.isLight ? '#71717a' : 'rgba(163,163,163,0.80)'),
                  backgroundColor: active
                    ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
                    : 'transparent',
                  cursor: 'pointer',
                  textAlign: 'left',
                  outline: 'none',
                  position: 'relative',
                  '&:hover, &:focus-visible': {
                    color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
                    backgroundColor: active
                      ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
                      : (lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)'),
                  },
                  '&:hover .sidebar-item-time, &:focus-within .sidebar-item-time': { opacity: 0 },
                  '&:hover .sidebar-item-archive, &:focus-within .sidebar-item-archive': { opacity: 1 },
                  // Without hover the archive button can never appear, so on a
                  // coarse pointer it is always shown. On a phone it gets its
                  // own column instead (below), so the time stays visible too.
                  '@media (hover: none)': {
                    '& .sidebar-item-time': { opacity: isPhone ? 1 : 0 },
                    '& .sidebar-item-archive': { opacity: 1 },
                  },
                  ...(isPhone && {
                    // Rows are tall enough on a phone that adjacent ones read as
                    // one block without a divider between them.
                    borderBottom: '1px solid',
                    borderColor: lightTheme.isLight
                      ? 'rgba(0,0,0,0.06)'
                      : 'rgba(241,243,247,0.07)',
                    borderRadius: 0,
                    '&:last-of-type': { borderBottom: 'none' },
                  }),
                }}
              >
                <Box
                  sx={isPhone
                    ? { display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, width: '100%' }
                    : { display: 'contents' }}
                >
                {item.kind === 'spec-task' && (
                  <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, flexShrink: 0 }}>
                    <Tooltip title={pullRequestIcon?.tooltip || ''}>
                      <Box
                        component="a"
                        href={pullRequestIcon?.url}
                        target={pullRequestIcon?.url ? '_blank' : undefined}
                        rel={pullRequestIcon?.url ? 'noopener noreferrer' : undefined}
                        aria-label={pullRequestIcon?.tooltip}
                        onMouseOver={(event) => event.stopPropagation()}
                        onClick={(event) => {
                          event.stopPropagation()
                          if (!pullRequestIcon?.url) event.preventDefault()
                        }}
                        sx={{
                          display: 'inline-flex',
                          color: pullRequestIcon?.color || 'currentColor',
                          cursor: pullRequestIcon?.url ? 'pointer' : 'default',
                        }}
                      >
                        <GitPullRequest size={13} />
                      </Box>
                    </Tooltip>
                    {status && (
                      <Tooltip title={status.tooltip || ''} disableHoverListener={!status.tooltip}>
                        <Box
                          onMouseOver={(event) => event.stopPropagation()}
                          sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.45 }}
                        >
                          <Box
                            sx={{
                              width: 5,
                              height: 5,
                              borderRadius: '50%',
                              backgroundColor: status.color,
                              animation: isAgentWorking
                                ? `${activeStatusDotPulse} 2s ease-in-out infinite`
                                : 'none',
                              '@media (prefers-reduced-motion: reduce)': {
                                animation: 'none',
                              },
                            }}
                          />
                          <Typography component="span" sx={{ fontSize: '0.66rem', color: status.color, lineHeight: 1 }}>
                            {status.label}
                          </Typography>
                        </Box>
                      </Tooltip>
                    )}
                  </Box>
                )}
                <Typography
                  component="span"
                  sx={{
                    minWidth: 0,
                    flex: 1,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    fontSize: '14px',
                    lineHeight: '20px',
                    fontWeight: active ? 500 : 400,
                  }}
                >
                  {item.title}
                </Typography>
                {showTaskAvatars && item.kind === 'spec-task' && (
                  <Tooltip title={`${taskPersonRole} ${taskPerson?.full_name || taskPerson?.username || taskPerson?.email || 'unknown user'}`}>
                    <Box sx={{ width: 18, height: 18, flexShrink: 0, display: 'inline-flex' }}>
                      <OrganizationUserAvatar
                        userId={taskPersonId}
                        members={organizationMembers}
                        currentUser={currentUser}
                        size={18}
                        fontSize="0.55rem"
                        iconSize={16}
                      />
                    </Box>
                  </Tooltip>
                )}
                {item.pinnedAt && (
                  <Tooltip title="Pinned">
                    <Box
                      component="span"
                      className="sidebar-item-pin"
                      aria-label="Pinned"
                      onMouseOver={(event) => event.stopPropagation()}
                      sx={{
                        width: 14,
                        height: 28,
                        flexShrink: 0,
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: active
                          ? (lightTheme.isLight ? 'rgba(39,39,42,0.68)' : 'rgba(241,243,247,0.78)')
                          : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.62)'),
                        transition: 'opacity 100ms ease',
                      }}
                    >
                      <Pin size={11} fill="currentColor" strokeWidth={1.7} />
                    </Box>
                  </Tooltip>
                )}
                <Box
                  sx={isPhone
                    ? { display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }
                    : { width: 28, height: 28, flexShrink: 0, position: 'relative' }}
                >
                  <Typography
                    className="sidebar-item-time"
                    component="span"
                    title={item.updatedAt ? new Date(item.updatedAt).toLocaleString() : undefined}
                    sx={{
                      ...(isPhone
                        ? { position: 'static' }
                        : { position: 'absolute', inset: 0 }),
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'flex-end',
                      color: active
                        ? (lightTheme.isLight ? 'rgba(39,39,42,0.58)' : 'rgba(241,243,247,0.72)')
                        : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
                      fontSize: '10px',
                      lineHeight: 1,
                      fontVariantNumeric: 'tabular-nums',
                      transition: 'opacity 100ms ease',
                    }}
                  >
                    {compactRelativeTime(item.updatedAt, relativeTimeNow)}
                  </Typography>
                  <Tooltip title={`${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'}`}>
                    <IconButton
                      className="sidebar-item-archive"
                      size="small"
                      disabled={isArchiving}
                      aria-label={`${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'} ${item.title}`}
                      onMouseOver={(event) => event.stopPropagation()}
                      onClick={(event) => {
                        event.stopPropagation()
                        onArchiveItem(item)
                      }}
                      sx={{
                        ...(isPhone
                          ? { position: 'static', width: 28, height: 28 }
                          : { position: 'absolute', top: 0, right: 0, bottom: 0, width: 20, height: 28 }),
                        opacity: 0,
                        color: 'inherit',
                        transition: 'opacity 100ms ease',
                      }}
                    >
                      {isArchiving
                        ? <CircularProgress size={12} color="inherit" />
                        : archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
                    </IconButton>
                  </Tooltip>
                </Box>
                </Box>
                {isPhone && subLine.length > 0 && (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 1.25,
                      minWidth: 0,
                      color: lightTheme.isLight
                        ? 'rgba(113,113,122,0.85)'
                        : 'rgba(163,163,163,0.72)',
                    }}
                  >
                    {subLine.map((entry) => (
                      <Box
                        key={entry.key}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 0.5,
                          minWidth: 0,
                          // The branch takes the slack; the other entries are
                          // icon-sized and should stay whole.
                          flexShrink: entry.key === 'branch' ? 1 : 0,
                        }}
                      >
                        <Box sx={{ display: 'inline-flex', flexShrink: 0 }}>{entry.icon}</Box>
                        {entry.value && (
                          <Typography
                            component="span"
                            sx={{
                              minWidth: 0,
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                              fontSize: '11px',
                              lineHeight: '15px',
                            }}
                          >
                            {entry.value}
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Box>
                )}
              </Box>
              </ProjectChatItemTooltip>
            )
          })}
          {(canShowLess || hasMore) && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {canShowLess && (
                <Box
                  component="button"
                  type="button"
                  disabled={isFetchingMore}
                  onClick={() => setAdditionalVisibleCount(0)}
                  sx={paginationButtonSx}
                >
                  Show less
                </Box>
              )}
              {hasMore && (
                <Box
                  component="button"
                  type="button"
                  disabled={isFetchingMore}
                  onClick={() => setAdditionalVisibleCount((count) => count + SHOW_MORE_COUNT)}
                  sx={paginationButtonSx}
                >
                  {isFetchingMore ? 'Loading…' : 'Show more'}
                </Box>
              )}
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatGroup
