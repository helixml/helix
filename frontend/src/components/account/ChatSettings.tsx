import React, { FC, useCallback, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Collapse from '@mui/material/Collapse'
import FormControlLabel from '@mui/material/FormControlLabel'
import Grid from '@mui/material/Grid'
import Slider from '@mui/material/Slider'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { ChevronDown, ChevronUp } from 'lucide-react'

import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'
import {
  useGetConfig,
  useGetUserChatSettings,
  useUpdateUserChatSettings,
} from '../../services/userService'
import { TypesUserChatSettings } from '../../api/api'

const TEMPERATURE_MIN = 0
const TEMPERATURE_MAX = 2
const TEMPERATURE_STEP = 0.1
const TEMPERATURE_DEFAULT = 0.7

const parseOptionalFloat = (value: string): number | undefined => {
  const trimmed = value.trim()
  if (trimmed === '') return undefined
  const parsed = parseFloat(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

const parseOptionalInt = (value: string): number | undefined => {
  const trimmed = value.trim()
  if (trimmed === '') return undefined
  const parsed = parseInt(trimmed, 10)
  return Number.isFinite(parsed) ? parsed : undefined
}

const ChatSettings: FC = () => {
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()

  const { data: serverConfig } = useGetConfig()
  const { data: chatSettings, isLoading: isLoadingChatSettings } = useGetUserChatSettings()
  const updateChatSettings = useUpdateUserChatSettings()

  const defaultSystemPrompt = serverConfig?.default_chat_system_prompt ?? ''

  const [systemPromptEnabled, setSystemPromptEnabled] = useState<boolean>(true)
  const [chatSystemPrompt, setChatSystemPrompt] = useState<string>('')
  const [temperature, setTemperature] = useState<number>(TEMPERATURE_DEFAULT)
  const [chatTopP, setChatTopP] = useState<string>('')
  const [chatMaxTokens, setChatMaxTokens] = useState<string>('')
  const [chatFrequencyPenalty, setChatFrequencyPenalty] = useState<string>('')
  const [chatPresencePenalty, setChatPresencePenalty] = useState<string>('')
  const [advancedOpen, setAdvancedOpen] = useState<boolean>(false)

  // Track whether we've hydrated form state from the server response. Until
  // that happens we don't trust the prefilled values (which would otherwise
  // immediately overwrite the saved system prompt with the default).
  const [hydrated, setHydrated] = useState<boolean>(false)

  useEffect(() => {
    if (!chatSettings) return

    const enabled = chatSettings.system_prompt_enabled !== false
    setSystemPromptEnabled(enabled)
    setChatSystemPrompt(
      chatSettings.system_prompt && chatSettings.system_prompt.length > 0
        ? chatSettings.system_prompt
        : defaultSystemPrompt,
    )
    setTemperature(
      chatSettings.temperature !== undefined ? chatSettings.temperature : TEMPERATURE_DEFAULT,
    )
    setChatTopP(chatSettings.top_p !== undefined ? String(chatSettings.top_p) : '')
    setChatMaxTokens(chatSettings.max_tokens !== undefined ? String(chatSettings.max_tokens) : '')
    setChatFrequencyPenalty(
      chatSettings.frequency_penalty !== undefined ? String(chatSettings.frequency_penalty) : '',
    )
    setChatPresencePenalty(
      chatSettings.presence_penalty !== undefined ? String(chatSettings.presence_penalty) : '',
    )
    setHydrated(true)
  }, [chatSettings, defaultSystemPrompt])

  // Default-fill the textbox when the user hasn't typed anything yet and the
  // backend default arrives after the saved settings.
  useEffect(() => {
    if (!hydrated) return
    if (chatSystemPrompt === '' && defaultSystemPrompt !== '') {
      setChatSystemPrompt(defaultSystemPrompt)
    }
  }, [defaultSystemPrompt, hydrated]) // eslint-disable-line react-hooks/exhaustive-deps

  const isSaving = updateChatSettings.isPending
  const disabled = isLoadingChatSettings || isSaving

  const handleSave = useCallback(async () => {
    const payload: TypesUserChatSettings = {
      system_prompt_enabled: systemPromptEnabled,
      system_prompt: systemPromptEnabled ? chatSystemPrompt.trim() : '',
      temperature,
      top_p: parseOptionalFloat(chatTopP),
      max_tokens: parseOptionalInt(chatMaxTokens),
      frequency_penalty: parseOptionalFloat(chatFrequencyPenalty),
      presence_penalty: parseOptionalFloat(chatPresencePenalty),
    }
    try {
      await updateChatSettings.mutateAsync(payload)
      snackbar.success('Chat settings saved')
    } catch (err) {
      console.error('Failed to save chat settings:', err)
      snackbar.error(err instanceof Error ? err.message : 'Failed to save chat settings')
    }
  }, [
    systemPromptEnabled,
    chatSystemPrompt,
    temperature,
    chatTopP,
    chatMaxTokens,
    chatFrequencyPenalty,
    chatPresencePenalty,
    updateChatSettings,
    snackbar,
  ])

  const temperatureMarks = useMemo(
    () => [
      { value: 0, label: '0' },
      { value: 1, label: '1' },
      { value: 2, label: '2' },
    ],
    [],
  )

  return (
    <>
      <Grid container spacing={2} sx={{ mb: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
        <Grid item xs={12}>
          <Typography variant="h6">Chat Defaults</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            These defaults apply when you chat with a model directly. Apps and agents
            always use their own configuration and ignore these settings.
          </Typography>

          <Box sx={{ mb: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
              <Typography variant="subtitle2">System Prompt</Typography>
              <FormControlLabel
                control={(
                  <Switch
                    checked={systemPromptEnabled}
                    onChange={(e) => setSystemPromptEnabled(e.target.checked)}
                    disabled={disabled}
                  />
                )}
                label={systemPromptEnabled ? 'Enabled' : 'Disabled'}
              />
            </Box>
            <TextField
              fullWidth
              multiline
              minRows={4}
              value={chatSystemPrompt}
              onChange={(e) => setChatSystemPrompt(e.target.value)}
              placeholder={defaultSystemPrompt || 'You are a helpful assistant…'}
              variant="outlined"
              disabled={disabled || !systemPromptEnabled}
              helperText={
                systemPromptEnabled
                  ? 'Pre-filled with the platform default. Edit to customise.'
                  : 'No system prompt will be sent to the model.'
              }
            />
          </Box>

          <Box sx={{ mb: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', mb: 1 }}>
              <Typography variant="subtitle2">Temperature</Typography>
              <Typography variant="body2" color="text.secondary">{temperature.toFixed(1)}</Typography>
            </Box>
            <Slider
              value={temperature}
              min={TEMPERATURE_MIN}
              max={TEMPERATURE_MAX}
              step={TEMPERATURE_STEP}
              marks={temperatureMarks}
              valueLabelDisplay="auto"
              onChange={(_, value) => setTemperature(Array.isArray(value) ? value[0] : value)}
              disabled={disabled}
              sx={{ mt: 1 }}
            />
            <Typography variant="caption" color="text.secondary">
              Lower is more deterministic, higher is more creative.
            </Typography>
          </Box>

          <Box sx={{ mb: 2 }}>
            <Button
              variant="text"
              size="small"
              onClick={() => setAdvancedOpen((open) => !open)}
              endIcon={advancedOpen ? <ChevronUp size={18} /> : <ChevronDown size={18} />}
              sx={{ pl: 0 }}
            >
              Advanced
            </Button>
            <Collapse in={advancedOpen} unmountOnExit>
              <Grid container spacing={2} sx={{ mt: 0.5 }}>
                <Grid item xs={12} sm={6} md={3}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Top P</Typography>
                  <TextField
                    fullWidth
                    value={chatTopP}
                    onChange={(e) => setChatTopP(e.target.value)}
                    placeholder="e.g. 1"
                    variant="outlined"
                    disabled={disabled}
                    helperText="0–1"
                  />
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Max Tokens</Typography>
                  <TextField
                    fullWidth
                    value={chatMaxTokens}
                    onChange={(e) => setChatMaxTokens(e.target.value)}
                    placeholder="e.g. 2048"
                    variant="outlined"
                    disabled={disabled}
                    helperText="Response length cap"
                  />
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Frequency Penalty</Typography>
                  <TextField
                    fullWidth
                    value={chatFrequencyPenalty}
                    onChange={(e) => setChatFrequencyPenalty(e.target.value)}
                    placeholder="e.g. 0"
                    variant="outlined"
                    disabled={disabled}
                    helperText="-2 to 2"
                  />
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Presence Penalty</Typography>
                  <TextField
                    fullWidth
                    value={chatPresencePenalty}
                    onChange={(e) => setChatPresencePenalty(e.target.value)}
                    placeholder="e.g. 0"
                    variant="outlined"
                    disabled={disabled}
                    helperText="-2 to 2"
                  />
                </Grid>
              </Grid>
            </Collapse>
          </Box>

        </Grid>
      </Grid>

      <Box
        sx={{
          position: 'sticky',
          bottom: 0,
          display: 'flex',
          justifyContent: 'flex-end',
          py: 1.5,
          px: 2,
          backgroundColor: themeConfig.darkPanel,
          borderTop: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          zIndex: 1,
        }}
      >
        <Button
          variant="contained"
          color="secondary"
          onClick={handleSave}
          disabled={disabled}
        >
          {isSaving ? 'Saving…' : 'Save'}
        </Button>
      </Box>
    </>
  )
}

export default ChatSettings
