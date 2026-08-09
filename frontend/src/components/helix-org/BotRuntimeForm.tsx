// AgentConfigForm is the controlled, presentational agent configuration picker.
// It mirrors the per-agent picker on the Agent settings page
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
import { useCodexSubscriptions } from '../../services/codexSubscriptionsService'
import {
  CLAUDE_SUBSCRIPTION_MODELS,
  CODEX_SUBSCRIPTION_MODELS,
  DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
  DEFAULT_CODEX_SUBSCRIPTION_MODEL,
} from '../agent/CodingAgentForm'
import CodeAgentEffortSelect, { getCodeAgentEffortOptions } from '../agent/CodeAgentEffortSelect'
import { useHelixModelsForProvider, useHelixProviders } from '../../services/helixOrgService'
import AgentHarness from '../agent/AgentHarness'

export interface AgentConfigValue {
  runtime: string
  credentials: string
  provider: string
  model: string
  reasoning_effort?: string
}

// Strong code-generation models to surface first in the picker.
const RECOMMENDED_MODELS = [
  'claude-opus-4-5-20251101',
  'claude-sonnet-4-5-20250929',
]

export const AgentConfigForm: FC<{
  value: AgentConfigValue
  onChange: (patch: Partial<AgentConfigValue>) => void
  showReasoningEffort?: boolean
  disabled?: boolean
}> = ({ value, onChange, showReasoningEffort = false, disabled = false }) => {
  const { data: providers } = useHelixProviders()
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const { data: codexSubscriptions } = useCodexSubscriptions()
  // Warm the models cache for the currently-selected provider.
  useHelixModelsForProvider(value.provider, { enabled: !!value.provider })

  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const hasCodexSubscription = (codexSubscriptions?.length ?? 0) > 0
  const hasAnthropicProvider = (providers ?? []).includes('anthropic')
  const hasOpenAIProvider = (providers ?? []).includes('openai')

  const isClaude = value.runtime === 'claude_code'
  const isCodex = value.runtime === 'codex_cli'
  const supportsSubscription = isClaude || isCodex
  const hasRuntimeSubscription = isClaude ? hasClaudeSubscription : isCodex ? hasCodexSubscription : false
  const hasRuntimeAPIKey = isClaude ? hasAnthropicProvider : isCodex ? hasOpenAIProvider : false
  const apiKeyMode = !supportsSubscription || value.credentials === 'api_key'
  const effortValue = !value.reasoning_effort || value.reasoning_effort === 'none' ? 'default' : value.reasoning_effort
  const effortOptions = getCodeAgentEffortOptions(value.runtime)
  const canConfigureEffort = showReasoningEffort && (isClaude || isCodex)

  const onEffort = (effort: string) => {
    onChange({ reasoning_effort: effort === 'default' ? 'none' : effort })
  }

  const onRuntime = (v: string) => {
    const patch: Partial<AgentConfigValue> = { runtime: v }
    if (v === 'claude_code' && value.credentials === 'subscription') {
      patch.model = DEFAULT_CLAUDE_SUBSCRIPTION_MODEL
    } else if (v === 'codex_cli' && value.credentials === 'subscription') {
      patch.model = DEFAULT_CODEX_SUBSCRIPTION_MODEL
    }
    // Runtimes without subscription support are always API-key routed; keep the stored value
    // consistent with what the server coerces to.
    if (v !== 'claude_code' && v !== 'codex_cli' && value.credentials !== 'api_key') {
      patch.credentials = 'api_key'
    }
    onChange(patch)
  }

  const onCredentials = (credentials: string) => {
    const patch: Partial<AgentConfigValue> = { credentials }
    if (credentials === 'subscription') {
      patch.provider = ''
      patch.model = isClaude ? DEFAULT_CLAUDE_SUBSCRIPTION_MODEL : DEFAULT_CODEX_SUBSCRIPTION_MODEL
    } else if (value.credentials === 'subscription') {
      patch.model = ''
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
            disabled={disabled}
            renderValue={(runtime) => (
              <AgentHarness runtime={runtime} variant="long" size={16} />
            )}
          >
            <MenuItem value="zed_agent">
              <Stack direction="row" spacing={1.25} alignItems="center">
                <AgentHarness runtime="zed_agent" variant="short" size={18} />
                <Box>
                  <Typography variant="body2">Zed Agent</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Built-in, Anthropic & OpenAI compatible
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value="qwen_code">
              <Stack direction="row" spacing={1.25} alignItems="center">
                <AgentHarness runtime="qwen_code" variant="short" size={18} />
                <Box>
                  <Typography variant="body2">Qwen Code</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Optimized for Qwen, including smaller models
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value="claude_code">
              <Stack direction="row" spacing={1.25} alignItems="center">
                <AgentHarness runtime="claude_code" variant="short" size={18} />
                <Box>
                  <Typography variant="body2">Claude Code</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Anthropic's coding agent
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value="codex_cli">
              <Stack direction="row" spacing={1.25} alignItems="center">
                <AgentHarness runtime="codex_cli" variant="short" size={18} />
                <Box>
                  <Typography variant="body2">Codex</Typography>
                  <Typography variant="caption" color="text.secondary">
                    OpenAI's coding agent
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value="goose_code">
              <Stack direction="row" spacing={1.25} alignItems="center">
                <AgentHarness runtime="goose_code" variant="short" size={18} />
                <Box>
                  <Typography variant="body2">Goose</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Open-source ACP agent (AAIF)
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
          </Select>
        </FormControl>
      </Box>

      {supportsSubscription && (
        <Box>
          <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
            Credentials
          </Typography>
          <FormControl>
            <RadioGroup value={value.credentials} onChange={(e) => onCredentials(e.target.value)}>
              <FormControlLabel
                value="subscription"
                control={<Radio size="small" />}
                disabled={disabled || !hasRuntimeSubscription}
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="body2">{isClaude ? 'Claude Subscription' : 'ChatGPT Subscription'}</Typography>
                    {hasRuntimeSubscription ? (
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
                disabled={disabled || !hasRuntimeAPIKey}
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="body2">{isClaude ? 'Anthropic API Key' : 'OpenAI API Key'}</Typography>
                    {hasRuntimeAPIKey ? (
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
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems="flex-start">
          <Box sx={{ flex: 1, width: '100%', minWidth: 0 }}>
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
              Model
            </Typography>
            <AdvancedModelPicker
              recommendedModels={RECOMMENDED_MODELS}
              hint="Select the default agent model"
              selectedProvider={value.provider}
              selectedModelId={value.model}
              disabled={disabled}
              onSelectModel={(prov, modelId) => {
                if (!disabled) onChange({ provider: prov, model: modelId })
              }}
              currentType="text"
              displayMode="short"
              buttonProps={{ fullWidth: true }}
            />
          </Box>
          {canConfigureEffort && (
            <CodeAgentEffortSelect options={effortOptions} value={effortValue} onChange={onEffort} disabled={disabled} />
          )}
        </Stack>
      ) : hasRuntimeSubscription ? (
        isCodex ? (
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems="flex-start">
            <Box sx={{ flex: 1, width: '100%' }}>
              <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>Model</Typography>
              <FormControl fullWidth size="small">
                <Select
                  value={value.model || DEFAULT_CODEX_SUBSCRIPTION_MODEL}
                  onChange={(event) => onChange({ model: event.target.value })}
                  disabled={disabled}
                >
                  {CODEX_SUBSCRIPTION_MODELS.map((supportedModel) => (
                    <MenuItem key={supportedModel.id} value={supportedModel.id}>
                      <Typography variant="body2">{supportedModel.label}</Typography>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>
            {canConfigureEffort && (
              <CodeAgentEffortSelect options={effortOptions} value={effortValue} onChange={onEffort} disabled={disabled} />
            )}
          </Stack>
        ) : isClaude ? (
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems="flex-start">
            <Box sx={{ flex: 1, width: '100%' }}>
              <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>Model</Typography>
              <FormControl fullWidth size="small">
                <Select
                  value={value.model || DEFAULT_CLAUDE_SUBSCRIPTION_MODEL}
                  onChange={(event) => onChange({ model: event.target.value })}
                  disabled={disabled}
                >
                  {CLAUDE_SUBSCRIPTION_MODELS.map((supportedModel) => (
                    <MenuItem key={supportedModel.id} value={supportedModel.id}>
                      <Typography variant="body2">{supportedModel.label}</Typography>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>
            {canConfigureEffort && (
              <CodeAgentEffortSelect options={effortOptions} value={effortValue} onChange={onEffort} disabled={disabled} />
            )}
          </Stack>
        ) : (
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems="flex-start">
            <Typography variant="caption" color="text.secondary" sx={{ flex: 1, pt: 1 }}>Uses the connected subscription.</Typography>
            {canConfigureEffort && (
              <CodeAgentEffortSelect options={effortOptions} value={effortValue} onChange={onEffort} disabled={disabled} />
            )}
          </Stack>
        )
      ) : null}
    </Stack>
  )
}

export default AgentConfigForm
