// HelixOrgStreams lists every event stream defined in the current
// org. Streams are the inbound side of the org's I/O: GitHub webhooks,
// Postmark inboxes, plain in-process buses. Workers subscribe via MCP;
// the chart edges (added in the same PR as this page) show which
// worker pulls from which stream.

import { FC, MouseEvent, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
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
  { value: 'github', label: 'github', help: 'GitHub webhook (inbound only). Stream config: {"repo":"owner/name","events":["issues","pull_request",...]}. Set transport.github.webhook_secret on the Settings page; the GitHub access token is reused from your existing GitHub OAuth connection automatically — no PAT needed.' },
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

  const tableData = useMemo(() => streams.map((s) => ({
    id: s.id,
    _data: s,
    _isHighlighted: highlightId === s.id,
    name: (
      <Typography variant="body1">
        <span
          style={{
            fontWeight: 'bold',
            color: highlightId === s.id
              ? theme.palette.warning.main
              : theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            fontFamily: 'monospace',
          }}
        >
          {s.id}
        </span>
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

  const helpFor = TRANSPORT_KINDS.find((k) => k.value === kind)?.help

  const submit = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    let config: Record<string, unknown> | undefined
    if (configText.trim()) {
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
      onClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  const handleClose = () => {
    setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
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
            <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }}>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                <strong>After Create</strong>, paste this into your GitHub repo's webhook settings (Settings → Webhooks → Add webhook):
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>
                Payload URL: <code>{`${window.location.origin}/api/v1/orgs/${(account.organizationTools.organization?.name) || '<org>'}/github/webhook`}</code>
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}>
                Content type: <code>application/json</code>
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}>
                Secret: same value as <code>transport.github.webhook_secret</code> on the Settings page
              </Typography>
            </Box>
          )}
          {kind !== 'local' && (
            <TextField
              label="Transport config (JSON)"
              placeholder='{"outbound_url": "https://example.com/hook"}'
              value={configText}
              onChange={(e) => setConfigText(e.target.value)}
              multiline
              minRows={4}
              fullWidth
              helperText="Kind-specific config. Optional for webhook (inbound-only without outbound_url); required for github + postmark."
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
