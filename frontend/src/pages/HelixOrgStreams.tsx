// HelixOrgStreams lists every event stream defined in the current
// org. Streams are the inbound side of the org's I/O: GitHub webhooks,
// Postmark inboxes, plain in-process buses. Workers subscribe via MCP;
// the chart edges (added in the same PR as this page) show which
// worker pulls from which stream.

import { FC, MouseEvent, useEffect, useMemo, useRef, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
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
import RefreshIcon from '@mui/icons-material/Refresh'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  InstallWebhookFailedError,
  StreamDTO,
  useCreateHelixOrgStream,
  useDeleteHelixOrgStream,
  useGitHubAppInstallation,
  useInstallGitHubWebhook,
  useListGitHubRepos,
  useListHelixOrgStreams,
} from '../services/helixOrgService'
import { GitHubEventsField, GitHubBranchesField } from '../components/helix-org/GitHubStreamConfigFields'
import { GitHubAppConnect } from '../components/helix-org/GitHubAppPanel'

const TRANSPORT_KINDS = [
  { value: 'local', label: 'local', help: 'In-process pub/sub. Default; no config needed.' },
  { value: 'webhook', label: 'webhook', help: 'HTTP webhook. Inbound by default; outbound URL = bidirectional.' },
  { value: 'github', label: 'github', help: 'GitHub webhook (inbound only). Scope this stream to a single repo + a whitelist of event types. Webhook secret is set once at the org level on the Settings page; the GitHub access token is reused from your OAuth connection automatically.' },
  { value: 'postmark', label: 'postmark', help: 'Inbound email (Postmark). Config: inbound_address.' },
]


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
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgStream()
  const installWebhook = useInstallGitHubWebhook()
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [kind, setKind] = useState('local')
  const [configText, setConfigText] = useState('')
  // github branch only needs the repo — everything else (events
  // whitelist, webhook secret, payload URL) is configured by helix
  // on the operator's behalf when we POST to GitHub's hook API.
  // Default events to ["*"] which GitHub honours as "send me
  // everything"; advanced users can narrow this down later via the
  // detail page's Edit flow.
  const [ghRepo, setGhRepo] = useState<string>('')
  // Events default to ["*"] (all). Branches optionally narrow push/create/
  // delete to specific branches. Both editable here; also on the detail page.
  const [ghEvents, setGhEvents] = useState<string[]>(['*'])
  const [ghBranches, setGhBranches] = useState<string[]>(['*'])

  // Probe GitHub on dialog open — the result tells us whether to
  // disable the `github` transport option (no OAuth connection →
  // operator gets a "Connect GitHub for streams" CTA instead of a
  // confusing 412 mid-flow). The hook suppresses its own snackbar
  // so this stays a quiet probe.
  const ghReposQuery = useListGitHubRepos({ enabled: open })
  const ghRepoOptions = useMemo(() => (ghReposQuery.data?.repos ?? []).map((r) => r.full_name), [ghReposQuery.data])

  // Probe whether the Helix GitHub App is installed for this org. This is
  // the single gate for the github transport: the user's own credentials
  // are used only to install the app; everything after (repo listing,
  // worker git/gh) acts as the bot. Replaces the old "Connect GitHub
  // OAuth" gate.
  const ghInstallQuery = useGitHubAppInstallation({ enabled: open, pollWhileNotInstalled: open })
  const ghInstalled = !ghInstallQuery.isLoading && ghInstallQuery.data?.installed === true
  // When the install transitions to done (detected via polling, since the
  // GitHub popup can't postMessage back through COOP), refetch the repo list
  // so the picker shows the bot's installation repos instead of stale state.
  const prevInstalledRef = useRef(false)
  useEffect(() => {
    if (ghInstalled && !prevInstalledRef.current) {
      ghReposQuery.refetch()
    }
    prevInstalledRef.current = ghInstalled
  }, [ghInstalled])

  // If the operator had `github` selected when the probe came back
  // negative (e.g. they disconnected OAuth between dialog opens),
  // drop back to `local` so the disabled MenuItem becomes the
  // current value's mismatch case.
  useEffect(() => {
    if (kind === 'github' && !ghInstallQuery.isLoading && !ghInstalled) {
      setKind('local')
    }
  }, [kind, ghInstallQuery.isLoading, ghInstalled])

  const helpFor = TRANSPORT_KINDS.find((k) => k.value === kind)?.help

  const submit = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    let config: Record<string, unknown> | undefined
    if (kind === 'github') {
      if (!ghRepo.trim()) {
        snackbar.error('Pick a GitHub repository')
        return
      }
      // "*" = wildcard — GitHub sends every event and helix's
      // transport accepts every event. The detail page's edit form
      // lets advanced operators narrow this to a specific
      // whitelist after the stream is created.
      const events = ghEvents.length > 0 ? ghEvents : ['*']
      config = { repo: ghRepo.trim(), events }
      const branches = ghBranches.map((b) => b.trim()).filter((b) => b.length > 0)
      if (branches.length > 0) (config as Record<string, unknown>).branches = branches
    } else if (configText.trim()) {
      try {
        config = JSON.parse(configText)
      } catch (e) {
        snackbar.error('Transport config must be valid JSON')
        return
      }
    }
    try {
      const created = await create.mutateAsync({
        id: id.trim() || undefined,
        name: name.trim(),
        description: description.trim() || undefined,
        transport: { kind, config },
      })
      if (kind === 'github' && created?.id && ghInstalled) {
        // App mode: the Helix GitHub App already delivers all events for
        // every installed repo to one webhook; this stream is just a
        // (repo, events) filter on that firehose. No per-repo webhook to
        // install — doing so would need admin rights and double-deliver.
        snackbar.success('Stream created · events arrive via the Helix GitHub App')
      } else if (kind === 'github' && created?.id) {
        // Legacy OAuth mode: auto-install a per-repo webhook on GitHub.
        // Idempotent — re-run adopts an existing hook.
        try {
          const inst = await installWebhook.mutateAsync(created.id)
          if (inst.warning) {
            // Webhook IS installed; the warning tells the user
            // what's left on their side. Show as a (warning) toast
            // — the success snackbar would mislead.
            snackbar.error(`Webhook installed on GitHub (id ${inst.webhook_id}), but: ${inst.warning}`)
          } else {
            snackbar.success(`Stream created · webhook installed on GitHub (id ${inst.webhook_id})`)
          }
        } catch (e: any) {
          // The stream is created but webhook install failed. The
          // useApi-layer already showed the server's error
          // snackbar (e.g. "SERVER_URL ... is a loopback
          // address"); skip our own duplicate when that's the
          // case. Otherwise (network failure, runtime error) fall
          // back to a contextual message.
          if (!(e instanceof InstallWebhookFailedError)) {
            const msg = e?.response?.data?.error ?? e?.message ?? 'install failed'
            snackbar.error(`Stream created but webhook install failed: ${msg}. Open the stream detail page and click "Re-install webhook".`)
          }
        }
      } else {
        snackbar.success('stream created')
      }
      setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
      setGhRepo(''); setGhEvents(['*']); setGhBranches(['*'])
      onClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  const handleClose = () => {
    setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
    setGhRepo(''); setGhEvents(['*']); setGhBranches(['*'])
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
              {TRANSPORT_KINDS.map((k) => {
                const disabled = k.value === 'github' && !ghInstallQuery.isLoading && !ghInstalled
                return (
                  <MenuItem
                    key={k.value}
                    value={k.value}
                    disabled={disabled}
                    sx={{ fontFamily: 'monospace' }}
                  >
                    {k.label}
                    {disabled && (
                      <Typography
                        component="span"
                        variant="caption"
                        color="text.secondary"
                        sx={{ ml: 1, fontFamily: 'inherit', fontStyle: 'italic' }}
                      >
                        — needs the Helix GitHub App (see below)
                      </Typography>
                    )}
                  </MenuItem>
                )
              })}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary">{helpFor}</Typography>
          {/* Shared GitHub App connector — same component as the Settings page.
              Shown while the app isn't ready (so the user can set it up from any
              transport), and alongside the repo picker once github is selected
              (where its "Add repositories" button is useful). */}
          {(!ghInstalled || kind === 'github') && (
            <GitHubAppConnect mode="gate" onChange={() => { ghInstallQuery.refetch(); ghReposQuery.refetch() }} />
          )}
          {kind === 'github' && (
            <>
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <Autocomplete
                  freeSolo
                  autoSelect
                  selectOnFocus
                  fullWidth
                  options={ghRepoOptions}
                  value={ghRepo || null}
                  onChange={(_, v) => setGhRepo((v ?? '').trim())}
                  onInputChange={(_, v, reason) => {
                    // Sync typed text into ghRepo so the submit
                    // handler sees what the operator typed even if
                    // they never picked a dropdown row.
                    if (reason === 'input') setGhRepo(v)
                  }}
                  loading={ghReposQuery.isLoading || ghReposQuery.isRefetching}
                  disablePortal
                  noOptionsText={
                    ghReposQuery.isError
                      ? 'Could not load repos — connect a GitHub account on Connected Services first.'
                      : (ghReposQuery.isLoading || ghReposQuery.isRefetching)
                        ? 'Loading…'
                        : 'No matches — type the full owner/name and press Enter.'
                  }
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      label="Repository"
                      placeholder="search or type owner/name…"
                      size="small"
                      InputProps={{
                        ...params.InputProps,
                        endAdornment: (
                          <>
                            {(ghReposQuery.isLoading || ghReposQuery.isRefetching) && <CircularProgress size={16} sx={{ mr: 1 }} />}
                            {params.InputProps.endAdornment}
                          </>
                        ),
                      }}
                      helperText={
                        ghRepoOptions.length >= 1000
                          ? `Showing the ${ghRepoOptions.length} most-recently-pushed repos. Type the full owner/name if the one you want isn't listed.`
                          : `Pick a repo, or type owner/name directly. Helix will register the webhook on GitHub for you — no manual setup. ${ghRepoOptions.length} repos loaded.`
                      }
                    />
                  )}
                />
                <IconButton
                  size="small"
                  onClick={() => ghReposQuery.refetch()}
                  disabled={ghReposQuery.isFetching}
                  title="Re-fetch repos from GitHub (use this after granting the helix OAuth app access to a new org)"
                  sx={{ mt: 0.5 }}
                >
                  <RefreshIcon fontSize="small" />
                </IconButton>
              </Stack>
              <GitHubEventsField events={ghEvents} onChange={setGhEvents} />
              <GitHubBranchesField branches={ghBranches} onChange={setGhBranches} />
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                The bot only sees repos the Helix App is installed on — use "Add repositories / another org" above to grant more.
              </Typography>
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
