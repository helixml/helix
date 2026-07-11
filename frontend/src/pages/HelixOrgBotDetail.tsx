// HelixOrgBotDetail shows a single bot and lets the operator edit it
// end-to-end and watch / drive its conversation inline.
//
// A Bot is the merge of the former Role and Worker: its markdown
// `content` is its identity/prompt, its `tools` list is its MCP tool
// surface, it carries topic subscriptions, and it reports to other bots
// (parent_ids). Content + tools are edited here via Monaco + a tools
// multi-select and saved in one step through useUpdateBot (there is no
// separate identity field). Subscriptions are managed in the panel below.
//
// The inline transcript (EmbeddedSessionView + RobustPromptInput) is the
// same view the spec-task page uses — it auto-shows when the bot's project
// already has an exploratory session. GET-only on load — never spins up
// infra by itself; sessions are provisioned by the bot's activation flow.

import { FC, Key, useEffect, useMemo, useRef, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import FormControlLabel from '@mui/material/FormControlLabel'
import Grid from '@mui/material/Grid'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import Typography from '@mui/material/Typography'
import CheckBoxIcon from '@mui/icons-material/CheckBox'
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import SaveIcon from '@mui/icons-material/Save'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import StopIcon from '@mui/icons-material/Stop'
import Tooltip from '@mui/material/Tooltip'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import MonacoEditor from '../components/widgets/MonacoEditor'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SessionPromptQueue from '../components/session/SessionPromptQueue'
import EmbeddedSessionView, {
  EmbeddedSessionViewHandle,
} from '../components/session/EmbeddedSessionView'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import RobustPromptInput from '../components/common/RobustPromptInput'

import router5 from '../router'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { deriveDisplaySettings } from '../services/externalAgentDisplay'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import {
  ToolDTO,
  useActivateBot,
  useDeleteBot,
  useHelixOrgBot,
  useListBotSubscriptions,
  useListHelixOrgTools,
  useListHelixOrgTopics,
  useRestartBotAgent,
  useStopBotAgent,
  useSubscribeBot,
  useUnsubscribeBot,
  useUpdateBot,
} from '../services/helixOrgService'
import {
  WorkerChatReader,
  fetchExistingWorkerSession,
} from '../services/workerChatSession'

const HelixOrgBotDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const api = useApi()
  const orgSlug = router.params.org_id as string | undefined
  const botId = router.params.bot_id as string | undefined

  const del = useDeleteBot()
  // Stop polling/refetching this bot once a delete is in flight or done —
  // the row is being torn down, so a refetch would only hit a 404. The
  // page navigates to the bots list on success.
  const { data, isLoading, refetch: refetchBot } = useHelixOrgBot(botId, {
    enabled: !del.isPending && !del.isSuccess,
  })
  const streaming = useStreaming()
  const updateBot = useUpdateBot()
  const activateAgent = useActivateBot()
  const stopAgent = useStopBotAgent()
  const restartAgent = useRestartBotAgent()
  const { data: toolCatalogue } = useListHelixOrgTools()
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [agentMenuEl, setAgentMenuEl] = useState<null | HTMLElement>(null)

  const bot = data?.bot
  const projectID = data?.project_id
  const agentAppID = data?.agent_app_id
  // A human node is a person placeholder — it never runs, so the agent-only
  // surfaces (Project Desktop session, tools, preserve-context, restart) make
  // no sense for it and are hidden below.
  const isHuman = bot?.kind === 'human'
  // agent_status from GET /bots/{id}; poll detail so the presence control stays fresh.
  const agentOnline = bot?.agent_status === 'running'
  useEffect(() => {
    if (!botId || isHuman) return
    const t = window.setInterval(() => { void refetchBot() }, 5000)
    return () => window.clearInterval(t)
  }, [botId, isHuman, refetchBot])

  // Editable content markdown + tools. Seeded from the bot every time it
  // loads/refreshes so a cancelled edit re-syncs to server state.
  const [name, setName] = useState('')
  const [content, setContent] = useState('')
  const [tools, setTools] = useState<string[]>([])
  const [preserveContext, setPreserveContext] = useState(false)
  useEffect(() => {
    setName(bot?.name ?? '')
    setContent(bot?.content ?? '')
    setTools(bot?.tools ?? [])
    setPreserveContext(bot?.preserve_context ?? false)
  }, [bot?.name, bot?.content, bot?.tools, bot?.preserve_context])

  // A human node is a person, not a bot — the agent detail page (desktop,
  // tools, activation) makes no sense for it. Redirect a direct hit on
  // /bots/h-<userId> to the dedicated person view. The render below also
  // guards on isHuman so the agent surfaces never flash before the redirect.
  useEffect(() => {
    if (isHuman && orgSlug && botId) {
      router.navigate('helix_org_human_detail', { org_id: orgSlug, bot_id: botId })
    }
  }, [isHuman, orgSlug, botId, router])

  // The Autocomplete needs Option objects, but the bot's tool list is
  // just a string[] of names. Render every catalogue entry plus any
  // bot-listed names the catalogue didn't return (defensive — if a tool
  // was unregistered but the bot still lists it, keep showing it as
  // selected so the operator can explicitly remove it).
  const toolOptions = useMemo<ToolDTO[]>(() => {
    const cat = toolCatalogue ?? []
    const known = new Set(cat.map((t) => t.name))
    const extras = tools
      .filter((name) => !known.has(name))
      .map<ToolDTO>((name) => ({ name, description: '(not in current catalogue)' }))
    return [...cat, ...extras]
  }, [toolCatalogue, tools])

  const dirty = useMemo(() => {
    if (!bot) return false
    if ((bot.name ?? '') !== name) return true
    if ((bot.content ?? '') !== content) return true
    if ((bot.tools ?? []).join(',') !== tools.join(',')) return true
    if ((bot.preserve_context ?? false) !== preserveContext) return true
    return false
  }, [bot, name, content, tools, preserveContext])

  const handleSave = async () => {
    if (!botId) return
    try {
      await updateBot.mutateAsync({ id: botId, name, content, tools, preserve_context: preserveContext })
      snackbar.success(`bot ${botId} saved`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'save failed')
    }
  }

  const pollForSession = async (previousSessionId: string | null, requireDifferent: boolean) => {
    if (!projectID) return
    for (let attempt = 0; attempt < 20; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 1500))
      let sid: string | null = null
      try {
        sid = await fetchExistingWorkerSession(projectID, chatApi)
      } catch {
        sid = null
      }
      if (!sid) continue
      if (!requireDifferent || sid !== previousSessionId) {
        setChatSessionId(sid)
        return
      }
    }
  }

  const handleStartSession = async () => {
    if (!botId || activateAgent.isPending) return
    try {
      await activateAgent.mutateAsync(botId)
      snackbar.success('Starting agent…')
      await pollForSession(chatSessionId, false)
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'start failed')
    }
  }

  const handleStopSession = async () => {
    if (!botId || stopAgent.isPending) return
    try {
      await stopAgent.mutateAsync(botId)
      snackbar.success('Agent stopped')
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'stop failed')
    }
  }

  // Full reset: stop + delete session, then activate a brand-new one.
  const handleRestartSession = async () => {
    if (!botId || restartAgent.isPending) return
    const previousSessionId = chatSessionId
    try {
      await restartAgent.mutateAsync(botId)
      setChatSessionId(null)
      setSessionTab('chat')
      snackbar.success('Restarting agent — a fresh session will come up shortly')
      await pollForSession(previousSessionId, true)
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'restart failed')
    }
  }

  // chatSessionId is the bot's long-lived "Project Desktop" exploratory
  // session — the transcript we render inline. Null until we've resolved
  // one (or there isn't one yet).
  const [chatSessionId, setChatSessionId] = useState<string | null>(null)
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)

  // The session panel toggles between the inline Chat transcript and the
  // live Desktop stream — both bound to the same exploratory session.
  const [sessionTab, setSessionTab] = useState<'chat' | 'desktop'>('chat')

  // Desktop resolution / fps for the stream, derived from the bot's agent
  // app config (same helper the spec-task desktop uses). Falls back to
  // 1920x1080x60 when the app or config is missing.
  const apps = useApps()
  const displaySettings = useMemo(
    () => deriveDisplaySettings(apps.apps?.find((a) => a.id === agentAppID)),
    [agentAppID, apps.apps],
  )

  // chatApi adapts the generated client to the read-only shape the
  // workerChatSession helper expects (we only GET the existing session
  // here — provisioning is owned by the bot's activation flow). The
  // exploratory-session GET returns 204 No Content when the project has
  // no session yet — map that to null rather than surfacing an error.
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

  // Auto-load the inline transcript when the bot already has a project.
  // GET-only — we never create a session just because the operator opened
  // this page; sessions are provisioned by the bot's activation flow.
  useEffect(() => {
    let cancelled = false
    if (!projectID) {
      setChatSessionId(null)
      return
    }
    fetchExistingWorkerSession(projectID, chatApi)
      .then((sid) => { if (!cancelled) setChatSessionId(sid) })
      .catch(() => { if (!cancelled) setChatSessionId(null) })
    return () => { cancelled = true }
    // chatApi is stable (memoised on the singleton api client); keying on
    // projectID alone follows the "only primitives in deps" rule.
  }, [projectID])

  // Subscribe the WebSocket to the inline session so in-flight turns live
  // (mirrors SpecTaskDetailContent, which likewise omits the streaming
  // context object from deps). Clear on unmount / session change.
  useEffect(() => {
    streaming.setCurrentSessionId(chatSessionId)
    return () => { streaming.setCurrentSessionId(null) }
  }, [chatSessionId])

  const handleDelete = async () => {
    if (!botId) return
    try {
      await del.mutateAsync(botId)
      snackbar.success(`deleted ${botId}`)
      if (orgSlug) {
        router.navigate('helix_org_bots', { org_id: orgSlug })
      }
    } catch (err: any) {
      const status = err?.response?.status
      if (status === 409) {
        snackbar.error('owner bot is protected and cannot be deleted')
      } else {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'delete failed')
      }
    } finally {
      setConfirmingDelete(false)
    }
  }

  return (
    <HelixOrgShell
      title={botId ?? 'Bot'}
      showChat={false}
      topbarActions={(
        <Button
          variant="contained"
          color="secondary"
          size="small"
          startIcon={<SaveIcon />}
          disabled={!dirty || updateBot.isPending}
          onClick={handleSave}
        >
          {updateBot.isPending ? 'Saving…' : 'Save'}
        </Button>
      )}
    >
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        {isLoading || !bot || isHuman ? (
          <LoadingSpinner />
        ) : (
          <>
          <Grid container spacing={3}>
            <Grid item xs={12} md={9}>
              <Stack spacing={3}>
                <Box>
                  <Stack direction="row" alignItems="baseline" spacing={1}>
                    <SmartToyOutlinedIcon sx={{ alignSelf: 'center' }} />
                    <Typography variant="h5">
                      {bot.name || bot.id}
                    </Typography>
                    {bot.name && (
                      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                        {bot.id}
                      </Typography>
                    )}
                  </Stack>
                </Box>

                {/* Session panel — Chat | Desktop toggle, both bound to the
                    bot's Project Desktop exploratory session (the same views
                    the spec-task page uses). Auto-loads when the bot already
                    has a session; otherwise shows an empty state. */}
                <Paper variant="outlined" sx={{ p: 3 }}>
                  <Stack spacing={2} alignItems="flex-start">
                    <Stack
                      direction={{ xs: 'column', sm: 'row' }}
                      justifyContent="space-between"
                      alignItems={{ xs: 'flex-start', sm: 'center' }}
                      spacing={1}
                      sx={{ width: '100%' }}
                    >
                      <Stack direction="row" alignItems="center" spacing={1}>
                        <Tooltip title={agentOnline ? 'Agent sandbox online' : 'Agent sandbox stopped'}>
                          <Box
                            sx={{
                              width: 10,
                              height: 10,
                              borderRadius: '50%',
                              backgroundColor: agentOnline ? 'rgb(46, 160, 67)' : 'rgba(0,0,0,0.28)',
                              boxShadow: agentOnline ? '0 0 0 2px rgba(46,160,67,0.2)' : 'none',
                              flexShrink: 0,
                            }}
                          />
                        </Tooltip>
                        <Typography variant="subtitle1">Agent activity</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {agentOnline ? 'Running' : 'Stopped'}
                        </Typography>
                      </Stack>
                      <Stack direction="row" spacing={0.5} alignItems="center">
                        <ToggleButtonGroup
                          size="small"
                          exclusive
                          value={sessionTab}
                          onChange={(_e, value) => { if (value) setSessionTab(value) }}
                        >
                          <ToggleButton value="chat">Chat</ToggleButton>
                          <ToggleButton value="desktop">Desktop</ToggleButton>
                        </ToggleButtonGroup>
                        <IconButton
                          size="small"
                          aria-label="Agent session actions"
                          onClick={(e) => setAgentMenuEl(e.currentTarget)}
                          disabled={activateAgent.isPending || stopAgent.isPending || restartAgent.isPending}
                        >
                          {(activateAgent.isPending || stopAgent.isPending || restartAgent.isPending)
                            ? <CircularProgress size={16} />
                            : <MoreVertIcon />}
                        </IconButton>
                        <Menu
                          anchorEl={agentMenuEl}
                          open={Boolean(agentMenuEl)}
                          onClose={() => setAgentMenuEl(null)}
                          anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                          transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                        >
                          {agentOnline ? (
                            <>
                              <MenuItem
                                onClick={() => {
                                  setAgentMenuEl(null)
                                  void handleStopSession()
                                }}
                              >
                                <StopIcon sx={{ mr: 1, fontSize: 20 }} />
                                Stop agent
                              </MenuItem>
                              <MenuItem
                                onClick={() => {
                                  setAgentMenuEl(null)
                                  void handleRestartSession()
                                }}
                              >
                                <RestartAltIcon sx={{ mr: 1, fontSize: 20 }} />
                                Restart agent
                              </MenuItem>
                            </>
                          ) : (
                            <MenuItem
                              onClick={() => {
                                setAgentMenuEl(null)
                                void handleStartSession()
                              }}
                            >
                              <PlayArrowIcon sx={{ mr: 1, fontSize: 20 }} />
                              Start agent
                            </MenuItem>
                          )}
                        </Menu>
                      </Stack>
                    </Stack>
                    <Typography variant="body2" color="text.secondary">
                      {sessionTab === 'chat'
                        ? "Chat with this bot’s agent — messages include tool calls. Use the menu to start, stop, or restart the agent."
                        : "Live desktop of the bot’s agent. Use the menu to start, stop, or restart the agent."}
                    </Typography>

                    {!agentOnline && !chatSessionId && (
                      <Box
                        sx={{
                          width: '100%',
                          py: 4,
                          px: 2,
                          textAlign: 'center',
                          border: (theme) =>
                            `1px dashed ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.12)' : 'rgba(255,255,255,0.12)'}`,
                          borderRadius: 1,
                        }}
                      >
                        <Typography variant="body2" color="text.secondary">
                          The agent is not running. Open the ⋮ menu and choose <strong>Start agent</strong> to begin.
                        </Typography>
                      </Box>
                    )}

                    {/* Inline transcript. EmbeddedSessionView self-fetches
                        the session + interactions and live-tails in-flight
                        turns; it needs a bounded, flex-column container to
                        scroll within. RobustPromptInput drives the same
                        session via streaming.NewInference. */}
                    {sessionTab === 'chat' ? (
                      chatSessionId ? (
                        <Box
                          sx={{
                            width: '100%',
                            height: 520,
                            display: 'flex',
                            flexDirection: 'column',
                            border: (theme) =>
                              `1px solid ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'}`,
                            borderRadius: 1,
                            overflow: 'hidden',
                          }}
                        >
                          <EmbeddedSessionView
                            ref={sessionViewRef}
                            sessionId={chatSessionId}
                            autoScrollOnMount
                          />
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
                              placeholder="Send message to agent..."
                            />
                          </Box>
                        </Box>
                      ) : agentOnline ? (
                        <Typography variant="body2" color="text.secondary">
                          Agent is starting — the transcript will appear when the session is ready.
                        </Typography>
                      ) : null
                    ) : (
                      chatSessionId ? (
                        <Box
                          sx={{
                            width: '100%',
                            height: 520,
                            display: 'flex',
                            flexDirection: 'column',
                            border: (theme) =>
                              `1px solid ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'}`,
                            borderRadius: 1,
                            overflow: 'hidden',
                          }}
                        >
                          <ExternalAgentDesktopViewer
                            sessionId={chatSessionId}
                            sandboxId={chatSessionId}
                            mode="stream"
                            displayWidth={displaySettings.width}
                            displayHeight={displaySettings.height}
                            displayFps={displaySettings.fps}
                          />
                        </Box>
                      ) : agentOnline ? (
                        <Typography variant="body2" color="text.secondary">
                          Agent is starting — the desktop will appear when ready.
                        </Typography>
                      ) : null
                    )}
                  </Stack>
                </Paper>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Name</Typography>
                  <TextField
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={bot.id}
                    fullWidth
                    size="small"
                    helperText="Human-readable display label. The id stays fixed."
                  />
                </Box>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Instructions</Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                    Instructions to follow, set on every interaction.
                    Cmd/Ctrl+S inside the editor saves.
                  </Typography>
                  <MonacoEditor
                    value={content}
                    onChange={setContent}
                    onSave={handleSave}
                    language="markdown"
                    minHeight={240}
                    maxHeight={600}
                    autoHeight={true}
                    theme="helix-dark"
                  />
                </Box>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Tools</Typography>
                  <Autocomplete
                    multiple
                    disableCloseOnSelect
                    options={toolOptions}
                    value={toolOptions.filter((o) => tools.includes(o.name))}
                    onChange={(_e, value) => setTools(value.map((v) => v.name))}
                    getOptionLabel={(o) => o.name}
                    isOptionEqualToValue={(a, b) => a.name === b.name}
                    renderOption={(props, option, { selected }) => {
                      // Pass key explicitly rather than via the props
                      // spread — React 18.3 warns when a spread object
                      // carries a key.
                      const { key, ...liProps } = props as typeof props & { key?: Key }
                      return (
                        <li key={key ?? option.name} {...liProps}>
                          <Checkbox
                            icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
                            checkedIcon={<CheckBoxIcon fontSize="small" />}
                            style={{ marginRight: 8 }}
                            checked={selected}
                          />
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                              {option.name}
                            </Typography>
                            {option.description && (
                              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                {option.description}
                              </Typography>
                            )}
                          </Box>
                        </li>
                      )
                    }}
                    renderTags={(value, getTagProps) =>
                      value.map((option, index) => {
                        const { key, ...tagProps } = getTagProps({ index })
                        return (
                          <Chip
                            key={key ?? option.name}
                            {...tagProps}
                            label={option.name}
                            size="small"
                            sx={{ fontFamily: 'monospace' }}
                          />
                        )
                      })
                    }
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        placeholder={tools.length === 0 ? 'Pick the tools this bot can call' : ''}
                        helperText="MCP tools this bot can call. Empty = no tools (the bot can still receive owner-chat)."
                      />
                    )}
                  />
                </Box>

                <Box>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={preserveContext}
                        onChange={(_e, checked) => setPreserveContext(checked)}
                      />
                    }
                    label="Preserve context across triggers"
                  />
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                    By default each trigger wipes the bot's session so every turn
                    starts on a fresh context window. Enable this to keep the
                    conversation across triggers — faster, more context-aware
                    follow-ups (e.g. for Slack), at the cost of the session
                    growing toward the model's context limit (where compaction
                    kicks in). Durable state still belongs in the bot's git
                    workspace, not the chat history.
                  </Typography>
                </Box>

                <SubscriptionsPanel botID={bot?.id} />
              </Stack>
            </Grid>

            {/* Right rail: identity context + delete action */}
            <Grid item xs={12} md={3}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={2}>
                  <Box>
                    <Typography variant="caption" color="text.secondary">ID</Typography>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{bot.id}</Typography>
                  </Box>
                  {(bot?.parent_ids ?? []).length > 0 && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Reports to</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                        {(bot?.parent_ids ?? []).join(', ')}
                      </Typography>
                    </Box>
                  )}
                  {bot?.created_at && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Created</Typography>
                      <Typography variant="body2">{new Date(bot.created_at).toLocaleString()}</Typography>
                    </Box>
                  )}
                  {bot?.updated_at && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Updated</Typography>
                      <Typography variant="body2">{new Date(bot.updated_at).toLocaleString()}</Typography>
                    </Box>
                  )}
                  {projectID && (
                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>Project</Typography>
                      {orgSlug ? (
                        <Link
                          href={router5.buildPath('org_project-specs', { org_id: orgSlug, id: projectID })}
                          target="_blank"
                          rel="noopener noreferrer"
                          underline="hover"
                          sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'inline-flex', alignItems: 'center', gap: 0.5, wordBreak: 'break-all' }}
                        >
                          {projectID}
                          <OpenInNewIcon sx={{ fontSize: 14, flexShrink: 0 }} />
                        </Link>
                      ) : (
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>{projectID}</Typography>
                      )}
                    </Box>
                  )}
                  {agentAppID && (
                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>Agent</Typography>
                      {orgSlug ? (
                        <Link
                          href={router5.buildPath('org_agent', { org_id: orgSlug, app_id: agentAppID })}
                          target="_blank"
                          rel="noopener noreferrer"
                          underline="hover"
                          sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'inline-flex', alignItems: 'center', gap: 0.5, wordBreak: 'break-all' }}
                        >
                          {agentAppID}
                          <OpenInNewIcon sx={{ fontSize: 14, flexShrink: 0 }} />
                        </Link>
                      ) : (
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>{agentAppID}</Typography>
                      )}
                    </Box>
                  )}
                  <Divider />
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteOutlineIcon />}
                    onClick={() => setConfirmingDelete(true)}
                    disabled={del.isPending}
                    fullWidth
                  >
                    Delete bot
                  </Button>
                  <Typography variant="caption" color="text.secondary">
                    Tears down the bot's per-bot Helix project and deletes the row,
                    dropping its subscriptions and reporting lines.
                  </Typography>
                </Stack>
              </Paper>
            </Grid>
          </Grid>

          </>
        )}
      </Container>

      {confirmingDelete && botId && (
        <DeleteConfirmWindow
          title="bot"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setConfirmingDelete(false)}
        >
          <Typography variant="body1">
            Deleting bot <b style={{ fontFamily: 'monospace' }}>{botId}</b> tears down its
            per-bot Helix project + agent app and clears its runtime state. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
          </Box>
    </HelixOrgShell>
  )
}

