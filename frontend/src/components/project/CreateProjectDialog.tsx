import React, { FC, useState, useEffect, useContext, useMemo, useCallback, useRef } from 'react'
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
  Accordion,
  AccordionSummary,
  AccordionDetails,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Avatar,
  Chip,
  InputAdornment,
} from '@mui/material'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import GitHubIcon from '@mui/icons-material/GitHub'
import LockIcon from '@mui/icons-material/Lock'
import { FolderGit2, Link as LinkIcon, Plus, Bot, RefreshCw, Search } from 'lucide-react'
import EditIcon from '@mui/icons-material/Edit'
import { TypesExternalRepositoryType, TypesRepositoryInfo } from '../../api/api'
import type { TypesGitRepository, TypesAzureDevOps } from '../../api/api'
import NewRepoForm from './forms/NewRepoForm'
import { useQueryClient } from '@tanstack/react-query'
import { useCreateProject } from '../../services'
import { useListOAuthConnections, useListOAuthProviders, useListOAuthConnectionRepositories, oauthConnectionsQueryKey } from '../../services/oauthProvidersService'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { AppsContext, ICreateAgentParams, CodeAgentRuntime, generateAgentName } from '../../contexts/apps'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { findOAuthConnectionForProvider, findOAuthProviderForType, hasRequiredScopes, PROVIDER_TYPES } from '../../utils/oauthProviders'

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

type RepoMode = 'auto' | 'select' | 'create' | 'link'

interface CreateProjectDialogProps {
  open: boolean
  onClose: () => void
  onSuccess?: (projectId: string) => void
  // For selecting existing repos
  repositories: TypesGitRepository[]
  reposLoading?: boolean
  // For creating new repos
  onCreateRepo?: (name: string, description: string) => Promise<TypesGitRepository | null>
  // For linking external repos (oauthConnectionId is used for OAuth-based linking)
  onLinkRepo?: (url: string, name: string, type: TypesExternalRepositoryType, username?: string, password?: string, azureDevOps?: TypesAzureDevOps, oauthConnectionId?: string) => Promise<TypesGitRepository | null>
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
  const api = useApi()
  const queryClient = useQueryClient()
  const { apps, loadApps, createAgent } = useContext(AppsContext)
  const createProjectMutation = useCreateProject()

  // OAuth connections for GitHub browse
  const { data: oauthConnections } = useListOAuthConnections()
  const { data: oauthProviders } = useListOAuthProviders()

