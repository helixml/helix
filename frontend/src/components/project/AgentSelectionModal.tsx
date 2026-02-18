import React, { FC, useState, useEffect, useContext, useMemo, useRef } from 'react'
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
  Alert,
  IconButton,
  Tooltip,
} from '@mui/material'
import { Bot } from 'lucide-react'
import AddIcon from '@mui/icons-material/Add'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import EditIcon from '@mui/icons-material/Edit'

import useAccount from '../../hooks/useAccount'

import { AppsContext, CodeAgentRuntime, generateAgentName } from '../../contexts/apps'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useListProviders } from '../../services/providersService'
import { TypesProviderEndpointType } from '../../api/api'
import CodingAgentForm, { CodingAgentFormHandle } from '../agent/CodingAgentForm'

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
  const { apps, loadApps } = useContext(AppsContext)
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const codingAgentFormRef = useRef<CodingAgentFormHandle>(null)

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
    const createdAgent = await codingAgentFormRef.current?.handleCreateAgent()
    if (!createdAgent?.id) return
    setSelectedAgentId(createdAgent.id)
    setShowCreateForm(false)
    onSelect(createdAgent.id)
    onClose()
  }

  const handleClose = () => {
    setSelectedAgentId(null)
    setShowCreateForm(false)
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

            <CodingAgentForm
              ref={codingAgentFormRef}
              value={{
                codeAgentRuntime,
                claudeCodeMode,
                selectedProvider,
                selectedModel,
                agentName: newAgentName,
              }}
              onChange={(nextValue) => {
                setCodeAgentRuntime(nextValue.codeAgentRuntime)
                setClaudeCodeMode(nextValue.claudeCodeMode)
                setSelectedProvider(nextValue.selectedProvider)
                setSelectedModel(nextValue.selectedModel)
                if (nextValue.agentName !== newAgentName) {
                  setUserModifiedName(true)
                }
                setNewAgentName(nextValue.agentName)
              }}
              disabled={isCreating}
              hasClaudeSubscription={hasClaudeSubscription}
              hasAnthropicProvider={hasAnthropicProvider}
              recommendedModels={RECOMMENDED_MODELS}
              createAgentDescription="Code development agent for spec tasks"
              onCreateStateChange={setIsCreating}
              showCreateButton={false}
              modelPickerHint="Choose a capable model for agentic coding. Recommended models appear at the top of the list."
              modelPickerDisplayMode="short"
              showMcpHint
              sx={{ mb: 2 }}
            />

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
            disabled={
              isCreating ||
              !newAgentName.trim() ||
              (!(
                codeAgentRuntime === 'claude_code' &&
                claudeCodeMode === 'subscription'
              ) &&
                (!selectedModel || !selectedProvider))
            }
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
