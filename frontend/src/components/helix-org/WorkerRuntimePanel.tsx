// WorkerRuntimePanel is the org-level "Default Bot Runtime" config. It mirrors
// the per-agent runtime picker on the Agent settings page (AppSettings) — same
// style and terms (Runtime / Credentials / Model, friendly labels) so it
// reads identically — but writes the org-level worker.* config registry keys
// that new Bots inherit. Changes auto-save; org context is resolved from
// router.params.org_id by the underlying hooks.

import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Paper from '@mui/material/Paper'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'

import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useSnackbar from '../../hooks/useSnackbar'
import {
  SettingsSpecDTO,
  useHelixModelsForProvider,
  useHelixOrgSettings,
  useHelixProviders,
  useSetHelixOrgSetting,
} from '../../services/helixOrgService'

// Strong code-generation models to surface first in the picker.
const RECOMMENDED_MODELS = [
  'claude-opus-4-5-20251101',
  'claude-sonnet-4-5-20250929',
]

// JSON-encode the value so it satisfies the registry's string-spec contract.
const encodeStringValue = (raw: string): string => JSON.stringify(raw)

// Read the persisted JSON value back into a plain string. Falls back to empty
// when parsing fails (e.g. a masked value).
const decodeStringValue = (v: string): string => {
  if (!v) return ''
  try {
    const parsed = JSON.parse(v)
    return typeof parsed === 'string' ? parsed : ''
  } catch {
    return v
  }
}

const WorkerRuntimePanel: FC = () => {
  const { data, isLoading } = useHelixOrgSettings()
  const { data: providers } = useHelixProviders()
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()

  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const hasAnthropicProvider = (providers ?? []).includes('anthropic')

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])

  const initialRuntime = decodeStringValue(specByKey.get('worker.runtime')?.value ?? '') || 'claude_code'
  const initialCreds = decodeStringValue(specByKey.get('worker.credentials')?.value ?? '') || 'subscription'
  const initialProvider = decodeStringValue(specByKey.get('worker.provider')?.value ?? '')
  const initialModel = decodeStringValue(specByKey.get('worker.model')?.value ?? '')

  const [runtime, setRuntime] = useState<string>(initialRuntime)
  const [credentials, setCredentials] = useState<string>(initialCreds)
  const [provider, setProvider] = useState<string>(initialProvider)
  const [model, setModel] = useState<string>(initialModel)

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setRuntime(initialRuntime)
    setCredentials(initialCreds)
    setProvider(initialProvider)
    setModel(initialModel)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  // Warm the models cache for the picker's currently-selected provider.
  useHelixModelsForProvider(provider, { enabled: !!provider })

  const save = async (key: string, value: string, label: string) => {
    try {
      await setMut.mutateAsync({ key, value: encodeStringValue(value) })
      snackbar.success(`${label} saved`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }

  const onRuntime = (v: string) => {
    setRuntime(v)
    save('worker.runtime', v, 'Runtime')
    // Non-claude runtimes are always API-key routed; persist that so the
    // stored value matches what the server coerces to.
    if (v !== 'claude_code' && credentials !== 'api_key') {
      setCredentials('api_key')
      save('worker.credentials', 'api_key', 'Credentials')
    }
  }

  const onCredentials = (v: string) => {
    setCredentials(v)
    save('worker.credentials', v, 'Credentials')
  }

  const onSelectModel = (prov: string, modelId: string) => {
    setProvider(prov)
    setModel(modelId)
    save('worker.provider', prov, 'Provider')
    save('worker.model', modelId, 'Model')
  }

  const isClaude = runtime === 'claude_code'
  // claude_code + subscription is the only mode that doesn't route through a
  // provider/model; everything else does.
  const apiKeyMode = !isClaude || credentials === 'api_key'

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      {isLoading ? (
        <LoadingSpinner />
      ) : (
        <Stack spacing={2}>
          <Box>
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
              Runtime
            </Typography>
            <FormControl fullWidth size="small">
              <Select
                value={runtime}
                onChange={(e) => onRuntime(e.target.value)}
                renderValue={(v) => {
                  if (v === 'claude_code') return 'Claude Code'
                  if (v === 'qwen_code') return 'Qwen Code'
                  if (v === 'goose_code') return 'Goose'
                  return 'Zed Agent'
                }}
              >
                <MenuItem value="zed_agent">
                  <Box>
                    <Typography variant="body2">Zed Agent</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Built-in, Anthropic & OpenAI compatible
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value="qwen_code">
                  <Box>
                    <Typography variant="body2">Qwen Code</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Optimized for Qwen, including smaller models
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value="claude_code">
                  <Box>
                    <Typography variant="body2">Claude Code</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Anthropic's coding agent
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value="goose_code">
                  <Box>
                    <Typography variant="body2">Goose</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Open-source ACP agent (AAIF)
                    </Typography>
                  </Box>
                </MenuItem>
              </Select>
            </FormControl>
          </Box>

          {isClaude && (
            <Box>
              <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                Credentials
              </Typography>
              <FormControl>
                <RadioGroup value={credentials} onChange={(e) => onCredentials(e.target.value)}>
                  <FormControlLabel
                    value="subscription"
                    control={<Radio size="small" />}
                    disabled={!hasClaudeSubscription}
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="body2">Claude Subscription</Typography>
                        {hasClaudeSubscription ? (
                          <CheckCircleIcon sx={{ fontSize: 14, color: 'success.main' }} />
                        ) : (
                          <Typography variant="caption" color="text.secondary">(not connected)</Typography>
                        )}
                      </Box>
                    }
                  />
                  <FormControlLabel
                    value="api_key"
                    control={<Radio size="small" />}
                    disabled={!hasAnthropicProvider}
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="body2">Anthropic API Key</Typography>
                        {hasAnthropicProvider ? (
                          <CheckCircleIcon sx={{ fontSize: 14, color: 'success.main' }} />
                        ) : (
                          <Typography variant="caption" color="text.secondary">(not configured)</Typography>
                        )}
                      </Box>
                    }
                  />
                </RadioGroup>
              </FormControl>
              {!hasClaudeSubscription && !hasAnthropicProvider && (
                <Typography variant="caption" color="warning.main" sx={{ display: 'block', mt: 0.5 }}>
                  Connect a Claude subscription or add an Anthropic API key above in Providers.
                </Typography>
              )}
            </Box>
          )}

          {apiKeyMode ? (
            <Box>
              <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                Model
              </Typography>
              <AdvancedModelPicker
                recommendedModels={RECOMMENDED_MODELS}
                hint="Select the model your Bots use by default"
                selectedProvider={provider}
                selectedModelId={model}
                onSelectModel={onSelectModel}
                currentType="text"
                displayMode="short"
              />
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary">
              Uses your connected Claude subscription — no model selection needed.
            </Typography>
          )}
        </Stack>
      )}
    </Paper>
  )
}

export default WorkerRuntimePanel
