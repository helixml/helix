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
    text: 'Create an Entra ID App Registration',
    link: 'https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    substeps: [
      'Click "New registration" and provide an application name (e.g., "helix-teams-bot")',
      'Select "Accounts in this organizational directory only (Single tenant)" for most cases',
      'Click "Register"',
      'Note the "Application (client) ID" - this is your App ID',
      'Note the "Directory (tenant) ID" - this is your Tenant ID (required for Single Tenant bots)'
    ]
  },
  {
    step: 2,
    text: 'Create a client secret',
    link: 'https://learn.microsoft.com/en-us/microsoftteams/platform/teams-ai-library/teams/app-authentication/client-secret',
    substeps: [
      'In your app registration, go to "Certificates & secrets"',
      'Click "New client secret", add a description, select an expiration period, and click "Add"',
      'IMPORTANT: Copy the "Value" column immediately - this is your App Password',
      'The value won\'t be shown again after you leave the page. The "Secret ID" is NOT the password.'
    ]
  },
  {
    step: 3,
    text: 'Create an Azure Bot resource',
    link: 'https://portal.azure.com/#create/Microsoft.AzureBot',
    substeps: [
      'Provide a Bot handle (e.g., "helix-agent-bot")',
      'Select your subscription, resource group, and pricing tier',
      'Under "Microsoft App ID", choose "Single Tenant" (or Multi Tenant if needed)',
      'Select "Use existing app registration"',
      'Enter the Application (client) ID from step 1',
      'Click "Review + create" then "Create"'
    ]
  },
  {
    step: 4,
    text: 'Configure the messaging endpoint:',
    substeps: [
      'Go to your Azure Bot resource → Settings → Configuration',
      'In the "Messaging endpoint" field, paste the webhook URL shown above',
      'Click "Apply" to save'
    ]
  },
  {
    step: 5,
    text: 'Enable the Teams channel:',
    substeps: [
      'In your Azure Bot resource, go to Settings → Channels',
      'Click "Microsoft Teams"',
      'Accept the Terms of Service and click "Apply"'
    ]
  },
  {
    step: 6,
    text: 'Install the bot in Teams',
    link: 'https://dev.teams.microsoft.com/apps',
    substeps: [
      'Go to Teams Developer Portal and create a new app',
      'Under "Configure" → "App features", add "Bot"',
      'Select "Enter a bot ID" and paste your Application (client) ID',
      'Select the scopes where the bot can be used (Personal, Team, Group chat)',
      'Go to "Publish" → "Publish to org" or "Download app package" to install'
    ]
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
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Follow these steps to set up your Microsoft Teams bot and connect it to Helix.
          For detailed documentation, see the{' '}
          <Typography
            component="a"
            href="https://learn.microsoft.com/en-us/microsoftteams/platform/teams-ai-library/teams/configuration/manual-configuration"
            target="_blank"
            rel="noopener noreferrer"
            variant="body2"
            sx={{ color: 'primary.main' }}
          >
            official Microsoft Teams bot configuration guide
          </Typography>.
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

        <Box sx={{ mt: 2 }}>
          <Typography variant="body2" color="text.secondary">
            <strong>Learn more:</strong>{' '}
            <Typography
              component="a"
              href="https://learn.microsoft.com/en-us/microsoftteams/platform/teams-ai-library/teams/overview"
              target="_blank"
              rel="noopener noreferrer"
              variant="body2"
              sx={{ color: 'primary.main' }}
            >
              Microsoft Teams AI Library Overview
            </Typography>
          </Typography>
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

export default TriggerTeamsSetup
