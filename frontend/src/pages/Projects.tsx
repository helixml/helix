import React, { FC, useState, useEffect } from 'react'
import {
  Container,
  Box,
  Button,
  Card,
  CardContent,
  CardActions,
  Grid,
  Typography,
  IconButton,
  Menu,
  MenuItem,
  Alert,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Tabs,
  Tab,
  Stack,
  FormControl,
  InputLabel,
  Select,
  FormControlLabel,
  Switch,
  Chip,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SettingsIcon from '@mui/icons-material/Settings'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import { Kanban, GitBranch, Plus, Link, Brain, ExternalLink } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import CreateProjectButton from '../components/project/CreateProjectButton'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import {
  useListProjects,
  useCreateProject,
  useListSampleProjects,
  useInstantiateSampleProject,
  TypesProject,
} from '../services'
import { useGitRepositories, getSampleTypeIcon } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import type { ServicesGitRepository, ServerSampleType } from '../api/api'

const Projects: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const { navigate } = router
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const api = useApi()

  const { data: projects = [], isLoading, error } = useListProjects()
  const { data: sampleProjects = [] } = useListSampleProjects()
  const createProjectMutation = useCreateProject()
  const instantiateSampleMutation = useInstantiateSampleProject()

  // Get tab from URL query parameter, default to 0
  const urlTab = router.route?.params?.tab || '0'
  const [currentTab, setCurrentTab] = useState(urlTab === 'repositories' ? 1 : 0)
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedProject, setSelectedProject] = useState<TypesProject | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectDescription, setNewProjectDescription] = useState('')

  // Repository management
  const currentOrg = account.organizationTools.organization
  const ownerId = currentOrg?.id || account.user?.id || ''
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'
  const { data: repositories = [], isLoading: reposLoading } = useGitRepositories(ownerId)
  const { data: sampleTypes, loading: sampleTypesLoading, createSampleRepository } = useSampleTypes()

  // Repository dialog states
  const [createRepoDialogOpen, setCreateRepoDialogOpen] = useState(false)
  const [demoRepoDialogOpen, setDemoRepoDialogOpen] = useState(false)
  const [linkRepoDialogOpen, setLinkRepoDialogOpen] = useState(false)
  const [selectedSampleType, setSelectedSampleType] = useState('')
  const [demoRepoName, setDemoRepoName] = useState('')
  const [demoKoditIndexing, setDemoKoditIndexing] = useState(true)
  const [repoName, setRepoName] = useState('')
  const [repoDescription, setRepoDescription] = useState('')
  const [koditIndexing, setKoditIndexing] = useState(true)

  // External repository states
  const [externalRepoName, setExternalRepoName] = useState('')
  const [externalRepoUrl, setExternalRepoUrl] = useState('')
  const [externalRepoType, setExternalRepoType] = useState<'github' | 'gitlab' | 'ado' | 'other'>('github')
  const [externalKoditIndexing, setExternalKoditIndexing] = useState(true)

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string>('')

  // Update URL when tab changes
  useEffect(() => {
    const tabParam = currentTab === 1 ? 'repositories' : undefined
    const newParams = { ...router.route?.params }
    if (tabParam) {
      newParams.tab = tabParam
    } else {
      delete newParams.tab
    }
    // Use router5's navigate with params to update URL
    navigate('projects', newParams, { replace: true })
  }, [currentTab])

  // Sync tab from URL parameter
  useEffect(() => {
    const urlTab = router.route?.params?.tab
    if (urlTab === 'repositories' && currentTab !== 1) {
      setCurrentTab(1)
    } else if (!urlTab && currentTab !== 0) {
      setCurrentTab(0)
    }
  }, [router.route?.params?.tab])

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, project: TypesProject) => {
    setAnchorEl(event.currentTarget)
    setSelectedProject(project)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedProject(null)
  }

  const handleCreateProject = async () => {
    if (!newProjectName.trim()) {
      snackbar.error('Project name is required')
      return
    }

    try {
      const result = await createProjectMutation.mutateAsync({
        name: newProjectName,
        description: newProjectDescription,
      })
      snackbar.success('Project created successfully')
      setCreateDialogOpen(false)
      setNewProjectName('')
      setNewProjectDescription('')

      // Navigate to the new project
      if (result) {
        account.orgNavigate('project-specs', { id: result.id })
      }
    } catch (err) {
      snackbar.error('Failed to create project')
    }
  }

  const handleViewProject = (project: TypesProject) => {
    account.orgNavigate('project-specs', { id: project.id })
  }

  const handleProjectSettings = () => {
    if (selectedProject) {
      account.orgNavigate('project-settings', { id: selectedProject.id })
    }
    handleMenuClose()
  }

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const handleNewProject = () => {
    if (!checkLoginStatus()) return
    setCreateDialogOpen(true)
  }

  const handleInstantiateSample = async (sampleId: string, sampleName: string) => {
    if (!checkLoginStatus()) return

    try {
      snackbar.info(`Creating ${sampleName}...`)

      const result = await instantiateSampleMutation.mutateAsync({
        sampleId,
        request: { project_name: sampleName }, // Use sample name as default
      })

      snackbar.success('Sample project created successfully!')

      // Navigate to the new project
      if (result && result.project_id) {
        account.orgNavigate('project-specs', { id: result.project_id })
      }
    } catch (err) {
      snackbar.error('Failed to create sample project')
    }
  }

  // Auto-fill name when sample type is selected
  const handleSampleTypeChange = (sampleTypeId: string) => {
    setSelectedSampleType(sampleTypeId)

    // Auto-generate default name from sample type
    if (sampleTypeId) {
      const selectedType = sampleTypes.find((t: ServerSampleType) => t.id === sampleTypeId)
      if (selectedType?.name) {
        // Generate a name like "nodejs-todo-repo" from "Node.js Todo App"
        const defaultName = selectedType.name
          .toLowerCase()
          .replace(/[^a-z0-9\s-]/g, '')
          .replace(/\s+/g, '-')
          .replace(/-+/g, '-')
        setDemoRepoName(defaultName)
      }
    }
  }

  const handleCreateDemoRepo = async () => {
    if (!selectedSampleType || !ownerId || !demoRepoName.trim()) return

    setCreating(true)
    try {
      await createSampleRepository({
        owner_id: ownerId,
        sample_type: selectedSampleType,
        name: demoRepoName,
        kodit_indexing: demoKoditIndexing,
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setDemoRepoDialogOpen(false)
      setSelectedSampleType('')
      setDemoRepoName('')
      setDemoKoditIndexing(true)
    } catch (error) {
      console.error('Failed to create demo repository:', error)
      snackbar.error('Failed to create demo repository')
    } finally {
      setCreating(false)
    }
  }

  const handleCreateCustomRepo = async () => {
    if (!repoName.trim() || !ownerId) return

    setCreating(true)
    setCreateError('')
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: repoDescription,
        owner_id: ownerId,
        repo_type: 'code' as any, // Helix-hosted code repository
        default_branch: 'main',
        metadata: {
          kodit_indexing: koditIndexing,
        },
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setCreateRepoDialogOpen(false)
      setRepoName('')
      setRepoDescription('')
      setKoditIndexing(true)
      setCreateError('')
      snackbar.success('Repository created successfully')
    } catch (error) {
      console.error('Failed to create repository:', error)
      setCreateError(error instanceof Error ? error.message : 'Failed to create repository')
      snackbar.error('Failed to create repository')
    } finally {
      setCreating(false)
    }
  }

  const handleLinkExternalRepo = async () => {
    if (!externalRepoUrl.trim() || !ownerId) return

    setCreating(true)
    try {
      const apiClient = api.getApiClient()

      // Extract repo name from URL if not provided
      let repoName = externalRepoName.trim()
      if (!repoName) {
        // Try to extract from URL (e.g., github.com/org/repo.git -> repo)
        const match = externalRepoUrl.match(/\/([^\/]+?)(\.git)?$/)
        repoName = match ? match[1] : 'external-repo'
      }

      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: `External ${externalRepoType} repository`,
        owner_id: ownerId,
        repo_type: 'project' as any,
        default_branch: 'main',
        metadata: {
          is_external: true,
          external_url: externalRepoUrl,
          external_type: externalRepoType,
          kodit_indexing: externalKoditIndexing,
        },
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setLinkRepoDialogOpen(false)
      setExternalRepoName('')
      setExternalRepoUrl('')
      setExternalRepoType('github')
      setExternalKoditIndexing(true)
      snackbar.success('External repository linked successfully')
    } catch (error) {
      console.error('Failed to link external repository:', error)
      snackbar.error('Failed to link external repository')
    } finally {
      setCreating(false)
    }
  }

  const handleViewRepository = (repo: ServicesGitRepository) => {
    account.orgNavigate('git-repo-detail', { repoId: repo.id })
  }

  if (isLoading || reposLoading) {
    return (
      <Page
        breadcrumbTitle="Projects"
        orgBreadcrumbs={true}
      >
        <Container maxWidth="lg">
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      breadcrumbTitle="Projects"
      orgBreadcrumbs={true}
      topbarContent={(
        <CreateProjectButton
          onCreateEmpty={handleNewProject}
          onCreateFromSample={handleInstantiateSample}
          sampleProjects={sampleProjects}
          isCreating={createProjectMutation.isPending || instantiateSampleMutation.isPending}
          variant="contained"
          color="secondary"
        />
      )}
    >
      <Container maxWidth="lg">
        {/* Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mt: 2 }}>
          <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
            <Tab label="Projects" />
            <Tab label="Repositories" />
          </Tabs>
        </Box>

        <Box sx={{ mt: 4 }}>
          {/* Tab 0: Projects */}
          {currentTab === 0 && (
            <>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error instanceof Error ? error.message : 'Failed to load projects'}
            </Alert>
          )}

          {projects.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Box sx={{ color: 'text.disabled', mb: 2 }}>
                <Kanban size={80} />
              </Box>
              <Typography variant="h6" color="text.secondary" gutterBottom>
                No projects yet
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Create your first project to get started
              </Typography>
              <CreateProjectButton
                onCreateEmpty={handleNewProject}
                onCreateFromSample={handleInstantiateSample}
                sampleProjects={sampleProjects}
                isCreating={createProjectMutation.isPending || instantiateSampleMutation.isPending}
                variant="contained"
                color="primary"
              />
            </Box>
          ) : (
            <Grid container spacing={3}>
              {projects.map((project) => (
                <Grid item xs={12} sm={6} md={4} key={project.id}>
                  <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                    <CardContent sx={{ flexGrow: 1, cursor: 'pointer' }} onClick={() => handleViewProject(project)}>
                      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                        <Kanban size={40} style={{ color: '#1976d2' }} />
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation()
                            handleMenuOpen(e, project)
                          }}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </Box>
                      <Typography variant="h6" gutterBottom>
                        {project.name}
                      </Typography>
                      {project.description && (
                        <Typography variant="body2" color="text.secondary" sx={{
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                        }}>
                          {project.description}
                        </Typography>
                      )}
                    </CardContent>
                    <CardActions>
                      <Button size="small" onClick={() => handleViewProject(project)}>
                        Open
                      </Button>
                      <Button
                        size="small"
                        startIcon={<SettingsIcon />}
                        onClick={(e) => {
                          e.stopPropagation()
                          account.orgNavigate('project-settings', { id: project.id })
                        }}
                      >
                        Settings
                      </Button>
                    </CardActions>
                  </Card>
                </Grid>
              ))}
            </Grid>
          )}
            </>
          )}

          {/* Tab 1: Repositories */}
          {currentTab === 1 && (
            <>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
                <Typography variant="h5">
                  Your Repositories
                </Typography>
                <Button
                  variant="contained"
                  color="secondary"
                  startIcon={<AddIcon />}
                  onClick={() => setCreateRepoDialogOpen(true)}
                >
                  New Repository
                </Button>
              </Box>

              {repositories.length === 0 ? (
                <Box sx={{ textAlign: 'center', py: 8 }}>
                  <Box sx={{ color: 'text.disabled', mb: 2 }}>
                    <GitBranch size={80} />
                  </Box>
                  <Typography variant="h6" color="text.secondary" gutterBottom>
                    No repositories yet
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                    Create your first repository to get started
                  </Typography>
                  <Button
                    variant="contained"
                    color="primary"
                    startIcon={<AddIcon />}
                    onClick={() => setCreateRepoDialogOpen(true)}
                  >
                    Create Repository
                  </Button>
                </Box>
              ) : (
                <Grid container spacing={3}>
                  {repositories.map((repo) => (
                    <Grid item xs={12} sm={6} md={4} key={repo.id}>
                      <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                        <CardContent sx={{ flexGrow: 1, cursor: 'pointer' }} onClick={() => handleViewRepository(repo)}>
                          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                            <GitBranch size={40} style={{ color: '#1976d2' }} />
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation()
                                handleRepoMenuOpen(e, repo)
                              }}
                            >
                              <MoreVertIcon />
                            </IconButton>
                          </Box>
                          <Typography variant="h6" gutterBottom>
                            {repo.name}
                          </Typography>
                          {repo.description && (
                            <Typography variant="body2" color="text.secondary" sx={{
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              display: '-webkit-box',
                              WebkitLineClamp: 2,
                              WebkitBoxOrient: 'vertical',
                            }}>
                              {repo.description}
                            </Typography>
                          )}
                        </CardContent>
                        <CardActions>
                          <Button size="small" onClick={() => handleViewRepository(repo)}>
                            Open
                          </Button>
                        </CardActions>
                      </Card>
                    </Grid>
                  ))}
                </Grid>
              )}
            </>
          )}
        </Box>

        {/* Project Menu */}
        <Menu
          anchorEl={anchorEl}
          open={Boolean(anchorEl)}
          onClose={handleMenuClose}
        >
          <MenuItem onClick={handleProjectSettings}>
            <SettingsIcon sx={{ mr: 1 }} fontSize="small" />
            Settings
          </MenuItem>
        </Menu>

        {/* Repository Menu */}
        <Menu
          anchorEl={repoMenuAnchor}
          open={Boolean(repoMenuAnchor)}
          onClose={handleRepoMenuClose}
        >
          <MenuItem onClick={() => {
            if (selectedRepo) {
              handleDeleteRepository(selectedRepo.id)
            }
          }}>
            <DeleteIcon sx={{ mr: 1 }} fontSize="small" />
            Delete
          </MenuItem>
        </Menu>

        {/* Create Project Dialog */}
        <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Create New Project</DialogTitle>
          <DialogContent>
            <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
              <TextField
                label="Project Name"
                fullWidth
                value={newProjectName}
                onChange={(e) => setNewProjectName(e.target.value)}
                autoFocus
              />
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={newProjectDescription}
                onChange={(e) => setNewProjectDescription(e.target.value)}
              />
            </Box>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setCreateDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleCreateProject}
              disabled={createProjectMutation.isPending}
            >
              {createProjectMutation.isPending ? 'Creating...' : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Create Repository Dialog */}
        <Dialog open={createRepoDialogOpen} onClose={() => setCreateRepoDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Create New Repository</DialogTitle>
          <DialogContent>
            <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
              <TextField
                label="Repository Name"
                fullWidth
                value={newRepoName}
                onChange={(e) => setNewRepoName(e.target.value)}
                autoFocus
              />
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={newRepoDescription}
                onChange={(e) => setNewRepoDescription(e.target.value)}
              />
            </Box>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setCreateRepoDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleCreateRepository}
              disabled={createRepoMutation.isPending}
            >
              {createRepoMutation.isPending ? 'Creating...' : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default Projects
