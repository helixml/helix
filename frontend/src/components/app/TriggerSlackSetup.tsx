import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import InputAdornment from '@mui/material/InputAdornment'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { IAppFlatState } from '../../types'
import { SetupStep, SetupStepList, CopyableCodeBlock } from '../slack/SlackSetupScaffold'

// Screenshots live in Vite's publicDir (frontend/assets), served at the
// site root — reference them by URL, NOT a JS import (Vite forbids
// importing publicDir assets, which silently breaks the <img>).
const createSlackAppScreenshot = '/img/slack/create_new_app.png'
const createSlackAppManifest = '/img/slack/manifest.png'
const createSlackApp = '/img/slack/create.png'
const createSlackAppToken = '/img/slack/app_token.png'
const createSlackAppTokenScopes = '/img/slack/app_token_scopes.png'
const createSlackAppInstall = '/img/slack/install_app.png'

const getSlackAppManifest = (appName: string, description: string) => `{
    "display_information": {
        "name": "${appName}",
        "description": "${description}",
        "background_color": "#69264d"
    },
    "features": {
        "app_home": {
            "home_tab_enabled": false,
            "messages_tab_enabled": true,
            "messages_tab_read_only_enabled": true
        },
        "bot_user": {
            "display_name": "${appName}",
            "always_online": true
        }
    },
    "oauth_config": {
        "scopes": {
            "bot": [
                "app_mentions:read",
                "channels:history",
                "channels:join",
                "channels:manage",
                "channels:read",
                "chat:write",
                "chat:write.customize",
                "chat:write.public",
                "files:read",
                "files:write",
                "groups:history",
                "groups:read",
                "groups:write",
                "im:history",
                "im:read",
                "im:write",
                "links:read",
                "links:write",
                "mpim:history",
                "mpim:read",
                "mpim:write",
                "pins:read",
                "pins:write",
                "reactions:read",
                "reactions:write",
                "reminders:read",
                "reminders:write",
                "team:read",
                "usergroups:read",
                "usergroups:write",
                "users.profile:read",
                "users:read",
                "users:write",
                "assistant:write",
                "users:read.email"
            ]
        }
    },
    "settings": {
        "event_subscriptions": {
            "bot_events": [
                "app_mention",
                "message.channels",
                "message.groups",
                "message.im",
                "message.mpim"
            ]
        },
        "interactivity": {
            "is_enabled": true
        },
        "org_deploy_enabled": false,
        "socket_mode_enabled": true,
        "token_rotation_enabled": false
    }
}`

interface TriggerSlackSetupProps {
  open: boolean
  onClose: () => void
  app: IAppFlatState
  appToken?: string
  botToken?: string
  onAppTokenChange?: (token: string) => void
  onBotTokenChange?: (token: string) => void
}

const TriggerSlackSetup: FC<TriggerSlackSetupProps> = ({
  open,
  onClose,
  app,
  appToken = '',
  botToken = '',
  onAppTokenChange,
  onBotTokenChange,
}) => {
  const [showAppToken, setShowAppToken] = useState(false)
  const [showBotToken, setShowBotToken] = useState(false)

  const manifest = getSlackAppManifest(app.name || 'Helix Agent', app.description || 'AI-powered Slack integration')

  const tokenField = (
    value: string,
    show: boolean,
    setShow: (v: boolean) => void,
    placeholder: string,
    helper: string,
    onChange?: (t: string) => void,
    label?: string,
  ) => (
    <>
      <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>{label}</Typography>
      <TextField
        fullWidth
        size="small"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange?.(e.target.value)}
        helperText={helper}
        type={show ? 'text' : 'password'}
        autoComplete="new-password"
        InputProps={{
          endAdornment: (
            <InputAdornment position="end">
              <IconButton onClick={() => setShow(!show)} edge="end">
                {show ? <VisibilityOff /> : <Visibility />}
              </IconButton>
            </InputAdornment>
          ),
        }}
      />
    </>
  )

  const steps: SetupStep[] = [
    { step: 1, text: 'Go to api.slack.com/apps', link: 'https://api.slack.com/apps', linkLabel: 'api.slack.com/apps ↗' },
    { step: 2, text: 'Click "Create New App"', image: createSlackAppScreenshot },
    { step: 3, text: 'Choose "From a manifest" and copy the provided manifest into the text field', below: <CopyableCodeBlock code={manifest} /> },
    { step: 4, text: 'Select the workspace that you want to install the app to' },
    { step: 5, text: 'Copy/paste the manifest into your app', image: createSlackAppManifest },
    { step: 6, text: 'Click "Create"', image: createSlackApp },
    { step: 7, text: 'In "Basic Information" create an App-Level Token with scope "connections:write" (required for Socket Mode)', image: createSlackAppToken },
    { step: 8, text: 'Generate the App-Level token and copy the xapp- token into Helix', image: createSlackAppTokenScopes,
      below: tokenField(appToken, showAppToken, setShowAppToken, 'xapp-...', 'Your Slack app token (starts with xapp-)', onAppTokenChange, 'Copy the generated App Token here:') },
    { step: 9, text: 'Go to "Install App" and generate/reinstall to get the "Bot User OAuth Token" (xoxb-)', image: createSlackAppInstall,
      below: tokenField(botToken, showBotToken, setShowBotToken, 'xoxb-...', 'Your Slack bot token (starts with xoxb-)', onBotTokenChange, 'Copy the generated Bot User OAuth Token here:') },
  ]

  return (
    <>
      <DarkDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
        <DialogTitle sx={{ pb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <SlackLogo sx={{ fontSize: 24, color: 'primary.main' }} />
            <Typography variant="h6">Slack App Setup Instructions</Typography>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Follow these steps to set up your Slack app and get the required tokens:
          </Typography>
          <SetupStepList steps={steps} />
        </DialogContent>
        <DialogActions sx={{ p: 3, pt: 1 }}>
          <Button onClick={onClose} variant="outlined">Close</Button>
        </DialogActions>
      </DarkDialog>
    </>
  )
}

export default TriggerSlackSetup
