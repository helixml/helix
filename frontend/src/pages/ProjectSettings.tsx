import React, { FC, useState, useEffect, useRef, useContext, useMemo } from 'react'
import {
  Container,
  Box,
  Paper,
  Typography,
  TextField,
  Button,
  Alert,
  CircularProgress,
  Divider,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Checkbox,
  Tooltip,
  Chip,
} from '@mui/material'
import SaveIcon from '@mui/icons-material/Save'
import StarIcon from '@mui/icons-material/Star'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import CodeIcon from '@mui/icons-material/Code'
import ExploreIcon from '@mui/icons-material/Explore'
import PeopleIcon from '@mui/icons-material/People'
import AddIcon from '@mui/icons-material/Add'
import WarningIcon from '@mui/icons-material/Warning'
import DeleteForeverIcon from '@mui/icons-material/DeleteForever'
import DeleteIcon from '@mui/icons-material/Delete'
import LinkIcon from '@mui/icons-material/Link'
import StopIcon from '@mui/icons-material/Stop'
import RefreshIcon from '@mui/icons-material/Refresh'
import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import EditIcon from '@mui/icons-material/Edit'
import HistoryIcon from '@mui/icons-material/History'
import DescriptionIcon from '@mui/icons-material/Description'

import Page from '../components/system/Page'
import AccessManagement from '../components/app/AccessManagement'
import StartupScriptEditor from '../components/project/StartupScriptEditor'
import { AdvancedModelPicker } from '../components/create/AdvancedModelPicker'
import { AppsContext, ICreateAgentParams, CodeAgentRuntime, generateAgentName } from '../contexts/apps'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../types'

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
import ProjectRepositoriesList from '../components/project/ProjectRepositoriesList'
import MoonlightStreamViewer from '../components/external-agent/MoonlightStreamViewer'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import { useFloatingModal } from '../contexts/floatingModal'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import {
  useGetProject,
  useUpdateProject,
  useGetProjectRepositories,
  useSetProjectPrimaryRepository,
  useAttachRepositoryToProject,
  useDetachRepositoryFromProject,
  useDeleteProject,
  useGetProjectExploratorySession,
  useStartProjectExploratorySession,
  useStopProjectExploratorySession,
  projectExploratorySessionQueryKey,
  useGetProjectGuidelinesHistory,
} from '../services'
import { useGitRepositories } from '../services/gitRepositoryService'
import {
  useListProjectAccessGrants,
  useCreateProjectAccessGrant,
  useDeleteProjectAccessGrant,
} from '../services/projectAccessGrantService'

const ProjectSettings: FC = () => {
  const account = useAccount()
  const { params, navigate } = useRouter()
  const snackbar = useSnackbar()
  const api = useApi()
  const projectId = params.id as string
  const floatingModal = useFloatingModal()
  const queryClient = useQueryClient()
  const { apps, loadApps, createAgent } = useContext(AppsContext)

  const { data: project, isLoading, error } = useGetProject(projectId)
  const { data: repositories = [] } = useGetProjectRepositories(projectId)

  const updateProjectMutation = useUpdateProject(projectId)
  const setPrimaryRepoMutation = useSetProjectPrimaryRepository(projectId)
  const attachRepoMutation = useAttachRepositoryToProject(projectId)
  const detachRepoMutation = useDetachRepositoryFromProject(projectId)
  const deleteProjectMutation = useDeleteProject()

  // Get current org context for fetching repositories
  const currentOrg = account.organizationTools.organization
  // List repos by organization_id when in org context, or by owner_id for personal workspace
  const { data: allUserRepositories = [] } = useGitRepositories(
    currentOrg?.id
      ? { organizationId: currentOrg.id }
      : { ownerId: account.user?.id }
  )

  // Access grants for RBAC
  const { data: accessGrants = [], isLoading: accessGrantsLoading } = useListProjectAccessGrants(projectId, !!project?.organization_id)
  const createAccessGrantMutation = useCreateProjectAccessGrant(projectId)
  const deleteAccessGrantMutation = useDeleteProjectAccessGrant(projectId)

  // Exploratory session
  const { data: exploratorySessionData } = useGetProjectExploratorySession(projectId)
  const startExploratorySessionMutation = useStartProjectExploratorySession(projectId)
  const stopExploratorySessionMutation = useStopProjectExploratorySession(projectId)

  // Create SpecTask mutation for "Fix Startup Script" feature
  const createSpecTaskMutation = useMutation({
    mutationFn: async (prompt: string) => {
      const response = await api.getApiClient().v1SpecTasksFromPromptCreate({
        project_id: projectId,
        prompt,
      })
      return response.data
    },
    onSuccess: (task) => {
      snackbar.success('Created task to fix startup script')
      // Navigate to the kanban board with the new task highlighted
      account.orgNavigate('project-specs', { id: projectId, highlight: task.id })
    },
    onError: () => {
      snackbar.error('Failed to create task')
    }
  })

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [startupScript, setStartupScript] = useState('')
  const [guidelines, setGuidelines] = useState('')
  const [autoStartBacklogTasks, setAutoStartBacklogTasks] = useState(false)
  const [showTestSession, setShowTestSession] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteConfirmName, setDeleteConfirmName] = useState('')
  const [attachRepoDialogOpen, setAttachRepoDialogOpen] = useState(false)
  const [selectedRepoToAttach, setSelectedRepoToAttach] = useState('')
  const [savingProject, setSavingProject] = useState(false)
  const [testingStartupScript, setTestingStartupScript] = useState(false)
  const [isSessionRestart, setIsSessionRestart] = useState(false)
  const [guidelinesHistoryDialogOpen, setGuidelinesHistoryDialogOpen] = useState(false)

  // Guidelines history
  const { data: guidelinesHistory = [] } = useGetProjectGuidelinesHistory(projectId, guidelinesHistoryDialogOpen)

  // Board settings state (initialized from query data)
  const [wipLimits, setWipLimits] = useState({
    planning: 3,
    review: 2,
    implementation: 5,
  })

  // Default agent state
  const [selectedAgentId, setSelectedAgentId] = useState<string>('')
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false)
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [newAgentName, setNewAgentName] = useState('-')
  const [userModifiedName, setUserModifiedName] = useState(false)
  const [creatingAgent, setCreatingAgent] = useState(false)
  const [agentError, setAgentError] = useState('')

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

  // Load apps when component mounts
  useEffect(() => {
    loadApps()
  }, [loadApps])

  // Auto-generate name when model or runtime changes (if user hasn't modified it)
  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm])

  // Initialize form from server data
  // This runs when project loads or refetches (standard React Query pattern)
  useEffect(() => {
    if (project) {
      setName(project.name || '')
      setDescription(project.description || '')
      setStartupScript(project.startup_script || '')
      setGuidelines(project.guidelines || '')
      setAutoStartBacklogTasks(project.auto_start_backlog_tasks || false)
      setSelectedAgentId(project.default_helix_app_id || '')

      // Load WIP limits from project metadata
      const projectWipLimits = project.metadata?.board_settings?.wip_limits
      if (projectWipLimits) {
        setWipLimits({
          planning: projectWipLimits.planning || 3,
          review: projectWipLimits.review || 2,
          implementation: projectWipLimits.implementation || 5,
        })
      }
    }
  }, [project])

  const handleSave = async (showSuccessMessage = true) => {
    console.log('[ProjectSettings] handleSave called', {
      showSuccessMessage,
      savingProject,
      hasProject: !!project,
      hasName: !!name,
      updatePending: updateProjectMutation.isPending,
    })

    if (savingProject) {
      console.warn('[ProjectSettings] Save already in progress, skipping')
      return false // Indicate save didn't happen
    }

    // Safety check: don't save if form hasn't been initialized yet
    if (!project || !name) {
      console.warn('[ProjectSettings] Attempted to save before form initialized, ignoring')
      return false // Indicate save didn't happen
    }

    try {
      setSavingProject(true)
      console.log('[ProjectSettings] Saving project settings...')

      // Save project basic settings
      await updateProjectMutation.mutateAsync({
        name,
        description,
        startup_script: startupScript,
        guidelines,
        auto_start_backlog_tasks: autoStartBacklogTasks,
        default_helix_app_id: selectedAgentId || undefined,
        metadata: {
          board_settings: {
            wip_limits: wipLimits,
          },
        },
      })
      console.log('[ProjectSettings] Project settings saved to database')

      if (showSuccessMessage) {
        snackbar.success('Project settings saved')
      }
      console.log('[ProjectSettings] handleSave returning true')
      return true // Indicate save succeeded
    } catch (err) {
      console.error('[ProjectSettings] Failed to save:', err)
      snackbar.error('Failed to save project settings')
      throw err // Re-throw so caller knows it failed
    } finally {
      setSavingProject(false)
    }
  }

  const handleFieldBlur = () => {
    handleSave(false) // Auto-save without showing success message
  }

  const handleCreateAgent = async () => {
    if (!newAgentName.trim()) {
      setAgentError('Please enter a name for the agent')
      return
    }
    if (!selectedModel) {
      setAgentError('Please select a model')
      return
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
        setSelectedAgentId(newApp.id)
        setShowCreateAgentForm(false)
        // Auto-save project with new agent
        await handleSave(true)
      }
    } catch (err) {
      console.error('Failed to create agent:', err)
      setAgentError(err instanceof Error ? err.message : 'Failed to create agent')
    } finally {
      setCreatingAgent(false)
    }
  }

  const handleSetPrimaryRepo = async (repoId: string) => {
    try {
      await setPrimaryRepoMutation.mutateAsync(repoId)
      snackbar.success('Primary repository updated')
    } catch (err) {
      snackbar.error('Failed to update primary repository')
    }
  }

  const handleAttachRepository = async () => {
    if (!selectedRepoToAttach) {
      snackbar.error('Please select a repository')
      return
    }

    try {
      await attachRepoMutation.mutateAsync(selectedRepoToAttach)
      snackbar.success('Repository attached successfully')
      setAttachRepoDialogOpen(false)
      setSelectedRepoToAttach('')
    } catch (err) {
      snackbar.error('Failed to attach repository')
    }
  }

  const handleDetachRepository = async (repoId: string) => {
    try {
      await detachRepoMutation.mutateAsync(repoId)
      snackbar.success('Repository detached successfully')
    } catch (err) {
      snackbar.error('Failed to detach repository')
    }
  }

  const handleTestStartupScript = async () => {
    const isRestart = !!exploratorySessionData
    setIsSessionRestart(isRestart)
    setTestingStartupScript(true)

    try {
      // 1. Save changes first
      const saved = await handleSave(false)
      if (!saved) {
        snackbar.error('Failed to save settings before testing')
        return
      }

      // 2. Stop existing session if running
      if (exploratorySessionData) {
        try {
          await stopExploratorySessionMutation.mutateAsync()
          // Short delay to let stop complete
          await new Promise(resolve => setTimeout(resolve, 1000))
        } catch (err: any) {
          // If session doesn't exist or already stopped, proceed anyway
          const isNotFound = err?.response?.status === 404 ||
                            err?.response?.status === 500 ||
                            err?.message?.includes('not found');
          if (!isNotFound) {
            snackbar.error('Failed to stop existing session')
            return
          }
        }
      }

      // 3. Start new session with fresh startup script
      const session = await startExploratorySessionMutation.mutateAsync()
      snackbar.success('Testing startup script')

      // 4. Wait for data to refetch with new lobby ID
      await queryClient.refetchQueries({ queryKey: projectExploratorySessionQueryKey(projectId) })

      // 5. Show test session viewer
      setShowTestSession(true)
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || err?.message || 'Failed to start exploratory session'
      snackbar.error(errorMessage)
    } finally {
      // Clear loading state after longer delay for restarts (connection takes time)
      // First start: 2 seconds, Restart: 7 seconds (needs time for reconnect retries)
      const delay = isRestart ? 7000 : 2000
      setTimeout(() => setTestingStartupScript(false), delay)
    }
  }

  const handleDeleteProject = async () => {
    if (deleteConfirmName !== project?.name) {
      snackbar.error('Project name does not match')
      return
    }

    try {
      await deleteProjectMutation.mutateAsync(projectId)
      snackbar.success('Project deleted successfully')
      setDeleteDialogOpen(false)
      // Navigate back to projects list
      account.orgNavigate('projects')
    } catch (err) {
      snackbar.error('Failed to delete project')
    }
  }

  const handleCreateAccessGrant = async (request: any) => {
    try {
      const result = await createAccessGrantMutation.mutateAsync(request)
      if (result) {
        snackbar.success('Access grant created successfully')
        return result
      }
      return null
    } catch (err) {
      snackbar.error('Failed to create access grant')
      return null
    }
  }

  const handleDeleteAccessGrant = async (grantId: string) => {
    try {
      await deleteAccessGrantMutation.mutateAsync(grantId)
      snackbar.success('Access grant removed successfully')
      return true
    } catch (err) {
      snackbar.error('Failed to remove access grant')
      return false
    }
  }

  if (isLoading) {
    return (
      <Page breadcrumbTitle="Loading..." orgBreadcrumbs={true}>
        <Container maxWidth="md">
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    )
  }

  if (error || !project) {
    return (
      <Page breadcrumbTitle="Project Settings" orgBreadcrumbs={true}>
        <Container maxWidth="md">
          <Alert severity="error" sx={{ mt: 4 }}>
            {error instanceof Error ? error.message : 'Project not found'}
          </Alert>
        </Container>
      </Page>
    )
  }

  const breadcrumbs = [
    {
      title: 'Projects',
      routeName: 'projects',
    },
    {
      title: project.name,
      routeName: 'project-specs',
      params: { id: projectId },
    },
    {
      title: 'Settings',
    },
  ]

  return (
    <Page
      breadcrumbs={breadcrumbs}
      orgBreadcrumbs={true}
      topbarContent={(
        <Box sx={{ display: 'flex', gap: 2, justifyContent: 'flex-end', width: '100%' }}>
          {/* Save/Load indicator lozenge */}
          {(savingProject || isLoading) && (
            <Chip
              icon={<CircularProgress size={16} sx={{ color: 'inherit !important' }} />}
              label={savingProject ? 'Saving...' : 'Loading...'}
              size="small"
              sx={{
                height: 28,
                backgroundColor: savingProject ? 'rgba(46, 125, 50, 0.1)' : 'rgba(25, 118, 210, 0.1)',
                color: savingProject ? 'success.main' : 'primary.main',
                borderRadius: 20,
              }}
            />
          )}
        </Box>
      )}
    >
      <Container maxWidth={showTestSession ? false : 'md'} sx={{ px: showTestSession ? 3 : 3 }}>
        <Box sx={{ mt: 4, display: 'flex', flexDirection: 'row', gap: 3, width: '100%' }}>
          {/* Left column: Settings sections */}
          <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: 3,
            width: showTestSession ? '600px' : '100%',
            flexShrink: 0,
          }}>
          {/* Basic Information */}
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Basic Information
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <TextField
                label="Project Name"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={handleFieldBlur}
                required
              />
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                onBlur={handleFieldBlur}
              />
            </Box>
          </Paper>

          {/* Startup Script */}
          <Paper sx={{ p: 3 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                <CodeIcon sx={{ mr: 1 }} />
                <Typography variant="h6">
                  Startup Script
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                This script runs when an agent starts working on this project. Use it to install dependencies, start dev servers, etc.
              </Typography>
              <Divider sx={{ mb: 3 }} />

              <StartupScriptEditor
                value={startupScript}
                onChange={setStartupScript}
                onTest={handleTestStartupScript}
                onSave={() => handleSave(true)}
                testDisabled={startExploratorySessionMutation.isPending}
                testLoading={testingStartupScript}
                testTooltip={exploratorySessionData ? 'Will restart the running exploratory session' : undefined}
                projectId={projectId}
              />
            </Paper>

          {/* Repositories */}
          <Paper sx={{ p: 3 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Box>
                <Typography variant="h6" gutterBottom>
                  Repositories
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Repositories attached to this project. The primary repository is opened by default when agents start.
                </Typography>
              </Box>
              <Button
                variant="outlined"
                startIcon={<AddIcon />}
                onClick={() => setAttachRepoDialogOpen(true)}
                size="small"
              >
                Attach Repository
              </Button>
            </Box>
            <Divider sx={{ mb: 2 }} />

            {/* User Code Repositories Section */}
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>
              Code Repositories
            </Typography>

            {repositories.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ textAlign: 'center', py: 4 }}>
                No code repositories attached to this project yet. Click "Attach Repository" to add one.
              </Typography>
            ) : (
              <ProjectRepositoriesList
                repositories={repositories}
                primaryRepoId={project.default_repo_id}
                onSetPrimaryRepo={handleSetPrimaryRepo}
                onDetachRepo={handleDetachRepository}
                setPrimaryRepoPending={setPrimaryRepoMutation.isPending}
                detachRepoPending={detachRepoMutation.isPending}
              />
            )}
          </Paper>

          {/* Default Agent */}
          <Paper sx={{ p: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <SmartToyIcon sx={{ mr: 1 }} />
              <Typography variant="h6">
                Default Agent
              </Typography>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Select the default agent for spec tasks in this project. You can configure MCP servers in the agent settings.
            </Typography>
            <Divider sx={{ mb: 3 }} />

            {!showCreateAgentForm ? (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <FormControl fullWidth size="small">
                  <InputLabel>Select Agent</InputLabel>
                  <Select
                    value={selectedAgentId}
                    label="Select Agent"
                    onChange={(e) => {
                      setSelectedAgentId(e.target.value)
                      // Defer save to avoid state race
                      setTimeout(() => handleSave(false), 0)
                    }}
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
                  startIcon={<AddIcon />}
                  onClick={() => setShowCreateAgentForm(true)}
                  sx={{ alignSelf: 'flex-start' }}
                >
                  Create new agent
                </Button>
              </Box>
            ) : (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Typography variant="subtitle2">Create New Agent</Typography>

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

                <Box sx={{ display: 'flex', gap: 1 }}>
                  <Button
                    variant="contained"
                    onClick={handleCreateAgent}
                    disabled={creatingAgent || !newAgentName.trim() || !selectedModel}
                    startIcon={creatingAgent ? <CircularProgress size={16} /> : undefined}
                  >
                    {creatingAgent ? 'Creating...' : 'Create Agent'}
                  </Button>
                  {sortedApps.length > 0 && (
                    <Button
                      onClick={() => setShowCreateAgentForm(false)}
                      disabled={creatingAgent}
                    >
                      Cancel
                    </Button>
                  )}
                </Box>
              </Box>
            )}
          </Paper>

          {/* Board Settings */}
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Kanban Board Settings
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Configure work-in-progress (WIP) limits for the Kanban board columns.
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 2 }}>
              <TextField
                label="Planning Column Limit"
                value={wipLimits.planning}
                onChange={(e) => setWipLimits({ ...wipLimits, planning: parseInt(e.target.value) || 0 })}
                onBlur={handleFieldBlur}
                helperText="Maximum tasks allowed in Planning column"
              />
              <TextField
                label="Review Column Limit"
                value={wipLimits.review}
                onChange={(e) => setWipLimits({ ...wipLimits, review: parseInt(e.target.value) || 0 })}
                onBlur={handleFieldBlur}
                helperText="Maximum tasks allowed in Review column"
              />
              <TextField
                label="Implementation Column Limit"
                value={wipLimits.implementation}
                onChange={(e) => setWipLimits({ ...wipLimits, implementation: parseInt(e.target.value) || 0 })}
                onBlur={handleFieldBlur}
                helperText="Maximum tasks allowed in Implementation column"
              />
            </Box>
          </Paper>

          {/* Automations */}
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Automations
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Configure automatic task scheduling and workflow automation.
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <FormControlLabel
              control={
                <Checkbox
                  checked={autoStartBacklogTasks}
                  onChange={(e) => {
                    setAutoStartBacklogTasks(e.target.checked)
                    handleFieldBlur()
                  }}
                />
              }
              label={
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    Automatically start backlog items when there's capacity in the planning column
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    When enabled, tasks in the backlog will automatically move to planning when the WIP limit allows.
                    When disabled, tasks must be manually moved from backlog to planning.
                  </Typography>
                </Box>
              }
            />
          </Paper>

          {/* Project Guidelines */}
          <Paper sx={{ p: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <DescriptionIcon sx={{ mr: 1 }} />
                <Typography variant="h6">
                  Project Guidelines
                </Typography>
              </Box>
              {project.guidelines_version && project.guidelines_version > 0 && (
                <Button
                  size="small"
                  startIcon={<HistoryIcon />}
                  onClick={() => setGuidelinesHistoryDialogOpen(true)}
                >
                  History (v{project.guidelines_version})
                </Button>
              )}
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Guidelines specific to this project. These are combined with organization guidelines and sent to AI agents during planning, implementation, and exploratory sessions.
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <TextField
              fullWidth
              multiline
              minRows={4}
              maxRows={12}
              placeholder="Example:
- Use React Query for all API calls
- Follow the existing component patterns in src/components
- Always add unit tests for new features
- Use MUI components for UI elements"
              value={guidelines}
              onChange={(e) => setGuidelines(e.target.value)}
              onBlur={handleFieldBlur}
            />
            {project.guidelines_updated_at && (
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                Last updated: {new Date(project.guidelines_updated_at).toLocaleDateString()}
                {project.guidelines_version ? ` (v${project.guidelines_version})` : ''}
              </Typography>
            )}
          </Paper>

          {/* Members & Access Control */}
          {project?.organization_id ? (
          <Paper sx={{ p: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <PeopleIcon sx={{ mr: 1 }} />
              <Typography variant="h6">
                Members & Access
              </Typography>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Manage who has access to this project and their roles.
            </Typography>
            <Divider sx={{ mb: 3 }} />

              <AccessManagement
                appId={projectId}
                accessGrants={accessGrants}
                isLoading={accessGrantsLoading}
                isReadOnly={project.user_id !== account.user?.id && !account.user?.admin}
                onCreateGrant={handleCreateAccessGrant}
                onDeleteGrant={handleDeleteAccessGrant}
              />
          </Paper>
          ) : null}
          {/* Danger Zone */}
          <Paper sx={{ p: 3, mb: 3, border: '2px solid', borderColor: 'error.main' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <WarningIcon sx={{ mr: 1, color: 'error.main' }} />
              <Typography variant="h6" color="error">
                Danger Zone
              </Typography>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Irreversible and destructive actions.
            </Typography>
            <Divider sx={{ mb: 3 }} />

            <Box sx={{
              p: 2,
              backgroundColor: 'rgba(211, 47, 47, 0.05)',
              borderRadius: 1,
              border: '1px solid',
              borderColor: 'error.light'
            }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                Delete Project
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Once you delete a project, there is no going back. This will permanently delete the project, all its tasks, and associated data.
              </Typography>
              <Button
                variant="outlined"
                color="error"
                startIcon={<DeleteForeverIcon />}
                onClick={() => setDeleteDialogOpen(true)}
              >
                Delete This Project
              </Button>
            </Box>
          </Paper>
          </Box>
          {/* End of left column */}

          {/* Test session viewer - fills width, natural height */}
          {showTestSession && exploratorySessionData && (
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
              {/* Spacer to align with Startup Script section (Basic Info section ~180px) */}
              <Box sx={{ height: '310px' }} />

            <Paper sx={{ p: 4 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                <Typography variant="h6" sx={{ flex: 1 }}>
                  Test Session
                </Typography>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => setShowTestSession(false)}
                >
                  Hide
                </Button>
              </Box>
              <Divider sx={{ mb: 3 }} />
              {/* Stream viewer - matches startup script editor height */}
              <Box
                sx={{
                  height: 500, // Slightly taller than Monaco editor to account for toolbar
                  backgroundColor: '#000',
                  overflow: 'hidden',
                }}
              >
                <MoonlightStreamViewer
                  sessionId={exploratorySessionData.id}
                  wolfLobbyId={exploratorySessionData.config?.wolf_lobby_id || ''}
                  showLoadingOverlay={testingStartupScript}
                  isRestart={isSessionRestart}
                />
              </Box>
              <Box sx={{ mt: 2, p: 2, backgroundColor: 'action.hover', borderRadius: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  Having trouble with your startup script?
                </Typography>
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={createSpecTaskMutation.isPending ? <CircularProgress size={16} /> : <AutoFixHighIcon />}
                  onClick={() => createSpecTaskMutation.mutate(
                    `Fix the project startup script at .helix/startup.sh. The current script is:\n\n\`\`\`bash\n${startupScript}\n\`\`\`\n\nPlease review and fix any issues. You can run the script to test it and iterate on it until it works. It should be idempotent. Once the user approves the changes, you will then push the changes to the repository, at which point the user can test it in the projects settings panel.`
                  )}
                  disabled={createSpecTaskMutation.isPending}
                >
                  {createSpecTaskMutation.isPending ? 'Creating task...' : 'Get AI to fix it'}
                </Button>
              </Box>
            </Paper>
            </Box>
          )}
        </Box>
      </Container>

      {/* Attach Repository Dialog */}
      <Dialog
        open={attachRepoDialogOpen}
        onClose={() => {
          setAttachRepoDialogOpen(false)
          setSelectedRepoToAttach('')
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <LinkIcon />
            Attach Repository to Project
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Select a repository from your account to attach to this project. Attached repositories will be cloned into the agent workspace when working on this project.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>Select Repository</InputLabel>
            <Select
              value={selectedRepoToAttach}
              onChange={(e) => setSelectedRepoToAttach(e.target.value)}
              label="Select Repository"
            >
              {allUserRepositories
                .filter((repo) => !repositories.some((pr) => pr.id === repo.id))
                .map((repo) => (
                  <MenuItem key={repo.id} value={repo.id}>
                    {repo.name}
                  </MenuItem>
                ))}
            </Select>
            {allUserRepositories.filter((repo) => !repositories.some((pr) => pr.id === repo.id)).length === 0 && (
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                All your repositories are already attached to this project.
              </Typography>
            )}
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAttachRepoDialogOpen(false)
              setSelectedRepoToAttach('')
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleAttachRepository}
            variant="contained"
            disabled={!selectedRepoToAttach || attachRepoMutation.isPending}
            startIcon={attachRepoMutation.isPending ? <CircularProgress size={16} /> : <LinkIcon />}
          >
            {attachRepoMutation.isPending ? 'Attaching...' : 'Attach Repository'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          setDeleteDialogOpen(false);
          setDeleteConfirmName('');
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <WarningIcon color="error" />
            <span>Delete Project</span>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Alert severity="error" sx={{ mb: 3 }}>
            <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
              This action cannot be undone!
            </Typography>
            <Typography variant="body2">
              This will permanently delete the project <strong>{project?.name}</strong>, all its tasks, work sessions, and associated data.
            </Typography>
          </Alert>

          <Typography variant="body2" sx={{ mb: 2 }}>
            Please type the project name <strong>{project?.name}</strong> to confirm:
          </Typography>

          <TextField
            fullWidth
            value={deleteConfirmName}
            onChange={(e) => setDeleteConfirmName(e.target.value)}
            placeholder={project?.name}
            autoFocus
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setDeleteDialogOpen(false);
              setDeleteConfirmName('');
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleDeleteProject}
            variant="contained"
            color="error"
            disabled={deleteConfirmName !== project?.name || deleteProjectMutation.isPending}
            startIcon={deleteProjectMutation.isPending ? <CircularProgress size={16} /> : <DeleteForeverIcon />}
          >
            {deleteProjectMutation.isPending ? 'Deleting...' : 'Delete Project'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Guidelines History Dialog */}
      <Dialog
        open={guidelinesHistoryDialogOpen}
        onClose={() => setGuidelinesHistoryDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <HistoryIcon />
            Guidelines Version History
          </Box>
        </DialogTitle>
        <DialogContent>
          {guidelinesHistory.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
              No previous versions found. History is created when guidelines are modified.
            </Typography>
          ) : (
            <List>
              {guidelinesHistory.map((entry, index) => (
                <ListItem
                  key={entry.id}
                  sx={{
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                    borderBottom: index < guidelinesHistory.length - 1 ? '1px solid' : 'none',
                    borderColor: 'divider',
                    py: 2,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 1, width: '100%' }}>
                    <Chip label={`v${entry.version}`} size="small" color="primary" variant="outlined" />
                    <Typography variant="body2" color="text.secondary">
                      {entry.updated_at ? new Date(entry.updated_at).toLocaleString() : 'Unknown date'}
                    </Typography>
                    {(entry.updated_by_name || entry.updated_by_email) && (
                      <Typography variant="caption" color="text.secondary">
                        by {entry.updated_by_name || 'Unknown'}{entry.updated_by_email ? ` (${entry.updated_by_email})` : ''}
                      </Typography>
                    )}
                  </Box>
                  <Typography
                    variant="body2"
                    sx={{
                      whiteSpace: 'pre-wrap',
                      fontFamily: 'monospace',
                      fontSize: '0.85rem',
                      backgroundColor: 'action.hover',
                      p: 1.5,
                      borderRadius: 1,
                      width: '100%',
                      maxHeight: 200,
                      overflow: 'auto',
                    }}
                  >
                    {entry.guidelines || '(empty)'}
                  </Typography>
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setGuidelinesHistoryDialogOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  )
}

export default ProjectSettings
