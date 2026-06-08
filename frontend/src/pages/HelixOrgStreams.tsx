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
  useGitHubManifestStart,
  useInstallGitHubWebhook,
  useListGitHubRepos,
  useListHelixOrgStreams,
} from '../services/helixOrgService'

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
  // app_exists = the Helix app has been created for this org (via the manifest
  // flow or BYO creds) but may not be installed on a repo yet.
  const ghAppExists = !ghInstallQuery.isLoading && ghInstallQuery.data?.app_exists === true
  const ghInstallURL = ghInstallQuery.data?.install_url ?? ''
  const manifestStart = useGitHubManifestStart()
  // GitHub org the user wants to create the app under (org-owned app).
  const [ghCreateOrg, setGhCreateOrg] = useState('')

  // installGitHubApp sends the user to GitHub to install (or reconfigure)
  // the Helix App. This is the ONLY step that uses the user's own GitHub
  // identity. After they install, GitHub bounces them back; we re-check
  // install status both via a postMessage from the app's Setup-URL callback
  // (when wired) and on window focus (the always-works fallback).
  // openGitHubPopup centres a popup; returns null (and toasts) if blocked.
  const openGitHubPopup = (url: string, name: string): Window | null => {
    const width = 1000
    const height = 800
    const left = (window.innerWidth - width) / 2
    const top = (window.innerHeight - height) / 2
    const popup = window.open(
      url,
      name,
      `width=${width},height=${height},left=${left},top=${top},toolbar=0,location=0,menubar=0,directories=0,scrollbars=1`,
    )
    if (!popup) snackbar.error('Popup blocked — allow popups for this site and try again.')
    return popup
  }

  // watchForInstallCompletion re-checks install status when the GitHub flow
  // finishes — either via the app's setup-URL callback postMessage, or when
  // the user returns focus to this window (the always-works fallback).
  const watchForInstallCompletion = () => {
    const recheck = () => {
      ghInstallQuery.refetch()
      ghReposQuery.refetch()
    }
    const onMessage = (event: MessageEvent) => {
      if (event.data?.type === 'github-app-installed') {
        window.removeEventListener('message', onMessage)
        window.removeEventListener('focus', onFocus)
        snackbar.success('Helix installed — refreshing…')
        recheck()
      } else if (event.data?.type === 'github-app-install-error') {
        window.removeEventListener('message', onMessage)
        snackbar.error('GitHub reported a problem completing the install.')
      }
    }
    const onFocus = () => {
      window.removeEventListener('message', onMessage)
      recheck()
    }
    window.addEventListener('message', onMessage)
    window.addEventListener('focus', onFocus, { once: true })
  }

  // installGitHubApp: app already created → send the user to GitHub to pick
  // repos to install it on.
  const installGitHubApp = () => {
    if (!ghInstallURL) {
      snackbar.error('The Helix GitHub App is not configured on this deployment. Ask an admin to set GITHUB_APP_SLUG.')
      return
    }
    if (!openGitHubPopup(ghInstallURL, 'github-app-install')) return
    watchForInstallCompletion()
  }

  // createGitHubApp: no app yet → run the Manifest flow. Ask the backend for
  // the GitHub POST URL + manifest, then submit it as a form inside a popup so
  // GitHub creates the app (org-owned) on the user's behalf. The popup then
  // chains create → install → setup-callback, which postMessages us back.
  const createGitHubApp = async () => {
    const githubOrg = ghCreateOrg.trim()
    if (!githubOrg) {
      snackbar.error('Enter the GitHub organization to create the app under')
      return
    }
    // Open the popup synchronously on the click — opening it after the await
    // below would lose the user gesture and get blocked.
    const popup = openGitHubPopup('about:blank', 'github-app-create')
    if (!popup) return
    try {
      popup.document.body.innerHTML = 'Preparing the Helix app…'
      const start = await manifestStart.mutateAsync({ github_org: githubOrg, origin: window.location.origin })
      // Build and submit the manifest form inside the (same-origin) popup.
      const doc = popup.document
      doc.body.innerHTML = 'Redirecting to GitHub to create the Helix app…'
      const form = doc.createElement('form')
      form.method = 'POST'
      form.action = start.post_url
      const input = doc.createElement('input')
      input.type = 'hidden'
      input.name = 'manifest'
      input.value = start.manifest
      form.appendChild(input)
      doc.body.appendChild(form)
      form.submit()
      watchForInstallCompletion()
    } catch (e: any) {
      try { popup.close() } catch { /* ignore */ }
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Could not start GitHub app creation')
    }
  }

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
      config = { repo: ghRepo.trim(), events: ['*'] }
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
      setGhRepo('')
      onClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  const handleClose = () => {
    setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
    setGhRepo('')
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
          {!ghInstallQuery.isLoading && !ghInstalled && !ghAppExists && (
            <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }} data-testid="github-app-create-gate">
              <Typography variant="body2" sx={{ mb: 1 }}>
                <strong>Create the Helix GitHub App for your org.</strong> Helix creates an app owned by your GitHub organization (one click — GitHub pre-fills the permissions). Afterwards Helix acts as the <code>helix</code> bot, not your personal account.
              </Typography>
              <Stack direction="row" spacing={1} alignItems="center">
                <TextField
                  size="small"
                  label="GitHub organization"
                  placeholder="e.g. helixml"
                  value={ghCreateOrg}
                  onChange={(e) => setGhCreateOrg(e.target.value)}
                  sx={{ flex: 1 }}
                  inputProps={{ 'data-testid': 'github-app-create-org' }}
                />
                <Button
                  size="small"
                  variant="contained"
                  onClick={createGitHubApp}
                  disabled={!ghCreateOrg.trim() || manifestStart.isPending}
                  data-testid="github-app-create-button"
                >
                  {manifestStart.isPending ? 'Starting…' : 'Create Helix app'}
                </Button>
              </Stack>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                You must be an owner of that GitHub org. You'll review the app on GitHub, click Create, then choose which repos to install it on — all in one popup.
              </Typography>
            </Box>
          )}
          {!ghInstallQuery.isLoading && !ghInstalled && ghAppExists && (
            <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }} data-testid="github-app-install-gate">
              <Typography variant="body2" sx={{ mb: 1 }}>
                <strong>The Helix app is created but not installed on any repo yet.</strong> Install it on the repos you want Helix to work with — afterwards Helix lists those repos and acts on them as the <code>helix</code> bot.
              </Typography>
              <Button
                size="small"
                variant="contained"
                onClick={installGitHubApp}
                disabled={!ghInstallURL}
                data-testid="github-app-install-button"
              >
                Install Helix
              </Button>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                You'll be sent to GitHub to choose the repositories, then bounced back here. Only an org owner can install it.
              </Typography>
            </Box>
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
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                Default: receive every event from this repo ("*"). You can narrow the events whitelist from the stream's detail page after it's created.
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                Missing a repo? The bot only sees repos the Helix App is installed on.{' '}
                <Button
                  size="small"
                  variant="text"
                  onClick={installGitHubApp}
                  disabled={!ghInstallURL}
                  sx={{ p: 0, minWidth: 0, textTransform: 'none', verticalAlign: 'baseline' }}
                >
                  Configure repositories on GitHub →
                </Button>
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
