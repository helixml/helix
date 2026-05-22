import { FC, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import SendIcon from '@mui/icons-material/Send'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useSnackbar from '../hooks/useSnackbar'
import {
  EventCard,
  StreamDTO,
  helixOrgStreamEventsUrl,
  useHelixOrgStreams,
  usePublishHelixOrgStream,
} from '../services/helixOrgService'

// LiveEventTail attaches an EventSource to /api/v1/org/streams/{id}/events
// and replaces its in-memory event list whenever the SSR sends a new
// array (the API emits the full last-50-newest list on every wake).
const LiveEventTail: FC<{ streamId: string }> = ({ streamId }) => {
  const [events, setEvents] = useState<EventCard[]>([])
  const [connected, setConnected] = useState(false)
  const sourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!streamId) return
    const es = new EventSource(helixOrgStreamEventsUrl(streamId), { withCredentials: true })
    sourceRef.current = es
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.addEventListener('message', (ev) => {
      try {
        const payload = JSON.parse((ev as MessageEvent).data) as EventCard[]
        if (Array.isArray(payload)) setEvents(payload)
      } catch {
        // Ignore parse errors — server sends keepalives as comments.
      }
    })
    return () => {
      es.close()
      sourceRef.current = null
    }
  }, [streamId])

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
        <Typography variant="subtitle2">Live events</Typography>
        <Typography variant="caption" color="text.secondary">
          {connected ? 'connected' : 'disconnected'}
        </Typography>
      </Stack>
      {events.length === 0 ? (
        <Typography variant="body2" color="text.secondary">No events yet.</Typography>
      ) : (
        <Stack divider={<Divider />} spacing={1}>
          {events.map((ev) => (
            <Box key={ev.id}>
              <Stack direction="row" spacing={1} alignItems="baseline">
                <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                  {ev.created_at}
                </Typography>
                {ev.from && (
                  <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                    from {ev.from}
                  </Typography>
                )}
                {ev.to && (
                  <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                    to {ev.to}
                  </Typography>
                )}
              </Stack>
              {ev.has_message ? (
                <>
                  {ev.subject && (
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>{ev.subject}</Typography>
                  )}
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {ev.message_body || ev.body}
                  </Typography>
                </>
              ) : (
                <Typography
                  variant="body2"
                  sx={{ whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: '0.75rem' }}
                >
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

const PublishForm: FC<{ stream: StreamDTO }> = ({ stream }) => {
  const snackbar = useSnackbar()
  const publish = usePublishHelixOrgStream(stream.id)
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [to, setTo] = useState('')

  const disabled = !stream.can_publish

  const handleSubmit = async () => {
    if (!body.trim()) {
      snackbar.error('Body is required')
      return
    }
    try {
      await publish.mutateAsync({
        body,
        subject: subject.trim() || undefined,
        to: to.trim() ? to.split(',').map((s) => s.trim()).filter(Boolean) : undefined,
      })
      snackbar.success('Published')
      setSubject('')
      setBody('')
      setTo('')
    } catch (e) {
      snackbar.error('Failed to publish')
    }
  }

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>Publish to this stream</Typography>
      {!stream.can_publish && (
        <Typography variant="caption" color="warning.main" sx={{ display: 'block', mb: 1 }}>
          {stream.disable_reason || 'Inbound only'}
        </Typography>
      )}
      <Stack spacing={1}>
        <TextField
          size="small"
          label="Subject (optional)"
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          disabled={disabled}
        />
        <TextField
          size="small"
          label="To (comma-separated, optional)"
          value={to}
          onChange={(e) => setTo(e.target.value)}
          disabled={disabled}
        />
        <TextField
          multiline
          minRows={3}
          label="Body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          disabled={disabled}
        />
        <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
          <Button
            variant="contained"
            color="secondary"
            startIcon={<SendIcon />}
            disabled={disabled || publish.isPending || !body.trim()}
            onClick={handleSubmit}
          >
            {publish.isPending ? 'Publishing…' : 'Publish'}
          </Button>
        </Box>
      </Stack>
    </Paper>
  )
}

const HelixOrgStreams: FC = () => {
  const { data, isLoading } = useHelixOrgStreams()
  const streams = data?.streams ?? []
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // Auto-select the first stream once the list arrives.
  useEffect(() => {
    if (!selectedId && streams.length > 0) {
      setSelectedId(streams[0].id)
    }
  }, [selectedId, streams])

  const selected = useMemo(
    () => streams.find((s) => s.id === selectedId) ?? null,
    [selectedId, streams],
  )

  return (
    <Page breadcrumbTitle="Streams" breadcrumbParent={{ title: 'Helix Org' }}>
      <Container maxWidth="xl" sx={{ py: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Streams</Typography>
            <Typography variant="body2" color="text.secondary">
              Inboxes the org listens on. Select a stream to tail its events and (when supported) publish a message.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : streams.length === 0 ? (
            <Typography variant="body2" color="text.secondary">No streams yet.</Typography>
          ) : (
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '320px 1fr' }, gap: 2 }}>
              <Paper variant="outlined" sx={{ p: 0 }}>
                <List dense>
                  {streams.map((s) => (
                    <ListItemButton
                      key={s.id}
                      selected={selectedId === s.id}
                      onClick={() => setSelectedId(s.id)}
                    >
                      <ListItemText
                        primary={
                          <Stack direction="row" spacing={1} alignItems="center">
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>{s.name}</Typography>
                            <Chip size="small" label={s.kind} sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }} />
                          </Stack>
                        }
                        secondary={
                          <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                            {s.id}
                          </Typography>
                        }
                      />
                    </ListItemButton>
                  ))}
                </List>
              </Paper>

              <Stack spacing={2}>
                {selected ? (
                  <>
                    <Paper variant="outlined" sx={{ p: 2 }}>
                      <Typography variant="subtitle1" sx={{ fontFamily: 'monospace' }}>{selected.id}</Typography>
                      <Typography variant="body2" sx={{ mt: 0.5 }}>{selected.name}</Typography>
                      {selected.description && (
                        <Typography variant="caption" color="text.secondary">
                          {selected.description}
                        </Typography>
                      )}
                      <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
                        <Chip size="small" label={`kind: ${selected.kind}`} sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }} />
                        {selected.subscribers && selected.subscribers.length > 0 && (
                          <Chip
                            size="small"
                            label={`subscribers: ${selected.subscribers.join(', ')}`}
                            sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }}
                          />
                        )}
                      </Stack>
                    </Paper>
                    <LiveEventTail streamId={selected.id} />
                    <PublishForm stream={selected} />
                  </>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    Select a stream to view its events.
                  </Typography>
                )}
              </Stack>
            </Box>
          )}
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgStreams
