import React, { FC, useState, useEffect, useCallback, useRef } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  ButtonBase,
  TextField,
  Box,
  FormControl,
  Select,
  MenuItem,
  Typography,
  Divider,
  Alert,
  CircularProgress,
  Tooltip,
} from '@mui/material'
import { Check, ChevronRight, FolderGit2, Link as LinkIcon, Plus } from 'lucide-react'
import { SiBitbucket, SiGithub, SiGitlab } from 'react-icons/si'
import { TypesExternalRepositoryType, TypesRepositoryInfo } from '../../api/api'
import type { TypesGitRepository, TypesAzureDevOps } from '../../api/api'
import { useCreateProject, useListProjects } from '../../services'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { CodeAgentRuntime } from '../../contexts/apps'
import { RECOMMENDED_CODING_MODELS } from '../../constants/models'
import CodingAgentForm from '../agent/CodingAgentForm'
import type { CodingAgentFormHandle } from '../agent/CodingAgentForm'
import { getAgentHarnessLabel } from '../agent/AgentHarness'
import BrowseProvidersDialog from './BrowseProvidersDialog'



type RepoMode = 'select' | 'create'

const normalizeName = (value: string) => value.trim().toLowerCase()

export const getUniqueRepositoryName = (
  baseName: string,
  repositories: TypesGitRepository[],
) => {
  const existingNames = new Set(
    repositories.map(repository => normalizeName(repository.name || '')),
  )
  let candidate = baseName
  let suffix = 2
  while (existingNames.has(normalizeName(candidate))) {
    candidate = `${baseName}-${suffix}`
    suffix += 1
  }
  return candidate
}

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
  onLinkRepo?: (url: string, name: string, type: TypesExternalRepositoryType, username?: string, password?: string, azureDevOps?: TypesAzureDevOps, oauthConnectionId?: string, gitProviderConnectionId?: string) => Promise<TypesGitRepository | null>
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
  const createProjectMutation = useCreateProject()
  const organizationId = account.organizationTools.organization?.id
  const {
    data: existingProjects = [],
    isLoading: projectsLoading,
  } = useListProjects(organizationId, {
    enabled: open && !!account.user?.id,
  })
  const [name, setName] = useState('')
  const [selectedRepoId, setSelectedRepoId] = useState('')
  const [repoMode, setRepoMode] = useState<RepoMode>('create')
  const [browseRepositoriesOpen, setBrowseRepositoriesOpen] = useState(false)

  const [creatingRepo, setCreatingRepo] = useState(false)
  const [repoError, setRepoError] = useState('')

  // Agent selection state
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [claudeCodeMode, setClaudeCodeMode] = useState<'subscription' | 'api_key'>('api_key')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [newAgentName, setNewAgentName] = useState(getAgentHarnessLabel('zed_agent'))
  const [creatingAgent, setCreatingAgent] = useState(false)
  const codingAgentFormRef = useRef<CodingAgentFormHandle>(null)

  // Agent names are implementation details in project creation. Keep a stable,
  // useful name derived from the selected runtime.
  useEffect(() => {
    setNewAgentName(getAgentHarnessLabel(codeAgentRuntime))
  }, [codeAgentRuntime])

  // Filter out internal repos - they're deprecated
  const codeRepos = repositories.filter(r => r.repo_type !== 'internal')
  const firstRepoId = codeRepos[0]?.id || ''
  const preselectedRepoName = codeRepos.find(
    repo => repo.id === preselectedRepoId,
  )?.name || ''
  const trimmedName = name.trim()
  const projectNameExists = !!trimmedName && existingProjects.some(
    project => normalizeName(project.name || '') === normalizeName(trimmedName),
  )
  const uniqueRepositoryName = repoMode === 'create'
    ? getUniqueRepositoryName(trimmedName, repositories)
    : trimmedName

  // Reset form when dialog closes or initialize with preselected repo
  useEffect(() => {
    if (!open) {
      setName('')
      setSelectedRepoId('')
      setRepoMode('create')
      setRepoError('')
      setBrowseRepositoriesOpen(false)
      // Reset agent state
      setNewAgentName(getAgentHarnessLabel('zed_agent'))
      setSelectedProvider('')
      setSelectedModel('')
      setCodeAgentRuntime('zed_agent')
      setClaudeCodeMode('api_key')
    } else if (preselectedRepoId) {
      // When opening with a preselected repo, switch to select mode
      setRepoMode('select')
      setSelectedRepoId(preselectedRepoId)
      setName(preselectedRepoName)
    }
  }, [open, preselectedRepoId, preselectedRepoName])

  // Auto-select first repo if available
  useEffect(() => {
    if (open && firstRepoId && !selectedRepoId) {
      setSelectedRepoId(firstRepoId)
    }
  }, [open, firstRepoId, selectedRepoId])

  const handleSelectHelixRepository = (repo: TypesGitRepository) => {
    setSelectedRepoId(repo.id || '')
    setRepoMode('select')
    setName(repo.name || '')
    setBrowseRepositoriesOpen(false)
  }

  const handleSelectExistingRepository = (repoId: string) => {
    setSelectedRepoId(repoId)
    const repository = codeRepos.find(repo => repo.id === repoId)
    setName(repository?.name || '')
  }

  const handleRepoModeChange = (mode: RepoMode) => {
    setRepoMode(mode)
    setRepoError('')
    if (mode === 'select') {
      const repository = codeRepos.find(repo => repo.id === selectedRepoId)
      setName(repository?.name || '')
    }
  }

  const handleBrowseExternalRepository = async (
    repo: TypesRepositoryInfo,
    providerTypeOrCredentials: string,
    oauthConnectionId?: string,
    gitProviderConnectionId?: string,
  ) => {
    if (!onLinkRepo) return

    let providerType = providerTypeOrCredentials
    let credentials: {
      pat?: string
      username?: string
      orgUrl?: string
    } | null = null
    if (providerTypeOrCredentials.startsWith('{')) {
      credentials = JSON.parse(providerTypeOrCredentials)
      providerType = (credentials as { type?: string }).type || 'github'
    }

    const externalTypes: Record<string, TypesExternalRepositoryType> = {
      github: TypesExternalRepositoryType.ExternalRepositoryTypeGitHub,
      gitlab: TypesExternalRepositoryType.ExternalRepositoryTypeGitLab,
      'azure-devops': TypesExternalRepositoryType.ExternalRepositoryTypeADO,
      bitbucket: TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket,
    }
    const externalRepositoryType = externalTypes[providerType]
    if (!externalRepositoryType) return

    setCreatingRepo(true)
    setRepoError('')
    try {
      const linkedRepo = await onLinkRepo(
        repo.clone_url || repo.html_url || '',
        repo.name || repo.full_name?.split('/').pop() || 'repository',
        externalRepositoryType,
        credentials?.username,
        credentials?.pat,
        providerType === 'azure-devops'
          ? {
              organization_url: credentials?.orgUrl || '',
              personal_access_token: credentials?.pat || '',
            }
          : undefined,
        oauthConnectionId,
        gitProviderConnectionId,
      )
      if (!linkedRepo?.id) {
        setRepoError('Failed to link repository')
        return
      }
      setSelectedRepoId(linkedRepo.id)
      setRepoMode('select')
      setName(linkedRepo.name || '')
      setBrowseRepositoriesOpen(false)
    } catch (error) {
      setRepoError(error instanceof Error ? error.message : 'Failed to link repository')
    } finally {
      setCreatingRepo(false)
    }
  }

  const handleSubmit = async () => {
    if (!trimmedName) {
      snackbar.error('Name is required')
      return
    }
    if (projectNameExists) {
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
      if (!onCreateRepo) {
        setRepoError('Repository creation not available')
        return
      }

      setCreatingRepo(true)
      try {
        const newRepo = await onCreateRepo(uniqueRepositoryName, '')
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
    }

    if (!repoIdToUse) {
      snackbar.error('Primary repository is required')
      return
    }

    const createdAgent = await codingAgentFormRef.current?.handleCreateAgent()
    if (!createdAgent?.id) {
      return
    }

    try {
      const result = await createProjectMutation.mutateAsync({
        name: trimmedName,
        description: '',
        default_repo_id: repoIdToUse,
        organization_id: account.organizationTools.organization?.id,
        default_helix_app_id: createdAgent.id,
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

  const isSubmitDisabled = createProjectMutation.isPending
    || creatingRepo
    || creatingAgent
    || projectsLoading
    || !name.trim()
    || projectNameExists
    || (repoMode === 'select' && !selectedRepoId)

  // Handle Cmd/Ctrl+Enter to submit
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    const isMod = e.metaKey || e.ctrlKey
    if (isMod && e.key === 'Enter' && !isSubmitDisabled) {
      e.preventDefault()
      handleSubmit()
    }
  }, [isSubmitDisabled, handleSubmit])

  return (
    <>
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      onKeyDown={handleKeyDown}
    >
      <DialogTitle>Create New Project</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {/* Git Repository picker */}
          <Box>
            <Typography variant="subtitle2" sx={{ mb: 0.25 }}>
              Repository
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              Choose where this project&apos;s code will live.
            </Typography>

            <Box
              sx={{
                border: 1,
                borderColor: 'divider',
                borderRadius: 1.5,
                overflow: 'hidden',
                mb: 2,
              }}
            >
              {([
                {
                  mode: 'create' as const,
                  icon: <Plus size={18} />,
                  title: 'New Helix repository',
                  description: 'Create and host a new repository in Helix.',
                },
                {
                  mode: 'select' as const,
                  icon: <FolderGit2 size={18} />,
                  title: 'Existing repository',
                  description: 'Choose a repository already connected to this organization.',
                },
              ]).map((tile) => {
                const isSelected = repoMode === tile.mode
                const isDisabled = tile.mode === 'select' && codeRepos.length === 0
                return (
                  <Tooltip
                    key={tile.mode}
                    title={isDisabled ? "You don't have any repositories yet." : ''}
                  >
                    <Box component="span" sx={{ display: 'block' }}>
                      <ButtonBase
                        component="button"
                        disabled={isDisabled}
                        onClick={() => handleRepoModeChange(tile.mode)}
                        sx={(theme) => ({
                          width: '100%',
                          minHeight: 56,
                          px: 1.5,
                          py: 1,
                          display: 'grid',
                          gridTemplateColumns: '24px minmax(0, 1fr) 20px',
                          gap: 1,
                          alignItems: 'center',
                          textAlign: 'left',
                          color: 'text.primary',
                          borderBottom: tile.mode === 'create' ? 1 : 0,
                          borderColor: 'divider',
                          bgcolor: isSelected
                            ? theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.07)' : 'rgba(0,0,0,0.06)'
                            : 'transparent',
                          '&:hover': {
                            bgcolor: isSelected
                              ? theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.09)' : 'rgba(0,0,0,0.08)'
                              : theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.04)',
                          },
                          '&.Mui-disabled': {
                            color: 'text.disabled',
                            opacity: 0.6,
                          },
                        })}
                      >
                        <Box sx={{ color: isSelected ? 'text.primary' : 'text.secondary', display: 'flex' }}>
                          {tile.icon}
                        </Box>
                        <Box sx={{ minWidth: 0 }}>
                          <Typography variant="body2" fontWeight={600}>
                            {tile.title}
                          </Typography>
                          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block' }}>
                            {tile.description}
                          </Typography>
                        </Box>
                        {isSelected && <Check size={16} color="currentColor" />}
                      </ButtonBase>
                    </Box>
                  </Tooltip>
                )
              })}
              <ButtonBase
                component="button"
                onClick={() => setBrowseRepositoriesOpen(true)}
                sx={(theme) => ({
                  width: '100%',
                  minHeight: 56,
                  px: 1.5,
                  py: 1,
                  display: 'grid',
                  gridTemplateColumns: '24px minmax(0, 1fr) 20px',
                  gap: 1,
                  alignItems: 'center',
                  textAlign: 'left',
                  color: 'text.primary',
                  borderTop: 1,
                  borderColor: 'divider',
                  '&:hover': {
                    bgcolor: theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.04)',
                  },
                })}
              >
                <Box sx={{ color: 'text.secondary', display: 'flex' }}>
                  <LinkIcon size={18} />
                </Box>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="body2" fontWeight={600}>
                    Connect a Git repository
                  </Typography>
                  <Box
                    sx={{ display: 'flex', alignItems: 'center', gap: 0.75, color: 'text.secondary' }}
                  >
                    <Box
                      aria-label="Supported Git providers"
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}
                    >
                      <Box
                        component={SiGithub}
                        size={12}
                        sx={(theme) => ({
                          color: theme.palette.mode === 'dark' ? '#f0f0f0' : '#24292f',
                        })}
                      />
                      <SiGitlab size={12} color="#FC6D26" />
                      <SiBitbucket size={12} color="#0052CC" />
                    </Box>
                    <Typography variant="caption" color="text.secondary" noWrap>
                      GitHub, GitLab, Bitbucket, and Azure DevOps.
                    </Typography>
                  </Box>
                </Box>
                <ChevronRight size={16} />
              </ButtonBase>
            </Box>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>

                {repoMode === 'select' && (
                  <FormControl fullWidth size="small">
                    <Typography variant="caption" color="text.secondary" sx={{ mb: 0.75 }}>
                      Repository
                    </Typography>
                    <Select
                      value={selectedRepoId}
                      aria-label="Repository"
                      onChange={(e) => handleSelectExistingRepository(e.target.value)}
                      disabled={reposLoading}
                      sx={{ borderRadius: 1.5 }}
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

            </Box>
          </Box>

          <TextField
            label="Name"
            fullWidth
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            error={projectNameExists}
            helperText={
              projectNameExists
                ? `A project named “${trimmedName}” already exists.`
                : repoMode === 'create' && uniqueRepositoryName !== trimmedName
                  ? `Creates project “${trimmedName}” and Helix-hosted repository “${uniqueRepositoryName}”.`
                  : repoMode === 'create'
                ? 'Creates a project and Helix-hosted repository with this name.'
                : 'Project name, populated from the selected repository.'
            }
            FormHelperTextProps={{ sx: { mx: 0, mt: 0.75 } }}
          />

          {repoError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {repoError}
            </Alert>
          )}

          <Divider sx={{ my: 1 }} />

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
              setNewAgentName(nextValue.agentName)
            }}
            disabled={creatingAgent || createProjectMutation.isPending || creatingRepo}
            recommendedModels={RECOMMENDED_CODING_MODELS}
            createAgentDescription="Code development agent for spec tasks"
            createAgentOrganizationId={account.organizationTools.organization?.id}
            onCreateStateChange={setCreatingAgent}
            showCredentialSelection={false}
            showModelSelection={false}
            showAgentName={false}
            autoSelectRuntime={false}
            deferModelSelection
            showCreateButton={false}
            runtimeLabel="Coding agent"
            runtimeDescription="Choose which coding agent runs this project&apos;s tasks. Models are selected when a task starts."
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          variant="contained"
          color="secondary"
          onClick={handleSubmit}
          disabled={isSubmitDisabled}
          sx={{ mr: 1, mb: 1 }}
        >
          {createProjectMutation.isPending || creatingRepo || creatingAgent ? (
            <>
              <CircularProgress size={16} sx={{ mr: 1 }} />
              {creatingAgent ? 'Creating Agent...' : 'Creating...'}
            </>
          ) : (
            <>
              Create Project
              <Box
                component="span"
                aria-hidden="true"
                sx={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 0.5,
                  ml: 1,
                  px: 0.75,
                  height: 20,
                  borderRadius: 0.75,
                  border: '1px solid rgba(0, 0, 0, 0.18)',
                  bgcolor: 'rgba(0, 0, 0, 0.1)',
                  fontSize: '0.7rem',
                  fontWeight: 600,
                  lineHeight: 1,
                  letterSpacing: 0,
                  whiteSpace: 'nowrap',
                }}
              >
                <Box component="span">
                  {navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}
                </Box>
                <Box component="span">Enter</Box>
              </Box>
            </>
          )}
        </Button>
      </DialogActions>
    </Dialog>
    <BrowseProvidersDialog
      open={browseRepositoriesOpen}
      onClose={() => setBrowseRepositoriesOpen(false)}
      onSelectRepository={handleBrowseExternalRepository}
      onSelectHelixRepository={handleSelectHelixRepository}
      helixRepositories={codeRepos.filter((repo) => !repo.is_external)}
      helixRepositoriesLoading={reposLoading}
      isLinking={creatingRepo}
      organizationName={account.organizationTools.organization?.display_name || account.organizationTools.organization?.name}
    />
    </>
  )
}

export default CreateProjectDialog
