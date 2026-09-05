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
  ExternalLink,
  MessageSquare,
  Play,
  RotateCcw,
  Settings,
  Square,
} from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useActivateBot,
  useRestartBotAgent,
  useStopBotAgent,
} from '../../services/helixOrgService'
import { PRESENCE_OFFLINE_COLOR, PRESENCE_ONLINE_COLOR } from '../widgets/PresenceDot'
import type { SidebarBot } from './ProjectChatSidebar.logic'

type ProjectChatBotsGroupProps = {
  bots: SidebarBot[]
  activeItemId: string
  onOpenSession: () => void
}

type BotMenuState = { bot: SidebarBot; mouseX: number; mouseY: number } | null

// Top-level list of the org's agents. Clicking one opens its chat session
// (starting the agent first if it has never run); the menu reaches the agent's
// settings without going through the org chart.
const ProjectChatBotsGroup: FC<ProjectChatBotsGroupProps> = ({ bots, activeItemId, onOpenSession }) => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
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
  useEffect(() => {
    if (!pendingOpenBotId) return
    const bot = bots.find((candidate) => candidate.id === pendingOpenBotId)
    if (!bot?.sessionId) return
    setPendingOpenBotId(null)
    openSession(bot.sessionId)
    // openSession closes over context objects; the bots list is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bots, pendingOpenBotId])

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
    <Box sx={{ pl: 0.4 }}>
      {bots.map((bot) => {
        const active = !!bot.sessionId && bot.sessionId === activeItemId
        const busy = busyBotId === bot.id || pendingOpenBotId === bot.id
        const statusTitle = bot.running
          ? (bot.restartRequired ? 'Running · restart required to apply changes' : 'Agent running')
          : 'Agent stopped'
        return (
          <Box
            key={bot.id}
            className="project-chat-item"
            role="button"
            tabIndex={0}
            aria-label={`Open chat with ${bot.name}`}
            onClick={() => openBot(bot)}
            onContextMenu={(event) => openMenu(event, bot)}
            onKeyDown={(event) => {
              if (event.target !== event.currentTarget) return
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                openBot(bot)
              }
            }}
            sx={{
              width: '100%',
              minWidth: 0,
              height: 32,
              px: 1,
              display: 'flex',
              alignItems: 'center',
              gap: 0.75,
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
                      openSettings(bot)
                    }}
                    sx={{ width: 24, height: 24, opacity: 0, color: 'inherit', transition: 'opacity 100ms ease' }}
                  >
                    <Settings size={14} />
                  </IconButton>
                </span>
              </Tooltip>
            )}
          </Box>
        )
      })}
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

export default ProjectChatBotsGroup
