// NewTopicDrawer is the shared "create topic" side drawer used by the
// Chart right-click menu / toolbar and the Topics list "+ New topic" action.

import { FC, useEffect, useRef, useState } from 'react'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import useSnackbar from '../../hooks/useSnackbar'
import {
  InstallWebhookFailedError,
  useCreateHelixOrgTopic,
  useGitHubAppInstallation,
  useInstallGitHubWebhook,
  useListGitHubRepos,
} from '../../services/helixOrgService'
import { getUserTimezone } from '../../utils/cronUtils'
import CronScheduleFields, { buildSpecificTimeCron } from './CronScheduleFields'
import { GitHubAppConnect } from './GitHubAppPanel'
import { GitHubBranchesField, GitHubEventsField } from './GitHubTopicConfigFields'
import GitHubRepoPicker from './GitHubRepoPicker'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'

const TRANSPORT_KINDS = [
  { value: 'local', label: 'local', help: 'In-process pub/sub. Default; no config needed.' },
  { value: 'webhook', label: 'webhook', help: 'HTTP webhook. Inbound by default; outbound URL = bidirectional.' },
  { value: 'github', label: 'github', help: 'GitHub webhook (inbound only). Scope this topic to a single repo + a whitelist of event types. Webhook secret is set once at the org level on the Settings page; the GitHub access token is reused from your OAuth connection automatically.' },
  { value: 'postmark', label: 'postmark', help: 'Inbound email (Postmark). Config: inbound_address.' },
  { value: 'cron', label: 'cron', help: 'Scheduled trigger. The server fires an event on this topic at the configured cadence; every subscribed Worker is activated. Minimum interval: 90 seconds.' },
]

// Default create-time schedule: weekdays at 09:00 in the operator's timezone.
const defaultCronSchedule = () =>
  buildSpecificTimeCron([1, 2, 3, 4, 5], 9, 0, getUserTimezone())

export type NewTopicDrawerProps = {
  open: boolean
  onClose: () => void
}

const NewTopicDrawer: FC<NewTopicDrawerProps> = ({ open, onClose }) => {
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
  // Cron schedule — human-friendly builder (weekdays + time / interval /
  // advanced cron). Stored as a standard 5-field expression (+ optional
  // CRON_TZ= prefix) on the topic config.
  const [cronSchedule, setCronSchedule] = useState<string>(defaultCronSchedule)
  const [cronMessage, setCronMessage] = useState<string>('')

  // Probe GitHub on open — the result tells us whether to disable the
  // `github` transport option (no app install → operator gets a
  // "Connect GitHub for topics" CTA instead of a confusing 412 mid-flow).
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ghInstalled])

  useEffect(() => {
    if (!open) return
    setId('')
    setName('')
    setDescription('')
    setKind('local')
    setConfigText('')
    setGhRepo('')
    setGhEvents(['*'])
    setGhBranches(['*'])
    setCronSchedule(defaultCronSchedule())
    setCronMessage('')
  }, [open])

  const helpFor = TRANSPORT_KINDS.find((k) => k.value === kind)?.help

  // Create is gated for github until the Helix App is installed and a repo is picked.
  const canSubmit =
    Boolean(name.trim()) &&
    !create.isPending &&
    (kind !== 'github' || (ghInstalled && Boolean(ghRepo.trim()))) &&
    (kind !== 'cron' || Boolean(cronSchedule.trim()))

  const resetAndClose = () => {
    setId('')
    setName('')
    setDescription('')
    setKind('local')
    setConfigText('')
    setGhRepo('')
    setGhEvents(['*'])
    setGhBranches(['*'])
    setCronSchedule(defaultCronSchedule())
    setCronMessage('')
    onClose()
  }

  const submit = async () => {
    if (!name.trim()) {
      snackbar.error('Name is required')
      return
    }
    let config: Record<string, unknown> | undefined
    if (kind === 'github') {
      if (!ghInstalled) {
        snackbar.error('Install the Helix GitHub App before creating a github topic')
        return
      }
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
      if (cronMessage.trim()) {
        config.message = cronMessage.trim()
      }
    } else if (configText.trim()) {
      try {
        config = JSON.parse(configText)
      } catch {
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
      resetAndClose()
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'create failed')
    }
  }

  return (
    <HelixOrgSideDrawer open={open} onClose={resetAndClose} title="New topic" width={480}>
      <Stack spacing={2}>
        <Typography variant="body2" color="text.secondary">
          Topics are named message buses. Bots subscribe on the chart; transports
          (local, webhook, GitHub, …) bring events in.
        </Typography>
        <TextField
          label="Topic ID (optional)"
          placeholder="s-github-pulls"
          value={id}
          onChange={(e) => setId(e.target.value)}
          helperText="Convention: s-<kebab-case>. Omit to auto-generate s-<uuid>."
          fullWidth
          size="small"
        />
        <TextField
          label="Name"
          placeholder="GitHub PR firehose"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          fullWidth
          size="small"
        />
        <TextField
          label="Description (optional)"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          multiline
          minRows={2}
          fullWidth
          size="small"
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
        {/* GitHub: always selectable; install gate + Create disabled until App is ready. */}
        {kind === 'github' && (
          <>
            {!ghInstalled && (
              <Typography variant="body2" color="text.secondary">
                Install the Helix GitHub App for this org to use the github transport.
                Create stays disabled until the app is installed and a repository is selected.
              </Typography>
            )}
            <GitHubAppConnect mode="gate" onChange={() => { ghInstallQuery.refetch(); ghReposQuery.refetch() }} />
            {ghInstalled && (
              <>
                <GitHubRepoPicker value={ghRepo} onChange={setGhRepo} enabled={open} />
                <GitHubEventsField events={ghEvents} onChange={setGhEvents} />
                <GitHubBranchesField branches={ghBranches} onChange={setGhBranches} />
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                  The bot only sees repos the Helix App is installed on — use &quot;Add repositories / another org&quot; above to grant more.
                </Typography>
              </>
            )}
          </>
        )}
        {kind === 'cron' && (
          <CronScheduleFields
            // Remount when the drawer opens so weekday/time state matches the seed schedule.
            key={open ? 'cron-open' : 'cron-closed'}
            value={cronSchedule}
            onChange={setCronSchedule}
            message={cronMessage}
            onMessageChange={setCronMessage}
            defaultMode="specific_time"
          />
        )}
        {kind !== 'local' && kind !== 'github' && kind !== 'cron' && (
          <TextField
            label="Transport config (JSON)"
            placeholder='{"outbound_url": "https://example.com/hook"}'
            value={configText}
            onChange={(e) => setConfigText(e.target.value)}
            multiline
            minRows={4}
            fullWidth
            size="small"
            helperText="Kind-specific config. Optional for webhook (inbound-only without outbound_url); required for postmark."
          />
        )}
        <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
          <Button onClick={submit} variant="contained" disabled={!canSubmit}>
            {create.isPending ? 'Creating…' : 'Create'}
          </Button>
          <Button onClick={resetAndClose} variant="text">Cancel</Button>
        </Stack>
      </Stack>
    </HelixOrgSideDrawer>
  )
}

export default NewTopicDrawer
