import React, { FC, useState } from 'react'
import {
  Container,
  Box,
  Button,
  Menu,
  MenuItem,
  Alert,
  CircularProgress,
} from '@mui/material'
import SettingsIcon from '@mui/icons-material/Settings'
import { Plus, Link } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import CreateProjectButton from '../components/project/CreateProjectButton'
import CreateProjectDialog from '../components/project/CreateProjectDialog'
import CreateRepositoryDialog from '../components/project/CreateRepositoryDialog'
import LinkExternalRepositoryDialog from '../components/project/LinkExternalRepositoryDialog'
import AgentSelectionModal from '../components/project/AgentSelectionModal'
import ProjectsListView from '../components/project/ProjectsListView'
import RepositoriesListView from '../components/project/RepositoriesListView'
import GuidelinesView from '../components/project/GuidelinesView'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import {
  useListProjects,
  useListSampleProjects,
  useInstantiateSampleProject,
  TypesProject,
} from '../services'
import { useGitRepositories } from '../services/gitRepositoryService'
import type { TypesExternalRepositoryType, TypesGitRepository, TypesAzureDevOps } from '../api/api'

const Projects: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const { navigate } = router
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const api = useApi()
  const apps = useApps()

  const isLoggedIn = !!account.user

  // Single helper to check login and show dialog if needed
  const requireLogin = React.useCallback((): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }, [account])

  // Show login dialog on mount if not logged in (only after account is initialized)
  React.useEffect(() => {
    if (account.initialized && !isLoggedIn) {
      account.setShowLoginWindow(true)
    }
  }, [account.initialized, isLoggedIn])

  // Load apps on mount to get app names for project lozenges
  React.useEffect(() => {
    if (account.user?.id) {
      apps.loadApps()
    }
  }, [account.user?.id])

  // Create a map of app ID -> app name for displaying in project cards
  const appNamesMap = React.useMemo(() => {
    const map: Record<string, string> = {}
    if (apps.apps) {
      apps.apps.forEach((app) => {
        if (app.id) {
          map[app.id] = app.config?.helix?.name || 'Unnamed Agent'
        }
      })
    }
    return map
  }, [apps.apps])

  // Check if org slug is set in the URL
  // const orgSlug = router.params.org_id || ''

  const { data: projects = [], isLoading, error } = useListProjects(account.organizationTools.organization?.id || '', { enabled: isLoggedIn })
  const { data: sampleProjects = [] } = useListSampleProjects({ enabled: isLoggedIn })
  const instantiateSampleMutation = useInstantiateSampleProject()

  // Get tab from URL query parameter
  const { tab } = router.params
  const currentView = tab || 'projects'
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedProject, setSelectedProject] = useState<TypesProject | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  // Repository management
  const currentOrg = account.organizationTools.organization
  const ownerId = account.user?.id || ''
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'
  // List repos by organization_id when in org context, or by owner_id for personal workspace
  const { data: repositories = [], isLoading: reposLoading } = useGitRepositories(
    currentOrg?.id
      ? { organizationId: currentOrg.id, enabled: isLoggedIn }
      : { ownerId: account.user?.id, enabled: isLoggedIn }
  )

  // Repository dialog states
  const [createRepoDialogOpen, setCreateRepoDialogOpen] = useState(false)
  const [linkRepoDialogOpen, setLinkRepoDialogOpen] = useState(false)

  // Agent selection modal state for sample project fork
  const [agentModalOpen, setAgentModalOpen] = useState(false)
  const [pendingSampleFork, setPendingSampleFork] = useState<{ sampleId: string; sampleName: string } | null>(null)

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
  const filteredRepositories = repositories.filter((repo: TypesGitRepository) =>
    repo.name?.toLowerCase().includes(reposSearchQuery.toLowerCase()) ||
    repo.description?.toLowerCase().includes(reposSearchQuery.toLowerCase())
  )
  const paginatedRepositories = filteredRepositories.slice(
    reposPage * reposPerPage,
    (reposPage + 1) * reposPerPage
  )
  const reposTotalPages = Math.ceil(filteredRepositories.length / reposPerPage)

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, project: TypesProject) => {
    setAnchorEl(event.currentTarget)
    setSelectedProject(project)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedProject(null)
  }


  // Helper to create a new repo for the project dialog
  const handleCreateRepoForProject = async (name: string, description: string): Promise<TypesGitRepository | null> => {
    if (!name.trim() || !account.user?.id) return null

    try {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitRepositoriesCreate({
        name,
        description,
        owner_id: account.user.id, // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: 'code' as any,
        default_branch: 'main',
      })

      // Invalidate repo queries (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      return response.data
    } catch (error) {
      console.error('Failed to create repository:', error)
      return null
    }
  }

  // Helper to link an external repo for the project dialog
  const handleLinkRepoForProject = async (
    url: string,
    name: string,
    type: TypesExternalRepositoryType,
    username?: string,
    password?: string,
    azureDevOps?: TypesAzureDevOps
  ): Promise<TypesGitRepository | null> => {
    if (!url.trim() || !account.user?.id) return null

    try {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitRepositoriesCreate({
        name,
        description: `External ${type} repository`,
        owner_id: account.user.id, // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: 'code' as any,
        default_branch: 'main',
        external_url: url,
        external_type: type,
        username,
        password,
        azure_devops: azureDevOps,
      })

      // Invalidate repo queries (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      return response.data
    } catch (error: any) {
      console.error('Failed to link repository:', error)
      // Re-throw with the actual error message so the dialog can display it
      const message = error?.response?.data?.message || error?.response?.data || error?.message || 'Failed to link repository'
      throw new Error(typeof message === 'string' ? message : JSON.stringify(message))
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

  const handleNewProject = () => {
    if (!requireLogin()) return
    setCreateDialogOpen(true)
  }

  // Step 1: User clicks on sample project - show agent selection modal
  const handleInstantiateSample = async (sampleId: string, sampleName: string) => {
    if (!requireLogin()) return

    // Store the pending fork request and show agent selection modal
    setPendingSampleFork({ sampleId, sampleName })
    setAgentModalOpen(true)
  }

  // Step 2: User selects an agent - proceed with fork
  const handleAgentSelected = async (agentId: string) => {
    if (!pendingSampleFork) return

    const { sampleId, sampleName } = pendingSampleFork
    setPendingSampleFork(null)

    try {
      snackbar.info(`Creating ${sampleName}...`)

      const result = await instantiateSampleMutation.mutateAsync({
        sampleId,
        request: {
          project_name: sampleName,
          organization_id: account.organizationTools.organization?.id, // Pass current workspace context
          helix_app_id: agentId, // Pass the selected agent
        },
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


  const handleCreateCustomRepo = async (name: string, description: string, koditIndexing: boolean) => {
    if (!name.trim() || !account.user?.id) return

    setCreating(true)
    setCreateError('')
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesCreate({
        name,
        description,
        owner_id: account.user.id, // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: 'code' as any, // Helix-hosted code repository
        default_branch: 'main',
        kodit_indexing: koditIndexing,
      })

      // Invalidate and refetch git repositories query (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

      setCreateRepoDialogOpen(false)
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

  const handleLinkExternalRepo = async (url: string, name: string, type: 'github' | 'gitlab' | 'ado' | 'other', koditIndexing: boolean, username?: string, password?: string, organizationUrl?: string, token?: string) => {
    if (!url.trim() || !ownerId) return

    setCreating(true)
    try {
      const apiClient = api.getApiClient()

      // Extract repo name from URL if not provided
      let repoName = name.trim()
      if (!repoName) {
        // Try to extract from URL (e.g., github.com/org/repo.git -> repo)
        const match = url.match(/\/([^\/]+?)(\.git)?$/)
        repoName = match ? match[1] : 'external-repo'
      }

      let azureDevOps: TypesAzureDevOps | undefined
      if (type === 'ado' && organizationUrl && token) {
        azureDevOps = {
          organization_url: organizationUrl,
          personal_access_token: token,
        }
      }
      
      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: `External ${type} repository`,
        owner_id: account.user?.id || '', // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: 'project' as any,
        default_branch: 'main',
        // Remote URL
        external_url: url,
        // Repository provider (github, gitlab, ado, etc.)
        external_type: type as TypesExternalRepositoryType,
        // Auth details
        username: username,
        password: password,
        // Azure DevOps specific
        azure_devops: azureDevOps,
        // Code intelligence
        kodit_indexing: koditIndexing,
      })

      // Invalidate and refetch git repositories query (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

      setLinkRepoDialogOpen(false)
      snackbar.success('External repository linked successfully')
    } catch (error) {
      console.error('Failed to link external repository:', error)
      snackbar.error('Failed to link external repository')
    } finally {
      setCreating(false)
    }
  }

  const handleViewRepository = (repo: TypesGitRepository) => {
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

  // Get breadcrumb title based on current view
  const getBreadcrumbTitle = () => {
    switch (currentView) {
      case 'repositories':
        return 'Repositories'
      case 'guidelines':
        return 'Guidelines'
      default:
        return 'Projects'
    }
  }

  return (
    <Page
      breadcrumbTitle={getBreadcrumbTitle()}
      breadcrumbs={[]}
      orgBreadcrumbs={true}
      topbarContent={currentView === 'projects' ? (
        <CreateProjectButton
          onCreateEmpty={handleNewProject}
          onCreateFromSample={handleInstantiateSample}
          sampleProjects={sampleProjects}
          isCreating={instantiateSampleMutation.isPending}
          variant="contained"
          color="secondary"
        />
      ) : currentView === 'repositories' ? (
        <>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={() => {
              if (!requireLogin()) return
              setCreateRepoDialogOpen(true)
            }}
            sx={{ textTransform: 'none', mr: 1 }}
          >
            New Repository
          </Button>
          <Button
            variant="contained"
            color="secondary"
            size="small"
            startIcon={<Link size={16} />}
            onClick={() => {
              if (!requireLogin()) return
              setLinkRepoDialogOpen(true)
            }}
          >
            Link External Repository
          </Button>
        </>
      ) : null}
    >
      <Container maxWidth="lg" sx={{ mt: 4 }}>
        {/* Projects View */}
        {currentView === 'projects' && (
          <ProjectsListView
            projects={projects}
            error={isLoggedIn ? error : null}
            searchQuery={projectsSearchQuery}
            onSearchChange={setProjectsSearchQuery}
            page={projectsPage}
            onPageChange={setProjectsPage}
            filteredProjects={filteredProjects}
            paginatedProjects={paginatedProjects}
            totalPages={projectsTotalPages}
            onViewProject={handleViewProject}
            onMenuOpen={handleMenuOpen}
            onNavigateToSettings={(id) => account.orgNavigate('project-settings', { id })}
            onCreateEmpty={handleNewProject}
            onCreateFromSample={handleInstantiateSample}
            sampleProjects={sampleProjects}
            isCreating={instantiateSampleMutation.isPending}
            appNamesMap={appNamesMap}
          />
        )}

        {/* Repositories View */}
        {currentView === 'repositories' && (
          <RepositoriesListView
            repositories={repositories}
            ownerSlug={ownerSlug}
            searchQuery={reposSearchQuery}
            onSearchChange={setReposSearchQuery}
            page={reposPage}
            onPageChange={setReposPage}
            filteredRepositories={filteredRepositories}
            paginatedRepositories={paginatedRepositories}
            totalPages={reposTotalPages}
            onViewRepository={handleViewRepository}
            onCreateRepo={() => {
              if (!requireLogin()) return
              setCreateRepoDialogOpen(true)
            }}
            onLinkExternalRepo={() => {
              if (!requireLogin()) return
              setLinkRepoDialogOpen(true)
            }}
          />
        )}

        {/* Guidelines View */}
        {currentView === 'guidelines' && (
          <GuidelinesView organization={currentOrg} isPersonalWorkspace={!currentOrg} />
        )}
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
        <CreateProjectDialog
          open={createDialogOpen}
          onClose={() => setCreateDialogOpen(false)}
          repositories={repositories}
          reposLoading={reposLoading}
          onCreateRepo={handleCreateRepoForProject}
          onLinkRepo={handleLinkRepoForProject}
        />


        {/* Custom Repository Dialog */}
        <CreateRepositoryDialog
          open={createRepoDialogOpen}
          onClose={() => {
            setCreateRepoDialogOpen(false)
            setCreateError('')
          }}
          onSubmit={handleCreateCustomRepo}
          isCreating={creating}
          error={createError}
        />

        {/* Link External Repository Dialog */}
        <LinkExternalRepositoryDialog
          open={linkRepoDialogOpen}
          onClose={() => setLinkRepoDialogOpen(false)}
          onSubmit={handleLinkExternalRepo}
          isCreating={creating}
        />

        {/* Agent Selection Modal for Sample Project Fork */}
        <AgentSelectionModal
          open={agentModalOpen}
          onClose={() => {
            setAgentModalOpen(false)
            setPendingSampleFork(null)
          }}
          onSelect={handleAgentSelected}
          title="Select Agent for Project"
          description="Choose a default agent for this project. You can override this when creating individual tasks."
        />
    </Page>
  )
}

export default Projects


