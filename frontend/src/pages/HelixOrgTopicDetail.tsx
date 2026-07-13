// HelixOrgTopicDetail is the per-topic "messages flowing through"
// view. It hydrates from GET /topics/{id} for the initial snapshot
// then keeps the event list live via the SSE endpoint at
// /topics/{id}/events — every push replaces the list wholesale so
// the frontend never has to diff partial updates. The shape mirrors
// what the old htmx /ui/topics?id=… surface used to render.
//
// The page also exposes inline editing of mutable fields (name,
// description, transport config) via PUT /topics/{id}. For the
// github transport the same Repository + Events picker as the New
// Topic dialog is shown; for other non-local transports a JSON
// textarea is offered.

import { FC, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import SaveIcon from '@mui/icons-material/Save'
import DeleteSweepIcon from '@mui/icons-material/DeleteSweep'
import { useQueryClient } from '@tanstack/react-query'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import CronScheduleFields from '../components/helix-org/CronScheduleFields'
import { GitHubBranchesField } from '../components/helix-org/GitHubTopicConfigFields'
import GitHubRepoPicker from '../components/helix-org/GitHubRepoPicker'
import { GITHUB_REPO_PATTERN } from '../components/helix-org/githubTopicConstants'
import CopyButtonWithCheck from '../components/session/CopyButtonWithCheck'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  EventCard,
  InstallWebhookFailedError,
  QUERY_KEYS,
  TopicDTO,
  useClearHelixOrgTopicMessages,
  useHelixOrgTopic,
  useGitHubWebhookStatus,
  useInstallGitHubWebhook,
  useTopicMessageCount,
  useUpdateHelixOrgTopic,
} from '../services/helixOrgService'

const HelixOrgTopicDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const orgSlug = router.params.org_id as string | undefined
  const topicId = router.params.topic_id as string | undefined

  const { data: topic, isLoading } = useHelixOrgTopic(topicId)
  const { data: messageCount } = useTopicMessageCount(topicId)
  const updateTopic = useUpdateHelixOrgTopic()
  const queryClient = useQueryClient()

  // Live event list. Seeded from the initial GET so the page renders
  // immediately; replaced wholesale on every SSE push from
  // /topics/{id}/events. Falling back to the initial snapshot keeps
  // the list non-empty across reconnect blips.
  const [liveEvents, setLiveEvents] = useState<EventCard[] | null>(null)
  const events = liveEvents ?? topic?.recent_events ?? []

  // SSE wiring. For normal browser sessions EventSource sends the
  // helix_session cookie automatically. For embed-token flows
  // (pages loaded with ?access_token=…) the EventSource constructor
  // is patched in useApi.ts to append ?access_token= to same-origin
  // URLs — see the embedToken block there. The server emits
  // `event: message` frames with a JSON-array payload of up to 50
  // events newest-first; we replace state on each.
  const orgID = account.organizationTools.organization?.id || orgSlug || ''
  const queryOrgID = orgSlug || orgID
  const sseUrlRef = useRef<string | null>(null)
  useEffect(() => {
    if (!orgID || !topicId) return
    const url = `/api/v1/orgs/${encodeURIComponent(orgID)}/topics/${encodeURIComponent(topicId)}/events`
    sseUrlRef.current = url
    const es = new EventSource(url, { withCredentials: true })
    const onMessage = (ev: MessageEvent) => {
      try {
        const arr = JSON.parse(ev.data) as EventCard[]
        if (Array.isArray(arr)) {
          setLiveEvents(arr)
          queryClient.invalidateQueries({ queryKey: QUERY_KEYS.topicMessageCount(queryOrgID, topicId) })
        }
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
  }, [orgID, queryOrgID, topicId])

  const subscribers = topic?.subscribers ?? []

  const formatTimestamp = (iso: string) => {
    if (!iso) return ''
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString()
  }

  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Topics', routeName: 'helix_org_topics' })
  const leafTitle = topic?.name || topic?.id || topicId || 'Topic'

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle={leafTitle}>
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          {isLoading ? (
            <LoadingSpinner />
          ) : !topic ? (
            <Typography color="text.secondary">Topic not found.</Typography>
          ) : (
            <>
              <Box>
                <Stack direction="row" alignItems="baseline" spacing={2}>
                  <Typography variant="h5" sx={{ fontFamily: 'monospace' }}>{topic.id}</Typography>
                  <CopyButtonWithCheck text={topic.id} />
                  <Chip label={topic.kind} size="small" sx={{ fontFamily: 'monospace' }} />
                </Stack>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  {topic.name}
                </Typography>
                {topic.description && (
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    {topic.description}
                  </Typography>
                )}
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1, fontFamily: 'monospace' }}>
                  created by {topic.created_by} · {formatTimestamp(topic.created_at)}
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

              <TopicConfigSection
                key={topic.id}
                topic={topic}
                onSave={async (payload) => {
                  try {
                    await updateTopic.mutateAsync({ topicId: topic.id, payload })
                    snackbar.success('topic updated')
                    return true
                  } catch (e: any) {
                    const msg = e?.response?.data?.error || e?.message || 'update failed'
                    snackbar.error(msg)
                    return false
                  }
                }}
                saving={updateTopic.isPending}
              />

              {topic.kind === 'github' && (
                <GitHubWebhookStatus
                  topic={topic}
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
                  <Stack direction="row" spacing={1.5} alignItems="center">
                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                      newest first · up to 50 · live
                    </Typography>
                    <ClearTopicMessagesButton
                      topic={topic}
                      messageCount={messageCount}
                      onCleared={() => setLiveEvents([])}
                    />
                  </Stack>
                </Stack>
                {events.length === 0 ? (
                  <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
                    No events on this topic yet.
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
      </Box>
    </HelixOrgShell>
  )
}

// The transport kind itself is immutable here: changing it mid-flight would
// orphan provider-side resources such as GitHub webhooks.
interface TopicConfigSectionProps {
  topic: TopicDTO
  onSave: (payload: {
    name: string
    description?: string
    transport?: { config?: Record<string, unknown> }
  }) => Promise<boolean>
  saving: boolean
  onCancel?: () => void
}

type TopicForm = {
  name: string
  description: string
  configText: string
  ghRepo: string
  ghBranches: string[]
  ghOriginalConfig: Record<string, unknown>
  cronSchedule: string
  cronMessage: string
}

const topicForm = (topic: TopicDTO): TopicForm => {
  const config = (topic.config ?? {}) as Record<string, unknown>
  return {
    name: topic.name,
    description: topic.description ?? '',
    configText: topic.kind !== 'local' && topic.kind !== 'github' && topic.kind !== 'cron'
      ? JSON.stringify(config, null, 2)
      : '',
    ghRepo: topic.kind === 'github' && typeof config.repo === 'string' ? config.repo : '',
    ghBranches: topic.kind === 'github' && Array.isArray(config.branches) && config.branches.length > 0
      ? config.branches as string[]
      : ['*'],
    ghOriginalConfig: topic.kind === 'github' ? config : {},
    cronSchedule: topic.kind === 'cron' && typeof config.schedule === 'string' ? config.schedule : '',
    cronMessage: topic.kind === 'cron' && typeof config.message === 'string' ? config.message : '',
  }
}

export const TopicConfigSection: FC<TopicConfigSectionProps> = ({ topic, onSave, saving, onCancel }) => {
  const snackbar = useSnackbar()
  const initialForm = topicForm(topic)
  const [form, setForm] = useState<TopicForm>(initialForm)
  const [savedForm, setSavedForm] = useState<TopicForm>(initialForm)
  const dirty = JSON.stringify(form) !== JSON.stringify(savedForm)

  const handleSave = async () => {
    if (!form.name.trim()) {
      snackbar.error('Name is required')
      return
    }
    const payload: {
      name: string
      description?: string
      transport?: { config?: Record<string, unknown> }
    } = { name: form.name.trim(), description: form.description.trim() || undefined }

    let saved = { ...form, name: form.name.trim(), description: form.description.trim() }

    if (topic.kind === 'github') {
      if (!form.ghRepo.trim() || !GITHUB_REPO_PATTERN.test(form.ghRepo.trim())) {
        snackbar.error('GitHub repo is required and must be owner/name')
        return
      }
      const ghConfig: Record<string, unknown> = { ...form.ghOriginalConfig, repo: form.ghRepo.trim() }
      const branches = form.ghBranches.map((b) => b.trim()).filter((b) => b.length > 0)
      if (branches.length > 0) {
        ghConfig.branches = branches
      } else {
        delete ghConfig.branches
      }
      payload.transport = { config: ghConfig }
      saved = { ...saved, ghRepo: form.ghRepo.trim(), ghBranches: branches, ghOriginalConfig: ghConfig }
    } else if (topic.kind === 'cron') {
      const sched = form.cronSchedule.trim()
      if (!sched) {
        snackbar.error('Schedule is required')
        return
      }
      const cronConfig: Record<string, unknown> = { schedule: sched }
      if (form.cronMessage.trim()) {
        cronConfig.message = form.cronMessage.trim()
      }
      payload.transport = { config: cronConfig }
      saved = { ...saved, cronSchedule: sched, cronMessage: form.cronMessage.trim() }
    } else if (topic.kind !== 'local' && form.configText.trim()) {
      try {
        const parsed = JSON.parse(form.configText)
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          snackbar.error('Transport config must be a JSON object')
          return
        }
        payload.transport = { config: parsed }
      } catch (e) {
        snackbar.error('Transport config must be valid JSON')
        return
      }
    } else if (topic.kind !== 'local') {
      // Allow clearing back to no config.
      payload.transport = { config: {} }
    }
    const ok = await onSave(payload)
    if (ok) {
      setForm(saved)
      setSavedForm(saved)
    }
  }

  const handleCancel = () => {
    setForm(savedForm)
    onCancel?.()
  }

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>Configuration</Typography>

      <Stack spacing={2}>
          <TextField
            label="Name"
            value={form.name}
            onChange={(e) => setForm((current) => ({ ...current, name: e.target.value }))}
            size="small"
            fullWidth
            required
          />
          <TextField
            label="Description (optional)"
            value={form.description}
            onChange={(e) => setForm((current) => ({ ...current, description: e.target.value }))}
            multiline
            minRows={2}
            size="small"
            fullWidth
          />
          {topic.kind === 'github' && (
            <>
              <GitHubRepoPicker value={form.ghRepo} onChange={(ghRepo) => setForm((current) => ({ ...current, ghRepo }))} />
              <GitHubBranchesField branches={form.ghBranches} onChange={(ghBranches) => setForm((current) => ({ ...current, ghBranches }))} />
              <Typography variant="caption" color="text.secondary">
                Which GitHub event types this webhook delivers is configured on GitHub,
                not here — open the webhook on GitHub (below) to change it.
              </Typography>
            </>
          )}
          {topic.kind === 'cron' && (
            <CronScheduleFields
              value={form.cronSchedule}
              onChange={(cronSchedule) => setForm((current) => ({ ...current, cronSchedule }))}
              message={form.cronMessage}
              onMessageChange={(cronMessage) => setForm((current) => ({ ...current, cronMessage }))}
            />
          )}
          {topic.kind !== 'local' && topic.kind !== 'github' && topic.kind !== 'cron' && (
            <TextField
              label="Transport config (JSON)"
              value={form.configText}
              onChange={(e) => setForm((current) => ({ ...current, configText: e.target.value }))}
              multiline
              minRows={4}
              size="small"
              fullWidth
              helperText='e.g. {"outbound_url": "https://example.com/hook"} for webhook, {"inbound_address": "ingest@…"} for postmark. Leave empty to clear config.'
              sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.8rem' } }}
            />
          )}
          {topic.kind === 'local' && (
            <Typography variant="caption" color="text.secondary">
              local transport has no config — nothing to edit beyond name/description.
            </Typography>
          )}
      </Stack>
      {dirty && (
        <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ mt: 2, pt: 2, borderTop: '1px solid', borderColor: 'divider' }}>
          <Button
            color="secondary"
            variant="contained"
            size="small"
            startIcon={<SaveIcon />}
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? 'Saving…' : 'Save'}
          </Button>
          <Button variant="text" size="small" onClick={handleCancel} disabled={saving}>
            Cancel
          </Button>
        </Stack>
      )}
    </Box>
  )
}

