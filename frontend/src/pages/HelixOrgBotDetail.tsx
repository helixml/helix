// HelixOrgBotDetail shows a single bot and lets the operator edit it
// end-to-end: identity, runtime, instructions, tools, subscriptions.
//
// A Bot is the merge of the former Role and Worker: its markdown
// `content` is its identity/prompt, its `tools` list is its MCP tool
// surface, it carries topic subscriptions, and it reports to other bots
// (parent_ids). Content + tools are edited here via Monaco + a grouped tools
// dialog and saved in one step through useUpdateBot (there is no
// separate identity field). Subscriptions are managed in the panel below.
//
// This page deliberately carries NO inline transcript or desktop viewer —
// conversing with the agent belongs in the real chat surface, not a
// duplicate embedded in a config page. What stays here is lifecycle
// control (start / stop / restart, in the header) because it acts on the
// thing this page configures. The bot's exploratory session id is still
// tracked (GET-only, never provisioned here) so a runtime change can
// switch the live session through the canonical lifecycle.

import { FC, Key, useEffect, useMemo, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
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
import Typography from '@mui/material/Typography'
import CheckBoxIcon from '@mui/icons-material/CheckBox'
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import EditOutlinedIcon from '@mui/icons-material/EditOutlined'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import SaveIcon from '@mui/icons-material/Save'
import SettingsBackupRestoreIcon from '@mui/icons-material/SettingsBackupRestore'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import StopIcon from '@mui/icons-material/Stop'
import Tooltip from '@mui/material/Tooltip'
import { useRouter as useRouter5 } from 'react-router5'

import AgentRestartRequiredBanner from '../components/helix-org/AgentRestartRequiredBanner'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import AgentConfigForm, { AgentConfigValue } from '../components/helix-org/BotRuntimeForm'
import ToolPickerDialog from '../components/helix-org/ToolPickerDialog'
import WorkerSecretsPanel from '../components/helix-org/WorkerSecretsPanel'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import MonacoEditor from '../components/widgets/MonacoEditor'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import { useStreaming } from '../contexts/streaming'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useListProjects } from '../services/projectService'
import {
  BotDTO,
  ToolDTO,
  useActivateBot,
  useDeleteBot,
  useHelixOrgBot,
  useListHelixOrgProcessors,
  useListHelixOrgTools,
  useRestartBotAgent,
  useStopBotAgent,
  useUpdateBot,
} from '../services/helixOrgService'
import {
  useAgentAttachments,
  useCreateAgentAttachment,
  useDeleteAgentAttachment,
  useTriggers,
} from '../services/triggerService'
import {
  WorkerChatReader,
  fetchExistingWorkerSession,
} from '../services/workerChatSession'
import { useSwitchAgent } from '../services/sessionService'

