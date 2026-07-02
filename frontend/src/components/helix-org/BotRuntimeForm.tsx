// BotRuntimeForm is the controlled, presentational runtime/credentials/model
// picker for a Bot. It mirrors the per-agent picker on the Agent settings page
// (AppSettings) — same style and terms — but is state-agnostic: the parent
// owns the value and decides how to persist it (auto-save to an org's config
// on the settings page, or deferred-until-create in the new-org dialog).

import { FC } from 'react'
import Box from '@mui/material/Box'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'

import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useHelixModelsForProvider, useHelixProviders } from '../../services/helixOrgService'

export interface BotRuntimeValue {
  runtime: string
  credentials: string
  provider: string
  model: string
}

// Strong code-generation models to surface first in the picker.
const RECOMMENDED_MODELS = [
  'claude-opus-4-5-20251101',
  'claude-sonnet-4-5-20250929',
]

export const BotRuntimeForm: FC<{
  value: BotRuntimeValue
  onChange: (patch: Partial<BotRuntimeValue>) => void
}> = ({ value, onChange }) => {
  const { data: providers } = useHelixProviders()
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  // Warm the models cache for the currently-selected provider.
  useHelixModelsForProvider(value.provider, { enabled: !!value.provider })

  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const hasAnthropicProvider = (providers ?? []).includes('anthropic')

  const isClaude = value.runtime === 'claude_code'
  // claude_code + subscription is the only mode that doesn't route through a
  // provider/model; everything else does.
  const apiKeyMode = !isClaude || value.credentials === 'api_key'

  const onRuntime = (v: string) => {
    const patch: Partial<BotRuntimeValue> = { runtime: v }
    // Non-claude runtimes are always API-key routed; keep the stored value
    // consistent with what the server coerces to.
    if (v !== 'claude_code' && value.credentials !== 'api_key') {
      patch.credentials = 'api_key'
    }
    onChange(patch)
  }

  return (
    <Stack spacing={2}>
      <Box>
        <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
          Runtime
        </Typography>
        <FormControl fullWidth size="small">
          <Select
            value={value.runtime}
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
            <RadioGroup value={value.credentials} onChange={(e) => onChange({ credentials: e.target.value })}>
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
            selectedProvider={value.provider}
            selectedModelId={value.model}
            onSelectModel={(prov, modelId) => onChange({ provider: prov, model: modelId })}
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
  )
}

export default BotRuntimeForm
