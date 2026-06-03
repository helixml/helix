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
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useFireHelixOrgWorker,
  useHelixOrgWorker,
} from '../services/helixOrgService'

const OWNER_WORKER = 'w-owner'

const HelixOrgWorkerDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const orgSlug = router.params.org_id as string | undefined
  const workerId = router.params.worker_id as string | undefined

  const { data, isLoading } = useHelixOrgWorker(workerId)
  const fire = useFireHelixOrgWorker()
  const [confirmingFire, setConfirmingFire] = useState(false)

  const isOwner = workerId === OWNER_WORKER
  const worker = data?.worker
  const agentAppID = data?.agent_app_id

  // openChat navigates to the agent app's chat surface WITHOUT pinning
  // a session id, so the agent page renders its empty composer ready
  // for a fresh conversation. Each click on either button gets a new
  // session-id from the backend on first message — the URL itself
  // carries no history.
  const openChat = () => {
    if (!orgSlug || !agentAppID) return
    router.navigate('org_agent', { org_id: orgSlug, app_id: agentAppID })
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
            disabled={!agentAppID}
          >
            Start new chat
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
                      Opens the worker's agent app in a brand-new chat session. Previous
                      sessions live on the agent page itself — this surface always lands
                      you in an empty composer.
                    </Typography>
                    <Button
                      variant="contained"
                      color="secondary"
                      startIcon={<ChatBubbleOutlineIcon />}
                      onClick={openChat}
                      disabled={!agentAppID}
                    >
                      {agentAppID ? 'Start new chat' : 'No agent app yet — chat once the worker has activated'}
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
                  {agentAppID && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">Agent app</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>
                        {agentAppID}
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
