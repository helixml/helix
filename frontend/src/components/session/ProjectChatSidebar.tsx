import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import InputBase from '@mui/material/InputBase'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import {
  ChevronDown,
  ChevronRight,
  Folder,
  GitPullRequest,
  Search,
} from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import { useListProjects } from '../../services/projectService'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasksForProjects } from '../../services/specTaskService'
import { TypesSessionSummary } from '../../api/api'
import {
  buildProjectChatGroups,
  compactRelativeTime,
  dedupeSessions,
  filterProjectChatGroups,
  getSidebarTaskStatus,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'

const PAGE_SIZE = 50
const INITIAL_VISIBLE_ITEMS = 6
const SHOW_MORE_COUNT = 20
const RELATIVE_TIME_REFRESH_MS = 15000
const T3_FONT_FAMILY = '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif'

const ProjectChatSidebar: FC<{ onOpenSession: () => void }> = ({ onOpenSession }) => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const orgId = router.params.org_id || ''

  const [currentPage, setCurrentPage] = useState(0)
  const [allSessions, setAllSessions] = useState<TypesSessionSummary[]>([])
  const [hasMoreSessions, setHasMoreSessions] = useState(false)
  const [query, setQuery] = useState('')
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())
  const [visibleCounts, setVisibleCounts] = useState<Record<string, number>>({})
  const [relativeTimeNow, setRelativeTimeNow] = useState(() => Date.now())

  useEffect(() => {
    const interval = window.setInterval(() => setRelativeTimeNow(Date.now()), RELATIVE_TIME_REFRESH_MS)
    return () => window.clearInterval(interval)
  }, [])

  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId, {
    enabled: !!account.user?.id && !!orgId,
  })
  const projectIds = projects.flatMap((project) => project.id ? [project.id] : [])
  const specTasks = useSpecTasksForProjects(projectIds, {
    enabled: !!account.user?.id && projectIds.length > 0,
    refetchInterval: 10000,
  })
  const {
    data: sessionsData,
    isLoading: sessionsLoading,
    isFetching: sessionsFetching,
    error: sessionsError,
  } = useListSessions(
    orgId,
    undefined,
    undefined,
    undefined,
    currentPage,
    PAGE_SIZE,
    { enabled: !!account.user?.id, includeExternalAgents: true },
  )

  useEffect(() => {
    setCurrentPage(0)
    setAllSessions([])
    setHasMoreSessions(false)
  }, [orgId])

  const sessionsPage = sessionsData?.data
  const sessionsPageSignature = (sessionsPage?.sessions || [])
    .map((session) => `${session.session_id}:${session.updated}:${session.name}`)
    .join('|')

  useEffect(() => {
    const page = sessionsPage
    if (!page) return
    const pageSessions = page.sessions || []
    setAllSessions((previous) => dedupeSessions(
      currentPage === 0 ? pageSessions : [...previous, ...pageSessions],
    ))
    setHasMoreSessions((page.totalPages || 0) > currentPage + 1)
  }, [currentPage, sessionsPageSignature]) // eslint-disable-line react-hooks/exhaustive-deps

  const groups = buildProjectChatGroups(projects, specTasks, allSessions)
  const filteredGroups = filterProjectChatGroups(groups, query)

  const activeItemId = router.params.taskId || router.params.session_id || ''

  const openItem = (item: SidebarItem) => {
    if (item.kind === 'spec-task' && item.projectId) {
      account.orgNavigate('chat-task', { id: item.projectId, taskId: item.id })
    } else if (item.session?.question_set_execution_id) {
      account.orgNavigate('qa-results', {
        question_set_id: item.session.question_set_id,
        execution_id: item.session.question_set_execution_id,
      })
    } else {
      account.orgNavigate('session', { session_id: item.id })
    }
    onOpenSession()
  }

  const toggleGroup = (groupId: string) => {
    setCollapsedGroups((current) => {
      const next = new Set(current)
      if (next.has(groupId)) next.delete(groupId)
      else next.add(groupId)
      return next
    })
  }

  const renderItem = (item: SidebarItem) => {
    const active = item.id === activeItemId
    const status = item.kind === 'spec-task' ? getSidebarTaskStatus(item.task) : null
    return (
      <Box
        component="button"
        type="button"
        key={`${item.kind}:${item.id}`}
        onClick={() => openItem(item)}
        sx={{
          appearance: 'none',
          border: 0,
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
          font: 'inherit',
          '&:hover': {
            color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
            backgroundColor: active
              ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
              : (lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)'),
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
        <Typography
          component="span"
          title={item.updatedAt ? new Date(item.updatedAt).toLocaleString() : undefined}
          sx={{
            minWidth: 28,
            color: active
              ? (lightTheme.isLight ? 'rgba(39,39,42,0.58)' : 'rgba(241,243,247,0.72)')
              : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
            fontSize: '10px',
            lineHeight: 1,
            fontVariantNumeric: 'tabular-nums',
            textAlign: 'right',
            flexShrink: 0,
            pl: 0.5,
          }}
        >
          {compactRelativeTime(item.updatedAt, relativeTimeNow)}
        </Typography>
      </Box>
    )
  }

  const loading = (projectsLoading || sessionsLoading) && allSessions.length === 0

  return (
    <Box
      sx={{
        height: '100%',
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        fontFamily: T3_FONT_FAMILY,
        color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
        backgroundColor: lightTheme.isLight ? '#fafafa' : '#000000',
        '& .MuiTypography-root': { fontFamily: 'inherit' },
      }}
    >
      <Box
        sx={{
          height: 60,
          minHeight: 60,
          px: 1.5,
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
        }}
      >
        <Search size={15} color="currentColor" style={{ opacity: 0.55, flexShrink: 0 }} />
        <InputBase
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search"
          inputProps={{ 'aria-label': 'Search' }}
          sx={{
            flex: 1,
            minWidth: 0,
            color: 'inherit',
            fontFamily: 'inherit',
            fontSize: '14px',
            fontWeight: 500,
            '& input::placeholder': {
              color: lightTheme.isLight ? '#71717a' : '#a3a3a3',
              opacity: 1,
            },
          }}
        />
        <Tooltip title="New chat">
          <IconButton size="small" onClick={() => account.orgNavigate('chat')} aria-label="New chat">
            <AddIcon sx={{ fontSize: 18 }} />
          </IconButton>
        </Tooltip>
      </Box>

      <Box sx={{ px: 1.5, pt: 1.25, pb: 0.5, display: 'flex', alignItems: 'center' }}>
        <Typography
          sx={{
            flex: 1,
            color: lightTheme.isLight ? 'rgba(113,113,122,0.80)' : 'rgba(163,163,163,0.80)',
            fontFamily: 'inherit',
            fontSize: '12px',
            fontWeight: 500,
          }}
        >
          Projects
        </Typography>
      </Box>

      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          overflowY: 'auto',
          px: 0.75,
          pb: 1.5,
          scrollbarWidth: 'none',
          msOverflowStyle: 'none',
          '&::-webkit-scrollbar': { display: 'none' },
        }}
      >
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={22} />
          </Box>
        ) : sessionsError ? (
          <Typography color="error" sx={{ px: 1, py: 2, fontSize: '0.75rem' }}>
            Failed to load chats
          </Typography>
        ) : filteredGroups.length === 0 ? (
          <Typography color="text.secondary" sx={{ px: 1, py: 2, fontSize: '0.75rem' }}>
            {query ? 'No chats match your search.' : 'No chats yet.'}
          </Typography>
        ) : filteredGroups.map((group) => {
          const collapsed = collapsedGroups.has(group.id)
          const requestedCount = visibleCounts[group.id] || INITIAL_VISIBLE_ITEMS
          const previewItems = query ? group.items : group.items.slice(0, requestedCount)
          const activeHiddenItem = !query && group.items.slice(requestedCount).find((item) => item.id === activeItemId)
          const renderedItems = activeHiddenItem ? [...previewItems, activeHiddenItem] : previewItems
          const remaining = group.items.length - previewItems.length
          return (
            <Box key={group.id} sx={{ mb: 0.5 }}>
              <Box
                component="button"
                type="button"
                onClick={() => toggleGroup(group.id)}
                sx={{
                  appearance: 'none',
                  border: 0,
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
                  '&:hover': {
                    backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
                  },
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
                  {group.name}
                </Typography>
                <Typography
                  component="span"
                  sx={{
                    color: lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)',
                    fontFamily: 'inherit',
                    fontSize: '10px',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  {group.items.length}
                </Typography>
              </Box>
              {!collapsed && (
                <Box sx={{ pl: 1.15 }}>
                  {renderedItems.map(renderItem)}
                  {!query && remaining > 0 && (
                    <Box
                      component="button"
                      type="button"
                      onClick={() => setVisibleCounts((current) => ({
                        ...current,
                        [group.id]: requestedCount + SHOW_MORE_COUNT,
                      }))}
                      sx={{
                        appearance: 'none',
                        border: 0,
                        height: 30,
                        px: 1,
                        backgroundColor: 'transparent',
                        color: lightTheme.isLight ? 'rgba(113,113,122,0.75)' : 'rgba(163,163,163,0.75)',
                        cursor: 'pointer',
                        font: 'inherit',
                        fontSize: '12px',
                        '&:hover': {
                          color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
                          backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
                        },
                      }}
                    >
                      Show {Math.min(remaining, SHOW_MORE_COUNT)} more
                    </Box>
                  )}
                </Box>
              )}
            </Box>
          )
        })}

        {hasMoreSessions && !query && (
          <Box sx={{ display: 'flex', justifyContent: 'center', pt: 0.5 }}>
            <Box
              component="button"
              type="button"
              disabled={sessionsFetching}
              onClick={() => setCurrentPage((page) => page + 1)}
              sx={{
                appearance: 'none',
                border: 0,
                borderRadius: 1,
                px: 1.5,
                py: 0.75,
                color: 'text.secondary',
                backgroundColor: 'transparent',
                cursor: sessionsFetching ? 'default' : 'pointer',
                font: 'inherit',
                fontSize: '0.7rem',
                '&:hover': { color: 'text.primary', backgroundColor: 'action.hover' },
              }}
            >
              {sessionsFetching ? 'Loading…' : 'Load older chats'}
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default ProjectChatSidebar
