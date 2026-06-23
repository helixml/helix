// HelixOrgTopics lists every event topic defined in the current
// org. Topics are the inbound side of the org's I/O: GitHub webhooks,
// Postmark inboxes, plain in-process buses. Workers subscribe via MCP;
// the chart edges (added in the same PR as this page) show which
// worker pulls from which topic.

import { FC, MouseEvent, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
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
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  InstallWebhookFailedError,
  TopicDTO,
  useCreateHelixOrgTopic,
  useDeleteHelixOrgTopic,
  useGitHubAppInstallation,
  useInstallGitHubWebhook,
  useListGitHubRepos,
  useListHelixOrgTopics,
  useListSlackWorkspaces,
} from '../services/helixOrgService'
import { GitHubEventsField, GitHubBranchesField } from '../components/helix-org/GitHubTopicConfigFields'
import GitHubRepoPicker from '../components/helix-org/GitHubRepoPicker'
import { GitHubAppConnect } from '../components/helix-org/GitHubAppPanel'

const TRANSPORT_KINDS = [
  { value: 'local', label: 'local', help: 'In-process pub/sub. Default; no config needed.' },
  { value: 'webhook', label: 'webhook', help: 'HTTP webhook. Inbound by default; outbound URL = bidirectional.' },
  { value: 'github', label: 'github', help: 'GitHub webhook (inbound only). Scope this topic to a single repo + a whitelist of event types. Webhook secret is set once at the org level on the Settings page; the GitHub access token is reused from your OAuth connection automatically.' },
  { value: 'postmark', label: 'postmark', help: 'Inbound email (Postmark). Config: inbound_address.' },
  { value: 'cron', label: 'cron', help: 'Scheduled trigger. The server fires an event on this topic at the configured cadence; every subscribed Worker is activated. Minimum interval: 90 seconds.' },
  { value: 'slack', label: 'slack', help: 'Slack channel. Pick one of your org\'s connected Slack workspaces (install on the Settings page) and a channel. Inbound messages publish onto this topic; Workers reply as their persona. Routing to a specific Worker is done with a filter (e.g. !qa-bot).' },
]

// CRON_PRESETS are one-click chips that inject a standard 5-field
// cron expression into the schedule field. We keep the UI on the
// literal cron grammar — no @-aliases — so users see exactly what's
// stored, and there's one syntax to learn rather than two.
const CRON_PRESETS: Array<{ label: string; value: string }> = [
  { label: 'Hourly', value: '0 * * * *' },
  { label: 'Daily 00:00', value: '0 0 * * *' },
  { label: 'Weekly Sun 00:00', value: '0 0 * * 0' },
  { label: 'Weekdays 00:00', value: '0 0 * * 1-5' },
  { label: 'Weekends 00:00', value: '0 0 * * 0,6' },
  { label: 'Mon 09:00', value: '0 9 * * 1' },
  { label: 'Fri 18:00', value: '0 18 * * 5' },
]


// topicRowId is the canonical HTML id assigned to each row in the
// topics table. Exported so other components (the chart deep-link,
// for example) can pin the contract — change the format here and all
// callers update at once.
export const topicRowId = (topicId: string) => `topic-row-${topicId}`

// HIGHLIGHT_DURATION_MS is how long the focused-row highlight stays
// up after the chart deep-links into the topics page. Kept short so
// the page doesn't feel busy; long enough for the user to register
// which row they landed on.
const HIGHLIGHT_DURATION_MS = 2400

