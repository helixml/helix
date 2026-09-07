import { FC, MouseEvent, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import {
  Bot,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  MessageSquare,
  Play,
  RotateCcw,
  Settings,
  Square,
} from 'lucide-react'

import type { TypesOrganizationMembership, TypesPinnedChat, TypesProject, TypesUser } from '../../api/api'
import useAccount from '../../hooks/useAccount'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useActivateBot,
  useRestartBotAgent,
  useStopBotAgent,
} from '../../services/helixOrgService'
import { useSpecTasks } from '../../services/specTaskService'
import { PRESENCE_OFFLINE_COLOR, PRESENCE_ONLINE_COLOR } from '../widgets/PresenceDot'
import ProjectChatItemRow from './ProjectChatItemRow'
import ProjectChatShowMore from './ProjectChatShowMore'
import {
  buildPersonChatItems,
  filterProjectChatGroups,
  pinnedAtByItemKeyFrom,
  sidebarBotMatchesQuery,
} from './ProjectChatSidebar.logic'
import type { SidebarBot, SidebarItem, SidebarThreadSortOrder } from './ProjectChatSidebar.logic'
import { useSidebarItemPagination, windowSidebarItems } from './useSidebarItemPagination'

export const botGroupId = (botId: string): string => `bot:${botId}`

type BotMenuState = { bot: SidebarBot; mouseX: number; mouseY: number } | null

