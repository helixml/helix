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
import { CrispLogo } from '../icons/ProviderIcons'
import { IAppFlatState } from '../../types'
import { useGetAppTriggerStatus } from '../../services/appService'

interface TriggerCrispProps {
  app: IAppFlatState
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerCrisp: FC<TriggerCrispProps> = ({
  app,
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasCrispTrigger = triggers.some(t => t.crisp && t.crisp.enabled === true)
  const crispTrigger = triggers.find(t => t.crisp)?.crisp

  // State for Crisp configuration
  const [identifier, setIdentifier] = useState<string>(crispTrigger?.identifier || '')
  const [token, setToken] = useState<string>(crispTrigger?.token || '')
  const [showToken, setShowToken] = useState<boolean>(false)

  // If crisp is configured, we need to get the status of the bot
  const { data: crispStatus, isLoading: isLoadingCrispStatus } = useGetAppTriggerStatus(appId, 'crisp', {
    enabled: hasCrispTrigger,
    refetchInterval: 1500
  })

  // Update state when triggers change
  useEffect(() => {
    if (crispTrigger) {
      setIdentifier(crispTrigger.identifier || '')
      setToken(crispTrigger.token || '')
    }
  }, [crispTrigger])

  const handleCrispToggle = (enabled: boolean) => {
    if (enabled) {
      // Enable the existing Crisp trigger or create a default one if none exists
      const currentCrispTrigger = triggers.find(t => t.crisp)?.crisp
      if (currentCrispTrigger) {
        // Preserve existing configuration but set enabled to true
        const newTriggers = [...triggers.filter(t => !t.crisp), { 
          crisp: { 
            enabled: true, 
            identifier: currentCrispTrigger.identifier || '', 
            token: currentCrispTrigger.token || ''
          } 
        }]
        onUpdate(newTriggers)
      } else {
        // Create a default Crisp trigger
        const newTriggers = [...triggers.filter(t => !t.crisp), { 
          crisp: { 
            enabled: true, 
            identifier: '', 
            token: ''
          } 
        }]
        onUpdate(newTriggers)
      }
    } else {
      // Keep the Crisp trigger but set enabled to false, preserving configuration
      const currentCrispTrigger = triggers.find(t => t.crisp)?.crisp
      if (currentCrispTrigger) {
        const updatedTriggers = [...triggers.filter(t => !t.crisp), { 
          crisp: { 
            enabled: false, 
            identifier: currentCrispTrigger.identifier || '', 
            token: currentCrispTrigger.token || ''
          } 
        }]
        onUpdate(updatedTriggers)
      } else {
        // Fallback: remove Crisp trigger if none exists
        const removedTriggers = triggers.filter(t => !t.crisp)
        onUpdate(removedTriggers)
      }
    }
  }

  const handleIdentifierChange = (value: string) => {
    setIdentifier(value)
    updateCrispTrigger(value, token)
  }

  const handleTokenChange = (value: string) => {
    setToken(value)
    updateCrispTrigger(identifier, value)
  }

  const updateCrispTrigger = (identifierValue: string, tokenValue: string) => {
    const currentCrispTrigger = triggers.find(t => t.crisp)?.crisp
    const newTriggers = [...triggers.filter(t => !t.crisp), { 
      crisp: { 
        enabled: true, 
        identifier: identifierValue, 
        token: tokenValue
      } 
    }]
    onUpdate(newTriggers)
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <CrispLogo sx={{ mr: 2, fontSize: 24, color: 'primary.main' }} />
          <Box>
            <Typography gutterBottom>Crisp</Typography>
            <Typography variant="body2" color="text.secondary">
              Connect your agent to Crisp for live chat support and customer service
            </Typography>
          </Box>
        </Box>
        <FormControlLabel
          control={
            <Switch
              checked={hasCrispTrigger}
              onChange={(e) => handleCrispToggle(e.target.checked)}
              disabled={readOnly}
            />
          }
          label=""
        />
      </Box>

      {(hasCrispTrigger) && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1, opacity: hasCrispTrigger ? 1 : 0.6 }}>
          {!hasCrispTrigger && crispTrigger && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Trigger is disabled. Enable it above to activate Crisp integration.
            </Alert>
          )}
          
          {/* Identifier */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Website Identifier
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="your-website-identifier"
              value={identifier}
              onChange={(e) => handleIdentifierChange(e.target.value)}
              disabled={readOnly || !hasCrispTrigger}
              helperText="Your Crisp website identifier"
            />
          </Box>

          {/* Token */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              API Token
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="your-api-token"
              value={token}
              onChange={(e) => handleTokenChange(e.target.value)}
              disabled={readOnly || !hasCrispTrigger}
              helperText="Your Crisp API token"
              type={showToken ? 'text' : 'password'}
              autoComplete="new-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle token visibility"
                      onClick={() => setShowToken(!showToken)}
                      edge="end"
                      disabled={readOnly || !hasCrispTrigger}
                    >
                      {showToken ? <VisibilityOff /> : <Visibility />}
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
                // If Crisp trigger is disabled
                if (!crispTrigger?.enabled) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Crisp integration disabled
                      </Typography>
                    </>
                  )
                }

                // If no identifier or token are configured, show grey circle
                if (crispTrigger?.enabled && (!identifier || !token)) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Crisp integration {identifier && token ? 'configured' : 'needs configuration'}
                      </Typography>
                    </>
                  )
                }
                
                // If we have configuration but no trigger status yet, show grey circle
                if (!crispStatus?.data && !isLoadingCrispStatus) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Crisp integration configured
                      </Typography>
                    </>
                  )
                }
                
                // If trigger status is OK, show green circle with status message
                if (crispStatus?.data?.ok === true) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'success.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {crispStatus.data.message || 'Crisp integration active'}
                      </Typography>
                    </>
                  )
                }
                
                // If trigger status is not OK, show red circle with error message
                if (crispStatus?.data?.ok === false) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'error.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {crispStatus.data.message || 'Crisp integration error'}
                      </Typography>
                    </>
                  )
                }
                
                // Loading state
                return (
                  <>
                    <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                    <Typography variant="body2" color="text.secondary">
                      <strong>Status:</strong> Checking Crisp integration status...
                    </Typography>
                  </>
                )
              })()}
            </Box>
            <Button
              variant="text"
              size="small"
              onClick={() => {
                // Open Crisp documentation or setup instructions
                window.open('https://docs.crisp.chat/', '_blank')
              }}
              disabled={readOnly}
            >
              View setup instructions
            </Button>
          </Box>
        </Box>
      )}
    </Box>
  )
}

export default TriggerCrisp