// SubscriptionsPanel surfaces the topics this Bot consumes — and the
// multi-select to change that set. Subscriptions are bot-anchored:
// deleting the bot drops them.
//
// disableCloseOnSelect so toggling several topics in one pass doesn't
// bounce the popper closed.
const SubscriptionsPanel: FC<{ botID?: string }> = ({ botID }) => {
  const snackbar = useSnackbar()
  const { data: streamsData, isLoading: streamsLoading } = useListHelixOrgTopics()
  const { data: subsData, isLoading: subsLoading } = useListBotSubscriptions(botID)
  const subscribe = useSubscribeBot(botID)
  const unsubscribe = useUnsubscribeBot(botID)

  const allTopics = streamsData?.topics ?? []
  const subscribedIDs = useMemo(
    () => new Set((subsData?.subscriptions ?? []).map((s) => s.topic_id)),
    [subsData],
  )
  const subscribedTopics = useMemo(
    () => allTopics.filter((s) => subscribedIDs.has(s.id)),
    [allTopics, subscribedIDs],
  )

  if (!botID) {
    return null
  }

  const handleChange = async (_e: unknown, next: typeof allTopics) => {
    const nextIDs = new Set(next.map((s) => s.id))
    const toAdd = next.filter((s) => !subscribedIDs.has(s.id))
    const toRemove = (subsData?.subscriptions ?? []).filter((s) => !nextIDs.has(s.topic_id))
    try {
      for (const s of toAdd) await subscribe.mutateAsync(s.id)
      for (const s of toRemove) await unsubscribe.mutateAsync(s.topic_id)
      if (toAdd.length || toRemove.length) {
        snackbar.success(`subscriptions updated (${toAdd.length} added, ${toRemove.length} removed)`)
      }
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'subscription update failed')
    }
  }

  return (
    <Box>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Subscriptions ({subscribedTopics.length})
      </Typography>
      <Autocomplete
        multiple
        disableCloseOnSelect
        loading={streamsLoading || subsLoading}
        options={allTopics}
        value={subscribedTopics}
        onChange={handleChange}
        getOptionLabel={(s) => s.id}
        isOptionEqualToValue={(a, b) => a.id === b.id}
        renderOption={(props, option, { selected }) => {
          // Pass key explicitly rather than via the props spread —
          // React 18.3 warns when a spread object carries a key.
          const { key, ...liProps } = props as typeof props & { key?: Key }
          return (
            <li key={key ?? option.id} {...liProps}>
              <Checkbox checked={selected} sx={{ mr: 1 }} />
              <Box>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option.id}</Typography>
                {option.description && (
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                    {option.description}
                  </Typography>
                )}
              </Box>
            </li>
          )
        }}
        renderInput={(params) => (
          <TextField
            {...params}
            placeholder={subscribedTopics.length === 0 ? 'Subscribe this bot to a topic…' : ''}
            variant="outlined"
            size="small"
          />
        )}
        renderTags={(value, getTagProps) =>
          value.map((option, index) => {
            const { key, ...tagProps } = getTagProps({ index })
            return (
              <Chip
                key={key ?? option.id}
                {...tagProps}
                label={option.id}
                size="small"
                sx={{ fontFamily: 'monospace' }}
              />
            )
          })
        }
      />
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
        Subscriptions are per-Bot — they die when this Bot is deleted.
      </Typography>
    </Box>
  )
}

export default HelixOrgBotDetail
