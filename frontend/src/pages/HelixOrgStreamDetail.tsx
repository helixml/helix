// HelixOrgStreamDetail is the per-stream "messages flowing through"
// view. It hydrates from GET /streams/{id} for the initial snapshot
// then keeps the event list live via the SSE endpoint at
// /streams/{id}/events — every push replaces the list wholesale so
// the frontend never has to diff partial updates. The shape mirrors
// what the old htmx /ui/streams?id=… surface used to render.
//
// The page also exposes inline editing of mutable fields (name,
// description, transport config) via PUT /streams/{id}. For the
// github transport the same Repository + Events picker as the New
// Stream dialog is shown; for other non-local transports a JSON
// textarea is offered.

import { FC, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import EditIcon from '@mui/icons-material/Edit'
import SaveIcon from '@mui/icons-material/Save'
import CloseIcon from '@mui/icons-material/Close'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import { GitHubBranchesField } from '../components/helix-org/GitHubStreamConfigFields'
import GitHubRepoPicker from '../components/helix-org/GitHubRepoPicker'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import { GITHUB_REPO_PATTERN } from '../components/helix-org/githubStreamConstants'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  EventCard,
  InstallWebhookFailedError,
  StreamDTO,
  useHelixOrgStream,
  useGitHubWebhookStatus,
  useInstallGitHubWebhook,
  useStreamMessageCount,
  useUpdateHelixOrgStream,
} from '../services/helixOrgService'

const HelixOrgStreamDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const orgSlug = router.params.org_id as string | undefined
  const streamId = router.params.stream_id as string | undefined
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Streams', routeName: 'helix_org_streams' })

  const { data: stream, isLoading } = useHelixOrgStream(streamId)
  const { data: messageCount } = useStreamMessageCount(streamId)
  const updateStream = useUpdateHelixOrgStream()

  // Live event list. Seeded from the initial GET so the page renders
  // immediately; replaced wholesale on every SSE push from
  // /streams/{id}/events. Falling back to the initial snapshot keeps
  // the list non-empty across reconnect blips.
  const [liveEvents, setLiveEvents] = useState<EventCard[] | null>(null)
  const events = liveEvents ?? stream?.recent_events ?? []

  // SSE wiring. For normal browser sessions EventSource sends the
  // helix_session cookie automatically. For embed-token flows
  // (pages loaded with ?access_token=…) the EventSource constructor
  // is patched in useApi.ts to append ?access_token= to same-origin
  // URLs — see the embedToken block there. The server emits
  // `event: message` frames with a JSON-array payload of up to 50
  // events newest-first; we replace state on each.
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

  const formatTimestamp = (iso: string) => {
    if (!iso) return ''
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString()
  }

  return (
    <Page
      breadcrumbTitle={stream?.name || streamId || 'Stream'}
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
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

              <StreamConfigSection
                stream={stream}
                onSave={async (payload) => {
                  try {
                    await updateStream.mutateAsync({ streamId: stream.id, payload })
                    snackbar.success('stream updated')
                    return true
                  } catch (e: any) {
                    const msg = e?.response?.data?.error || e?.message || 'update failed'
                    snackbar.error(msg)
                    return false
                  }
                }}
                saving={updateStream.isPending}
              />

              {stream.kind === 'github' && (
                <GitHubWebhookStatus
                  stream={stream}
                  orgSlug={orgSlug}
                />
              )}

              <Divider />

              <Box>
                <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2} sx={{ mb: 1 }}>
                  <Stack direction="row" alignItems="center" spacing={1.5}>
                    <Typography variant="h6">Messages</Typography>
                    <MessageCountCard count={messageCount} />
                  </Stack>
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

