import React, { FC } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  Button,
  Chip,
  Paper,
  Stack,
  Divider,
  IconButton,
  Tooltip,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Alert,
} from '@mui/material'
import {
  GitBranch,
  Folder,
  FileText,
  ChevronRight,
  Plus,
  ArrowUpDown,
  Pencil,
  X as CloseIcon,
  MoreVertical,
  GitPullRequest,
  ExternalLink,
} from 'lucide-react'
import {
  getEnrichmentTypeIcon,
  getEnrichmentTypeName,
} from '../../services/koditService'
import {
  useCreateGitRepositoryPullRequest,
  useListRepositoryPullRequests,
} from '../../services/gitRepositoryService'
import useSnackbar from '../../hooks/useSnackbar'
import BranchSelect from './BranchSelect'

interface CodeTabProps {
  repository: any
  enrichments: any[]
  groupedEnrichments: any
  treeData: any
  treeLoading: boolean
  fileData: any
  fileLoading: boolean
  selectedFile: string | null
  setSelectedFile: (file: string | null) => void
  currentPath: string
  setCurrentPath: (path: string) => void
  currentBranch: string
  setCurrentBranch: (branch: string) => void
  branches: string[]
  isExternal: boolean
  pushPullMutation: any
  handleNavigateToDirectory: (path: string) => void
  handleSelectFile: (path: string, isDir: boolean) => void
  handleNavigateUp: () => void
  handlePushPull: () => void
  handleCreateBranch: () => void
  handleCreateFile: () => void
  getPathBreadcrumbs: () => string[]
  createBranchDialogOpen: boolean
  setCreateBranchDialogOpen: (open: boolean) => void
  newBranchName: string
  setNewBranchName: (name: string) => void
  newBranchBase: string
  setNewBranchBase: (base: string) => void
  createFileDialogOpen: boolean
  setCreateFileDialogOpen: (open: boolean) => void
  newFilePath: string
  setNewFilePath: (path: string) => void
  newFileContent: string
  setNewFileContent: (content: string) => void
  isEditingFile: boolean
  setIsEditingFile: (editing: boolean) => void
  creatingFile: boolean
  createBranchMutation: any
  createOrUpdateFileMutation: any
}

const CodeTab: FC<CodeTabProps> = ({
  repository,
  enrichments,
  groupedEnrichments,
  treeData,
  treeLoading,
  fileData,
  fileLoading,
  selectedFile,
  setSelectedFile,
  currentPath,
  setCurrentPath,
  currentBranch,
  setCurrentBranch,
  branches,
  isExternal,
  pushPullMutation,
  handleNavigateToDirectory,
  handleSelectFile,
  handleNavigateUp,
  handlePushPull,
  handleCreateBranch,
  handleCreateFile,
  getPathBreadcrumbs,
  createBranchDialogOpen,
  setCreateBranchDialogOpen,
  newBranchName,
  setNewBranchName,
  newBranchBase,
  setNewBranchBase,
  createFileDialogOpen,
  setCreateFileDialogOpen,
  newFilePath,
  setNewFilePath,
  newFileContent,
  setNewFileContent,
  isEditingFile,
  setIsEditingFile,
  creatingFile,
  createBranchMutation,
  createOrUpdateFileMutation,
}) => {
  const createPullRequestMutation = useCreateGitRepositoryPullRequest()
  const { data: pullRequests = [] } = useListRepositoryPullRequests(repository?.id || '')
  const snackbar = useSnackbar()

  const [anchorEl, setAnchorEl] = React.useState<null | HTMLElement>(null)
  const openMenu = Boolean(anchorEl)
  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
  }

  // Create PR Dialog State
  const [createPRDialogOpen, setCreatePRDialogOpen] = React.useState(false)
  const [existingPR, setExistingPR] = React.useState<any>(null)

  const handleCheckAndOpenCreatePR = () => {
    // Check if PR already exists
    const existing = pullRequests.find(pr => pr.source_branch === currentBranch)
    if (existing) {
      setExistingPR(existing)
    } else {
      setExistingPR(null)
    }
    setCreatePRDialogOpen(true)
  }

  const handleCreatePR = async () => {
    if (!repository?.id) return

    try {
      await createPullRequestMutation.mutateAsync({
        repositoryId: repository.id,
        request: {
          title: `Pull Request from ${currentBranch}`,
          source_branch: currentBranch,
          target_branch: repository.default_branch || 'main',
        }
      })
      setCreatePRDialogOpen(false)
      snackbar.success('Pull request created successfully')
    } catch (error) {
      console.error('Failed to create PR:', error)
      snackbar.error('Failed to create pull request')
    }
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
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

      <Box sx={{ display: 'flex', gap: 3 }}>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Paper variant="outlined" sx={{ borderRadius: 2 }}>
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              p: 2,
              borderBottom: 1,
              borderColor: 'divider',
              bgcolor: 'rgba(0, 0, 0, 0.02)'
            }}>
              <BranchSelect
                repository={repository}
                currentBranch={currentBranch}
                setCurrentBranch={setCurrentBranch}
                branches={branches}
                showNewBranchButton={false}
                onBranchChange={(branch) => {
                  setCurrentPath('.')
                  setSelectedFile(null)
                }}
              />

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

              <IconButton onClick={handleMenuClick} size="small">
                <MoreVertical size={20} />
              </IconButton>

              <Menu
                anchorEl={anchorEl}
                open={openMenu}
                onClose={handleMenuClose}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
              >
                <MenuItem
                  disabled={
                    currentBranch === (repository?.default_branch || 'main') ||
                    currentBranch === 'master'
                  }
                  onClick={() => {
                    handleMenuClose()
                    handleCheckAndOpenCreatePR()
                  }}
                >
                  <ListItemIcon>
                    <GitPullRequest size={16} />
                  </ListItemIcon>
                  <ListItemText>Create Pull Request</ListItemText>
                </MenuItem>

                <MenuItem
                  onClick={() => {
                    handleMenuClose()
                    setNewBranchName('')
                    setNewBranchBase(currentBranch || repository?.default_branch || 'main')
                    setCreateBranchDialogOpen(true)
                  }}
                >
                  <ListItemIcon>
                    <GitBranch size={16} />
                  </ListItemIcon>
                  <ListItemText>New Branch</ListItemText>
                </MenuItem>

                <MenuItem
                  onClick={() => {
                    handleMenuClose()
                    setNewFilePath(currentPath === '.' ? '' : `${currentPath}/`)
                    setNewFileContent('')
                    setIsEditingFile(false)
                    setCreateFileDialogOpen(true)
                  }}
                >
                  <ListItemIcon>
                    <Plus size={16} />
                  </ListItemIcon>
                  <ListItemText>New File</ListItemText>
                </MenuItem>

                {isExternal && (
                  <MenuItem
                    onClick={() => {
                      handleMenuClose()
                      handlePushPull()
                    }}
                    disabled={pushPullMutation.isPending}
                  >
                    <ListItemIcon>
                      <ArrowUpDown size={16} />
                    </ListItemIcon>
                    <ListItemText>
                      {pushPullMutation.isPending ? 'Syncing...' : 'Sync'}
                    </ListItemText>
                  </MenuItem>
                )}
              </Menu>
            </Box>

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

              {/* If repository is external, show the external URL */}
              {repository.is_external && repository.external_url && (
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                    Upstream URL
                  </Typography>
                  <Typography variant="body2">
                    <a href={repository.external_url} target="_blank" rel="noopener noreferrer">
                      {repository.external_url}
                    </a>
                  </Typography>
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

      {/* Create PR Dialog */}
      <Dialog open={createPRDialogOpen} onClose={() => setCreatePRDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create Pull Request</DialogTitle>
        <DialogContent>
          {existingPR ? (
            <Box>
              <Alert severity="info" sx={{ mb: 2 }}>
                A pull request already exists for this branch.
              </Alert>
              <Typography variant="body2">
                You can view the existing pull request here:
              </Typography>
              <Box sx={{ mt: 2, p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                {existingPR.url ? (
                    <a href={existingPR.url} target="_blank" rel="noopener noreferrer" style={{ textDecoration: 'none', color: '#3b82f6', display: 'flex', alignItems: 'center', gap: 8 }}>
                       <GitPullRequest size={16} /> #{existingPR.number} {existingPR.title} <ExternalLink size={14} />
                    </a>
                ) : (
                    <Typography variant="body2">
                        #{existingPR.number} {existingPR.title} (No URL available)
                    </Typography>
                )}
              </Box>
            </Box>
          ) : (
            <Box>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Are you sure you want to create a pull request for branch <strong>{currentBranch}</strong>?
              </Typography>
              <Typography variant="body2" color="text.secondary">
                This will create a pull request to merge changes from <strong>{currentBranch}</strong> into <strong>{repository?.default_branch || 'main'}</strong>.
              </Typography>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          {!existingPR && (
            <Button
              onClick={handleCreatePR}
              color="secondary"
              variant="contained"
              disabled={createPullRequestMutation.isPending}
              sx={{mr: 1, mb: 1}}
            >
              {createPullRequestMutation.isPending ? <CircularProgress size={20} /> : 'Create Pull Request'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default CodeTab

