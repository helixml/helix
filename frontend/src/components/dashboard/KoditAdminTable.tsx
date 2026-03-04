import React, { FC, useState, useCallback } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
  Box,
  CircularProgress,
  Chip,
  TablePagination,
  IconButton,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Checkbox,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SyncIcon from '@mui/icons-material/Sync'
import RefreshIcon from '@mui/icons-material/Refresh'
import DeleteIcon from '@mui/icons-material/Delete'
import InfoIcon from '@mui/icons-material/Info'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useAdminKoditRepositories,
  useAdminSyncKoditRepository,
  useAdminRescanKoditRepository,
  useAdminDeleteKoditRepository,
  useAdminBatchDeleteKoditRepositories,
  useAdminBatchRescanKoditRepositories,
} from '../../services/koditAdminService'
import { ServerKoditAdminRepoDTO } from '../../api/api'

const statusColor: Record<string, 'success' | 'warning' | 'error' | 'default' | 'info'> = {
  cloned: 'success',
  syncing: 'info',
  cloning: 'info',
  pending: 'default',
  failed: 'error',
  deleting: 'warning',
}

const formatDate = (dateStr: string | undefined): string => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

const repoName = (repo: ServerKoditAdminRepoDTO): string => {
  const url = repo.attributes?.remote_url || ''
  if (repo.attributes?.helix_repo_name) return repo.attributes.helix_repo_name
  // Extract last path segment from URL
  const parts = url.replace(/\.git$/, '').split('/')
  return parts[parts.length - 1] || url
}

const KoditAdminTable: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const [page, setPage] = useState(0)
  const [rowsPerPage, setRowsPerPage] = useState(25)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null)
  const [menuRepoId, setMenuRepoId] = useState<string | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ ids: number[], label: string } | null>(null)

  const { data, isLoading, error } = useAdminKoditRepositories(page + 1, rowsPerPage)
  const syncMutation = useAdminSyncKoditRepository()
  const rescanMutation = useAdminRescanKoditRepository()
  const deleteMutation = useAdminDeleteKoditRepository()
  const batchDeleteMutation = useAdminBatchDeleteKoditRepositories()
  const batchRescanMutation = useAdminBatchRescanKoditRepositories()

  const repos = data?.data || []
  const total = data?.meta?.total || 0

  const handleChangePage = useCallback((_: unknown, newPage: number) => {
    setPage(newPage)
    setSelected(new Set())
  }, [])

  const handleChangeRowsPerPage = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10))
    setPage(0)
    setSelected(new Set())
  }, [])

  const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>, repoId: string) => {
    setMenuAnchor(event.currentTarget)
    setMenuRepoId(repoId)
  }, [])

  const handleMenuClose = useCallback(() => {
    setMenuAnchor(null)
    setMenuRepoId(null)
  }, [])

  const handleViewDetail = useCallback((repoId: string) => {
    handleMenuClose()
    router.setParams({ tab: 'kodit', repo_id: repoId })
  }, [router])

  const handleSync = useCallback((repoId: string) => {
    handleMenuClose()
    syncMutation.mutate(Number(repoId), {
      onSuccess: () => snackbar.success('Sync triggered'),
      onError: (err) => snackbar.error(`Sync failed: ${err.message}`),
    })
  }, [syncMutation, snackbar])

  const handleRescan = useCallback((repoId: string) => {
    handleMenuClose()
    rescanMutation.mutate(Number(repoId), {
      onSuccess: () => snackbar.success('Rescan triggered'),
      onError: (err) => snackbar.error(`Rescan failed: ${err.message}`),
    })
  }, [rescanMutation, snackbar])

  const handleDeleteConfirm = useCallback((ids: number[], label: string) => {
    handleMenuClose()
    setDeleteTarget({ ids, label })
    setDeleteDialogOpen(true)
  }, [])

  const handleDeleteExecute = useCallback(() => {
    if (!deleteTarget) return
    setDeleteDialogOpen(false)
    if (deleteTarget.ids.length === 1) {
      deleteMutation.mutate(deleteTarget.ids[0], {
        onSuccess: () => {
          snackbar.success('Delete queued')
          setSelected(new Set())
        },
        onError: (err) => snackbar.error(`Delete failed: ${err.message}`),
      })
    } else {
      batchDeleteMutation.mutate(deleteTarget.ids, {
        onSuccess: (resp) => {
          const succeeded = resp?.succeeded?.length || 0
          const failed = resp?.failed?.length || 0
          snackbar.success(`Deleted ${succeeded} repositories${failed > 0 ? `, ${failed} failed` : ''}`)
          setSelected(new Set())
        },
        onError: (err) => snackbar.error(`Batch delete failed: ${err.message}`),
      })
    }
    setDeleteTarget(null)
  }, [deleteTarget, deleteMutation, batchDeleteMutation, snackbar])

  const handleBatchRescan = useCallback(() => {
    const ids = Array.from(selected).map(Number)
    batchRescanMutation.mutate(ids, {
      onSuccess: (resp) => {
        const succeeded = resp?.succeeded?.length || 0
        const failed = resp?.failed?.length || 0
        snackbar.success(`Rescan triggered for ${succeeded} repositories${failed > 0 ? `, ${failed} failed` : ''}`)
        setSelected(new Set())
      },
      onError: (err) => snackbar.error(`Batch rescan failed: ${err.message}`),
    })
  }, [selected, batchRescanMutation, snackbar])

  const handleToggleSelect = useCallback((id: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  const handleToggleSelectAll = useCallback(() => {
    if (selected.size === repos.length) {
      setSelected(new Set())
    } else {
      setSelected(new Set(repos.map(r => r.id || '')))
    }
  }, [repos, selected.size])

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    return (
      <Box sx={{ py: 4, textAlign: 'center' }}>
        <Typography color="error">
          Failed to load Kodit repositories: {(error as Error).message}
        </Typography>
      </Box>
    )
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Typography variant="h6">Kodit Repositories</Typography>
        {selected.size > 0 && (
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              size="small"
              variant="outlined"
              startIcon={<RefreshIcon />}
              onClick={handleBatchRescan}
              disabled={batchRescanMutation.isPending}
            >
              Rescan ({selected.size})
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              startIcon={<DeleteIcon />}
              onClick={() => handleDeleteConfirm(
                Array.from(selected).map(Number),
                `${selected.size} repositories`
              )}
              disabled={batchDeleteMutation.isPending}
            >
              Delete ({selected.size})
            </Button>
          </Box>
        )}
      </Box>

      <TableContainer component={Paper} variant="outlined">
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell padding="checkbox">
                <Checkbox
                  indeterminate={selected.size > 0 && selected.size < repos.length}
                  checked={repos.length > 0 && selected.size === repos.length}
                  onChange={handleToggleSelectAll}
                />
              </TableCell>
              <TableCell>Name</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Helix Repository</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {repos.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    No Kodit repositories found
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {repos.map((repo) => {
              const id = repo.id || ''
              const isSelected = selected.has(id)
              return (
                <TableRow
                  key={id}
                  hover
                  selected={isSelected}
                  sx={{ cursor: 'pointer' }}
                  onClick={() => handleViewDetail(id)}
                >
                  <TableCell padding="checkbox" onClick={(e) => e.stopPropagation()}>
                    <Checkbox
                      checked={isSelected}
                      onChange={() => handleToggleSelect(id)}
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" fontWeight={500}>
                      {repoName(repo)}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                      {repo.attributes?.remote_url}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={repo.attributes?.status || 'unknown'}
                      size="small"
                      color={statusColor[repo.attributes?.status || ''] || 'default'}
                    />
                    {repo.attributes?.last_error && (
                      <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
                        {repo.attributes.last_error}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    {repo.attributes?.helix_repo_name || (
                      <Typography variant="caption" color="text.secondary">-</Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    {formatDate(repo.attributes?.created_at)}
                  </TableCell>
                  <TableCell align="right" onClick={(e) => e.stopPropagation()}>
                    <IconButton size="small" onClick={(e) => handleMenuOpen(e, id)}>
                      <MoreVertIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </TableContainer>

      <TablePagination
        rowsPerPageOptions={[10, 25, 50, 100]}
        component="div"
        count={total}
        rowsPerPage={rowsPerPage}
        page={page}
        onPageChange={handleChangePage}
        onRowsPerPageChange={handleChangeRowsPerPage}
      />

      <Menu
        anchorEl={menuAnchor}
        open={Boolean(menuAnchor)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => menuRepoId && handleViewDetail(menuRepoId)}>
          <ListItemIcon><InfoIcon fontSize="small" /></ListItemIcon>
          <ListItemText>View Details</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => menuRepoId && handleSync(menuRepoId)}>
          <ListItemIcon><SyncIcon fontSize="small" /></ListItemIcon>
          <ListItemText>Sync</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => menuRepoId && handleRescan(menuRepoId)}>
          <ListItemIcon><RefreshIcon fontSize="small" /></ListItemIcon>
          <ListItemText>Rescan HEAD</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => menuRepoId && handleDeleteConfirm([Number(menuRepoId)], 'this repository')}>
          <ListItemIcon><DeleteIcon fontSize="small" color="error" /></ListItemIcon>
          <ListItemText sx={{ color: 'error.main' }}>Delete</ListItemText>
        </MenuItem>
      </Menu>

      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Confirm Deletion</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to delete {deleteTarget?.label}? This action queues the
            repository for deletion and cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDeleteExecute} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default KoditAdminTable
