import React, { FC, useState, useEffect } from 'react'
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
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
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

import Page from '../components/system/Page'
import AccessManagement from '../components/app/AccessManagement'
import StartupScriptEditor from '../components/project/StartupScriptEditor'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import {
  useGetProject,
  useUpdateProject,
  useGetProjectRepositories,
  useSetProjectPrimaryRepository,
  useAttachRepositoryToProject,
  useDetachRepositoryFromProject,
  useDeleteProject,
  useGetBoardSettings,
  useUpdateBoardSettings,
  useGetProjectExploratorySession,
  useStartProjectExploratorySession,
  useStopProjectExploratorySession,
} from '../services'
import { useGitRepositories } from '../services/gitRepositoryService'
import {
  useListProjectAccessGrants,
  useCreateProjectAccessGrant,
  useDeleteProjectAccessGrant,
} from '../services/projectAccessGrantService'
import { useDeleteSession } from '../services/sessionService'

const ProjectSettings: FC = () => {
  const account = useAccount()
  const { params, navigate } = useRouter()
  const snackbar = useSnackbar()
  const projectId = params.id as string

  const { data: project, isLoading, error } = useGetProject(projectId)
  const { data: allRepositories = [] } = useGetProjectRepositories(projectId)

  // Separate internal repo from code repos
  const internalRepo = allRepositories.find(repo => repo.id?.endsWith('-internal'))
  const repositories = allRepositories.filter(repo => !repo.id?.endsWith('-internal'))

  const updateProjectMutation = useUpdateProject(projectId)
  const setPrimaryRepoMutation = useSetProjectPrimaryRepository(projectId)
  const attachRepoMutation = useAttachRepositoryToProject(projectId)
  const detachRepoMutation = useDetachRepositoryFromProject(projectId)
  const deleteProjectMutation = useDeleteProject()

  // Get current org/user ID for fetching all user repositories
  const currentOrg = account.organizationTools.organization
  const ownerId = currentOrg?.id || account.user?.id || ''
  const { data: allUserRepositories = [] } = useGitRepositories(ownerId)

  // Access grants for RBAC
  const { data: accessGrants = [], isLoading: accessGrantsLoading } = useListProjectAccessGrants(projectId, !!project?.organization_id)
  const createAccessGrantMutation = useCreateProjectAccessGrant(projectId)
  const deleteAccessGrantMutation = useDeleteProjectAccessGrant(projectId)

  // Board settings
  const { data: boardSettingsData } = useGetBoardSettings()
  const updateBoardSettingsMutation = useUpdateBoardSettings()

  // Exploratory session
  const { data: exploratorySessionData } = useGetProjectExploratorySession(projectId)
  const startExploratorySessionMutation = useStartProjectExploratorySession(projectId)
  const stopExploratorySessionMutation = useStopProjectExploratorySession(projectId)
  const deleteSessionMutation = useDeleteSession(exploratorySessionData?.id || '')

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [startupScript, setStartupScript] = useState('')
  const [testStartupScriptDialogOpen, setTestStartupScriptDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteConfirmName, setDeleteConfirmName] = useState('')
  const [attachRepoDialogOpen, setAttachRepoDialogOpen] = useState(false)
  const [selectedRepoToAttach, setSelectedRepoToAttach] = useState('')
  const [savingProject, setSavingProject] = useState(false)

  // Board settings state (initialized from query data)
  const [wipLimits, setWipLimits] = useState({
    planning: 3,
    review: 2,
    implementation: 5,
  })

  useEffect(() => {
    if (project) {
      setName(project.name || '')
      setDescription(project.description || '')
      setStartupScript(project.startup_script || '')
    }
  }, [project])

  // Load board settings from query data
  useEffect(() => {
    if (boardSettingsData?.wip_limits) {
      setWipLimits({
        planning: boardSettingsData.wip_limits.planning || 3,
        review: boardSettingsData.wip_limits.review || 2,
        implementation: boardSettingsData.wip_limits.implementation || 5,
      })
    }
  }, [boardSettingsData])

  const handleSave = async (showSuccessMessage = true) => {
    if (savingProject) return // Prevent concurrent saves

    try {
      setSavingProject(true)
      // Save project basic settings
      await updateProjectMutation.mutateAsync({
        name,
        description,
        startup_script: startupScript,
      })

      // Save board settings
      await updateBoardSettingsMutation.mutateAsync({
        wip_limits: wipLimits,
      })

      if (showSuccessMessage) {
        snackbar.success('Project settings saved')
      }
    } catch (err) {
      snackbar.error('Failed to save project settings')
    } finally {
      setSavingProject(false)
    }
  }

  const handleFieldBlur = () => {
    handleSave(false) // Auto-save without showing success message
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

  const handleStartExploratorySession = async () => {
    try {
      const session = await startExploratorySessionMutation.mutateAsync()
      snackbar.success('Exploratory session started')
      // Navigate to the project-scoped session
      account.orgNavigate('project-session', { id: projectId, session_id: session.id })
    } catch (err) {
      snackbar.error('Failed to start exploratory session')
    }
  }

  const handleStopExploratorySession = async () => {
    try {
      await stopExploratorySessionMutation.mutateAsync()
      snackbar.success('Exploratory session stopped')
    } catch (err) {
      snackbar.error('Failed to stop exploratory session')
    }
  }

  const handleTestStartupScript = async () => {
    // First, ensure all changes are saved
    await handleSave(false)
    // Open dialog - exploratorySessionData will already be set from the query
    setTestStartupScriptDialogOpen(true)
  }

  const handleConfirmTestStartupScript = async () => {
    setTestStartupScriptDialogOpen(false)

    // If session already exists, delete it first to restart with fresh startup script
    if (exploratorySessionData) {
      try {
        await deleteSessionMutation.mutateAsync()
        // Short delay to let the delete complete
        await new Promise(resolve => setTimeout(resolve, 1000))
      } catch (err) {
        console.error('Failed to delete existing session:', err)
        snackbar.error('Failed to delete existing session')
        return
      }
    }

    // Start a new session (with the updated startup script from the saved project)
    await handleStartExploratorySession()
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
          {!exploratorySessionData ? (
            <Button
              variant="outlined"
              color="secondary"
              startIcon={<ExploreIcon />}
              onClick={handleStartExploratorySession}
              disabled={startExploratorySessionMutation.isPending}
            >
              {startExploratorySessionMutation.isPending ? 'Starting...' : 'Start Exploratory Session'}
            </Button>
          ) : (
            <>
              <Button
                variant="contained"
                color="primary"
                startIcon={<ExploreIcon />}
                onClick={() => account.orgNavigate('project-session', { id: projectId, session_id: exploratorySessionData.id })}
              >
                View Session
              </Button>
              <Button
                variant="outlined"
                color="error"
                startIcon={<StopIcon />}
                onClick={handleStopExploratorySession}
                disabled={stopExploratorySessionMutation.isPending}
              >
                {stopExploratorySessionMutation.isPending ? 'Stopping...' : 'Stop Session'}
              </Button>
            </>
          )}
        </Box>
      )}
    >
      <Container maxWidth="md">
        <Box sx={{ mt: 4, display: 'flex', flexDirection: 'column', gap: 3 }}>
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
              testDisabled={startExploratorySessionMutation.isPending}
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
              <List>
                {repositories.map((repo) => (
                  <ListItem key={repo.id} divider>
                    <ListItemText
                      primary={repo.name}
                      secondary={repo.clone_url}
                    />
                    <ListItemSecondaryAction>
                      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                        {project.default_repo_id === repo.id ? (
                          <Chip
                            icon={<StarIcon />}
                            label="Primary"
                            color="primary"
                            size="small"
                          />
                        ) : (
                          <IconButton
                            onClick={() => handleSetPrimaryRepo(repo.id)}
                            disabled={setPrimaryRepoMutation.isPending}
                            title="Set as primary"
                          >
                            <StarBorderIcon />
                          </IconButton>
                        )}
                        <IconButton
                          onClick={() => handleDetachRepository(repo.id)}
                          disabled={detachRepoMutation.isPending}
                          title="Detach from project"
                          color="error"
                        >
                          <DeleteIcon />
                        </IconButton>
                      </Box>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            )}

            {/* Internal Repository Section - MOVED TO BOTTOM */}
            {internalRepo && (
              <>
                <Divider sx={{ my: 2 }} />
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>
                  Internal Repository
                </Typography>
                <List>
                  <ListItem
                    sx={{
                      border: 1,
                      borderColor: 'divider',
                      borderRadius: 1,
                      backgroundColor: 'rgba(0, 0, 0, 0.02)',
                      cursor: 'pointer',
                      '&:hover': {
                        backgroundColor: 'rgba(0, 0, 0, 0.04)',
                      },
                    }}
                    onClick={() => {
                      account.orgNavigate('git-repo-detail', { repoId: internalRepo.id });
                    }}
                  >
                    <ListItemText
                      primary={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            {internalRepo.name}
                          </Typography>
                          <Chip label="Project Config" size="small" variant="outlined" />
                        </Box>
                      }
                      secondary="Stores .helix/project.json and .helix/startup.sh"
                    />
                  </ListItem>
                </List>
              </>
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

          {/* Members & Access Control */}
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

            {project?.organization_id ? (
              <AccessManagement
                appId={projectId}
                accessGrants={accessGrants}
                isLoading={accessGrantsLoading}
                isReadOnly={project.user_id !== account.user?.id && !account.user?.admin}
                onCreateGrant={handleCreateAccessGrant}
                onDeleteGrant={handleDeleteAccessGrant}
              />
            ) : (
              <Box sx={{ textAlign: 'center', py: 4, backgroundColor: 'rgba(0, 0, 0, 0.02)', borderRadius: 1 }}>
                <Typography variant="body2" color="text.secondary">
                  This project is not part of an organization. Only the owner can access it.
                </Typography>
              </Box>
            )}
          </Paper>

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
      </Container>

      {/* Test Startup Script Confirmation Dialog */}
      <Dialog
        open={testStartupScriptDialogOpen}
        onClose={() => setTestStartupScriptDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <ExploreIcon color="primary" />
            <span>{exploratorySessionData ? 'Join Exploratory Session' : 'Test Startup Script'}</span>
          </Box>
        </DialogTitle>
        <DialogContent>
          {exploratorySessionData ? (
            <>
              <Alert severity="warning" sx={{ mb: 2 }}>
                <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                  This will restart the existing exploratory session
                </Typography>
                <Typography variant="body2">
                  The current session will be stopped and a new one will be started with the updated startup script. Any unsaved work in the session will be lost.
                </Typography>
              </Alert>
              <Typography variant="body2" sx={{ mb: 1 }}>
                <strong>Current Session:</strong> {exploratorySessionData.name}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                <strong>Created:</strong> {new Date(exploratorySessionData.created).toLocaleString()}
              </Typography>
            </>
          ) : (
            <>
              <Typography variant="body2" sx={{ mb: 2 }}>
                This will create a new exploratory session for this project with the current startup script.
              </Typography>
              <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="body2">
                  The startup script will run when the agent environment launches. You'll be able to connect and verify it worked correctly.
                </Typography>
              </Alert>
            </>
          )}
          {!exploratorySessionData && (
            <>
              {startupScript.trim() ? (
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                    Current startup script:
                  </Typography>
                  <Box sx={{
                    p: 1.5,
                    backgroundColor: 'rgba(0, 0, 0, 0.05)',
                    borderRadius: 1,
                    fontFamily: 'monospace',
                    fontSize: '0.75rem',
                    maxHeight: 200,
                    overflow: 'auto',
                    whiteSpace: 'pre-wrap'
                  }}>
                    {startupScript}
                  </Box>
                </Box>
              ) : (
                <Alert severity="warning">
                  No startup script configured. The session will start without any initialization.
                </Alert>
              )}
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setTestStartupScriptDialogOpen(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmTestStartupScript}
            variant="contained"
            color={exploratorySessionData ? 'warning' : 'primary'}
            startIcon={<ExploreIcon />}
          >
            {exploratorySessionData ? 'Restart Session' : 'Start Session'}
          </Button>
        </DialogActions>
      </Dialog>

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
    </Page>
  )
}

export default ProjectSettings