const HelixOrgBotDetail: FC = () => {
  const router5 = useRouter5()
  const router = useRouter()
  const snackbar = useSnackbar()
  const api = useApi()
  const streaming = useStreaming()
  const orgSlug = router.params.org_id as string | undefined
  const botId = router.params.bot_id as string | undefined
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Agents', routeName: 'helix_org_bots' })

  const del = useDeleteBot()
  // Stop polling/refetching this bot once a delete is in flight or done —
  // the row is being torn down, so a refetch would only hit a 404. The
  // page navigates to the bots list on success.
  const { data, isLoading, refetch: refetchBot } = useHelixOrgBot(botId, {
    enabled: !del.isPending && !del.isSuccess,
  })
  const updateBot = useUpdateBot()
  const activateAgent = useActivateBot()
  const stopAgent = useStopBotAgent()
  const restartAgent = useRestartBotAgent()
  const { data: toolCatalogue } = useListHelixOrgTools()
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [confirmingResetInstructions, setConfirmingResetInstructions] = useState(false)
  const [editingTools, setEditingTools] = useState(false)
  const [agentMenuEl, setAgentMenuEl] = useState<null | HTMLElement>(null)
  // The bot owns one durable exploratory session. Runtime edits must switch
  // that session through the canonical lifecycle instead of leaving it bound
  // to an agent server that the updated app config no longer registers.
  const [chatSessionId, setChatSessionId] = useState<string | null>(null)
  const switchAgent = useSwitchAgent(chatSessionId ?? '')

  const bot = data?.bot
  const projectID = data?.project_id
  const { data: projects = [] } = useListProjects(bot?.organization_id, { enabled: !!bot?.organization_id })
  const agentID = data?.agent_id ?? data?.agent_app_id
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
  const [projectIDs, setProjectIDs] = useState<string[]>([])
  const [preserveContext, setPreserveContext] = useState(false)
  const [runtimeConfig, setRuntimeConfig] = useState<AgentConfigValue>({
    runtime: '',
    credentials: '',
    provider: '',
    model: '',
    reasoning_effort: '',
  })
  useEffect(() => {
    setName(bot?.name ?? '')
    setContent(bot?.content ?? '')
    setTools(bot?.tools ?? [])
    setProjectIDs(Array.from(new Set([...(bot?.project_ids ?? []), ...(projectID ? [projectID] : [])])))
    setPreserveContext(bot?.preserve_context ?? false)
  }, [bot?.name, bot?.content, bot?.tools, bot?.project_ids, bot?.preserve_context, projectID])

  useEffect(() => {
    setRuntimeConfig({
      runtime: bot?.code_agent_runtime ?? '',
      credentials: bot?.code_agent_credential_type ?? 'api_key',
      provider: bot?.provider ?? '',
      model: bot?.model ?? '',
      reasoning_effort: bot?.reasoning_effort ?? 'none',
    })
  }, [bot?.code_agent_runtime, bot?.code_agent_credential_type, bot?.provider, bot?.model, bot?.reasoning_effort])

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
    const savedProjectIDs = Array.from(new Set([...(bot.project_ids ?? []), ...(projectID ? [projectID] : [])])).sort()
    if (savedProjectIDs.join(',') !== [...projectIDs].sort().join(',')) return true
    if ((bot.preserve_context ?? false) !== preserveContext) return true
    if ((bot.code_agent_runtime ?? '') !== runtimeConfig.runtime) return true
    if ((bot.code_agent_credential_type ?? 'api_key') !== runtimeConfig.credentials) return true
    if ((bot.provider ?? '') !== runtimeConfig.provider) return true
    if ((bot.model ?? '') !== runtimeConfig.model) return true
    if ((bot.reasoning_effort ?? 'none') !== (runtimeConfig.reasoning_effort ?? 'none')) return true
    return false
  }, [bot, name, content, tools, projectIDs, projectID, preserveContext, runtimeConfig.runtime, runtimeConfig.credentials, runtimeConfig.provider, runtimeConfig.model, runtimeConfig.reasoning_effort])

  const handleSave = async () => {
    if (!botId) return
    const runtimeChanged =
      (bot?.code_agent_runtime ?? '') !== runtimeConfig.runtime
    try {
      await updateBot.mutateAsync({
        id: botId,
        name,
        content,
        tools,
        project_ids: projectIDs,
        preserve_context: preserveContext,
        code_agent_runtime: runtimeConfig.runtime as NonNullable<BotDTO['code_agent_runtime']>,
        code_agent_credential_type: runtimeConfig.credentials as NonNullable<BotDTO['code_agent_credential_type']>,
        provider: runtimeConfig.provider,
        model: runtimeConfig.model,
        reasoning_effort: runtimeConfig.reasoning_effort ?? 'none',
      })
      await refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'save failed')
      return
    }
    if (runtimeChanged && chatSessionId && agentID) {
      try {
        await switchAgent.mutateAsync({ helix_app_id: agentID })
        snackbar.success(`Agent ${botId} saved and the active session switched to ${runtimeConfig.runtime}`)
      } catch (err: any) {
        await refetchBot()
        const message = err?.response?.data?.error ?? err?.message ?? 'session switch failed'
        snackbar.error(`Agent saved, but active session switch failed: ${message}`)
      }
    } else {
      snackbar.success(`Agent ${botId} saved`)
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
      snackbar.success('Restarting agent — a fresh session will come up shortly')
      await pollForSession(previousSessionId, true)
      void refetchBot()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'restart failed')
    }
  }

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

  // Track the bot's existing exploratory session when it has a project.
  // GET-only — we never create a session just because the operator opened
  // this page; sessions are provisioned by the bot's activation flow. The
  // id is what lets a runtime change switch the live session on save.
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

  // Built-in seed prompt for this node, when it has one. Only seeded
  // nodes (the Chief of Staff every org gets) carry a default; for
  // operator-created agents the server sends nothing and the reset
  // affordance stays hidden — there is no default to go back to.
  const defaultInstructions = bot?.default_instructions ?? ''

  // Reset the editor to the built-in prompt and persist it in one step,
  // so the operator does not have to remember a follow-up Save. Other
  // unsaved edits on the page are left alone — this patches content only.
  const handleResetInstructions = async () => {
    if (!botId || !defaultInstructions) return
    try {
      await updateBot.mutateAsync({ id: botId, content: defaultInstructions })
      setContent(defaultInstructions)
      await refetchBot()
      snackbar.success('Instructions reset to the default')
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'reset failed')
    } finally {
      setConfirmingResetInstructions(false)
    }
  }

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
        snackbar.error('owner agent is protected and cannot be deleted')
      } else {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'delete failed')
      }
    } finally {
      setConfirmingDelete(false)
    }
  }

  const leafTitle = bot?.name || botId || 'Agent'

  return (
    <HelixOrgShell
      showChat={false}
      breadcrumbs={breadcrumbs}
      breadcrumbTitle={leafTitle}
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
            <Grid item xs={12} md={8}>
              <Stack spacing={3}>
                {/* Header — identity plus the agent's lifecycle control.
                    The start/stop/restart menu lives here (rather than in a
                    session panel) because it acts on the agent this page
                    configures; conversing with it belongs in the real chat. */}
                <Box>
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    justifyContent="space-between"
                    alignItems={{ xs: 'flex-start', sm: 'center' }}
                    spacing={1}
                  >
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
                      <Typography variant="caption" color="text.secondary">
                        {agentOnline ? 'Running' : 'Stopped'}
                      </Typography>
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
                </Box>

                <AgentRestartRequiredBanner
                  visible={!!bot.restart_required}
                  working={!!chatSessionId && streaming.currentResponses.has(chatSessionId)}
                  busy={activateAgent.isPending || stopAgent.isPending || restartAgent.isPending}
                  onRestart={() => { void handleRestartSession() }}
                />

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
                  {/* This is a prose editor, so Monaco's overview ruler (the
                      decoration strip drawn in the scrollbar gutter) carries
                      nothing worth showing — it only rendered a coloured line
                      beside the scrollbar. Zero lanes drops the decorations,
                      and the border/cursor marks go with it. */}
                  <MonacoEditor
                    value={content}
                    onChange={setContent}
                    onSave={handleSave}
                    language="markdown"
                    minHeight={240}
                    maxHeight={600}
                    autoHeight={true}
                    theme="helix-dark"
                    options={{
                      overviewRulerLanes: 0,
                      overviewRulerBorder: false,
                      hideCursorInOverviewRuler: true,
                    }}
                  />
                </Box>

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Project access</Typography>
                  <Autocomplete
                    multiple
                    disableCloseOnSelect
                    options={projects}
                    value={projects.filter((project) => !!project.id && projectIDs.includes(project.id))}
                    onChange={(_e, value) => {
                      const selected = value.map((project) => project.id).filter((id): id is string => !!id)
                      setProjectIDs(Array.from(new Set([...(projectID ? [projectID] : []), ...selected])))
                    }}
                    getOptionDisabled={(project) => project.id === projectID}
                    getOptionLabel={(project) => project.name || project.id || 'Unnamed project'}
                    isOptionEqualToValue={(a, b) => a.id === b.id}
                    renderOption={(props, option, { selected }) => {
                      const { key, ...liProps } = props as typeof props & { key?: Key }
                      return (
                        <li key={key ?? option.id} {...liProps}>
                          <Checkbox
                            icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
                            checkedIcon={<CheckBoxIcon fontSize="small" />}
                            sx={{ mr: 1 }}
                            checked={selected}
                          />
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2">{option.name || option.id}</Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                              {option.id}{option.id === projectID ? ' · own project (default)' : ''}
                            </Typography>
                          </Box>
                        </li>
                      )
                    }}
                    renderTags={(value, getTagProps) => value.map((option, index) => {
                      const { key, onDelete, ...tagProps } = getTagProps({ index })
                      const ownProject = option.id === projectID
                      return (
                        <Chip
                          key={key ?? option.id}
                          {...tagProps}
                          onDelete={ownProject ? undefined : onDelete}
                          label={`${option.name || option.id}${ownProject ? ' (default)' : ''}`}
                          size="small"
                        />
                      )
                    })}
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        placeholder="Select projects"
                        helperText="The agent's own project is always allowed and is used when a tool omits project_id. Other projects must be selected here."
                      />
                    )}
                  />
                </Box>

                <Box>
                  <Paper variant="outlined" sx={{ p: 2 }}>
                    <Stack spacing={1.5}>
                      <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
                        <Box>
                          <Typography variant="subtitle2">Tools</Typography>
                          <Typography variant="caption" color="text.secondary">
                            {tools.length} MCP {tools.length === 1 ? 'capability' : 'capabilities'} enabled
                          </Typography>
                        </Box>
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={<EditOutlinedIcon />}
                          onClick={() => setEditingTools(true)}
                        >
                          Edit tools
                        </Button>
                      </Stack>
                      {tools.length > 0 ? (
                        <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
                          {[...tools].sort().slice(0, 8).map((toolName) => (
                            <Chip
                              key={toolName}
                              label={toolName}
                              size="small"
                              sx={{ fontFamily: 'monospace' }}
                            />
                          ))}
                          {tools.length > 8 && (
                            <Chip label={`+${tools.length - 8} more`} size="small" variant="outlined" />
                          )}
                        </Stack>
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          No tools selected. The agent can still receive owner chat, but cannot call Helix Org MCP capabilities.
                        </Typography>
                      )}
                      <Typography variant="caption" color="text.secondary">
                        Changes made in the tool picker are saved with the rest of this agent configuration.
                      </Typography>
                    </Stack>
                  </Paper>
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
                    By default each trigger wipes the agent's session so every turn
                    starts on a fresh context window. Enable this to keep the
                    conversation across triggers — faster, more context-aware
                    follow-ups (e.g. for Slack), at the cost of the session
                    growing toward the model's context limit (where compaction
                    kicks in). Durable state still belongs in the agent's git
                    workspace, not the chat history.
                  </Typography>
                </Box>

                <AttachmentsPanel botID={bot?.id} />
                <Divider sx={{ my: 2 }} />
                <WorkerSecretsPanel agentID={bot?.id} projectID={projectID} />
              </Stack>
            </Grid>

            {/* Right rail: identity context, runtime config, and the
                destructive actions (reset instructions / delete). */}
            <Grid item xs={12} md={4}>
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
                  {/* The agent app id is machine detail nobody reads — the
                      only useful thing is getting to the agent, so the label
                      itself is the link. Without an org slug there is no
                      route to build, so the row is dropped entirely. */}
                  {agentID && orgSlug && (
                    <Box>
                      <Link
                        href={router5.buildPath('org_agent', { org_id: orgSlug, app_id: agentID })}
                        target="_blank"
                        rel="noopener noreferrer"
                        underline="hover"
                        sx={{ fontSize: '0.8rem', display: 'inline-flex', alignItems: 'center', gap: 0.5 }}
                      >
                        Agent
                        <OpenInNewIcon sx={{ fontSize: 14, flexShrink: 0 }} />
                      </Link>
                    </Box>
                  )}

                  {/* Runtime / credentials / model / effort. Lives in the
                      rail with the other agent-level controls rather than
                      in the editing column, which is for the prompt. */}
                  {agentID && (
                    <>
                      <Divider />
                      <AgentConfigForm
                        value={runtimeConfig}
                        showReasoningEffort
                        onChange={(patch) => setRuntimeConfig((current) => ({ ...current, ...patch }))}
                      />
                    </>
                  )}

                  <Divider />
                  {defaultInstructions && (
                    <>
                      <Button
                        variant="outlined"
                        startIcon={<SettingsBackupRestoreIcon />}
                        onClick={() => setConfirmingResetInstructions(true)}
                        disabled={updateBot.isPending}
                        fullWidth
                      >
                        Reset instructions
                      </Button>
                      <Typography variant="caption" color="text.secondary">
                        Restores this agent's built-in default instructions,
                        discarding local edits to them. Tools, subscriptions,
                        and reporting lines are untouched.
                      </Typography>
                      <Divider />
                    </>
                  )}
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteOutlineIcon />}
                    onClick={() => setConfirmingDelete(true)}
                    disabled={del.isPending}
                    fullWidth
                  >
                    Delete agent
                  </Button>
                  <Typography variant="caption" color="text.secondary">
                    Deletes the canonical Agent configuration and knowledge,
                    tears down its Helix project, and drops its subscriptions
                    and reporting lines.
                  </Typography>
                </Stack>
              </Paper>
            </Grid>
          </Grid>

          </>
        )}
      </Container>

      {/* Reset is destructive to the current prompt but nothing else, so a
          plain confirm is enough — no type-the-word gate like delete. */}
      <Dialog
        open={confirmingResetInstructions}
        onClose={() => setConfirmingResetInstructions(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Reset instructions</DialogTitle>
        <DialogContent>
          <Typography variant="body1">
            This replaces the instructions for{' '}
            <b style={{ fontFamily: 'monospace' }}>{botId}</b> with its built-in
            default prompt. Any edits you have made to the instructions are
            lost and cannot be recovered.
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            The agent's tools, subscriptions, reporting lines, runtime, and
            project access are not affected. The new instructions apply on the
            agent's next activation.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setConfirmingResetInstructions(false)}
            disabled={updateBot.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={() => { void handleResetInstructions() }}
            disabled={updateBot.isPending}
          >
            {updateBot.isPending ? 'Resetting…' : 'Reset instructions'}
          </Button>
        </DialogActions>
      </Dialog>

      <ToolPickerDialog
        open={editingTools}
        tools={toolOptions}
        selectedTools={tools}
        onClose={() => setEditingTools(false)}
        onApply={setTools}
      />

      {confirmingDelete && botId && (
        <DeleteConfirmWindow
          title="agent"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setConfirmingDelete(false)}
        >
          <Typography variant="body1">
            Deleting agent <b style={{ fontFamily: 'monospace' }}>{botId}</b> deletes its
            canonical Agent configuration and knowledge sources, tears down its
            Helix project, and clears its subscriptions, reporting lines, and runtime state.
            This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
          </Box>
    </HelixOrgShell>
  )
}

type SourceOption = { key: string; label: string; description: string; source: { kind: string; trigger_id?: string; processor_id?: string; output_id?: string } }

const AttachmentsPanel: FC<{ botID?: string }> = ({ botID }) => {
  const snackbar = useSnackbar()
  const { data: triggers = [], isLoading: triggersLoading } = useTriggers()
  const { data: processors = [], isLoading: processorsLoading } = useListHelixOrgProcessors()
  const { data: attachments = [], isLoading: attachmentsLoading } = useAgentAttachments(botID)
  const attach = useCreateAgentAttachment(botID)
  const detach = useDeleteAgentAttachment(botID)
  const options = useMemo<SourceOption[]>(() => [
    ...triggers.map((trigger) => ({ key: `trigger:${trigger.id}`, label: trigger.name || trigger.id!, description: `Trigger · ${trigger.kind}`, source: { kind: 'trigger', trigger_id: trigger.id } })),
    ...processors.flatMap((processor) => processor.outputs.map((output) => ({ key: `processor_output:${processor.id}:${output.id}`, label: `${processor.name} · ${output.label || output.id}`, description: `Processed event · ${output.id}`, source: { kind: 'processor_output', processor_id: processor.id, output_id: output.id } }))),
  ], [triggers, processors])
  const attachmentKey = (source: { kind?: string; trigger_id?: string; processor_id?: string; output_id?: string }) => source.kind === 'trigger' ? `trigger:${source.trigger_id}` : `processor_output:${source.processor_id}:${source.output_id}`
  const selectedKeys = useMemo(() => new Set(attachments.map((item) => attachmentKey(item.source ?? {}))), [attachments])
  const selected = useMemo(() => options.filter((option) => selectedKeys.has(option.key)), [options, selectedKeys])

  if (!botID) {
    return null
  }

  const handleChange = async (_e: unknown, next: SourceOption[]) => {
    const nextKeys = new Set(next.map((source) => source.key))
    const toAdd = next.filter((source) => !selectedKeys.has(source.key))
    const toRemove = attachments.filter((item) => !nextKeys.has(attachmentKey(item.source ?? {})))
    try {
      for (const source of toAdd) await attach.mutateAsync({ source: source.source })
      for (const item of toRemove) await detach.mutateAsync(item.id!)
      if (toAdd.length || toRemove.length) {
        snackbar.success(`Triggers updated (${toAdd.length} added, ${toRemove.length} removed)`)
      }
    } catch (err: any) {
      snackbar.error(err?.response?.data?.summary ?? err?.message ?? 'Could not update triggers')
    }
  }

  return (
    <Box>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Triggers ({selected.length})
      </Typography>
      <Autocomplete
        multiple
        disableCloseOnSelect
        loading={triggersLoading || processorsLoading || attachmentsLoading}
        options={options}
        value={selected}
        onChange={handleChange}
        getOptionLabel={(source) => source.label}
        isOptionEqualToValue={(a, b) => a.key === b.key}
        renderOption={(props, option, { selected }) => {
          // Pass key explicitly rather than via the props spread —
          // React 18.3 warns when a spread object carries a key.
          const { key, ...liProps } = props as typeof props & { key?: Key }
          return (
            <li key={key ?? option.key} {...liProps}>
              <Checkbox checked={selected} sx={{ mr: 1 }} />
              <Box>
                <Typography variant="body2">{option.label}</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>{option.description}</Typography>
              </Box>
            </li>
          )
        }}
        renderInput={(params) => (
          <TextField
            {...params}
            placeholder={selected.length === 0 ? 'Choose triggers or processed events…' : ''}
            variant="outlined"
            size="small"
          />
        )}
        renderTags={(value, getTagProps) =>
          value.map((option, index) => {
            const { key, ...tagProps } = getTagProps({ index })
            return (
              <Chip
                key={key ?? option.key}
                {...tagProps}
                label={option.label}
                size="small"
              />
            )
          })
        }
      />
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
        This agent starts when any selected Trigger or processed event occurs.
      </Typography>
    </Box>
  )
}

export default HelixOrgBotDetail