// StreamConfigSection renders the stream's mutable configuration —
// name, description, transport config — in a read-only Paper that
// flips into edit mode when the operator clicks Edit. The transport
// kind itself is NOT editable here (changing transport mid-flight
// would orphan webhook deliveries, GitHub installations etc); spin
// up a new stream and delete the old one for that.
interface StreamConfigSectionProps {
  stream: StreamDTO
  onSave: (payload: {
    name: string
    description?: string
    transport?: { config?: Record<string, unknown> }
  }) => Promise<boolean>
  saving: boolean
}

const StreamConfigSection: FC<StreamConfigSectionProps> = ({ stream, onSave, saving }) => {
  const snackbar = useSnackbar()
  const [editing, setEditing] = useState(false)

  // Local edit-mode state. Seeded from the stream every time the
  // user enters edit mode so cancel → re-edit starts from the
  // current server state, not whatever the user last typed.
  const [name, setName] = useState(stream.name)
  const [description, setDescription] = useState(stream.description ?? '')
  const [configText, setConfigText] = useState('')
  const [ghRepo, setGhRepo] = useState('')
  const [ghBranches, setGhBranches] = useState<string[]>([])
  // The full github config as it was on the server when edit opened.
  // We overlay only the operator-editable fields (repo, branches) on
  // top of this at save time so server-managed fields — events (now
  // owned by GitHub's webhook UI), webhook_id, webhook_html_url —
  // survive the round-trip instead of being wiped.
  const [ghOriginalConfig, setGhOriginalConfig] = useState<Record<string, unknown>>({})

  const enterEdit = () => {
    setName(stream.name)
    setDescription(stream.description ?? '')
    if (stream.kind === 'github') {
      const cfg = (stream.config ?? {}) as Record<string, unknown>
      setGhOriginalConfig(cfg)
      setGhRepo(typeof cfg.repo === 'string' ? cfg.repo : '')
      setGhBranches(Array.isArray(cfg.branches) && cfg.branches.length > 0 ? (cfg.branches as string[]) : ['*'])
    } else if (stream.config) {
      setConfigText(JSON.stringify(stream.config, null, 2))
    } else {
      setConfigText('')
    }
    setEditing(true)
  }

  const cancelEdit = () => setEditing(false)

  const handleSave = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    const payload: {
      name: string
      description?: string
      transport?: { config?: Record<string, unknown> }
    } = { name: name.trim(), description: description.trim() || undefined }

    if (stream.kind === 'github') {
      if (!ghRepo.trim() || !GITHUB_REPO_PATTERN.test(ghRepo.trim())) {
        snackbar.error('GitHub repo is required and must be owner/name')
        return
      }
      // Overlay the editable fields onto the original config so
      // events (managed on GitHub now), webhook_id and webhook_html_url
      // are preserved. The server's GitHubConfig.Validate requires a
      // non-empty events list — dropping it here is what used to make
      // the save silently fail.
      const ghConfig: Record<string, unknown> = { ...ghOriginalConfig, repo: ghRepo.trim() }
      const branches = ghBranches.map((b) => b.trim()).filter((b) => b.length > 0)
      if (branches.length > 0) {
        ghConfig.branches = branches
      } else {
        delete ghConfig.branches
      }
      payload.transport = { config: ghConfig }
    } else if (stream.kind !== 'local' && configText.trim()) {
      try {
        const parsed = JSON.parse(configText)
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          snackbar.error('Transport config must be a JSON object')
          return
        }
        payload.transport = { config: parsed }
      } catch (e) {
        snackbar.error('Transport config must be valid JSON')
        return
      }
    } else if (stream.kind !== 'local') {
      // Allow clearing back to no config.
      payload.transport = { config: {} }
    }
    const ok = await onSave(payload)
    if (ok) setEditing(false)
  }

  const configPreview = useMemo(() => {
    if (stream.kind === 'local') return null
    if (!stream.config || Object.keys(stream.config).length === 0) return '(empty)'
    return JSON.stringify(stream.config, null, 2)
  }, [stream.kind, stream.config])

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack direction="row" alignItems="baseline" justifyContent="space-between" sx={{ mb: 1 }}>
        <Typography variant="h6">Configuration</Typography>
        {!editing && (
          <Button size="small" startIcon={<EditIcon />} onClick={enterEdit}>
            Edit
          </Button>
        )}
      </Stack>

      {!editing ? (
        <Stack spacing={1.5}>
          <ReadOnlyRow label="Name" value={stream.name} />
          <ReadOnlyRow label="Description" value={stream.description || '—'} />
          <ReadOnlyRow label="Transport" value={stream.kind} mono />
          {stream.kind !== 'local' && (
            <Box>
              <Typography variant="caption" color="text.secondary">Config</Typography>
              <Typography
                component="pre"
                variant="body2"
                sx={{
                  mt: 0.5, mb: 0, p: 1, borderRadius: 1,
                  backgroundColor: 'action.hover',
                  fontFamily: 'monospace', fontSize: '0.75rem',
                  whiteSpace: 'pre-wrap', wordBreak: 'break-all',
                }}
              >
                {configPreview}
              </Typography>
            </Box>
          )}
        </Stack>
      ) : (
        <Stack spacing={2}>
          <TextField
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            size="small"
            fullWidth
            required
          />
          <TextField
            label="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            minRows={2}
            size="small"
            fullWidth
          />
          {stream.kind === 'github' && (
            <>
              <GitHubRepoPicker value={ghRepo} onChange={setGhRepo} />
              <GitHubBranchesField branches={ghBranches} onChange={setGhBranches} />
              <Typography variant="caption" color="text.secondary">
                Which GitHub event types this webhook delivers is configured on GitHub,
                not here — open the webhook on GitHub (below) to change it.
              </Typography>
            </>
          )}
          {stream.kind !== 'local' && stream.kind !== 'github' && (
            <TextField
              label="Transport config (JSON)"
              value={configText}
              onChange={(e) => setConfigText(e.target.value)}
              multiline
              minRows={4}
              size="small"
              fullWidth
              helperText='e.g. {"outbound_url": "https://example.com/hook"} for webhook, {"inbound_address": "ingest@…"} for postmark. Leave empty to clear config.'
              sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.8rem' } }}
            />
          )}
          {stream.kind === 'local' && (
            <Typography variant="caption" color="text.secondary">
              local transport has no config — nothing to edit beyond name/description.
            </Typography>
          )}
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button onClick={cancelEdit} startIcon={<CloseIcon />} disabled={saving}>
              Cancel
            </Button>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              onClick={handleSave}
              disabled={saving}
            >
              {saving ? 'Saving…' : 'Save'}
            </Button>
          </Stack>
        </Stack>
      )}
    </Paper>
  )
}

