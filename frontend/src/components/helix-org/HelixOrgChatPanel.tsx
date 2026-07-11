// Left-rail chat for the helix-org shell — same building blocks as the
// spec-task chat (EmbeddedSessionView + RobustPromptInput), scoped to one
// bot at a time. Header shows which bot you are talking to.

import { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import StopIcon from '@mui/icons-material/Stop'

import RobustPromptInput from '../common/RobustPromptInput'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import SessionPromptQueue from '../session/SessionPromptQueue'
import EmbeddedSessionView, {
  EmbeddedSessionViewHandle,
} from '../session/EmbeddedSessionView'
import useApi from '../../hooks/useApi'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import { useStreaming } from '../../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../../types'
import {
  BotDTO,
  useActivateBot,
  useHelixOrgBot,
  useListHelixOrgBots,
  useRestartBotAgent,
  useStopBotAgent,
} from '../../services/helixOrgService'
import {
  WorkerChatReader,
  fetchExistingWorkerSession,
} from '../../services/workerChatSession'

const storageKey = (orgId: string) => `helix-org-chat-bot:${orgId}`

const HelixOrgChatPanel: FC = () => {
  const lightTheme = useLightTheme()
  const router = useRouter()
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const orgId = (router.params.org_id as string) || ''

  const { data: botsData } = useListHelixOrgBots({ refetchInterval: 5000 })
  const agents = useMemo(
    () => (botsData ?? []).filter((b: BotDTO) => b.kind !== 'human' && b.id),
    [botsData],
  )

  const [selectedBotId, setSelectedBotId] = useState<string>('')
  // Restore last-used bot (or pick a sensible default once the list loads).
  useEffect(() => {
    if (agents.length === 0) return
    const saved = orgId ? localStorage.getItem(storageKey(orgId)) : null
    if (saved && agents.some((b) => b.id === saved)) {
      setSelectedBotId(saved)
      return
    }
    if (!selectedBotId || !agents.some((b) => b.id === selectedBotId)) {
      const preferred =
        agents.find((b) => (b.id ?? '').includes('chief') || (b.id ?? '').includes('owner'))
        ?? agents[0]
      setSelectedBotId(preferred.id ?? '')
    }
  }, [agents, orgId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!orgId || !selectedBotId) return
    localStorage.setItem(storageKey(orgId), selectedBotId)
  }, [orgId, selectedBotId])

  const selectedBot = agents.find((b) => b.id === selectedBotId)
  const agentOnline = selectedBot?.agent_status === 'running'

  // Project + session for the selected bot (detail endpoint carries project_id).
  const { data: botDetail, refetch: refetchBot } = useHelixOrgBot(selectedBotId || undefined, {
    enabled: !!selectedBotId,
  })
  const projectID = botDetail?.project_id
  const activateAgent = useActivateBot()
  const stopAgent = useStopBotAgent()
  const restartAgent = useRestartBotAgent()

  const [chatSessionId, setChatSessionId] = useState<string | null>(null)
  const [view, setView] = useState<'chat' | 'desktop'>('chat')
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)

  const chatApi: WorkerChatReader = useMemo(() => ({
    getExploratorySession: async (pid: string) => {
      try {
        const resp = await api.getApiClient().v1ProjectsExploratorySessionDetail(pid)
        return resp.data ?? null
      } catch (err: any) {
        if (err?.response?.status === 204) return null
        throw err
      }
    },
  }), [api])

  useEffect(() => {
    let cancelled = false
    setChatSessionId(null)
    if (!projectID) return
    fetchExistingWorkerSession(projectID, chatApi)
      .then((sid) => { if (!cancelled) setChatSessionId(sid) })
      .catch(() => { if (!cancelled) setChatSessionId(null) })
    return () => { cancelled = true }
  }, [projectID, selectedBotId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    streaming.setCurrentSessionId(chatSessionId)
    return () => { streaming.setCurrentSessionId(null) }
  }, [chatSessionId]) // eslint-disable-line react-hooks/exhaustive-deps

  const pollForSession = useCallback(async (previous: string | null, requireDifferent: boolean) => {
    if (!projectID) return
    for (let i = 0; i < 20; i++) {
      await new Promise((r) => setTimeout(r, 1500))
      try {
        const sid = await fetchExistingWorkerSession(projectID, chatApi)
        if (sid && (!requireDifferent || sid !== previous)) {
          setChatSessionId(sid)
          return
        }
      } catch { /* keep polling */ }
    }
  }, [projectID, chatApi])

  const handleStart = async () => {
    if (!selectedBotId) return
    try {
      await activateAgent.mutateAsync(selectedBotId)
      snackbar.success('Starting agent…')
      await pollForSession(chatSessionId, false)
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'start failed')
    }
  }
  const handleStop = async () => {
    if (!selectedBotId) return
    try {
      await stopAgent.mutateAsync(selectedBotId)
      snackbar.success('Agent stopped')
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'stop failed')
    }
  }
  const handleRestart = async () => {
    if (!selectedBotId) return
    const prev = chatSessionId
    try {
      await restartAgent.mutateAsync(selectedBotId)
      setChatSessionId(null)
      snackbar.success('Restarting agent…')
      await pollForSession(prev, true)
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'restart failed')
    }
  }

  const busy = activateAgent.isPending || stopAgent.isPending || restartAgent.isPending
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'
  const statusColor = agentOnline ? 'rgb(46, 160, 67)' : (lightTheme.isLight ? 'rgba(0,0,0,0.28)' : 'rgba(255,255,255,0.28)')

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0, borderRight: `1px solid ${border}` }}>
      {/* Header: bot name + switcher + status + start/stop */}
      <Box
        sx={{
          px: 1.5,
          py: 1,
          minHeight: 52,
          borderBottom: `1px solid ${border}`,
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          flexShrink: 0,
          backgroundColor: 'background.paper',
        }}
      >
        <SmartToyOutlinedIcon sx={{ fontSize: 18, color: 'text.secondary', flexShrink: 0 }} />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          {agents.length === 0 ? (
            <Typography variant="body2" color="text.secondary">No bots yet</Typography>
          ) : (
            <Select
              size="small"
              variant="standard"
              disableUnderline
              value={selectedBotId}
              onChange={(e) => setSelectedBotId(e.target.value)}
              sx={{
                width: '100%',
                fontWeight: 600,
                fontSize: '0.9rem',
                '& .MuiSelect-select': { py: 0.25, pr: '24px !important' },
              }}
            >
              {agents.map((b) => (
                <MenuItem key={b.id} value={b.id ?? ''}>
                  {b.name || b.id}
                </MenuItem>
              ))}
            </Select>
          )}
          <Stack direction="row" alignItems="center" spacing={0.75}>
            <Tooltip title={agentOnline ? 'Agent sandbox online' : 'Agent sandbox stopped'}>
              <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: statusColor, flexShrink: 0 }} />
            </Tooltip>
            <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.2 }}>
              {agentOnline ? 'Running' : 'Stopped'}
              {selectedBotId ? ` · ${selectedBotId}` : ''}
            </Typography>
          </Stack>
        </Box>
        <Stack direction="row" spacing={0.25} sx={{ flexShrink: 0 }}>
          {agentOnline ? (
            <>
              <Tooltip title="Stop agent">
                <span>
                  <IconButton size="small" onClick={() => void handleStop()} disabled={busy}>
                    {stopAgent.isPending ? <CircularProgress size={16} /> : <StopIcon fontSize="small" />}
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Restart agent">
                <span>
                  <IconButton size="small" onClick={() => void handleRestart()} disabled={busy}>
                    {restartAgent.isPending ? <CircularProgress size={16} /> : <RestartAltIcon fontSize="small" />}
                  </IconButton>
                </span>
              </Tooltip>
            </>
          ) : (
            <Tooltip title="Start agent">
              <span>
                <IconButton size="small" color="primary" onClick={() => void handleStart()} disabled={busy || !selectedBotId}>
                  {activateAgent.isPending ? <CircularProgress size={16} /> : <PlayArrowIcon fontSize="small" />}
                </IconButton>
              </span>
            </Tooltip>
          )}
        </Stack>
      </Box>

      {/* Mini Chat / Desktop toggle */}
      <Stack
        direction="row"
        spacing={0.5}
        sx={{ px: 1.5, py: 0.75, borderBottom: `1px solid ${border}`, flexShrink: 0 }}
      >
        <Button
          size="small"
          variant={view === 'chat' ? 'contained' : 'text'}
          onClick={() => setView('chat')}
          sx={{ textTransform: 'none', minWidth: 0, px: 1.25 }}
        >
          Chat
        </Button>
        <Button
          size="small"
          variant={view === 'desktop' ? 'contained' : 'text'}
          onClick={() => setView('desktop')}
          sx={{ textTransform: 'none', minWidth: 0, px: 1.25 }}
        >
          Desktop
        </Button>
      </Stack>

      {/* Body — must flex-fill remaining height so EmbeddedSessionView scrolls inside. */}
      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {!selectedBotId ? (
          <Box sx={{ p: 3, textAlign: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              Create a bot on the chart to start chatting.
            </Typography>
          </Box>
        ) : !chatSessionId ? (
          <Box sx={{ p: 3, textAlign: 'center', m: 'auto' }}>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              {agentOnline
                ? 'Session is starting…'
                : `Start ${selectedBot?.name || selectedBotId} to open chat.`}
            </Typography>
            {!agentOnline && (
              <Button
                variant="contained"
                size="small"
                startIcon={activateAgent.isPending ? <CircularProgress size={14} color="inherit" /> : <PlayArrowIcon />}
                onClick={() => void handleStart()}
                disabled={busy}
              >
                Start agent
              </Button>
            )}
          </Box>
        ) : view === 'chat' ? (
          <>
            <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              <EmbeddedSessionView ref={sessionViewRef} sessionId={chatSessionId} autoScrollOnMount />
            </Box>
            <SessionPromptQueue sessionId={chatSessionId} />
            <Box sx={{ p: 1.5, flexShrink: 0 }}>
              <RobustPromptInput
                sessionId={chatSessionId}
                projectId={projectID}
                apiClient={api.getApiClient()}
                onSend={async (message: string, interrupt?: boolean) => {
                  await streaming.NewInference({
                    type: SESSION_TYPE_TEXT,
                    message,
                    sessionId: chatSessionId,
                    interrupt: interrupt ?? true,
                  })
                }}
                onHeightChange={() => sessionViewRef.current?.scrollToBottom()}
                placeholder={`Message ${selectedBot?.name || selectedBotId}…`}
              />
            </Box>
          </>
        ) : (
          <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <ExternalAgentDesktopViewer
              sessionId={chatSessionId}
              sandboxId={chatSessionId}
              mode="stream"
              displayWidth={1920}
              displayHeight={1080}
              displayFps={30}
            />
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default HelixOrgChatPanel
