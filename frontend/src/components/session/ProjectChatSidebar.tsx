import { FC, MouseEvent, useCallback, useEffect, useRef, useState } from 'react'
import {
  closestCenter,
  DndContext,
} from '@dnd-kit/core'
import {
  SortableContext,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import InputBase from '@mui/material/InputBase'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Archive, FolderPlus, Search, SquarePen } from 'lucide-react'

import {
  TypesExternalRepositoryType,
  TypesGitRepositoryType,
} from '../../api/api'
import type { TypesAzureDevOps, TypesGitRepository, TypesProject } from '../../api/api'
import useAccount from '../../hooks/useAccount'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import { useSettingsDialog } from '../../contexts/settingsDialog'
import { useCreateGitRepository, useGitRepositories } from '../../services/gitRepositoryService'
import { useListHelixOrgBots } from '../../services/helixOrgService'
import { useListProjects } from '../../services/projectService'
import { useArchiveSession } from '../../services/sessionService'
import { useArchiveSpecTask } from '../../services/specTaskService'
import { usePinnedChats } from '../../services/chatPinService'
import CreateProjectDialog from '../project/CreateProjectDialog'
import SimpleConfirmWindow from '../widgets/SimpleConfirmWindow'
import {
  ALL_PROJECTS_FILTER,
  collapsedGroupsStorageKey,
  getChatShortcutNumber,
  isChatShortcutModifier,
  isNewThreadShortcut,
  parseSidebarParticipantIds,
  parseSidebarProjectFilter,
  resolveSidebarProjectFilter,
  shouldConfirmArchive,
  parseCollapsedGroupIds,
  serializeSidebarParticipantIds,
  sidebarPreferencesStorageKey,
  sidebarPeopleFilterStorageKey,
  sidebarProjectFilterStorageKey,
  serializeCollapsedGroupIds,
} from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'
import ProjectChatGroup from './ProjectChatGroup'
import ProjectChatItemContextMenu from './ProjectChatItemContextMenu'
import type { ProjectChatContextMenuPosition } from './ProjectChatItemContextMenu'
import ProjectChatProjectContextMenu from './ProjectChatProjectContextMenu'
import ProjectChatSidebarPeopleFilter from './ProjectChatSidebarPeopleFilter'
import ProjectChatSidebarOptions from './ProjectChatSidebarOptions'
import ProjectChatSidebarProjectFilter from './ProjectChatSidebarProjectFilter'
import SortableProject from './SortableProject'
import useProjectChatSidebarDrag from './useProjectChatSidebarDrag'
import useProjectChatSidebarPreferences from './useProjectChatSidebarPreferences'
import ChatSidebarBrandHeader from './ChatSidebarBrandHeader'
import NewChatProjectDialog from './NewChatProjectDialog'
import type { NewChatTarget } from './NewChatProjectDialog'
import ProjectChatSidebarMobileBar, { MOBILE_BAR_CLEARANCE } from './ProjectChatSidebarMobileBar'

const RELATIVE_TIME_REFRESH_MS = 15000
const T3_FONT_FAMILY = '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif'

const readCollapsedGroups = (storageKey: string): Set<string> => {
  try {
    return parseCollapsedGroupIds(window.localStorage.getItem(storageKey))
  } catch {
    return new Set()
  }
}

const readParticipantIds = (storageKey: string): string[] | null => {
  try {
    return parseSidebarParticipantIds(window.localStorage.getItem(storageKey))
  } catch {
    return null
  }
}

const readProjectFilter = (storageKey: string): string => {
  try {
    return parseSidebarProjectFilter(window.localStorage.getItem(storageKey))
  } catch {
    return ALL_PROJECTS_FILTER
  }
}

