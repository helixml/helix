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
import { TeamsLogo } from '../icons/ProviderIcons'

import { useGetAppTriggerStatus } from '../../services/appService'
import { IAppFlatState } from '../../types'
import TriggerTeamsSetup from './TriggerTeamsSetup'

interface TriggerTeamsProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerTeams: FC<TriggerTeamsProps> = ({
  app,
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasTeamsTrigger = triggers.some(t => t.teams && t.teams.enabled === true)
  const teamsTrigger = triggers.find(t => t.teams)?.teams

  // State for Teams configuration
  const [msAppId, setMsAppId] = useState<string>(teamsTrigger?.app_id || '')
  const [appPassword, setAppPassword] = useState<string>(teamsTrigger?.app_password || '')
  const [tenantId, setTenantId] = useState<string>(teamsTrigger?.tenant_id || '')
  const [showAppPassword, setShowAppPassword] = useState<boolean>(false)
  const [showSetupDialog, setShowSetupDialog] = useState<boolean>(false)

  // If teams is configured, we need to get the status of the bot
  const { data: teamsStatus, isLoading: isLoadingTeamsStatus } = useGetAppTriggerStatus(appId, 'teams', {
    enabled: hasTeamsTrigger,
    refetchInterval: 1500
  })

  // Update state when triggers change
  useEffect(() => {
    if (teamsTrigger) {
      setMsAppId(teamsTrigger.app_id || '')
      setAppPassword(teamsTrigger.app_password || '')
      setTenantId(teamsTrigger.tenant_id || '')
    }
  }, [teamsTrigger])

  const handleTeamsToggle = (enabled: boolean) => {
    if (enabled) {
      // Enable the existing Teams trigger or create a default one if none exists
      const currentTeamsTrigger = triggers.find(t => t.teams)?.teams
      if (currentTeamsTrigger) {
        // Preserve existing configuration but set enabled to true
        const newTriggers = [...triggers.filter(t => !t.teams), {
          teams: {
            enabled: true,
            app_id: currentTeamsTrigger.app_id || '',
            app_password: currentTeamsTrigger.app_password || '',
            tenant_id: currentTeamsTrigger.tenant_id || ''
          }
        }]
        onUpdate(newTriggers)
      } else {
        // Create a default Teams trigger
        const newTriggers = [...triggers.filter(t => !t.teams), {
          teams: {
            enabled: true,
            app_id: '',
            app_password: '',
            tenant_id: ''
          }
        }]
        onUpdate(newTriggers)
      }
    } else {
      // Keep the Teams trigger but set enabled to false, preserving configuration
      const currentTeamsTrigger = triggers.find(t => t.teams)?.teams
      if (currentTeamsTrigger) {
        const updatedTriggers = [...triggers.filter(t => !t.teams), {
          teams: {
            enabled: false,
            app_id: currentTeamsTrigger.app_id || '',
            app_password: currentTeamsTrigger.app_password || '',
            tenant_id: currentTeamsTrigger.tenant_id || ''
          }
        }]
        onUpdate(updatedTriggers)
      } else {
        // Fallback: remove Teams trigger if none exists
        const removedTriggers = triggers.filter(t => !t.teams)
        onUpdate(removedTriggers)
      }
    }
  }

  const handleAppIdChange = (value: string) => {
    setMsAppId(value)
    updateTeamsTrigger(value, appPassword, tenantId)
  }

  const handleAppPasswordChange = (value: string) => {
    setAppPassword(value)
    updateTeamsTrigger(msAppId, value, tenantId)
  }

  const handleTenantIdChange = (value: string) => {
    setTenantId(value)
    updateTeamsTrigger(msAppId, appPassword, value)
  }

  const updateTeamsTrigger = (appIdValue: string, appPasswordValue: string, tenantIdValue: string) => {
    const newTriggers = [...triggers.filter(t => !t.teams), {
      teams: {
        enabled: true,
        app_id: appIdValue,
        app_password: appPasswordValue,
        tenant_id: tenantIdValue
      }
    }]
    onUpdate(newTriggers)
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <TeamsLogo sx={{ mr: 2, fontSize: 24, color: 'primary.main' }} />
          <Box>
            <Typography gutterBottom>Microsoft Teams</Typography>
            <Typography variant="body2" color="text.secondary">
              Connect your agent to Microsoft Teams for notifications and commands
            </Typography>
          </Box>
        </Box>
        <FormControlLabel
          control={
            <Switch
              checked={hasTeamsTrigger}
              onChange={(e) => handleTeamsToggle(e.target.checked)}
              disabled={readOnly}
            />
          }
          label=""
        />
      </Box>

      {(hasTeamsTrigger) && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1, opacity: hasTeamsTrigger ? 1 : 0.6 }}>
          {!hasTeamsTrigger && teamsTrigger && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Trigger is disabled. Enable it above to activate Teams integration.
            </Alert>
          )}

          {/* App ID */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Microsoft App ID
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={msAppId}
              onChange={(e) => handleAppIdChange(e.target.value)}
              disabled={readOnly || !hasTeamsTrigger}
              helperText="Your Microsoft Bot Framework App ID (GUID format)"
            />
          </Box>

          {/* App Password */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              App Password
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="Your app password"
              value={appPassword}
              onChange={(e) => handleAppPasswordChange(e.target.value)}
              disabled={readOnly || !hasTeamsTrigger}
              helperText="Client secret Value (not Secret ID) from Azure AD app registration"
              type={showAppPassword ? 'text' : 'password'}
              autoComplete="new-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle app password visibility"
                      onClick={() => setShowAppPassword(!showAppPassword)}
                      edge="end"
                      disabled={readOnly || !hasTeamsTrigger}
                    >
                      {showAppPassword ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
          </Box>

          {/* Tenant ID */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Tenant ID
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={tenantId}
              onChange={(e) => handleTenantIdChange(e.target.value)}
              disabled={readOnly || !hasTeamsTrigger}
              helperText="Required for Single Tenant bots. Leave empty for Multi Tenant bots."
            />
          </Box>

          {/* Configuration summary */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {(() => {
                // If Teams trigger is disabled
                if (!teamsTrigger?.enabled) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Teams integration disabled
                      </Typography>
                    </>
                  )
                }

                // If no credentials are configured, show grey circle with existing message
                if (teamsTrigger?.enabled && (!msAppId || !appPassword)) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Teams integration {msAppId && appPassword ? 'configured' : 'needs credentials'}
                      </Typography>
                    </>
                  )
                }

                // If we have credentials but no trigger status yet, show grey circle
                if (!teamsStatus?.data && !isLoadingTeamsStatus) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Teams integration configured
                      </Typography>
                    </>
                  )
                }

                // If trigger status is OK, show green circle with status message
                if (teamsStatus?.data?.ok === true) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'success.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {teamsStatus.data.message || 'Teams integration active'}
                      </Typography>
                    </>
                  )
                }

                // If trigger status is not OK, show red circle with error message
                if (teamsStatus?.data?.ok === false) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'error.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {teamsStatus.data.message || 'Teams integration error'}
                      </Typography>
                    </>
                  )
                }

                // Loading state
                return (
                  <>
                    <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                    <Typography variant="body2" color="text.secondary">
                      <strong>Status:</strong> Checking Teams integration status...
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
      <TriggerTeamsSetup
        open={showSetupDialog}
        onClose={() => setShowSetupDialog(false)}
        app={app}
        appId={appId}
        msAppId={msAppId}
        appPassword={appPassword}
        onAppIdChange={handleAppIdChange}
        onAppPasswordChange={handleAppPasswordChange}
      />
    </Box>
  )
}

export default TriggerTeams