type ItemRowProps = {
  projects: TypesProject[]
  query: string
  activeItemId: string
  relativeTimeNow: number
  enabled: boolean
  threadSortOrder?: SidebarThreadSortOrder
  visibleThreadCount?: number
  archived?: boolean
  organizationMembers: TypesOrganizationMembership[]
  currentUser?: TypesUser
  pinnedChats?: TypesPinnedChat[]
  archivingItemId: string | null
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

type ProjectChatBotsGroupProps = ItemRowProps & {
  orgId: string
  bots: SidebarBot[]
  collapsedGroups: ReadonlySet<string>
  onToggleBot: (botId: string) => void
  onOpenSession: () => void
}

// Top-level list of the org's agents. Each agent is its own collapsible group
// holding the spec tasks it created; the agent row itself opens its chat
// session (starting the agent first if it has never run), and the menu
// reaches the agent's settings without going through the org chart.
const ProjectChatBotsGroup: FC<ProjectChatBotsGroupProps> = ({
  orgId,
  bots,
  collapsedGroups,
  onToggleBot,
  onOpenSession,
  ...rowProps
}) => {
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  const activateBot = useActivateBot()
  const stopBot = useStopBotAgent()
  const restartBot = useRestartBotAgent()
  const [menu, setMenu] = useState<BotMenuState>(null)
  const [pendingOpenBotId, setPendingOpenBotId] = useState<string | null>(null)
  const [busyBotId, setBusyBotId] = useState<string | null>(null)
  const orgSlug = (router.params.org_id as string) || ''

  const openSession = (sessionId: string) => {
    account.orgNavigate('session', { session_id: sessionId })
    onOpenSession()
  }

  // A freshly started agent gets its session id from the polled bots list, so
  // the open completes reactively once it lands rather than by re-clicking.
  const pendingSessionId = pendingOpenBotId
    ? bots.find((candidate) => candidate.id === pendingOpenBotId)?.sessionId || ''
    : ''
  useEffect(() => {
    if (!pendingSessionId) return
    setPendingOpenBotId(null)
    openSession(pendingSessionId)
    // openSession closes over context objects; the session id is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingSessionId])

  const runBotAction = async (bot: SidebarBot, action: 'start' | 'stop' | 'restart') => {
    setBusyBotId(bot.id)
    try {
      if (action === 'start') {
        await activateBot.mutateAsync(bot.id)
        snackbar.success(`Starting ${bot.name}…`)
      } else if (action === 'stop') {
        await stopBot.mutateAsync(bot.id)
        snackbar.success(`${bot.name} stopped`)
      } else {
        await restartBot.mutateAsync(bot.id)
        snackbar.success(`Restarting ${bot.name}…`)
      }
    } catch (error: any) {
      snackbar.error(error?.response?.data?.error ?? error?.message ?? `Failed to ${action} ${bot.name}`)
      if (action === 'start') setPendingOpenBotId(null)
    } finally {
      setBusyBotId(null)
    }
  }

  const openBot = (bot: SidebarBot) => {
    if (bot.sessionId) {
      openSession(bot.sessionId)
      return
    }
    setPendingOpenBotId(bot.id)
    void runBotAction(bot, 'start')
  }

  const openSettings = (bot: SidebarBot) => {
    if (!orgSlug || !bot.agentAppId) return
    router.navigate('org_agent', { org_id: orgSlug, app_id: bot.agentAppId })
    onOpenSession()
  }

  const openBotPage = (bot: SidebarBot) => {
    if (!orgSlug) return
    router.navigate('helix_org_bot_detail', { org_id: orgSlug, bot_id: bot.id })
    onOpenSession()
  }

  const openMenu = (event: MouseEvent<HTMLElement>, bot: SidebarBot) => {
    event.preventDefault()
    event.stopPropagation()
    setMenu({ bot, mouseX: event.clientX, mouseY: event.clientY })
  }
  const closeMenu = () => setMenu(null)

  const menuIconSize = 15

  return (
    <Box>
      {bots.map((bot) => (
        <ProjectChatBotEntry
          key={bot.id}
          orgId={orgId}
          bot={bot}
          collapsed={collapsedGroups.has(botGroupId(bot.id))}
          busy={busyBotId === bot.id || pendingOpenBotId === bot.id}
          onToggle={() => onToggleBot(bot.id)}
          onOpen={() => openBot(bot)}
          onOpenSettings={() => openSettings(bot)}
          onOpenMenu={(event) => openMenu(event, bot)}
          {...rowProps}
        />
      ))}
      <Menu
        open={!!menu}
        onClose={closeMenu}
        anchorReference="anchorPosition"
        anchorPosition={menu ? { top: menu.mouseY, left: menu.mouseX } : undefined}
      >
        {menu && (
          <MenuItem onClick={() => { closeMenu(); openBot(menu.bot) }}>
            <MessageSquare size={menuIconSize} style={{ marginRight: 10 }} />
            Open chat
          </MenuItem>
        )}
        {menu && (
          <MenuItem disabled={!menu.bot.agentAppId} onClick={() => { closeMenu(); openSettings(menu.bot) }}>
            <Settings size={menuIconSize} style={{ marginRight: 10 }} />
            Agent settings
          </MenuItem>
        )}
        {menu && (menu.bot.running ? (
          <MenuItem onClick={() => { const { bot } = menu; closeMenu(); void runBotAction(bot, 'stop') }}>
            <Square size={menuIconSize} style={{ marginRight: 10 }} />
            Stop agent
          </MenuItem>
        ) : (
          <MenuItem onClick={() => { const { bot } = menu; closeMenu(); void runBotAction(bot, 'start') }}>
            <Play size={menuIconSize} style={{ marginRight: 10 }} />
            Start agent
          </MenuItem>
        ))}
        {menu && (
          <MenuItem onClick={() => { const { bot } = menu; closeMenu(); void runBotAction(bot, 'restart') }}>
            <RotateCcw size={menuIconSize} style={{ marginRight: 10 }} />
            Restart agent
          </MenuItem>
        )}
        {menu && (
          <MenuItem onClick={() => { closeMenu(); openBotPage(menu.bot) }}>
            <ExternalLink size={menuIconSize} style={{ marginRight: 10 }} />
            Agent page
          </MenuItem>
        )}
      </Menu>
    </Box>
  )
}

type ProjectChatBotEntryProps = ItemRowProps & {
  orgId: string
  bot: SidebarBot
  collapsed: boolean
  busy: boolean
  onToggle: () => void
  onOpen: () => void
  onOpenSettings: () => void
  onOpenMenu: (event: MouseEvent<HTMLElement>) => void
}

// One agent: its row plus the spec tasks it created, across every project the
// viewer can read. Tasks only — the agent's own chat is the row itself. A
// search opens the group so its tasks can match, and hides it when neither the
// agent's name nor any task does.
const ProjectChatBotEntry: FC<ProjectChatBotEntryProps> = ({
  orgId,
  bot,
  collapsed,
  busy,
  onToggle,
  onOpen,
  onOpenSettings,
  onOpenMenu,
  projects,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  threadSortOrder = 'updated_at',
  visibleThreadCount = 6,
  archived = false,
  organizationMembers,
  currentUser,
  pinnedChats = [],
  archivingItemId,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const isPhone = useIsPhone()
  const searching = !!query.trim()
  const open = !collapsed || searching
  const pagination = useSidebarItemPagination(visibleThreadCount)
  const tasksQuery = useSpecTasks({
    organizationId: orgId,
    createdByOrgAgent: bot.id,
    limit: pagination.requestCount,
    offset: 0,
    sort: threadSortOrder === 'created_at' ? 'created' : 'last_message',
    archivedOnly: archived,
    enabled: enabled && open && !!orgId,
    refetchInterval: archived ? false : 10000,
  })
  const tasks = tasksQuery.data || []
  const items = buildPersonChatItems(projects, tasks, [], threadSortOrder, pinnedAtByItemKeyFrom(pinnedChats))
  const filteredItems = filterProjectChatGroups([{ id: bot.id, name: bot.name, items }], query)[0]?.items || []
  const renderedItems = windowSidebarItems(filteredItems, activeItemId, pagination.visibleCount)
  const hasMore = filteredItems.length > pagination.visibleCount || tasks.length >= pagination.requestCount
  const active = !!bot.sessionId && bot.sessionId === activeItemId
  const statusTitle = bot.running
    ? (bot.restartRequired ? 'Running · restart required to apply changes' : 'Agent running')
    : 'Agent stopped'

  if (searching && !tasksQuery.isLoading && filteredItems.length === 0 && !sidebarBotMatchesQuery(bot, query)) {
    return null
  }

  return (
    <Box sx={{ mb: 0.25 }}>
      <Box
        // Not a `.project-chat-item`: the Cmd/Ctrl+digit shortcuts number those,
        // and a digit must open a chat, never start an agent.
        className="project-chat-agent"
        role="button"
        tabIndex={0}
        aria-label={`Open chat with ${bot.name}`}
        onClick={onOpen}
        onContextMenu={onOpenMenu}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onOpen()
          }
        }}
        sx={{
          width: '100%',
          minWidth: 0,
          height: 32,
          pl: 0.75,
          pr: 1,
          display: 'flex',
          alignItems: 'center',
          gap: 0.65,
          borderRadius: '6px',
          cursor: 'pointer',
          position: 'relative',
          outline: 'none',
          color: active
            ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
            : (lightTheme.isLight ? '#71717a' : 'rgba(163,163,163,0.80)'),
          backgroundColor: active
            ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
            : 'transparent',
          '&:hover, &:focus-visible': {
            color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
            backgroundColor: active
              ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
              : (lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)'),
          },
          '&:hover .sidebar-bot-settings, &:focus-within .sidebar-bot-settings': { opacity: 1 },
          '@media (hover: none)': { '& .sidebar-bot-settings': { opacity: 1 } },
        }}
      >
        <Box
          component="button"
          type="button"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${bot.name}'s tasks`}
          aria-expanded={open}
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
            width: isPhone ? 28 : 16,
            height: isPhone ? 28 : 16,
            borderRadius: '4px',
            '&:hover': {
              backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(241,243,247,0.12)',
            },
          }}
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </Box>
        <Tooltip title={statusTitle}>
          <Box
            component="span"
            data-bot-status={bot.running ? 'running' : 'stopped'}
            onMouseOver={(event) => event.stopPropagation()}
            sx={{
              width: 7,
              height: 7,
              borderRadius: '50%',
              flexShrink: 0,
              backgroundColor: bot.running ? PRESENCE_ONLINE_COLOR : PRESENCE_OFFLINE_COLOR,
              boxShadow: bot.running && bot.restartRequired ? '0 0 0 2px rgba(251,191,36,0.55)' : 'none',
            }}
          />
        </Tooltip>
        <Bot size={14} style={{ flexShrink: 0, opacity: 0.8 }} />
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
          {bot.name}
        </Typography>
        {busy ? (
          <CircularProgress size={12} color="inherit" sx={{ mr: 0.5 }} />
        ) : (
          <Tooltip title="Agent settings">
            <span>
              <IconButton
                className="sidebar-bot-settings"
                size="small"
                aria-label={`Settings for ${bot.name}`}
                disabled={!bot.agentAppId}
                onMouseOver={(event) => event.stopPropagation()}
                onClick={(event) => {
                  event.stopPropagation()
                  onOpenSettings()
                }}
                sx={{ width: 24, height: 24, opacity: 0, color: 'inherit', transition: 'opacity 100ms ease' }}
              >
                <Settings size={14} />
              </IconButton>
            </span>
          </Tooltip>
        )}
      </Box>
      {open && tasksQuery.isError && (
        <Typography color="error" sx={{ pl: 2.15, py: 0.5, fontSize: '0.7rem' }}>
          Failed to load tasks
        </Typography>
      )}
      {/* An agent with nothing to show keeps a quiet row rather than an empty
          "no tasks" line under every idle agent. */}
      {open && renderedItems.length > 0 && (
        <Box sx={{ pl: 1.15 }}>
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
              projectName={item.projectName}
              onOpenItem={onOpenItem}
              onOpenItemContextMenu={onOpenItemContextMenu}
              onArchiveItem={onArchiveItem}
            />
          ))}
          <ProjectChatShowMore pagination={pagination} hasMore={hasMore} fetching={tasksQuery.isFetching} />
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatBotsGroup
