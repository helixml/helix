import React, { forwardRef, useImperativeHandle, useState } from 'react'
import { Alert, Box, Button, CircularProgress, FormControl, FormControlLabel, MenuItem, Radio, RadioGroup, Select, TextField, Typography } from '@mui/material'
import { SxProps, Theme } from '@mui/material/styles'

import { CodeAgentRuntime, ICreateAgentParams } from '../../contexts/apps'
import useApps from '../../hooks/useApps'
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from '../../types'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'

export type ClaudeCodeMode = 'subscription' | 'api_key'

export interface CodingAgentFormValue {
  codeAgentRuntime: CodeAgentRuntime
  claudeCodeMode: ClaudeCodeMode
  selectedProvider: string
  selectedModel: string
  agentName: string
}

export interface CodingAgentFormHandle {
  handleCreateAgent: () => Promise<IApp | null>
}

interface CodingAgentFormProps {
  value: CodingAgentFormValue
  onChange: (value: CodingAgentFormValue) => void
  disabled?: boolean
  hasClaudeSubscription?: boolean
  hasAnthropicProvider?: boolean
  recommendedModels: string[]
  modelPickerHint?: string
  modelPickerDisplayMode?: 'short' | 'full'
  modelPickerAutoSelectFirst?: boolean
  onModelSelect?: () => void
  showMcpHint?: boolean
  runtimeLabel?: string
  runtimeDescription?: string
  modelLabel?: string
  agentNameLabel?: string
  agentNameHelperText?: string
  showClaudeCodeOption?: boolean
  sx?: SxProps<Theme>
  labelSx?: SxProps<Theme>
  captionSx?: SxProps<Theme>
  selectSx?: SxProps<Theme>
  menuPaperSx?: SxProps<Theme>
  textFieldInputSx?: SxProps<Theme>
  textFieldLabelSx?: SxProps<Theme>
  textFieldHelperSx?: SxProps<Theme>
  claudeCredentialsBoxSx?: SxProps<Theme>
  claudeRadioSx?: SxProps<Theme>
  createAgentDescription?: string
  createAgentOrganizationId?: string
  onCreateStateChange?: (isCreating: boolean) => void
  onAgentCreated?: (app: IApp) => void
  showCreateButton?: boolean
  createButtonLabel?: string
  createButtonVariant?: 'text' | 'outlined' | 'contained'
  createButtonColor?: 'inherit' | 'primary' | 'secondary' | 'success' | 'error' | 'info' | 'warning'
  createButtonSx?: SxProps<Theme>
}

const defaultRuntimeDescription = 'Choose which code agent runtime to use inside Zed.'
const defaultModelLabel = 'Code Agent Model'
const defaultAgentNameLabel = 'Agent Name'
const defaultAgentNameHelper = 'Auto-generated from model and runtime. Edit to customize.'
const DEFAULT_CLAUDE_AGENT_PROVIDER = 'anthropic'

