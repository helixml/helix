import { FC, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import SaveIcon from '@mui/icons-material/Save'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'

import Page from '../components/system/Page'
import PageLoader from '../components/widgets/PageLoader'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  EventCard,
  helixOrgStreamEventsUrl,
  useHelixOrgWorker,
  useUpdateHelixOrgWorkerIdentity,
  useUpdateHelixOrgWorkerRole,
} from '../services/helixOrgService'

// HelixOrgWorkerDetail surfaces a single Worker with two editable
// markdown fields (identity + the role of its Position) and a live SSE
// tail of its activation stream. Save buttons fire POSTs against the
// JSON API; the stream tail attaches to /api/v1/org/streams/{id}/events
// (cookie-authenticated EventSource — no custom headers needed).

const ActivationStream: FC<{ workerId: string }> = ({ workerId }) => {
  const [events, setEvents] = useState<EventCard[]>([])
  const [connected, setConnected] = useState(false)
  const sourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    const url = helixOrgStreamEventsUrl(`s-activations-${workerId}`)
    const es = new EventSource(url, { withCredentials: true })
    sourceRef.current = es
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.addEventListener('message', (ev) => {
      try {
        const payload = JSON.parse((ev as MessageEvent).data) as EventCard[]
        if (Array.isArray(payload)) setEvents(payload)
      } catch {
        // Ignore non-JSON keepalives.
      }
    })
    return () => {
      es.close()
      sourceRef.current = null
    }
  }, [workerId])

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
        <Typography variant="subtitle2">Activation events (live)</Typography>
        <Typography variant="caption" color="text.secondary">
          stream: s-activations-{workerId} · {connected ? 'connected' : 'disconnected'}
        </Typography>
      </Stack>
      {events.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No events yet. They will appear here as the Spawner activates this Worker.
        </Typography>
      ) : (
        <Stack divider={<Divider />} spacing={1}>
          {events.map((ev) => (
            <Box key={ev.id}>
              <Stack direction="row" spacing={1} alignItems="baseline">
                <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                  {ev.created_at}
                </Typography>
                {ev.source && (
                  <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                    {ev.source}
                  </Typography>
                )}
              </Stack>
              {ev.has_message && (
                <>
                  {ev.subject && (
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>{ev.subject}</Typography>
                  )}
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {ev.message_body || ev.body}
                  </Typography>
                </>
              )}
              {!ev.has_message && (
                <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                  {ev.body}
                </Typography>
              )}
            </Box>
          ))}
        </Stack>
      )}
    </Paper>
  )
}

const HelixOrgWorkerDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const workerId = router.params.worker_id as string | undefined

  const { data, isLoading } = useHelixOrgWorker(workerId)
  const updateIdentity = useUpdateHelixOrgWorkerIdentity(workerId ?? '')
  const updateRole = useUpdateHelixOrgWorkerRole(workerId ?? '')

  const [identity, setIdentity] = useState('')
  const [roleContent, setRoleContent] = useState('')
  const [identityDirty, setIdentityDirty] = useState(false)
  const [roleDirty, setRoleDirty] = useState(false)

  useEffect(() => {
    if (data) {
      setIdentity(data.worker.identity_content ?? '')
      setIdentityDirty(false)
      setRoleContent(data.role?.content ?? '')
      setRoleDirty(false)
    }
  }, [data])

  if (!workerId) {
    return (
      <Page breadcrumbTitle="Worker" breadcrumbParent={{ title: 'Helix Org' }}>
        <Container maxWidth="lg" sx={{ py: 3 }}>
          <Typography>Missing worker id</Typography>
        </Container>
      </Page>
    )
  }

  if (isLoading || !data) {
    return (
      <Page breadcrumbTitle="Worker" breadcrumbParent={{ title: 'Helix Org' }}>
        <PageLoader message="Loading worker…" />
      </Page>
    )
  }

  const worker = data.worker
  const hasPosition = Boolean(worker.position_id)

  const handleSaveIdentity = async () => {
    try {
      await updateIdentity.mutateAsync(identity)
      setIdentityDirty(false)
      snackbar.success('Identity saved')
    } catch (e) {
      snackbar.error('Failed to save identity')
    }
  }

  const handleSaveRole = async () => {
    try {
      await updateRole.mutateAsync(roleContent)
      setRoleDirty(false)
      snackbar.success('Role saved')
    } catch (e) {
      snackbar.error('Failed to save role')
    }
  }

  return (
    <Page
      breadcrumbTitle={worker.id}
      breadcrumbs={[
        { title: 'Helix Org' },
        { title: 'Workers', routeName: 'helix_org_workers', params: {} },
      ]}
      topbarContent={(
        <Button
          variant="outlined"
          startIcon={<ArrowBackIcon />}
          onClick={() => router.navigate('helix_org_workers', {})}
        >
          Back
        </Button>
      )}
    >
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Stack spacing={3}>
          <Box>
            <Typography variant="h5" sx={{ fontFamily: 'monospace' }}>{worker.id}</Typography>
            <Typography variant="body2" color="text.secondary">
              Kind: {worker.kind}
              {worker.position_id && <> · Position: {worker.position_id}</>}
              {data.role && <> · Role: {data.role.id}</>}
            </Typography>
            {worker.organization_id && (
              <Typography variant="caption" color="text.secondary">
                Organization: {worker.organization_id}
              </Typography>
            )}
          </Box>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
              <Typography variant="subtitle2">Identity (per-Worker markdown)</Typography>
              <Button
                size="small"
                variant="contained"
                color="secondary"
                startIcon={<SaveIcon />}
                disabled={!identityDirty || updateIdentity.isPending}
                onClick={handleSaveIdentity}
              >
                {updateIdentity.isPending ? 'Saving…' : 'Save identity'}
              </Button>
            </Stack>
            <TextField
              multiline
              fullWidth
              minRows={6}
              value={identity}
              onChange={(e) => {
                setIdentity(e.target.value)
                setIdentityDirty(true)
              }}
              placeholder="# Identity\nThis Worker is…"
              InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.85rem' } }}
            />
          </Paper>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
              <Typography variant="subtitle2">
                Role markdown {hasPosition ? '' : '— unassigned Worker, no role to edit'}
              </Typography>
              <Button
                size="small"
                variant="contained"
                color="secondary"
                startIcon={<SaveIcon />}
                disabled={!hasPosition || !roleDirty || updateRole.isPending}
                onClick={handleSaveRole}
              >
                {updateRole.isPending ? 'Saving…' : 'Save role'}
              </Button>
            </Stack>
            <TextField
              multiline
              fullWidth
              minRows={8}
              value={roleContent}
              onChange={(e) => {
                setRoleContent(e.target.value)
                setRoleDirty(true)
              }}
              disabled={!hasPosition}
              placeholder="# Role\nResponsibilities…"
              InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.85rem' } }}
            />
          </Paper>

          <ActivationStream workerId={workerId} />
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgWorkerDetail
