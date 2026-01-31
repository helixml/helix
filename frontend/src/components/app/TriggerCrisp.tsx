import React, { FC, useState, useEffect, useCallback, useRef } from 'react'
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
import TriggerCrispSetup from './TriggerCrispSetup'

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
  const [nickname, setNickname] = useState<string>(crispTrigger?.nickname || '')
  const [showToken, setShowToken] = useState<boolean>(false)
  const [setupDialogOpen, setSetupDialogOpen] = useState<boolean>(false)
  const debounceTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  // If crisp is configured, we need to get the status of the bot
  const { data: crispStatus, isLoading: isLoadingCrispStatus } = useGetAppTriggerStatus(appId, 'crisp', {
    enabled: hasCrispTrigger,
    refetchInterval: 1500
  })

  // Debounced update function
  const debouncedUpdate = useCallback((triggers: TypesTrigger[]) => {
    if (debounceTimeoutRef.current) {
      clearTimeout(debounceTimeoutRef.current)
    }
    debounceTimeoutRef.current = setTimeout(() => {
      onUpdate(triggers)
    }, 500)
  }, [onUpdate])

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current)
      }
    }
  }, [])

  // Update state when triggers change
  useEffect(() => {
    if (crispTrigger) {
      setIdentifier(crispTrigger.identifier || '')
      setToken(crispTrigger.token || '')
      setNickname(crispTrigger.nickname || '')
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
            token: currentCrispTrigger.token || '',
            nickname: currentCrispTrigger.nickname || ''
          } 
        }]
        debouncedUpdate(newTriggers)
      } else {
        // Create a default Crisp trigger
        const newTriggers = [...triggers.filter(t => !t.crisp), { 
          crisp: { 
            enabled: true, 
            identifier: '', 
            token: '',
            nickname: ''
          } 
        }]
        debouncedUpdate(newTriggers)
      }
    } else {
      // Keep the Crisp trigger but set enabled to false, preserving configuration
      const currentCrispTrigger = triggers.find(t => t.crisp)?.crisp
      if (currentCrispTrigger) {
        const updatedTriggers = [...triggers.filter(t => !t.crisp), { 
          crisp: { 
            enabled: false, 
            identifier: currentCrispTrigger.identifier || '', 
            token: currentCrispTrigger.token || '',
            nickname: currentCrispTrigger.nickname || ''
          } 
        }]
        debouncedUpdate(updatedTriggers)
      } else {
        // Fallback: remove Crisp trigger if none exists
        const removedTriggers = triggers.filter(t => !t.crisp)
        debouncedUpdate(removedTriggers)
      }
    }
  }

  const handleIdentifierChange = (value: string) => {
    setIdentifier(value)
    updateCrispTrigger(value, token, nickname)
  }

  const handleTokenChange = (value: string) => {
    setToken(value)
    updateCrispTrigger(identifier, value, nickname)
  }

  const handleNicknameChange = (value: string) => {
    setNickname(value)
    updateCrispTrigger(identifier, token, value)
  }

  const updateCrispTrigger = (identifierValue: string, tokenValue: string, nicknameValue: string) => {
    const currentCrispTrigger = triggers.find(t => t.crisp)?.crisp
    const newTriggers = [...triggers.filter(t => !t.crisp), { 
      crisp: { 
        enabled: true, 
        identifier: identifierValue, 
        token: tokenValue,
        nickname: nicknameValue
      } 
    }]
    debouncedUpdate(newTriggers)
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
              Identifier
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="your-token-identifier"
              value={identifier}
              onChange={(e) => handleIdentifierChange(e.target.value)}
              disabled={readOnly || !hasCrispTrigger}
              helperText="Your Crisp token identifier"
            />
          </Box>

          {/* Token */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Key
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="your-key"
              value={token}
              onChange={(e) => handleTokenChange(e.target.value)}
              disabled={readOnly || !hasCrispTrigger}
              helperText="Your Crisp key"
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

          {/* Nickname */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Bot Nickname (Optional)
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="Helix"
              value={nickname}
              onChange={(e) => handleNicknameChange(e.target.value)}
              disabled={readOnly || !hasCrispTrigger}
              helperText="The nickname that will appear in Crisp chat (defaults to 'Helix' if empty). As an operator you can also trigger the bot by typing 'Hey <bot_nickname>'"
            />
          </Box>

          {/* Example commands */}
          <Box sx={{ mb: 3, p: 2}}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Example Commands:</strong>
            </Typography>
            <Box component="ul" sx={{ m: 0, pl: 2, '& li': { mb: 0.5 } }}>
              <Typography component="li" variant="body2" color="text.secondary">
                To trigger the bot as an operator (normally bot will ignore operator messages) say <strong>"Hey {nickname || 'Helix'}"</strong>
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                To prevent bot from replying to user messages for current day say <strong>"{nickname || 'Helix'} stop"</strong>
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                To re-enable bot to handle messages from the user say <strong>"{nickname || 'Helix'} continue"</strong>
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                Normally bot will not reply to the user message after the human operator said something. This is to prevent scenarios where human operator and customer are talking and the bot would be also trying to get into the conversation.
              </Typography>
            </Box>
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
              onClick={() => setSetupDialogOpen(true)}
              disabled={readOnly}
            >
              View setup instructions
            </Button>
          </Box>
        </Box>
      )}

      <TriggerCrispSetup
        open={setupDialogOpen}
        onClose={() => setSetupDialogOpen(false)}
        app={app}
        identifier={identifier}
        token={token}
        onIdentifierChange={handleIdentifierChange}
        onTokenChange={handleTokenChange}
      />
    </Box>
  )
}

export default TriggerCrisp
