import React, { FC, useState } from 'react'
import {
  Container,
  Box,
  Button,
  Menu,
  MenuItem,
  CircularProgress,
} from '@mui/material'
import SettingsIcon from '@mui/icons-material/Settings'
import { Plus, Link, FolderSearch } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import CreateProjectButton from '../components/project/CreateProjectButton'
import CreateProjectDialog from '../components/project/CreateProjectDialog'
import CreateRepositoryDialog from '../components/project/CreateRepositoryDialog'
import LinkExternalRepositoryDialog from '../components/project/LinkExternalRepositoryDialog'
import BrowseProvidersDialog from '../components/project/BrowseProvidersDialog'
import AgentSelectionModal from '../components/project/AgentSelectionModal'
import SampleProjectWizard from '../components/project/SampleProjectWizard'
import ProjectsListView from '../components/project/ProjectsListView'
import RepositoriesListView from '../components/project/RepositoriesListView'
import GuidelinesView from '../components/project/GuidelinesView'
import PromptsListView from '../components/project/PromptsListView'
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
import type { TypesExternalRepositoryType, TypesGitRepository, TypesAzureDevOps, TypesGitHub, TypesGitLab, TypesBitbucket, TypesRepositoryInfo } from '../api/api'

const Projects: FC = () => {
  const account = useAccount()
  const router = useRouter()
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

  const isOrgResolved = !account.organizationTools.orgID || !!account.organizationTools.organization
  const shouldLoadProjects = isLoggedIn && !account.organizationTools.loading && isOrgResolved
  const { data: projects = [], isLoading, error } = useListProjects(
    account.organizationTools.organization?.id || '',
    { enabled: shouldLoadProjects }
  )
  const isProjectsLoading = isLoading || (isLoggedIn && (account.organizationTools.loading || !isOrgResolved))
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
  const [pendingSampleFork, setPendingSampleFork] = useState<{ sampleId: string; sampleName: string; sampleProject?: any } | null>(null)

  // GitHub auth wizard for sample projects that require it (e.g., helix-in-helix)
  const [sampleWizardOpen, setSampleWizardOpen] = useState(false)
  const [sampleWizardProject, setSampleWizardProject] = useState<any>(null)
  const [selectedAgentForWizard, setSelectedAgentForWizard] = useState<string | undefined>(undefined)

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string>('')

  // Browse providers dialog state
  const [browseProvidersOpen, setBrowseProvidersOpen] = useState(false)
  const [linkingFromBrowser, setLinkingFromBrowser] = useState(false)

  // Pagination for projects
  const [projectsPage, setProjectsPage] = useState(0)
  const projectsPerPage = 24

  // Search and pagination for repositories
  const [reposSearchQuery, setReposSearchQuery] = useState('')
  const [reposPage, setReposPage] = useState(0)
  const reposPerPage = 10

  // Paginate projects
  const filteredProjects = projects
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
    azureDevOps?: TypesAzureDevOps,
    oauthConnectionId?: string
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
        oauth_connection_id: oauthConnectionId, // OAuth connection for push access
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

  // Step 1: User clicks on sample project - always show agent selection modal first
  const handleInstantiateSample = async (sampleId: string, sampleName: string) => {
    if (!requireLogin()) return

    // Find the sample project
    const sampleProject = sampleProjects.find((p: any) => p.id === sampleId)

    // Always show agent selection modal first
    // Store the sample project for later (GitHub auth check happens after agent selection)
    setPendingSampleFork({ sampleId, sampleName, sampleProject })
    setAgentModalOpen(true)
  }

  // Step 2: User selects an agent - proceed with fork or show GitHub wizard
  const handleAgentSelected = async (agentId: string) => {
    if (!pendingSampleFork) return

    const { sampleId, sampleName, sampleProject } = pendingSampleFork
    setPendingSampleFork(null)

    // Check if this sample requires GitHub auth
    if (sampleProject?.requires_github_auth || (sampleProject?.required_repositories?.length || 0) > 0) {
      // Store the selected agent and open the GitHub wizard
      setSelectedAgentForWizard(agentId)
      setSampleWizardProject(sampleProject)
      setSampleWizardOpen(true)
    } else {
      // Standard flow - create project directly
      try {
        snackbar.info(`Creating ${sampleName}...`)

        const result = await instantiateSampleMutation.mutateAsync({
          sampleId,
          request: {
            project_name: sampleName,
            organization_id: account.organizationTools.organization?.id,
            helix_app_id: agentId,
          },
        })

        snackbar.success('Sample project created successfully!')

        if (result && result.project_id) {
          account.orgNavigate('project-specs', { id: result.project_id })
        }
      } catch (err) {
        snackbar.error('Failed to create sample project')
      }
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

  const handleLinkExternalRepo = async (url: string, name: string, type: 'github' | 'gitlab' | 'ado' | 'other', koditIndexing: boolean, username?: string, password?: string, organizationUrl?: string, token?: string, gitlabBaseUrl?: string) => {
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

      // Build provider-specific config objects
      let azureDevOps: TypesAzureDevOps | undefined
      let github: TypesGitHub | undefined
      let gitlab: TypesGitLab | undefined

      if (type === 'ado' && organizationUrl && token) {
        azureDevOps = {
          organization_url: organizationUrl,
          personal_access_token: token,
        }
      }

      if (type === 'github' && token) {
        github = {
          personal_access_token: token,
        }
      }

      if (type === 'gitlab' && (token || gitlabBaseUrl)) {
        gitlab = {
          personal_access_token: token,
          base_url: gitlabBaseUrl,
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
        // Provider-specific config
        azure_devops: azureDevOps,
        github: github,
        gitlab: gitlab,
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

  // Handle repository selection from OAuth browser or PAT flow
  const handleBrowseSelectRepository = async (repo: TypesRepositoryInfo, providerTypeOrCreds: string, oauthConnectionId?: string  ) => {
    if (!account.user?.id) return    

    setLinkingFromBrowser(true)
    try {
      const apiClient = api.getApiClient()

      // Check if providerTypeOrCreds is JSON (PAT credentials) or plain provider type
      let providerType: string
      let patCredentials: { pat?: string; username?: string; orgUrl?: string; gitlabBaseUrl?: string; githubBaseUrl?: string; bitbucketBaseUrl?: string } | null = null

      try {
        const parsed = JSON.parse(providerTypeOrCreds)
        providerType = parsed.type
        patCredentials = {
          pat: parsed.pat,
          username: parsed.username,
          orgUrl: parsed.orgUrl,
          gitlabBaseUrl: parsed.gitlabBaseUrl,
          githubBaseUrl: parsed.githubBaseUrl,
          bitbucketBaseUrl: parsed.bitbucketBaseUrl,
        }
      } catch {
        // Not JSON, it's a plain provider type (OAuth flow)
        providerType = providerTypeOrCreds
      }

      // Map provider type to external type
      const externalTypeMap: Record<string, TypesExternalRepositoryType> = {
        'github': 'github' as TypesExternalRepositoryType,
        'gitlab': 'gitlab' as TypesExternalRepositoryType,
        'azure-devops': 'ado' as TypesExternalRepositoryType,
        'bitbucket': 'bitbucket' as TypesExternalRepositoryType,
      }

      // Build provider-specific config if using PAT
      let github: TypesGitHub | undefined
      let gitlab: TypesGitLab | undefined
      let azureDevOps: TypesAzureDevOps | undefined
      let bitbucket: TypesBitbucket | undefined

      if (patCredentials?.pat) {
        if (providerType === 'github') {
          github = {
            personal_access_token: patCredentials.pat,
            base_url: patCredentials.githubBaseUrl,
          }
        } else if (providerType === 'gitlab') {
          gitlab = {
            personal_access_token: patCredentials.pat,
            base_url: patCredentials.gitlabBaseUrl,
          }
        } else if (providerType === 'azure-devops') {
          azureDevOps = {
            organization_url: patCredentials.orgUrl || '',
            personal_access_token: patCredentials.pat,
          }
        } else if (providerType === 'bitbucket') {
          bitbucket = {
            username: patCredentials.username || '',
            app_password: patCredentials.pat,
            base_url: patCredentials.bitbucketBaseUrl,
          }
        }
      }

      await apiClient.v1GitRepositoriesCreate({
        name: repo.name || 'repository',
        description: repo.description || `${providerType} repository`,
        owner_id: account.user.id,
        organization_id: currentOrg?.id,
        repo_type: 'code' as any,
        default_branch: repo.default_branch || 'main',
        is_external: true,
        external_url: repo.clone_url || repo.html_url || '',
        external_type: externalTypeMap[providerType] || ('github' as TypesExternalRepositoryType),
        kodit_indexing: true,
        github,
        gitlab,
        azure_devops: azureDevOps,
        bitbucket,
        oauth_connection_id: oauthConnectionId,
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      // Reset pagination to show the new repo at the top
      setReposPage(0)
      setReposSearchQuery('')

      setBrowseProvidersOpen(false)
      snackbar.success('Repository linked successfully')
    } catch (error) {
      console.error('Failed to link repository from browser:', error)
      snackbar.error('Failed to link repository')
    } finally {
      setLinkingFromBrowser(false)
    }
  }

  if (isProjectsLoading || reposLoading) {
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
      case 'prompts':
        return 'Prompts'
      default:
        return 'Projects'
    }
  }

  return (
    <Page
      breadcrumbTitle={getBreadcrumbTitle()}
      breadcrumbParent={currentView !== 'projects' ? { title: 'Projects', routeName: 'projects' } : undefined}
      breadcrumbs={[]}
      orgBreadcrumbs={true}
      globalSearch={true}
      organizationId={account.organizationTools.organization?.id}
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
            variant="contained"
            color="secondary"
            startIcon={<FolderSearch size={20} />}
            onClick={() => {
              if (!requireLogin()) return
              setBrowseProvidersOpen(true)
            }}
            sx={{ mr: 1 }}
          >
            Connect & Browse
          </Button>
          <Button
            variant="outlined"
            startIcon={<Link size={20} />}
            onClick={() => {
              if (!requireLogin()) return
              setLinkRepoDialogOpen(true)
            }}
            sx={{ mr: 1 }}
          >
            Link Manually
          </Button>
          <Button
            variant="outlined"
            startIcon={<Plus size={20} />}
            onClick={() => {
              if (!requireLogin()) return
              setCreateRepoDialogOpen(true)
            }}
          >
            New Empty
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
            isLoading={isProjectsLoading}
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

        {/* Prompts View */}
        {currentView === 'prompts' && (
          <PromptsListView />
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

        {/* GitHub Auth Wizard for Sample Projects */}
        <SampleProjectWizard
          open={sampleWizardOpen}
          onClose={() => {
            setSampleWizardOpen(false)
            setSampleWizardProject(null)
          }}
          onComplete={(projectId) => {
            setSampleWizardOpen(false)
            setSampleWizardProject(null)
            snackbar.success('Project created successfully!')
            account.orgNavigate('project-specs', { id: projectId })
          }}
          sampleProject={sampleWizardProject}
          organizationId={account.organizationTools.organization?.id}
          selectedAgentId={selectedAgentForWizard}
        />

        {/* Browse Connected Providers Dialog */}
        <BrowseProvidersDialog
          open={browseProvidersOpen}
          onClose={() => setBrowseProvidersOpen(false)}
          onSelectRepository={handleBrowseSelectRepository}
          isLinking={linkingFromBrowser}
          organizationName={currentOrg?.name}
        />
    </Page>
  )
}

export default Projects


