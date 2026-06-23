// SlackAppSetup is the instructions dialog for creating the single,
// deployment-wide Helix Slack app — the one org admins later install into
// their own workspaces. It reuses the shared Slack setup scaffold
// (numbered steps, expandable manifest, copy fields) and only supplies
// the global-app content: a manifest pre-filled with THIS deployment's
// OAuth redirect + Events request URLs, and steps that differ for REST
// (Events API, per-org OAuth install) vs Socket Mode (self-hosted).

import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { SetupStep, SetupStepList, CopyField, CopyableCodeBlock } from '../slack/SlackSetupScaffold'

// Screenshots live in Vite's publicDir (frontend/assets), served at the
// site root — reference them by URL, not a JS import.
const createSlackAppScreenshot = '/img/slack/create_new_app.png'
const createSlackAppManifest = '/img/slack/manifest.png'
const createSlackAppToken = '/img/slack/app_token.png'
const createSlackAppInstall = '/img/slack/install_app.png'

// Bot scopes the global app requests — keep in sync with the backend's
// defaultSlackBotScopes (helix_org_slack.go).
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
]

const BOT_EVENTS = ['app_mention', 'message.channels', 'message.groups', 'message.im', 'message.mpim']

// buildManifest returns a Slack app manifest pre-filled for this
// deployment. REST embeds the OAuth redirect URL and disables Socket
// Mode; Socket Mode enables the socket (events arrive over the WebSocket,
// so no public request URL is needed).
const buildManifest = (mode: 'rest' | 'socket', redirectURL: string): string => {
  const manifest: any = {
    display_information: {
      name: 'Helix',
      description: 'Helix AI — connect your Slack workspace to Helix agents.',
      background_color: '#69264d',
    },
    features: { bot_user: { display_name: 'Helix', always_online: true } },
    oauth_config: { scopes: { bot: BOT_SCOPES } },
    settings: {
      event_subscriptions: { bot_events: BOT_EVENTS },
      org_deploy_enabled: false,
      socket_mode_enabled: mode === 'socket',
      token_rotation_enabled: false,
    },
  }
  if (mode === 'rest') {
    // The Events request URL is added by hand AFTER the signing secret is
    // saved in Helix (Slack verifies it on submit), so it's left out here
    // to avoid a failed verification at create time — the step below has
    // a copy field for it.
    manifest.oauth_config.redirect_urls = [redirectURL]
  }
  return JSON.stringify(manifest, null, 2)
}

interface SlackAppSetupProps {
  open: boolean
  onClose: () => void
  ingressMode: 'rest' | 'socket'
}

const SlackAppSetup: FC<SlackAppSetupProps> = ({ open, onClose, ingressMode }) => {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const redirectURL = `${origin}/api/v1/slack/oauth/callback`
  const eventsURL = `${origin}/api/v1/slack/events`
  const manifest = useMemo(() => buildManifest(ingressMode, redirectURL), [ingressMode, redirectURL])

  const steps: SetupStep[] = ingressMode === 'rest'
    ? [
        { step: 1, text: 'Go to api.slack.com/apps and click "Create New App" → "From a manifest".', link: 'https://api.slack.com/apps', image: createSlackAppScreenshot },
        { step: 2, text: 'Choose any workspace to author the app in. This is only where the app lives — your org admins install it into their own workspaces, they don\'t create their own.' },
        { step: 3, text: 'Paste this manifest (it pre-fills the bot scopes, events, and your Helix OAuth Redirect URL), then click "Create".', image: createSlackAppManifest, below: <CopyableCodeBlock code={manifest} /> },
        { step: 4, text: 'Open "Basic Information" → "App Credentials". Copy the Client ID, Client Secret and Signing Secret into the form below, and Save — Helix needs them before the next step.' },
        { step: 5, text: 'Open "Event Subscriptions", turn it on, and set the Request URL to the value below. Slack verifies it instantly now that Helix has the signing secret.', below: <CopyField label="Events Request URL" value={eventsURL} /> },
        { step: 6, text: 'Done. Org admins can now click "Install to Slack" from their org Settings to add Helix to their workspace.' },
      ]
    : [
        { step: 1, text: 'Go to api.slack.com/apps and click "Create New App" → "From a manifest".', link: 'https://api.slack.com/apps', image: createSlackAppScreenshot },
        { step: 2, text: 'Choose the single workspace this self-hosted Helix will serve.' },
        { step: 3, text: 'Paste this manifest (Socket Mode enabled — events arrive over a WebSocket, no public URL needed), then click "Create".', image: createSlackAppManifest, below: <CopyableCodeBlock code={manifest} /> },
        { step: 4, text: 'Open "Basic Information" → "App-Level Tokens" and generate a token with the connections:write scope. Copy the xapp- token into the form below.', image: createSlackAppToken },
        { step: 5, text: 'Open "Install App", install to the workspace, and copy the Bot User OAuth Token (xoxb-) into the form below.', image: createSlackAppInstall },
        { step: 6, text: 'Save. Helix opens the Socket Mode connection automatically — no Redirect or Request URL required.' },
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
          {ingressMode === 'rest'
            ? ' Each org admin then installs it into their own Slack workspace with one click — they never create their own app.'
            : ' In Socket Mode it serves a single workspace (self-hosted / on-prem).'}
        </Typography>

        {ingressMode === 'rest' && (
          <Box sx={{ mb: 2 }}>
            <CopyField label="OAuth Redirect URL (already in the manifest)" value={redirectURL} />
          </Box>
        )}

        <SetupStepList steps={steps} />
      </DialogContent>
      <DialogActions sx={{ p: 3, pt: 1 }}>
        <Button onClick={onClose} variant="outlined">Close</Button>
      </DialogActions>
    </DarkDialog>
  )
}

export default SlackAppSetup
