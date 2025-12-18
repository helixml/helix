import React, { FC, useState, useEffect, useContext, useMemo, useCallback } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Typography,
  Divider,
  Alert,
  ToggleButtonGroup,
  ToggleButton,
  CircularProgress,
  IconButton,
  Tooltip,
} from '@mui/material'
import { FolderGit2, Link as LinkIcon, Plus } from 'lucide-react'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import EditIcon from '@mui/icons-material/Edit'
import { TypesExternalRepositoryType } from '../../api/api'
import type { TypesGitRepository, TypesAzureDevOps } from '../../api/api'
import NewRepoForm from './forms/NewRepoForm'
import ExternalRepoForm from './forms/ExternalRepoForm'
import { useCreateProject } from '../../services'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { AppsContext, ICreateAgentParams, CodeAgentRuntime, generateAgentName } from '../../contexts/apps'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'

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

type RepoMode = 'select' | 'create' | 'link'

interface CreateProjectDialogProps {
  open: boolean
  onClose: () => void
  onSuccess?: (projectId: string) => void
  // For selecting existing repos
  repositories: TypesGitRepository[]
  reposLoading?: boolean
  // For creating new repos
  onCreateRepo?: (name: string, description: string) => Promise<TypesGitRepository | null>
  // For linking external repos
  onLinkRepo?: (url: string, name: string, type: TypesExternalRepositoryType, username?: string, password?: string, azureDevOps?: TypesAzureDevOps) => Promise<TypesGitRepository | null>
  // Preselect an existing repo (used when creating project from repo detail page)
  preselectedRepoId?: string
}