const HelixOrgTopics: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const breadcrumbs = useHelixOrgBreadcrumbs()
  const snackbar = useSnackbar()
  const theme = useTheme()

  const { data, isLoading } = useListHelixOrgTopics()
  const deleteTopic = useDeleteHelixOrgTopic()

  const topics = data?.topics ?? []
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<TopicDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentTopic, setCurrentTopic] = useState<TopicDTO | null>(null)

  const focusId = (router.params.focus as string | undefined) ?? undefined
  const [highlightId, setHighlightId] = useState<string | undefined>(undefined)

  // When the page lands with `?focus=<topicId>` (the chart's
  // topic-node click sets this) scroll the matching row into view
  // and pulse a highlight on it. Runs after every render where the
  // focus param or the topics list changes, so the deep-link works
  // even if the API call is still in flight on initial mount.
  useEffect(() => {
    if (!focusId) {
      setHighlightId(undefined)
      return
    }
    if (!topics.some((s) => s.id === focusId)) return
    const el = document.getElementById(topicRowId(focusId))
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    setHighlightId(focusId)
    const t = setTimeout(() => setHighlightId(undefined), HIGHLIGHT_DURATION_MS)
    return () => clearTimeout(t)
  }, [focusId, topics])

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, s: TopicDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentTopic(s)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentTopic(null)
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await deleteTopic.mutateAsync(deleting.id)
      snackbar.success(`topic ${deleting.id} deleted`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'delete failed')
    } finally {
      setDeleting(undefined)
    }
  }

  const orgSlug = (router.params.org_id as string | undefined) ?? ''
  const openTopicDetail = (sid: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_topic_detail', { org_id: orgSlug, topic_id: sid })
  }

  const tableData = useMemo(() => topics.map((s) => ({
    id: s.id,
    _data: s,
    _isHighlighted: highlightId === s.id,
    name: (
      <Typography variant="body1">
        <a
          href="#"
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openTopicDetail(s.id) }}
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
  })), [topics, theme, highlightId])

  const getActions = (row: any) => {
    const s = row._data as TopicDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, s)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <Page
      breadcrumbTitle="Topics"
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <Button
          variant="contained"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={() => setCreateOpen(true)}
        >
          New Topic
        </Button>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Topics</Typography>
            <Typography variant="body2" color="text.secondary">
              Named event channels Workers can subscribe to. Each topic carries a Transport (local
              pub/sub, GitHub webhooks, Postmark inbound email, plain webhooks). Workers subscribe via
              the <code>subscribe</code> MCP tool; the chart shows the resulting (worker → topic)
              edges as dashed lines.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : topics.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No topics yet.
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={() => setCreateOpen(true)}
                sx={{ mt: 1 }}
              >
                Create your first topic
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
              getRowId={(row) => topicRowId(row.id as string)}
            />
          )}
        </Stack>
      </Container>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentTopic) setDeleting(currentTopic)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>

      {deleting && (
        <DeleteConfirmWindow
          title="topic"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setDeleting(undefined)}
        >
          <Typography variant="body1">
            Deleting topic <b style={{ fontFamily: 'monospace' }}>{deleting.id}</b> removes the row.
            Subscriptions + events stay until drained explicitly. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}

      <NewTopicDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </Page>
  )
}

