// HelixOrgWorkerDetail shows a single worker and lets the operator
// watch and drive its conversation inline — the same transcript view
// the spec-task page uses (EmbeddedSessionView), reading the worker's
// per-Worker project "Human Desktop" session. The point is to avoid
// forcing the operator to click out to the external desktop tab just
// to see what the worker is doing.
//
// The inline transcript (EmbeddedSessionView + RobustPromptInput) is
// auto-shown when the worker's project already has an exploratory
// session. GET-only on load — never spins up infra by itself; sessions
// are provisioned by the worker's activation flow.
//
// The worker's identity markdown is editable here (Monaco + Save),
// persisted via the workers/{id}/identity endpoint.

import { FC, Key, useEffect, useMemo, useRef, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Grid from '@mui/material/Grid'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import EmbeddedSessionView, {
  EmbeddedSessionViewHandle,
} from '../components/session/EmbeddedSessionView'
import RobustPromptInput from '../components/common/RobustPromptInput'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import {
  useFireHelixOrgWorker,
  useHelixOrgWorker,
  useListHelixOrgStreams,
  useListWorkerSubscriptions,
  useSubscribeWorker,
  useUnsubscribeWorker,
} from '../services/helixOrgService'
import {
  WorkerChatReader,
  fetchExistingWorkerSession,
} from '../services/workerChatSession'

const OWNER_WORKER = 'w-owner'

const HelixOrgWorkerDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const api = useApi()
  const orgSlug = router.params.org_id as string | undefined
  const workerId = router.params.worker_id as string | undefined

  const fire = useFireHelixOrgWorker()
  // Stop polling/refetching this worker once a fire is in flight or
  // done — the row is being torn down, so a refetch would only hit a
  // 404 (QA F3). The page navigates to the workers list on success.
  const { data, isLoading } = useHelixOrgWorker(workerId, {
    enabled: !fire.isPending && !fire.isSuccess,
  })
  const streaming = useStreaming()
  const [confirmingFire, setConfirmingFire] = useState(false)

  const isOwner = workerId === OWNER_WORKER
  const worker = data?.worker
  const projectID = data?.project_id

  // chatSessionId is the worker's long-lived "Human Desktop" exploratory
  // session — the transcript we render inline. Null until we've resolved
  // one (or there isn't one yet).
  const [chatSessionId, setChatSessionId] = useState<string | null>(null)
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)

  // chatApi adapts the generated client to the read-only shape the
  // workerChatSession helper expects (we only GET the existing session
  // here — provisioning is owned by the worker's activation flow). The
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

  // Auto-load the inline transcript when the worker already has a project.
  // GET-only — we never create a session just because the operator opened
  // this page; sessions are provisioned by the worker's activation flow.
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

  // Subscribe the WebSocket to the inline session so in-flight turns stream
  // live (mirrors SpecTaskDetailContent, which likewise omits the streaming
  // context object from deps). Clear on unmount / session change.
  useEffect(() => {
    streaming.setCurrentSessionId(chatSessionId)
    return () => { streaming.setCurrentSessionId(null) }
  }, [chatSessionId])

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
      breadcrumbs={[{
        title: 'Workers',
        routeName: 'helix_org_workers',
        params: { org_id: orgSlug ?? '' },
        useOrgRouter: false,
      }]}
      organizationId={account.organizationTools.organization?.id}
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

                {/* Chat panel — inline transcript (same view the spec-task
                    page uses) plus the desktop launch buttons. The
                    transcript auto-loads when the worker already has a
                    session; otherwise the call to action provisions one. */}
                <Paper variant="outlined" sx={{ p: 3 }}>
                  <Stack spacing={2} alignItems="flex-start">
                    <Typography variant="subtitle1">Chat with this worker</Typography>
                    <Typography variant="body2" color="text.secondary">
                      The conversation below is the worker's Human Desktop session —
                      the same transcript you'd see in the desktop tab, including its
                      MCP tool calls. Send a message to drive it from here.
                    </Typography>

                    {/* Inline transcript. EmbeddedSessionView self-fetches
                        the session + interactions and live-streams in-flight
                        turns; it needs a bounded, flex-column container to
                        scroll within. RobustPromptInput drives the same
                        session via streaming.NewInference. */}
                    {chatSessionId ? (
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
                        />
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
                        No conversation yet for this worker.
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
                      Tools ({worker.tools.length})
                    </Typography>
                    <Stack direction="row" flexWrap="wrap" gap={0.5}>
                      {worker.tools.map((t) => (
                        <Chip key={t} label={t} size="small" sx={{ fontFamily: 'monospace' }} />
                      ))}
                    </Stack>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                      Derived from the Role's Tools list. Edit the Role to change what this
                      Worker can call.
                    </Typography>
                  </Box>
                )}

                <SubscriptionsPanel workerID={worker?.id} />
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
                  {(worker?.parent_ids ?? []).length > 0 && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Reports to</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                        {(worker?.parent_ids ?? []).join(', ')}
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
                    Tears down the worker's per-Worker Helix project and deletes the
                    row. Tool capability comes from the Role, so nothing extra to revoke.
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
            per-worker Helix project + agent app and clears its runtime state. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
    </Page>
  )
}

// SubscriptionsPanel surfaces the streams this Worker consumes — and
// the multi-select to change that set. Subscriptions are
// worker-anchored: firing the worker drops them; a new hire into the
// same Role does NOT inherit.
//
// Patterned after the role editor's tools multi-select:
// disableCloseOnSelect so toggling several streams in one pass
// doesn't bounce the popper closed.
const SubscriptionsPanel: FC<{ workerID?: string }> = ({ workerID }) => {
  const snackbar = useSnackbar()
  const { data: streamsData, isLoading: streamsLoading } = useListHelixOrgStreams()
  const { data: subsData, isLoading: subsLoading } = useListWorkerSubscriptions(workerID)
  const subscribe = useSubscribeWorker(workerID)
  const unsubscribe = useUnsubscribeWorker(workerID)

  const allStreams = streamsData?.streams ?? []
  const subscribedIDs = useMemo(
    () => new Set((subsData?.subscriptions ?? []).map((s) => s.stream_id)),
    [subsData],
  )
  const subscribedStreams = useMemo(
    () => allStreams.filter((s) => subscribedIDs.has(s.id)),
    [allStreams, subscribedIDs],
  )

  if (!workerID) {
    return null
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
            placeholder={subscribedStreams.length === 0 ? 'Subscribe this worker to a stream…' : ''}
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
        Subscriptions are per-Worker — they die when this Worker is fired. A
        new hire into the same Role won't inherit them.
      </Typography>
    </Box>
  )
}

export default HelixOrgWorkerDetail
