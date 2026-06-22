import React, { useState, useEffect, FC, useRef, useMemo } from 'react'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Switch from '@mui/material/Switch'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import Slider from '@mui/material/Slider'
import FormControl from '@mui/material/FormControl'
import Typography from '@mui/material/Typography'
import Stack from '@mui/material/Stack'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import Menu from '@mui/material/Menu'
import { styled } from '@mui/material/styles'

import { useQuery } from '@tanstack/react-query'

import {
  IAppFlatState,
  IAgentType,
  IExternalAgentConfig,
  AGENT_TYPE_HELIX_AGENT,
  AGENT_TYPE_ZED_EXTERNAL,
} from '../../types'

import * as api from '../../api/api'


import useApi from '../../hooks/useApi'
import useDebouncedCallback from '../../hooks/useDebouncedCallback'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { AgentTypeSelector } from '../agent'
import { CLAUDE_SUBSCRIPTION_MODELS, DEFAULT_CLAUDE_SUBSCRIPTION_MODEL } from '../agent/CodingAgentForm'
import GooseRecipesEditor from './GooseRecipesEditor'
import Divider from '@mui/material/Divider'
import { useListProviders } from '../../services/providersService'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import useRouter from '../../hooks/useRouter'
import { useGetOrgByName } from '../../services/orgService'

// Recommended models configuration
const RECOMMENDED_MODELS = {
  // Tool use required, reasoning and tool calling, must be strong model for complex tasks
  reasoning: [
    'o3-mini',
    'o4-mini',
    'Qwen/Qwen2.5-72B-Instruct-Turbo',
    'Qwen/Qwen3-235B-A22B-fp8-tput',
  ],
  // Tool use required, planning next actions using skills
  generation: [
    'gpt-4o',
    'gpt-4o-mini',
    'Qwen/Qwen3-235B-A22B-fp8-tput',
    'meta-llama/Llama-4-Scout-17B-16E-Instruct',
    'meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8'
  ],
  // No tool use required but might be useful
  smallReasoning: [
    'o3-mini',
    'o4-mini',
    'gpt-4o-mini',
    'gpt-4o',
    'Qwen/Qwen2.5-72B-Instruct-Turbo',
    'Qwen/Qwen2.5-7B-Instruct-Turbo',
  ],
  // No tool use required, can be any text generation model
  smallGeneration: ['gpt-4o', 'gpt-4o-mini', 'Qwen/Qwen2.5-7B-Instruct-Turbo', 'openai/gpt-oss-20b'],
  // Zed external agent - strong code generation models
  zedExternal: [
    // Anthropic
    'claude-opus-4-5-20251101',
    'claude-sonnet-4-5-20250929',
    'claude-haiku-4-5-20251001',
    // OpenAI
    'openai/gpt-5.1-codex',
    'openai/gpt-oss-120b',
    // Google Gemini
    'gemini-2.5-pro',
    'gemini-2.5-flash',
    // Zhipu GLM
    'glm-4.6',
    // Qwen (Coder + Large)
    'Qwen/Qwen3-Coder-480B-A35B-Instruct',
    'Qwen/Qwen3-Coder-30B-A3B-Instruct',
    'Qwen/Qwen3-235B-A22B-fp8-tput',
  ]
};

interface AppSettingsProps {
  id: string,
  app: IAppFlatState,
  // Partial — every call site sends only the fields that actually changed.
  // mergeFlatStateIntoApp ignores `undefined`, so omitted fields are
  // preserved on the persisted assistant config (the bug this fix was
  // written to address: full-state sends were clobbering hidden fields).
  onUpdate: (updates: Partial<IAppFlatState>) => Promise<void>,
  readOnly?: boolean,
  showErrors?: boolean,
  isAdmin?: boolean,
}

const DEFAULT_SYSTEM_PROMPT = `You are a helpful AI assistant called Helix. Today is {{ .LocalDate }}, local time is {{ .LocalTime }}.`

// Define default values.
// If you are updating these, also update
// 'func setAppDefaults(apps ...*types.App)' in api/pkg/store/store_apps.go
const DEFAULT_VALUES = {
  system_prompt: DEFAULT_SYSTEM_PROMPT,
  context_limit: 0,
  temperature: 0.1,
  frequency_penalty: 0,
  presence_penalty: 0,
  top_p: 1,
  max_tokens: 2000,
  reasoning_effort: 'medium',
  max_iterations: 10,
} as const

// Add BarsIcon component
const BarsIcon = ({ effort }: { effort: string }) => {
  const getBars = () => {
    switch (effort) {
      case 'none':
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 2, height: 4, bgcolor: 'text.secondary' }} />
          </Box>
        )
      case 'low':
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 2, height: 8, bgcolor: 'info.main' }} />
          </Box>
        )
      case 'medium':
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 2, height: 8, bgcolor: 'success.main' }} />
            <Box sx={{ width: 2, height: 12, bgcolor: 'success.main' }} />
          </Box>
        )
      case 'high':
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 2, height: 8, bgcolor: 'error.main' }} />
            <Box sx={{ width: 2, height: 12, bgcolor: 'error.main' }} />
            <Box sx={{ width: 2, height: 16, bgcolor: 'error.main' }} />
          </Box>
        )
      default:
        return null
    }
  }

  // Add a fixed width and center the bars
  return (
    <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
      {getBars()}
    </Box>
  )
}

// Add styled resizable textarea component
const ResizableTextarea = styled('textarea')(({ theme }) => ({
  width: '100%',
  minHeight: '200px', // Increased from 120px to 200px
  padding: '16.5px 14px',
  fontSize: '1rem',
  fontFamily: 'inherit',
  lineHeight: '1.4375em',
  border: `1px solid ${theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.23)' : 'rgba(255, 255, 255, 0.23)'}`,
  borderRadius: '4px',
  backgroundColor: 'transparent',
  color: theme.palette.text.primary,
  resize: 'vertical',
  outline: 'none',
  transition: theme.transitions.create(['border-color', 'box-shadow']),
  '&:focus': {
    borderColor: theme.palette.primary.main,
    boxShadow: `0 0 0 2px ${theme.palette.primary.main}20`,
  },
  '&:disabled': {
    backgroundColor: theme.palette.action.disabledBackground,
    color: theme.palette.action.disabled,
    cursor: 'not-allowed',
  },
  '&::placeholder': {
    color: theme.palette.text.disabled,
  },
}));

const AppSettings: FC<AppSettingsProps> = ({
  id,
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
  isAdmin = false,
}) => {
  // Get initial showAdvanced value from URL
  const [showAdvanced, setShowAdvanced] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    return params.get('showAdvanced') === 'true'
  })

  // Update URL when showAdvanced changes
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (showAdvanced) {
      params.set('showAdvanced', 'true')
    } else {
      params.delete('showAdvanced')
    }
    // Update URL without causing a page reload
    window.history.replaceState({}, '', `${window.location.pathname}?${params}`)
  }, [showAdvanced])

  // State for form fields
  const [system_prompt, setSystemPrompt] = useState(app.system_prompt || '')
  const [global, setGlobal] = useState(app.global || false)
  const [model, setModel] = useState(app.model || '')
  const [provider, setProvider] = useState(app.provider || '')

  // Agent mode settings
  const [agent_mode, setAgentMode] = useState(app.agent_mode || false)
  const [memory, setMemory] = useState(app.memory || false)
  const [max_iterations, setMaxIterations] = useState(app.max_iterations ?? DEFAULT_VALUES.max_iterations)

  // Agent type settings — auto-migrate helix_basic to helix_agent
  const migratedAgentType = (app.default_agent_type === 'helix_basic' || !app.default_agent_type) ? AGENT_TYPE_HELIX_AGENT : app.default_agent_type
  const [default_agent_type, setDefaultAgentType] = useState<IAgentType>(migratedAgentType)
  const [external_agent_config, setExternalAgentConfig] = useState<IExternalAgentConfig>(app.external_agent_config || {})
  const [reasoning_model, setReasoningModel] = useState(app.reasoning_model || '')
  const [reasoning_model_provider, setReasoningModelProvider] = useState(app.reasoning_model_provider || '')
  const [reasoning_model_effort, setReasoningModelEffort] = useState(app.reasoning_model_effort || 'none')
  const [generation_model, setGenerationModel] = useState(app.generation_model || '')
  const [generation_model_provider, setGenerationModelProvider] = useState(app.generation_model_provider || '')
  const [code_agent_runtime, setCodeAgentRuntime] = useState<'zed_agent' | 'qwen_code' | 'claude_code' | 'gemini_cli' | 'codex_cli' | 'goose_code'>(app.code_agent_runtime || 'zed_agent')
  // External agent display settings
  const [resolution, setResolution] = useState<'1080p' | '4k' | '5k'>(app.external_agent_config?.resolution as '1080p' | '4k' | '5k' || '1080p')
  const [desktopType, setDesktopType] = useState<'ubuntu' | 'sway'>(app.external_agent_config?.desktop_type as 'ubuntu' | 'sway' || 'ubuntu')
  const [zoomLevel, setZoomLevel] = useState<number>(app.external_agent_config?.zoom_level || ((app.external_agent_config?.resolution === '5k' || app.external_agent_config?.resolution === '4k') ? 200 : 100))
  const [refreshRate, setRefreshRate] = useState<number>(app.external_agent_config?.display_refresh_rate || 60)
  const [small_reasoning_model, setSmallReasoningModel] = useState(app.small_reasoning_model || '')
  const [small_reasoning_model_provider, setSmallReasoningModelProvider] = useState(app.small_reasoning_model_provider || '')
  const [small_reasoning_model_effort, setSmallReasoningModelEffort] = useState(app.small_reasoning_model_effort || 'none')
  const [small_generation_model, setSmallGenerationModel] = useState(app.small_generation_model || '')
  const [small_generation_model_provider, setSmallGenerationModelProvider] = useState(app.small_generation_model_provider || '')

  // Claude Code mode: 'subscription' (OAuth) or 'api_key' (Anthropic provider)
  const [claudeCodeMode, setClaudeCodeMode] = useState<'subscription' | 'api_key'>(
    app.code_agent_credential_type === 'subscription' ? 'subscription' :
    app.code_agent_credential_type === 'api_key' ? 'api_key' :
    // Legacy: infer from generation_model_provider for apps created before this field existed
    app.generation_model_provider ? 'api_key' : 'subscription'
  )

  // Claude subscription model (Opus/Sonnet/Haiku). Empty defaults to Opus.
  const [claudeSubscriptionModel, setClaudeSubscriptionModel] = useState(
    app.claude_subscription_model || DEFAULT_CLAUDE_SUBSCRIPTION_MODEL
  )

  // Provider availability checks for Claude Code mode selector
  const router = useRouter()
  const orgName = router.params.org_id
  const { data: orgForProviders } = useGetOrgByName(orgName, orgName !== undefined)
  const { data: providerEndpoints = [] } = useListProviders({
    loadModels: false,
    orgId: orgForProviders?.id,
    enabled: true,
  })
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const hasAnthropicProvider = providerEndpoints.some(ep => ep.name === 'anthropic')

  // Advanced settings state. Defaults must match the useEffect re-init below
  // and DEFAULT_VALUES (which mirrors api/pkg/store/store_apps.go).
  // Use `??` so an explicitly persisted 0 (e.g. temperature: 0, top_p: 0)
  // isn't silently rewritten to the default — `||` would treat 0 as falsy.
  const [contextLimit, setContextLimit] = useState(app.context_limit ?? DEFAULT_VALUES.context_limit)
  const [frequencyPenalty, setFrequencyPenalty] = useState(app.frequency_penalty ?? DEFAULT_VALUES.frequency_penalty)
  const [maxTokens, setMaxTokens] = useState(app.max_tokens ?? DEFAULT_VALUES.max_tokens)
  const [presencePenalty, setPresencePenalty] = useState(app.presence_penalty ?? DEFAULT_VALUES.presence_penalty)
  const [reasoningEffort, setReasoningEffort] = useState(app.reasoning_effort ?? DEFAULT_VALUES.reasoning_effort)
  const [temperature, setTemperature] = useState(app.temperature ?? DEFAULT_VALUES.temperature)
  const [topP, setTopP] = useState(app.top_p ?? DEFAULT_VALUES.top_p)

  // Track if component has been initialized
  const isInitialized = useRef(false)

  // Query sandboxes to determine which desktop types are available
  const apiHook = useApi()
  const { data: sandboxInstances } = useQuery({
    queryKey: ['sandbox-instances-desktop-types'],
    queryFn: async () => {
      const response = await apiHook.getApiClient().v1SandboxesList()
      return response.data
    },
    staleTime: 60000,
  })

  const availableDesktopTypes = useMemo(() => {
    const types = new Set<string>()
    if (sandboxInstances) {
      for (const sandbox of sandboxInstances) {
        // desktop_versions is Record<string, string> from the API but typed as number[] due to codegen bug
        const versions = sandbox.desktop_versions as unknown as Record<string, string> | undefined
        if (versions) {
          for (const key of Object.keys(versions)) {
            types.add(key)
          }
        }
      }
    }
    // Always include ubuntu as it's the default
    types.add('ubuntu')
    return types
  }, [sandboxInstances])

  // Update local state ONLY on initial mount, not when app prop changes
  useEffect(() => {
    // Only initialize values if not already initialized
    if (!isInitialized.current) {
      setSystemPrompt(app.system_prompt || DEFAULT_SYSTEM_PROMPT)
      setGlobal(app.global || false)
      setModel(app.model || '')
      // Agent configuration
      setAgentMode(app.agent_mode || false)
      setDefaultAgentType((app.default_agent_type === 'helix_basic' || !app.default_agent_type) ? AGENT_TYPE_HELIX_AGENT : app.default_agent_type)
      setExternalAgentConfig(app.external_agent_config || {})
      // Reasoning configuration
      setReasoningModel(app.reasoning_model || '')
      setReasoningModelProvider(app.reasoning_model_provider || '')

      setGenerationModel(app.generation_model || '')
      setGenerationModelProvider(app.generation_model_provider || '')
      setCodeAgentRuntime(app.code_agent_runtime || 'zed_agent')
      // Default to api_key when the field is unset. The previous fallback
      // (`generation_model_provider ? 'api_key' : 'subscription'`) flipped a
      // freshly-created zed_agent assistant to subscription mode, which then
      // strips the Helix-routed default_model and leaves the dev container
      // stuck at "Agent never connected" because start-zed-helix.sh waits for
      // the default_model key before launching Zed. Subscription is opt-in
      // (user must explicitly click the radio for a Claude Code agent).
      setClaudeCodeMode(
        app.code_agent_credential_type === 'subscription' ? 'subscription' : 'api_key'
      )
      setClaudeSubscriptionModel(app.claude_subscription_model || DEFAULT_CLAUDE_SUBSCRIPTION_MODEL)
      // External agent display settings
      setResolution(app.external_agent_config?.resolution as '1080p' | '4k' | '5k' || '1080p')
      setDesktopType(app.external_agent_config?.desktop_type as 'ubuntu' | 'sway' || 'ubuntu')
      setZoomLevel(app.external_agent_config?.zoom_level || (app.external_agent_config?.resolution === '4k' ? 200 : 100))
      setRefreshRate(app.external_agent_config?.display_refresh_rate || 60)

      setSmallReasoningModel(app.small_reasoning_model || '')
      setSmallReasoningModelProvider(app.small_reasoning_model_provider || '')

      setSmallGenerationModel(app.small_generation_model || '')
      setSmallGenerationModelProvider(app.small_generation_model_provider || '')

      setProvider(app.provider || '')
      setContextLimit(app.context_limit ?? DEFAULT_VALUES.context_limit)
      setFrequencyPenalty(app.frequency_penalty ?? DEFAULT_VALUES.frequency_penalty)
      setMaxTokens(app.max_tokens ?? DEFAULT_VALUES.max_tokens)
      setPresencePenalty(app.presence_penalty ?? DEFAULT_VALUES.presence_penalty)
      setReasoningEffort(app.reasoning_effort ?? DEFAULT_VALUES.reasoning_effort)
      setTemperature(app.temperature ?? DEFAULT_VALUES.temperature)
      setTopP(app.top_p ?? DEFAULT_VALUES.top_p)
      setMaxIterations(app.max_iterations ?? DEFAULT_VALUES.max_iterations)

      // Mark as initialized
      isInitialized.current = true
    }
  }, [app]) // Still depend on app, but we'll only use it for initialization

  // Auto-migrate helix_basic agents to helix_agent on first load
  useEffect(() => {
    if (app.default_agent_type === 'helix_basic') {
      onUpdate({ default_agent_type: AGENT_TYPE_HELIX_AGENT })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Map UI field names to IAppFlatState keys. Keep this small — only the
  // fields actually edited via the debounced text/number inputs.
  const ADVANCED_FIELD_MAP = {
    contextLimit: 'context_limit',
    frequencyPenalty: 'frequency_penalty',
    maxTokens: 'max_tokens',
    presencePenalty: 'presence_penalty',
    reasoningEffort: 'reasoning_effort',
    temperature: 'temperature',
    topP: 'top_p',
    system_prompt: 'system_prompt',
    maxIterations: 'max_iterations',
  } as const satisfies Record<string, keyof IAppFlatState>

  type AdvancedField = keyof typeof ADVANCED_FIELD_MAP

  // Debounced save: send ONLY the field that changed. mergeFlatStateIntoApp in
  // useApp ignores fields that are undefined, so partial updates are safe and
  // can't clobber unrelated values with stale local state.
  const debouncedUpdate = useDebouncedCallback((field: AdvancedField, value: number | string) => {
    const flatField = ADVANCED_FIELD_MAP[field]
    onUpdate({ [flatField]: value })
  }, 300)

  // Handle checkbox changes - these update immediately since they're not typing events.
  // Send only the changed field; mergeFlatStateIntoApp leaves everything else alone.
  const handleCheckboxChange = (field: 'global' | 'agent_mode', value: boolean) => {
    if (field === 'global') {
      setGlobal(value)
    } else if (field === 'agent_mode') {
      setAgentMode(value)
    }
    onUpdate({ [field]: value })
  }

  // Handle agent type changes
  const handleAgentTypeChange = (agentType: IAgentType, config?: IExternalAgentConfig) => {
    setDefaultAgentType(agentType)
    if (config !== undefined) {
      setExternalAgentConfig(config)
    }
    const updates: Partial<IAppFlatState> = { default_agent_type: agentType }
    if (config !== undefined) {
      updates.external_agent_config = config
    }
    onUpdate(updates)
  }

  const handleModelChange = (provider: string, model: string) => {
    setModel(model)
    setProvider(provider)
    onUpdate({ model, provider })
  }

  // Helper function to check if a value matches its default
  const isDefault = (field: keyof typeof DEFAULT_VALUES, value: number | string) => {
    return DEFAULT_VALUES[field] === value
  }

  // Helper function to create reset link
  const ResetLink = ({ field, value, onClick }: { field: keyof typeof DEFAULT_VALUES, value: number | string, onClick: () => void }) => {
    if (isDefault(field, value)) {
      return <Typography component="span" color="text.secondary" sx={{ ml: 1, fontSize: '0.875rem' }}>(Default)</Typography>
    }
    return (
      <Link
        component="button"
        variant="body2"
        onClick={onClick}
        sx={{ ml: 1, fontSize: '0.875rem' }}
      >
        (Reset to default)
      </Link>
    )
  }

  // Reset handlers for each field
  const handleReset = (field: keyof typeof DEFAULT_VALUES) => {
    const value = DEFAULT_VALUES[field]
    switch(field) {
      case 'context_limit':
        setContextLimit(value as number)
        debouncedUpdate('contextLimit', value as number)
        break
      case 'temperature':
        setTemperature(value as number)
        debouncedUpdate('temperature', value as number)
        break
      case 'frequency_penalty':
        setFrequencyPenalty(value as number)
        debouncedUpdate('frequencyPenalty', value as number)
        break
      case 'presence_penalty':
        setPresencePenalty(value as number)
        debouncedUpdate('presencePenalty', value as number)
        break
      case 'top_p':
        setTopP(value as number)
        debouncedUpdate('topP', value as number)
        break
      case 'max_tokens':
        setMaxTokens(value as number)
        debouncedUpdate('maxTokens', value as number)
        break
      case 'reasoning_effort':
        setReasoningEffort(value as string)
        debouncedUpdate('reasoningEffort', value as string)
        break
      case 'system_prompt':
        setSystemPrompt(value as string)
        debouncedUpdate('system_prompt', value as string)
        break
      case 'max_iterations':
        setMaxIterations(value as number)
        debouncedUpdate('maxIterations', value as number)
        break
    }
  }

  const [mainEffortMenuAnchor, setMainEffortMenuAnchor] = useState<null | HTMLElement>(null);
  const [smallEffortMenuAnchor, setSmallEffortMenuAnchor] = useState<null | HTMLElement>(null);

  const handleMainEffortClick = (event: React.MouseEvent<HTMLElement>) => {
    setMainEffortMenuAnchor(event.currentTarget);
  };

  const handleSmallEffortClick = (event: React.MouseEvent<HTMLElement>) => {
    setSmallEffortMenuAnchor(event.currentTarget);
  };

  const handleMainEffortClose = () => {
    setMainEffortMenuAnchor(null);
  };

  const handleSmallEffortClose = () => {
    setSmallEffortMenuAnchor(null);
  };

  const handleEffortSelect = (effort: string, isMain: boolean) => {
    if (isMain) {
      setReasoningModelEffort(effort);
      onUpdate({ reasoning_model_effort: effort });
      handleMainEffortClose();
    } else {
      setSmallReasoningModelEffort(effort);
      onUpdate({ small_reasoning_model_effort: effort });
      handleSmallEffortClose();
    }
  };

  const getEffortTooltip = (effort: string) => {
    switch (effort) {
      case 'none':
        return 'Reasoning disabled - no additional reasoning steps will be performed';
      case 'low':
        return 'Low effort - minimal reasoning steps, faster responses but may be less thorough';
      case 'medium':
        return 'Medium effort - balanced reasoning steps, good balance of speed and thoroughness';
      case 'high':
        return 'High effort - extensive reasoning steps, most thorough but may be slower';
      default:
        return '';
    }
  };

  return (
    <Box sx={{ mt: 2, mr: 2 }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          Configuration
        </Typography>
        <Stack direction="row" alignItems="center">
          <Typography gutterBottom>System Instructions</Typography>
          <ResetLink field="system_prompt" value={system_prompt} onClick={() => handleReset('system_prompt')} />
        </Stack>
        <Typography variant="body2" color="text.secondary">

        </Typography>
        <Box sx={{ mb: 3, mt: 1 }}>
          <ResizableTextarea
            value={system_prompt}
            onChange={(e) => {
              setSystemPrompt(e.target.value)
              debouncedUpdate('system_prompt', e.target.value)
            }}
            disabled={readOnly}
            placeholder="What does this agent do? How does it behave? What should it avoid doing?"
            style={{
              minHeight: '200px',
              resize: 'vertical'
            }}
          />
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
            What does this agent do? How does it behave? What should it avoid doing?
          </Typography>
        </Box>

        {/* Agent Type Selection */}
      <Box sx={{ mb: 3 }}>
        <Typography variant="subtitle1" sx={{ mb: 2 }}>Agent Type</Typography>
        <AgentTypeSelector
          value={default_agent_type}
          onChange={handleAgentTypeChange}
          externalAgentConfig={external_agent_config}
          disabled={readOnly}
          size="small"
        />
      </Box>

      {/* External Agent Configuration */}
      {default_agent_type === AGENT_TYPE_ZED_EXTERNAL && (
        <Box sx={{ mb: 3 }}>
          {/* Agent Runtime & Model - compact section */}
          <Stack spacing={2} sx={{ mb: 3 }}>
            <Box>
              <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                Agent Runtime
              </Typography>
              <FormControl fullWidth size="small">
                <Select
                  value={code_agent_runtime}
                  onChange={(e) => {
                    const newRuntime = e.target.value as 'zed_agent' | 'qwen_code' | 'claude_code' | 'goose_code';
                    setCodeAgentRuntime(newRuntime);
                    onUpdate({ code_agent_runtime: newRuntime });
                  }}
                  disabled={readOnly}
                  renderValue={(value) => {
                    if (value === 'claude_code') return 'Claude Code'
                    if (value === 'qwen_code') return 'Qwen Code'
                    if (value === 'goose_code') return 'Goose'
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

            {code_agent_runtime === 'claude_code' ? (
              <>
                <Box>
                  <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                    Credentials
                  </Typography>
                  <FormControl>
                    <RadioGroup
                      value={claudeCodeMode}
                      onChange={(e) => {
                        const mode = e.target.value as 'subscription' | 'api_key'
                        setClaudeCodeMode(mode)
                        if (mode === 'subscription') {
                          setGenerationModel('')
                          setGenerationModelProvider('')
                          onUpdate({
                            code_agent_credential_type: 'subscription',
                            generation_model: '',
                            generation_model_provider: '',
                          })
                        } else {
                          onUpdate({ code_agent_credential_type: 'api_key' })
                        }
                      }}
                    >
                      <FormControlLabel
                        value="subscription"
                        control={<Radio size="small" />}
                        disabled={readOnly || !hasClaudeSubscription}
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
                        disabled={readOnly || !hasAnthropicProvider}
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
                      Connect a Claude subscription or add an Anthropic API key in Settings &gt; Providers.
                    </Typography>
                  )}
                  {claudeCodeMode === 'subscription' && (
                    <Box sx={{ mt: 1.5 }}>
                      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                        Model
                      </Typography>
                      <FormControl fullWidth>
                        <Select
                          size="small"
                          value={claudeSubscriptionModel}
                          disabled={readOnly}
                          onChange={(e) => {
                            const nextModel = e.target.value
                            setClaudeSubscriptionModel(nextModel)
                            onUpdate({ claude_subscription_model: nextModel })
                          }}
                        >
                          {CLAUDE_SUBSCRIPTION_MODELS.map((m) => (
                            <MenuItem key={m.id} value={m.id}>
                              <Typography variant="body2">{m.label}</Typography>
                            </MenuItem>
                          ))}
                        </Select>
                      </FormControl>
                    </Box>
                  )}
                </Box>

                {claudeCodeMode === 'api_key' && (
                  <Box>
                    <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                      Model
                    </Typography>
                    <AdvancedModelPicker
                      recommendedModels={RECOMMENDED_MODELS.zedExternal}
                      hint="Select the Claude model for code generation"
                      selectedProvider={generation_model_provider}
                      selectedModelId={generation_model}
                      onSelectModel={(provider, modelId) => {
                        setGenerationModel(modelId);
                        setGenerationModelProvider(provider);
                        onUpdate({
                          generation_model: modelId,
                          generation_model_provider: provider,
                        });
                      }}
                      currentType="text"
                      displayMode="short"
                    />
                  </Box>
                )}
              </>
            ) : (
              <Box>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                  Model
                </Typography>
                <AdvancedModelPicker
                  recommendedModels={RECOMMENDED_MODELS.zedExternal}
                  hint="Select the LLM for code generation"
                  selectedProvider={provider}
                  selectedModelId={model}
                  onSelectModel={(provider, modelId) => {
                    setModel(modelId);
                    setProvider(provider);
                    onUpdate({ model: modelId, provider });
                  }}
                  currentType="text"
                  displayMode="short"
                />
              </Box>
            )}

            {code_agent_runtime === 'goose_code' && (
              <Box>
                <GooseRecipesEditor
                  recipeRepoURL={app.goose_recipe_repo_url || ''}
                  recipes={app.goose_recipes || []}
                  disabled={readOnly}
                  onChange={(next) => {
                    onUpdate({
                      goose_recipe_repo_url: next.recipeRepoURL,
                      goose_recipes: next.recipes,
                    })
                  }}
                />
              </Box>
            )}
          </Stack>

          <Divider sx={{ my: 2 }} />

          {/* Display Settings - side by side */}
          <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1.5 }}>
            Display
          </Typography>
          <Stack direction="row" spacing={2} sx={{ mb: 2 }}>
            <FormControl size="small" sx={{ minWidth: 160 }}>
              <Select
                value={resolution}
                onChange={(e) => {
                  const newResolution = e.target.value as '1080p' | '4k' | '5k';
                  const oldResolution = resolution; // Track previous resolution
                  setResolution(newResolution);
                  // Zoom level logic:
                  // - 1080p: always 100% (no HiDPI needed)
                  // - 4k/5k from 1080p: auto-set to 200% if zoom was default 100%
                  // - 4k/5k from 4k/5k: preserve current zoom (user may intentionally use 100% on large monitor)
                  let newZoom: number;
                  if (newResolution === '1080p') {
                    newZoom = 100;
                  } else if (oldResolution === '1080p' && zoomLevel === 100) {
                    // Coming from 1080p with default zoom - auto-set 200% for better readability
                    newZoom = 200;
                  } else {
                    // Already on 4k/5k or user set custom zoom - preserve their choice
                    newZoom = zoomLevel;
                  }
                  setZoomLevel(newZoom);
                  const updatedConfig = { ...external_agent_config, resolution: newResolution, zoom_level: newZoom };
                  setExternalAgentConfig(updatedConfig);
                  onUpdate({ external_agent_config: updatedConfig });
                }}
                disabled={readOnly}
                renderValue={(value) => value === '1080p' ? '1080p' : value === '4k' ? '4K' : '5K'}
              >
                <MenuItem value="1080p">
                  <Box>
                    <Typography variant="body2">1080p (1920×1080)</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Standard HD - works with all GPUs
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value="4k">
                  <Box>
                    <Typography variant="body2">4K (3840×2160)</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Ultra HD - requires powerful GPU
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value="5k">
                  <Box>
                    <Typography variant="body2">5K (5120×2880)</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Maximum - for the bold
                    </Typography>
                  </Box>
                </MenuItem>
              </Select>
            </FormControl>
            {availableDesktopTypes.size > 1 && (
              <FormControl size="small" sx={{ minWidth: 180 }}>
                <Select
                  value={desktopType}
                  onChange={(e) => {
                    const newDesktopType = e.target.value as 'ubuntu' | 'sway';
                    setDesktopType(newDesktopType);
                    const updatedConfig = { ...external_agent_config, desktop_type: newDesktopType };
                    setExternalAgentConfig(updatedConfig);
                    onUpdate({ external_agent_config: updatedConfig });
                  }}
                  disabled={readOnly}
                  renderValue={(value) => value === 'ubuntu' ? 'Ubuntu Desktop' : 'Sway'}
                >
                  <MenuItem value="ubuntu">
                    <Box>
                      <Typography variant="body2">Ubuntu Desktop (25.10)</Typography>
                      <Typography variant="caption" color="text.secondary">
                        Ubuntu GNOME desktop, user friendly and recommended
                      </Typography>
                    </Box>
                  </MenuItem>
                  {availableDesktopTypes.has('sway') && (
                    <MenuItem value="sway">
                      <Box>
                        <Typography variant="body2">Sway (Ubuntu 25.10)</Typography>
                        <Typography variant="caption" color="text.secondary">
                          i3-compatible tiling WM, for advanced users
                        </Typography>
                      </Box>
                    </MenuItem>
                  )}
                </Select>
              </FormControl>
            )}
            <FormControl size="small" sx={{ minWidth: 100 }}>
              <Select
                value={refreshRate}
                onChange={(e) => {
                  const newRefreshRate = e.target.value as number;
                  setRefreshRate(newRefreshRate);
                  const updatedConfig = { ...external_agent_config, display_refresh_rate: newRefreshRate };
                  setExternalAgentConfig(updatedConfig);
                  onUpdate({ external_agent_config: updatedConfig });
                }}
                disabled={readOnly}
                renderValue={(value) => `${value} fps`}
              >
                <MenuItem value={30}>
                  <Box>
                    <Typography variant="body2">30 fps</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Low bandwidth
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value={60}>
                  <Box>
                    <Typography variant="body2">60 fps</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Standard - recommended
                    </Typography>
                  </Box>
                </MenuItem>
                <MenuItem value={120}>
                  <Box>
                    <Typography variant="body2">120 fps</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Smooth - for ProMotion displays
                    </Typography>
                  </Box>
                </MenuItem>
              </Select>
            </FormControl>
          </Stack>

          {/* UI Zoom Setting */}
          <Box sx={{ mb: 2, maxWidth: 300 }}>
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
              UI Zoom: {zoomLevel}%
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
              {resolution === '1080p'
                ? 'Increase zoom if text is too small on your display.'
                : 'Higher resolutions require a powerful GPU. Increase zoom if text is too small.'}
            </Typography>
            <Slider
                value={zoomLevel}
                min={100}
                max={300}
                step={100}
                size="small"
                marks={[
                  { value: 100, label: '100%' },
                  { value: 200, label: '200%' },
                  { value: 300, label: '300%' },
                ]}
                onChange={(_, value) => {
                  const newZoom = value as number;
                  setZoomLevel(newZoom);
                  const updatedConfig = { ...external_agent_config, zoom_level: newZoom };
                  setExternalAgentConfig(updatedConfig);
                  onUpdate({ external_agent_config: updatedConfig });
                }}
                disabled={readOnly}
                sx={{
                  ml: 1, // Prevent thumb from being clipped at 100%
                  mr: 1, // Prevent thumb from being clipped at 300%
                  width: 'calc(100% - 16px)',
                  '& .MuiSlider-markLabel[data-index="0"]': {
                    transform: 'translateX(0%)',
                  },
                  '& .MuiSlider-markLabel[data-index="2"]': {
                    transform: 'translateX(-100%)',
                  },
                }}
              />
            </Box>
        </Box>
      )}

        {/* Multi-Turn Agent Configuration */}
        {default_agent_type === AGENT_TYPE_HELIX_AGENT && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1" sx={{ mb: 2 }}>Multi-Turn Agent Configuration</Typography>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Main Reasoning Model (tool calling)</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                The model used for reasoning and tool calling tasks. Adjust reasoning effort based on complexity of the task.
                You will need the strongest model for this, must support tool use.
              </Typography>
              <Stack direction="row" spacing={2} alignItems="flex-start">
                <AdvancedModelPicker
                  recommendedModels={RECOMMENDED_MODELS.reasoning}
                  hint='Recommended to use o3-mini level models, should be a strong model capable of using tools and reasoning.'
                  selectedProvider={reasoning_model_provider}
                  selectedModelId={reasoning_model}
                  onSelectModel={(provider, model) => {
                    setReasoningModel(model);
                    setReasoningModelProvider(provider);
                    onUpdate({
                      reasoning_model: model,
                      reasoning_model_provider: provider,
                    });
                  }}
                  currentType="text"
                  displayMode="short"
                />
                <Box>
                  <Tooltip
                    title={getEffortTooltip(reasoning_model_effort)}
                    placement="top"
                    arrow
                  >
                    <span>
                      <Button
                        variant="text"
                        onClick={handleMainEffortClick}
                        endIcon={<ArrowDropDownIcon />}
                        disabled={readOnly}
                        sx={{
                          borderRadius: '8px',
                          color: 'text.primary',
                          textTransform: 'none',
                          fontSize: '0.875rem',
                          padding: '4px 8px',
                          height: '32px',
                          minWidth: 'auto',
                          maxWidth: '120px',
                          display: 'flex',
                          alignItems: 'center',
                          border: '1px solid #fff',
                          '&:hover': {
                            backgroundColor: (theme) => theme.palette.mode === 'light' ? "#efefef" : "#13132b",
                          },
                          ...(readOnly && {
                            opacity: 0.5,
                            pointerEvents: 'none',
                          }),
                        }}
                      >
                        <Typography
                          variant="caption"
                          component="span"
                          sx={{
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            display: 'inline-block',
                            lineHeight: 1.2,
                            verticalAlign: 'middle',
                            fontSize: '0.875rem',
                          }}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            <BarsIcon effort={reasoning_model_effort} />
                            <Typography>
                              {reasoning_model_effort.charAt(0).toUpperCase() + reasoning_model_effort.slice(1)}
                            </Typography>
                          </Stack>
                        </Typography>
                      </Button>
                    </span>
                  </Tooltip>
                  <Menu
                    anchorEl={mainEffortMenuAnchor}
                    open={Boolean(mainEffortMenuAnchor)}
                    onClose={handleMainEffortClose}
                  >
                    {[
                      { value: 'none', tooltip: 'Reasoning disabled - no additional reasoning steps will be performed' },
                      { value: 'low', tooltip: 'Low reasoning effort - minimal reasoning steps, faster responses but may be less thorough' },
                      { value: 'medium', tooltip: 'Medium reasoning effort - balanced reasoning steps, good balance of speed and thoroughness' },
                      { value: 'high', tooltip: 'High reasoning effort - extensive reasoning steps, most thorough but may be slower' }
                    ].map(({ value, tooltip }) => (
                      <Tooltip
                        key={value}
                        title={tooltip}
                        placement="right"
                        arrow
                      >
                        <MenuItem
                          onClick={() => handleEffortSelect(value, true)}
                          selected={value === reasoning_model_effort}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            <BarsIcon effort={value} />
                            <Typography>
                              {value.charAt(0).toUpperCase() + value.slice(1)}
                            </Typography>
                          </Stack>
                        </MenuItem>
                      </Tooltip>
                    ))}
                  </Menu>
                </Box>
              </Stack>
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Generation Model (planning next actions)</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                The model used for generating responses. Recommended to use gpt-4o level models. Must support tool use.
              </Typography>
              <AdvancedModelPicker
                recommendedModels={RECOMMENDED_MODELS.generation}
                hint='Recommended to use gpt-4o level models, should be a strong model capable of planning next actions and interpreting tool responses.'
                selectedProvider={generation_model_provider}
                selectedModelId={generation_model}
                onSelectModel={(provider, model) => {
                  setGenerationModel(model);
                  setGenerationModelProvider(provider);
                  onUpdate({
                    generation_model: model,
                    generation_model_provider: provider,
                  });
                }}
                currentType="text"
                displayMode="short"
              />
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Small Reasoning Model (tool results interpretation)</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                A smaller model used for quick reasoning tasks and interpreting tool responses.
                If model doesn't support reasoning, set reasoning effort to none. Tool use is recommended but not required.
              </Typography>
              <Stack direction="row" spacing={2} alignItems="flex-start">
                <AdvancedModelPicker
                  recommendedModels={RECOMMENDED_MODELS.smallReasoning}
                  hint='Recommended to use o3-mini level models, should be a strong model capable of using tools and reasoning.'
                  selectedProvider={small_reasoning_model_provider}
                  selectedModelId={small_reasoning_model}
                  onSelectModel={(provider, model) => {
                    setSmallReasoningModel(model);
                    setSmallReasoningModelProvider(provider);
                    onUpdate({
                      small_reasoning_model: model,
                      small_reasoning_model_provider: provider,
                    });
                  }}
                  currentType="text"
                  displayMode="short"
                />
                <Box>
                  <Tooltip
                    title={getEffortTooltip(small_reasoning_model_effort)}
                    placement="top"
                    arrow
                  >
                    <span>
                      <Button
                        variant="text"
                        onClick={handleSmallEffortClick}
                        endIcon={<ArrowDropDownIcon />}
                        disabled={readOnly}
                        sx={{
                          borderRadius: '8px',
                          color: 'text.primary',
                          textTransform: 'none',
                          fontSize: '0.875rem',
                          padding: '4px 8px',
                          height: '32px',
                          minWidth: 'auto',
                          maxWidth: '120px',
                          display: 'flex',
                          alignItems: 'center',
                          border: '1px solid #fff',
                          '&:hover': {
                            backgroundColor: (theme) => theme.palette.mode === 'light' ? "#efefef" : "#13132b",
                          },
                          ...(readOnly && {
                            opacity: 0.5,
                            pointerEvents: 'none',
                          }),
                        }}
                      >
                        <Typography
                          variant="caption"
                          component="span"
                          sx={{
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            display: 'inline-block',
                            lineHeight: 1.2,
                            verticalAlign: 'middle',
                            fontSize: '0.875rem',
                          }}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            <BarsIcon effort={small_reasoning_model_effort} />
                            <Typography>
                              {small_reasoning_model_effort.charAt(0).toUpperCase() + small_reasoning_model_effort.slice(1)}
                            </Typography>
                          </Stack>
                        </Typography>
                      </Button>
                    </span>
                  </Tooltip>
                  <Menu
                    anchorEl={smallEffortMenuAnchor}
                    open={Boolean(smallEffortMenuAnchor)}
                    onClose={handleSmallEffortClose}
                  >
                    {[
                      { value: 'none', tooltip: 'Reasoning disabled - no additional reasoning steps will be performed' },
                      { value: 'low', tooltip: 'Low effort - minimal reasoning steps, faster responses but may be less thorough' },
                      { value: 'medium', tooltip: 'Medium effort - balanced reasoning steps, good balance of speed and thoroughness' },
                      { value: 'high', tooltip: 'High effort - extensive reasoning steps, most thorough but may be slower' }
                    ].map(({ value, tooltip }) => (
                      <Tooltip
                        key={value}
                        title={tooltip}
                        placement="right"
                        arrow
                      >
                        <MenuItem
                          onClick={() => handleEffortSelect(value, false)}
                          selected={value === small_reasoning_model_effort}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            <BarsIcon effort={value} />
                            <Typography>
                              {value.charAt(0).toUpperCase() + value.slice(1)}
                            </Typography>
                          </Stack>
                        </MenuItem>
                      </Tooltip>
                    ))}
                  </Menu>
                </Box>
              </Stack>
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Small Generation Model</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                A smaller model used for quick response generation. Recommended to use gpt-4o-mini level models.
              </Typography>
              <AdvancedModelPicker
                recommendedModels={RECOMMENDED_MODELS.smallGeneration}
                hint='Recommended to use gpt-4o level models, should be a strong model capable of planning next actions and interpreting tool responses.'
                selectedProvider={provider}
                selectedModelId={small_generation_model}
                onSelectModel={(provider, model) => {
                  setSmallGenerationModel(model);
                  setSmallGenerationModelProvider(provider);
                  onUpdate({
                    small_generation_model: model,
                    small_generation_model_provider: provider,
                  });
                }}
                currentType="text"
                displayMode="short"
              />
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Max Reasoning Iterations (per task)</Typography>
              <Box sx={{ mb: 1 }}>
                <Typography variant="body2" color="text.secondary" component="div">
                  The maximum number of reasoning iterations allowed for each reasoning task. This limit applies separately to:
                </Typography>
                <Box component="ul" sx={{ pl: 2, mt: 0.5, mb: 0.5 }}>
                  <li>Global reasoning (overall conversation planning)</li>
                  <li>Each individual skill's reasoning. Includes preparation and execution of the skill.</li>
                </Box>
                <Typography variant="body2" color="text.secondary" component="div">
                  For example, with 3 skills and max iterations set to 10, you could have up to 40 total iterations (10 for global + 10 for each skill).
                  This acts as a safety mechanism to prevent infinite loops.
                  <ResetLink field="max_iterations" value={max_iterations} onClick={() => handleReset('max_iterations')} />
                </Typography>
              </Box>

              <TextField
                sx={{ mt: 1 }}
                type="number"
                value={max_iterations}
                onChange={(e) => {
                  const value = parseInt(e.target.value) || DEFAULT_VALUES.max_iterations;
                  setMaxIterations(value);
                  debouncedUpdate('maxIterations', value);
                }}
                fullWidth
                disabled={readOnly}
                inputProps={{ min: 1 }}
              />
            </Box>
          </Box>
        )}
      </Box>

    </Box>
  )
}

export default AppSettings
