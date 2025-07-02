import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import Alert from '@mui/material/Alert'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import CopyButton from '../common/CopyButton'
import { IAppFlatState } from '../../types'

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
                "message.channels"
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

const setupSteps = [
  {
    step: 1,
    text: 'Go to https://api.slack.com/apps',
    link: 'https://api.slack.com/apps'
  },
  {
    step: 2,
    text: 'Click "Create New App"'
  },
  {
    step: 3,
    text: 'Choose "From a manifest"'
  },
  {
    step: 4,
    text: 'Select workspace'
  },
  {
    step: 5,
    text: 'Copy paste the manifest into your app'
  },
  {
    step: 6,
    text: 'Get app token'
  },
  {
    step: 7,
    text: 'Go to "Install App" and generate the "Bot User OAuth Token"'
  }
]

interface TriggerSlackSetupProps {
  open: boolean
  onClose: () => void
  app: IAppFlatState
}

const TriggerSlackSetup: FC<TriggerSlackSetupProps> = ({
  open,
  onClose,
  app
}) => {
  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
    >
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
        
        <List sx={{ mb: 3 }}>
          {setupSteps.map((step, index) => (
            <React.Fragment key={step.step}>
              <ListItem sx={{ px: 0 }}>
                <ListItemIcon sx={{ minWidth: 40 }}>
                  <Box
                    sx={{
                      width: 24,
                      height: 24,
                      borderRadius: '50%',
                      backgroundColor: 'primary.main',
                      color: 'white',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: '0.875rem',
                      fontWeight: 'bold'
                    }}
                  >
                    {step.step}
                  </Box>
                </ListItemIcon>
                <ListItemText
                  primary={
                    step.link ? (
                      <Typography
                        component="a"
                        href={step.link}
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{
                          color: 'primary.main',
                          textDecoration: 'none',
                          '&:hover': {
                            textDecoration: 'underline'
                          }
                        }}
                      >
                        {step.text}
                      </Typography>
                    ) : (
                      <Typography>{step.text}</Typography>
                    )
                  }
                />
              </ListItem>
              {index < setupSteps.length - 1 && <Divider sx={{ ml: 6 }} />}
            </React.Fragment>
          ))}
        </List>

        <Alert severity="info" sx={{ mb: 2 }}>
          <Typography variant="body2">
            <strong>Note:</strong> After completing the setup, you'll need to copy the App Token and Bot User OAuth Token into the fields above.
          </Typography>
        </Alert>

        <Box sx={{ mt: 3, p: 2, borderRadius: 1 }}>
          <Typography variant="subtitle2" gutterBottom>
            App Manifest (copy this when prompted):
          </Typography>
          <Box sx={{ position: 'relative' }}>
            <CopyButton 
              content={getSlackAppManifest(app.name || 'Helix Agent', app.description || 'AI-powered Slack integration')} 
              title="App Manifest"
            />
            <Box
              component="pre"
              sx={{
                backgroundColor: 'rgba(0,0,0,0.3)',
                p: 2,
                borderRadius: 1,
                fontSize: '0.75rem',
                overflow: 'auto',
                maxHeight: 200,
                border: '1px solid rgba(255,255,255,0.1)'
              }}
            >
              {getSlackAppManifest(app.name || 'Helix Agent', app.description || 'AI-powered Slack integration')}
            </Box>
          </Box>
        </Box>
      </DialogContent>
      <DialogActions sx={{ p: 3, pt: 1 }}>
        <Button onClick={onClose} variant="outlined">
          Close
        </Button>
      </DialogActions>
    </DarkDialog>
  )
}

export default TriggerSlackSetup
