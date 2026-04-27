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
  Tooltip,
  Card,
  CardContent,
  Grid,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Button,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  LinearProgress,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'

import useSnackbar from '../../hooks/useSnackbar'
import {
  useAdminKoditQueue,
  useAdminDeleteKoditTask,
  useAdminUpdateKoditTaskPriority,
  KoditAdminQueueTask,
} from '../../services/koditAdminService'

const PRIORITY_PRESETS: { label: string; value: number }[] = [
  { label: 'Background (1000)', value: 1000 },
  { label: 'Normal (2000)', value: 2000 },
  { label: 'User Initiated (5000)', value: 5000 },
  { label: 'Critical (10000)', value: 10000 },
]

const priorityLabel = (priority: number): string => {
  if (priority >= 10000) return 'Critical'
  if (priority >= 5000) return 'User Initiated'
  if (priority >= 2000) return 'Normal'
  return 'Background'
}

const priorityColor = (priority: number): 'error' | 'warning' | 'info' | 'default' => {
  if (priority >= 10000) return 'error'
  if (priority >= 5000) return 'warning'
  if (priority >= 2000) return 'info'
  return 'default'
}

const formatOperation = (op: string): string => {
  const parts = op.split('.')
  const last = parts[parts.length - 1] || op
  return last.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

const formatDate = (dateStr: string | undefined): string => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

const timeAgo = (dateStr: string | undefined): string => {
  if (!dateStr) return ''
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diffMs = now - then
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  return `${diffDay}d ago`
}

const KoditAdminQueue: FC = () => {
  const snackbar = useSnackbar()
  const [page, setPage] = useState(0)
  const [rowsPerPage, setRowsPerPage] = useState(25)
  const [deleteTarget, setDeleteTarget] = useState<KoditAdminQueueTask | null>(null)
  const [priorityTarget, setPriorityTarget] = useState<KoditAdminQueueTask | null>(null)
  const [newPriority, setNewPriority] = useState<number>(2000)

  const { data, isLoading, error } = useAdminKoditQueue(page + 1, rowsPerPage)
  const deleteMutation = useAdminDeleteKoditTask()
  const updatePriorityMutation = useAdminUpdateKoditTaskPriority()

  const activeTasks = data?.active_tasks || []
  const tasks = data?.data || []
  const total = data?.meta?.total || 0
  const stats = data?.stats

  const handleChangePage = useCallback((_: unknown, newPage: number) => {
    setPage(newPage)
  }, [])

  const handleChangeRowsPerPage = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10))
    setPage(0)
  }, [])

  const handleDelete = useCallback(() => {
    if (!deleteTarget) return
    deleteMutation.mutate(deleteTarget.id, {
      onSuccess: () => {
        snackbar.success('Task deleted')
        setDeleteTarget(null)
      },
      onError: (err) => {
        snackbar.error(`Delete failed: ${err.message}`)
        setDeleteTarget(null)
      },
    })
  }, [deleteTarget, deleteMutation, snackbar])

  const handleUpdatePriority = useCallback(() => {
    if (!priorityTarget) return
    updatePriorityMutation.mutate(
      { taskId: priorityTarget.id, priority: newPriority },
      {
        onSuccess: () => {
          snackbar.success('Priority updated')
          setPriorityTarget(null)
        },
        onError: (err) => {
          snackbar.error(`Update failed: ${err.message}`)
          setPriorityTarget(null)
        },
      },
    )
  }, [priorityTarget, newPriority, updatePriorityMutation, snackbar])

  const openPriorityDialog = useCallback((task: KoditAdminQueueTask) => {
    setPriorityTarget(task)
    setNewPriority(task.priority)
  }, [])

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
          Failed to load queue: {(error as Error).message}
        </Typography>
      </Box>
    )
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Typography variant="h6">Kodit Queue</Typography>
      </Box>

      {stats && (
        <Grid container spacing={2} sx={{ mb: 3 }}>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="h5" fontWeight={600}>{stats.total}</Typography>
                <Typography variant="caption" color="text.secondary">Pending Tasks</Typography>
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="h5" fontWeight={600}>{stats.oldest_task_age || '-'}</Typography>
                <Typography variant="caption" color="text.secondary">Oldest Task Wait</Typography>
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="h5" fontWeight={600}>
                  {stats.by_operation ? Object.keys(stats.by_operation).length : 0}
                </Typography>
                <Typography variant="caption" color="text.secondary">Operation Types</Typography>
                {stats.by_operation && Object.keys(stats.by_operation).length > 0 && (
                  <Box sx={{ mt: 0.5 }}>
                    {Object.entries(stats.by_operation).map(([op, count]) => (
                      <Typography key={op} variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.65rem' }}>
                        {op.replace(/_/g, ' ')}: {count}
                      </Typography>
                    ))}
                  </Box>
                )}
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="h5" fontWeight={600}>
                  {stats.by_priority_level
                    ? (stats.by_priority_level.critical || 0) + (stats.by_priority_level.user_initiated || 0)
                    : 0}
                </Typography>
                <Typography variant="caption" color="text.secondary">High Priority</Typography>
                {stats.by_priority_level && (
                  <Box sx={{ mt: 0.5 }}>
                    {Object.entries(stats.by_priority_level).map(([level, count]) => (
                      <Typography key={level} variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.65rem' }}>
                        {level.replace(/_/g, ' ')}: {count}
                      </Typography>
                    ))}
                  </Box>
                )}
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>Operation</TableCell>
              <TableCell>Repository</TableCell>
              <TableCell>Priority</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {activeTasks.map((active, idx) => (
              <TableRow key={`active-${idx}`} sx={{ bgcolor: 'action.hover', opacity: 0.85 }}>
                <TableCell>
                  <Chip label="In Progress" size="small" color="info" variant="outlined" />
                </TableCell>
                <TableCell>
                  <Tooltip title={active.operation}>
                    <Typography variant="body2" fontWeight={500}>
                      {formatOperation(active.operation)}
                    </Typography>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <Typography variant="body2">
                    {active.repo_name || (active.repository_id ? `#${active.repository_id}` : '-')}
                  </Typography>
                </TableCell>
                <TableCell colSpan={2}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <LinearProgress
                      variant={active.total > 0 ? 'determinate' : 'indeterminate'}
                      value={active.total > 0 ? (active.current / active.total) * 100 : undefined}
                      sx={{ flexGrow: 1, height: 6, borderRadius: 3 }}
                    />
                    {active.total > 0 && (
                      <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                        {active.current}/{active.total}
                      </Typography>
                    )}
                  </Box>
                  {active.message && (
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
                      {active.message}
                    </Typography>
                  )}
                </TableCell>
                <TableCell />
              </TableRow>
            ))}
            {activeTasks.length === 0 && tasks.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    Queue is empty
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {tasks.map((task) => (
              <TableRow key={task.id} hover>
                <TableCell>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                    {task.id}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Tooltip title={task.operation}>
                    <Typography variant="body2" fontWeight={500}>
                      {formatOperation(task.operation)}
                    </Typography>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <Typography variant="body2">
                    {task.repo_name || (task.repository_id ? `#${task.repository_id}` : '-')}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Chip
                    label={`${priorityLabel(task.priority)} (${task.priority})`}
                    size="small"
                    color={priorityColor(task.priority)}
                  />
                </TableCell>
                <TableCell>
                  <Tooltip title={formatDate(task.created_at)}>
                    <Typography variant="body2" color="text.secondary">
                      {timeAgo(task.created_at)}
                    </Typography>
                  </Tooltip>
                </TableCell>
                <TableCell align="right">
                  <Tooltip title="Increase priority">
                    <IconButton
                      size="small"
                      onClick={() => openPriorityDialog(task)}
                    >
                      <ArrowUpwardIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Delete task">
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => setDeleteTarget(task)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      <TablePagination
        component="div"
        count={total}
        page={page}
        onPageChange={handleChangePage}
        rowsPerPage={rowsPerPage}
        onRowsPerPageChange={handleChangeRowsPerPage}
        rowsPerPageOptions={[10, 25, 50, 100]}
      />

      {/* Delete confirmation dialog */}
      <Dialog open={!!deleteTarget} onClose={() => setDeleteTarget(null)}>
        <DialogTitle>Delete task?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Remove task <strong>#{deleteTarget?.id}</strong> ({formatOperation(deleteTarget?.operation || '')}) from the queue?
            This cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)} disabled={deleteMutation.isPending}>
            Cancel
          </Button>
          <Button
            onClick={handleDelete}
            color="error"
            variant="contained"
            disabled={deleteMutation.isPending}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>

      {/* Priority update dialog */}
      <Dialog open={!!priorityTarget} onClose={() => setPriorityTarget(null)}>
        <DialogTitle>Update priority</DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ mb: 2 }}>
            Change priority for task <strong>#{priorityTarget?.id}</strong> ({formatOperation(priorityTarget?.operation || '')}).
            Higher values are processed first.
          </DialogContentText>
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel>Preset</InputLabel>
            <Select
              value={PRIORITY_PRESETS.find(p => p.value === newPriority) ? newPriority : ''}
              label="Preset"
              onChange={(e) => {
                if (e.target.value !== '') setNewPriority(Number(e.target.value))
              }}
            >
              {PRIORITY_PRESETS.map((p) => (
                <MenuItem key={p.value} value={p.value}>{p.label}</MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            label="Custom priority"
            type="text"
            fullWidth
            value={newPriority}
            onChange={(e) => {
              const val = parseInt(e.target.value, 10)
              if (!isNaN(val) && val >= 0) setNewPriority(val)
            }}
            helperText="Background=1000, Normal=2000, User Initiated=5000, Critical=10000"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPriorityTarget(null)} disabled={updatePriorityMutation.isPending}>
            Cancel
          </Button>
          <Button
            onClick={handleUpdatePriority}
            variant="contained"
            disabled={updatePriorityMutation.isPending}
          >
            Update
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default KoditAdminQueue
