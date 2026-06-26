// SlackAppSetup is the instructions dialog for creating the single,
// deployment-wide Helix Slack app — the one org admins later install into
// their own workspaces. It reuses the shared Slack setup scaffold
// (numbered steps, expandable manifest, copy fields) and only supplies
// the global-app content: a manifest pre-filled with THIS deployment's
// OAuth redirect + Events request URLs, and steps that differ for REST
// (Events API, per-org OAuth install) vs Socket Mode (self-hosted).

import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { SetupStep, SetupStepList, CopyField, CopyableCodeBlock } from '../slack/SlackSetupScaffold'
import useAccount from '../../hooks/useAccount'

// Screenshots live in Vite's publicDir (frontend/assets), served at the
// site root — reference them by URL, not a JS import.
const createSlackAppManifest = '/img/slack/manifest.png'
const createSlackAppToken = '/img/slack/app_token.png'

// The Helix logo the operator uploads as the Slack app icon. Slack
// manifests can't carry an image, so this is the one branding step done by
// hand. The helixml GitHub org avatar is a public, square PNG (512px) —
// the same mark the bot posts under (reply_hint.go's icon_url).
const helixSlackAppIcon = 'https://avatars.githubusercontent.com/u/149581110?s=512'

// Bot scopes the global app requests. The backend's defaultSlackBotScopes
// (helix_org_slack.go) is authoritative — it's what the OAuth install
// actually requests — so this manifest list must stay a superset of it.
const BOT_SCOPES = [
  'app_mentions:read',
  'channels:history',
  'channels:read',
  'channels:join',
  'groups:history',
  'groups:read',
  'im:history',
  'chat:write',
  'chat:write.customize',
  'reactions:write',
  'files:write',
]

// Each subscribed message.* event requires its matching *:history scope,
// so the event list is derived from BOT_SCOPES rather than maintained by
// hand — adding (or dropping) a *:history scope updates both at once.
const SCOPE_EVENT: Record<string, string> = {
  'app_mentions:read': 'app_mention',
  'channels:history': 'message.channels',
  'groups:history': 'message.groups',
  'im:history': 'message.im',
  'mpim:history': 'message.mpim',
}
const BOT_EVENTS = BOT_SCOPES.map((s) => SCOPE_EVENT[s]).filter(Boolean)

// buildManifest returns a Slack app manifest pre-filled for this
// deployment. REST embeds the OAuth redirect URL and disables Socket
// Mode; Socket Mode enables the socket (events arrive over the WebSocket,
// so no public request URL is needed).
const buildManifest = (mode: 'rest' | 'socket', redirectURL: string, appName?: string): string => {
  // Slack caps the app name at 35 chars and the bot display name at 80;
  // fall back to "Helix" when no connection name was given.
  const name = (appName || '').trim().slice(0, 35) || 'Helix'
  const manifest: any = {
    display_information: {
      name,
      description: 'Helix AI — connect your Slack workspace to Helix agents.',
      // The richest branding the manifest can carry. Slack does NOT
      // support an app icon in the manifest (it's uploaded by hand in
      // Basic Information → Display Information after the app is created),
      // so name / description / long_description / background_color are
      // all the Helix identity we can pre-fill.
      long_description:
        'Helix connects this Slack workspace to your Helix AI agents and org-chart Workers. ' +
        'Mention a Worker or post in a connected channel and the right agent picks the message up, ' +
        'reads the surrounding thread, and replies right here in Slack — all backed by your own ' +
        'Helix deployment. Learn more at https://helix.ml.',
      background_color: '#69264d',
    },
    features: { bot_user: { display_name: name, always_online: true } },
    oauth_config: { scopes: { bot: BOT_SCOPES } },
    settings: {
      event_subscriptions: { bot_events: BOT_EVENTS },
      org_deploy_enabled: false,
      socket_mode_enabled: mode === 'socket',
      token_rotation_enabled: false,
    },
  }
  manifest.oauth_config.redirect_urls = [redirectURL]
  return JSON.stringify(manifest, null, 2)
}

interface SlackAppSetupProps {
  open: boolean
  onClose: () => void
  ingressMode: 'rest' | 'socket'
  // appName is the connection name the operator typed; the manifest uses it
  // so the Slack app + bot are named to match, rather than a generic "Helix".
  appName?: string
}