const CreateProjectDialog: FC<CreateProjectDialogProps> = ({
  open,
  onClose,
  onSuccess,
  repositories,
  reposLoading,
  onCreateRepo,
  onLinkRepo,
  preselectedRepoId,
}) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const { apps, loadApps, createAgent } = useContext(AppsContext)
  const createProjectMutation = useCreateProject()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedRepoId, setSelectedRepoId] = useState('')
  const [repoMode, setRepoMode] = useState<RepoMode>('create')

  // New repo creation fields
  const [newRepoName, setNewRepoName] = useState('')
  const [newRepoDescription, setNewRepoDescription] = useState('')
  const [userModifiedRepoName, setUserModifiedRepoName] = useState(false)

  // External repo linking fields
  const [externalUrl, setExternalUrl] = useState('')
  const [externalName, setExternalName] = useState('')
  const [externalType, setExternalType] = useState<TypesExternalRepositoryType>(TypesExternalRepositoryType.ExternalRepositoryTypeADO)
  const [externalUsername, setExternalUsername] = useState('')
  const [externalPassword, setExternalPassword] = useState('')
  const [externalOrgUrl, setExternalOrgUrl] = useState('')
  const [externalToken, setExternalToken] = useState('')

  const [creatingRepo, setCreatingRepo] = useState(false)
  const [repoError, setRepoError] = useState('')

  // Agent selection state
  const [selectedAgentId, setSelectedAgentId] = useState<string>('')
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false)
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [newAgentName, setNewAgentName] = useState('-')
  const [userModifiedName, setUserModifiedName] = useState(false)
  const [creatingAgent, setCreatingAgent] = useState(false)
  const [agentError, setAgentError] = useState('')

  // Auto-generate name when model or runtime changes (if user hasn't modified it)
  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm])

  // Convert project name to lowercase hyphenated repo name
  const toRepoName = (projectName: string): string => {
    return projectName
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9\s-]/g, '') // Remove special chars except spaces and hyphens
      .replace(/\s+/g, '-')          // Replace spaces with hyphens
      .replace(/-+/g, '-')           // Collapse multiple hyphens
      .replace(/^-|-$/g, '')         // Remove leading/trailing hyphens
  }

  // Auto-sync repo name from project name (if user hasn't modified it)
  useEffect(() => {
    if (!userModifiedRepoName && repoMode === 'create') {
      setNewRepoName(toRepoName(name))
    }
  }, [name, userModifiedRepoName, repoMode])

  // Sort apps: zed_external first, then others
  const sortedApps = useMemo(() => {
    if (!apps) return []
    const zedExternalApps: IApp[] = []
    const otherApps: IApp[] = []
    apps.forEach((app) => {
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

  // Filter out internal repos - they're deprecated
  const codeRepos = repositories.filter(r => r.repo_type !== 'internal')

  // Load apps when dialog opens
  useEffect(() => {
    if (open) {
      loadApps()
    }
  }, [open, loadApps])

  // Reset form when dialog closes or initialize with preselected repo
  useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
      setSelectedRepoId('')
      setRepoMode('create')
      setNewRepoName('')
      setNewRepoDescription('')
      setUserModifiedRepoName(false)
      setExternalUrl('')
      setExternalName('')
      setExternalType(TypesExternalRepositoryType.ExternalRepositoryTypeADO)
      setExternalUsername('')
      setExternalPassword('')
      setExternalOrgUrl('')
      setExternalToken('')
      setRepoError('')
      // Reset agent state
      setSelectedAgentId('')
      setShowCreateAgentForm(false)
      setNewAgentName('-')
      setUserModifiedName(false)
      setSelectedProvider('')
      setSelectedModel('')
      setCodeAgentRuntime('zed_agent')
      setAgentError('')
    } else if (preselectedRepoId) {
      // When opening with a preselected repo, switch to select mode
      setRepoMode('select')
      setSelectedRepoId(preselectedRepoId)
    }
  }, [open, preselectedRepoId])

  // Auto-select first repo if available
  useEffect(() => {
    if (open && codeRepos.length > 0 && !selectedRepoId) {
      setSelectedRepoId(codeRepos[0].id || '')
    }
  }, [open, codeRepos, selectedRepoId])

  // Auto-select first zed_external agent, or show create form if none
  useEffect(() => {
    if (open && apps && !selectedAgentId) {
      if (sortedApps.length > 0) {
        setSelectedAgentId(sortedApps[0].id)
        setShowCreateAgentForm(false)
      } else {
        setShowCreateAgentForm(true)
      }
    }
  }, [open, apps, sortedApps, selectedAgentId])

  // Handle creating a new agent
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

      const newApp = await createAgent(params)
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

  const handleSubmit = async () => {
    if (!name.trim()) {
      snackbar.error('Project name is required')
      return
    }

    let repoIdToUse = ''
    setRepoError('')

    if (repoMode === 'select') {
      if (!selectedRepoId) {
        setRepoError('Please select a repository')
        return
      }
      repoIdToUse = selectedRepoId
    } else if (repoMode === 'create') {
      if (!newRepoName.trim()) {
        setRepoError('Please enter a repository name')
        return
      }
      if (!onCreateRepo) {
        setRepoError('Repository creation not available')
        return
      }

      setCreatingRepo(true)
      try {
        const newRepo = await onCreateRepo(newRepoName, newRepoDescription)
        if (!newRepo?.id) {
          setRepoError('Failed to create repository')
          return
        }
        repoIdToUse = newRepo.id
      } catch (err) {
        setRepoError(err instanceof Error ? err.message : 'Failed to create repository')
        return
      } finally {
        setCreatingRepo(false)
      }
    } else if (repoMode === 'link') {
      if (!externalUrl.trim()) {
        setRepoError('Please enter a repository URL')
        return
      }
      if (!onLinkRepo) {
        setRepoError('External repository linking not available')
        return
      }

      // ADO validation
      if (externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!externalOrgUrl.trim() || !externalToken.trim())) {
        setRepoError('Organization URL and Personal Access Token are required for Azure DevOps')
        return
      }

      setCreatingRepo(true)
      try {
        const repoName = externalName || externalUrl.split('/').pop()?.replace('.git', '') || 'external-repo'
        const azureDevOps: TypesAzureDevOps | undefined = externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO ? {
          organization_url: externalOrgUrl,
          personal_access_token: externalToken,
        } : undefined

        const linkedRepo = await onLinkRepo(
          externalUrl,
          repoName,
          externalType,
          externalUsername || undefined,
          externalPassword || undefined,
          azureDevOps
        )
        if (!linkedRepo?.id) {
          setRepoError('Failed to link repository')
          return
        }
        repoIdToUse = linkedRepo.id
      } catch (err) {
        setRepoError(err instanceof Error ? err.message : 'Failed to link repository')
        return
      } finally {
        setCreatingRepo(false)
      }
    }

    if (!repoIdToUse) {
      snackbar.error('Primary repository is required')
      return
    }

    // Handle agent creation if needed
    let agentIdToUse = selectedAgentId
    setAgentError('')
    if (showCreateAgentForm) {
      const newAgentId = await handleCreateAgent()
      if (!newAgentId) {
        // Error already set in handleCreateAgent
        return
      }
      agentIdToUse = newAgentId
    }

    try {
      // DEBUG: Check org state when creating project
      console.log('[CreateProjectDialog] Creating project with:', {
        orgID: account.organizationTools.orgID,
        organization: account.organizationTools.organization,
        organizationId: account.organizationTools.organization?.id,
        defaultHelixAppId: agentIdToUse,
      })

      const result = await createProjectMutation.mutateAsync({
        name,
        description,
        default_repo_id: repoIdToUse,
        organization_id: account.organizationTools.organization?.id,
        default_helix_app_id: agentIdToUse || undefined,
      })
      snackbar.success('Project created successfully')
      onClose()

      if (result?.id) {
        if (onSuccess) {
          onSuccess(result.id)
        } else {
          account.orgNavigate('project-specs', { id: result.id })
        }
      }
    } catch (err) {
      snackbar.error('Failed to create project')
    }
  }

  // Check if agent selection is valid
  const agentValid = showCreateAgentForm
    ? (newAgentName.trim() && selectedModel)
    : !!selectedAgentId

  const isSubmitDisabled = createProjectMutation.isPending || creatingRepo || creatingAgent || !name.trim() || !agentValid || (
    repoMode === 'select' ? !selectedRepoId :
    repoMode === 'create' ? !newRepoName.trim() :
    !externalUrl.trim() || (externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!externalOrgUrl.trim() || !externalToken.trim()))
  )

  // Handle Cmd/Ctrl+Enter to submit
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    const isMod = e.metaKey || e.ctrlKey
    if (isMod && e.key === 'Enter' && !isSubmitDisabled) {
      e.preventDefault()
      handleSubmit()
    }
  }, [isSubmitDisabled, handleSubmit])

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth onKeyDown={handleKeyDown}>
      <DialogTitle>Create New Project</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label="Project Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />

          <Divider sx={{ my: 1 }} />

          <Typography variant="subtitle2" color="text.secondary">
            Primary Repository (required)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Project configuration and startup scripts will be stored in this repository.
            You can attach additional repositories later in Project Settings.
          </Typography>

          <ToggleButtonGroup
            value={repoMode}
            exclusive
            onChange={(_, v) => {
              if (v && !preselectedRepoId) {
                setRepoMode(v)
              }
            }}
            size="small"
            fullWidth
            disabled={!!preselectedRepoId}
          >
            <ToggleButton value="create">
              <Plus size={16} style={{ marginRight: 4 }} />
              New
            </ToggleButton>
            <ToggleButton value="select">
              <FolderGit2 size={16} style={{ marginRight: 4 }} />
              Existing
            </ToggleButton>
            <ToggleButton value="link">
              <LinkIcon size={16} style={{ marginRight: 4 }} />
              External
            </ToggleButton>
          </ToggleButtonGroup>

          {repoMode === 'select' && (
            <FormControl fullWidth size="small">
              <InputLabel>Select Repository</InputLabel>
              <Select
                value={selectedRepoId}
                label="Select Repository"
                onChange={(e) => setSelectedRepoId(e.target.value)}
                disabled={reposLoading || !!preselectedRepoId}
              >
                {codeRepos.map((repo) => (
                  <MenuItem key={repo.id} value={repo.id}>
                    {repo.name}
                    {repo.is_external && ` (${repo.external_type || 'external'})`}
                  </MenuItem>
                ))}
                {codeRepos.length === 0 && (
                  <MenuItem disabled value="">
                    No repositories available
                  </MenuItem>
                )}
              </Select>
            </FormControl>
          )}

          {repoMode === 'create' && (
            <NewRepoForm
              name={newRepoName}
              onNameChange={(value) => {
                setNewRepoName(value)
                setUserModifiedRepoName(true)
              }}
              size="small"
              showDescription={false}
            />
          )}

          {repoMode === 'link' && (
            <ExternalRepoForm
              url={externalUrl}
              onUrlChange={setExternalUrl}
              name={externalName}
              onNameChange={setExternalName}
              type={externalType}
              onTypeChange={setExternalType}
              username={externalUsername}
              onUsernameChange={setExternalUsername}
              password={externalPassword}
              onPasswordChange={setExternalPassword}
              organizationUrl={externalOrgUrl}
              onOrganizationUrlChange={setExternalOrgUrl}
              token={externalToken}
              onTokenChange={setExternalToken}
              size="small"
            />
          )}

          {repoError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {repoError}
            </Alert>
          )}

          <Divider sx={{ my: 1 }} />

          <Typography variant="subtitle2" color="text.secondary">
            Default Agent for Spec Tasks
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Select an agent to use for code development tasks in this project. You can configure MCP servers in the agent settings.
          </Typography>

          {!showCreateAgentForm ? (
            <>
              <FormControl fullWidth size="small">
                <InputLabel>Select Agent</InputLabel>
                <Select
                  value={selectedAgentId}
                  label="Select Agent"
                  onChange={(e) => setSelectedAgentId(e.target.value)}
                  renderValue={(value) => {
                    const app = sortedApps.find(a => a.id === value)
                    return app?.config?.helix?.name || 'Select Agent'
                  }}
                >
                  {sortedApps.map((app) => (
                    <MenuItem key={app.id} value={app.id}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                        <SmartToyIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                        <span style={{ flex: 1 }}>{app.config?.helix?.name || 'Unnamed Agent'}</span>
                        <Tooltip title="Edit agent">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation()
                              account.orgNavigate('app', { app_id: app.id })
                            }}
                            sx={{ ml: 'auto' }}
                          >
                            <EditIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    </MenuItem>
                  ))}
                  {sortedApps.length === 0 && (
                    <MenuItem disabled value="">
                      No agents available
                    </MenuItem>
                  )}
                </Select>
              </FormControl>
              <Button
                size="small"
                onClick={() => setShowCreateAgentForm(true)}
                sx={{ alignSelf: 'flex-start', mt: 0.5 }}
              >
                + Create new agent
              </Button>
            </>
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

          {agentError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {agentError}
            </Alert>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          variant="contained"
          color="secondary"
          onClick={handleSubmit}
          disabled={isSubmitDisabled}
          sx={{ mr: 1, mb: 1 }}
          endIcon={
            !createProjectMutation.isPending && !creatingRepo && !creatingAgent ? (
              <Box component="span" sx={{ opacity: 0.7, fontFamily: 'monospace', ml: 0.5 }}>
                {navigator.platform.includes('Mac') ? '⌘↵' : 'Ctrl+↵'}
              </Box>
            ) : null
          }
        >
          {createProjectMutation.isPending || creatingRepo || creatingAgent ? (
            <>
              <CircularProgress size={16} sx={{ mr: 1 }} />
              {creatingAgent ? 'Creating Agent...' : 'Creating...'}
            </>
          ) : (
            'Create Project'
          )}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateProjectDialog
