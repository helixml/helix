import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import IconButton from '@mui/material/IconButton'
import CloseIcon from '@mui/icons-material/Close'
import TextField from '@mui/material/TextField'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import InputAdornment from '@mui/material/InputAdornment'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { IAppFlatState } from '../../types'
import { SetupStep, SetupStepList, CopyableCodeBlock } from '../slack/SlackSetupScaffold'

import createSlackAppScreenshot from '../../../assets/img/slack/create_new_app.png'
import createSlackAppManifest from '../../../assets/img/slack/manifest.png'
import createSlackApp from '../../../assets/img/slack/create.png'
import createSlackAppToken from '../../../assets/img/slack/app_token.png'
import createSlackAppTokenScopes from '../../../assets/img/slack/app_token_scopes.png'
import createSlackAppInstall from '../../../assets/img/slack/install_app.png'

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

// Screenshot rendered indented under a step. Clicking enlarges it.
const StepImage: FC<{ src: string; step: number; onClick: (src: string) => void }> = ({ src, step, onClick }) => (
  <Box sx={{ position: 'relative', display: 'inline-block' }}>
    <Box
      component="img"
      src={src}
      alt={`Step ${step} screenshot`}
      onClick={() => onClick(src)}
      sx={{
        width: '80%', maxWidth: '80%', maxHeight: '200px', height: 'auto',
        borderRadius: 1, border: '1px solid rgba(255,255,255,0.1)',
        boxShadow: '0 2px 8px rgba(0,0,0,0.3)', cursor: 'pointer',
        transition: 'transform 0.2s ease-in-out',
        '&:hover': { transform: 'scale(1.02)', boxShadow: '0 4px 12px rgba(0,0,0,0.4)' },
      }}
    />
  </Box>
)

const TriggerSlackSetup: FC<TriggerSlackSetupProps> = ({
  open,
  onClose,
  app,
  appToken = '',
  botToken = '',
  onAppTokenChange,
  onBotTokenChange,
}) => {
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
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
    { step: 2, text: 'Click "Create New App"', below: <StepImage src={createSlackAppScreenshot} step={2} onClick={setSelectedImage} /> },
    { step: 3, text: 'Choose "From a manifest" and copy the provided manifest into the text field', below: <CopyableCodeBlock code={manifest} /> },
    { step: 4, text: 'Select the workspace that you want to install the app to' },
    { step: 5, text: 'Copy/paste the manifest into your app', below: <StepImage src={createSlackAppManifest} step={5} onClick={setSelectedImage} /> },
    { step: 6, text: 'Click "Create"', below: <StepImage src={createSlackApp} step={6} onClick={setSelectedImage} /> },
    { step: 7, text: 'In "Basic Information" create an App-Level Token with scope "connections:write" (required for Socket Mode)', below: <StepImage src={createSlackAppToken} step={7} onClick={setSelectedImage} /> },
    { step: 8, text: 'Generate the App-Level token and copy the xapp- token into Helix', below: (
      <>
        <Box sx={{ mb: 2 }}><StepImage src={createSlackAppTokenScopes} step={8} onClick={setSelectedImage} /></Box>
        {tokenField(appToken, showAppToken, setShowAppToken, 'xapp-...', 'Your Slack app token (starts with xapp-)', onAppTokenChange, 'Copy the generated App Token here:')}
      </>
    ) },
    { step: 9, text: 'Go to "Install App" and generate/reinstall to get the "Bot User OAuth Token" (xoxb-)', below: (
      <>
        <Box sx={{ mb: 2 }}><StepImage src={createSlackAppInstall} step={9} onClick={setSelectedImage} /></Box>
        {tokenField(botToken, showBotToken, setShowBotToken, 'xoxb-...', 'Your Slack bot token (starts with xoxb-)', onBotTokenChange, 'Copy the generated Bot User OAuth Token here:')}
      </>
    ) },
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

      {/* Image Modal */}
      <DarkDialog
        open={!!selectedImage}
        onClose={() => setSelectedImage(null)}
        PaperProps={{ sx: { background: 'transparent', boxShadow: 'none', overflow: 'visible', display: 'inline-block', p: 0, m: 0 } }}
      >
        <Box sx={{ position: 'relative', textAlign: 'center', p: 0, m: 0 }}>
          <IconButton
            aria-label="close"
            onClick={() => setSelectedImage(null)}
            sx={{ position: 'absolute', top: 8, right: 8, zIndex: 2, color: 'white', background: 'rgba(0,0,0,0.4)', '&:hover': { background: 'rgba(0,0,0,0.7)' } }}
          >
            <CloseIcon />
          </IconButton>
          {selectedImage && (
            <Box component="img" src={selectedImage} alt="Enlarged screenshot"
              sx={{ maxWidth: '600px', maxHeight: '60vh', height: 'auto', borderRadius: 1, boxShadow: '0 4px 24px rgba(0,0,0,0.7)' }} />
          )}
        </Box>
      </DarkDialog>
    </>
  )
}

export default TriggerSlackSetup
