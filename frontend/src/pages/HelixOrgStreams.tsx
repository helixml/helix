// HelixOrgStreams lists every event stream defined in the current
// org. Streams are the inbound side of the org's I/O: GitHub webhooks,
// Postmark inboxes, plain in-process buses. Workers subscribe via MCP;
// the chart edges (added in the same PR as this page) show which
// worker pulls from which stream.

import { FC, MouseEvent, useEffect, useMemo, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  StreamDTO,
  useCreateHelixOrgStream,
  useDeleteHelixOrgStream,
  useListHelixOrgStreams,
} from '../services/helixOrgService'

const TRANSPORT_KINDS = [
  { value: 'local', label: 'local', help: 'In-process pub/sub. Default; no config needed.' },
  { value: 'webhook', label: 'webhook', help: 'HTTP webhook. Inbound by default; outbound URL = bidirectional.' },
  { value: 'github', label: 'github', help: 'GitHub webhook (inbound only). Scope this stream to a single repo + a whitelist of event types. Webhook secret is set once at the org level on the Settings page; the GitHub access token is reused from your OAuth connection automatically.' },
  { value: 'postmark', label: 'postmark', help: 'Inbound email (Postmark). Config: inbound_address.' },
]

// GITHUB_EVENT_OPTIONS mirrors the backend's knownGitHubEvents map in
// api/pkg/org/domain/transport/github.go — the validator rejects any
// event not in this list. Keep the two in sync; adding a new event is
// a one-line edit on each side. Order matches GitHub's docs grouping
// (issue stuff first, then PR stuff).
const GITHUB_EVENT_OPTIONS: { value: string; help: string }[] = [
  { value: 'issues', help: 'Issue opened/closed/labeled/etc.' },
  { value: 'issue_comment', help: 'Comment on an issue or pull request.' },
  { value: 'pull_request', help: 'PR opened/synced/closed/etc.' },
  { value: 'pull_request_review', help: 'Submitted PR reviews (approve/changes-requested/comment).' },
  { value: 'pull_request_review_comment', help: 'Line-level review comments inside a PR diff.' },
]

// owner/name pattern — exactly one slash, both halves non-empty. Mirrors
// transport/github.go's Validate; we surface the error before submit
// rather than after a 400.
const GITHUB_REPO_PATTERN = /^[^/\s]+\/[^/\s]+$/

// streamRowId is the canonical HTML id assigned to each row in the
// streams table. Exported so other components (the chart deep-link,
// for example) can pin the contract — change the format here and all
// callers update at once.
export const streamRowId = (streamId: string) => `stream-row-${streamId}`

// HIGHLIGHT_DURATION_MS is how long the focused-row highlight stays
// up after the chart deep-links into the streams page. Kept short so
// the page doesn't feel busy; long enough for the user to register
// which row they landed on.
const HIGHLIGHT_DURATION_MS = 2400

const HelixOrgStreams: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  const theme = useTheme()

  const { data, isLoading } = useListHelixOrgStreams()
  const deleteStream = useDeleteHelixOrgStream()

  const streams = data?.streams ?? []
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<StreamDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentStream, setCurrentStream] = useState<StreamDTO | null>(null)

  const focusId = (router.params.focus as string | undefined) ?? undefined
  const [highlightId, setHighlightId] = useState<string | undefined>(undefined)

  // When the page lands with `?focus=<streamId>` (the chart's
  // stream-node click sets this) scroll the matching row into view
  // and pulse a highlight on it. Runs after every render where the
  // focus param or the streams list changes, so the deep-link works
  // even if the API call is still in flight on initial mount.
  useEffect(() => {
    if (!focusId) {
      setHighlightId(undefined)
      return
    }
    if (!streams.some((s) => s.id === focusId)) return
    const el = document.getElementById(streamRowId(focusId))
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    setHighlightId(focusId)
    const t = setTimeout(() => setHighlightId(undefined), HIGHLIGHT_DURATION_MS)
    return () => clearTimeout(t)
  }, [focusId, streams])

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, s: StreamDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentStream(s)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentStream(null)
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await deleteStream.mutateAsync(deleting.id)
      snackbar.success(`stream ${deleting.id} deleted`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'delete failed')
    } finally {
      setDeleting(undefined)
    }
  }

  const orgSlug = (router.params.org_id as string | undefined) ?? ''
  const openStreamDetail = (sid: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_stream_detail', { org_id: orgSlug, stream_id: sid })
  }

  const tableData = useMemo(() => streams.map((s) => ({
    id: s.id,
    _data: s,
    _isHighlighted: highlightId === s.id,
    name: (
      <Typography variant="body1">
        <a
          href="#"
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openStreamDetail(s.id) }}
          style={{
            fontWeight: 'bold',
            color: highlightId === s.id
              ? theme.palette.warning.main
              : theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            fontFamily: 'monospace',
            textDecoration: 'none',
            cursor: 'pointer',
          }}
        >
          {s.id}
        </a>
      </Typography>
    ),
    nameField: (
      <Typography variant="body2" color="text.secondary">{s.name}</Typography>
    ),
    kind: (
      <Typography variant="body2" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>{s.kind}</Typography>
    ),
    subscribers: (
      <Typography variant="body2" color="text.secondary">{s.subscribers?.length ?? 0}</Typography>
    ),
    created: (
      <Typography variant="body2" color="text.secondary">
        {s.created_at ? new Date(s.created_at).toLocaleString() : '—'}
      </Typography>
    ),
  })), [streams, theme, highlightId])

  const getActions = (row: any) => {
    const s = row._data as StreamDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, s)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <Page
      breadcrumbTitle="Streams"
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <Button
          variant="contained"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={() => setCreateOpen(true)}
        >
          New Stream
        </Button>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Streams</Typography>
            <Typography variant="body2" color="text.secondary">
              Named event channels Workers can subscribe to. Each stream carries a Transport (local
              pub/sub, GitHub webhooks, Postmark inbound email, plain webhooks). Workers subscribe via
              the <code>subscribe</code> MCP tool; the chart shows the resulting (worker → stream)
              edges as dashed lines.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : streams.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No streams yet.
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={() => setCreateOpen(true)}
                sx={{ mt: 1 }}
              >
                Create your first stream
              </Button>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'ID' },
                { name: 'nameField', title: 'Name' },
                { name: 'kind', title: 'Transport' },
                { name: 'subscribers', title: 'Subscribers' },
                { name: 'created', title: 'Created' },
              ]}
              data={tableData}
              getActions={getActions}
              getRowId={(row) => streamRowId(row.id as string)}
            />
          )}
        </Stack>
      </Container>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentStream) setDeleting(currentStream)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>

      {deleting && (
        <DeleteConfirmWindow
          title="stream"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setDeleting(undefined)}
        >
          <Typography variant="body1">
            Deleting stream <b style={{ fontFamily: 'monospace' }}>{deleting.id}</b> removes the row.
            Subscriptions + events stay until drained explicitly. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}

      <NewStreamDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </Page>
  )
}

