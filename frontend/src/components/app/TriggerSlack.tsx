import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import Circle from '@mui/icons-material/Circle'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import Button from '@mui/material/Button'
import { TypesTrigger } from '../../api/api'
import { SlackLogo } from '../icons/ProviderIcons'

import { useGetAppTriggerStatus } from '../../services/appService'
import { IAppFlatState } from '../../types'
import TriggerSlackSetup from './TriggerSlackSetup'

interface TriggerSlackProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerSlack: FC<TriggerSlackProps> = ({
  app,
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasSlackTrigger = triggers.some(t => t.slack && t.slack.enabled === true)
  const slackTrigger = triggers.find(t => t.slack)?.slack

  // State for Slack configuration
  const [appToken, setAppToken] = useState<string>(slackTrigger?.app_token || '')
  const [botToken, setBotToken] = useState<string>(slackTrigger?.bot_token || '')
  const [showAppToken, setShowAppToken] = useState<boolean>(false)
  const [showBotToken, setShowBotToken] = useState<boolean>(false)
  const [showSetupDialog, setShowSetupDialog] = useState<boolean>(false)

  // If slack is configured, we need to get the status of the bot
  const { data: slackStatus, isLoading: isLoadingSlackStatus } = useGetAppTriggerStatus(appId, 'slack', {
    enabled: hasSlackTrigger,
    refetchInterval: 1500
  })

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

      {(hasSlackTrigger) && (
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
              type={showAppToken ? 'text' : 'password'}
              autoComplete="new-bot-app-token-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle app token visibility"
                      onClick={() => setShowAppToken(!showAppToken)}
                      edge="end"
                      disabled={readOnly || !hasSlackTrigger}
                    >
                      {showAppToken ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
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
              type={showBotToken ? 'text' : 'password'}
              autoComplete="new-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle bot token visibility"
                      onClick={() => setShowBotToken(!showBotToken)}
                      edge="end"
                      disabled={readOnly || !hasSlackTrigger}
                    >
                      {showBotToken ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
          </Box>

          {/* Configuration summary */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {(() => {
                // If Slack trigger is disabled
                if (!slackTrigger?.enabled) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Slack integration disabled
                      </Typography>
                    </>
                  )
                }

                // If no tokens are configured, show grey circle with existing message
                if (slackTrigger?.enabled && (!appToken || !botToken)) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Slack integration {appToken && botToken ? 'configured' : 'needs tokens'}
                      </Typography>
                    </>
                  )
                }
                
                // If we have tokens but no trigger status yet, show grey circle
                if (!slackStatus?.data && !isLoadingSlackStatus) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Slack integration configured
                      </Typography>
                    </>
                  )
                }
                
                // If trigger status is OK, show green circle with status message
                if (slackStatus?.data?.ok === true) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'success.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {slackStatus.data.message || 'Slack integration active'}
                      </Typography>
                    </>
                  )
                }
                
                // If trigger status is not OK, show red circle with error message
                if (slackStatus?.data?.ok === false) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'error.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {slackStatus.data.message || 'Slack integration error'}
                      </Typography>
                    </>
                  )
                }
                
                // Loading state
                return (
                  <>
                    <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                    <Typography variant="body2" color="text.secondary">
                      <strong>Status:</strong> Checking Slack integration status...
                    </Typography>
                  </>
                )
              })()}
            </Box>
            <Button
              variant="text"
              size="small"
              onClick={() => setShowSetupDialog(true)}
              disabled={readOnly}
            >
              View setup instructions
            </Button>
          </Box>
        </Box>
      )}

      {/* Setup Instructions Dialog */}
      <TriggerSlackSetup
        open={showSetupDialog}
        onClose={() => setShowSetupDialog(false)}
        app={app}
        appToken={appToken}
        botToken={botToken}
        onAppTokenChange={handleAppTokenChange}
        onBotTokenChange={handleBotTokenChange}
      />
    </Box>
  )
}

export default TriggerSlack
