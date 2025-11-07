import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import {
  Box,
  Typography,
  Card,
  CardContent,
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
} from '@mui/material'
import {
  GitBranch,
  Copy,
  ExternalLink,
  ArrowLeft,
  Edit,
  Brain,
  Link,
  Trash2,
  Plus,
  Folder,
  File,
  FileText,
  ChevronRight,
  X as CloseIcon,
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
} from '../services/gitRepositoryService'
import {
  useListRepositoryAccessGrants,
  useCreateRepositoryAccessGrant,
  useDeleteRepositoryAccessGrant,
} from '../services/repositoryAccessGrantService'

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
  const { data: branches = [] } = useListRepositoryBranches(repoId || '')

  // Access grants for RBAC
  const { data: accessGrants = [], isLoading: accessGrantsLoading } = useListRepositoryAccessGrants(repoId || '', !!repoId)
  const createAccessGrantMutation = useCreateRepositoryAccessGrant(repoId || '')
  const deleteAccessGrantMutation = useDeleteRepositoryAccessGrant(repoId || '')
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editKoditIndexing, setEditKoditIndexing] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [copiedClone, setCopiedClone] = useState(false)
  const [currentPath, setCurrentPath] = useState('.')
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [currentBranch, setCurrentBranch] = useState<string>('') // Empty = default branch (HEAD)

  // Browse repository tree
  const { data: treeData, isLoading: treeLoading } = useBrowseRepositoryTree(repoId || '', currentPath, currentBranch)
  const { data: fileData, isLoading: fileLoading } = useGetRepositoryFile(
    repoId || '',
    selectedFile || '',
    !!selectedFile
  )

  const handleOpenEdit = () => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditKoditIndexing(repository.metadata?.kodit_indexing || false)
      setEditDialogOpen(true)
    }
  }

  const handleUpdateRepository = async () => {
    if (!repository || !repoId) return

    setUpdating(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesUpdate(repoId, {
        name: editName,
        description: editDescription,
        metadata: {
          ...repository.metadata,
          kodit_indexing: editKoditIndexing,
        },
      })

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repository', repoId] })
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setEditDialogOpen(false)
    } catch (error) {
      console.error('Failed to update repository:', error)
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

      // Navigate back to list
      navigate('git-repos')
    } catch (error) {
      console.error('Failed to delete repository:', error)
      setDeleting(false)
    }
  }

  const handleCopyCloneCommand = (command: string) => {
    navigator.clipboard.writeText(command)
    setCopiedClone(true)
    setTimeout(() => setCopiedClone(false), 2000)
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

  const getPathBreadcrumbs = () => {
    if (currentPath === '.') return []
    return currentPath.split('/').filter(p => p !== '.')
  }

  if (isLoading) {
    return (
      <Page breadcrumbTitle="" orgBreadcrumbs={false}>
        <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
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
        <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Repository not found'}
          </Alert>
          <Button
            startIcon={<ArrowLeft size={16} />}
            onClick={() => navigate('git-repos')}
          >
            Back to Repositories
          </Button>
        </Container>
      </Page>
    )
  }

  const cloneUrl = repository.clone_url || `git@github.com:${ownerSlug}/${repository.name}.git`
  const isExternal = repository.metadata?.is_external || false

  return (
    <Page breadcrumbTitle="" orgBreadcrumbs={false}>
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        {/* Header with breadcrumb-style navigation */}
        <Box sx={{ mb: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
            <Button
              startIcon={<ArrowLeft size={16} />}
              onClick={() => navigate('git-repos')}
              sx={{ textTransform: 'none', color: '#0969da' }}
            >
              Repositories
            </Button>
          </Box>

          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <GitBranch size={32} color="#656d76" />
              <Box>
                <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
                  <span
                    style={{ color: '#0969da', cursor: 'pointer' }}
                    onClick={() => navigate('git-repos')}
                  >
                    {ownerSlug}
                  </span>
                  <span style={{ color: '#656d76', fontWeight: 300 }}>/</span>
                  <span style={{ fontWeight: 600 }}>{repository.name}</span>
                </Typography>
                {repository.description && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    {repository.description}
                  </Typography>
                )}
              </Box>
            </Box>

            <Box sx={{ display: 'flex', gap: 1 }}>
              <IconButton
                onClick={handleOpenEdit}
                size="small"
                sx={{ border: 1, borderColor: 'divider' }}
              >
                <Edit size={16} />
              </IconButton>
              <IconButton
                onClick={() => setDeleteDialogOpen(true)}
                size="small"
                sx={{ border: 1, borderColor: 'divider' }}
              >
                <Trash2 size={16} />
              </IconButton>
            </Box>
          </Box>

          {/* Chips */}
          <Box sx={{ display: 'flex', gap: 1, mt: 2 }}>
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

        <Divider sx={{ mb: 3 }} />

        {/* New Spec Task Button */}
        <Box sx={{ mb: 3 }}>
          <Button
            variant="contained"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={() => navigate('spec-tasks', { new: 'true', repo_id: repoId })}
            sx={{ textTransform: 'none' }}
          >
            New Spec Task
          </Button>
        </Box>

        {/* Clone instructions */}
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Clone this repository
            </Typography>

            {isExternal && repository.metadata?.external_url ? (
              <>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  This is an external repository. Use the URL below to clone it:
                </Typography>
                <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                  <TextField
                    fullWidth
                    value={repository.metadata.external_url}
                    InputProps={{
                      readOnly: true,
                      sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                    }}
                  />
                  <Button
                    variant="outlined"
                    startIcon={copiedClone ? undefined : <Copy size={16} />}
                    onClick={() => handleCopyCloneCommand(repository.metadata.external_url)}
                  >
                    {copiedClone ? 'Copied!' : 'Copy'}
                  </Button>
                  <Button
                    variant="outlined"
                    startIcon={<ExternalLink size={16} />}
                    onClick={() => window.open(repository.metadata.external_url, '_blank')}
                  >
                    Open
                  </Button>
                </Box>
              </>
            ) : isExternal ? (
              <Alert severity="warning">
                <Typography variant="body2">
                  This external repository does not have a clone URL configured.
                </Typography>
              </Alert>
            ) : (
              <Alert severity="info">
                <Typography variant="body2">
                  This is a Helix-hosted repository. It is automatically cloned by agents when working on tasks.
                  You can browse files using the file browser below.
                </Typography>
              </Alert>
            )}
          </CardContent>
        </Card>

        {/* Repository information */}
        <Card>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Repository Information
            </Typography>

            <Stack spacing={2}>
              <Box>
                <Typography variant="caption" color="text.secondary">
                  Repository ID
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {repository.id}
                </Typography>
              </Box>

              <Box>
                <Typography variant="caption" color="text.secondary">
                  Type
                </Typography>
                <Typography variant="body2">
                  {repository.repo_type || 'project'}
                </Typography>
              </Box>

              {repository.default_branch && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Default Branch
                  </Typography>
                  <Typography variant="body2">
                    {repository.default_branch}
                  </Typography>
                </Box>
              )}

              <Box>
                <Typography variant="caption" color="text.secondary">
                  Created
                </Typography>
                <Typography variant="body2">
                  {repository.created_at ? new Date(repository.created_at).toLocaleString() : 'N/A'}
                </Typography>
              </Box>

              {repository.updated_at && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Last Updated
                  </Typography>
                  <Typography variant="body2">
                    {new Date(repository.updated_at).toLocaleString()}
                  </Typography>
                </Box>
              )}
            </Stack>
          </CardContent>
        </Card>

        {/* File Browser */}
        <Card sx={{ mt: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Browse Files
            </Typography>

            {/* Branch selector */}
            <Box sx={{ mb: 2 }}>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel>Branch</InputLabel>
                <Select
                  value={currentBranch}
                  label="Branch"
                  onChange={(e) => {
                    setCurrentBranch(e.target.value)
                    setCurrentPath('.') // Reset to root when switching branches
                    setSelectedFile(null) // Clear selected file
                  }}
                  startAdornment={<GitBranch size={16} style={{ marginRight: 8, marginLeft: 4 }} />}
                >
                  <MenuItem value="">
                    <em>Default ({repository?.default_branch || 'main'})</em>
                  </MenuItem>
                  {branches.map((branch) => (
                    <MenuItem key={branch} value={branch}>
                      {branch}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>

            {/* Breadcrumb navigation */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2, flexWrap: 'wrap' }}>
              <Chip
                label={repository.name}
                size="small"
                onClick={() => handleNavigateToDirectory('.')}
                sx={{ cursor: 'pointer' }}
              />
              {getPathBreadcrumbs().map((part, index, arr) => {
                const path = arr.slice(0, index + 1).join('/')
                return (
                  <React.Fragment key={path}>
                    <ChevronRight size={16} color="#656d76" />
                    <Chip
                      label={part}
                      size="small"
                      onClick={() => handleNavigateToDirectory(path)}
                      sx={{ cursor: 'pointer' }}
                    />
                  </React.Fragment>
                )
              })}
            </Box>

            <Divider sx={{ mb: 2 }} />

            {treeLoading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : (
              <Box sx={{ display: 'flex', gap: 2 }}>
                {/* File tree */}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  {currentPath !== '.' && (
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        p: 1,
                        cursor: 'pointer',
                        borderRadius: 1,
                        '&:hover': {
                          backgroundColor: 'rgba(0, 0, 0, 0.04)',
                        },
                      }}
                      onClick={handleNavigateUp}
                    >
                      <Folder size={16} color="#54aeff" />
                      <Typography variant="body2">
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
                            gap: 1,
                            p: 1,
                            cursor: 'pointer',
                            borderRadius: 1,
                            backgroundColor: selectedFile === entry.path ? 'rgba(25, 118, 210, 0.08)' : 'transparent',
                            '&:hover': {
                              backgroundColor: selectedFile === entry.path
                                ? 'rgba(25, 118, 210, 0.12)'
                                : 'rgba(0, 0, 0, 0.04)',
                            },
                          }}
                          onClick={() => handleSelectFile(entry.path || '', entry.is_dir || false)}
                        >
                          {entry.is_dir ? (
                            <Folder size={16} color="#54aeff" />
                          ) : (
                            <FileText size={16} color="#656d76" />
                          )}
                          <Typography variant="body2" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {entry.name}
                          </Typography>
                          {!entry.is_dir && (
                            <Typography variant="caption" color="text.secondary">
                              {(entry.size || 0) > 1024
                                ? `${Math.round((entry.size || 0) / 1024)} KB`
                                : `${entry.size || 0} B`}
                            </Typography>
                          )}
                        </Box>
                      ))
                  ) : (
                    <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
                      Empty directory
                    </Typography>
                  )}
                </Box>

                {/* File viewer */}
                {selectedFile && (
                  <Box sx={{ flex: 2, minWidth: 0 }}>
                    <Card variant="outlined">
                      <CardContent>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                            {selectedFile}
                          </Typography>
                          <IconButton size="small" onClick={() => setSelectedFile(null)}>
                            <CloseIcon />
                          </IconButton>
                        </Box>
                        {fileLoading ? (
                          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                            <CircularProgress size={20} />
                          </Box>
                        ) : (
                          <Box
                            component="pre"
                            sx={{
                              fontFamily: 'monospace',
                              fontSize: '0.75rem',
                              backgroundColor: 'background.paper',
                              color: 'text.primary',
                              padding: 2,
                              borderRadius: 1,
                              overflow: 'auto',
                              maxHeight: '500px',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                              border: 1,
                              borderColor: 'divider',
                            }}
                          >
                            {fileData?.content || 'No content'}
                          </Box>
                        )}
                      </CardContent>
                    </Card>
                  </Box>
                )}
              </Box>
            )}
          </CardContent>
        </Card>

        {/* Members & Access Control */}
        <Card sx={{ mt: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Members & Access
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Manage who has access to this repository and their roles.
            </Typography>
            <Divider sx={{ mb: 2 }} />

            {repository.organization_id ? (
              <AccessManagement
                appId={repoId || ''}
                accessGrants={accessGrants}
                isLoading={accessGrantsLoading}
                isReadOnly={repository.owner_id !== account.user?.id && !account.user?.admin}
                onCreateGrant={handleCreateAccessGrant}
                onDeleteGrant={handleDeleteAccessGrant}
              />
            ) : (
              <Box sx={{ textAlign: 'center', py: 4, backgroundColor: 'rgba(0, 0, 0, 0.02)', borderRadius: 1 }}>
                <Typography variant="body2" color="text.secondary">
                  This repository is not associated with an organization. Only the owner can access it.
                </Typography>
              </Box>
            )}
          </CardContent>
        </Card>

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
            <Alert severity="warning" sx={{ mb: 2 }}>
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
      </Container>
    </Page>
  )
}

export default GitRepoDetail
