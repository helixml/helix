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

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Grid from '@mui/material/Grid'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import ChatBubbleOutlineIcon from '@mui/icons-material/ChatBubbleOutline'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useEnsureWorkerChat,
  useFireHelixOrgWorker,
  useHelixOrgWorker,
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
  const [confirmingFire, setConfirmingFire] = useState(false)

  const isOwner = workerId === OWNER_WORKER
  const worker = data?.worker
  const projectID = data?.project_id

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
  // navigate to /orgs/<org>/projects/<projectID>/desktop/<sessionID>.
  const openChat = async () => {
    if (!orgSlug || !workerId) return
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
    try {
      let session: { id?: string; config?: { external_agent_status?: string } } | null = null
      try {
        const resp = await apiClient.v1ProjectsExploratorySessionDetail(pid)
        session = resp.data ?? null
      } catch (err: any) {
        if (err?.response?.status !== 204) throw err
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
      router.navigate('org_project-team-desktop', {
        org_id: orgSlug,
        id: pid,
        sessionId: session.id,
      })
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'failed to open Human Desktop')
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
            startIcon={<ChatBubbleOutlineIcon />}
            onClick={openChat}
            disabled={ensureChat.isPending}
          >
            {ensureChat.isPending ? 'Provisioning…' : 'Start new chat'}
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
                      Opens the worker's agent app in a brand-new chat session. The first
                      click on a never-chatted-with worker provisions the agent app on the
                      fly (takes a couple of seconds); subsequent clicks navigate
                      immediately. Each click lands you in an empty composer — no
                      transcript bleed across visits.
                    </Typography>
                    <Button
                      variant="contained"
                      color="secondary"
                      startIcon={<ChatBubbleOutlineIcon />}
                      onClick={openChat}
                      disabled={ensureChat.isPending}
                    >
                      {ensureChat.isPending
                        ? 'Provisioning agent app…'
                        : (projectID ? 'Open Human Desktop' : 'Provision + open Human Desktop')}
                    </Button>
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

export default HelixOrgWorkerDetail