const CodingAgentForm = forwardRef<CodingAgentFormHandle, CodingAgentFormProps>(function CodingAgentForm({
  value,
  onChange,
  disabled = false,
  hasClaudeSubscription = false,
  hasAnthropicProvider = false,
  recommendedModels,
  modelPickerHint = 'Choose a capable model for agentic coding.',
  modelPickerDisplayMode = 'short',
  modelPickerAutoSelectFirst,
  onModelSelect,
  showMcpHint = false,
  runtimeLabel = 'Code Agent Runtime',
  runtimeDescription = defaultRuntimeDescription,
  modelLabel = defaultModelLabel,
  agentNameLabel = defaultAgentNameLabel,
  agentNameHelperText = defaultAgentNameHelper,
  showClaudeCodeOption = true,
  sx,
  labelSx,
  captionSx,
  selectSx,
  menuPaperSx,
  textFieldInputSx,
  textFieldLabelSx,
  textFieldHelperSx,
  claudeCredentialsBoxSx,
  claudeRadioSx,
  createAgentDescription = 'Code development agent for spec tasks',
  createAgentOrganizationId,
  onCreateStateChange,
  onAgentCreated,
  showCreateButton = true,
  createButtonLabel = 'Create Agent',
  createButtonVariant = 'outlined',
  createButtonColor = 'secondary',
  createButtonSx,
}: CodingAgentFormProps, ref) {
  const apps = useApps()
  const [createError, setCreateError] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const showModelPicker = value.codeAgentRuntime !== 'claude_code' || value.claudeCodeMode === 'api_key'
  const isClaudeCodeSubscription = value.codeAgentRuntime === 'claude_code' && value.claudeCodeMode === 'subscription'

  const handleCreateAgent = async (): Promise<IApp | null> => {
    if (!value.agentName.trim()) {
      setCreateError('Please enter a name for the agent')
      return null
    }

    if (!isClaudeCodeSubscription && (!value.selectedModel || !value.selectedProvider)) {
      setCreateError('Please select both provider and model')
      return null
    }

    const modelToUse = isClaudeCodeSubscription ? '' : (value.selectedModel || '')
    const providerToUse = isClaudeCodeSubscription ? '' : (value.selectedProvider || '')

    if (!isClaudeCodeSubscription && (!modelToUse || !providerToUse)) {
      setCreateError('Please select both provider and model')
      return null
    }

    onCreateStateChange?.(true)
    setIsCreating(true)
    setCreateError('')

    try {
      const params: ICreateAgentParams = {
        name: value.agentName.trim(),
        description: createAgentDescription,
        agentType: AGENT_TYPE_ZED_EXTERNAL,
        codeAgentRuntime: value.codeAgentRuntime,
        codeAgentCredentialType: value.claudeCodeMode === 'subscription' ? 'subscription' : 'api_key',
        provider: providerToUse,
        model: modelToUse,
        organizationId: createAgentOrganizationId,
        generationModelProvider: '',
        generationModel: '',
        reasoningModelProvider: '',
        reasoningModel: '',
        reasoningModelEffort: 'none',
        smallReasoningModelProvider: '',
        smallReasoningModel: '',
        smallReasoningModelEffort: 'none',
        smallGenerationModelProvider: '',
        smallGenerationModel: '',
      }

      const newApp = await apps.createAgent(params)
      if (!newApp) {
        setCreateError('Failed to create agent')
        return null
      }
      onAgentCreated?.(newApp)
      return newApp
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create agent')
      return null
    } finally {
      setIsCreating(false)
      onCreateStateChange?.(false)
    }
  }

  useImperativeHandle(ref, () => ({
    handleCreateAgent,
  }), [
    handleCreateAgent,
    apps,
    createAgentDescription,
    createAgentOrganizationId,
    isClaudeCodeSubscription,
    onAgentCreated,
    onCreateStateChange,
    value.agentName,
    value.claudeCodeMode,
    value.codeAgentRuntime,
    value.selectedModel,
    value.selectedProvider,
  ])
  const canCreateAgent = !!value.agentName.trim() && (isClaudeCodeSubscription || (!!value.selectedModel && !!value.selectedProvider))
  const createButtonDisabled = disabled || isCreating || !canCreateAgent

  return (
    <Box sx={sx}>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ...labelSx }}>
        {runtimeLabel}
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block', ...captionSx }}>
        {runtimeDescription}
      </Typography>
      <FormControl fullWidth sx={{ mb: 2 }}>
        <Select
          value={value.codeAgentRuntime}
          onChange={(event) => {
            onChange({
              ...value,
              codeAgentRuntime: event.target.value as CodeAgentRuntime,
            })
          }}
          disabled={disabled}
          size="small"
          sx={selectSx}
          MenuProps={{
            PaperProps: {
              sx: menuPaperSx,
            },
          }}
        >
          <MenuItem value="zed_agent">
            <Box>
              <Typography variant="body2">Zed Agent (Built-in)</Typography>
              <Typography variant="caption" color="text.secondary">
                Uses Zed&apos;s native agent panel with direct API integration
              </Typography>
            </Box>
          </MenuItem>
          <MenuItem value="qwen_code">
            <Box>
              <Typography variant="body2">Qwen Code</Typography>
              <Typography variant="caption" color="text.secondary">
                Uses qwen-code CLI as a custom agent server (OpenAI-compatible)
              </Typography>
            </Box>
          </MenuItem>
          {showClaudeCodeOption && (
            <MenuItem value="claude_code">
              <Box>
                <Typography variant="body2">Claude Code</Typography>
                <Typography variant="caption" color="text.secondary">
                  Anthropic&apos;s coding agent — works with Claude subscriptions
                </Typography>
              </Box>
            </MenuItem>
          )}
        </Select>
      </FormControl>

      {value.codeAgentRuntime === 'claude_code' && (
        <Box sx={{ p: 1.5, mb: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider', ...claudeCredentialsBoxSx }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5, ...labelSx }}>
            Credentials
          </Typography>
          <FormControl>
            <RadioGroup
              value={value.claudeCodeMode}
              onChange={(event) => {
                const nextMode = event.target.value as ClaudeCodeMode
                onChange({
                  ...value,
                  claudeCodeMode: nextMode,
                  selectedProvider: nextMode === 'subscription' ? '' : value.selectedProvider,
                  selectedModel: nextMode === 'subscription' ? '' : value.selectedModel,
                })
              }}
            >
              <FormControlLabel
                value="subscription"
                control={<Radio size="small" sx={claudeRadioSx} />}
                disabled={!hasClaudeSubscription}
                label={
                  <Typography variant="body2" color="inherit">
                    Claude Subscription{hasClaudeSubscription ? ' (connected)' : ' (not connected)'}
                  </Typography>
                }
              />
              <FormControlLabel
                value="api_key"
                control={<Radio size="small" sx={claudeRadioSx} />}
                disabled={!hasAnthropicProvider}
                label={
                  <Typography variant="body2" color="inherit">
                    Anthropic API Key{hasAnthropicProvider ? ' (configured)' : ' (not configured)'}
                  </Typography>
                }
              />
            </RadioGroup>
          </FormControl>
          {!hasClaudeSubscription && !hasAnthropicProvider && (
            <Alert severity="warning" sx={{ mt: 1 }}>
              Connect a Claude subscription or add an Anthropic API key in Providers.
            </Alert>
          )}
        </Box>
      )}

      {showModelPicker && (
        <>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ...labelSx }}>
            {modelLabel}
          </Typography>
          <Box sx={{ mb: 2 }}>
            <AdvancedModelPicker
              recommendedModels={recommendedModels}
              autoSelectFirst={modelPickerAutoSelectFirst}
              hint={modelPickerHint}
              selectedProvider={value.selectedProvider}
              selectedModelId={value.selectedModel}
              onSelectModel={(provider, model) => {
                onModelSelect?.()
                onChange({
                  ...value,
                  selectedProvider: provider,
                  selectedModel: model,
                })
              }}
              currentType="text"
              displayMode={modelPickerDisplayMode}
              disabled={disabled}
            />
          </Box>
        </>
      )}

      <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ...labelSx }}>
        {agentNameLabel}
      </Typography>
      <TextField
        value={value.agentName}
        onChange={(event) => {
          onChange({
            ...value,
            agentName: event.target.value,
          })
        }}
        fullWidth
        size="small"
        disabled={disabled}
        helperText={agentNameHelperText}
        InputProps={{ sx: textFieldInputSx }}
        InputLabelProps={{ sx: textFieldLabelSx }}
        FormHelperTextProps={{ sx: textFieldHelperSx }}
      />
      {showCreateButton && (
        <Box sx={{ mt: 2 }}>
          <Button
            variant={createButtonVariant}
            color={createButtonColor}
            onClick={handleCreateAgent}
            disabled={createButtonDisabled}
            startIcon={isCreating ? <CircularProgress size={16} /> : undefined}
            sx={createButtonSx}
          >
            {isCreating ? 'Creating...' : createButtonLabel}
          </Button>
        </Box>
      )}

      {showMcpHint && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1, ...captionSx }}>
          You can configure MCP servers in the agent settings after creation.
        </Typography>
      )}
      {createError && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {createError}
        </Alert>
      )}
    </Box>
  )
})

export default CodingAgentForm
