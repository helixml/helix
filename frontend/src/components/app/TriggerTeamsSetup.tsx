import React, { FC, useState } from 'react'
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
import IconButton from '@mui/material/IconButton'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import TextField from '@mui/material/TextField'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import InputAdornment from '@mui/material/InputAdornment'
import Alert from '@mui/material/Alert'
import { TeamsLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { IAppFlatState } from '../../types'

interface SetupStep {
  step: number
  text: string
  link?: string
  substeps?: string[]
}

const setupSteps: SetupStep[] = [
  {
    step: 1,
    text: 'Go to the Azure Portal and create a new Bot resource',
    link: 'https://portal.azure.com/#create/Microsoft.AzureBot'
  },
  {
    step: 2,
    text: 'Configure your bot:',
    substeps: [
      'Choose a unique Bot handle (e.g., "helix-agent-bot")',
      'Select your subscription and resource group',
      'Choose "Single Tenant" or "Multi Tenant" for Type of App',
      'Select "Create new Microsoft App ID"',
      'Click "Review + create" then "Create"'
    ]
  },
  {
    step: 3,
    text: 'After creation, go to your Bot resource and click "Configuration"',
    substeps: [
      'Copy the "Microsoft App ID" (this is your App ID)',
      'Click "Manage Password" next to Microsoft App ID',
      'This opens the Azure AD app registration - go to "Certificates & secrets"',
      'Click "New client secret", add a description, and click "Add"',
      'IMPORTANT: Copy the "Value" column immediately (this is your App Password) - it will be hidden after you leave the page',
      'Note: The "Secret ID" is NOT the password - you need the "Value"',
      'To find Tenant ID: In the app registration, go to "Overview" - the "Directory (tenant) ID" is your Tenant ID (REQUIRED for Single Tenant bots)'
    ]
  },
  {
    step: 4,
    text: 'Configure the messaging endpoint:',
    substeps: [
      'Go back to your Azure Bot resource (not the app registration)',
      'Click "Configuration" in the left menu',
      'Find the "Messaging endpoint" field',
      'Paste the webhook URL shown above in this dialog',
      'Click "Apply" to save'
    ]
  },
  {
    step: 5,
    text: 'Enable the Teams channel:',
    substeps: [
      'In your Bot resource, go to "Channels"',
      'Click "Microsoft Teams"',
      'Accept the Terms of Service',
      'Click "Apply"'
    ]
  },
  {
    step: 6,
    text: 'Install the bot in Teams:',
    substeps: [
      'Go to Teams Developer Portal',
      'Create a new app or import your bot',
      'Add the Bot capability with your App ID',
      'Publish to your organization or install directly'
    ],
    link: 'https://dev.teams.microsoft.com/apps'
  },
  {
    step: 7,
    text: 'Enter your credentials above and test by mentioning your bot in Teams'
  }
]

interface TriggerTeamsSetupProps {
  open: boolean
  onClose: () => void
  app: IAppFlatState
  appId: string
  msAppId?: string
  appPassword?: string
  onAppIdChange?: (value: string) => void
  onAppPasswordChange?: (value: string) => void
}

const TriggerTeamsSetup: FC<TriggerTeamsSetupProps> = ({
  open,
  onClose,
  app,
  appId,
  msAppId = '',
  appPassword = '',
  onAppIdChange,
  onAppPasswordChange
}) => {
  const [showAppPassword, setShowAppPassword] = useState<boolean>(false)

  // Construct webhook URL
  const webhookUrl = `${window.location.origin}/api/v1/teams/webhook/${appId}`

  const handleCopyWebhookUrl = async () => {
    try {
      await navigator.clipboard.writeText(webhookUrl)
    } catch (err) {
      console.error('Failed to copy webhook URL:', err)
    }
  }

  const handleAppIdChange = (value: string) => {
    onAppIdChange?.(value)
  }

  const handleAppPasswordChange = (value: string) => {
    onAppPasswordChange?.(value)
  }

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle sx={{ pb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <TeamsLogo sx={{ fontSize: 24, color: 'primary.main' }} />
          <Typography variant="h6">Microsoft Teams Bot Setup Instructions</Typography>
        </Box>
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Follow these steps to set up your Microsoft Teams bot and connect it to Helix:
        </Typography>

        {/* Webhook URL Box */}
        <Alert
          severity="info"
          sx={{ mb: 3 }}
          action={
            <Button
              size="small"
              startIcon={<ContentCopyIcon />}
              onClick={handleCopyWebhookUrl}
            >
              Copy
            </Button>
          }
        >
          <Typography variant="body2" sx={{ fontWeight: 'bold', mb: 0.5 }}>
            Your Webhook URL (Messaging Endpoint):
          </Typography>
          <Typography
            variant="body2"
            sx={{
              fontFamily: 'monospace',
              wordBreak: 'break-all'
            }}
          >
            {webhookUrl}
          </Typography>
        </Alert>

        {/* Credentials Input */}
        <Box sx={{ mb: 3, p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
          <Typography variant="subtitle2" sx={{ mb: 2 }}>
            Enter your credentials:
          </Typography>
          <Box sx={{ mb: 2 }}>
            <TextField
              fullWidth
              size="small"
              label="Microsoft App ID"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={msAppId}
              onChange={(e) => handleAppIdChange(e.target.value)}
              helperText="The App ID from your Azure Bot resource"
            />
          </Box>
          <Box>
            <TextField
              fullWidth
              size="small"
              label="App Password"
              placeholder="Your client secret"
              value={appPassword}
              onChange={(e) => handleAppPasswordChange(e.target.value)}
              helperText="The client secret Value (not Secret ID) from Azure AD app registration"
              type={showAppPassword ? 'text' : 'password'}
              autoComplete="new-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle app password visibility"
                      onClick={() => setShowAppPassword(!showAppPassword)}
                      edge="end"
                    >
                      {showAppPassword ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
          </Box>
        </Box>

        <List sx={{ mb: 3 }}>
          {setupSteps.map((step, index) => (
            <React.Fragment key={step.step}>
              <ListItem sx={{ px: 0, flexDirection: 'column', alignItems: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
                  <ListItemIcon sx={{ minWidth: 40, mt: 0 }}>
                    <Box
                      sx={{
                        width: 24,
                        height: 24,
                        borderRadius: '50%',
                        mt: 0.7,
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
                </Box>

                {/* Substeps */}
                {step.substeps && (
                  <Box sx={{ ml: 6, mt: 1, width: 'calc(100% - 48px)' }}>
                    <List dense disablePadding>
                      {step.substeps.map((substep, subIndex) => (
                        <ListItem key={subIndex} sx={{ py: 0.25, px: 0 }}>
                          <ListItemText
                            primary={
                              <Typography variant="body2" color="text.secondary">
                                {'\u2022'} {substep}
                              </Typography>
                            }
                          />
                        </ListItem>
                      ))}
                    </List>
                  </Box>
                )}
              </ListItem>
              {index < setupSteps.length - 1 && <Divider sx={{ ml: 6 }} />}
            </React.Fragment>
          ))}
        </List>

        <Alert severity="warning" sx={{ mt: 2 }}>
          <Typography variant="body2">
            <strong>Important:</strong> The messaging endpoint must be publicly accessible via HTTPS.
            For local development, use a tunnel service like ngrok.
          </Typography>
        </Alert>
      </DialogContent>
      <DialogActions sx={{ p: 3, pt: 1 }}>
        <Button onClick={onClose} variant="outlined">
          Close
        </Button>
      </DialogActions>
    </DarkDialog>
  )
}

export default TriggerTeamsSetup
