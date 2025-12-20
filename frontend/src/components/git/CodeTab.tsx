import React, { FC, useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
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
  ArrowUp,
  ArrowDown,
  Brain,
  Database,
  ArrowRight,
} from 'lucide-react'
import {
  koditEnrichmentDetailQueryKey,
  KODIT_SUBTYPE_PHYSICAL,
  KODIT_SUBTYPE_ARCHITECTURE,
  KODIT_SUBTYPE_DATABASE_SCHEMA,
} from '../../services/koditService'
import MermaidDiagram, { extractMermaidDiagrams, hasMermaidDiagram } from '../widgets/MermaidDiagram'
import useRouter from '../../hooks/useRouter'
import useApi from '../../hooks/useApi'
import {
  useCreateGitRepositoryPullRequest,
  useListRepositoryPullRequests,
  usePushToRemote,
  usePullFromRemote,
} from '../../services/gitRepositoryService'
import useSnackbar from '../../hooks/useSnackbar'
import BranchSelect from './BranchSelect'
import ExternalStatus from './ExternalStatus'

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
  searchText?: string
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
  getPathBreadcrumbs,
  setCreateBranchDialogOpen,
  setNewBranchName,
  setNewBranchBase,
  setCreateFileDialogOpen,
  setNewFilePath,
  setNewFileContent,
  setIsEditingFile,
  searchText,
}) => {
  const fileContentRef = React.useRef<HTMLPreElement>(null)
  const highlightRef = React.useRef<HTMLSpanElement>(null)

  // Scroll to highlighted text when file loads with search text
  React.useEffect(() => {
    if (highlightRef.current && searchText && fileData?.content) {
      // Small delay to ensure DOM is updated
      setTimeout(() => {
        highlightRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }, 100)
    }
  }, [fileData?.content, searchText])

  const router = useRouter()
  const api = useApi()
  const apiClient = api.getApiClient()
  const createPullRequestMutation = useCreateGitRepositoryPullRequest()
  const { data: pullRequests = [] } = useListRepositoryPullRequests(repository?.id || '')
  const pushToRemoteMutation = usePushToRemote()
  const pullFromRemoteMutation = usePullFromRemote()
  const snackbar = useSnackbar()
  const fallbackBranch = getFallbackBranch(repository?.default_branch, branches)
  const repoId = repository?.id || ''

  // Find enrichments that might contain Mermaid diagrams (physical, architecture, database_schema)
  // Prioritize physical first
  const diagramEnrichmentIds = useMemo(() => {
    const DIAGRAM_SUBTYPES = [KODIT_SUBTYPE_PHYSICAL, KODIT_SUBTYPE_ARCHITECTURE, KODIT_SUBTYPE_DATABASE_SCHEMA]

    // Sort by priority (physical first)
    const sorted = [...(enrichments || [])].sort((a, b) => {
      const aSubtype = a?.attributes?.subtype || ''
      const bSubtype = b?.attributes?.subtype || ''
      const aIndex = DIAGRAM_SUBTYPES.indexOf(aSubtype)
      const bIndex = DIAGRAM_SUBTYPES.indexOf(bSubtype)
      if (aIndex >= 0 && bIndex >= 0) return aIndex - bIndex
      if (aIndex >= 0) return -1
      if (bIndex >= 0) return 1
      return 0
    })

    return sorted
      .filter(e => {
        const subtype = e?.attributes?.subtype
        return DIAGRAM_SUBTYPES.includes(subtype)
      })
      .map(e => e.id)
      .filter(Boolean)
      .slice(0, 1) // Only fetch the first one for the CTA preview
  }, [enrichments])

  // Fetch full details for the first diagram enrichment
  const diagramEnrichmentQueries = useQueries({
    queries: diagramEnrichmentIds.map(enrichmentId => ({
      queryKey: koditEnrichmentDetailQueryKey(repoId, enrichmentId),
      queryFn: async () => {
        const response = await apiClient.v1GitRepositoriesEnrichmentsDetail2(repoId, enrichmentId)
        return response.data
      },
      enabled: !!repoId && !!enrichmentId && repository?.kodit_indexing,
      staleTime: 5 * 60 * 1000,
    })),
  })

  // Extract first Mermaid diagram for preview CTA from full content
  const firstMermaidDiagram = useMemo(() => {
    for (const query of diagramEnrichmentQueries) {
      if (query.data?.attributes) {
        const content = query.data?.attributes?.content || ''
        if (hasMermaidDiagram(content)) {
          const diagrams = extractMermaidDiagrams(content)
          if (diagrams.length > 0) {
            const isERD = diagrams[0].toLowerCase().includes('erdiagram')
            return {
              diagram: diagrams[0],
              type: isERD ? 'erd' : 'graph',
            }
          }
        }
      }
    }
    return null
  }, [diagramEnrichmentQueries])

  // Check if diagrams are still loading
  const diagramsLoading = diagramEnrichmentQueries.some(q => q.isLoading) && diagramEnrichmentIds.length > 0

  const handleNavigateToCodeIntelligence = () => {
    router.mergeParams({ tab: 'code-intelligence' })
  }

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
          target_branch: fallbackBranch,
        }
      })
      setCreatePRDialogOpen(false)
      snackbar.success('Pull request created successfully')
    } catch (error) {
      console.error('Failed to create PR:', error)
      snackbar.error('Failed to create pull request')
    }
  }

  const handlePushToRemote = async () => {
    if (!repository?.id || !currentBranch) return

    try {
      await pushToRemoteMutation.mutateAsync({
        repositoryId: repository.id,
        branch: currentBranch,
      })
      snackbar.success('Successfully pushed to remote')
    } catch (error) {
      console.error('Failed to push:', error)
      snackbar.error('Failed to push to remote')
    }
  }

  const handlePullFromRemote = async () => {
    if (!repository?.id || !currentBranch) return

    try {
      await pullFromRemoteMutation.mutateAsync({
        repositoryId: repository.id,
        branch: currentBranch,
        force: false,
      })
      snackbar.success('Successfully pulled from remote')
    } catch (error) {
      console.error('Failed to pull:', error)
      snackbar.error('Failed to pull from remote')
    }
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
      {enrichments?.length > 0 && groupedEnrichments?.['architecture'] && !firstMermaidDiagram && (
        <Paper variant="outlined" sx={{ borderRadius: 2, p: 3, bgcolor: 'rgba(0, 213, 255, 0.04)', borderColor: 'rgba(0, 213, 255, 0.2)' }}>
          <Typography variant="h6" sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2, fontWeight: 600 }}>
            {getEnrichmentTypeIcon('architecture')} {getEnrichmentTypeName('architecture')} Insights
          </Typography>
          <Stack spacing={2}>
            {groupedEnrichments['architecture']?.map((enrichment: any, index: number) => (
              <Box key={enrichment?.id || index}>
                {enrichment?.attributes?.subtype && (
                  <Chip
                    label={enrichment.attributes.subtype}
                    size="small"
                    sx={{ mb: 1, bgcolor: 'rgba(0, 213, 255, 0.15)', color: '#00d5ff', fontWeight: 600 }}
                  />
                )}
                <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', lineHeight: 1.7 }}>
                  {enrichment?.attributes?.content}
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
              
              <ExternalStatus
                repositoryId={repository?.id || ''}
                branch={currentBranch}
                isExternal={isExternal}
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
                    currentBranch === fallbackBranch ||
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
                    setNewBranchBase(currentBranch || fallbackBranch)
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
                  <>
                    <MenuItem
                      onClick={() => {
                        handleMenuClose()
                        handlePushToRemote()
                      }}
                      disabled={pushToRemoteMutation.isPending}
                    >
                      <ListItemIcon>
                        <ArrowUp size={16} />
                      </ListItemIcon>
                      <ListItemText>
                        {pushToRemoteMutation.isPending ? 'Pushing...' : 'Push'}
                      </ListItemText>
                    </MenuItem>

                    <MenuItem
                      onClick={() => {
                        handleMenuClose()
                        handlePullFromRemote()
                      }}
                      disabled={pullFromRemoteMutation.isPending}
                    >
                      <ListItemIcon>
                        <ArrowDown size={16} />
                      </ListItemIcon>
                      <ListItemText>
                        {pullFromRemoteMutation.isPending ? 'Pulling...' : 'Pull'}
                      </ListItemText>
                    </MenuItem>

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
                  </>
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
                        if (a?.is_dir !== b?.is_dir) return a?.is_dir ? -1 : 1
                        return (a?.name || '').localeCompare(b?.name || '')
                      })
                      .map((entry) => (
                        <Box
                          key={entry?.path}
                          sx={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 2,
                            px: 3,
                            py: 1.5,
                            cursor: 'pointer',
                            borderBottom: 1,
                            borderColor: 'divider',
                            backgroundColor: selectedFile === entry?.path ? 'rgba(25, 118, 210, 0.08)' : 'transparent',
                            '&:hover': {
                              backgroundColor: selectedFile === entry?.path
                                ? 'rgba(25, 118, 210, 0.12)'
                                : 'rgba(0, 0, 0, 0.02)',
                            },
                            '&:last-child': {
                              borderBottom: 0,
                            },
                          }}
                          onClick={() => handleSelectFile(entry?.path || '', entry?.is_dir || false)}
                        >
                          {entry?.is_dir ? (
                            <Folder size={18} color="#54aeff" />
                          ) : (
                            <FileText size={18} color="#656d76" />
                          )}
                          <Typography variant="body2" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', fontWeight: 500 }}>
                            {entry?.name}
                          </Typography>
                          {!entry?.is_dir && entry?.size !== undefined && (
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
                  {selectedFile?.split('/').pop() || ''}
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
                  ref={fileContentRef}
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
                  {(() => {
                    const content = fileData?.content || 'No content'
                    if (!searchText || !content.includes(searchText)) {
                      return content
                    }
                    // Split content and highlight the first match
                    const index = content.indexOf(searchText)
                    const before = content.slice(0, index)
                    const match = content.slice(index, index + searchText.length)
                    const after = content.slice(index + searchText.length)
                    return (
                      <>
                        {before}
                        <span
                          ref={highlightRef}
                          style={{
                            backgroundColor: 'rgba(255, 213, 0, 0.4)',
                            borderRadius: '2px',
                            padding: '1px 2px',
                          }}
                        >
                          {match}
                        </span>
                        {after}
                      </>
                    )
                  })()}
                </Box>
              )}
            </Paper>
          )}
        </Box>

        <Box sx={{ width: 300, flexShrink: 0 }}>
          {/* Architecture Diagram Preview */}
          {diagramsLoading && (
            <Paper variant="outlined" sx={{ p: 3, borderRadius: 2, mb: 2, bgcolor: 'rgba(0, 213, 255, 0.04)', borderColor: 'rgba(0, 213, 255, 0.2)' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <Brain size={18} color="#00d5ff" />
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: '#00d5ff' }}>
                  Loading Diagram...
                </Typography>
              </Box>
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                <CircularProgress size={24} sx={{ color: '#00d5ff' }} />
              </Box>
            </Paper>
          )}
          {firstMermaidDiagram && !diagramsLoading && (
            <Paper
              variant="outlined"
              sx={{
                p: 2,
                borderRadius: 2,
                mb: 2,
                bgcolor: 'rgba(0, 213, 255, 0.04)',
                borderColor: 'rgba(0, 213, 255, 0.2)',
                cursor: 'pointer',
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                  borderColor: 'rgba(0, 213, 255, 0.5)',
                  boxShadow: '0 4px 20px rgba(0, 213, 255, 0.15)',
                },
              }}
              onClick={handleNavigateToCodeIntelligence}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 1.5 }}>
                {firstMermaidDiagram.type === 'erd' ? (
                  <Database size={18} color="#00d5ff" />
                ) : (
                  <Brain size={18} color="#00d5ff" />
                )}
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: '#00d5ff' }}>
                  {firstMermaidDiagram.type === 'erd' ? 'Database Schema' : 'Architecture'}
                </Typography>
                <ArrowRight size={14} color="#00d5ff" style={{ marginLeft: 'auto' }} />
              </Box>
              <Box sx={{ minHeight: 180, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <MermaidDiagram code={firstMermaidDiagram.diagram} compact enableFullscreen={false} onClick={handleNavigateToCodeIntelligence} />
              </Box>
            </Paper>
          )}

          <Paper variant="outlined" sx={{ p: 3, borderRadius: 2 }}>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600, fontSize: '1rem' }}>
              About
            </Typography>

            <Stack spacing={2}>
              {repository?.description && (
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
                  {repository?.repo_type || 'project'}
                </Typography>
              </Box>

              {repository?.default_branch && (
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
              {repository?.is_external && repository?.external_url && (
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                    Upstream URL
                  </Typography>
                  <Typography variant="body2">
                    <a
                      href={repository.external_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: '#00d5ff', textDecoration: 'none' }}
                    >
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
                  {repository?.created_at ? new Date(repository.created_at).toLocaleDateString() : 'N/A'}
                </Typography>
              </Box>

              {repository?.updated_at && (
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
                {existingPR?.url ? (
                    <a href={existingPR.url} target="_blank" rel="noopener noreferrer" style={{ textDecoration: 'none', color: '#00d5ff', display: 'flex', alignItems: 'center', gap: 8 }}>
                       <GitPullRequest size={16} /> #{existingPR?.number} {existingPR?.title} <ExternalLink size={14} />
                    </a>
                ) : (
                    <Typography variant="body2">
                        #{existingPR?.number} {existingPR?.title} (No URL available)
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
                This will create a pull request to merge changes from <strong>{currentBranch}</strong> into <strong>{fallbackBranch}</strong>.
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