  // Track OAuth popup window to detect when it closes
  const oauthPopupRef = useRef<Window | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedRepoId, setSelectedRepoId] = useState('')
  const [repoMode, setRepoMode] = useState<RepoMode>('auto')
  const [advancedExpanded, setAdvancedExpanded] = useState(false)

  // New repo creation fields
  const [newRepoName, setNewRepoName] = useState('')
  const [newRepoDescription, setNewRepoDescription] = useState('')
  const [userModifiedRepoName, setUserModifiedRepoName] = useState(false)

  // External repo linking fields
  const [externalUrl, setExternalUrl] = useState('')
  const [externalName, setExternalName] = useState('')
  const [externalType, setExternalType] = useState<TypesExternalRepositoryType>(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
  const [externalUsername, setExternalUsername] = useState('')
  const [externalPassword, setExternalPassword] = useState('')
  const [externalOrgUrl, setExternalOrgUrl] = useState('')
  const [externalToken, setExternalToken] = useState('')

  const [creatingRepo, setCreatingRepo] = useState(false)
  const [repoError, setRepoError] = useState('')

  // OAuth browse state
  const [selectedOAuthRepo, setSelectedOAuthRepo] = useState<TypesRepositoryInfo | null>(null)
  const [selectedOAuthConnectionId, setSelectedOAuthConnectionId] = useState<string | null>(null)
  const [repoSearchQuery, setRepoSearchQuery] = useState('')

  // Check if GitHub OAuth is connected with repo scope
  const githubConnection = useMemo(() => {
    return findOAuthConnectionForProvider(oauthConnections, PROVIDER_TYPES.GITHUB)
  }, [oauthConnections])

  const githubHasRepoScope = useMemo(() => {
    if (!githubConnection) return false
    return hasRequiredScopes(githubConnection.scopes, ['repo'])
  }, [githubConnection])

  const githubProvider = useMemo(() => {
    return findOAuthProviderForType(oauthProviders, PROVIDER_TYPES.GITHUB)
  }, [oauthProviders])

  // Fetch GitHub repos when connected with repo scope
  const { data: githubReposData, isLoading: githubReposLoading, error: githubReposError } =
    useListOAuthConnectionRepositories(
      githubHasRepoScope && externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub
        ? (githubConnection?.id || '')
        : ''
    )

  const githubRepos = githubReposData?.repositories || []

  // Filter repos by search query
  const filteredGithubRepos = useMemo(() => {
    if (!repoSearchQuery) return githubRepos
    const query = repoSearchQuery.toLowerCase()
    return githubRepos.filter(repo =>
      repo.name?.toLowerCase().includes(query) ||
      repo.full_name?.toLowerCase().includes(query) ||
      repo.description?.toLowerCase().includes(query)
    )
  }, [githubRepos, repoSearchQuery])

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

  // Open OAuth popup for GitHub authorization
  const openOAuthPopup = useCallback(async (providerId: string) => {
    try {
      const response = await api.get(
        `/api/v1/oauth/flow/start/${providerId}?scopes=repo,read:org,read:user,user:email`
      )
      const authUrl = response.auth_url || response?.data?.auth_url
      if (authUrl) {
        const width = 800
        const height = 700
        const left = (window.innerWidth - width) / 2
        const top = (window.innerHeight - height) / 2
        oauthPopupRef.current = window.open(
          authUrl,
          'oauth-popup',
          `width=${width},height=${height},left=${left},top=${top}`
        )
      }
    } catch (err) {
      console.error('Failed to start OAuth flow:', err)
      snackbar.error('Failed to start GitHub authorization')
    }
  }, [api, snackbar])

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
      setRepoMode('auto')
      setAdvancedExpanded(false)
      setNewRepoName('')
      setNewRepoDescription('')
      setUserModifiedRepoName(false)
      setExternalUrl('')
      setExternalName('')
      setExternalType(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
      setExternalUsername('')
      setExternalPassword('')
      setExternalOrgUrl('')
      setExternalToken('')
      setRepoError('')
      // Reset OAuth browse state
      setSelectedOAuthRepo(null)
      setSelectedOAuthConnectionId(null)
      setRepoSearchQuery('')
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
      setAdvancedExpanded(true)
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

  // Detect OAuth popup closure and refresh connections
  useEffect(() => {
    const checkPopupClosed = () => {
      if (oauthPopupRef.current && oauthPopupRef.current.closed) {
        oauthPopupRef.current = null
        // Refresh OAuth connections after popup closes
        queryClient.invalidateQueries({ queryKey: oauthConnectionsQueryKey() })
      }
    }

    const interval = setInterval(checkPopupClosed, 500)
    return () => clearInterval(interval)
  }, [queryClient])

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

  // Handle inline repo selection from GitHub OAuth list
  const handleSelectGitHubRepo = (repo: TypesRepositoryInfo) => {
    setSelectedOAuthRepo(repo)
    setSelectedOAuthConnectionId(githubConnection?.id || null)
    setExternalUrl(repo.clone_url || repo.html_url || '')
    setExternalName(repo.name || '')
  }

  const handleSubmit = async () => {
    if (!name.trim()) {
      snackbar.error('Project name is required')
      return
    }

    let repoIdToUse = ''
    setRepoError('')

    if (repoMode === 'auto') {
      // Auto mode: create a repo with the same name as the project
      if (!onCreateRepo) {
        setRepoError('Repository creation not available')
        return
      }

      const autoRepoName = toRepoName(name)
      if (!autoRepoName) {
        setRepoError('Please enter a valid project name')
        return
      }

      setCreatingRepo(true)
      try {
        const newRepo = await onCreateRepo(autoRepoName, description || `Files for ${name}`)
        if (!newRepo?.id) {
          setRepoError('Failed to create file storage')
          return
        }
        repoIdToUse = newRepo.id
      } catch (err) {
        setRepoError(err instanceof Error ? err.message : 'Failed to create file storage')
        return
      } finally {
        setCreatingRepo(false)
      }
    } else if (repoMode === 'select') {
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

      // ADO validation - requires org URL and PAT
      if (externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!externalOrgUrl.trim() || !externalToken.trim())) {
        setRepoError('Organization URL and Personal Access Token are required for Azure DevOps')
        return
      }

      // GitHub/GitLab validation - skip if using OAuth connection
      const usingOAuth = !!selectedOAuthConnectionId
      if (!usingOAuth && externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub && !externalToken.trim()) {
        // Allow without token for public repos, but warn
        console.log('[CreateProjectDialog] Linking GitHub repo without token - will only work for public repos')
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
          externalPassword || externalToken || undefined, // Use token as password for GitHub/GitLab
          azureDevOps,
          selectedOAuthConnectionId || undefined // Pass OAuth connection ID
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
    repoMode === 'auto' ? false : // Auto mode only needs project name
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
            helperText="Give your project a descriptive name"
          />

          <TextField
            label="Description"
            fullWidth
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            rows={2}
            placeholder="What is this project about?"
          />

          {/* Advanced: Git Repository Options */}
          <Accordion
            expanded={advancedExpanded || !!preselectedRepoId}
            onChange={(_, expanded) => {
              if (!preselectedRepoId) {
                setAdvancedExpanded(expanded)
                // When expanding, default to 'create' mode if currently 'auto'
                if (expanded && repoMode === 'auto') {
                  setRepoMode('create')
                }
                // When collapsing, reset to 'auto' mode
                if (!expanded) {
                  setRepoMode('auto')
                }
              }
            }}
            sx={{
              boxShadow: 'none',
              border: (theme) => `1px solid ${theme.palette.divider}`,
              '&:before': { display: 'none' },
              borderRadius: 1,
            }}
          >
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <FolderGit2 size={18} />
                <Typography variant="body2">
                  {advancedExpanded || preselectedRepoId
                    ? 'Git Repository'
                    : 'Connect a Git repository (optional)'}
                </Typography>
              </Box>
            </AccordionSummary>
            <AccordionDetails>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Typography variant="body2" color="text.secondary">
                  Connect an existing Git repository or create a new one with a custom name.
                  {!advancedExpanded && ' Leave collapsed to auto-create storage for your project.'}
                </Typography>

                <ToggleButtonGroup
                  value={repoMode}
                  exclusive
                  onChange={(_, v) => {
                    if (v && !preselectedRepoId && v !== 'auto') {
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
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Repository Type selector - always shown */}
                    <FormControl fullWidth size="small">
                      <InputLabel>Repository Type</InputLabel>
                      <Select
                        value={externalType}
                        label="Repository Type"
                        onChange={(e) => {
                          setExternalType(e.target.value as TypesExternalRepositoryType)
                          // Clear OAuth selection when changing type
                          if (selectedOAuthRepo) {
                            setSelectedOAuthRepo(null)
                            setSelectedOAuthConnectionId(null)
                            setExternalUrl('')
                            setExternalName('')
                          }
                        }}
                      >
                        <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitHub}>GitHub</MenuItem>
                        <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitLab}>GitLab</MenuItem>
                        <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeADO}>Azure DevOps</MenuItem>
                        <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket}>Bitbucket (coming soon)</MenuItem>
                      </Select>
                    </FormControl>

                    {/* CASE 1: GitHub + OAuth with repo scope - Inline repo browser */}
                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub && githubHasRepoScope && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                        {/* Search field */}
                        <TextField
                          fullWidth
                          size="small"
                          placeholder="Search your repositories..."
                          value={repoSearchQuery}
                          onChange={(e) => setRepoSearchQuery(e.target.value)}
                          InputProps={{
                            startAdornment: (
                              <InputAdornment position="start">
                                <Search size={18} />
                              </InputAdornment>
                            ),
                          }}
                        />

                        {/* Error state */}
                        {githubReposError && (
                          <Alert severity="error" sx={{ mt: 1 }}>
                            {githubReposError instanceof Error ? githubReposError.message : 'Failed to load repositories'}
                          </Alert>
                        )}

                        {/* Loading state */}
                        {githubReposLoading ? (
                          <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                            <CircularProgress size={24} />
                          </Box>
                        ) : filteredGithubRepos.length === 0 ? (
                          <Box sx={{ textAlign: 'center', py: 3 }}>
                            <Typography variant="body2" color="text.secondary">
                              {repoSearchQuery ? 'No repositories match your search' : 'No repositories found'}
                            </Typography>
                          </Box>
                        ) : (
                          /* Repo list */
                          <List
                            sx={{
                              maxHeight: 200,
                              overflow: 'auto',
                              border: 1,
                              borderColor: 'divider',
                              borderRadius: 1,
                              bgcolor: 'background.paper',
                            }}
                            dense
                          >
                            {filteredGithubRepos.map((repo, index) => {
                              const isSelected = selectedOAuthRepo?.full_name === repo.full_name ||
                                selectedOAuthRepo?.id === repo.id
                              return (
                                <ListItem key={repo.id || repo.full_name || index} disablePadding>
                                  <ListItemButton
                                    selected={isSelected}
                                    onClick={() => handleSelectGitHubRepo(repo)}
                                    sx={{
                                      '&.Mui-selected': {
                                        bgcolor: 'action.selected',
                                        '&:hover': { bgcolor: 'action.selected' },
                                      },
                                    }}
                                  >
                                    <ListItemIcon sx={{ minWidth: 36 }}>
                                      <Avatar sx={{ width: 24, height: 24, fontSize: '0.75rem', bgcolor: 'action.hover' }}>
                                        {repo.name?.[0]?.toUpperCase() || 'R'}
                                      </Avatar>
                                    </ListItemIcon>
                                    <ListItemText
                                      primary={repo.full_name || repo.name}
                                      secondary={repo.description || 'No description'}
                                      primaryTypographyProps={{ variant: 'body2' }}
                                      secondaryTypographyProps={{ variant: 'caption', noWrap: true }}
                                    />
                                    {repo.private && (
                                      <Chip
                                        icon={<LockIcon sx={{ fontSize: 12 }} />}
                                        label="Private"
                                        size="small"
                                        variant="outlined"
                                        sx={{ height: 20, '& .MuiChip-label': { px: 0.5, fontSize: '0.65rem' } }}
                                      />
                                    )}
                                  </ListItemButton>
                                </ListItem>
                              )
                            })}
                          </List>
                        )}

                        {/* Selected repo details */}
                        {selectedOAuthRepo && (
                          <Box sx={{ mt: 1 }}>
                            <TextField
                              label="Display Name in Helix (optional)"
                              fullWidth
                              size="small"
                              value={externalName}
                              onChange={(e) => setExternalName(e.target.value)}
                              helperText="Name shown in Helix. Leave empty to use the repository name."
                            />
                          </Box>
                        )}
                      </Box>
                    )}

                    {/* CASE 2: GitHub + OAuth without repo scope - Upgrade prompt + PAT fallback */}
                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub && githubConnection && !githubHasRepoScope && githubProvider && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <Alert
                          severity="info"
                          action={
                            <Button
                              color="inherit"
                              size="small"
                              startIcon={<RefreshCw size={14} />}
                              onClick={() => openOAuthPopup(githubProvider.id)}
                            >
                              Upgrade
                            </Button>
                          }
                        >
                          Upgrade your GitHub connection to browse repos directly.
                        </Alert>
                        <Divider>
                          <Typography variant="caption" color="text.secondary">
                            or use Personal Access Token
                          </Typography>
                        </Divider>
                        <TextField
                          label="Repository URL"
                          fullWidth
                          size="small"
                          value={externalUrl}
                          onChange={(e) => setExternalUrl(e.target.value)}
                          placeholder="https://github.com/owner/repository"
                          helperText="HTTPS URL to the GitHub repository"
                          required
                        />
                        <TextField
                          label="Display Name in Helix (optional)"
                          fullWidth
                          size="small"
                          value={externalName}
                          onChange={(e) => setExternalName(e.target.value)}
                          helperText="Name shown in Helix. Leave empty to use the repository name."
                        />
                        <TextField
                          label="Personal Access Token"
                          fullWidth
                          size="small"
                          type="password"
                          value={externalToken}
                          onChange={(e) => setExternalToken(e.target.value)}
                          placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
                          helperText="Required for private repositories"
                        />
                      </Box>
                    )}

                    {/* CASE 3: GitHub but no OAuth connection - PAT form with hint */}
                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub && !githubConnection && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        {githubProvider && (
                          <Alert
                            severity="info"
                            action={
                              <Button
                                color="inherit"
                                size="small"
                                onClick={() => openOAuthPopup(githubProvider.id)}
                              >
                                Connect
                              </Button>
                            }
                          >
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                              Connect via OAuth
                              <Chip label="Recommended" size="small" color="primary" sx={{ height: 20 }} />
                            </Box>
                          </Alert>
                        )}
                        <TextField
                          label="Repository URL"
                          fullWidth
                          size="small"
                          value={externalUrl}
                          onChange={(e) => setExternalUrl(e.target.value)}
                          placeholder="https://github.com/owner/repository"
                          helperText="HTTPS URL to the GitHub repository"
                          required
                        />
                        <TextField
                          label="Display Name in Helix (optional)"
                          fullWidth
                          size="small"
                          value={externalName}
                          onChange={(e) => setExternalName(e.target.value)}
                          helperText="Name shown in Helix. Leave empty to use the repository name."
                        />
                        <TextField
                          label="Personal Access Token"
                          fullWidth
                          size="small"
                          type="password"
                          value={externalToken}
                          onChange={(e) => setExternalToken(e.target.value)}
                          placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
                          helperText="Required for private repositories"
                        />
                      </Box>
                    )}

                    {/* CASE 4: Non-GitHub providers - standard forms */}
                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeGitLab && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <TextField
                          label="Repository URL"
                          fullWidth
                          size="small"
                          value={externalUrl}
                          onChange={(e) => setExternalUrl(e.target.value)}
                          placeholder="https://gitlab.com/group/project"
                          helperText="HTTPS URL to the GitLab project"
                          required
                        />
                        <TextField
                          label="Display Name in Helix (optional)"
                          fullWidth
                          size="small"
                          value={externalName}
                          onChange={(e) => setExternalName(e.target.value)}
                          helperText="Name shown in Helix. Leave empty to use the repository name."
                        />
                        <TextField
                          label="Personal Access Token"
                          fullWidth
                          size="small"
                          type="password"
                          value={externalToken}
                          onChange={(e) => setExternalToken(e.target.value)}
                          placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
                          helperText="Required for private repositories (needs read_repository scope)"
                        />
                      </Box>
                    )}

                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <TextField
                          label="Repository URL"
                          fullWidth
                          size="small"
                          value={externalUrl}
                          onChange={(e) => setExternalUrl(e.target.value)}
                          placeholder="https://dev.azure.com/organization/project/_git/repository"
                          helperText="Paste the full URL - organization will be auto-filled"
                          required
                        />
                        <TextField
                          label="Display Name in Helix (optional)"
                          fullWidth
                          size="small"
                          value={externalName}
                          onChange={(e) => setExternalName(e.target.value)}
                          helperText="Name shown in Helix. Leave empty to use the repository name."
                        />
                        <TextField
                          label="Organization URL"
                          fullWidth
                          size="small"
                          required
                          value={externalOrgUrl}
                          onChange={(e) => setExternalOrgUrl(e.target.value)}
                          placeholder="https://dev.azure.com/organization"
                          helperText="Azure DevOps organization URL"
                        />
                        <TextField
                          label="Personal Access Token"
                          fullWidth
                          size="small"
                          required
                          type="password"
                          value={externalToken}
                          onChange={(e) => setExternalToken(e.target.value)}
                          helperText="Personal Access Token for Azure DevOps"
                        />
                      </Box>
                    )}

                    {externalType === TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket && (
                      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <Alert severity="warning">Bitbucket support coming soon</Alert>
                        <TextField
                          label="Repository URL"
                          fullWidth
                          size="small"
                          value={externalUrl}
                          onChange={(e) => setExternalUrl(e.target.value)}
                          placeholder="https://bitbucket.org/owner/repository"
                          disabled
                        />
                      </Box>
                    )}
                  </Box>
                )}
              </Box>
            </AccordionDetails>
          </Accordion>

          {repoError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {repoError}
            </Alert>
          )}

          <Divider sx={{ my: 1 }} />

          <Typography variant="subtitle2" color="text.secondary">
            AI Agent
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Choose which AI agent will work on tasks in this project.
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
                        <Bot size={18} color="#9e9e9e" />
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