const SlackAppSetup: FC<SlackAppSetupProps> = ({ open, onClose, ingressMode, appName }) => {
  const account = useAccount()
  const origin = account.serverConfig?.server_url || (typeof window !== 'undefined' ? window.location.origin : '')
  const redirectURL = `${origin}/api/v1/slack/oauth/callback`
  const eventsURL = `${origin}/api/v1/slack/events`
  const manifest = useMemo(() => buildManifest(ingressMode, redirectURL, appName), [ingressMode, redirectURL, appName])
  const createAppURL = `https://api.slack.com/apps?new_app=1&manifest_json=${encodeURIComponent(manifest)}`

  const manifestFallback = (
    <CopyableCodeBlock title="Prefer to paste the manifest yourself?" code={manifest} />
  )

  const distributionNote = 'Optional — only if orgs will install into workspaces other than the one that owns the app (e.g. a SaaS deployment): open "Manage Distribution" and activate public distribution.'

  // The manifest pre-fills every field Slack allows, but not the app icon
  // (Slack has no manifest field for it). This is the one branding step the
  // operator does by hand, on the same Basic Information page as the
  // credentials below.
  const iconStep: SetupStep = {
    step: 2,
    text: 'Give the app its Helix icon: under "Basic Information" → "Display Information", upload the Helix logo as the App icon (the manifest can\'t carry an image, so this is the one branding step you do by hand).',
    below: (
      <Button size="small" variant="text" href={helixSlackAppIcon} target="_blank" rel="noopener noreferrer" sx={{ pl: 0 }}>
        Download the Helix icon
      </Button>
    ),
  }

  const steps: SetupStep[] = ingressMode === 'rest'
    ? [
        { step: 1, text: 'Click "Create the app in Slack" above. Slack opens its create screen pre-filled with the scopes, events, and your Helix redirect URL — pick the workspace to own the app and click "Create".', image: createSlackAppManifest, below: manifestFallback },
        iconStep,
        { step: 3, text: 'Open "Basic Information" → "App Credentials" and copy the Client ID, Client Secret, and Signing Secret into the form below, then Save.' },
        { step: 4, text: 'Open "Event Subscriptions", turn it on, and set the Request URL to the value below — Slack verifies it instantly once the signing secret is saved.', below: <CopyField label="Events Request URL" value={eventsURL} /> },
        { step: 5, text: distributionNote },
      ]
    : [
        { step: 1, text: 'Click "Create the app in Slack" above. Slack opens pre-filled (Socket Mode enabled + your redirect URL) — pick the workspace to own the app and click "Create".', image: createSlackAppManifest, below: manifestFallback },
        iconStep,
        { step: 3, text: 'Open "Basic Information" → "App Credentials" and copy the Client ID and Client Secret into the form below.' },
        { step: 4, text: 'Open "Basic Information" → "App-Level Tokens", generate a token with the connections:write scope, and copy the xapp- token into the form below, then Save.', image: createSlackAppToken },
        { step: 5, text: distributionNote },
      ]

  return (
    <DarkDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <SlackLogo sx={{ fontSize: 24, color: 'primary.main' }} />
          <Typography variant="h6">Create the global Helix Slack app</Typography>
        </Box>
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          You're creating <strong>one</strong> Slack app for this whole Helix deployment.
          {' '}Each org admin then installs it into their own workspace with one click — they never create their own app.
          {ingressMode === 'socket' && ' Socket Mode delivers every installed workspace\'s events over one WebSocket.'}
        </Typography>

        <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 2 }}>
          <Button variant="contained" startIcon={<SlackLogo sx={{ fontSize: 18 }} />} href={createAppURL} target="_blank" rel="noopener noreferrer">
            Create the app in Slack
          </Button>
          <Typography variant="caption" color="text.secondary">Opens Slack pre-filled with this configuration.</Typography>
        </Stack>

        <Box sx={{ mb: 2 }}>
          <CopyField label="OAuth Redirect URL" value={redirectURL} />
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
            The manifest adds this for newly-created apps. For an existing app, make sure it's listed under
            OAuth &amp; Permissions → Redirect URLs, or the install will fail with a redirect_uri mismatch.
          </Typography>
        </Box>

        <SetupStepList steps={steps} />

        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          That's it — you don't install the app or copy a bot token yourself. Org admins open their
          Settings → Slack and click <strong>Install into your workspace</strong>; Helix runs the OAuth
          install and stores the bot token for them.
        </Typography>
      </DialogContent>
      <DialogActions sx={{ p: 3, pt: 1 }}>
        <Button onClick={onClose} variant="outlined">Close</Button>
      </DialogActions>
    </DarkDialog>
  )
}

export default SlackAppSetup
