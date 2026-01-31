import React, { FC, useMemo, useCallback, useState } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Typography,
  TextField,
  InputAdornment,
  Pagination,
  Chip,
  Menu,
  MenuItem,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Alert,
  CircularProgress,
} from '@mui/material'
import WarningIcon from '@mui/icons-material/Warning'
import SearchIcon from '@mui/icons-material/Search'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import { GitBranch, Link as LinkIcon, Brain, RefreshCw, Trash, Plus, FolderSearch } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import SimpleTable from '../widgets/SimpleTable'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useTheme from '@mui/material/styles/useTheme'
import { useDeleteGitRepository, QUERY_KEYS } from '../../services/gitRepositoryService'
import useSnackbar from '../../hooks/useSnackbar'

import type { TypesGitRepository } from '../../api/api'

interface RepositoriesListViewProps {
  repositories: TypesGitRepository[]
  ownerSlug: string
  searchQuery: string
  onSearchChange: (query: string) => void
  page: number
  onPageChange: (page: number) => void
  filteredRepositories: TypesGitRepository[]
  paginatedRepositories: TypesGitRepository[]
  totalPages: number
  onViewRepository: (repo: TypesGitRepository) => void
  // Optional callbacks for creating repos from within this view
  onCreateRepo?: () => void
  onLinkExternalRepo?: () => void
  onBrowseProviders?: () => void
}

