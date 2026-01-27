import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Box,
  Button,
  Typography,
  Alert,
  Chip,
  Stack,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  CircularProgress,
  Checkbox,
  FormControlLabel,
  Tooltip,
  IconButton,
} from '@mui/material'
import {
  Add as AddIcon,
  Close as CloseIcon,
} from '@mui/icons-material'
import { X } from 'lucide-react'

import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { CodeAgentRuntime, generateAgentName, ICreateAgentParams } from '../../contexts/apps'
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from '../../types'
import { TypesCreateTaskRequest, TypesSpecTaskPriority, TypesBranchMode, TypesSpecTask } from '../../api/api'
import AgentDropdown from '../agent/AgentDropdown'

import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useApps from '../../hooks/useApps'
import { useGetProject, useGetProjectRepositories } from '../../services'

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

interface NewSpecTaskFormProps {
  projectId: string
  onTaskCreated: (task: TypesSpecTask) => void
  onClose?: () => void
  showHeader?: boolean // Show header with close button (for panel mode)
  embedded?: boolean // Embedded in tab (affects styling)
}

const NewSpecTaskForm: React.FC<NewSpecTaskFormProps> = ({
  projectId,
  onTaskCreated,
  onClose,
  showHeader = true,
  embedded = false,
}) => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const apps = useApps()

  // Fetch project data
  const { data: project } = useGetProject(projectId, !!projectId)
  const { data: projectRepositories = [] } = useGetProjectRepositories(projectId, !!projectId)

  // Form state
  const [taskPrompt, setTaskPrompt] = useState('')
  const [taskPriority, setTaskPriority] = useState('medium')
  const [selectedHelixAgent, setSelectedHelixAgent] = useState('')
  const [justDoItMode, setJustDoItMode] = useState(false)
  const [useHostDocker, setUseHostDocker] = useState(false)
  const [isCreating, setIsCreating] = useState(false)

  // Branch configuration state
  const [branchMode, setBranchMode] = useState<TypesBranchMode>(TypesBranchMode.BranchModeNew)
  const [baseBranch, setBaseBranch] = useState('')
  const [branchPrefix, setBranchPrefix] = useState('')
  const [workingBranch, setWorkingBranch] = useState('')
  const [showBranchCustomization, setShowBranchCustomization] = useState(false)

  // Get the default repository ID from the project
  const defaultRepoId = project?.default_repo_id

  // Fetch branches for the default repository
  const { data: branchesData } = useQuery({
    queryKey: ['repository-branches', defaultRepoId],
    queryFn: async () => {
      if (!defaultRepoId) return []
      const response = await api.getApiClient().listGitRepositoryBranches(defaultRepoId)
      return response.data || []
    },
    enabled: !!defaultRepoId,
    staleTime: 30000,
  })

  // Get the default branch name from the repository
  const defaultBranchName = useMemo(() => {
    const defaultRepo = projectRepositories.find(r => r.id === defaultRepoId)
    return defaultRepo?.default_branch || 'main'
  }, [projectRepositories, defaultRepoId])

  // Set baseBranch to default when component mounts
  useEffect(() => {
    if (defaultBranchName && !baseBranch) {
      setBaseBranch(defaultBranchName)
    }
  }, [defaultBranchName, baseBranch])

  // Inline agent creation state
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false)
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [newAgentName, setNewAgentName] = useState('-')
  const [userModifiedName, setUserModifiedName] = useState(false)
  const [creatingAgent, setCreatingAgent] = useState(false)
  const [agentError, setAgentError] = useState('')

  // Ref for task prompt text field
  const taskPromptRef = useRef<HTMLTextAreaElement>(null)

  // Sort apps: project default first, then zed_external, then others
  const sortedApps = useMemo(() => {
    if (!apps.apps) return []
    const zedExternalApps: IApp[] = []
    const otherApps: IApp[] = []
    let defaultApp: IApp | null = null
    const projectDefaultId = project?.default_helix_app_id

    apps.apps.forEach((app) => {
      if (projectDefaultId && app.id === projectDefaultId) {
        defaultApp = app
        return
      }
      const hasZedExternal = app.config?.helix?.assistants?.some(
        (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL
      ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
      if (hasZedExternal) {
        zedExternalApps.push(app)
      } else {
        otherApps.push(app)
      }
    })

    const result: IApp[] = []
    if (defaultApp) result.push(defaultApp)
    result.push(...zedExternalApps, ...otherApps)
    return result
  }, [apps.apps, project?.default_helix_app_id])

  // Auto-generate agent name when model or runtime changes
  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm])

  // Load apps on mount
  useEffect(() => {
    if (account.user?.id) {
      apps.loadApps()
    }
  }, [])

  // Auto-select default agent
  useEffect(() => {
    if (project?.default_helix_app_id) {
      setSelectedHelixAgent(project.default_helix_app_id)
      setShowCreateAgentForm(false)
    } else if (sortedApps.length === 0) {
      setShowCreateAgentForm(true)
      setSelectedHelixAgent('')
    } else {
      setSelectedHelixAgent(sortedApps[0]?.id || '')
      setShowCreateAgentForm(false)
    }
  }, [sortedApps, project?.default_helix_app_id])

  // Focus text field on mount (embedded mode)
  useEffect(() => {
    if (embedded) {
      setTimeout(() => {
        if (taskPromptRef.current) {
          taskPromptRef.current.focus()
        }
      }, 100)
    }
  }, [embedded])

  // Handle inline agent creation
  const handleCreateAgent = async (): Promise<string | null> => {
    if (!newAgentName.trim()) {
      setAgentError('Please enter a name for the agent')
      return null
    }
    if (!selectedModel) {
      setAgentError('Please select a model')
      return null
    }

    setCreatingAgent(true)
    setAgentError('')

    try {
      const params: ICreateAgentParams = {
        name: newAgentName.trim(),
        description: 'Code development agent for spec tasks',
        agentType: AGENT_TYPE_ZED_EXTERNAL,
        codeAgentRuntime,
        model: selectedModel,
        generationModelProvider: selectedProvider,
        generationModel: selectedModel,
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
      if (newApp) {
        return newApp.id
      }
      setAgentError('Failed to create agent')
      return null
    } catch (err) {
      console.error('Failed to create agent:', err)
      setAgentError(err instanceof Error ? err.message : 'Failed to create agent')
      return null
    } finally {
      setCreatingAgent(false)
    }
  }

  // Reset form
  const resetForm = useCallback(() => {
    setTaskPrompt('')
    setTaskPriority('medium')
    setSelectedHelixAgent('')
    setJustDoItMode(false)
    setUseHostDocker(false)
    setBranchMode(TypesBranchMode.BranchModeNew)
    setBaseBranch(defaultBranchName)
    setBranchPrefix('')
    setWorkingBranch('')
    setShowBranchCustomization(false)
    setShowCreateAgentForm(false)
    setCodeAgentRuntime('zed_agent')
    setSelectedProvider('')
    setSelectedModel('')
    setNewAgentName('-')
    setUserModifiedName(false)
    setAgentError('')
  }, [defaultBranchName])

  // Handle task creation
  const handleCreateTask = async () => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }

    if (!taskPrompt.trim()) {
      snackbar.error('Please describe what you want to get done')
      return
    }

    setIsCreating(true)

    try {
      let agentId = selectedHelixAgent
      setAgentError('')

      // Create agent inline if showing create form
      if (showCreateAgentForm) {
        const newAgentId = await handleCreateAgent()
        if (!newAgentId) {
          setIsCreating(false)
          return
        }
        agentId = newAgentId
      }

      const createTaskRequest: TypesCreateTaskRequest = {
        prompt: taskPrompt,
        priority: taskPriority as TypesSpecTaskPriority,
        project_id: projectId,
        app_id: agentId || undefined,
        just_do_it_mode: justDoItMode,
        use_host_docker: useHostDocker,
        branch_mode: branchMode,
        base_branch: branchMode === TypesBranchMode.BranchModeNew ? baseBranch : undefined,
        branch_prefix: branchMode === TypesBranchMode.BranchModeNew && branchPrefix ? branchPrefix : undefined,
        working_branch: branchMode === TypesBranchMode.BranchModeExisting ? workingBranch : undefined,
      }

      const response = await api.getApiClient().v1SpecTasksFromPromptCreate(createTaskRequest)

      if (response.data) {
        snackbar.success('SpecTask created! Planning agent will generate specifications.')
        resetForm()
        onTaskCreated(response.data)
      }
    } catch (error: any) {
      console.error('Failed to create SpecTask:', error)
      const errorMessage = error?.response?.data?.message || error?.message || 'Failed to create SpecTask. Please try again.'
      snackbar.error(errorMessage)
    } finally {
      setIsCreating(false)
    }
  }

  // Keyboard shortcut: Ctrl/Cmd+Enter to submit
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        if (taskPrompt.trim()) {
          e.preventDefault()
          handleCreateTask()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [taskPrompt, justDoItMode, selectedHelixAgent, useHostDocker])

  // Keyboard shortcut: Ctrl/Cmd+J to toggle Just Do It mode
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'j') {
        e.preventDefault()
        setJustDoItMode(prev => !prev)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      {showHeader && (
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: 2, borderBottom: 1, borderColor: 'divider' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <AddIcon />
            <Typography variant="h6">New SpecTask</Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {onClose && (
              <IconButton onClick={onClose}>
                <X size={20} />
              </IconButton>
            )}
          </Box>
        </Box>
      )}

      {/* Content */}
      <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
        <Stack spacing={2}>
          {/* Priority Selector */}
          <FormControl fullWidth size="small">
            <InputLabel>Priority</InputLabel>
            <Select
              value={taskPriority}
              onChange={(e) => setTaskPriority(e.target.value)}
              label="Priority"
            >
              <MenuItem value="low">Low</MenuItem>
              <MenuItem value="medium">Medium</MenuItem>
              <MenuItem value="high">High</MenuItem>
              <MenuItem value="critical">Critical</MenuItem>
            </Select>
          </FormControl>

          {/* Single text box for everything */}
          <TextField
            label="Describe what you want to get done"
            fullWidth
            required
            multiline
            rows={embedded ? 6 : 9}
            value={taskPrompt}
            onChange={(e) => setTaskPrompt(e.target.value)}
            onKeyDown={(e) => {
              // If user presses Enter in empty text box, close panel
              if (e.key === 'Enter' && !taskPrompt.trim() && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
                e.preventDefault()
                onClose?.()
              }
            }}
            placeholder={justDoItMode
              ? "Describe what you want the agent to do. It will start immediately without planning."
              : "Describe the task - the AI will generate specs from this."
            }
            helperText={justDoItMode
              ? "Agent will start working immediately"
              : "Planning agent extracts task name, description, and generates specifications"
            }
            inputRef={taskPromptRef}
            size="small"
          />

          {/* Branch Configuration */}
          {defaultRepoId && (
            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
              <Typography variant="subtitle2" gutterBottom>
                Where do you want to work?
              </Typography>

              {/* Mode Selection - Two Cards */}
              <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                <Box
                  onClick={() => setBranchMode(TypesBranchMode.BranchModeNew)}
                  sx={{
                    flex: 1,
                    p: 1.5,
                    border: 2,
                    borderColor: branchMode === TypesBranchMode.BranchModeNew ? 'primary.main' : 'divider',
                    borderRadius: 1,
                    cursor: 'pointer',
                    bgcolor: branchMode === TypesBranchMode.BranchModeNew ? 'action.selected' : 'transparent',
                    '&:hover': { bgcolor: 'action.hover' },
                  }}
                >
                  <Typography variant="body2" fontWeight={600}>
                    Start fresh
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Create a new branch
                  </Typography>
                </Box>
                <Box
                  onClick={() => setBranchMode(TypesBranchMode.BranchModeExisting)}
                  sx={{
                    flex: 1,
                    p: 1.5,
                    border: 2,
                    borderColor: branchMode === TypesBranchMode.BranchModeExisting ? 'primary.main' : 'divider',
                    borderRadius: 1,
                    cursor: 'pointer',
                    bgcolor: branchMode === TypesBranchMode.BranchModeExisting ? 'action.selected' : 'transparent',
                    '&:hover': { bgcolor: 'action.hover' },
                  }}
                >
                  <Typography variant="body2" fontWeight={600}>
                    Continue existing
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Resume work on a branch
                  </Typography>
                </Box>
              </Box>

              {/* Mode-specific options */}
              {branchMode === TypesBranchMode.BranchModeNew ? (
                <Stack spacing={1.5}>
                  {!showBranchCustomization ? (
                    <Box>
                      <Typography variant="caption" color="text.secondary">
                        New branch from <strong>{baseBranch || defaultBranchName}</strong>
                      </Typography>
                      <Button
                        size="small"
                        onClick={() => setShowBranchCustomization(true)}
                        sx={{ display: 'block', textTransform: 'none', fontSize: '0.75rem', p: 0, mt: 0.5 }}
                      >
                        Customize branches
                      </Button>
                    </Box>
                  ) : (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                      <Box>
                        <FormControl fullWidth size="small">
                          <InputLabel>Base branch</InputLabel>
                          <Select
                            value={baseBranch}
                            onChange={(e) => setBaseBranch(e.target.value)}
                            label="Base branch"
                          >
                            {branchesData?.map((branch: string) => (
                              <MenuItem key={branch} value={branch}>
                                {branch}
                                {branch === defaultBranchName && (
                                  <Chip label="default" size="small" sx={{ ml: 1, height: 18 }} />
                                )}
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, ml: 1.75, display: 'block' }}>
                          New branch will be created from this base. Use to build on existing work.
                        </Typography>
                      </Box>
                      <TextField
                        label="Working branch name"
                        size="small"
                        fullWidth
                        value={branchPrefix}
                        onChange={(e) => setBranchPrefix(e.target.value)}
                        placeholder="feature/user-auth"
                        helperText={branchPrefix
                          ? `Work will be done on: ${branchPrefix}-{task#}`
                          : "Leave empty to auto-generate. This is where the agent commits changes."
                        }
                      />
                      <Button
                        size="small"
                        onClick={() => {
                          setShowBranchCustomization(false)
                          setBaseBranch(defaultBranchName)
                          setBranchPrefix('')
                        }}
                        sx={{ alignSelf: 'flex-start', textTransform: 'none', fontSize: '0.75rem' }}
                      >
                        Use defaults
                      </Button>
                    </Box>
                  )}
                </Stack>
              ) : (
                <FormControl fullWidth size="small">
                  <InputLabel>Select branch</InputLabel>
                  <Select
                    value={workingBranch}
                    onChange={(e) => setWorkingBranch(e.target.value)}
                    label="Select branch"
                  >
                    {branchesData
                      ?.filter((branch: string) => branch !== defaultBranchName)
                      .map((branch: string) => (
                        <MenuItem key={branch} value={branch}>
                          {branch}
                        </MenuItem>
                      ))}
                    {branchesData?.filter((branch: string) => branch !== defaultBranchName).length === 0 && (
                      <MenuItem disabled value="">
                        No feature branches available
                      </MenuItem>
                    )}
                  </Select>
                </FormControl>
              )}
            </Box>
          )}

          {/* Agent Selection (dropdown) */}
          <Box>
            {!showCreateAgentForm ? (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                <AgentDropdown
                  value={selectedHelixAgent}
                  onChange={setSelectedHelixAgent}
                  agents={sortedApps}
                />
                <Button
                  size="small"
                  onClick={() => setShowCreateAgentForm(true)}
                  sx={{ alignSelf: 'flex-start', textTransform: 'none', fontSize: '0.75rem' }}
                >
                  + Create new agent
                </Button>
              </Box>
            ) : (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Typography variant="body2" color="text.secondary">
                  Code Agent Runtime
                </Typography>
                <FormControl fullWidth size="small">
                  <Select
                    value={codeAgentRuntime}
                    onChange={(e) => setCodeAgentRuntime(e.target.value as CodeAgentRuntime)}
                    disabled={creatingAgent}
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
                  </Select>
                </FormControl>

                <Typography variant="body2" color="text.secondary">
                  Code Agent Model
                </Typography>
                <AdvancedModelPicker
                  recommendedModels={RECOMMENDED_MODELS}
                  hint="Choose a capable model for agentic coding."
                  selectedProvider={selectedProvider}
                  selectedModelId={selectedModel}
                  onSelectModel={(provider, model) => {
                    setSelectedProvider(provider)
                    setSelectedModel(model)
                  }}
                  currentType="text"
                  displayMode="short"
                  disabled={creatingAgent}
                />

                <Typography variant="body2" color="text.secondary">
                  Agent Name
                </Typography>
                <TextField
                  value={newAgentName}
                  onChange={(e) => {
                    setNewAgentName(e.target.value)
                    setUserModifiedName(true)
                  }}
                  size="small"
                  fullWidth
                  disabled={creatingAgent}
                  helperText="Auto-generated from model and runtime. Edit to customize."
                />

                {agentError && (
                  <Alert severity="error">{agentError}</Alert>
                )}

                {sortedApps.length > 0 && (
                  <Button
                    size="small"
                    onClick={() => setShowCreateAgentForm(false)}
                    sx={{ alignSelf: 'flex-start' }}
                    disabled={creatingAgent}
                  >
                    Back to agent list
                  </Button>
                )}
              </Box>
            )}
          </Box>

          {/* Just Do It Mode Checkbox */}
          <FormControl fullWidth>
            <Tooltip title={`Skip writing a spec and just get the agent to immediately start doing what you ask (${navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'})`} placement="top">
              <FormControlLabel
                control={
                  <Checkbox
                    checked={justDoItMode}
                    onChange={(e) => setJustDoItMode(e.target.checked)}
                    color="warning"
                  />
                }
                label={
                  <Box>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography variant="body2" sx={{ fontWeight: 600 }}>
                        Just Do It
                      </Typography>
                      <Box component="span" sx={{ fontSize: '0.65rem', opacity: 0.6, fontFamily: 'monospace', border: '1px solid', borderColor: 'divider', borderRadius: '3px', px: 0.5 }}>
                        {navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'}
                      </Box>
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                      Skip spec planning — useful for tasks that don't require planning code changes
                    </Typography>
                  </Box>
                }
              />
            </Tooltip>
          </FormControl>
        </Stack>
      </Box>

      {/* Footer Actions */}
      <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', display: 'flex', gap: 2, justifyContent: 'flex-end' }}>
        {onClose && (
          <Button onClick={() => { resetForm(); onClose(); }}>
            Cancel
          </Button>
        )}
        <Button
          onClick={handleCreateTask}
          variant="contained"
          color="secondary"
          disabled={!taskPrompt.trim() || isCreating || creatingAgent || (showCreateAgentForm && !selectedModel)}
          startIcon={isCreating || creatingAgent ? <CircularProgress size={16} /> : <AddIcon />}
          sx={{
            '& .MuiButton-endIcon': {
              ml: 1,
              opacity: 0.6,
              fontSize: '0.75rem',
            },
          }}
          endIcon={
            <Box component="span" sx={{
              fontSize: '0.75rem',
              opacity: 0.6,
              fontFamily: 'monospace',
              ml: 1,
            }}>
              {navigator.platform.includes('Mac') ? '⌘↵' : 'Ctrl+↵'}
            </Box>
          }
        >
          Create Task
        </Button>
      </Box>
    </Box>
  )
}

export default NewSpecTaskForm
