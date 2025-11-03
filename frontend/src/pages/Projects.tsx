import React, { FC, useState } from 'react'
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
  Tooltip,
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SettingsIcon from '@mui/icons-material/Settings'
import { Kanban } from 'lucide-react'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useListProjects,
  useCreateProject,
  useListSampleProjects,
  useInstantiateSampleProject,
  TypesProject,
} from '../services'

const Projects: FC = () => {
  const account = useAccount()
  const { navigate } = useRouter()
  const snackbar = useSnackbar()

  const { data: projects = [], isLoading, error } = useListProjects()
  const { data: sampleProjects = [] } = useListSampleProjects()
  const createProjectMutation = useCreateProject()
  const instantiateSampleMutation = useInstantiateSampleProject()

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedProject, setSelectedProject] = useState<TypesProject | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectDescription, setNewProjectDescription] = useState('')

  // New Project dropdown menu state
  const [newProjectMenuAnchor, setNewProjectMenuAnchor] = useState<null | HTMLElement>(null)

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
    setNewProjectMenuAnchor(null) // Close dropdown
  }

  const handleInstantiateSample = async (sampleId: string, sampleName: string) => {
    if (!checkLoginStatus()) return

    try {
      setNewProjectMenuAnchor(null) // Close dropdown

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

  if (isLoading) {
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
        <>
          <Button
            variant="contained"
            color="secondary"
            startIcon={<AddIcon />}
            endIcon={<ArrowDropDownIcon />}
            onClick={(e) => setNewProjectMenuAnchor(e.currentTarget)}
          >
            New Project
          </Button>
          <Menu
            anchorEl={newProjectMenuAnchor}
            open={Boolean(newProjectMenuAnchor)}
            onClose={() => setNewProjectMenuAnchor(null)}
            anchorOrigin={{
              vertical: 'bottom',
              horizontal: 'right',
            }}
            transformOrigin={{
              vertical: 'top',
              horizontal: 'right',
            }}
          >
            <Tooltip
              title="Create a blank project with no sample code or pre-configured tasks"
              placement="right"
              arrow
            >
              <MenuItem onClick={handleNewProject}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 200 }}>
                  <AddIcon fontSize="small" />
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    Empty Project
                  </Typography>
                </Box>
              </MenuItem>
            </Tooltip>
            {sampleProjects.length > 0 && <MenuItem disabled><Typography variant="caption" sx={{ fontWeight: 600, opacity: 0.6 }}>Sample Projects</Typography></MenuItem>}
            {sampleProjects.map((sample) => (
              <Tooltip
                key={`tooltip-${sample.id}`}
                title={
                  <Box>
                    <Typography variant="body2" sx={{ mb: 0.5 }}>
                      {sample.description || 'Sample project with pre-configured tasks'}
                    </Typography>
                    <Typography variant="caption" sx={{ opacity: 0.8 }}>
                      {sample.category} • {sample.difficulty}
                    </Typography>
                  </Box>
                }
                placement="right"
                arrow
              >
                <MenuItem
                  key={sample.id}
                  onClick={() => handleInstantiateSample(sample.id || '', sample.name)}
                  disabled={instantiateSampleMutation.isPending}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 200 }}>
                    <Kanban size={18} />
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      {sample.name}
                    </Typography>
                  </Box>
                </MenuItem>
              </Tooltip>
            ))}
          </Menu>
        </>
      )}
    >
      <Container maxWidth="lg">
        <Box sx={{ mt: 4 }}>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error instanceof Error ? error.message : 'Failed to load projects'}
            </Alert>
          )}

          {projects.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Kanban size={80} color="currentColor" style={{ color: 'rgba(0, 0, 0, 0.6)', marginBottom: 16 }} />
              <Typography variant="h6" color="text.secondary" gutterBottom>
                No projects yet
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Create your first project to get started
              </Typography>
              <Button
                variant="contained"
                color="primary"
                endIcon={<AddIcon />}
                onClick={handleNewProject}
              >
                Create Project
              </Button>
            </Box>
          ) : (
            <Grid container spacing={3}>
              {projects.map((project) => (
                <Grid item xs={12} sm={6} md={4} key={project.id}>
                  <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                    <CardContent sx={{ flexGrow: 1, cursor: 'pointer' }} onClick={() => handleViewProject(project)}>
                      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
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
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                          {project.description}
                        </Typography>
                      )}
                      <Typography variant="caption" color="text.secondary">
                        {project.status || 'active'}
                      </Typography>
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
        </Box>

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
      </Container>
    </Page>
  )
}

export default Projects
