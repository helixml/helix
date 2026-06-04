// HelixOrgStreamDetail is the per-stream "messages flowing through"
// view. It hydrates from GET /streams/{id} for the initial snapshot
// then keeps the event list live via the SSE endpoint at
// /streams/{id}/events — every push replaces the list wholesale so
// the frontend never has to diff partial updates. The shape mirrors
// what the old htmx /ui/streams?id=… surface used to render.

import { FC, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { EventCard, useHelixOrgStream } from '../services/helixOrgService'

const HelixOrgStreamDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const orgSlug = router.params.org_id as string | undefined
  const streamId = router.params.stream_id as string | undefined

  const { data: stream, isLoading } = useHelixOrgStream(streamId)

  // Live event list. Seeded from the initial GET so the page renders
  // immediately; replaced wholesale on every SSE push from
  // /streams/{id}/events. Falling back to the initial snapshot keeps
  // the list non-empty across reconnect blips.
  const [liveEvents, setLiveEvents] = useState<EventCard[] | null>(null)
  const events = liveEvents ?? stream?.recent_events ?? []

  // SSE wiring. EventSource sends `Cookie` automatically, so the
  // browser session auth flows through without extra headers. The
  // server emits `event: message` frames with a JSON-array payload of
  // up to 50 events newest-first; we replace state on each.
  const orgID = account.organizationTools.organization?.id || orgSlug || ''
  const sseUrlRef = useRef<string | null>(null)
  useEffect(() => {
    if (!orgID || !streamId) return
    const url = `/api/v1/orgs/${encodeURIComponent(orgID)}/streams/${encodeURIComponent(streamId)}/events`
    sseUrlRef.current = url
    const es = new EventSource(url, { withCredentials: true })
    const onMessage = (ev: MessageEvent) => {
      try {
        const arr = JSON.parse(ev.data) as EventCard[]
        if (Array.isArray(arr)) setLiveEvents(arr)
      } catch {
        // Malformed payload: drop, keep prior state. Next frame is
        // typically well-formed.
      }
    }
    es.addEventListener('message', onMessage)
    return () => {
      es.removeEventListener('message', onMessage)
      es.close()
    }
  }, [orgID, streamId])

  const subscribers = stream?.subscribers ?? []

  const backToList = () => {
    if (orgSlug) router.navigate('helix_org_streams', { org_id: orgSlug })
  }

  const formatTimestamp = (iso: string) => {
    if (!iso) return ''
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString()
  }

  return (
    <Page
      breadcrumbTitle={stream?.name || streamId || 'Stream'}
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Stack direction="row" alignItems="center" spacing={1}>
            <Button startIcon={<ArrowBackIcon />} variant="text" onClick={backToList}>
              Streams
            </Button>
          </Stack>

          {isLoading ? (
            <LoadingSpinner />
          ) : !stream ? (
            <Typography color="text.secondary">Stream not found.</Typography>
          ) : (
            <>
              <Box>
                <Stack direction="row" alignItems="baseline" spacing={2}>
                  <Typography variant="h5" sx={{ fontFamily: 'monospace' }}>{stream.id}</Typography>
                  <Chip label={stream.kind} size="small" sx={{ fontFamily: 'monospace' }} />
                </Stack>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  {stream.name}
                </Typography>
                {stream.description && (
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    {stream.description}
                  </Typography>
                )}
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1, fontFamily: 'monospace' }}>
                  created by {stream.created_by} · {formatTimestamp(stream.created_at)}
                </Typography>
                {subscribers.length > 0 && (
                  <Box sx={{ mt: 1 }}>
                    <Typography variant="caption" color="text.secondary">subscribers: </Typography>
                    {subscribers.map((s) => (
                      <Chip key={s} label={s} size="small" sx={{ fontFamily: 'monospace', mr: 0.5, mt: 0.25 }} />
                    ))}
                  </Box>
                )}
              </Box>

              <Divider />

              <Box>
                <Stack direction="row" alignItems="baseline" justifyContent="space-between" sx={{ mb: 1 }}>
                  <Typography variant="h6">Messages</Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                    newest first · up to 50 · live
                  </Typography>
                </Stack>
                {events.length === 0 ? (
                  <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
                    No events on this stream yet.
                  </Typography>
                ) : (
                  <Stack spacing={1}>
                    {events.map((ev) => (
                      <EventRow key={ev.id} ev={ev} formatTs={formatTimestamp} />
                    ))}
                  </Stack>
                )}
              </Box>
            </>
          )}
        </Stack>
      </Container>
    </Page>
  )
}

const EventRow: FC<{ ev: EventCard; formatTs: (iso: string) => string }> = ({ ev, formatTs }) => {
  const headerLine = useMemo(() => {
    const who = ev.from || ev.source || ''
    const to = ev.to ? ` → ${ev.to}` : ''
    return `${who}${to}`
  }, [ev.from, ev.source, ev.to])

  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Stack direction="row" alignItems="baseline" justifyContent="space-between" spacing={2}>
        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
          {headerLine || <span style={{ opacity: 0.6 }}>—</span>}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
          {formatTs(ev.created_at)}
        </Typography>
      </Stack>
      {ev.subject && (
        <Typography variant="subtitle2" sx={{ mt: 0.5 }}>{ev.subject}</Typography>
      )}
      {ev.has_message && ev.message_body ? (
        <Typography
          component="pre"
          variant="body2"
          sx={{ mt: 1, mb: 0, fontFamily: 'monospace', whiteSpace: 'pre-wrap', fontSize: '0.8rem' }}
        >
          {ev.message_body}
        </Typography>
      ) : (
        <Typography
          component="pre"
          variant="caption"
          color="text.secondary"
          sx={{ mt: 1, mb: 0, fontFamily: 'monospace', whiteSpace: 'pre-wrap', fontSize: '0.75rem' }}
        >
          {ev.body}
        </Typography>
      )}
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1, fontFamily: 'monospace', fontSize: '0.65rem' }}>
        {ev.id}
      </Typography>
    </Paper>
  )
}

export default HelixOrgStreamDetail
