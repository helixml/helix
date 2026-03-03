import React, { FC, useState, useEffect, useCallback, useRef } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'
import TextField from '@mui/material/TextField'
import Circle from '@mui/icons-material/Circle'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import TelegramIcon from '@mui/icons-material/Telegram'
import { TypesTrigger } from '../../api/api'
import { useGetAppTriggerStatus } from '../../services/appService'
import TriggerTelegramSetup from './TriggerTelegramSetup'

interface TriggerTelegramProps {
  appId: string
  triggers?: TypesTrigger[]
  onUpdate: (triggers: TypesTrigger[]) => void
  readOnly?: boolean
}

const TriggerTelegram: FC<TriggerTelegramProps> = ({
  appId,
  triggers = [],
  onUpdate,
  readOnly = false
}) => {
  const hasTelegramTrigger = triggers.some(t => t.telegram && t.telegram.enabled === true)
  const telegramTrigger = triggers.find(t => t.telegram)?.telegram

  const [botToken, setBotToken] = useState<string>(telegramTrigger?.bot_token || '')
  const [useGlobalBot, setUseGlobalBot] = useState<boolean>(!telegramTrigger?.bot_token)
  const [allowedUsers, setAllowedUsers] = useState<string>(
    (telegramTrigger?.allowed_users || []).join(', ')
  )
  const [showToken, setShowToken] = useState<boolean>(false)
  const [setupDialogOpen, setSetupDialogOpen] = useState<boolean>(false)
  const debounceTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  const { data: telegramStatus, isLoading: isLoadingStatus } = useGetAppTriggerStatus(appId, 'telegram', {
    enabled: hasTelegramTrigger,
    refetchInterval: 1500
  })

  const debouncedUpdate = useCallback((triggers: TypesTrigger[]) => {
    if (debounceTimeoutRef.current) {
      clearTimeout(debounceTimeoutRef.current)
    }
    debounceTimeoutRef.current = setTimeout(() => {
      onUpdate(triggers)
    }, 500)
  }, [onUpdate])

  useEffect(() => {
    return () => {
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (telegramTrigger) {
      setBotToken(telegramTrigger.bot_token || '')
      setUseGlobalBot(!telegramTrigger.bot_token)
      setAllowedUsers((telegramTrigger.allowed_users || []).join(', '))
    }
  }, [telegramTrigger])

  const parseAllowedUsers = (value: string): number[] => {
    if (!value.trim()) return []
    return value
      .split(',')
      .map(s => parseInt(s.trim(), 10))
      .filter(n => !isNaN(n) && n > 0)
  }

  const buildTriggers = (overrides: {
    enabled?: boolean
    bot_token?: string
    allowed_users?: number[]
    useGlobal?: boolean
  }): TypesTrigger[] => {
    const enabled = overrides.enabled ?? hasTelegramTrigger
    const token = overrides.useGlobal ? '' : (overrides.bot_token ?? botToken)
    const users = overrides.allowed_users ?? parseAllowedUsers(allowedUsers)

    return [...triggers.filter(t => !t.telegram), {
      telegram: {
        enabled,
        bot_token: token,
        allowed_users: users,
      }
    }]
  }

  const handleToggle = (enabled: boolean) => {
    if (enabled) {
      debouncedUpdate(buildTriggers({ enabled: true }))
    } else {
      const current = triggers.find(t => t.telegram)?.telegram
      if (current) {
        debouncedUpdate(buildTriggers({ enabled: false }))
      } else {
        debouncedUpdate(triggers.filter(t => !t.telegram))
      }
    }
  }

  const handleUseGlobalToggle = (checked: boolean) => {
    setUseGlobalBot(checked)
    if (checked) {
      setBotToken('')
    }
    debouncedUpdate(buildTriggers({ useGlobal: checked, bot_token: checked ? '' : botToken }))
  }

  const handleBotTokenChange = (value: string) => {
    setBotToken(value)
    debouncedUpdate(buildTriggers({ bot_token: value, useGlobal: false }))
  }

  const handleAllowedUsersChange = (value: string) => {
    setAllowedUsers(value)
    debouncedUpdate(buildTriggers({ allowed_users: parseAllowedUsers(value) }))
  }

  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <TelegramIcon sx={{ mr: 2, fontSize: 24, color: '#26A5E4' }} />
          <Box>
            <Typography gutterBottom>Telegram</Typography>
            <Typography variant="body2" color="text.secondary">
              Connect your agent to a Telegram bot to chat with users
            </Typography>
          </Box>
        </Box>
        <FormControlLabel
          control={
            <Switch
              checked={hasTelegramTrigger}
              onChange={(e) => handleToggle(e.target.checked)}
              disabled={readOnly}
            />
          }
          label=""
        />
      </Box>

      {hasTelegramTrigger && (
        <Box sx={{ mt: 2, p: 2, borderRadius: 1 }}>
          {/* Global Bot Toggle */}
          <Box sx={{ mb: 2 }}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={useGlobalBot}
                  onChange={(e) => handleUseGlobalToggle(e.target.checked)}
                  disabled={readOnly}
                  size="small"
                />
              }
              label={
                <Typography variant="body2">
                  Use global bot (configured by admin via TELEGRAM_BOT_TOKEN)
                </Typography>
              }
            />
          </Box>

          {/* Bot Token (only when not using global) */}
          {!useGlobalBot && (
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
                Bot Token
              </Typography>
              <TextField
                fullWidth
                size="small"
                placeholder="123456789:ABCdefGhIjKlMnOpQrStUvWxYz"
                value={botToken}
                onChange={(e) => handleBotTokenChange(e.target.value)}
                disabled={readOnly || !hasTelegramTrigger}
                helperText="The token you received from @BotFather"
                type={showToken ? 'text' : 'password'}
                autoComplete="new-password"
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        aria-label="toggle token visibility"
                        onClick={() => setShowToken(!showToken)}
                        edge="end"
                        disabled={readOnly || !hasTelegramTrigger}
                      >
                        {showToken ? <VisibilityOff /> : <Visibility />}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>
          )}

          {/* Allowed Users */}
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 2 }}>
              Allowed Users (Telegram User IDs)
            </Typography>
            <TextField
              fullWidth
              size="small"
              placeholder="123456789, 987654321"
              value={allowedUsers}
              onChange={(e) => handleAllowedUsersChange(e.target.value)}
              disabled={readOnly || !hasTelegramTrigger}
              helperText="Comma-separated Telegram user IDs. Leave empty to allow all users. Forward a message to @userinfobot to find your ID."
            />
          </Box>

          {/* Info */}
          <Box sx={{ mb: 3, p: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>How it works:</strong>
            </Typography>
            <Box component="ul" sx={{ m: 0, pl: 2, '& li': { mb: 0.5 } }}>
              <Typography component="li" variant="body2" color="text.secondary">
                In private chats, the bot responds to all messages
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                In group chats, mention the bot with <strong>@botname</strong> or reply to its messages
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                Use <strong>/project</strong> to list and select projects
              </Typography>
              <Typography component="li" variant="body2" color="text.secondary">
                Use <strong>/updates</strong> to toggle spec task notifications
              </Typography>
            </Box>
          </Box>

          {/* Status */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {(() => {
                if (!telegramTrigger?.enabled) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Telegram integration disabled
                      </Typography>
                    </>
                  )
                }

                if (telegramTrigger?.enabled && !botToken && !useGlobalBot) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Needs configuration
                      </Typography>
                    </>
                  )
                }

                if (!telegramStatus?.data && !isLoadingStatus) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> Telegram integration configured
                      </Typography>
                    </>
                  )
                }

                if (telegramStatus?.data?.ok === true) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'success.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {telegramStatus.data.message || 'Telegram integration active'}
                      </Typography>
                    </>
                  )
                }

                if (telegramStatus?.data?.ok === false) {
                  return (
                    <>
                      <Circle sx={{ fontSize: 12, color: 'error.main' }} />
                      <Typography variant="body2" color="text.secondary">
                        <strong>Status:</strong> {telegramStatus.data.message || 'Telegram integration error'}
                      </Typography>
                    </>
                  )
                }

                return (
                  <>
                    <Circle sx={{ fontSize: 12, color: 'grey.400' }} />
                    <Typography variant="body2" color="text.secondary">
                      <strong>Status:</strong> Checking Telegram integration status...
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

      <TriggerTelegramSetup
        open={setupDialogOpen}
        onClose={() => setSetupDialogOpen(false)}
        botToken={botToken}
        onBotTokenChange={handleBotTokenChange}
        useGlobalBot={useGlobalBot}
      />
    </Box>
  )
}

export default TriggerTelegram
