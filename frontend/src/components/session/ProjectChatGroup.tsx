import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import {
  Archive,
  ChevronDown,
  ChevronRight,
  Folder,
  GitPullRequest,
  Plus,
} from 'lucide-react'

import type { TypesProject } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasks } from '../../services/specTaskService'
import {
  buildProjectChatGroups,
  compactRelativeTime,
  filterProjectChatGroups,
  getSidebarTaskStatus,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'

const INITIAL_VISIBLE_ITEMS = 6
const SHOW_MORE_COUNT = 20

type ProjectChatGroupProps = {
  orgId: string
  project?: TypesProject
  collapsed: boolean
  query: string
  activeItemId: string
  relativeTimeNow: number
  enabled: boolean
  archivingItemId: string | null
  onToggle: () => void
  onNewTask?: () => void
  onOpenItem: (item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

const ProjectChatGroup: FC<ProjectChatGroupProps> = ({
  orgId,
  project,
  collapsed,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  archivingItemId,
  onToggle,
  onNewTask,
  onOpenItem,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_ITEMS)
  const projectId = project?.id
  const groupId = projectId || 'default'
  const groupName = project?.name || 'None'
  const requestCount = visibleCount + 1

  const sessionsQuery = useListSessions(
    orgId,
    undefined,
    undefined,
    projectId,
    0,
    requestCount,
    {
      enabled,
      includeExternalAgents: true,
      projectScope: projectId ? 'project' : 'none',
      sort: 'updated',
    },
  )
  const tasksQuery = useSpecTasks({
    projectId,
    limit: requestCount,
    offset: 0,
    enabled: enabled && !!projectId,
    refetchInterval: 10000,
  })

  const sessionsPage = sessionsQuery.data?.data
  const sessions = sessionsPage?.sessions || []
  const tasks = projectId ? tasksQuery.data || [] : []
  const group = buildProjectChatGroups(project ? [project] : [], tasks, sessions)
    .find((candidate) => candidate.id === groupId)
  const items = group?.items || []
  const filteredItems = filterProjectChatGroups([{ id: groupId, name: groupName, items }], query)[0]?.items || []
  const previewItems = filteredItems.slice(0, visibleCount)
  const activeHiddenItem = filteredItems.slice(visibleCount).find((item) => item.id === activeItemId)
  const renderedItems = activeHiddenItem ? [...previewItems, activeHiddenItem] : previewItems
  const sessionsHaveMore = (sessionsPage?.totalCount || 0) > sessions.length
  const tasksMayHaveMore = !!projectId && tasks.length === requestCount
  const hasMore = filteredItems.length > visibleCount || sessionsHaveMore || tasksMayHaveMore
  const isLoading = enabled && (sessionsQuery.isLoading || (!!projectId && tasksQuery.isLoading))
  const isFetchingMore = sessionsQuery.isFetching || tasksQuery.isFetching
  const hasError = sessionsQuery.isError || tasksQuery.isError

  if (!collapsed && !isLoading && !hasError && !hasMore && items.length === 0 && !!projectId) {
    return null
  }
  if (query && !isLoading && !hasError && !hasMore && filteredItems.length === 0) {
    return null
  }

  const countLabel = hasMore ? `${items.length}+` : `${items.length}`

  return (
    <Box sx={{ mb: 0.5 }}>
      <Box
        role="button"
        tabIndex={0}
        onClick={onToggle}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onToggle()
          }
        }}
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
          cursor: 'pointer',
          textAlign: 'left',
          font: 'inherit',
          outline: 'none',
          '&:hover': {
            backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
          },
          '&:hover .sidebar-group-count, &:focus-within .sidebar-group-count': {
            opacity: onNewTask ? 0 : 1,
          },
          '&:hover .sidebar-group-new, &:focus-within .sidebar-group-new': {
            opacity: 1,
          },
          '@media (hover: none)': onNewTask ? {
            '& .sidebar-group-count': { opacity: 0 },
            '& .sidebar-group-new': { opacity: 1 },
          } : undefined,
        }}
      >
        {collapsed ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
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
        {isLoading ? (
          <CircularProgress size={11} color="inherit" />
        ) : (
          <Box sx={{ width: onNewTask ? 44 : 'auto', height: 26, position: 'relative', flexShrink: 0 }}>
            <Typography
              className="sidebar-group-count"
              component="span"
              sx={{
                position: onNewTask ? 'absolute' : 'static',
                inset: onNewTask ? 0 : undefined,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'flex-end',
                color: lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)',
                fontFamily: 'inherit',
                fontSize: '10px',
                fontVariantNumeric: 'tabular-nums',
                transition: 'opacity 100ms ease',
              }}
            >
              {countLabel}
            </Typography>
            {onNewTask && (
              <Box
                className="sidebar-group-new"
                component="button"
                type="button"
                onClick={(event) => {
                  event.stopPropagation()
                  onNewTask()
                }}
                sx={{
                  appearance: 'none',
                  position: 'absolute',
                  inset: 0,
                  border: 0,
                  p: 0,
                  backgroundColor: 'transparent',
                  color: lightTheme.isLight ? '#52525b' : 'rgba(212,212,216,0.82)',
                  cursor: 'pointer',
                  font: 'inherit',
                  fontSize: '10px',
                  fontWeight: 500,
                  opacity: 0,
                  transition: 'opacity 100ms ease',
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
            )}
          </Box>
        )}
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
            const isArchiving = archivingItemId === item.id
            return (
              <Box
                key={`${item.kind}:${item.id}`}
                role="button"
                tabIndex={0}
                onClick={() => onOpenItem(item)}
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
                  height: 32,
                  px: 1,
                  borderRadius: '6px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.75,
                  color: active
                    ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
                    : (lightTheme.isLight ? '#71717a' : 'rgba(163,163,163,0.80)'),
                  backgroundColor: active
                    ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
                    : 'transparent',
                  cursor: 'pointer',
                  textAlign: 'left',
                  outline: 'none',
                  '&:hover, &:focus-visible': {
                    color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
                    backgroundColor: active
                      ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
                      : (lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)'),
                  },
                  '&:hover .sidebar-item-time, &:focus-within .sidebar-item-time': { opacity: 0 },
                  '&:hover .sidebar-item-archive, &:focus-within .sidebar-item-archive': { opacity: 1 },
                  '@media (hover: none)': {
                    '& .sidebar-item-time': { opacity: 0 },
                    '& .sidebar-item-archive': { opacity: 1 },
                  },
                }}
              >
                {item.kind === 'spec-task' && (
                  <GitPullRequest size={13} color={status?.color || 'currentColor'} style={{ flexShrink: 0 }} />
                )}
                {status && (
                  <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.45, flexShrink: 0 }}>
                    <Box sx={{ width: 5, height: 5, borderRadius: '50%', backgroundColor: status.color }} />
                    <Typography component="span" sx={{ fontSize: '0.66rem', color: status.color, lineHeight: 1 }}>
                      {status.label}
                    </Typography>
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
                <Box sx={{ width: 28, height: 28, flexShrink: 0, position: 'relative' }}>
                  <Typography
                    className="sidebar-item-time"
                    component="span"
                    title={item.updatedAt ? new Date(item.updatedAt).toLocaleString() : undefined}
                    sx={{
                      position: 'absolute',
                      inset: 0,
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
                  <Tooltip title={item.kind === 'spec-task' ? 'Archive task' : 'Archive chat'}>
                    <IconButton
                      className="sidebar-item-archive"
                      size="small"
                      disabled={isArchiving}
                      aria-label={item.kind === 'spec-task' ? `Archive task ${item.title}` : `Archive chat ${item.title}`}
                      onClick={(event) => {
                        event.stopPropagation()
                        onArchiveItem(item)
                      }}
                      sx={{
                        position: 'absolute',
                        top: 0,
                        right: 0,
                        bottom: 0,
                        width: 20,
                        height: 28,
                        opacity: 0,
                        color: 'inherit',
                        transition: 'opacity 100ms ease',
                      }}
                    >
                      {isArchiving ? <CircularProgress size={12} color="inherit" /> : <Archive size={14} />}
                    </IconButton>
                  </Tooltip>
                </Box>
              </Box>
            )
          })}
          {hasMore && (
            <Box
              component="button"
              type="button"
              disabled={isFetchingMore}
              onClick={() => setVisibleCount((count) => count + SHOW_MORE_COUNT)}
              sx={{
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
              }}
            >
              {isFetchingMore ? 'Loading…' : 'Show more'}
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatGroup
