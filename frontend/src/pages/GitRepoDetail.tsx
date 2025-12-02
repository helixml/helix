import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  Button,
  Chip,
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  IconButton,
  Divider,
  Stack,
  FormControlLabel,
  Switch,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  InputAdornment,
  Tabs,
  Tab,
  Tooltip,
  Paper,
} from '@mui/material'
import {
  GitBranch,
  Copy,
  ExternalLink,
  ArrowLeft,
  Brain,
  Link,
  X as CloseIcon,
  Settings,
  Users,
  Code as CodeIcon,
  Eye,
  EyeOff,
  GitCommit,
  GitPullRequest,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import AccessManagement from '../components/app/AccessManagement'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useGitRepository,
  useBrowseRepositoryTree,
  useGetRepositoryFile,
  useListRepositoryBranches,
  useListRepositoryCommits,
  useCreateOrUpdateRepositoryFile,
  usePushPullGitRepository,
  useCreateBranch,
} from '../services/gitRepositoryService'
import {
  useListRepositoryAccessGrants,
  useCreateRepositoryAccessGrant,
  useDeleteRepositoryAccessGrant,
} from '../services/repositoryAccessGrantService'
import {
  useKoditEnrichments,
  groupEnrichmentsBySubtype,
} from '../services/koditService'
import MonacoEditor from '../components/widgets/MonacoEditor'
import CodeTab from '../components/git/CodeTab'
import CodeIntelligenceTab from '../components/git/CodeIntelligenceTab'
import CommitsTab from '../components/git/CommitsTab'
import SettingsTab from '../components/git/SettingsTab'
import PullRequests from '../components/git/PullRequests'
import { TypesExternalRepositoryType } from '../api/api'

const TAB_NAMES = ['code-intelligence', 'code', 'settings', 'access', 'commits', 'pull-requests'] as const
type TabName = typeof TAB_NAMES[number]

const getTabName = (name: string | undefined): TabName => {
  if (name && TAB_NAMES.includes(name as TabName)) {
    return name as TabName
  }
  return TAB_NAMES[0]
}

const getFallbackBranch = (defaultBranch: string | undefined, branches: string[] | null | undefined): string => {
  if (!branches || branches.length === 0) {
    return ''
  }

  if (branches.includes('main')) {
    return 'main'
  }
  if (branches.includes('master')) {
    return 'master'
  }

  if (defaultBranch && branches.includes(defaultBranch)) {
    return defaultBranch
  }

  return branches[0] || ''
}

