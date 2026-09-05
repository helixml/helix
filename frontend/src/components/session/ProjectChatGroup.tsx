import { FC, MouseEvent, MutableRefObject, useEffect, useState } from 'react'
import { useQueries } from '@tanstack/react-query'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import {
  ChevronDown,
  ChevronRight,
  Folder,
  Plus,
} from 'lucide-react'

import type { TypesOrganizationMembership, TypesProject, TypesSessionSummary, TypesUser } from '../../api/api'
import type { TypesPinnedChat } from '../../api/api'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import useApi from '../../hooks/useApi'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasks } from '../../services/specTaskService'
import type { SpecTask } from '../../services/specTaskService'
import { useGetProjectRepositories } from '../../services/projectService'
import {
  buildProjectChatGroups,
  filterProjectChatGroups,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'
import type { SidebarThreadSortOrder } from './ProjectChatSidebar.logic'
import type { SortableProjectHandleProps } from './SortableProject'
import ProjectChatItemRow from './ProjectChatItemRow'

const SHOW_MORE_COUNT = 20

type GroupVisibility = 'unknown' | 'visible' | 'empty'

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
          {renderedItems.map((item) => (
            <ProjectChatItemRow
              key={`${item.kind}:${item.id}`}
              item={item}
              active={item.id === activeItemId}
              relativeTimeNow={relativeTimeNow}
              archived={archived}
              archivingItemId={archivingItemId}
              organizationMembers={organizationMembers}
              currentUser={currentUser}
              showTaskAvatars={showTaskAvatars}
              repositoryName={repositoryName}
              defaultBranch={project?.default_branch}
              onOpenItem={onOpenItem}
              onOpenItemContextMenu={onOpenItemContextMenu}
              onArchiveItem={onArchiveItem}
            />
          ))}
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