export const ClearTopicMessagesButton: FC<{
  topic: TopicDTO
  messageCount?: number
  onCleared?: () => void
}> = ({ topic, messageCount, onCleared }) => {
  const snackbar = useSnackbar()
  const clearMessages = useClearHelixOrgTopicMessages()
  const { data: queriedMessageCount } = useTopicMessageCount(topic.id)
  const [confirming, setConfirming] = useState(false)
  const count = messageCount ?? queriedMessageCount

  const clear = async () => {
    try {
      await clearMessages.mutateAsync(topic.id)
      onCleared?.()
      setConfirming(false)
      snackbar.success('topic messages cleared')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error || e?.message || 'failed to clear topic messages')
    }
  }

  return (
    <>
      <Button
        color="error"
        variant="outlined"
        size="small"
        startIcon={<DeleteSweepIcon />}
        onClick={() => setConfirming(true)}
        disabled={count === 0}
      >
        Clear messages
      </Button>
      <Dialog open={confirming} onClose={() => !clearMessages.isPending && setConfirming(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Clear all topic messages?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This permanently deletes all messages retained on “{topic.name || topic.id}”. The topic and its subscribers will remain active.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirming(false)} disabled={clearMessages.isPending}>
            Cancel
          </Button>
          <Button color="error" variant="contained" onClick={clear} disabled={clearMessages.isPending}>
            {clearMessages.isPending ? 'Clearing…' : 'Clear messages'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

// GitHubWebhookStatus is the simplified "Connect to GitHub" panel.
// Helix auto-installs the webhook on GitHub when the topic is
// created, so most operators never need to copy a URL or paste a
// secret. This component just surfaces the current state and gives
// the operator a deep-link to the GitHub UI for tweaks.
//
// States:
//   - webhook_id set on the topic config → "Helix installed
//     webhook #N on owner/name." + Edit-on-GitHub link
//   - webhook_id unset → "Webhook not installed yet" + button to
//     re-run the install
//   - localhost SERVER_URL → red warning ("change SERVER_URL or
//     GitHub can't deliver")
interface GitHubWebhookStatusProps {
  topic: TopicDTO
  orgSlug?: string
}

// SettingsLink is the actionable button on the loopback warning —
// jumps the operator to the helix-org Settings page where
// `topics.public_url` can be set. Stops the user staring at
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

const GitHubWebhookStatus: FC<GitHubWebhookStatusProps> = ({ topic, orgSlug }) => {
  const snackbar = useSnackbar()
  const install = useInstallGitHubWebhook()
  // Live truth from GitHub: does a webhook for this topic's payload URL
  // actually exist on the repo? This is the source of truth for the link vs
  // re-install decision — the stored config can be stale (hook deleted on
  // GitHub, or installed before we tracked the id).
  const status = useGitHubWebhookStatus(topic.id)

  // Check the EFFECTIVE public URL — what the install endpoint
  // would actually use (topics.public_url override applied on
  // top of SERVER_URL). When the operator sets that org config
  // to a publicly reachable URL the warning goes away even
  // though the SERVER_URL env still points at localhost.
  // Fallback to window.location.origin only when the server
  // sent nothing (e.g. older API).
  const effectivePublicURL = topic.effective_public_url && topic.effective_public_url.length > 0
    ? topic.effective_public_url
    : window.location.origin
  const isLocalhost = /(localhost|127\.0\.0\.1|0\.0\.0\.0)/i.test(effectivePublicURL)

  const cfg = (topic.config ?? {}) as { repo?: string; webhook_id?: number; webhook_html_url?: string }

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
      const out = await install.mutateAsync(topic.id)
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
            (a) setting <code>topics.public_url</code> on the helix-org Settings page to a publicly reachable host (cloudflared / ngrok / reverse proxy), or
            (b) editing <code>SERVER_URL</code> in helix's .env and restarting the api container.
          </Typography>
          <SettingsLink orgSlug={orgSlug} />
        </Box>
      )}
      {view === 'loading' ? (
        <Stack direction="row" spacing={1} alignItems="center">
          <CircularProgress size={16} />
          <Typography variant="body2" color="text.secondary">
            Checking GitHub for this topic's webhook…
          </Typography>
        </Stack>
      ) : view === 'installed' ? (
        <Stack spacing={1}>
          <Typography variant="body2">
            Webhook registered on <strong>{cfg.repo}</strong>{webhookId ? <> (id <code>{webhookId}</code>)</> : null}.
            {active ? ' Deliveries flow into this topic automatically.' : ' ⚠ It is currently disabled on GitHub, so no deliveries arrive — re-install to re-enable.'}
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
            Tweak the events whitelist (or any other webhook settings) directly on GitHub's UI. Helix routes deliveries by repo + topic id, so as long as the payload URL stays intact your changes take effect immediately.
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
// header showing how many messages are retained on the topic (meta.total
// from the paginated messages endpoint). Undefined while the count query
// is in flight — render an em-dash placeholder rather than 0 so a
// loading state doesn't read as "empty topic".
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
      retained
    </Typography>
  </Paper>
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

export default HelixOrgTopicDetail