const NewStreamDialog: FC<{ open: boolean; onClose: () => void }> = ({ open, onClose }) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgStream()
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [kind, setKind] = useState('local')
  const [configText, setConfigText] = useState('')
  const [copied, setCopied] = useState(false)
  // Structured fields for kind=github. The backend validator rejects
  // anything missing repo or with an empty/unknown events list, so the
  // dialog presents them as first-class inputs rather than expecting
  // the operator to hand-write the JSON.
  const [ghRepo, setGhRepo] = useState('')
  const [ghEvents, setGhEvents] = useState<string[]>([])

  // The operator-configured public origin is what we want to show in
  // the github-stream URL hint; window.location.origin is the
  // FALLBACK, but if the user opened helix at localhost the hint
  // would be useless to paste into a real GitHub repo. server_url
  // arrives via /api/v1/config (loaded into account.serverConfig).
  const serverURL = (account.serverConfig as any)?.server_url as string | undefined
  const dialogOrigin = (serverURL && serverURL.length > 0) ? serverURL : window.location.origin
  const orgName = account.organizationTools.organization?.name
  const webhookURL = `${dialogOrigin}/api/v1/orgs/${encodeURIComponent(orgName || '<org>')}/github/webhook`
  const isLocalhostHint = /(localhost|127\.0\.0\.1|0\.0\.0\.0)/i.test(dialogOrigin)

  const copyWebhookURL = async () => {
    try {
      await navigator.clipboard.writeText(webhookURL)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      // Clipboard API can fail under non-https / restricted contexts.
      // Surface the failure quietly; the user can still select+copy.
      snackbar.error('Could not copy — select the URL manually')
    }
  }

  const helpFor = TRANSPORT_KINDS.find((k) => k.value === kind)?.help

  const ghRepoValid = ghRepo.trim() !== '' && GITHUB_REPO_PATTERN.test(ghRepo.trim())
  const ghRepoError = ghRepo.trim() !== '' && !ghRepoValid
    ? 'Format must be exactly owner/name (e.g. helixml/helix).'
    : ''

  const submit = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    let config: Record<string, unknown> | undefined
    if (kind === 'github') {
      // Structured fields → wire config. Mirror the backend's
      // GitHubConfig shape exactly so the validator passes first try.
      if (!ghRepo.trim() || !ghRepoValid) {
        snackbar.error('GitHub repo is required and must be owner/name')
        return
      }
      if (ghEvents.length === 0) {
        snackbar.error('Pick at least one GitHub event type')
        return
      }
      config = { repo: ghRepo.trim(), events: ghEvents }
    } else if (configText.trim()) {
      try {
        config = JSON.parse(configText)
      } catch (e) {
        snackbar.error('Transport config must be valid JSON')
        return
      }
    }
    try {
      await create.mutateAsync({
        id: id.trim() || undefined,
        name: name.trim(),
        description: description.trim() || undefined,
        transport: { kind, config },
      })
      snackbar.success('stream created')
      setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
      setGhRepo(''); setGhEvents([])
      onClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  const handleClose = () => {
    setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
    setGhRepo(''); setGhEvents([])
    onClose()
  }

  return (
    <Dialog open={open} onClose={handleClose} fullWidth maxWidth="sm">
      <DialogTitle>New stream</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Stream ID (optional)"
            placeholder="s-github-pulls"
            value={id}
            onChange={(e) => setId(e.target.value)}
            helperText="Convention: s-<kebab-case>. Omit to auto-generate s-<uuid>."
            fullWidth
          />
          <TextField
            label="Name"
            placeholder="GitHub PR firehose"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            fullWidth
          />
          <TextField
            label="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            minRows={2}
            fullWidth
          />
          <FormControl fullWidth size="small">
            <InputLabel id="kind-label">Transport</InputLabel>
            <Select
              labelId="kind-label"
              value={kind}
              label="Transport"
              onChange={(e) => setKind(e.target.value)}
            >
              {TRANSPORT_KINDS.map((k) => (
                <MenuItem key={k.value} value={k.value} sx={{ fontFamily: 'monospace' }}>
                  {k.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary">{helpFor}</Typography>
          {kind === 'github' && (
            <>
              {/* Scoping fields — backed by the Go validator's
                  GitHubConfig{Repo, Events}. The validator rejects
                  missing/invalid values with a 400; we surface the
                  same rules inline so the user catches the mistake
                  before submit. */}
              <TextField
                label="Repository"
                placeholder="helixml/helix"
                value={ghRepo}
                onChange={(e) => setGhRepo(e.target.value)}
                error={!!ghRepoError}
                helperText={ghRepoError || 'Exactly one repo per stream — GitHub `owner/name` (e.g. helixml/helix). Matched case-insensitively against repository.full_name in the payload.'}
                fullWidth
                size="small"
              />
              <Autocomplete
                multiple
                disableCloseOnSelect
                options={GITHUB_EVENT_OPTIONS}
                value={GITHUB_EVENT_OPTIONS.filter((o) => ghEvents.includes(o.value))}
                onChange={(_, next) => setGhEvents(next.map((o) => o.value))}
                getOptionLabel={(o) => o.value}
                isOptionEqualToValue={(a, b) => a.value === b.value}
                renderOption={(props, option, { selected }) => (
                  <li {...props}>
                    <Checkbox checked={selected} sx={{ mr: 1 }} />
                    <Box>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option.value}</Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                        {option.help}
                      </Typography>
                    </Box>
                  </li>
                )}
                renderTags={(value, getTagProps) =>
                  value.map((option, index) => (
                    <Chip
                      {...getTagProps({ index })}
                      key={option.value}
                      label={option.value}
                      size="small"
                      sx={{ fontFamily: 'monospace' }}
                    />
                  ))
                }
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Events"
                    placeholder={ghEvents.length === 0 ? 'Pick the GitHub events this stream consumes…' : ''}
                    size="small"
                    helperText="Anything not listed here is dropped at the transport, so subscribed Workers don't activate for events they'd ignore."
                  />
                )}
              />
              <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }}>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                  <strong>After Create</strong>, paste this into your GitHub repo's webhook settings (Settings → Webhooks → Add webhook):
                </Typography>
                <Stack direction="row" alignItems="center" spacing={1} sx={{ mt: 0.5 }}>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all', flex: 1 }}>
                    <strong>Payload URL:</strong> <code>{webhookURL}</code>
                  </Typography>
                  <Button size="small" variant="outlined" onClick={copyWebhookURL} sx={{ minWidth: 56, fontSize: '0.65rem' }}>
                    {copied ? 'Copied' : 'Copy'}
                  </Button>
                </Stack>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', mt: 0.5 }}>
                  <strong>Content type:</strong> <code>application/json</code>
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}>
                  <strong>Secret:</strong> same value as <code>transport.github.webhook_secret</code> on the Settings page
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}>
                  <strong>Which events?</strong> Match this stream's selection above when filling GitHub's "Let me select individual events" — extra events GitHub sends but you didn't whitelist here are dropped at the transport.
                </Typography>
                {isLocalhostHint && (
                  <Typography variant="caption" sx={{ display: 'block', mt: 1, color: 'warning.main', fontFamily: 'monospace', fontSize: '0.65rem' }}>
                    ⚠ Your helix origin is a loopback address — GitHub's servers cannot reach it. Configure SERVER_URL to your public origin (e.g. behind a reverse proxy or cloudflared tunnel), or expose this listen-port publicly, before pasting into GitHub.
                  </Typography>
                )}
              </Box>
            </>
          )}
          {kind !== 'local' && kind !== 'github' && (
            <TextField
              label="Transport config (JSON)"
              placeholder='{"outbound_url": "https://example.com/hook"}'
              value={configText}
              onChange={(e) => setConfigText(e.target.value)}
              multiline
              minRows={4}
              fullWidth
              helperText="Kind-specific config. Optional for webhook (inbound-only without outbound_url); required for postmark."
            />
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button onClick={submit} variant="contained" disabled={create.isPending}>
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default HelixOrgStreams
