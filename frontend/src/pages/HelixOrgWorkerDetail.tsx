// HelixOrgWorkerDetail shows a single worker and lets the operator
// chat to it in a fresh session. "Fresh" matters — the user explicitly
// asked for no session history on this page; clicking the chat button
// always opens a new conversation against the worker's per-Worker
// agent app.
//
// Implementation: each "Start new chat" click navigates to the agent
// app's chat page (/orgs/<org>/agent/<app_id>) WITHOUT a session_id
// query param. The agent page's default behaviour for a missing
// session id is to show an empty composer ready for a new session.
// By avoiding any session_id pinning we guarantee no transcript bleed
// across worker visits.

import { FC, useMemo, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import CircularProgress from '@mui/material/CircularProgress'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Grid from '@mui/material/Grid'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import ChatBubbleOutlineIcon from '@mui/icons-material/ChatBubbleOutline'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import PowerSettingsNewIcon from '@mui/icons-material/PowerSettingsNew'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useActivateWorker,
  useEnsureWorkerChat,
  useFireHelixOrgWorker,
  useHelixOrgWorker,
  useListHelixOrgStreams,
  useListPositionSubscriptions,
  useSubscribePosition,
  useUnsubscribePosition,
} from '../services/helixOrgService'

const OWNER_WORKER = 'w-owner'

const HelixOrgWorkerDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const api = useApi()
  const orgSlug = router.params.org_id as string | undefined
  const workerId = router.params.worker_id as string | undefined

  const { data, isLoading } = useHelixOrgWorker(workerId)
  const fire = useFireHelixOrgWorker()
  const ensureChat = useEnsureWorkerChat()
  const activate = useActivateWorker()
  const [confirmingFire, setConfirmingFire] = useState(false)

  const isOwner = workerId === OWNER_WORKER
  const worker = data?.worker
  const projectID = data?.project_id

  // launching covers the whole openChat flow, not just the
  // ensureChat mutation. The user-facing "5 seconds of nothing"
  // before was because ensureChat.isPending only spans the
  // /workers/{id}/chat call; the remaining work (GET exploratory
  // session, optional POST create, optional POST resume, finally
  // the window.open) all ran while the button looked idle. Now the
  // button stays disabled with a spinner from the first click
  // through to the new tab opening.
  const [launching, setLaunching] = useState(false)

  // openChat takes the operator to the Human Desktop for the Worker's
  // per-Worker Helix project, matching how chat works for regular
  // projects in the rest of the app (the desktop session IS the chat
  // surface — Zed talks to Claude Code inside it). The owner Worker
  // has no project on bootstrap (the spawner provisions one only on
  // AI-Worker activation), so the first click POSTs /workers/{id}/chat
  // to fast-path through WorkerProject.Ensure and get the project_id
  // back; later clicks already have it from the worker detail fetch.
  //
  // The exploratory session is the project's single long-lived "Human
  // Desktop" — we start it on demand (idempotent server-side when
  // already running) and resume from a paused state if needed, then
  // open /orgs/<org>/projects/<projectID>/desktop/<sessionID> in a
  // NEW TAB so the operator keeps the worker detail page as the
  // home base they can fire / inspect from while the desktop is
  // up.
  const openChat = async () => {
    if (!orgSlug || !workerId) return
    if (launching) return
    setLaunching(true)
    try {
      let pid = projectID
      if (!pid) {
        try {
          const resp = await ensureChat.mutateAsync(workerId)
          pid = resp.project_id
        } catch (err: any) {
          snackbar.error(err?.response?.data?.error ?? err?.message ?? 'provisioning chat failed')
          return
        }
      }
      if (!pid) {
        snackbar.error('no project id returned for worker')
        return
      }

      const apiClient = api.getApiClient()
      let session: { id?: string; config?: { external_agent_status?: string } } | null = null
      try {
        const resp = await apiClient.v1ProjectsExploratorySessionDetail(pid)
        session = resp.data ?? null
      } catch (err: any) {
        if (err?.response?.status !== 204) {
          snackbar.error(err?.response?.data?.error ?? err?.message ?? 'failed to open Human Desktop')
          return
        }
      }
      if (!session?.id) {
        const created = await apiClient.v1ProjectsExploratorySessionCreate(pid)
        session = created.data
      } else if (session.config?.external_agent_status === 'stopped') {
        // Paused desktop — kick it back to running before navigating so
        // the user doesn't land on a dead viewer.
        await apiClient.v1SessionsResumeCreate(session.id)
      }
      if (!session?.id) {
        snackbar.error('failed to open Human Desktop session')
        return
      }
      const url = `/orgs/${encodeURIComponent(orgSlug)}/projects/${encodeURIComponent(pid)}/desktop/${encodeURIComponent(session.id)}`
      window.open(url, '_blank', 'noopener,noreferrer')
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'failed to open Human Desktop')
    } finally {
      setLaunching(false)
    }
  }

  // handleRestartDesktop manually triggers a fresh activation for this
  // Worker. The /activate endpoint runs ensureProject synchronously
  // (which re-attaches the helix-org MCP that helix's project-apply
  // wipes on update), then enqueues a manual trigger; the spawner
  // picks it up, opens or resumes the per-Worker chat session, and
  // helix spins the desktop container back up as part of session
  // start. The intended use case: the operator stopped the desktop
  // ("reset it") and wants it back online with the MCP correctly
  // attached. Plain "resume" doesn't re-attach because it only
  // restarts the container — Config.Helix isn't touched.
  //
  // We stay on this page rather than navigating: the user's desktop
  // tab may already be open in another window; if not, "Open Human
  // Desktop" above is one click away.
  const handleRestartDesktop = async () => {
    if (!workerId) return
    if (activate.isPending) return
    try {
      await activate.mutateAsync(workerId)
      snackbar.success('Activation queued — desktop will come up shortly')
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'activate failed')
    }
  }

  const handleFire = async () => {
    if (!workerId) return
    try {
      await fire.mutateAsync(workerId)
      snackbar.success(`fired ${workerId}`)
      if (orgSlug) {
        router.navigate('helix_org_workers', { org_id: orgSlug })
      }
    } catch (err: any) {
      const status = err?.response?.status
      if (status === 409) {
        snackbar.error('owner worker is protected and cannot be fired')
      } else {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'fire failed')
      }
    } finally {
      setConfirmingFire(false)
    }
  }

  return (
    <Page
      breadcrumbTitle={workerId ?? 'Worker'}
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <Stack direction="row" spacing={1}>
          <Button
            startIcon={<ArrowBackIcon />}
            onClick={() => orgSlug && router.navigate('helix_org_workers', { org_id: orgSlug })}
          >
            Workers
          </Button>
          <Button
            variant="contained"
            color="secondary"
            startIcon={launching ? <CircularProgress size={16} color="inherit" /> : <ChatBubbleOutlineIcon />}
            onClick={openChat}
            disabled={launching}
          >
            {launching ? 'Launching Human Desktop…' : 'Start new chat'}
          </Button>
        </Stack>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        {isLoading || !worker ? (
          <LoadingSpinner />
        ) : (
          <Grid container spacing={3}>
            <Grid item xs={12} md={9}>
              <Stack spacing={3}>
                <Box>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    {worker.kind === 'ai' ? (
                      <SmartToyOutlinedIcon />
                    ) : (
                      <PersonOutlineIcon />
                    )}
                    <Typography variant="h5" sx={{ fontFamily: 'monospace' }}>
                      {worker.id}
                    </Typography>
                    <Chip size="small" label={worker.kind} />
                    {isOwner && <Chip size="small" label="owner — protected" />}
                  </Stack>
                </Box>

                {/* Chat panel — no transcript, just the call to action. The
                    user asked for "no previous sessions"; the agent chat page
                    is where the actual transcript lives. */}
                <Paper variant="outlined" sx={{ p: 3 }}>
                  <Stack spacing={2} alignItems="flex-start">
                    <Typography variant="subtitle1">Chat with this worker</Typography>
                    <Typography variant="body2" color="text.secondary">
                      Opens the worker's Human Desktop in a new tab. The first click on
                      a never-chatted-with worker provisions the agent app + project on
                      the fly (takes a few seconds — the button shows a spinner and the
                      label flips to "Launching Human Desktop…"); subsequent clicks open
                      the same session immediately.
                    </Typography>
                    <Stack direction="row" spacing={1} flexWrap="wrap">
                      <Button
                        variant="contained"
                        color="secondary"
                        startIcon={launching ? <CircularProgress size={16} color="inherit" /> : <ChatBubbleOutlineIcon />}
                        onClick={openChat}
                        disabled={launching}
                      >
                        {launching
                          ? 'Launching Human Desktop…'
                          : (projectID ? 'Open Human Desktop' : 'Provision + open Human Desktop')}
                      </Button>
                      {/* Restart Desktop kicks a fresh manual
                          activation. Re-attaches the helix-org MCP
                          and brings the container back up after a
                          manual stop. Every identity has the same
                          chat+desktop surface, so the button shows
                          for all workers — human and AI — once the
                          worker has been provisioned at least once
                          (no agent app to activate against
                          otherwise). */}
                      <Button
                        variant="outlined"
                        color="secondary"
                        startIcon={activate.isPending ? <CircularProgress size={16} color="inherit" /> : <PowerSettingsNewIcon />}
                        onClick={handleRestartDesktop}
                        disabled={activate.isPending || !projectID}
                      >
                        {activate.isPending ? 'Activating…' : 'Restart Desktop'}
                      </Button>
                    </Stack>
                    {!projectID && (
                      <Typography variant="caption" color="text.secondary">
                        Restart Desktop is available after the first "Open Human Desktop".
                      </Typography>
                    )}
                  </Stack>
                </Paper>

                {worker.identity_content && (
                  <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>Identity</Typography>
                    <Box
                      component="pre"
                      sx={{
                        p: 1.5,
                        borderRadius: 1,
                        backgroundColor: (theme) => theme.palette.mode === 'light' ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.04)',
                        fontSize: '0.8rem',
                        whiteSpace: 'pre-wrap',
                        fontFamily: 'monospace',
                        maxHeight: 360,
                        overflow: 'auto',
                      }}
                    >
                      {worker.identity_content}
                    </Box>
                  </Box>
                )}

                {worker.tools && worker.tools.length > 0 && (
                  <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Tool grants ({worker.tools.length})
                    </Typography>
                    <Stack direction="row" flexWrap="wrap" gap={0.5}>
                      {worker.tools.map((t) => (
                        <Chip key={t} label={t} size="small" sx={{ fontFamily: 'monospace' }} />
                      ))}
                    </Stack>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                      Grants on this specific worker (rows in <code>org_grants</code>) — distinct from
                      the role's tool manifest.
                    </Typography>
                  </Box>
                )}

                <SubscriptionsPanel positionID={data?.position?.id} />
              </Stack>
            </Grid>

            {/* Right rail: role / position context + Fire action */}
            <Grid item xs={12} md={3}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={2}>
                  <Box>
                    <Typography variant="caption" color="text.secondary">ID</Typography>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{worker.id}</Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">Kind</Typography>
                    <Typography variant="body2">{worker.kind}</Typography>
                  </Box>
                  {data?.position && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Position</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                        {data.position.id}
                      </Typography>
                    </Box>
                  )}
                  {data?.role && (
                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                        Role
                      </Typography>
                      <Button
                        size="small"
                        variant="text"
                        onClick={() => orgSlug && router.navigate('helix_org_role_detail', { org_id: orgSlug, role_id: data.role!.id })}
                        sx={{ fontFamily: 'monospace', textTransform: 'none', justifyContent: 'flex-start', p: 0, minWidth: 0 }}
                      >
                        {data.role.id}
                      </Button>
                    </Box>
                  )}
                  {projectID && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Project</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>
                        {projectID}
                      </Typography>
                    </Box>
                  )}
                  <Divider />
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteOutlineIcon />}
                    onClick={() => setConfirmingFire(true)}
                    disabled={isOwner || fire.isPending}
                    fullWidth
                  >
                    {isOwner ? 'Owner — protected' : 'Fire worker'}
                  </Button>
                  <Typography variant="caption" color="text.secondary">
                    Tears down the worker's per-Worker Helix project, removes grants,
                    deletes the row.
                  </Typography>
                </Stack>
              </Paper>
            </Grid>
          </Grid>
        )}
      </Container>

      {confirmingFire && workerId && (
        <DeleteConfirmWindow
          title="worker"
          submitTitle="Fire"
          onSubmit={handleFire}
          onCancel={() => setConfirmingFire(false)}
        >
          <Typography variant="body1">
            Firing worker <b style={{ fontFamily: 'monospace' }}>{workerId}</b> tears down its
            per-worker Helix project + agent app and removes its grants. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
    </Page>
  )
}