const ProjectChatSidebar: FC<{
  onCollapse: () => void
  onOpenSession: () => void
}> = ({ onCollapse, onOpenSession }) => {
  const account = useAccount()
  const router = useRouter()
  const isPhone = useIsPhone()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const { openDialog } = useSettingsDialog()
  const orgSlug = router.params.org_id || ''
  const orgId = account.organizationTools.organization?.id || ''
  const currentUserId = account.user?.id || ''
  const storageKey = collapsedGroupsStorageKey(orgSlug)
  const preferencesStorageKey = sidebarPreferencesStorageKey(orgSlug)
  const projectFilterStorageKey = sidebarProjectFilterStorageKey(orgId)

  const [query, setQuery] = useState('')
  const [projectFilter, setProjectFilter] = useState(() => readProjectFilter(projectFilterStorageKey))
  const peopleFilterStorageKey = sidebarPeopleFilterStorageKey(currentUserId, orgSlug, projectFilter)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => readCollapsedGroups(storageKey))
  const [relativeTimeNow, setRelativeTimeNow] = useState(() => Date.now())
  const [archiveConfirmation, setArchiveConfirmation] = useState<SidebarItem | null>(null)
  const [contextMenuItem, setContextMenuItem] = useState<SidebarItem | null>(null)
  const [contextMenuPosition, setContextMenuPosition] = useState<ProjectChatContextMenuPosition | null>(null)
  const [projectContextMenuProject, setProjectContextMenuProject] = useState<TypesProject | null>(null)
  const [projectContextMenuPosition, setProjectContextMenuPosition] = useState<ProjectChatContextMenuPosition | null>(null)
  const [archivingItemId, setArchivingItemId] = useState<string | null>(null)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)
  const [newChatPickerOpen, setNewChatPickerOpen] = useState(false)
  const [showArchived, setShowArchived] = useState(false)
  const [chatShortcutsVisible, setChatShortcutsVisible] = useState(false)
  const sidebarRef = useRef<HTMLDivElement | null>(null)
  const chatShortcutModifierHeldRef = useRef(false)
  const [participantIdsOverride, setParticipantIdsOverride] = useState<string[] | null>(() => (
    readParticipantIds(peopleFilterStorageKey)
  ))

  useEffect(() => {
    setCollapsedGroups(readCollapsedGroups(storageKey))
  }, [storageKey])

  useEffect(() => {
    setProjectFilter(readProjectFilter(projectFilterStorageKey))
  }, [projectFilterStorageKey])

  useEffect(() => {
    setParticipantIdsOverride(readParticipantIds(peopleFilterStorageKey))
  }, [peopleFilterStorageKey])

  useEffect(() => {
    const interval = window.setInterval(() => setRelativeTimeNow(Date.now()), RELATIVE_TIME_REFRESH_MS)
    return () => window.clearInterval(interval)
  }, [])

  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId, {
    enabled: !!account.user?.id && !!orgId,
    refetchInterval: 10000,
  })
  const {
    preferences,
    sortedProjects,
    setProjectSortOrder,
    setThreadSortOrder,
    setVisibleThreadCount,
    setManualProjectOrder,
  } = useProjectChatSidebarPreferences(preferencesStorageKey, projects)
  const sidebarProjects = projects
  const focusedProject = projectFilter === ALL_PROJECTS_FILTER
    ? undefined
    : sidebarProjects.find((project) => project.id === projectFilter)
  const resolvedProjectFilter = resolveSidebarProjectFilter(projectFilter, sidebarProjects)
  const focusMode = projectFilter !== ALL_PROJECTS_FILTER && !!focusedProject
  const displayedProjects = focusMode && focusedProject
    ? [focusedProject]
    : sortedProjects

  useEffect(() => {
    if (projectsLoading || resolvedProjectFilter === projectFilter) return
    setProjectFilter(resolvedProjectFilter)
    try {
      window.localStorage.setItem(projectFilterStorageKey, resolvedProjectFilter)
    } catch {
      // Persistence is optional when browser storage is unavailable.
    }
  }, [projectFilter, projectFilterStorageKey, projectsLoading, resolvedProjectFilter])
  const { data: orgAgents = [] } = useListHelixOrgBots({
    enabled: !!account.user?.id && !!orgId,
  })
  const orgAgentAppIds = new Set(orgAgents.flatMap((agent) => [
    agent.agent_id,
    agent.agent_app_id,
  ]).filter((appId): appId is string => !!appId))
  const { data: repositories = [], isLoading: repositoriesLoading } = useGitRepositories({
    organizationId: orgId,
    enabled: createProjectOpen && !!account.user?.id && !!orgId,
  })
  const createGitRepository = useCreateGitRepository()
  const archiveSession = useArchiveSession()
  const archiveSpecTask = useArchiveSpecTask()
  const { data: pinnedChats = [] } = usePinnedChats(!!account.user?.id)
  const activeItemId = router.params.taskId || router.params.session_id || ''
  const organizationMembers = account.organizationTools.organization?.memberships || []
  const memberUserIds = new Set(organizationMembers.flatMap((member) => (
    member.user_id && member.user ? [member.user_id] : []
  )))
  const selectableMembers = currentUserId && !memberUserIds.has(currentUserId) && account.user
    ? [{ user_id: currentUserId, user: account.user }, ...organizationMembers]
    : organizationMembers
  const selectedParticipantIds = participantIdsOverride === null
    ? currentUserId ? [currentUserId] : []
    : participantIdsOverride.filter((userId) => userId === currentUserId || memberUserIds.has(userId))
  const {
    dragInProgressRef,
    suppressClickAfterDragRef,
    sensors: projectDragSensors,
    onDragStart: handleProjectDragStart,
    onDragCancel: handleProjectDragCancel,
    onDragEnd: handleProjectDragEnd,
  } = useProjectChatSidebarDrag(
    preferences.projectSortOrder,
    query,
    sortedProjects,
    setManualProjectOrder,
  )

  // Which project a chat belongs to is its first real decision, and picking it
  // up front doubles as the quickest way to move between projects — so the
  // compose affordances open the picker rather than dropping you into whichever
  // project you happened to be looking at.
  const openNewChatPicker = useCallback(() => setNewChatPickerOpen(true), [])

  const startNewChat = useCallback(({ projectId }: NewChatTarget) => {
    setShowArchived(false)
    account.orgNavigate('chat', {}, projectId ? { project_id: projectId } : {})
    onOpenSession()
  }, [account, onOpenSession])

  // openNewChatPicker is the only dependency; keeping it in the array (rather
  // than a route id) is what stops the listener from calling a stale closure.
  useEffect(() => {
    const handleNewThreadShortcut = (event: KeyboardEvent) => {
      if (!isNewThreadShortcut(event)) return

      event.preventDefault()
      openNewChatPicker()
    }

    window.addEventListener('keydown', handleNewThreadShortcut, { capture: true })
    return () => window.removeEventListener('keydown', handleNewThreadShortcut, { capture: true })
  }, [openNewChatPicker])

  useEffect(() => {
    const sidebar = sidebarRef.current
    if (!sidebar) return

    const assignShortcuts = () => {
      const items = sidebar.querySelectorAll<HTMLElement>('.project-chat-item')
      items.forEach((item, index) => {
        if (index < 9) item.dataset.chatShortcut = String(index + 1)
        else delete item.dataset.chatShortcut
      })
    }
    assignShortcuts()
    const observer = new MutationObserver(assignShortcuts)
    observer.observe(sidebar, { childList: true, subtree: true })
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    const isMac = /Mac|iPhone|iPad|iPod/.test(navigator.platform)
    const hideShortcuts = () => {
      chatShortcutModifierHeldRef.current = false
      setChatShortcutsVisible(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      const modifierKey = isMac ? 'Meta' : 'Control'
      if (event.key === modifierKey) {
        chatShortcutModifierHeldRef.current = true
        setChatShortcutsVisible(true)
        return
      }
      const shortcutNumber = getChatShortcutNumber(
        event,
        isMac,
        chatShortcutModifierHeldRef.current,
      )
      if (shortcutNumber !== null) {
        const item = sidebarRef.current?.querySelector<HTMLElement>(
          `.project-chat-item[data-chat-shortcut="${shortcutNumber}"]`,
        )
        if (!item) return
        event.preventDefault()
        event.stopPropagation()
        item.click()
        return
      }
      if (isChatShortcutModifier(event, isMac)) {
        chatShortcutModifierHeldRef.current = true
        setChatShortcutsVisible(true)
      }
    }
    const handleKeyUp = (event: KeyboardEvent) => {
      if ((isMac && event.key === 'Meta') || (!isMac && event.key === 'Control')) hideShortcuts()
    }

    window.addEventListener('keydown', handleKeyDown, { capture: true })
    window.addEventListener('keyup', handleKeyUp, { capture: true })
    window.addEventListener('blur', hideShortcuts)
    return () => {
      window.removeEventListener('keydown', handleKeyDown, { capture: true })
      window.removeEventListener('keyup', handleKeyUp, { capture: true })
      window.removeEventListener('blur', hideShortcuts)
    }
  }, [])

  const createRepository = async (
    name: string,
    description: string,
  ): Promise<TypesGitRepository | null> => {
    if (!account.user?.id || !orgId) return null

    try {
      const repository = await createGitRepository.mutateAsync({
        name,
        description,
        owner_id: account.user.id,
        organization_id: orgId,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: 'main',
      })
      return repository || null
    } catch (error) {
      console.error('Failed to create repository:', error)
      return null
    }
  }

  const linkRepository = async (
    url: string,
    name: string,
    type: TypesExternalRepositoryType,
    username?: string,
    password?: string,
    azureDevOps?: TypesAzureDevOps,
    oauthConnectionId?: string,
    gitProviderConnectionId?: string,
  ): Promise<TypesGitRepository | null> => {
    if (!account.user?.id || !orgId) return null

    try {
      const repository = await createGitRepository.mutateAsync({
        name,
        description: `External ${type} repository`,
        owner_id: account.user.id,
        organization_id: orgId,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: 'main',
        is_external: true,
        external_url: url,
        external_type: type,
        username,
        password,
        azure_devops: azureDevOps,
        oauth_connection_id: oauthConnectionId,
        git_provider_connection_id: gitProviderConnectionId,
      })
      return repository || null
    } catch (error: any) {
      const message = error?.response?.data?.message
        || error?.response?.data
        || error?.message
        || 'Failed to link repository'
      throw new Error(typeof message === 'string' ? message : JSON.stringify(message))
    }
  }

  const openItem = (item: SidebarItem) => {
    if (item.kind === 'spec-task' && item.projectId) {
      account.orgNavigate('chat-task', { id: item.projectId, taskId: item.id })
    } else {
      account.orgNavigate('session', { session_id: item.id })
    }
    onOpenSession()
  }

  const openItemContextMenu = (event: MouseEvent<HTMLElement>, item: SidebarItem) => {
    event.preventDefault()
    event.stopPropagation()
    setContextMenuItem(item)
    setContextMenuPosition({ mouseX: event.clientX, mouseY: event.clientY })
  }

  const closeItemContextMenu = () => {
    setContextMenuItem(null)
    setContextMenuPosition(null)
  }

  const openProjectContextMenu = (event: MouseEvent<HTMLElement>, project: TypesProject) => {
    event.preventDefault()
    event.stopPropagation()
    setProjectContextMenuProject(project)
    setProjectContextMenuPosition({ mouseX: event.clientX, mouseY: event.clientY })
  }

  const closeProjectContextMenu = () => {
    setProjectContextMenuProject(null)
    setProjectContextMenuPosition(null)
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
    // In the Archived view the same control restores the item instead.
    const archived = !showArchived
    const label = item.kind === 'spec-task' ? 'task' : 'chat'
    setArchivingItemId(item.id)
    try {
      if (item.kind === 'spec-task') {
        await archiveSpecTask.mutateAsync({ taskId: item.id, archived })
      } else {
        await archiveSession.mutateAsync({ sessionId: item.id, archived })
      }
      setArchiveConfirmation(null)
      if (archived && item.id === activeItemId) account.orgNavigate('chat')
    } catch (error: any) {
      const message = typeof error?.response?.data === 'string'
        ? error.response.data
        : error?.response?.data?.message
      snackbar.error(message || `Failed to ${archived ? 'archive' : 'restore'} ${label}`)
    } finally {
      setArchivingItemId(null)
    }
  }

  const requestArchive = (item: SidebarItem) => {
    if (shouldConfirmArchive(item, orgAgentAppIds, showArchived)) {
      setArchiveConfirmation(item)
      return
    }
    void performArchive(item)
  }

  const updateSelectedParticipantIds = (userIds: string[]) => {
    const selectedUserIds = userIds.filter((userId) => (
      userId === currentUserId || memberUserIds.has(userId)
    ))
    setParticipantIdsOverride(selectedUserIds)
    try {
      window.localStorage.setItem(
        peopleFilterStorageKey,
        serializeSidebarParticipantIds(selectedUserIds),
      )
    } catch {
      // Persistence is optional when browser storage is unavailable.
    }
  }

  const selectProjectFilter = (projectId: string) => {
    setProjectFilter(projectId)
    if (projectId !== ALL_PROJECTS_FILTER) {
      setCollapsedGroups((current) => {
        if (!current.has(projectId)) return current
        const next = new Set(current)
        next.delete(projectId)
        try {
          window.localStorage.setItem(storageKey, serializeCollapsedGroupIds(next))
        } catch {
          // Persistence is optional when browser storage is unavailable.
        }
        return next
      })
    }
    try {
      window.localStorage.setItem(projectFilterStorageKey, projectId)
    } catch {
      // Persistence is optional when browser storage is unavailable.
    }
  }

  // Starting a chat has its own button in the phone's bottom bar, so a project
  // row there does not need to offer it a second time. Without a "new task"
  // handler the row goes back to collapsing the group — which is the only thing
  // left for it to do, and what the chevron beside it already suggests.
  const groupsOfferNewTask = !showArchived && !isPhone

  const effectiveCollapsedGroups = query ? new Set<string>() : collapsedGroups
  // The desktop toolbar's controls, reused verbatim in the phone's filter sheet
  // so the two surfaces cannot offer different filters.
  const filterControls = (
    <>
        <ProjectChatSidebarProjectFilter
          projects={sidebarProjects}
          selectedProjectId={projectFilter}
          archived={showArchived}
          onChange={selectProjectFilter}
        />
        {!focusMode && (
          <ProjectChatSidebarOptions
            projectSortOrder={preferences.projectSortOrder}
            threadSortOrder={preferences.threadSortOrder}
            visibleThreadCount={preferences.visibleThreadCount}
            onProjectSortOrderChange={setProjectSortOrder}
            onThreadSortOrderChange={setThreadSortOrder}
            onVisibleThreadCountChange={setVisibleThreadCount}
          />
        )}
        <ProjectChatSidebarPeopleFilter
          members={selectableMembers}
          currentUser={account.user}
          selectedUserIds={selectedParticipantIds}
          onSelectedUserIdsChange={updateSelectedParticipantIds}
        />
        <Tooltip title={showArchived ? 'Back to active chats' : 'Show archived'}>
          <IconButton
            size="small"
            onClick={() => setShowArchived((current) => !current)}
            aria-label={showArchived ? 'Back to active chats' : 'Show archived'}
            aria-pressed={showArchived}
            sx={{
              color: showArchived
                ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
                : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
            }}
          >
            <Archive size={15} strokeWidth={1.7} />
          </IconButton>
        </Tooltip>
        {!showArchived && (
          <Tooltip title="New project">
            <span>
              <IconButton
                size="small"
                onClick={() => setCreateProjectOpen(true)}
                disabled={!account.user?.id || !orgId}
                aria-label="New project"
                sx={{
                  color: lightTheme.isLight
                    ? 'rgba(113,113,122,0.65)'
                    : 'rgba(163,163,163,0.55)',
                }}
              >
                <FolderPlus size={15} strokeWidth={1.7} />
              </IconButton>
            </span>
          </Tooltip>
        )}
    </>
  )
  const groupsEnabled = !!account.user?.id && !!orgId

  return (
    <Box
      ref={sidebarRef}
      data-chat-shortcuts-visible={chatShortcutsVisible}
      sx={{
        height: '100%',
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        // Positioning context for the phone's floating bottom bar.
        position: 'relative',
        fontFamily: T3_FONT_FAMILY,
        color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
        backgroundColor: lightTheme.isLight ? '#fafafa' : '#000000',
        '& .MuiTypography-root': { fontFamily: 'inherit' },
        '&[data-chat-shortcuts-visible="true"] .project-chat-item[data-chat-shortcut]::after': {
          content: 'attr(data-chat-shortcut)',
          position: 'absolute',
          right: 42,
          top: 7,
          width: 14,
          height: 18,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: '4px',
          color: lightTheme.isLight ? '#52525b' : '#d4d4d8',
          backgroundColor: lightTheme.isLight ? 'rgba(39,39,42,0.08)' : 'rgba(241,243,247,0.12)',
          fontSize: '10px',
          fontWeight: 600,
          lineHeight: 1,
          fontVariantNumeric: 'tabular-nums',
          pointerEvents: 'none',
          zIndex: 1,
        },
        '&[data-chat-shortcuts-visible="true"] .project-chat-item[data-chat-shortcut] .sidebar-item-pin': {
          opacity: 0,
        },
      }}
    >
      <ChatSidebarBrandHeader onCollapse={onCollapse} />

      {!isPhone && (
        <>
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
          <Tooltip title="New thread (⌘⇧O / Ctrl+Shift+O)">
            <IconButton
              size="small"
              onClick={openNewChatPicker}
              aria-label="New thread"
              aria-keyshortcuts="Meta+Shift+O Control+Shift+O"
            >
              <SquarePen size={16} strokeWidth={1.7} />
            </IconButton>
          </Tooltip>
        </Box>
        <Box sx={{ pl: 0.75, pr: 0.75, pt: 1.25, pb: 0.5, display: 'flex', alignItems: 'center' }}>
          <ProjectChatSidebarProjectFilter
            projects={sidebarProjects}
            selectedProjectId={projectFilter}
            archived={showArchived}
            onChange={selectProjectFilter}
          />
          {!focusMode && (
            <ProjectChatSidebarOptions
              projectSortOrder={preferences.projectSortOrder}
              threadSortOrder={preferences.threadSortOrder}
              visibleThreadCount={preferences.visibleThreadCount}
              onProjectSortOrderChange={setProjectSortOrder}
              onThreadSortOrderChange={setThreadSortOrder}
              onVisibleThreadCountChange={setVisibleThreadCount}
            />
          )}
          <ProjectChatSidebarPeopleFilter
            members={selectableMembers}
            currentUser={account.user}
            selectedUserIds={selectedParticipantIds}
            onSelectedUserIdsChange={updateSelectedParticipantIds}
          />
          <Tooltip title={showArchived ? 'Back to active chats' : 'Show archived'}>
            <IconButton
              size="small"
              onClick={() => setShowArchived((current) => !current)}
              aria-label={showArchived ? 'Back to active chats' : 'Show archived'}
              aria-pressed={showArchived}
              sx={{
                color: showArchived
                  ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
                  : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
              }}
            >
              <Archive size={15} strokeWidth={1.7} />
            </IconButton>
          </Tooltip>
          {!showArchived && (
            <Tooltip title="New project">
              <span>
                <IconButton
                  size="small"
                  onClick={() => setCreateProjectOpen(true)}
                  disabled={!account.user?.id || !orgId}
                  aria-label="New project"
                  sx={{
                    color: lightTheme.isLight
                      ? 'rgba(113,113,122,0.65)'
                      : 'rgba(163,163,163,0.55)',
                  }}
                >
                  <FolderPlus size={15} strokeWidth={1.7} />
                </IconButton>
              </span>
            </Tooltip>
          )}
        </Box>
        </>
      )}

      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          overflowY: 'auto',
          // Momentum scrolling, and a floor the floating bar cannot cover.
          WebkitOverflowScrolling: 'touch',
          overscrollBehavior: 'contain',
          px: 0.75,
          pb: isPhone ? `${MOBILE_BAR_CLEARANCE}px` : 1.5,
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
            {!focusMode && <ProjectChatGroup
              orgId={orgId}
              collapsed={effectiveCollapsedGroups.has('default')}
              query={query}
              activeItemId={activeItemId}
              relativeTimeNow={relativeTimeNow}
              enabled={groupsEnabled}
              threadSortOrder={preferences.threadSortOrder}
              visibleThreadCount={preferences.visibleThreadCount}
              participantIds={selectedParticipantIds}
              organizationMembers={selectableMembers}
              currentUser={account.user}
              showTaskAvatars={selectedParticipantIds.some((userId) => userId !== currentUserId)}
              archived={showArchived}
              pinnedChats={pinnedChats}
              archivingItemId={archivingItemId}
              onToggle={() => toggleGroup('default')}
              onNewTask={groupsOfferNewTask ? () => account.orgNavigate('chat') : undefined}
              onOpenItem={openItem}
              onOpenItemContextMenu={openItemContextMenu}
              onArchiveItem={requestArchive}
            />}
            <DndContext
              sensors={projectDragSensors}
              collisionDetection={closestCenter}
              onDragStart={handleProjectDragStart}
              onDragCancel={handleProjectDragCancel}
              onDragEnd={handleProjectDragEnd}
            >
              <SortableContext
                items={displayedProjects.flatMap((project) => project.id ? [project.id] : [])}
                strategy={verticalListSortingStrategy}
              >
                {displayedProjects.flatMap((project) => project.id ? [(
                  <SortableProject
                    key={project.id}
                    projectId={project.id}
                    disabled={focusMode || preferences.projectSortOrder !== 'manual' || !!query}
                    render={(dragHandleProps) => (
                      <ProjectChatGroup
                        orgId={orgId}
                        project={project}
                        collapsed={effectiveCollapsedGroups.has(project.id!)}
                        query={query}
                        activeItemId={activeItemId}
                        relativeTimeNow={relativeTimeNow}
                        enabled={groupsEnabled}
                        threadSortOrder={preferences.threadSortOrder}
                        visibleThreadCount={preferences.visibleThreadCount}
                        participantIds={selectedParticipantIds}
                        organizationMembers={selectableMembers}
                        currentUser={account.user}
                        showTaskAvatars={selectedParticipantIds.some((userId) => userId !== currentUserId)}
                        archived={showArchived}
                        pinnedChats={pinnedChats}
                        archivingItemId={archivingItemId}
                        onToggle={() => toggleGroup(project.id!)}
                        onNewTask={groupsOfferNewTask
                          ? () => account.orgNavigate('chat', {}, { project_id: project.id })
                          : undefined}
                        onOpenItem={openItem}
                        onOpenItemContextMenu={openItemContextMenu}
                        onOpenProjectContextMenu={openProjectContextMenu}
                        onArchiveItem={requestArchive}
                        manualSorting={!focusMode && preferences.projectSortOrder === 'manual' && !query}
                        dragHandleProps={dragHandleProps}
                        dragInProgressRef={dragInProgressRef}
                        suppressClickAfterDragRef={suppressClickAfterDragRef}
                      />
                    )}
                  />
                )] : [])}
              </SortableContext>
            </DndContext>
          </>
        )}
      </Box>

      {isPhone && (
        <ProjectChatSidebarMobileBar
          query={query}
          onQueryChange={setQuery}
          onNewChat={openNewChatPicker}
          filters={filterControls}
        />
      )}

      <NewChatProjectDialog
        open={newChatPickerOpen}
        projects={projects}
        onClose={() => setNewChatPickerOpen(false)}
        onSelect={startNewChat}
      />

      <ProjectChatItemContextMenu
        item={contextMenuItem}
        position={contextMenuPosition}
        onClose={closeItemContextMenu}
      />

      <ProjectChatProjectContextMenu
        project={projectContextMenuProject}
        position={projectContextMenuPosition}
        onClose={closeProjectContextMenu}
        onOpenBoard={(project) => {
          if (!project.id) return
          account.orgNavigate('project-specs', { id: project.id })
          onOpenSession()
        }}
        onOpenSettings={(project) => {
          if (!project.id) return
          openDialog('project-settings', { projectId: project.id })
        }}
      />

      {archiveConfirmation && (
        <SimpleConfirmWindow
          title={archiveConfirmation.kind === 'spec-task' ? 'Archive spec task' : 'Archive chat'}
          // Only reached when archiving really does stop an agent — see
          // shouldConfirmArchive. Plain chats archive without a prompt.
          message={`Archive “${archiveConfirmation.title}”? ${archiveConfirmation.kind === 'spec-task'
            ? 'Any running task agent will be stopped.'
            : 'Its external agent will be stopped.'} You can restore it from the Archived view.`}
          confirmTitle={archivingItemId === archiveConfirmation.id ? 'Archiving…' : 'Archive'}
          onCancel={() => {
            if (!archivingItemId) setArchiveConfirmation(null)
          }}
          onSubmit={() => void performArchive(archiveConfirmation)}
        />
      )}

      {createProjectOpen && (
        <CreateProjectDialog
          open
          onClose={() => setCreateProjectOpen(false)}
          onSuccess={(projectId) => {
            account.orgNavigate('chat', {}, { project_id: projectId })
            onOpenSession()
          }}
          repositories={repositories}
          reposLoading={repositoriesLoading}
          onCreateRepo={createRepository}
          onLinkRepo={linkRepository}
        />
      )}
    </Box>
  )
}

export default ProjectChatSidebar
