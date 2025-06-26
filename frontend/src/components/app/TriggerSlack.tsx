import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import { TypesTrigger } from '../../api/api'
import { SlackLogo } from '../icons/ProviderIcons'

interface TriggerSlackProps {
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerSlack: FC<TriggerSlackProps> = ({
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasSlackTrigger = triggers.some(t => t.slack && t.slack.enabled === true)
  const slackTrigger = triggers.find(t => t.slack)?.slack

  // State for Slack configuration
  const [appToken, setAppToken] = useState<string>(slackTrigger?.app_token || '')
  const [botToken, setBotToken] = useState<string>(slackTrigger?.bot_token || '')

  // Update state when triggers change
  useEffect(() => {
    if (slackTrigger) {
      setAppToken(slackTrigger.app_token || '')
      setBotToken(slackTrigger.bot_token || '')
    }
  }, [slackTrigger])

  const handleSlackToggle = (enabled: boolean) => {
    if (enabled) {
      // Enable the existing Slack trigger or create a default one if none exists
      const currentSlackTrigger = triggers.find(t => t.slack)?.slack
      if (currentSlackTrigger) {
        // Preserve existing configuration but set enabled to true
        const newTriggers = [...triggers.filter(t => !t.slack), { 
          slack: { 
            enabled: true, 
            app_token: currentSlackTrigger.app_token || '', 
            bot_token: currentSlackTrigger.bot_token || '',
            channels: currentSlackTrigger.channels || []
          } 
        }]
        onUpdate(newTriggers)
      } else {
        // Create a default Slack trigger
        const newTriggers = [...triggers.filter(t => !t.slack), { 
          slack: { 
            enabled: true, 
            app_token: '', 
            bot_token: '',
            channels: []
          } 
        }]
        onUpdate(newTriggers)
      }
    } else {
      // Keep the Slack trigger but set enabled to false, preserving configuration
      const currentSlackTrigger = triggers.find(t => t.slack)?.slack
      if (currentSlackTrigger) {
        const updatedTriggers = [...triggers.filter(t => !t.slack), { 
          slack: { 
            enabled: false, 
            app_token: currentSlackTrigger.app_token || '', 
            bot_token: currentSlackTrigger.bot_token || '',
            channels: currentSlackTrigger.channels || []
          } 
        }]
        onUpdate(updatedTriggers)
      } else {
        // Fallback: remove Slack trigger if none exists
        const removedTriggers = triggers.filter(t => !t.slack)
        onUpdate(removedTriggers)
      }
    }
  }

  const handleAppTokenChange = (token: string) => {
    setAppToken(token)
    updateSlackTrigger(token, botToken)
  }

  const handleBotTokenChange = (token: string) => {
    setBotToken(token)
    updateSlackTrigger(appToken, token)
  }

  const updateSlackTrigger = (appTokenValue: string, botTokenValue: string) => {
    const currentSlackTrigger = triggers.find(t => t.slack)?.slack
    const newTriggers = [...triggers.filter(t => !t.slack), { 
      slack: { 
        enabled: true, 
        app_token: appTokenValue, 
        bot_token: botTokenValue,
        channels: currentSlackTrigger?.channels || []
      } 
    }]
    onUpdate(newTriggers)
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <SlackLogo sx={{ mr: 2, fontSize: 24, color: 'primary.main' }} />
          <Box>
            <Typography gutterBottom>Slack</Typography>
            <Typography variant="body2" color="text.secondary">
              Connect your agent to Slack for notifications and commands
            </Typography>
          </Box>
        </Box>
        <FormControlLabel
          control={
            <Switch
              checked={hasSlackTrigger}
              onChange={(e) => handleSlackToggle(e.target.checked)}
              disabled={readOnly}
            />
          }
          label=""
        />
      </Box>

      {(hasSlackTrigger || slackTrigger) && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1, opacity: hasSlackTrigger ? 1 : 0.6 }}>
          {!hasSlackTrigger && slackTrigger && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Trigger is disabled. Enable it above to activate Slack integration.
            </Alert>
          )}
          
          {/* App Token */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              App Token
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="xapp-..."
              value={appToken}
              onChange={(e) => handleAppTokenChange(e.target.value)}
              disabled={readOnly || !hasSlackTrigger}
              helperText="Your Slack app token (starts with xapp-)"
            />
          </Box>

          {/* Bot Token */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Bot Token
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="xoxb-..."
              value={botToken}
              onChange={(e) => handleBotTokenChange(e.target.value)}
              disabled={readOnly || !hasSlackTrigger}
              helperText="Your Slack bot token (starts with xoxb-)"
            />
          </Box>

          {/* Configuration summary */}
          <Box>
            <Typography variant="body2" color="text.secondary">
              <strong>Summary:</strong> {hasSlackTrigger 
                ? `Slack integration ${appToken && botToken ? 'configured' : 'needs tokens'}`
                : 'Slack integration disabled'
              }
            </Typography>
          </Box>
        </Box>
      )}
    </Box>
  )
}

export default TriggerSlack