// GitHubWebhookStatus is the simplified "Connect to GitHub" panel.
// Helix auto-installs the webhook on GitHub when the stream is
// created, so most operators never need to copy a URL or paste a
// secret. This component just surfaces the current state and gives
// the operator a deep-link to the GitHub UI for tweaks.
//
// States:
//   - webhook_id set on the stream config → "Helix installed
//     webhook #N on owner/name." + Edit-on-GitHub link
//   - webhook_id unset → "Webhook not installed yet" + button to
//     re-run the install
//   - localhost SERVER_URL → red warning ("change SERVER_URL or
//     GitHub can't deliver")
interface GitHubWebhookStatusProps {
  stream: StreamDTO
  orgSlug?: string
}

// SettingsLink is the actionable button on the loopback warning —
// jumps the operator to the helix-org Settings page where
// `streams.public_url` can be set. Stops the user staring at
// "what URL is wrong, where do I change it?" The Settings page
// has every per-org config including the new public_url override.
const SettingsLink: FC<{ orgSlug?: string }> = ({ orgSlug }) => {
  const router = useRouter()
  if (!orgSlug) return null
  return (
    <Button
      size="small"
      variant="outlined"
      onClick={() => router.navigate('helix_org_settings', { org_id: orgSlug })}
      sx={{ color: 'warning.contrastText', borderColor: 'warning.contrastText' }}
    >
      Open helix-org Settings →
    </Button>
  )
}