// SubscriptionsPanel surfaces the streams the Worker's filling
// Position consumes — and the multi-select to change that set.
// Subscriptions are position-anchored: editing here mutates the
// position's subscriptions, which automatically applies to whoever
// fills the position next.
//
// Patterned after the role editor's tools multi-select:
// disableCloseOnSelect so toggling several streams in one pass
// doesn't bounce the popper closed.
const SubscriptionsPanel: FC<{ positionID?: string }> = ({ positionID }) => {
  const snackbar = useSnackbar()
  const { data: streamsData, isLoading: streamsLoading } = useListHelixOrgStreams()
  const { data: subsData, isLoading: subsLoading } = useListPositionSubscriptions(positionID)
  const subscribe = useSubscribePosition(positionID)
  const unsubscribe = useUnsubscribePosition(positionID)

  const allStreams = streamsData?.streams ?? []
  const subscribedIDs = useMemo(
    () => new Set((subsData?.subscriptions ?? []).map((s) => s.stream_id)),
    [subsData],
  )
  const subscribedStreams = useMemo(
    () => allStreams.filter((s) => subscribedIDs.has(s.id)),
    [allStreams, subscribedIDs],
  )

  if (!positionID) {
    return (
      <Box>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>Subscriptions</Typography>
        <Typography variant="body2" color="text.secondary">
          This Worker is unassigned (no position) — there's nothing to subscribe.
        </Typography>
      </Box>
    )
  }

  const handleChange = async (_e: unknown, next: typeof allStreams) => {
    const nextIDs = new Set(next.map((s) => s.id))
    const toAdd = next.filter((s) => !subscribedIDs.has(s.id))
    const toRemove = (subsData?.subscriptions ?? []).filter((s) => !nextIDs.has(s.stream_id))
    try {
      for (const s of toAdd) await subscribe.mutateAsync(s.id)
      for (const s of toRemove) await unsubscribe.mutateAsync(s.stream_id)
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
        Subscriptions ({subscribedStreams.length})
      </Typography>
      <Autocomplete
        multiple
        disableCloseOnSelect
        loading={streamsLoading || subsLoading}
        options={allStreams}
        value={subscribedStreams}
        onChange={handleChange}
        getOptionLabel={(s) => s.id}
        isOptionEqualToValue={(a, b) => a.id === b.id}
        renderOption={(props, option, { selected }) => (
          <li {...props}>
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
        )}
        renderInput={(params) => (
          <TextField
            {...params}
            placeholder={subscribedStreams.length === 0 ? 'Subscribe this position to a stream…' : ''}
            variant="outlined"
            size="small"
          />
        )}
        renderTags={(value, getTagProps) =>
          value.map((option, index) => (
            <Chip
              {...getTagProps({ index })}
              key={option.id}
              label={option.id}
              size="small"
              sx={{ fontFamily: 'monospace' }}
            />
          ))
        }
      />
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
        Subscriptions are position-anchored — they outlive the worker. Whoever fills
        <code style={{ marginLeft: 4, marginRight: 4 }}>{positionID}</code>
        next inherits this set.
      </Typography>
    </Box>
  )
}

export default HelixOrgWorkerDetail