const NewTopicDialog: FC<{ open: boolean; onClose: () => void }> = ({ open, onClose }) => {
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgTopic()
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
  // Cron schedule — free text accepting either a 5-field cron expression
  // or one of the @aliases the backend recognises (@hourly, @daily, …).
  // CRON_PRESETS populate it via one-click chips.
  const [cronSchedule, setCronSchedule] = useState<string>('0 0 * * *')
  // Slack: pick one of the org's connected workspaces + a channel id.
  const [slackConnId, setSlackConnId] = useState<string>('')
  const [slackChannel, setSlackChannel] = useState<string>('')
  const slackWorkspacesQuery = useListSlackWorkspaces({ enabled: open })
  const slackWorkspaces = slackWorkspacesQuery.data ?? []

  // Probe GitHub on dialog open — the result tells us whether to
  // disable the `github` transport option (no OAuth connection →
  // operator gets a "Connect GitHub for topics" CTA instead of a
  // confusing 412 mid-flow). The hook suppresses its own snackbar
  // so this stays a quiet probe.
  // Kept for the refetch side-effects (App install completing,
  // GitHubAppConnect's onChange). GitHubRepoPicker fetches the list
  // itself via the same React Query key, so both stay in sync.
  const ghReposQuery = useListGitHubRepos({ enabled: open })

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
      // whitelist after the topic is created.
      const events = ghEvents.length > 0 ? ghEvents : ['*']
      config = { repo: ghRepo.trim(), events }
      const branches = ghBranches.map((b) => b.trim()).filter((b) => b.length > 0)
      if (branches.length > 0) (config as Record<string, unknown>).branches = branches
    } else if (kind === 'cron') {
      const sched = cronSchedule.trim()
      if (!sched) {
        snackbar.error('Schedule is required')
        return
      }
      config = { schedule: sched }
    } else if (kind === 'slack') {
      if (!slackConnId) {
        snackbar.error('Pick a connected Slack workspace')
        return
      }
      if (!slackChannel.trim()) {
        snackbar.error('Slack channel id is required')
        return
      }
      config = { service_connection_id: slackConnId, channel: slackChannel.trim() }
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
      if (kind === 'github' && created?.id) {
        // Auto-install a per-repo webhook on GitHub. The backend uses the
        // installed Helix App's installation token (repository_hooks
        // permission) when the app is present, falling back to a member's
        // OAuth token otherwise. Idempotent — re-run adopts an existing hook.
        try {
          const inst = await installWebhook.mutateAsync(created.id)
          if (inst.warning) {
            // Webhook IS installed; the warning tells the user
            // what's left on their side. Show as a (warning) toast
            // — the success snackbar would mislead.
            snackbar.error(`Webhook installed on GitHub (id ${inst.webhook_id}), but: ${inst.warning}`)
          } else {
            snackbar.success(`Topic created · webhook installed on GitHub (id ${inst.webhook_id})`)
          }
        } catch (e: any) {
          // The topic is created but webhook install failed. The
          // useApi-layer already showed the server's error
          // snackbar (e.g. "SERVER_URL ... is a loopback
          // address"); skip our own duplicate when that's the
          // case. Otherwise (network failure, runtime error) fall
          // back to a contextual message.
          if (!(e instanceof InstallWebhookFailedError)) {
            const msg = e?.response?.data?.error ?? e?.message ?? 'install failed'
            snackbar.error(`Topic created but webhook install failed: ${msg}. Open the topic detail page and click "Re-install webhook".`)
          }
        }
      } else {
        snackbar.success('topic created')
      }
      setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
      setGhRepo(''); setGhEvents(['*']); setGhBranches(['*']); setCronSchedule('0 0 * * *'); setSlackConnId(''); setSlackChannel('')
      onClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  const handleClose = () => {
    setId(''); setName(''); setDescription(''); setKind('local'); setConfigText('')
    setGhRepo(''); setGhEvents(['*']); setGhBranches(['*']); setCronSchedule('0 0 * * *'); setSlackConnId(''); setSlackChannel('')
    onClose()
  }

  return (
    <Dialog open={open} onClose={handleClose} fullWidth maxWidth="sm">
      <DialogTitle>New topic</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Topic ID (optional)"
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
              <GitHubRepoPicker value={ghRepo} onChange={setGhRepo} enabled={open} />
              <GitHubEventsField events={ghEvents} onChange={setGhEvents} />
              <GitHubBranchesField branches={ghBranches} onChange={setGhBranches} />
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                The bot only sees repos the Helix App is installed on — use "Add repositories / another org" above to grant more.
              </Typography>
            </>
          )}
          {kind === 'cron' && (
            <Box>
              <TextField
                label="Schedule"
                placeholder="0 9 * * 1"
                value={cronSchedule}
                onChange={(e) => setCronSchedule(e.target.value)}
                fullWidth
                helperText="Standard 5-field cron: minute hour day-of-month month day-of-week. Prefix with CRON_TZ=<zone> to pin the timezone (defaults to UTC). Minimum interval: 90 seconds."
              />
              <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
                {CRON_PRESETS.map((p) => (
                  <Chip
                    key={p.value}
                    label={p.label}
                    size="small"
                    variant={cronSchedule === p.value ? 'filled' : 'outlined'}
                    onClick={() => setCronSchedule(p.value)}
                    sx={{ fontFamily: 'monospace' }}
                  />
                ))}
              </Stack>
              <Box
                sx={{
                  mt: 1.5,
                  px: 1.5,
                  py: 1,
                  borderRadius: 1,
                  border: '1px solid rgba(0,0,0,0.08)',
                  bgcolor: 'rgba(0,0,0,0.02)',
                }}
              >
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                  Examples
                </Typography>
                {[
                  { value: '0 0 * * *', description: 'every day at midnight' },
                  { value: '0 9 * * 1', description: 'every Monday at 09:00' },
                  { value: '0 18 * * 5', description: 'every Friday at 18:00' },
                  { value: '0 0 * * 1-5', description: 'weekdays at midnight' },
                  { value: '*/15 * * * *', description: 'every 15 minutes' },
                  { value: '0 0 1 * *', description: 'first day of every month at 00:00' },
                  { value: 'CRON_TZ=America/New_York 30 14 * * 1-5', description: 'weekdays at 14:30 New York time' },
                ].map((ex) => (
                  <Typography
                    key={ex.value}
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block', lineHeight: 1.6 }}
                  >
                    <code style={{ fontFamily: 'monospace', fontWeight: 600 }}>{ex.value}</code>
                    {' — '}
                    {ex.description}
                  </Typography>
                ))}
              </Box>
            </Box>
          )}
          {kind === 'slack' && (
            <>
              <FormControl fullWidth size="small">
                <InputLabel id="slack-ws-label">Slack workspace</InputLabel>
                <Select
                  labelId="slack-ws-label"
                  value={slackConnId}
                  label="Slack workspace"
                  onChange={(e) => setSlackConnId(e.target.value)}
                >
                  {slackWorkspaces.length === 0 && (
                    <MenuItem value="" disabled>
                      No workspaces connected — install one on the Settings page
                    </MenuItem>
                  )}
                  {slackWorkspaces.map((ws: any) => (
                    <MenuItem key={ws.id} value={ws.id}>
                      {ws.slack_team_name || ws.name || ws.slack_team_id || ws.id}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              <TextField
                label="Channel ID"
                placeholder="C0123ABCD"
                value={slackChannel}
                onChange={(e) => setSlackChannel(e.target.value)}
                fullWidth
                size="small"
                helperText="The Slack channel id (not the #name) this topic binds to — find it under a channel's View details. Invite the Helix bot to that channel (/invite) so it receives messages."
              />
            </>
          )}
          {kind !== 'local' && kind !== 'github' && kind !== 'cron' && kind !== 'slack' && (
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

export default HelixOrgTopics