const GitHubWebhookStatus: FC<GitHubWebhookStatusProps> = ({ stream, orgSlug }) => {
  const snackbar = useSnackbar()
  const install = useInstallGitHubWebhook()
  // Live truth from GitHub: does a webhook for this stream's payload URL
  // actually exist on the repo? This is the source of truth for the link vs
  // re-install decision — the stored config can be stale (hook deleted on
  // GitHub, or installed before we tracked the id).
  const status = useGitHubWebhookStatus(stream.id)

  // Check the EFFECTIVE public URL — what the install endpoint
  // would actually use (streams.public_url override applied on
  // top of SERVER_URL). When the operator sets that org config
  // to a publicly reachable URL the warning goes away even
  // though the SERVER_URL env still points at localhost.
  // Fallback to window.location.origin only when the server
  // sent nothing (e.g. older API).
  const effectivePublicURL = stream.effective_public_url && stream.effective_public_url.length > 0
    ? stream.effective_public_url
    : window.location.origin
  const isLocalhost = /(localhost|127\.0\.0\.1|0\.0\.0\.0)/i.test(effectivePublicURL)

  const cfg = (stream.config ?? {}) as { repo?: string; webhook_id?: number; webhook_html_url?: string }

  // Resolve the live status into a concrete view. "unknown" (couldn't reach
  // GitHub / no creds / no public URL) falls back to the stored config so the
  // panel still works degraded instead of always claiming "missing".
  const live = status.data
  let view: 'loading' | 'installed' | 'missing' = 'missing'
  let webhookHtmlUrl = ''
  let webhookId: number | undefined
  let active = true
  let unknownNote = ''
  if (status.isLoading) {
    view = 'loading'
  } else if (live?.state === 'installed') {
    view = 'installed'
    webhookHtmlUrl = live.webhook_html_url || cfg.webhook_html_url || ''
    webhookId = live.webhook_id
    active = live.active ?? true
  } else if (live?.state === 'missing') {
    view = 'missing'
  } else {
    // "unknown" or a query error — degrade to stored config.
    unknownNote = live?.detail || (status.error as any)?.message || ''
    if (cfg.webhook_id) {
      view = 'installed'
      webhookHtmlUrl = cfg.webhook_html_url || ''
      webhookId = cfg.webhook_id
    } else {
      view = 'missing'
    }
  }

  const reinstall = async () => {
    try {
      const out = await install.mutateAsync(stream.id)
      if (out.warning) {
        // Webhook is registered on GitHub; the warning tells the
        // operator what's still wrong on their side (typically
        // SERVER_URL pointing at localhost). Use an error toast
        // since "success" would be misleading.
        snackbar.error(`Webhook installed (id ${out.webhook_id}), but: ${out.warning}`)
      } else {
        snackbar.success(`Webhook installed on GitHub (id ${out.webhook_id})`)
      }
    } catch (e: any) {
      // useApi.post already showed the server's error snackbar
      // when the response was null. Skip our own redundant toast
      // in that case; surface anything else (network failures
      // etc).
      if (!(e instanceof InstallWebhookFailedError)) {
        const msg = e?.response?.data?.error ?? e?.message ?? 'install failed'
        snackbar.error(msg)
      }
    }
  }

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>Connect to GitHub</Typography>
      {isLocalhost && (
        <Box sx={{ mb: 1.5, p: 1.5, borderRadius: 1, backgroundColor: 'warning.main', color: 'warning.contrastText' }}>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            ⚠ Helix's effective public URL is <code>{effectivePublicURL}</code> — a loopback address.
          </Typography>
          <Typography variant="caption" sx={{ display: 'block', mt: 0.5, mb: 1 }}>
            GitHub's servers can't reach this URL, so webhook deliveries won't arrive. Fix by either:
            (a) setting <code>streams.public_url</code> on the helix-org Settings page to a publicly reachable host (cloudflared / ngrok / reverse proxy), or
            (b) editing <code>SERVER_URL</code> in helix's .env and restarting the api container.
          </Typography>
          <SettingsLink orgSlug={orgSlug} />
        </Box>
      )}
      {view === 'loading' ? (
        <Stack direction="row" spacing={1} alignItems="center">
          <CircularProgress size={16} />
          <Typography variant="body2" color="text.secondary">
            Checking GitHub for this stream's webhook…
          </Typography>
        </Stack>
      ) : view === 'installed' ? (
        <Stack spacing={1}>
          <Typography variant="body2">
            Webhook registered on <strong>{cfg.repo}</strong>{webhookId ? <> (id <code>{webhookId}</code>)</> : null}.
            {active ? ' Deliveries flow into this stream automatically.' : ' ⚠ It is currently disabled on GitHub, so no deliveries arrive — re-install to re-enable.'}
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            {webhookHtmlUrl && (
              <Button
                size="small"
                variant="outlined"
                component="a"
                href={webhookHtmlUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                View on GitHub →
              </Button>
            )}
            <Button
              size="small"
              variant="text"
              onClick={reinstall}
              disabled={install.isPending}
            >
              {install.isPending ? 'Re-installing…' : 'Re-install'}
            </Button>
          </Stack>
          {unknownNote && (
            <Typography variant="caption" color="text.secondary">
              Couldn't confirm against GitHub ({unknownNote}); showing last-known state.
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            Tweak the events whitelist (or any other webhook settings) directly on GitHub's UI. Helix routes deliveries by repo + stream id, so as long as the payload URL stays intact your changes take effect immediately.
          </Typography>
        </Stack>
      ) : (
        <Stack spacing={1.5}>
          <Typography variant="body2">
            No webhook found on GitHub for <strong>{cfg.repo || '(repo not set)'}</strong>. Helix can install it for you — one click, no copying URLs.
          </Typography>
          <Box>
            <Button
              variant="contained"
              onClick={reinstall}
              disabled={install.isPending || !cfg.repo}
            >
              {install.isPending ? 'Installing…' : 'Install webhook on GitHub'}
            </Button>
          </Box>
          {unknownNote && (
            <Typography variant="caption" color="text.secondary">
              Note: couldn't verify against GitHub ({unknownNote}).
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            Installed as the Helix GitHub App bot when it's installed on this repo (no human admin needed); otherwise falls back to a connected GitHub OAuth (on the helix Connected Services page) with admin rights on the repo.
          </Typography>
        </Stack>
      )}
    </Paper>
  )
}

// MessageCountCard is the compact metric chip beside the Messages
// header showing how many messages are waiting on the stream (meta.total
// from the paginated messages endpoint). Undefined while the count query
// is in flight — render an em-dash placeholder rather than 0 so a
// loading state doesn't read as "empty stream".
const MessageCountCard: FC<{ count: number | undefined }> = ({ count }) => (
  <Paper
    variant="outlined"
    sx={{
      px: 1.25,
      py: 0.5,
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      lineHeight: 1,
      minWidth: 56,
    }}
  >
    <Typography variant="h6" sx={{ fontFamily: 'monospace', fontWeight: 700, lineHeight: 1.1 }}>
      {count === undefined ? '—' : count.toLocaleString()}
    </Typography>
    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.6rem', textTransform: 'uppercase', letterSpacing: 0.4 }}>
      waiting
    </Typography>
  </Paper>
)

const ReadOnlyRow: FC<{ label: string; value: string; mono?: boolean }> = ({ label, value, mono }) => (
  <Box>
    <Typography variant="caption" color="text.secondary">{label}</Typography>
    <Typography
      variant="body2"
      sx={{ mt: 0.25, fontFamily: mono ? 'monospace' : undefined }}
    >
      {value}
    </Typography>
  </Box>
)

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
