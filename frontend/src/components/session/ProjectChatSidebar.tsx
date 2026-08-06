import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import InputBase from '@mui/material/InputBase'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Search, SquarePen } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import { useListProjects } from '../../services/projectService'
import { useArchiveSession } from '../../services/sessionService'
import { useArchiveSpecTask } from '../../services/specTaskService'
import SimpleConfirmWindow from '../widgets/SimpleConfirmWindow'
import {
  collapsedGroupsStorageKey,
  isNewThreadShortcut,
  shouldConfirmTaskArchive,
  parseCollapsedGroupIds,
  serializeCollapsedGroupIds,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'
import ProjectChatGroup from './ProjectChatGroup'

const RELATIVE_TIME_REFRESH_MS = 15000
const T3_FONT_FAMILY = '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif'

const readCollapsedGroups = (storageKey: string): Set<string> => {
  try {
    return parseCollapsedGroupIds(window.localStorage.getItem(storageKey))
  } catch {
    return new Set()
  }
}

const ProjectChatSidebar: FC<{ onOpenSession: () => void }> = ({ onOpenSession }) => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const orgId = router.params.org_id || ''
  const storageKey = collapsedGroupsStorageKey(orgId)

  const [query, setQuery] = useState('')
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => readCollapsedGroups(storageKey))
  const [relativeTimeNow, setRelativeTimeNow] = useState(() => Date.now())
  const [archiveConfirmation, setArchiveConfirmation] = useState<SidebarItem | null>(null)
  const [archivingItemId, setArchivingItemId] = useState<string | null>(null)

  useEffect(() => {
    setCollapsedGroups(readCollapsedGroups(storageKey))
  }, [storageKey])

  useEffect(() => {
    const interval = window.setInterval(() => setRelativeTimeNow(Date.now()), RELATIVE_TIME_REFRESH_MS)
    return () => window.clearInterval(interval)
  }, [])

  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId, {
    enabled: !!account.user?.id && !!orgId,
  })
  const archiveSession = useArchiveSession()
  const archiveSpecTask = useArchiveSpecTask()
  const activeItemId = router.params.taskId || router.params.session_id || ''

  const openNewThread = () => {
    account.orgNavigate('chat')
    onOpenSession()
  }

  useEffect(() => {
    const handleNewThreadShortcut = (event: KeyboardEvent) => {
      if (!isNewThreadShortcut(event)) return

      event.preventDefault()
      openNewThread()
    }

    window.addEventListener('keydown', handleNewThreadShortcut, { capture: true })
    return () => window.removeEventListener('keydown', handleNewThreadShortcut, { capture: true })
  }, [orgId])

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
      try {
        window.localStorage.setItem(storageKey, serializeCollapsedGroupIds(next))
      } catch {
        // Persistence is optional when browser storage is unavailable.
      }
      return next
    })
  }

  const performArchive = async (item: SidebarItem) => {
    if (archivingItemId) return
    setArchivingItemId(item.id)
    try {
      if (item.kind === 'spec-task') {
        await archiveSpecTask.mutateAsync({ taskId: item.id, archived: true })
      } else {
        await archiveSession.mutateAsync({ sessionId: item.id, archived: true })
      }
      setArchiveConfirmation(null)
      if (item.id === activeItemId) account.orgNavigate('chat')
    } catch (error: any) {
      const message = typeof error?.response?.data === 'string'
        ? error.response.data
        : error?.response?.data?.message
      snackbar.error(message || `Failed to archive ${item.kind === 'spec-task' ? 'task' : 'chat'}`)
    } finally {
      setArchivingItemId(null)
    }
  }

  const requestArchive = (item: SidebarItem) => {
    if (shouldConfirmTaskArchive(item)) {
      setArchiveConfirmation(item)
      return
    }
    void performArchive(item)
  }

  const effectiveCollapsedGroups = query ? new Set<string>() : collapsedGroups
  const groupsEnabled = !!account.user?.id && !!orgId

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
        <Tooltip title="New thread (⌘N / Ctrl+N)">
          <IconButton
            size="small"
            onClick={openNewThread}
            aria-label="New thread"
            aria-keyshortcuts="Meta+N Control+N"
          >
            <SquarePen size={16} strokeWidth={1.7} />
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
        {projectsLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={22} />
          </Box>
        ) : (
          <>
            <ProjectChatGroup
              orgId={orgId}
              collapsed={effectiveCollapsedGroups.has('default')}
              query={query}
              activeItemId={activeItemId}
              relativeTimeNow={relativeTimeNow}
              enabled={groupsEnabled}
              archivingItemId={archivingItemId}
              onToggle={() => toggleGroup('default')}
              onNewTask={() => account.orgNavigate('chat')}
              onOpenItem={openItem}
              onArchiveItem={requestArchive}
            />
            {projects.flatMap((project) => project.id ? [(
              <ProjectChatGroup
                key={project.id}
                orgId={orgId}
                project={project}
                collapsed={effectiveCollapsedGroups.has(project.id)}
                query={query}
                activeItemId={activeItemId}
                relativeTimeNow={relativeTimeNow}
                enabled={groupsEnabled}
                archivingItemId={archivingItemId}
                onToggle={() => toggleGroup(project.id!)}
                onNewTask={() => account.orgNavigate('chat', {}, { project_id: project.id })}
                onOpenItem={openItem}
                onArchiveItem={requestArchive}
              />
            )] : [])}
          </>
        )}
      </Box>

      {archiveConfirmation && (
        <SimpleConfirmWindow
          title="Archive spec task"
          message={`Archive “${archiveConfirmation.title}”? Any running task agent will be stopped.`}
          confirmTitle={archivingItemId === archiveConfirmation.id ? 'Archiving…' : 'Archive'}
          onCancel={() => {
            if (!archivingItemId) setArchiveConfirmation(null)
          }}
          onSubmit={() => void performArchive(archiveConfirmation)}
        />
      )}
    </Box>
  )
}

export default ProjectChatSidebar
