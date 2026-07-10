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
import Accordion from '@mui/material/Accordion'
import AccordionDetails from '@mui/material/AccordionDetails'
import AccordionSummary from '@mui/material/AccordionSummary'
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
import Link from '@mui/material/Link'
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
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import SaveIcon from '@mui/icons-material/Save'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import MonacoEditor from '../components/widgets/MonacoEditor'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SessionPromptQueue from '../components/session/SessionPromptQueue'
import EmbeddedSessionView, {
  EmbeddedSessionViewHandle,
} from '../components/session/EmbeddedSessionView'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import RobustPromptInput from '../components/common/RobustPromptInput'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'

import router5 from '../router'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { deriveDisplaySettings } from '../services/externalAgentDisplay'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import {
  ToolDTO,
  useDeleteBot,
  useHelixOrgBot,
  useListBotSubscriptions,
  useListHelixOrgTools,
  useListHelixOrgTopics,
  useRestartBotAgent,
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
  const account = useAccount()
  const snackbar = useSnackbar()
  const api = useApi()
  const orgSlug = router.params.org_id as string | undefined
  const botId = router.params.bot_id as string | undefined
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Bots', routeName: 'helix_org_bots' })

  const del = useDeleteBot()
  // Stop polling/refetching this bot once a delete is in flight or done —
  // the row is being torn down, so a refetch would only hit a 404. The
  // page navigates to the bots list on success.
  const { data, isLoading } = useHelixOrgBot(botId, {
    enabled: !del.isPending && !del.isSuccess,
  })
  const streaming = useStreaming()
  const updateBot = useUpdateBot()
  const restartAgent = useRestartBotAgent()
  const { data: toolCatalogue } = useListHelixOrgTools()
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const bot = data?.bot
  const projectID = data?.project_id
  const agentAppID = data?.agent_app_id
  // A human node is a person placeholder — it never runs, so the agent-only
  // surfaces (Project Desktop session, tools, preserve-context, restart) make
  // no sense for it and are hidden below.
  const isHuman = bot?.kind === 'human'

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

  // handleRestartSession gives the Bot a genuinely fresh session: the
  // backend stops + deletes the current session and enqueues a brand-new
  // one (new desktop, new thread, current MCP services), which the spawner
  // provisions asynchronously. We drop the stale transcript immediately,
  // switch to Chat, then poll the project's exploratory session until the
  // new one appears and bind the chat/desktop panels to it. Tucked behind
  // the Advanced accordion; destructive to in-flight work.
  const handleRestartSession = async () => {
    if (!botId || restartAgent.isPending) return
    const previousSessionId = chatSessionId
    try {
      await restartAgent.mutateAsync(botId)
      setChatSessionId(null)
      setSessionTab('chat')
      snackbar.success('Fresh agent session started — it will come up shortly')
      // The new session is created asynchronously; poll the project's
      // exploratory session until a different (fresh) one is resolvable.
      if (projectID) {
        for (let attempt = 0; attempt < 20; attempt++) {
          await new Promise((resolve) => setTimeout(resolve, 1500))
          let sid: string | null = null
          try {
            sid = await fetchExistingWorkerSession(projectID, chatApi)
          } catch {
            sid = null
          }
          if (sid && sid !== previousSessionId) {
            setChatSessionId(sid)
            break
          }
        }
      }
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
    <Page
      breadcrumbTitle={botId ?? 'Bot'}
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <Stack direction="row" spacing={1}>
          <Button
            variant="contained"
            color="secondary"
            startIcon={<SaveIcon />}
            disabled={!dirty || updateBot.isPending}
            onClick={handleSave}
          >
            {updateBot.isPending ? 'Saving…' : 'Save'}
          </Button>
        </Stack>
      )}
    >
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
                      <Typography variant="subtitle1">Agent activity</Typography>
                      <ToggleButtonGroup
                        size="small"
                        exclusive
                        value={sessionTab}
                        onChange={(_e, value) => { if (value) setSessionTab(value) }}
                      >
                        <ToggleButton value="chat">Transcript</ToggleButton>
                        <ToggleButton value="desktop">Desktop</ToggleButton>
                      </ToggleButtonGroup>
                    </Stack>
                    <Typography variant="body2" color="text.secondary">
                      {sessionTab === 'chat'
                        ? "The bot's agent session — the transcript of what it's doing, including its MCP tool calls. Send a message to drive it."
                        : "The live desktop of the bot's agent session — watch and drive what it is doing in real time."}
                    </Typography>

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
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          No conversation yet for this bot.
                        </Typography>
                      )
                    ) : (
                      // Desktop stream — same widget as the spec-task page.
                      // ExternalAgentDesktopViewer handles the sandbox
                      // lifecycle (starting/running/paused) internally; it
                      // needs a bounded, flex-column container to fill.
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
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          No desktop yet for this bot.
                        </Typography>
                      )
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
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Content (markdown)</Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                    The bot's prompt / identity markdown. Read on every activation.
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

          {/* Advanced — collapsed by default. Houses destructive
              maintenance actions kept out of the main flow. */}
          <Accordion
            disableGutters
            elevation={0}
            sx={{
              mt: 3,
              border: (theme) => `1px solid ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'}`,
              borderRadius: 1,
              '&:before': { display: 'none' },
              backgroundImage: 'none',
            }}
          >
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography variant="subtitle2">Advanced</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={1.5} alignItems="flex-start">
                <Button
                  variant="outlined"
                  color="warning"
                  startIcon={restartAgent.isPending ? <CircularProgress size={16} color="inherit" /> : <RestartAltIcon />}
                  onClick={handleRestartSession}
                  disabled={restartAgent.isPending}
                >
                  {restartAgent.isPending ? 'Restarting…' : 'Restart agent session'}
                </Button>
                <Typography variant="caption" color="text.secondary">
                  Deletes the current session and starts a brand-new one — a fresh
                  desktop, thread and tools. Any in-progress work in the current
                  session will be lost.
                </Typography>
              </Stack>
            </AccordionDetails>
          </Accordion>
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
    </Page>
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