const RepositoriesListView: FC<RepositoriesListViewProps> = ({
  repositories,
  ownerSlug,
  searchQuery,
  onSearchChange,
  page,
  onPageChange,
  filteredRepositories,
  paginatedRepositories,
  totalPages,
  onViewRepository,
  onCreateRepo,
  onLinkExternalRepo,
  onBrowseProviders,
}) => {
  const theme = useTheme()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()
  const deleteRepositoryMutation = useDeleteGitRepository()

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentRepo, setCurrentRepo] = useState<TypesGitRepository | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [repoToDelete, setRepoToDelete] = useState<TypesGitRepository | null>(null)
  const [deleteConfirmName, setDeleteConfirmName] = useState('')

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, repo: any) => {
    setAnchorEl(event.currentTarget)
    setCurrentRepo(repo._data as TypesGitRepository)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentRepo(null)
  }

  const handleRefresh = async () => {
    if (!currentRepo?.id) return

    try {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepositories })
      await queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepository(currentRepo.id) })
      snackbar.success('Repository refreshed')
    } catch (error) {
      console.error('Failed to refresh repository:', error)
      snackbar.error('Failed to refresh repository')
    }
    handleMenuClose()
  }

  const handleDelete = () => {
    if (!currentRepo) return
    setRepoToDelete(currentRepo)
    setDeleteDialogOpen(true)
    handleMenuClose()
  }

  const handleCloseDeleteDialog = () => {
    setDeleteDialogOpen(false)
    setRepoToDelete(null)
    setDeleteConfirmName('')
  }

  const handleConfirmDelete = async () => {
    if (!repoToDelete?.id) return

    try {
      await deleteRepositoryMutation.mutateAsync(repoToDelete.id)
      snackbar.success('Repository deleted successfully')
      handleCloseDeleteDialog()
    } catch (error) {
      console.error('Failed to delete repository:', error)
      snackbar.error('Failed to delete repository')
    }
  }

  const tableData = useMemo(() => {
    return paginatedRepositories.map((repo: TypesGitRepository) => {
      const updatedAt = repo.updated_at || repo.created_at
      const updatedTime = updatedAt
        ? new Date(updatedAt).toLocaleDateString('en-US', {
          month: 'short',
          day: 'numeric',
          year: 'numeric',
          hour: '2-digit',
          minute: '2-digit'
        })
        : 'Never'

      return {
        id: repo.id,
        _data: repo,
        name: (
          <Row>
            <Cell sx={{ pr: 2 }}>
              <GitBranch size={20} color={theme.palette.text.secondary} />
            </Cell>
            <Cell grow>
              <Typography variant="body1">
                <a
                  style={{
                    textDecoration: 'none',
                    fontWeight: 'bold',
                    color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
                  }}
                  href="#"
                  onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
                    e.preventDefault()
                    e.stopPropagation()
                    onViewRepository(repo)
                  }}
                >
                  {ownerSlug}/{repo.name || repo.id}
                </a>
              </Typography>
              {repo.description && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                  {repo.description}
                </Typography>
              )}
              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5, flexWrap: 'wrap' }}>
                {repo.metadata?.is_external && (
                  <Chip
                    icon={<LinkIcon size={12} />}
                    label={repo.metadata.external_type.toUpperCase() || 'External'}
                    size="small"
                    sx={{ height: 20, fontSize: '0.75rem' }}
                  />
                )}
                {repo.kodit_indexing && (
                  <Chip
                    icon={<Brain size={12} />}
                    label="Code Intelligence"
                    size="small"
                    color="success"
                    sx={{ height: 20, fontSize: '0.75rem' }}
                  />
                )}
              </Box>
            </Cell>
          </Row>
        ),
        updated: (
          <Typography variant="body2" color="text.secondary">
            {updatedTime}
          </Typography>
        ),
      }
    })
  }, [paginatedRepositories, ownerSlug, theme, onViewRepository])

  const getActions = useCallback((repo: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={(e) => {
          e.stopPropagation()
          handleMenuClick(e, repo)
        }}
      >
        <MoreVertIcon />
      </IconButton>
    )
  }, [])

  const tableFields = useMemo(() => [
    {
      name: 'name',
      title: 'Name',
    },
    {
      name: 'updated',
      title: 'Updated',
    },
  ], [])

  return (
    <>
      {/* GitHub-style header with owner/repositories */}
      <Box sx={{ mb: 3, pb: 2 }}>
        <Box sx={{ mb: 2 }}>
          <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
            <span style={{ color: theme.palette.secondary.main, cursor: 'pointer' }}>{ownerSlug}</span>
            <span style={{ color: 'text.secondary', fontWeight: 300 }}>/</span>
            <span style={{ fontWeight: 600 }}>repositories</span>
          </Typography>
        </Box>
      </Box>

      {filteredRepositories.length === 0 && searchQuery ? (
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
              onClick={() => onSearchChange('')}
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
              Connect your existing repositories or create new ones to start collaborating with AI agents.
            </Typography>
            <Box sx={{ display: 'flex', justifyContent: 'center', gap: 2 }}>
              {onBrowseProviders && (
                <Button
                  variant="contained"
                  color="secondary"
                  size="large"
                  startIcon={<FolderSearch size={18} />}
                  onClick={onBrowseProviders}
                >
                  Connect & Browse
                </Button>
              )}
              {onLinkExternalRepo && (
                <Button
                  variant="outlined"
                  size="large"
                  startIcon={<LinkIcon size={18} />}
                  onClick={onLinkExternalRepo}
                >
                  Link manually
                </Button>
              )}
              {onCreateRepo && (
                <Button
                  variant="outlined"
                  size="large"
                  startIcon={<Plus size={18} />}
                  onClick={onCreateRepo}
                >
                  New empty
                </Button>
              )}
            </Box>
          </CardContent>
        </Card>
      ) : (
        <>
          <SimpleTable
            authenticated={true}
            fields={tableFields}
            data={tableData}
            getActions={getActions}
          />

          {/* Pagination */}
          {totalPages > 1 && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
              <Pagination
                count={totalPages}
                page={page + 1}
                onChange={(_, newPage) => onPageChange(newPage - 1)}
                color="primary"
                showFirstButton
                showLastButton
              />
            </Box>
          )}

          {/* Actions Menu */}
          <Menu
            id="long-menu"
            anchorEl={anchorEl}
            open={Boolean(anchorEl)}
            onClose={handleMenuClose}
          >
            <MenuItem onClick={handleRefresh}>
              <RefreshCw size={16} style={{ marginRight: 5 }} />
              Refresh
            </MenuItem>
            <MenuItem onClick={handleDelete}>
              <Trash size={16} style={{ marginRight: 5 }} />
              Delete
            </MenuItem>
          </Menu>
        </>
      )}

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={handleCloseDeleteDialog}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <WarningIcon color="error" />
            <span>Delete Repository</span>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Alert severity="error" sx={{ mb: 3 }}>
            <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
              This action cannot be undone!
            </Typography>
            <Typography variant="body2">
              This will permanently delete the repository <strong>{repoToDelete?.name}</strong> and all its data from Helix.
              {repoToDelete?.metadata?.is_external && (
                <> The external repository will not be affected.</>
              )}
            </Typography>
          </Alert>

          <Typography variant="body2" sx={{ mb: 2 }}>
            Please type the repository name <strong>{repoToDelete?.name}</strong> to confirm:
          </Typography>

          <TextField
            fullWidth
            value={deleteConfirmName}
            onChange={(e) => setDeleteConfirmName(e.target.value)}
            placeholder={repoToDelete?.name}
            autoFocus
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDeleteDialog}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmDelete}
            variant="contained"
            color="error"
            disabled={deleteConfirmName !== repoToDelete?.name || deleteRepositoryMutation.isPending}
            startIcon={deleteRepositoryMutation.isPending ? <CircularProgress size={16} /> : <Trash size={16} />}
          >
            {deleteRepositoryMutation.isPending ? 'Deleting...' : 'Delete Repository'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default RepositoriesListView
