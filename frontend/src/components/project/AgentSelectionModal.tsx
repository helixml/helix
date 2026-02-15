import React, { FC, useState, useEffect, useContext, useMemo } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Avatar,
  CircularProgress,
  Divider,
  TextField,
  Alert,
  FormControl,
  Select,
  MenuItem,
  IconButton,
  Tooltip,
  Radio,
  RadioGroup,
  FormControlLabel,
} from '@mui/material'
import { Bot } from 'lucide-react'
import AddIcon from '@mui/icons-material/Add'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import EditIcon from '@mui/icons-material/Edit'

import useAccount from '../../hooks/useAccount'

import { AppsContext, ICreateAgentParams, CodeAgentRuntime, generateAgentName, CODE_AGENT_RUNTIME_DISPLAY_NAMES } from '../../contexts/apps'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useListProviders } from '../../services/providersService'
import { TypesProviderEndpointType } from '../../api/api'

// Recommended models for zed_external agents (state-of-the-art coding models)
const RECOMMENDED_MODELS = [
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

interface AgentSelectionModalProps {
  open: boolean
  onClose: () => void
  onSelect: (agentId: string) => void
  title?: string
  description?: string
}

const AgentSelectionModal: FC<AgentSelectionModalProps> = ({
  open,
  onClose,
  onSelect,
  title = 'Select Agent',
  description = 'Choose a default agent for this project. You can override this when creating individual tasks.',
}) => {
  const account = useAccount()
  const { apps, loadApps, createAgent } = useContext(AppsContext)
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [createError, setCreateError] = useState<string>('')

  // Claude subscription + provider state
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const { data: providerEndpoints } = useListProviders({ loadModels: false })
  const hasAnthropicProvider = useMemo(() => {
    if (!providerEndpoints) return false
    return providerEndpoints.some(p => p.endpoint_type === TypesProviderEndpointType.ProviderEndpointTypeUser && p.name === 'anthropic')
  }, [providerEndpoints])
  const userProviderCount = useMemo(() => {
    if (!providerEndpoints) return 0
    return providerEndpoints.filter(p => p.endpoint_type === TypesProviderEndpointType.ProviderEndpointTypeUser).length
  }, [providerEndpoints])

  // Create agent form state
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [claudeCodeMode, setClaudeCodeMode] = useState<'subscription' | 'api_key'>('subscription')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [newAgentName, setNewAgentName] = useState('-')
  const [userModifiedName, setUserModifiedName] = useState(false)
  const [isCreating, setIsCreating] = useState(false)

  // Auto-generate name when model or runtime changes (if user hasn't modified it)
  useEffect(() => {
    if (!userModifiedName) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName])

  // Load apps when modal opens
  useEffect(() => {
    if (open) {
      loadApps()
    }
  }, [open, loadApps])

  // Sort apps: zed_external first, then others
  const sortedApps = useMemo(() => {
    if (!apps) return []

    const zedExternalApps: IApp[] = []
    const otherApps: IApp[] = []

    apps.forEach((app) => {
      // Check if app has zed_external assistant type
      const hasZedExternal = app.config?.helix?.assistants?.some(
        (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL
      ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL

      if (hasZedExternal) {
        zedExternalApps.push(app)
      } else {
        otherApps.push(app)
      }
    })

    return [...zedExternalApps, ...otherApps]
  }, [apps])

  // Auto-select first zed_external agent if available
  useEffect(() => {
    if (open && sortedApps.length > 0 && !selectedAgentId) {
      setSelectedAgentId(sortedApps[0].id)
    }
  }, [open, sortedApps, selectedAgentId])

  // Show create form if no apps exist
  useEffect(() => {
    if (open && apps && apps.length === 0) {
      setShowCreateForm(true)
    }
  }, [open, apps])

  // Auto-default to Claude Code when it's the only available AI provider
  useEffect(() => {
    if (hasClaudeSubscription && !hasAnthropicProvider && userProviderCount === 0) {
      setCodeAgentRuntime('claude_code')
      setClaudeCodeMode('subscription')
    }
  }, [hasClaudeSubscription, hasAnthropicProvider, userProviderCount])

  const handleSelect = () => {
    if (selectedAgentId) {
      onSelect(selectedAgentId)
      onClose()
    }
  }

  const handleCreateAgent = async () => {
    if (!newAgentName.trim()) {
      setCreateError('Please enter a name for the agent')
      return
    }

    if (!selectedModel && !(codeAgentRuntime === 'claude_code' && claudeCodeMode === 'subscription')) {
      setCreateError('Please select a model')
      return
    }

    setIsCreating(true)
    setCreateError('')

    try {
      const isClaudeCodeSub = codeAgentRuntime === 'claude_code' && claudeCodeMode === 'subscription'
      const params: ICreateAgentParams = {
        name: newAgentName.trim(),
        description: 'Code development agent for spec tasks',
        agentType: AGENT_TYPE_ZED_EXTERNAL,
        codeAgentRuntime,
        model: isClaudeCodeSub ? '' : selectedModel,
        // For zed_external, the generation model is what matters (that's what Zed uses)
        generationModelProvider: isClaudeCodeSub ? '' : selectedProvider,
        generationModel: isClaudeCodeSub ? '' : selectedModel,
        // Set reasonable defaults for other model settings
        reasoningModelProvider: '',
        reasoningModel: '',
        reasoningModelEffort: 'none',
        smallReasoningModelProvider: '',
        smallReasoningModel: '',
        smallReasoningModelEffort: 'none',
        smallGenerationModelProvider: '',
        smallGenerationModel: '',
      }

      const newApp = await createAgent(params)

      if (newApp) {
        setSelectedAgentId(newApp.id)
        setShowCreateForm(false)
        // Proceed with the selection
        onSelect(newApp.id)
        onClose()
      }
    } catch (err) {
      console.error('Failed to create agent:', err)
      setCreateError(err instanceof Error ? err.message : 'Failed to create agent')
    } finally {
      setIsCreating(false)
    }
  }

  const handleClose = () => {
    setSelectedAgentId(null)
    setShowCreateForm(false)
    setCreateError('')
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {description}
        </Typography>

        {!showCreateForm ? (
          <>
            {/* Existing agents list */}
            {sortedApps.length > 0 ? (
              <List sx={{ pt: 0 }}>
                {sortedApps.map((app) => {
                  const isSelected = selectedAgentId === app.id

                  return (
                    <ListItem key={app.id} disablePadding>
                      <ListItemButton
                        selected={isSelected}
                        onClick={() => setSelectedAgentId(app.id)}
                        sx={{
                          borderRadius: 1,
                          mb: 0.5,
                          border: isSelected ? '2px solid' : '1px solid',
                          borderColor: isSelected ? 'primary.main' : 'divider',
                          pr: 10, // Make room for edit and check icons
                        }}
                      >
                        <ListItemIcon>
                          <Avatar
                            src={app.config?.helix?.avatar}
                            sx={{ width: 40, height: 40 }}
                          >
                            <Bot size={24} />
                          </Avatar>
                        </ListItemIcon>
                        <ListItemText
                          primary={app.config?.helix?.name || 'Unnamed Agent'}
                          secondary={app.config?.helix?.description || 'No description'}
                        />
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                          <Tooltip title="Edit agent">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation()
                                account.orgNavigate('app', { app_id: app.id })
                              }}
                            >
                              <EditIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                          {isSelected && (
                            <CheckCircleIcon color="primary" />
                          )}
                        </Box>
                      </ListItemButton>
                    </ListItem>
                  )
                })}
              </List>
            ) : (
              <Alert severity="info" sx={{ mb: 2 }}>
                No agents found. Create one to get started.
              </Alert>
            )}

            <Divider sx={{ my: 2 }} />

            {/* Create new agent button */}
            <Button
              startIcon={<AddIcon />}
              onClick={() => setShowCreateForm(true)}
              fullWidth
              variant="outlined"
            >
              Create New Agent
            </Button>
          </>
        ) : (
          <>
            {/* Create agent form */}
            <Typography variant="subtitle2" sx={{ mb: 2 }}>
              Create New Agent
            </Typography>

            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Code Agent Runtime
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
              Choose which code agent runtime to use inside Zed.
            </Typography>
            <FormControl fullWidth sx={{ mb: 2 }}>
              <Select
                value={codeAgentRuntime}
                onChange={(e) => setCodeAgentRuntime(e.target.value as CodeAgentRuntime)}
                disabled={isCreating}
                size="small"
              >
                <MenuItem value="zed_agent">
                  <Box>
                    <Typography variant="body2">Zed Agent (Built-in)</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Uses Zed's native agent panel with direct API integration
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
                <MenuItem value="claude_code">
                  <Box>
                    <Typography variant="body2">Claude Code</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Anthropic's coding agent — works with Claude subscriptions
                    </Typography>
                  </Box>
                </MenuItem>
              </Select>
            </FormControl>

            {codeAgentRuntime === 'claude_code' && (
              <Box sx={{ p: 1.5, mb: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
                  Credentials
                </Typography>
                <FormControl>
                  <RadioGroup
                    value={claudeCodeMode}
                    onChange={(e) => {
                      const mode = e.target.value as 'subscription' | 'api_key'
                      setClaudeCodeMode(mode)
                      if (mode === 'subscription') {
                        setSelectedProvider('')
                        setSelectedModel('')
                      }
                    }}
                  >
                    <FormControlLabel
                      value="subscription"
                      control={<Radio size="small" />}
                      disabled={!hasClaudeSubscription}
                      label={
                        <Typography variant="body2">
                          Claude Subscription{hasClaudeSubscription ? ' (connected)' : ' (not connected)'}
                        </Typography>
                      }
                    />
                    <FormControlLabel
                      value="api_key"
                      control={<Radio size="small" />}
                      disabled={!hasAnthropicProvider}
                      label={
                        <Typography variant="body2">
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

            {(codeAgentRuntime !== 'claude_code' || claudeCodeMode === 'api_key') && (
              <>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  Code Agent Model
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                  Choose a model for code generation.
                </Typography>

                <Box sx={{ mb: 2 }}>
                  <AdvancedModelPicker
                    recommendedModels={RECOMMENDED_MODELS}
                    hint="Choose a capable model for agentic coding. Recommended models appear at the top of the list."
                    selectedProvider={selectedProvider}
                    selectedModelId={selectedModel}
                    onSelectModel={(provider, model) => {
                      setSelectedProvider(provider)
                      setSelectedModel(model)
                    }}
                    currentType="text"
                    displayMode="short"
                    disabled={isCreating}
                  />
                </Box>
              </>
            )}

            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Agent Name
            </Typography>
            <TextField
              value={newAgentName}
              onChange={(e) => {
                setNewAgentName(e.target.value)
                setUserModifiedName(true)
              }}
              fullWidth
              size="small"
              sx={{ mb: 2 }}
              disabled={isCreating}
              helperText="Auto-generated from model and runtime. Edit to customize."
            />

            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
              You can configure MCP servers in the agent settings after creation.
            </Typography>

            {createError && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {createError}
              </Alert>
            )}

            {sortedApps.length > 0 && (
              <Button
                onClick={() => setShowCreateForm(false)}
                sx={{ mt: 1 }}
                disabled={isCreating}
              >
                Back to Agent List
              </Button>
            )}
          </>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={handleClose} disabled={isCreating}>
          Cancel
        </Button>
        {showCreateForm ? (
          <Button
            onClick={handleCreateAgent}
            variant="contained"
            disabled={isCreating || !newAgentName.trim() || (!selectedModel && !(codeAgentRuntime === 'claude_code' && claudeCodeMode === 'subscription'))}
            startIcon={isCreating ? <CircularProgress size={16} /> : undefined}
          >
            {isCreating ? 'Creating...' : 'Create & Continue'}
          </Button>
        ) : (
          <Button
            onClick={handleSelect}
            variant="contained"
            disabled={!selectedAgentId}
          >
            Continue
          </Button>
        )}
      </DialogActions>
    </Dialog>
  )
}

export default AgentSelectionModal
