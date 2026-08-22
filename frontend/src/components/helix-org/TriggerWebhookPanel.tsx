// TriggerWebhookPanel is the "Connect to GitHub" panel on a GitHub
// Trigger's detail page: it asks GitHub whether a webhook for this
// Trigger's payload URL really exists, and installs or re-installs it
// with one click. Live status beats the stored config, which goes stale
// the moment someone deletes the hook on GitHub.

import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import {
  InstallWebhookFailedError,
  TriggerDTO,
  useGitHubWebhookStatus,
  useInstallGitHubWebhook,
} from '../../services/triggerService'

const SettingsLink: FC<{ orgSlug?: string }> = ({ orgSlug }) => {
  const router = useRouter()
  if (!orgSlug) return null
  return (
    <Button
      size="small"
      variant="outlined"
      onClick={() => router.navigate('org_general', { org_id: orgSlug })}
      sx={{ color: 'warning.contrastText', borderColor: 'warning.contrastText' }}
    >
      Open Organization Settings
    </Button>
  )
}

const TriggerWebhookPanel: FC<{ trigger: TriggerDTO; orgSlug?: string }> = ({ trigger, orgSlug }) => {
  const snackbar = useSnackbar()
  const install = useInstallGitHubWebhook()
  const status = useGitHubWebhookStatus(trigger.id)

  // The EFFECTIVE public URL is what the install endpoint would actually
  // use. Fall back to the browser origin only when the server sent
  // nothing.
  const effectivePublicURL = trigger.effective_public_url || window.location.origin
  const isLocalhost = /(localhost|127\.0\.0\.1|0\.0\.0\.0)/i.test(effectivePublicURL)

  const cfg = (trigger.config ?? {}) as { repo?: string; webhook_id?: number; webhook_html_url?: string }

  // Resolve live status into one view. "unknown" (couldn't reach GitHub /
  // no creds / no public URL) degrades to the stored config so the panel
  // still works instead of always claiming "missing".
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
    unknownNote = live?.detail || (status.error as any)?.message || ''
    if (cfg.webhook_id) {
      view = 'installed'
      webhookHtmlUrl = cfg.webhook_html_url || ''
      webhookId = cfg.webhook_id
    }
  }

  const reinstall = async () => {
    try {
      const out = await install.mutateAsync(trigger.id ?? '')
      if (out.warning) {
        // The webhook IS registered; the warning says what is still wrong
        // on the operator's side (typically SERVER_URL on localhost). A
        // success toast would be misleading.
        snackbar.error(`Webhook installed (id ${out.webhook_id}), but: ${out.warning}`)
      } else {
        snackbar.success(`Webhook installed on GitHub (id ${out.webhook_id})`)
      }
    } catch (e: any) {
      // The API layer already showed the server's error snackbar for
      // InstallWebhookFailedError — don't double-toast.
      if (!(e instanceof InstallWebhookFailedError)) {
        snackbar.error(e?.response?.data?.error ?? e?.message ?? 'install failed')
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
            (a) setting <code>topics.public_url</code> in Organization Settings to a publicly reachable host (cloudflared / ngrok / reverse proxy), or
            (b) editing <code>SERVER_URL</code> in helix's .env and restarting the api container.
          </Typography>
          <SettingsLink orgSlug={orgSlug} />
        </Box>
      )}
      {view === 'loading' ? (
        <Stack direction="row" spacing={1} alignItems="center">
          <CircularProgress size={16} />
          <Typography variant="body2" color="text.secondary">Checking GitHub for this Trigger's webhook…</Typography>
        </Stack>
      ) : view === 'installed' ? (
        <Stack spacing={1}>
          <Typography variant="body2">
            Webhook registered on <strong>{cfg.repo}</strong>{webhookId ? <> (id <code>{webhookId}</code>)</> : null}.
            {active ? ' Deliveries start your agents automatically.' : ' ⚠ It is currently disabled on GitHub, so no deliveries arrive — re-install to re-enable.'}
          </Typography>
          {live?.state === 'installed' && live.events && live.events.length > 0 && (
            <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap>
              <Typography variant="caption" color="text.secondary">GitHub events:</Typography>
              {live.events.map((event) => (
                <Chip key={event} label={event} size="small" variant="outlined" sx={{ fontFamily: 'monospace' }} />
              ))}
            </Stack>
          )}
          <Stack direction="row" spacing={1} alignItems="center">
            {webhookHtmlUrl && (
              <Button size="small" variant="outlined" component="a" href={webhookHtmlUrl} target="_blank" rel="noopener noreferrer">
                View on GitHub →
              </Button>
            )}
            <Button size="small" variant="text" onClick={reinstall} disabled={install.isPending}>
              {install.isPending ? 'Re-installing…' : 'Re-install'}
            </Button>
          </Stack>
          {unknownNote && (
            <Typography variant="caption" color="text.secondary">
              Couldn't confirm against GitHub ({unknownNote}); showing last-known state.
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            Tweak the events whitelist (or any other webhook settings) directly on GitHub's UI. Helix routes deliveries by repo + Trigger id, so as long as the payload URL stays intact your changes take effect immediately.
          </Typography>
        </Stack>
      ) : (
        <Stack spacing={1.5}>
          <Typography variant="body2">
            No webhook found on GitHub for <strong>{cfg.repo || '(repo not set)'}</strong>. Helix can install it for you — one click, no copying URLs.
          </Typography>
          <Box>
            <Button variant="contained" onClick={reinstall} disabled={install.isPending || !cfg.repo}>
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

export default TriggerWebhookPanel
