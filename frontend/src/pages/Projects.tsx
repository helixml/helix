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
  Stack,
  FormControl,
  InputLabel,
  Select,
  FormControlLabel,
  Switch,
  Chip,
  Pagination,
  InputAdornment,
  List,
  ListItemButton,
  ListItemText,
  Paper,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SettingsIcon from '@mui/icons-material/Settings'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import SearchIcon from '@mui/icons-material/Search'
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
import { useGitRepositories } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import { getSampleProjectIcon } from '../utils/sampleProjectIcons'
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

  // Get view from URL query parameter
  const currentView = router.route?.params?.view || 'projects'
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

  // Search and pagination for projects
  const [projectsSearchQuery, setProjectsSearchQuery] = useState('')
  const [projectsPage, setProjectsPage] = useState(0)
  const projectsPerPage = 12

  // Search and pagination for repositories
  const [reposSearchQuery, setReposSearchQuery] = useState('')
  const [reposPage, setReposPage] = useState(0)
  const reposPerPage = 10

  // Filter and paginate projects
  const filteredProjects = projects.filter(project =>
    project.name?.toLowerCase().includes(projectsSearchQuery.toLowerCase()) ||
    project.description?.toLowerCase().includes(projectsSearchQuery.toLowerCase())
  )
  const paginatedProjects = filteredProjects.slice(
    projectsPage * projectsPerPage,
    (projectsPage + 1) * projectsPerPage
  )
  const projectsTotalPages = Math.ceil(filteredProjects.length / projectsPerPage)

  // Filter and paginate repositories
  const filteredRepositories = repositories.filter((repo: ServicesGitRepository) =>
    repo.name?.toLowerCase().includes(reposSearchQuery.toLowerCase()) ||
    repo.description?.toLowerCase().includes(reposSearchQuery.toLowerCase())
  )
  const paginatedRepositories = filteredRepositories.slice(
    reposPage * reposPerPage,
    (reposPage + 1) * reposPerPage
  )
  const reposTotalPages = Math.ceil(filteredRepositories.length / reposPerPage)

  // Handle view change
  const handleViewChange = (view: 'projects' | 'repositories') => {
    const newParams = view === 'repositories' ? { view: 'repositories' } : {}
    navigate('projects', newParams, { replace: true })
  }

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

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

      setDemoRepoDialogOpen(false)
      setSelectedSampleType('')
      setDemoRepoName('')
      setDemoKoditIndexing(true)
      snackbar.success('Demo repository created successfully')
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

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

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

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

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
      topbarContent={currentView === 'projects' ? (
        <CreateProjectButton
          onCreateEmpty={handleNewProject}
          onCreateFromSample={handleInstantiateSample}
          sampleProjects={sampleProjects}
          isCreating={createProjectMutation.isPending || instantiateSampleMutation.isPending}
          variant="contained"
          color="secondary"
        />
      ) : null}
    >
      <Container maxWidth="xl" sx={{ display: 'flex', gap: 3, py: 3 }}>
        {/* Left Sidebar */}
        <Paper
          elevation={0}
          sx={{
            width: 240,
            flexShrink: 0,
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 2,
            overflow: 'hidden',
          }}
        >
          <List component="nav" disablePadding>
            <ListItemButton
              selected={currentView === 'projects'}
              onClick={() => handleViewChange('projects')}
              sx={{
                py: 1.5,
                '&.Mui-selected': {
                  backgroundColor: 'action.selected',
                  borderLeft: '3px solid',
                  borderLeftColor: 'primary.main',
                },
              }}
            >
              <Kanban size={18} style={{ marginRight: 12 }} />
              <ListItemText primary="Projects" primaryTypographyProps={{ fontSize: '0.9rem', fontWeight: 500 }} />
            </ListItemButton>
            <ListItemButton
              selected={currentView === 'repositories'}
              onClick={() => handleViewChange('repositories')}
              sx={{
                py: 1.5,
                '&.Mui-selected': {
                  backgroundColor: 'action.selected',
                  borderLeft: '3px solid',
                  borderLeftColor: 'primary.main',
                },
              }}
            >
              <GitBranch size={18} style={{ marginRight: 12 }} />
              <ListItemText primary="Repositories" primaryTypographyProps={{ fontSize: '0.9rem', fontWeight: 500 }} />
            </ListItemButton>
          </List>
        </Paper>

        {/* Main Content */}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          {/* Projects View */}
          {currentView === 'projects' && (
            <>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error instanceof Error ? error.message : 'Failed to load projects'}
            </Alert>
          )}

          {/* Search bar */}
          {projects.length > 0 && (
            <Box sx={{ mb: 3 }}>
              <TextField
                placeholder="Search projects..."
                size="small"
                value={projectsSearchQuery}
                onChange={(e) => {
                  setProjectsSearchQuery(e.target.value)
                  setProjectsPage(0) // Reset to first page on search
                }}
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon />
                    </InputAdornment>
                  ),
                }}
                sx={{ maxWidth: 400 }}
              />
              {projectsSearchQuery && (
                <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                  {filteredProjects.length} of {projects.length} projects
                </Typography>
              )}
            </Box>
          )}

          {filteredProjects.length === 0 && projectsSearchQuery ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="h6" color="text.secondary" gutterBottom>
                No projects found
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Try adjusting your search query
              </Typography>
              <Button
                variant="outlined"
                onClick={() => setProjectsSearchQuery('')}
              >
                Clear Search
              </Button>
            </Box>
          ) : projects.length === 0 ? (
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
            <>
            <Grid container spacing={3}>
              {paginatedProjects.map((project) => (
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

            {/* Pagination */}
            {projectsTotalPages > 1 && (
              <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
                <Pagination
                  count={projectsTotalPages}
                  page={projectsPage + 1}
                  onChange={(_, page) => setProjectsPage(page - 1)}
                  color="primary"
                  showFirstButton
                  showLastButton
                />
              </Box>
            )}
            </>
          )}
            </>
          )}

          {/* Repositories View */}
          {currentView === 'repositories' && (
            <>
              {/* GitHub-style header with owner/repositories */}
              <Box sx={{ mb: 3, pb: 2 }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                  <Box>
                    <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
                      <span style={{ color: '#3b82f6', cursor: 'pointer' }}>{ownerSlug}</span>
                      <span style={{ color: 'text.secondary', fontWeight: 300 }}>/</span>
                      <span style={{ fontWeight: 600 }}>repositories</span>
                    </Typography>
                  </Box>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<GitBranch size={16} />}
                      onClick={() => setDemoRepoDialogOpen(true)}
                      sx={{ textTransform: 'none' }}
                    >
                      From demo
                    </Button>
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<Link size={16} />}
                      onClick={() => setLinkRepoDialogOpen(true)}
                      sx={{ textTransform: 'none' }}
                    >
                      Link external
                    </Button>
                    <Button
                      variant="contained"
                      color="secondary"
                      size="small"
                      startIcon={<Plus size={16} />}
                      onClick={() => setCreateRepoDialogOpen(true)}
                    >
                      New
                    </Button>
                  </Box>
                </Box>

                {/* Search bar */}
                {repositories.length > 0 && (
                  <Box>
                    <TextField
                      placeholder="Find a repository..."
                      size="small"
                      value={reposSearchQuery}
                      onChange={(e) => {
                        setReposSearchQuery(e.target.value)
                        setReposPage(0)
                      }}
                      InputProps={{
                        startAdornment: (
                          <InputAdornment position="start">
                            <SearchIcon />
                          </InputAdornment>
                        ),
                      }}
                      sx={{ maxWidth: 400 }}
                    />
                    {reposSearchQuery && (
                      <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                        {filteredRepositories.length} of {repositories.length} repositories
                      </Typography>
                    )}
                  </Box>
                )}
              </Box>

              {filteredRepositories.length === 0 && reposSearchQuery ? (
                <Card sx={{ textAlign: 'center', py: 8 }}>
                  <CardContent>
                    <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
                    <Typography variant="h6" gutterBottom>
                      No repositories found
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                      Try adjusting your search query
                    </Typography>
                    <Button
                      variant="outlined"
                      onClick={() => setReposSearchQuery('')}
                    >
                      Clear Search
                    </Button>
                  </CardContent>
                </Card>
              ) : repositories.length === 0 ? (
                <Card sx={{ textAlign: 'center', py: 8 }}>
                  <CardContent>
                    <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
                    <Typography variant="h6" gutterBottom>
                      No repositories yet
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                      Create your first git repository to start collaborating with AI agents and your team.
                    </Typography>
                  </CardContent>
                </Card>
              ) : (
                <>
                {/* GitHub-style list view */}
                <Box>
                  {paginatedRepositories.map((repo: ServicesGitRepository) => (
                    <Box
                      key={repo.id}
                      sx={{
                        py: 3,
                        px: 2,
                        borderBottom: 1,
                        borderColor: 'divider',
                        '&:hover': {
                          bgcolor: 'rgba(0, 0, 0, 0.02)',
                        },
                        cursor: 'pointer'
                      }}
                      onClick={() => handleViewRepository(repo)}
                    >
                      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                        <Box sx={{ flex: 1 }}>
                          {/* Repo name as owner/repo */}
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                            <GitBranch size={16} color="#656d76" />
                            <Typography
                              variant="h6"
                              sx={{
                                fontSize: '1.25rem',
                                fontWeight: 600,
                                color: '#3b82f6',
                                display: 'flex',
                                alignItems: 'center',
                                gap: 0.5,
                                '&:hover': {
                                  textDecoration: 'underline'
                                }
                              }}
                            >
                              {ownerSlug}
                              <span style={{ color: 'text.secondary', fontWeight: 400 }}>/</span>
                              {repo.name || repo.id}
                            </Typography>

                            {/* Chips */}
                            {repo.metadata?.is_external && (
                              <Chip
                                icon={<Link size={12} />}
                                label={repo.metadata.external_type || 'External'}
                                size="small"
                                sx={{ height: 20, fontSize: '0.75rem' }}
                              />
                            )}
                            {repo.metadata?.kodit_indexing && (
                              <Chip
                                icon={<Brain size={12} />}
                                label="Code Intelligence"
                                size="small"
                                color="success"
                                sx={{ height: 20, fontSize: '0.75rem' }}
                              />
                            )}
                            <Chip
                              label={repo.repo_type || 'project'}
                              size="small"
                              variant="outlined"
                              sx={{ height: 20, fontSize: '0.75rem', borderRadius: '12px' }}
                            />
                          </Box>

                          {/* Description */}
                          {repo.description && (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ml: 3 }}>
                              {repo.description}
                            </Typography>
                          )}

                          {/* Metadata row */}
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, ml: 3, mt: 1 }}>
                            {repo.created_at && (
                              <Typography variant="caption" color="text.secondary">
                                Updated {new Date(repo.created_at).toLocaleDateString()}
                              </Typography>
                            )}
                          </Box>
                        </Box>

                      </Box>
                    </Box>
                  ))}
                </Box>

                {/* Pagination */}
                {reposTotalPages > 1 && (
                  <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
                    <Pagination
                      count={reposTotalPages}
                      page={reposPage + 1}
                      onChange={(_, page) => setReposPage(page - 1)}
                      color="primary"
                      showFirstButton
                      showLastButton
                    />
                  </Box>
                )}
                </>
              )}
            </>
          )}
        </Box>
      </Container>

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

        {/* Demo Repository Dialog */}
        <Dialog open={demoRepoDialogOpen} onClose={() => setDemoRepoDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Create from Demo Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Choose a demo repository template to get started quickly with common project types.
              </Typography>

              <FormControl fullWidth required>
                <InputLabel>Demo Template</InputLabel>
                <Select
                  value={selectedSampleType}
                  onChange={(e) => handleSampleTypeChange(e.target.value)}
                  disabled={sampleTypesLoading}
                >
                  <MenuItem value="">
                    <em>Select a demo template</em>
                  </MenuItem>
                  {sampleTypes && sampleTypes.map((type: ServerSampleType) => (
                    <MenuItem key={type.id} value={type.id}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {getSampleProjectIcon(type.id, type.category, 18)}
                        <span>{type.name}</span>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              {selectedSampleType && (
                <>
                  <TextField
                    label="Repository Name"
                    fullWidth
                    required
                    value={demoRepoName}
                    onChange={(e) => setDemoRepoName(e.target.value)}
                    helperText="Auto-generated from template, customize if needed"
                  />

                  <FormControlLabel
                    control={
                      <Switch
                        checked={demoKoditIndexing}
                        onChange={(e) => setDemoKoditIndexing(e.target.checked)}
                        color="primary"
                      />
                    }
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Brain size={18} />
                        <Typography variant="body2">
                          Enable Code Intelligence
                        </Typography>
                      </Box>
                    }
                  />

                  <Alert severity="info">
                    {demoKoditIndexing
                      ? 'Code Intelligence enabled: Kodit will index this repository to provide code snippets and architectural summaries via MCP server.'
                      : sampleTypes && sampleTypes.find((t: ServerSampleType) => t.id === selectedSampleType)?.description
                    }
                  </Alert>
                </>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setDemoRepoDialogOpen(false)
              setSelectedSampleType('')
              setDemoRepoName('')
              setDemoKoditIndexing(true)
            }}>Cancel</Button>
            <Button
              onClick={handleCreateDemoRepo}
              variant="contained"
              disabled={!selectedSampleType || !demoRepoName.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Custom Repository Dialog */}
        <Dialog open={createRepoDialogOpen} onClose={() => {
          setCreateRepoDialogOpen(false)
          setCreateError('')
        }} maxWidth="sm" fullWidth>
          <DialogTitle>Create New Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {createError && (
                <Alert severity="error" onClose={() => setCreateError('')}>
                  {createError}
                </Alert>
              )}

              <TextField
                label="Repository Name"
                fullWidth
                value={repoName}
                onChange={(e) => setRepoName(e.target.value)}
                helperText="Enter a name for your repository"
                autoFocus
              />

              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={repoDescription}
                onChange={(e) => setRepoDescription(e.target.value)}
                helperText="Describe the purpose of this repository"
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={koditIndexing}
                    onChange={(e) => setKoditIndexing(e.target.checked)}
                    color="primary"
                  />
                }
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Brain size={18} />
                    <Typography variant="body2">
                      Enable Code Intelligence
                    </Typography>
                  </Box>
                }
              />

              <Alert severity="info">
                {koditIndexing
                  ? 'Code Intelligence enabled: Kodit will index this repository to provide code snippets and architectural summaries via MCP server.'
                  : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
                }
              </Alert>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setCreateRepoDialogOpen(false)
              setRepoName('')
              setRepoDescription('')
              setKoditIndexing(true)
              setCreateError('')
            }}>Cancel</Button>
            <Button
              onClick={handleCreateCustomRepo}
              variant="contained"
              disabled={!repoName.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Link External Repository Dialog */}
        <Dialog open={linkRepoDialogOpen} onClose={() => setLinkRepoDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Link External Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Link an existing repository from GitHub, GitLab, or Azure DevOps to enable AI collaboration.
              </Typography>

              <FormControl fullWidth required>
                <InputLabel>Repository Type</InputLabel>
                <Select
                  value={externalRepoType}
                  onChange={(e) => setExternalRepoType(e.target.value as 'github' | 'gitlab' | 'ado' | 'other')}
                  label="Repository Type"
                >
                  <MenuItem value="github">GitHub</MenuItem>
                  <MenuItem value="gitlab">GitLab</MenuItem>
                  <MenuItem value="ado">Azure DevOps</MenuItem>
                  <MenuItem value="other">Other (Bitbucket, Gitea, Self-hosted, etc.)</MenuItem>
                </Select>
              </FormControl>

              <TextField
                label="Repository URL"
                fullWidth
                required
                value={externalRepoUrl}
                onChange={(e) => setExternalRepoUrl(e.target.value)}
                placeholder="https://github.com/org/repo.git"
                helperText="Full URL to the external repository"
              />

              <TextField
                label="Repository Name (Optional)"
                fullWidth
                value={externalRepoName}
                onChange={(e) => setExternalRepoName(e.target.value)}
                helperText="Display name (auto-extracted from URL if empty)"
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={externalKoditIndexing}
                    onChange={(e) => setExternalKoditIndexing(e.target.checked)}
                    color="primary"
                  />
                }
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Brain size={18} />
                    <Typography variant="body2">
                      Enable Code Intelligence
                    </Typography>
                  </Box>
                }
              />

              <Alert severity="warning">
                Authentication to external repositories is not yet implemented. You can link repositories for reference, but cloning and syncing will require manual setup.
              </Alert>

              <Alert severity="info">
                {externalKoditIndexing
                  ? 'Code Intelligence enabled: Kodit will index this external repository to provide code snippets and architectural summaries via MCP server.'
                  : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
                }
              </Alert>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setLinkRepoDialogOpen(false)
              setExternalRepoName('')
              setExternalRepoUrl('')
              setExternalRepoType('github')
              setExternalKoditIndexing(true)
            }}>Cancel</Button>
            <Button
              onClick={handleLinkExternalRepo}
              variant="contained"
              disabled={!externalRepoUrl.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Link Repository'}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default Projects

