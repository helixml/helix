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
  InputAdornment,
  Tabs,
  Tab,
  Tooltip,
  Paper,
  Collapse,
} from '@mui/material'
import {
  GitBranch,
  Copy,
  ExternalLink,
  ArrowLeft, 
  Brain,
  Link,
  Trash2,
  Folder,
  FileText,
  ChevronRight,
  ChevronDown,
  X as CloseIcon,
  Settings,
  Users,
  Code as CodeIcon,
  Eye,
  EyeOff,
  Plus,
  Pencil,
  ArrowUpDown,
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
  useCreateOrUpdateRepositoryFile,
  usePushPullGitRepository,
} from '../services/gitRepositoryService'
import {
  useListRepositoryAccessGrants,
  useCreateRepositoryAccessGrant,
  useDeleteRepositoryAccessGrant,
} from '../services/repositoryAccessGrantService'
import {
  useKoditEnrichments,
  groupEnrichmentsByType,
  getEnrichmentTypeName,
  getEnrichmentTypeIcon,
} from '../services/koditService'
import MonacoEditor from '../components/widgets/MonacoEditor'

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

  // Kodit code intelligence enrichments
  const { data: enrichmentsData } = useKoditEnrichments(repoId || '', { enabled: !!repoId })
  const enrichments = enrichmentsData?.data || []
  const groupedEnrichments = groupEnrichmentsByType(enrichments)

  // UI State
  const [currentTab, setCurrentTab] = useState(0)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [cloneDialogOpen, setCloneDialogOpen] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editDefaultBranch, setEditDefaultBranch] = useState('')
  const [editKoditIndexing, setEditKoditIndexing] = useState(false)
  const [editExternalUrl, setEditExternalUrl] = useState('')
  const [editUsername, setEditUsername] = useState('')
  const [editPassword, setEditPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [copiedClone, setCopiedClone] = useState(false)
  const [currentPath, setCurrentPath] = useState('.')
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [currentBranch, setCurrentBranch] = useState<string>('') // Empty = default branch (HEAD)
  const [dangerZoneExpanded, setDangerZoneExpanded] = useState(false)

  // Create/Edit File Dialog State
  const [createFileDialogOpen, setCreateFileDialogOpen] = useState(false)
  const [isEditingFile, setIsEditingFile] = useState(false)
  const [newFilePath, setNewFilePath] = useState('')
  const [newFileContent, setNewFileContent] = useState('')
  const [creatingFile, setCreatingFile] = useState(false)

  // Browse repository tree
  const { data: treeData, isLoading: treeLoading } = useBrowseRepositoryTree(repoId || '', currentPath, currentBranch)
  const { data: fileData, isLoading: fileLoading } = useGetRepositoryFile(
    repoId || '',
    selectedFile || '',
    currentBranch,
    !!selectedFile
  )

  // Initialize edit fields when repository loads
  React.useEffect(() => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditDefaultBranch(repository.default_branch || '')
      setEditKoditIndexing(repository.metadata?.kodit_indexing || false)
      setEditExternalUrl(repository.external_url || '')
      setEditUsername(repository.username || '')
      setEditPassword('')
    }
  }, [repository])

  // Auto-load README.md when repository loads
  React.useEffect(() => {
    if (treeData?.entries && !selectedFile) {
      const readme = treeData.entries.find(entry =>
        entry.name?.toLowerCase() === 'readme.md' && !entry.is_dir
      )
      if (readme && readme.path) {
        setSelectedFile(readme.path)
      }
    }
  }, [treeData, selectedFile])

  const handleOpenEdit = () => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditDefaultBranch(repository.default_branch || '')
      setEditKoditIndexing(repository.metadata?.kodit_indexing || false)
      setEditExternalUrl(repository.external_url || '')
      setEditUsername(repository.username || '')
      setEditPassword('')
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
        metadata: {
          ...repository.metadata,
          kodit_indexing: editKoditIndexing,
        },
      }

      if (repository.is_external || repository.external_url) {
        updateData.external_url = editExternalUrl || undefined
        updateData.username = editUsername || undefined
      }
      if (editPassword && editPassword !== '') {
        updateData.password = editPassword
      }

      await apiClient.v1GitRepositoriesUpdate(repoId, updateData)

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repository', repoId] })
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setEditDialogOpen(false)
      setEditPassword('')
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

  const handlePushPull = async () => {
    if (!repoId) return

    try {
      const branch = currentBranch || repository?.default_branch || undefined
      await pushPullMutation.mutateAsync({ repositoryId: repoId, branch })
      snackbar.success('Repository synchronized successfully')
    } catch (error) {
      console.error('Failed to push/pull repository:', error)
      snackbar.error('Failed to synchronize repository')
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

  const handleCreateFile = async () => {
    if (!repoId || !newFilePath) return

    setCreatingFile(true)
    try {
      // Base64 encode content (handling unicode)
      const encodedContent = btoa(unescape(encodeURIComponent(newFileContent)))
      const branch = currentBranch || repository.default_branch || 'main'

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
                  onClick={() => navigate('projects', { tab: 'repositories' })}
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
                {repository.metadata?.kodit_indexing && (
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
            <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
              <Tab
                icon={<CodeIcon size={16} />}
                iconPosition="start"
                label="Code"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
                icon={<Settings size={16} />}
                iconPosition="start"
                label="Settings"
                sx={{ textTransform: 'none', minHeight: 48 }}
              />
              <Tab
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
          {/* Code Tab */}
          {currentTab === 0 && (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
              {/* Code Intelligence - Architecture enrichments from Kodit */}
              {enrichments.length > 0 && groupedEnrichments['architecture'] && (
                <Paper variant="outlined" sx={{ borderRadius: 2, p: 3, bgcolor: 'rgba(0, 213, 255, 0.04)', borderColor: 'rgba(0, 213, 255, 0.2)' }}>
                  <Typography variant="h6" sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2, fontWeight: 600 }}>
                    {getEnrichmentTypeIcon('architecture')} {getEnrichmentTypeName('architecture')} Insights
                  </Typography>
                  <Stack spacing={2}>
                    {groupedEnrichments['architecture'].map((enrichment: any, index: number) => (
                      <Box key={enrichment.id || index}>
                        {enrichment.attributes?.subtype && (
                          <Chip
                            label={enrichment.attributes.subtype}
                            size="small"
                            sx={{ mb: 1, bgcolor: 'rgba(0, 213, 255, 0.15)', color: '#00d5ff', fontWeight: 600 }}
                          />
                        )}
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', lineHeight: 1.7 }}>
                          {enrichment.attributes?.content}
                        </Typography>
                      </Box>
                    ))}
                  </Stack>
                </Paper>
              )}

              {/* File browser and file viewer */}
              <Box sx={{ display: 'flex', gap: 3 }}>
                {/* Main content - File browser */}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                <Paper variant="outlined" sx={{ borderRadius: 2 }}>
                  {/* Branch selector bar */}
                  <Box sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
                    p: 2,
                    borderBottom: 1,
                    borderColor: 'divider',
                    bgcolor: 'rgba(0, 0, 0, 0.02)'
                  }}>
                    <FormControl size="small" sx={{ minWidth: 200 }}>
                      <Select
                        value={currentBranch}
                        onChange={(e) => {
                          setCurrentBranch(e.target.value)
                          setCurrentPath('.') // Reset to root when switching branches
                          setSelectedFile(null) // Clear selected file
                        }}
                        displayEmpty
                        renderValue={(value) => (
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <GitBranch size={14} />
                            <span>{value || repository?.default_branch || 'main'}</span>
                          </Box>
                        )}
                        sx={{ fontWeight: 500 }}
                      >
                        <MenuItem value="">
                          {repository?.default_branch || 'main'}
                        </MenuItem>
                        {branches.filter(b => b !== repository?.default_branch).map((branch) => (
                          <MenuItem key={branch} value={branch}>
                            {branch}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>                    
                    <Button
                      startIcon={<Plus size={16} />}
                      variant="outlined"
                      size="small"
                      onClick={() => {
                        setNewFilePath(currentPath === '.' ? '' : `${currentPath}/`)
                        setNewFileContent('')
                        setIsEditingFile(false)
                        setCreateFileDialogOpen(true)
                      }}
                      sx={{  height: 40, whiteSpace: 'nowrap' }}
                    >
                      Add File
                    </Button>
                    {isExternal && (
                      <Button
                        startIcon={<ArrowUpDown size={16} />}
                        variant="outlined"
                        size="small"
                        onClick={handlePushPull}
                        disabled={pushPullMutation.isPending}
                        sx={{ height: 40, whiteSpace: 'nowrap' }}
                      >
                        {pushPullMutation.isPending ? 'Syncing...' : 'Sync'}
                      </Button>
                    )}

                    {/* Breadcrumb navigation */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, overflow: 'auto' }}>
                      
                      {getPathBreadcrumbs().map((part, index, arr) => {
                        const path = arr.slice(0, index + 1).join('/')
                        const isLast = index === arr.length - 1
                        return (
                          <React.Fragment key={path}>
                            <ChevronRight size={14} color="#656d76" />
                            <Chip
                              label={part}
                              size="small"
                              onClick={() => handleNavigateToDirectory(path)}
                              sx={{ cursor: 'pointer', fontWeight: 500 }}
                              variant={isLast ? 'filled' : 'outlined'}
                            />
                          </React.Fragment>
                        )
                      })}
                    </Box>
                  </Box>

                  {/* File tree */}
                  <Box>
                    {treeLoading ? (
                      <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                        <CircularProgress size={24} />
                      </Box>
                    ) : (
                      <>
                        {currentPath !== '.' && (
                          <Box
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: 2,
                              px: 3,
                              py: 1.5,
                              cursor: 'pointer',
                              borderBottom: 1,
                              borderColor: 'divider',
                              '&:hover': {
                                backgroundColor: 'rgba(0, 0, 0, 0.02)',
                              },
                            }}
                            onClick={handleNavigateUp}
                          >
                            <Folder size={18} color="#54aeff" />
                            <Typography variant="body2" sx={{ fontWeight: 500 }}>
                              ..
                            </Typography>
                          </Box>
                        )}

                        {treeData?.entries && treeData.entries.length > 0 ? (
                          treeData.entries
                            .sort((a, b) => {
                              // Directories first, then files
                              if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
                              return (a.name || '').localeCompare(b.name || '')
                            })
                            .map((entry) => (
                              <Box
                                key={entry.path}
                                sx={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: 2,
                                  px: 3,
                                  py: 1.5,
                                  cursor: 'pointer',
                                  borderBottom: 1,
                                  borderColor: 'divider',
                                  backgroundColor: selectedFile === entry.path ? 'rgba(25, 118, 210, 0.08)' : 'transparent',
                                  '&:hover': {
                                    backgroundColor: selectedFile === entry.path
                                      ? 'rgba(25, 118, 210, 0.12)'
                                      : 'rgba(0, 0, 0, 0.02)',
                                  },
                                  '&:last-child': {
                                    borderBottom: 0,
                                  },
                                }}
                                onClick={() => handleSelectFile(entry.path || '', entry.is_dir || false)}
                              >
                                {entry.is_dir ? (
                                  <Folder size={18} color="#54aeff" />
                                ) : (
                                  <FileText size={18} color="#656d76" />
                                )}
                                <Typography variant="body2" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', fontWeight: 500 }}>
                                  {entry.name}
                                </Typography>
                                {!entry.is_dir && entry.size !== undefined && (
                                  <Typography variant="caption" color="text.secondary">
                                    {entry.size > 1024
                                      ? `${Math.round(entry.size / 1024)} KB`
                                      : `${entry.size} B`}
                                  </Typography>
                                )}
                              </Box>
                            ))
                        ) : (
                          <Box sx={{ py: 8, textAlign: 'center' }}>
                            <Typography variant="body2" color="text.secondary">
                              Empty directory
                            </Typography>
                          </Box>
                        )}
                      </>
                    )}
                  </Box>
                </Paper>

                {/* File viewer */}
                {selectedFile && (
                  <Paper variant="outlined" sx={{ mt: 3, borderRadius: 2 }}>
                    <Box sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      px: 3,
                      py: 2,
                      borderBottom: 1,
                      borderColor: 'divider',
                      bgcolor: 'rgba(0, 0, 0, 0.02)'
                    }}>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontWeight: 600 }}>
                        {selectedFile.split('/').pop()}
                      </Typography>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <Tooltip title="Edit file">
                          <IconButton 
                            size="small" 
                            onClick={() => {
                              if (selectedFile && fileData?.content) {
                                setNewFilePath(selectedFile)
                                setNewFileContent(fileData.content)
                                setIsEditingFile(true)
                                setCreateFileDialogOpen(true)
                              }
                            }}
                          >
                            <Pencil size={16} />
                          </IconButton>
                        </Tooltip>
                        <IconButton size="small" onClick={() => setSelectedFile(null)}>
                          <CloseIcon size={16} />
                        </IconButton>
                      </Box>
                    </Box>
                    {fileLoading ? (
                      <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                        <CircularProgress size={24} />
                      </Box>
                    ) : (
                      <Box
                        component="pre"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.875rem',
                          color: 'text.primary',
                          p: 3,
                          overflow: 'auto',
                          maxHeight: '600px',
                          whiteSpace: 'pre',
                          margin: 0,
                        }}
                      >
                        {fileData?.content || 'No content'}
                      </Box>
                    )}
                  </Paper>
                )}
              </Box>

              {/* Sidebar - About */}
              <Box sx={{ width: 300, flexShrink: 0 }}>
                <Paper variant="outlined" sx={{ p: 3, borderRadius: 2 }}>
                  <Typography variant="h6" sx={{ mb: 2, fontWeight: 600, fontSize: '1rem' }}>
                    About
                  </Typography>

                  <Stack spacing={2}>
                    {repository.description && (
                      <Typography variant="body2" color="text.secondary">
                        {repository.description}
                      </Typography>
                    )}

                    <Divider />

                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                        Type
                      </Typography>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {repository.repo_type || 'project'}
                      </Typography>
                    </Box>

                    {repository.default_branch && (
                      <Box>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                          Default Branch
                        </Typography>
                        <Chip
                          icon={<GitBranch size={12} />}
                          label={repository.default_branch}
                          size="small"
                          sx={{ fontWeight: 500 }}
                        />
                      </Box>
                    )}

                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                        Created
                      </Typography>
                      <Typography variant="body2">
                        {repository.created_at ? new Date(repository.created_at).toLocaleDateString() : 'N/A'}
                      </Typography>
                    </Box>

                    {repository.updated_at && (
                      <Box>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                          Last Updated
                        </Typography>
                        <Typography variant="body2">
                          {new Date(repository.updated_at).toLocaleDateString()}
                        </Typography>
                      </Box>
                    )}
                  </Stack>
                </Paper>
              </Box>
            </Box>
            </Box>
          )}

          {/* Settings Tab */}
          {currentTab === 1 && (
            <Box sx={{ maxWidth: 800 }}>
              <Paper variant="outlined" sx={{ p: 4, borderRadius: 2 }}>
                <Typography variant="h6" sx={{ mb: 3, fontWeight: 600 }}>
                  Repository Settings
                </Typography>

                <Stack spacing={3}>
                  <TextField
                    label="Repository Name"
                    fullWidth
                    value={editName || repository.name}
                    onChange={(e) => setEditName(e.target.value)}
                    helperText="The name of this repository"
                  />

                  <TextField
                    label="Description"
                    fullWidth
                    multiline
                    rows={3}
                    value={editDescription || repository.description}
                    onChange={(e) => setEditDescription(e.target.value)}
                    helperText="A short description of what this repository contains"
                  />

                  <TextField
                    label="Default Branch"
                    fullWidth
                    value={editDefaultBranch || repository.default_branch || ''}
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
                        checked={editKoditIndexing !== undefined ? editKoditIndexing : (repository.metadata?.kodit_indexing || false)}
                        onChange={(e) => setEditKoditIndexing(e.target.checked)}
                        color="primary"
                      />
                    }
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Brain size={18} />
                        <Box>
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            Code Intelligence
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            Index this repository with Kodit for AI-powered code understanding
                          </Typography>
                        </Box>
                      </Box>
                    }
                  />

                  <Divider />

                  {(repository.is_external || repository.external_url) && (
                    <>
                      <Typography variant="h6" sx={{ mt: 2, mb: 1, fontWeight: 600 }}>
                        External Repository Settings
                      </Typography>

                      <TextField
                        label="External URL"
                        fullWidth
                        value={editExternalUrl || repository.external_url || ''}
                        onChange={(e) => setEditExternalUrl(e.target.value)}
                        helperText="Full URL to the external repository (e.g., https://github.com/org/repo)"
                        InputProps={{
                          startAdornment: (
                            <InputAdornment position="start">
                              <ExternalLink size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
                            </InputAdornment>
                          ),
                        }}
                      />

                      <TextField
                        label="Username"
                        fullWidth
                        value={editUsername || repository.username || ''}
                        onChange={(e) => setEditUsername(e.target.value)}
                        helperText="Username for authenticating with the external repository"
                      />

                      <TextField
                        label="Password"
                        fullWidth
                        type={showPassword ? 'text' : 'password'}
                        value={editPassword}
                        onChange={(e) => setEditPassword(e.target.value)}
                        helperText={repository.password ? "Leave blank to keep current password" : "Password for authenticating with the external repository"}
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

                  <Divider />

                  <Box sx={{ display: 'flex', gap: 2 }}>
                    {/* Add spacing between buttons */}
                    <Box sx={{ flex: 1 }} />
                    <Button
                      color="secondary"
                      onClick={handleUpdateRepository}
                      variant="contained"
                      disabled={updating}
                    >
                      {updating ? <CircularProgress size={20} /> : 'Save Changes'}
                    </Button>                    
                  </Box>

                  <Divider sx={{ my: 2 }} />

                  <Box>
                    <Box
                      onClick={() => setDangerZoneExpanded(!dangerZoneExpanded)}
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        cursor: 'pointer',
                        mb: dangerZoneExpanded ? 2 : 0,
                        '&:hover': {
                          opacity: 0.8,
                        },
                      }}
                    >
                      <Typography variant="h6" sx={{ fontWeight: 600, color: 'error.main' }}>
                        Danger Zone
                      </Typography>
                      <ChevronDown
                        size={20}
                        style={{
                          color: 'var(--mui-palette-error-main)',
                          transform: dangerZoneExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                          transition: 'transform 0.2s',
                        }}
                      />
                    </Box>
                    <Collapse in={dangerZoneExpanded}>
                      <Box>
                        <Alert severity="error" sx={{ mb: 2 }}>
                          Once you delete a repository, there is no going back. This action cannot be undone.
                        </Alert>
                        <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                          <Button
                            onClick={() => setDeleteDialogOpen(true)}
                            variant="outlined"
                            color="error"
                            startIcon={<Trash2 size={16} />}
                          >
                            Delete Repository
                          </Button>
                        </Box>
                      </Box>
                    </Collapse>
                  </Box>
                </Stack>
              </Paper>
            </Box>
          )}

          {/* Access Tab */}
          {currentTab === 2 && (
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
                      value={repository.metadata.external_url}
                      InputProps={{
                        readOnly: true,
                        sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                      }}
                    />
                    <Tooltip title={copiedClone ? 'Copied!' : 'Copy'}>
                      <IconButton
                        onClick={() => handleCopyCloneCommand(repository.metadata.external_url)}
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
                    onClick={() => window.open(repository.metadata.external_url, '_blank')}
                  >
                    Open in {repository.metadata.external_type || 'Browser'}
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
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, flex: 1, height: '100%' }}>
              <TextField
                label="Filename"
                fullWidth
                value={newFilePath}
                onChange={(e) => setNewFilePath(e.target.value)}
                placeholder="path/to/file.txt"
                disabled={isEditingFile}
                helperText={`${isEditingFile ? 'Editing' : 'Creating'} in branch ${currentBranch || repository.default_branch || 'main'}`}
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