const GitRepoDetail: FC = () => {
  const router = useRouter()
  const repoId = router.params.repoId
  const account = useAccount()
  const { navigate } = router
  const queryClient = useQueryClient()
  const api = useApi()
  const snackbar = useSnackbar()

  const currentOrg = account.organizationTools.organization
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'
  const ownerId = currentOrg?.id || account.user?.id || ''

  const { data: repository, isLoading, error } = useGitRepository(repoId || '')

  // List branches for branch switcher
  const { data: branches = [], isLoading: branchesLoading } = useListRepositoryBranches(repoId || '')

  // Access grants for RBAC
  const { data: accessGrants = [], isLoading: accessGrantsLoading } = useListRepositoryAccessGrants(repoId || '', !!repoId)
  const createAccessGrantMutation = useCreateRepositoryAccessGrant(repoId || '')
  const deleteAccessGrantMutation = useDeleteRepositoryAccessGrant(repoId || '')
  const createOrUpdateFileMutation = useCreateOrUpdateRepositoryFile()
  const pushPullMutation = usePushPullGitRepository()
  const createBranchMutation = useCreateBranch()

  // Query parameters
  const branchFromQuery = router.params.branch || ''
  const commitFromQuery = router.params.commit || ''
  const currentBranch = branchFromQuery
  const commitsBranch = branchFromQuery

  // Kodit code intelligence enrichments (internal summary types filtered in backend)
  const { data: enrichmentsData } = useKoditEnrichments(repoId || '', commitFromQuery, { enabled: !!repoId })
  const enrichments = enrichmentsData?.data || []
  const groupedEnrichmentsBySubtype = groupEnrichmentsBySubtype(enrichments)

  // UI State
  const [currentTab, setCurrentTab] = useState<TabName>(() => getTabName(router.params.tab))
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [cloneDialogOpen, setCloneDialogOpen] = useState(false)
  const [forcePushDialogOpen, setForcePushDialogOpen] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editDefaultBranch, setEditDefaultBranch] = useState('')
  const [editKoditIndexing, setEditKoditIndexing] = useState<boolean | undefined>(undefined)
  const [editExternalUrl, setEditExternalUrl] = useState('')
  const [editExternalType, setEditExternalType] = useState<TypesExternalRepositoryType | undefined>(undefined)
  const [editUsername, setEditUsername] = useState('')
  const [editPassword, setEditPassword] = useState('')
  const [editOrganizationUrl, setEditOrganizationUrl] = useState('')
  const [editPersonalAccessToken, setEditPersonalAccessToken] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showPersonalAccessToken, setShowPersonalAccessToken] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [copiedClone, setCopiedClone] = useState(false)
  const [copiedSha, setCopiedSha] = useState<string | null>(null)
  const [currentPath, setCurrentPath] = useState('.')
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [dangerZoneExpanded, setDangerZoneExpanded] = useState(false)

  const setCurrentBranch = (branch: string) => {
    if (branch) {
      router.mergeParams({ branch })
    } else {
      router.removeParams(['branch'])
    }
  }

  const setCommitsBranch = (branch: string) => {
    if (branch) {
      router.mergeParams({ branch })
    } else {
      router.removeParams(['branch'])
    }
  }

  // List commits
  const { data: commitsData, isLoading: commitsLoading } = useListRepositoryCommits(
    repoId || '',
    commitsBranch || undefined,
    1,
    100
  )
  const commits = commitsData?.commits || []

  // Create/Edit File Dialog State
  const [createFileDialogOpen, setCreateFileDialogOpen] = useState(false)
  const [isEditingFile, setIsEditingFile] = useState(false)
  const [newFilePath, setNewFilePath] = useState('')
  const [newFileContent, setNewFileContent] = useState('')
  const [creatingFile, setCreatingFile] = useState(false)

  // Create Branch Dialog State
  const [createBranchDialogOpen, setCreateBranchDialogOpen] = useState(false)
  const [newBranchName, setNewBranchName] = useState('')
  const [newBranchBase, setNewBranchBase] = useState('')

  // Browse repository tree
  const { data: treeData, isLoading: treeLoading } = useBrowseRepositoryTree(repoId || '', currentPath, currentBranch)
  const { data: fileData, isLoading: fileLoading } = useGetRepositoryFile(
    repoId || '',
    selectedFile || '',
    currentBranch,
    !!selectedFile
  )

  // Sync tab state with query parameter
  React.useEffect(() => {
    const tabName = getTabName(router.params.tab)
    if (tabName !== currentTab) {
      setCurrentTab(tabName)
    }
  }, [router.params.tab, currentTab])

  // Initialize edit fields when repository loads
  React.useEffect(() => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditDefaultBranch(repository.default_branch || '')
      setEditKoditIndexing(repository.kodit_indexing || false)
      setEditExternalUrl(repository.external_url || '')
      setEditExternalType(repository.external_type)
      setEditUsername(repository.username || '')
      setEditPassword('')
      setEditOrganizationUrl(repository.azure_devops?.organization_url || '')
      setEditPersonalAccessToken('')
    }
  }, [repository])

  // Auto-select default branch when repository loads and no branch is specified
  React.useEffect(() => {
    if (repository && !branchFromQuery && branches && branches.length > 0) {
      const defaultBranch = getFallbackBranch(repository.default_branch, branches)
      setCurrentBranch(defaultBranch)
    }
  }, [repository, branchFromQuery, branches])

  // Auto-load README.md when repository loads
  React.useEffect(() => {
    if (treeData?.entries && !selectedFile) {
      const readme = treeData.entries.find(entry =>
        entry?.name?.toLowerCase() === 'readme.md' && !entry?.is_dir
      )
      if (readme?.path) {
        setSelectedFile(readme.path)
      }
    }
  }, [treeData, selectedFile])

  const handleOpenEdit = () => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditDefaultBranch(repository.default_branch || '')
      setEditKoditIndexing(repository.kodit_indexing || false)
      setEditExternalUrl(repository.external_url || '')
      setEditExternalType(repository.external_type)
      setEditUsername(repository.username || '')
      setEditPassword('')
      setEditOrganizationUrl(repository.azure_devops?.organization_url || '')
      setEditPersonalAccessToken('')
      setEditDialogOpen(true)
    }
  }

  const handleUpdateRepository = async () => {
    if (!repository || !repoId) return

    setUpdating(true)
    try {
      const apiClient = api.getApiClient()
      const updateData: any = {
        name: editName,
        description: editDescription,
        default_branch: editDefaultBranch || undefined,
        kodit_indexing: editKoditIndexing !== undefined ? editKoditIndexing : repository.kodit_indexing,
        metadata: repository.metadata,
      }

      if (repository.is_external || repository.external_url) {
        updateData.external_url = editExternalUrl || undefined
        updateData.external_type = editExternalType || undefined

        if (editExternalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO) {
          updateData.azure_devops = {
            organization_url: editOrganizationUrl || undefined,
            ...(editPersonalAccessToken && editPersonalAccessToken !== ''
              ? { personal_access_token: editPersonalAccessToken }
              : repository.azure_devops?.personal_access_token
                ? { personal_access_token: repository.azure_devops.personal_access_token }
                : {}),
          }
          updateData.username = undefined
          updateData.password = undefined
        } else {
          updateData.username = editUsername || undefined
          if (editPassword && editPassword !== '') {
            updateData.password = editPassword
          }
          updateData.azure_devops = undefined
        }
      }

      await apiClient.v1GitRepositoriesUpdate(repoId, updateData)

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repository', repoId] })
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setEditDialogOpen(false)
      setEditPassword('')
      setEditPersonalAccessToken('')
      snackbar.success('Repository updated successfully')
    } catch (error) {
      console.error('Failed to update repository:', error)
      snackbar.error('Failed to update repository')
    } finally {
      setUpdating(false)
    }
  }

  const handleDeleteRepository = async () => {
    if (!repoId) return

    setDeleting(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesDelete(repoId)

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      // Navigate back to repositories tab in Projects
      navigate('projects', { tab: 'repositories' })
      snackbar.success('Repository deleted successfully')
    } catch (error) {
      console.error('Failed to delete repository:', error)
      snackbar.error('Failed to delete repository')
      setDeleting(false)
    }
  }

  const handleCopyCloneCommand = (command: string) => {
    navigator.clipboard.writeText(command)
    setCopiedClone(true)
    snackbar.success('Clone URL copied to clipboard')
    setTimeout(() => setCopiedClone(false), 2000)
  }

  const handleCopySha = (sha: string) => {
    navigator.clipboard.writeText(sha)
    setCopiedSha(sha)
    snackbar.success('SHA copied to clipboard')
    setTimeout(() => setCopiedSha(null), 2000)
  }

  const handleViewEnrichments = (commitSha: string) => {
    router.mergeParams({ tab: 'code-intelligence', commit: commitSha })
  }

  const handlePushPull = async (force = false) => {
    if (!repoId) return

    try {
      const branch = currentBranch || repository?.default_branch || undefined
      await pushPullMutation.mutateAsync({ repositoryId: repoId, branch, force })
      snackbar.success(force ? 'Repository force pushed successfully' : 'Repository synchronized successfully')
      setForcePushDialogOpen(false)
    } catch (error: any) {
      console.error('Failed to push/pull repository:', error)
      let errorMessage: string
      if (error?.response?.data) {
        if (typeof error.response.data === 'string') {
          errorMessage = error.response.data
        } else {
          errorMessage = error.response.data.error || error.response.data.message || String(error)
        }
      } else {
        errorMessage = error?.message || String(error)
      }
      const errorLower = errorMessage.toLowerCase()
      if (errorLower.includes('non-fast-forward') || errorLower.includes('non fast forward')) {
        setForcePushDialogOpen(true)
      } else {
        snackbar.error('Failed to synchronize repository, error: ' + errorMessage)
      }
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

  const handleNavigateToDirectory = (path: string) => {
    setCurrentPath(path)
    setSelectedFile(null) // Clear file selection when navigating
  }

  const handleSelectFile = (path: string, isDir: boolean) => {
    if (isDir) {
      handleNavigateToDirectory(path)
    } else {
      setSelectedFile(path)
    }
  }

  const handleNavigateUp = () => {
    if (currentPath === '.') return
    const parts = currentPath.split('/').filter(p => p !== '.')
    parts.pop()
    const newPath = parts.length === 0 ? '.' : parts.join('/')
    setCurrentPath(newPath)
    setSelectedFile(null)
  }

  const handleCreateBranch = async () => {
    if (!repoId || !newBranchName.trim()) return

    try {
      await createBranchMutation.mutateAsync({
        repositoryId: repoId,
        request: {
          branch_name: newBranchName.trim(),
          base_branch: newBranchBase || undefined,
        }
      })

      snackbar.success('Branch created successfully')
      setCreateBranchDialogOpen(false)
      setNewBranchName('')
      setNewBranchBase('')
      setCurrentBranch(newBranchName.trim())
      setCurrentPath('.')
      setSelectedFile(null)
    } catch (error) {
      console.error('Failed to create branch:', error)
      snackbar.error('Failed to create branch')
    }
  }

  const handleCreateFile = async () => {
    if (!repoId || !newFilePath) return

    setCreatingFile(true)
    try {
      // Base64 encode content (handling unicode)
      const encodedContent = btoa(unescape(encodeURIComponent(newFileContent)))
      const branch = currentBranch || repository?.default_branch || 'main'

      await createOrUpdateFileMutation.mutateAsync({
        repositoryId: repoId,
        request: {
          path: newFilePath,
          branch: branch,
          content: encodedContent,
          message: isEditingFile ? `Update ${newFilePath}` : `Create ${newFilePath}`,
        }
      })

      // Calculate parent directory of the file being created
      const filePathParts = newFilePath.split('/').filter(p => p)
      const parentDir = filePathParts.length > 1
        ? filePathParts.slice(0, -1).join('/')
        : '.'

      // Use the same branch value as the query (currentBranch, which can be empty string)
      const queryBranch = currentBranch || ''

      // Invalidate tree queries for the parent directory and current path
      // This ensures the file list refreshes whether viewing the parent dir or a subdirectory
      const pathsToInvalidate = [parentDir]
      if (currentPath !== parentDir) {
        pathsToInvalidate.push(currentPath)
      }

      for (const path of pathsToInvalidate) {
        queryClient.invalidateQueries({
          queryKey: ['git-repositories', repoId, 'tree', path, queryBranch]
        })
      }

      // Invalidate the file query if we're editing
      if (isEditingFile) {
        queryClient.invalidateQueries({
          queryKey: ['git-repositories', repoId, 'file', newFilePath, queryBranch]
        })
      }

      snackbar.success(isEditingFile ? 'File updated successfully' : 'File created successfully')
      setCreateFileDialogOpen(false)
      setNewFilePath('')
      setNewFileContent('')
      setIsEditingFile(false)
    } catch (error) {
      console.error('Failed to create/update file:', error)
      snackbar.error(isEditingFile ? 'Failed to update file' : 'Failed to create file')
    } finally {
      setCreatingFile(false)
    }
  }

  const getPathBreadcrumbs = () => {
    if (currentPath === '.') return []
    return currentPath.split('/').filter(p => p !== '.')
  }

  if (isLoading) {
    return (
      <Page breadcrumbTitle="" orgBreadcrumbs={false}>
        <Container maxWidth="xl" sx={{ mt: 4, mb: 4 }}>
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    )
  }

  if (error || !repository) {
    return (
      <Page breadcrumbTitle="" orgBreadcrumbs={false}>
        <Container maxWidth="xl" sx={{ mt: 4, mb: 4 }}>
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Repository not found'}
          </Alert>
          <Button
            startIcon={<ArrowLeft size={16} />}
            onClick={() => navigate('projects', { tab: 'repositories' })}
          >
            Back to Repositories
          </Button>
        </Container>
      </Page>
    )
  }

  // Generate proper clone URL
  // External repos: use their external URL
  // Helix repos: use the git server URL format
  const isExternal = repository.external_url !== '' || false
  const cloneUrl = isExternal
    ? (repository.metadata?.external_url || '')
    : (repository.clone_url || `${window.location.origin}/git/${repoId}`)

  return (
    <Page
      breadcrumbTitle={repository.name || 'Repository'}
      breadcrumbs={[
        {
          title: 'Repositories',
          routeName: 'projects',
          params: { tab: 'repositories' }
        }
      ]}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl" sx={{ mt: 2, mb: 4 }}>
        {/* GitHub-style header */}
        <Box sx={{ mb: 3 }}>

          {/* Repo name and actions */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1, mb: 1, color: 'text.primary' }}>
                <GitBranch size={24} style={{ color: 'currentColor', opacity: 0.6 }} />
                <Box
                  component="span"
                  onClick={() => account.orgNavigate('projects', { tab: 'repositories' })}
                  sx={{ color: '#3b82f6', cursor: 'pointer', '&:hover': { textDecoration: 'underline' } }}
                >
                  {ownerSlug}
                </Box>
                <Box component="span" sx={{ color: 'text.secondary', fontWeight: 300 }}>/</Box>
                <Box component="span" sx={{ fontWeight: 600 }}>{repository.name}</Box>
              </Typography>

              {repository.description && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  {repository.description}
                </Typography>
              )}

              {/* Chips */}
              <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                {isExternal && (
                  <Chip
                    icon={<Link size={12} />}
                    label={repository.metadata?.external_type || 'External'}
                    size="small"
                  />
                )}
                {repository.kodit_indexing && (
                  <Chip
                    icon={<Brain size={12} />}
                    label="Code Intelligence"
                    size="small"
                    color="success"
                  />
                )}
                <Chip
                  label={repository.repo_type || 'project'}
                  size="small"
                  variant="outlined"
                />
              </Box>
            </Box>
          </Box>

          {/* Navigation tabs */}
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <Tabs value={currentTab} onChange={(_, newValue) => {
              const tabName = newValue as TabName
              setCurrentTab(tabName)
              router.mergeParams({ tab: tabName })
            }}>
              <Tab
                value="code-intelligence"
                icon={<Brain size={16} />}
                iconPosition="start"
                label="Code Intelligence"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                value="code"
                icon={<CodeIcon size={16} />}
                iconPosition="start"
                label="Code"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                value="commits"
                icon={<GitCommit size={16} />}
                iconPosition="start"
                label="Commits"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                value="pull-requests"
                icon={<GitPullRequest size={16} />}
                iconPosition="start"
                label="Pull Requests"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                value="settings"
                icon={<Settings size={16} />}
                iconPosition="start"
                label="Settings"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                value="access"
                icon={<Users size={16} />}
                iconPosition="start"
                label="Access"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
            </Tabs>
          </Box>
        </Box>

        {/* Tab panels */}
        <Box sx={{ mt: 3 }}>
          {/* Code Intelligence Tab */}
          {currentTab === 'code-intelligence' && (
            <CodeIntelligenceTab
              repository={repository}
              enrichments={enrichments}
              repoId={repoId || ''}
              commitSha={commitFromQuery}
            />
          )}

          {/* Code Tab */}
          {currentTab === 'code' && (
            <CodeTab
              repository={repository}
              enrichments={enrichments}
              groupedEnrichments={groupedEnrichmentsBySubtype}
              treeData={treeData}
              treeLoading={treeLoading}
              fileData={fileData}
              fileLoading={fileLoading}
              selectedFile={selectedFile}
              setSelectedFile={setSelectedFile}
              currentPath={currentPath}
              setCurrentPath={setCurrentPath}
              currentBranch={currentBranch}
              setCurrentBranch={setCurrentBranch}
              branches={branches}
              isExternal={isExternal}
              pushPullMutation={pushPullMutation}
              handleNavigateToDirectory={handleNavigateToDirectory}
              handleSelectFile={handleSelectFile}
              handleNavigateUp={handleNavigateUp}
              handlePushPull={handlePushPull}
              handleCreateBranch={handleCreateBranch}
              handleCreateFile={handleCreateFile}
              getPathBreadcrumbs={getPathBreadcrumbs}
              createBranchDialogOpen={createBranchDialogOpen}
              setCreateBranchDialogOpen={setCreateBranchDialogOpen}
              newBranchName={newBranchName}
              setNewBranchName={setNewBranchName}
              newBranchBase={newBranchBase}
              setNewBranchBase={setNewBranchBase}
              createFileDialogOpen={createFileDialogOpen}
              setCreateFileDialogOpen={setCreateFileDialogOpen}
              newFilePath={newFilePath}
              setNewFilePath={setNewFilePath}
              newFileContent={newFileContent}
              setNewFileContent={setNewFileContent}
              isEditingFile={isEditingFile}
              setIsEditingFile={setIsEditingFile}
              creatingFile={creatingFile}
              createBranchMutation={createBranchMutation}
              createOrUpdateFileMutation={createOrUpdateFileMutation}
            />
          )}

          {/* Settings Tab */}
          {currentTab === 'settings' && (
            <SettingsTab
              repository={repository}
              editName={editName}
              setEditName={setEditName}
              editDescription={editDescription}
              setEditDescription={setEditDescription}
              editDefaultBranch={editDefaultBranch}
              setEditDefaultBranch={setEditDefaultBranch}
              editKoditIndexing={editKoditIndexing}
              setEditKoditIndexing={setEditKoditIndexing}
              editExternalUrl={editExternalUrl}
              setEditExternalUrl={setEditExternalUrl}
              editExternalType={editExternalType}
              setEditExternalType={setEditExternalType}
              editUsername={editUsername}
              setEditUsername={setEditUsername}
              editPassword={editPassword}
              setEditPassword={setEditPassword}
              editOrganizationUrl={editOrganizationUrl}
              setEditOrganizationUrl={setEditOrganizationUrl}
              editPersonalAccessToken={editPersonalAccessToken}
              setEditPersonalAccessToken={setEditPersonalAccessToken}
              showPassword={showPassword}
              setShowPassword={setShowPassword}
              showPersonalAccessToken={showPersonalAccessToken}
              setShowPersonalAccessToken={setShowPersonalAccessToken}
              updating={updating}
              dangerZoneExpanded={dangerZoneExpanded}
              setDangerZoneExpanded={setDangerZoneExpanded}
              onUpdateRepository={handleUpdateRepository}
              onDeleteClick={() => setDeleteDialogOpen(true)}
            />
          )}

          {/* Access Tab */}
          {currentTab === 'access' && (
            <Box sx={{ maxWidth: 800 }}>
              <Paper variant="outlined" sx={{ p: 4, borderRadius: 2 }}>
                <Typography variant="h6" sx={{ mb: 1, fontWeight: 600 }}>
                  Members & Access
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                  Manage who has access to this repository and their roles.
                </Typography>

                {repository.organization_id ? (
                  <AccessManagement
                    appId={repoId || ''}
                    accessGrants={accessGrants}
                    isLoading={accessGrantsLoading}
                    isReadOnly={repository.owner_id !== account.user?.id && !account.admin}
                    onCreateGrant={handleCreateAccessGrant}
                    onDeleteGrant={handleDeleteAccessGrant}
                  />
                ) : (
                  <Box sx={{ textAlign: 'center', py: 8, backgroundColor: 'rgba(0, 0, 0, 0.02)', borderRadius: 2 }}>
                    <Users size={48} color="#656d76" style={{ marginBottom: 16 }} />
                    <Typography variant="body2" color="text.secondary">
                      This repository is not associated with an organization. Only the owner can access it.
                    </Typography>
                  </Box>
                )}
              </Paper>
            </Box>
          )}

          {/* Commits Tab */}
          {currentTab === 'commits' && (
            <CommitsTab
              repository={repository}
              commitsBranch={commitsBranch}
              setCommitsBranch={setCommitsBranch}
              branches={branches}
              commits={commits}
              commitsLoading={commitsLoading}
              handleCopySha={handleCopySha}
              copiedSha={copiedSha}
              onViewEnrichments={handleViewEnrichments}
            />
          )}

          {/* Pull Requests Tab */}
          {currentTab === 'pull-requests' && (
            <PullRequests
              repository={repository}
            />
          )}
        </Box>

        {/* Clone Dialog */}
        <Dialog open={cloneDialogOpen} onClose={() => setCloneDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Clone Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {isExternal && repository.metadata?.external_url ? (
                <>
                  <Typography variant="body2" color="text.secondary">
                    This is an external repository. Use the URL below to clone it:
                  </Typography>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <TextField
                      fullWidth
                      value={repository.metadata?.external_url || ''}
                      InputProps={{
                        readOnly: true,
                        sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                      }}
                    />
                    <Tooltip title={copiedClone ? 'Copied!' : 'Copy'}>
                      <IconButton
                        onClick={() => handleCopyCloneCommand(repository.metadata?.external_url || '')}
                        color={copiedClone ? 'success' : 'default'}
                      >
                        <Copy size={18} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                  <Button
                    variant="outlined"
                    fullWidth
                    startIcon={<ExternalLink size={16} />}
                    onClick={() => window.open(repository.metadata?.external_url || '', '_blank')}
                  >
                    Open in {repository.metadata?.external_type || 'Browser'}
                  </Button>
                </>
              ) : isExternal ? (
                <Alert severity="warning">
                  This external repository does not have a clone URL configured.
                </Alert>
              ) : (
                <>
                  <Alert severity="info">
                    This is a Helix-hosted repository. It is automatically cloned by agents when working on spec tasks.
                  </Alert>
                  <Typography variant="body2" color="text.secondary">
                    Clone command (for reference):
                  </Typography>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <TextField
                      fullWidth
                      value={cloneUrl}
                      InputProps={{
                        readOnly: true,
                        sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                      }}
                    />
                    <Tooltip title={copiedClone ? 'Copied!' : 'Copy'}>
                      <IconButton
                        onClick={() => handleCopyCloneCommand(cloneUrl)}
                        color={copiedClone ? 'success' : 'default'}
                      >
                        <Copy size={18} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                </>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setCloneDialogOpen(false)}>Close</Button>
          </DialogActions>
        </Dialog>

        {/* Edit Dialog */}
        <Dialog open={editDialogOpen} onClose={() => setEditDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Edit Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField
                label="Repository Name"
                fullWidth
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
              />

              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
              />

              <TextField
                label="Default Branch"
                fullWidth
                value={editDefaultBranch}
                onChange={(e) => setEditDefaultBranch(e.target.value)}
                helperText="The default branch for this repository"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <GitBranch size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
                    </InputAdornment>
                  ),
                }}
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={editKoditIndexing}
                    onChange={(e) => setEditKoditIndexing(e.target.checked)}
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

              {(repository?.is_external || repository?.external_url) && (
                <>
                  <Divider />
                  <TextField
                    label="External URL"
                    fullWidth
                    value={editExternalUrl}
                    onChange={(e) => setEditExternalUrl(e.target.value)}
                    helperText="Full URL to the external repository"
                  />

                  <TextField
                    label="Username"
                    fullWidth
                    value={editUsername}
                    onChange={(e) => setEditUsername(e.target.value)}
                    helperText="Username for authenticating with the external repository"
                  />

                  <TextField
                    label="Password"
                    fullWidth
                    type={showPassword ? 'text' : 'password'}
                    value={editPassword}
                    onChange={(e) => setEditPassword(e.target.value)}
                    helperText={repository?.password ? "Leave blank to keep current password" : "Password for authenticating with the external repository"}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowPassword(!showPassword)}
                            edge="end"
                            size="small"
                          >
                            {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    }}
                  />
                </>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setEditDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleUpdateRepository}
              variant="contained"
              disabled={!editName.trim() || updating}
            >
              {updating ? <CircularProgress size={20} /> : 'Save Changes'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Delete Confirmation Dialog */}
        <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Delete Repository</DialogTitle>
          <DialogContent>
            <Alert severity="error" sx={{ mb: 2 }}>
              This action cannot be undone. This will permanently delete the repository metadata.
            </Alert>
            <Typography variant="body2">
              Are you sure you want to delete <strong>{repository.name}</strong>?
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleDeleteRepository}
              variant="contained"
              color="error"
              disabled={deleting}
            >
              {deleting ? <CircularProgress size={20} /> : 'Delete Repository'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Force Push Dialog */}
        <Dialog open={forcePushDialogOpen} onClose={() => setForcePushDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Non-Fast-Forward Update</DialogTitle>
          <DialogContent>
            <Alert severity="warning" sx={{ mb: 2 }}>
              The remote branch has commits that your local branch doesn't have. A regular push cannot be performed.
            </Alert>
            <Typography variant="body2" sx={{ mb: 2 }}>
              You can force push to overwrite the remote branch with your local changes. This will discard any commits on the remote branch that aren't in your local branch.
            </Typography>
            <Typography variant="body2" color="error">
              <strong>Warning:</strong> Force pushing can cause data loss if others are working on this branch. Only proceed if you're sure you want to overwrite the remote branch.
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setForcePushDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={() => handlePushPull(true)}
              variant="contained"
              color="error"
              disabled={pushPullMutation.isPending}
            >
              {pushPullMutation.isPending ? <CircularProgress size={20} /> : 'Force Push'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Create Branch Dialog */}
        <Dialog
          open={createBranchDialogOpen}
          onClose={() => {
            setCreateBranchDialogOpen(false)
            setNewBranchName('')
            setNewBranchBase('')
          }}
          maxWidth="sm"
          fullWidth
        >
          <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              Create New Branch
            </Typography>
            <IconButton
              onClick={() => {
                setCreateBranchDialogOpen(false)
                setNewBranchName('')
                setNewBranchBase('')
              }}
              edge="end"
              size="small"
            >
              <CloseIcon size={20} />
            </IconButton>
          </DialogTitle>
          <DialogContent sx={{ pt: 4, px: 3, pb: 2 }}>
            <Stack spacing={3} pt={2}>
              <TextField
                label="Branch Name"
                fullWidth
                value={newBranchName}
                onChange={(e) => setNewBranchName(e.target.value)}
                placeholder="feature/my-feature"
                helperText="Enter a name for the new branch"
                InputProps={{
                  sx: { fontFamily: 'monospace' }
                }}
                autoFocus
              />
              <FormControl fullWidth>
                <InputLabel>Base Branch</InputLabel>
                <Select
                  value={newBranchBase}
                  onChange={(e) => setNewBranchBase(e.target.value)}
                  label="Base Branch"
                  renderValue={(value) => (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <GitBranch size={14} />
                      <span>{value}</span>
                    </Box>
                  )}
                >
                  <MenuItem value={getFallbackBranch(repository?.default_branch, branches)}>
                    {getFallbackBranch(repository?.default_branch, branches)}
                  </MenuItem>
                  {branches?.filter(b => b !== repository?.default_branch).map((branch) => (
                    <MenuItem key={branch} value={branch}>
                      {branch}
                    </MenuItem>
                  ))}
                </Select>
                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                  The branch to create from
                </Typography>
              </FormControl>
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, py: 2 }}>
            <Button
              onClick={() => {
                setCreateBranchDialogOpen(false)
                setNewBranchName('')
                setNewBranchBase('')
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={handleCreateBranch}
              color="secondary"
              variant="contained"
              disabled={!newBranchName.trim() || createBranchMutation.isPending}
            >
              {createBranchMutation.isPending ? <CircularProgress size={20} /> : 'Create Branch'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Create/Edit File Dialog */}
        <Dialog
          open={createFileDialogOpen}
          onClose={() => {
            setCreateFileDialogOpen(false)
            setIsEditingFile(false)
            setNewFilePath('')
            setNewFileContent('')
          }}
          maxWidth="lg"
          fullWidth
        >
          <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              {isEditingFile ? 'Edit File' : 'Create New File'}
            </Typography>
            <IconButton
              onClick={() => {
                setCreateFileDialogOpen(false)
                setIsEditingFile(false)
                setNewFilePath('')
                setNewFileContent('')
              }}
              edge="end"
              size="small"
            >
              <CloseIcon size={20} />
            </IconButton>
          </DialogTitle>

          <DialogContent sx={{ p: 3, height: '70vh', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, flex: 1, height: '100%', pt: 2 }}>
              <TextField
                label="Filename"
                fullWidth
                value={newFilePath}
                onChange={(e) => setNewFilePath(e.target.value)}
                placeholder="path/to/file.txt"
                disabled={isEditingFile}
                helperText={`${isEditingFile ? 'Editing' : 'Creating'} in branch ${currentBranch || repository?.default_branch || 'main'}`}
                InputProps={{
                  sx: { fontFamily: 'monospace' }
                }}
              />
              <Box sx={{
                flex: 1,
                minHeight: 0,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
                display: 'flex',
                flexDirection: 'column',
                overflow: 'hidden',
                position: 'relative'
              }}>
                <Box sx={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }}>
                  <MonacoEditor
                    value={newFileContent}
                    onChange={setNewFileContent}
                    language={
                      newFilePath.endsWith('.json') ? 'json' :
                        newFilePath.endsWith('.yaml') || newFilePath.endsWith('.yml') ? 'yaml' :
                          newFilePath.endsWith('.ts') || newFilePath.endsWith('.tsx') ? 'typescript' :
                            newFilePath.endsWith('.js') || newFilePath.endsWith('.jsx') ? 'javascript' :
                              newFilePath.endsWith('.go') ? 'go' :
                                newFilePath.endsWith('.py') ? 'python' :
                                  newFilePath.endsWith('.md') ? 'markdown' :
                                    newFilePath.endsWith('.css') ? 'css' :
                                      newFilePath.endsWith('.html') ? 'html' :
                                        newFilePath.endsWith('.sh') ? 'shell' :
                                          'plaintext'
                    }
                    height="100%"
                    autoHeight={false}
                    theme="helix-dark"
                    options={{
                      minimap: { enabled: true },
                      scrollBeyondLastLine: false,
                      fontSize: 14,
                      padding: { top: 16, bottom: 16 },
                      automaticLayout: true,
                    }}
                  />
                </Box>
              </Box>
            </Box>
          </DialogContent>
          <DialogActions sx={{ px: 3, py: 2 }}>
            <Button
              onClick={handleCreateFile}
              color="secondary"
              variant="contained"
              disabled={!newFilePath.trim() || creatingFile}
            >
              {creatingFile ? <CircularProgress size={20} /> : (isEditingFile ? 'Update File' : 'Create File')}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default GitRepoDetail
