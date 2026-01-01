import React from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Alert,
  CircularProgress,
} from '@mui/material'

interface ArchiveConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  taskName?: string
  isArchiving?: boolean
}

/**
 * Shared confirmation dialog for archiving/rejecting tasks.
 * Used by both KanbanBoard and TabsView to ensure consistent UX.
 */
const ArchiveConfirmDialog: React.FC<ArchiveConfirmDialogProps> = ({
  open,
  onClose,
  onConfirm,
  taskName,
  isArchiving = false,
}) => {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ fontWeight: 600, fontSize: '1.125rem' }}>
        Archive Task?
      </DialogTitle>
      <DialogContent>
        <Alert severity="warning" sx={{ mb: 2, border: '1px solid rgba(245, 158, 11, 0.2)' }}>
          <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
            Archiving this task will:
          </Typography>
          <ul style={{ marginTop: 0, marginBottom: 0, paddingLeft: 20 }}>
            <li><Typography variant="body2">Stop any running external agents</Typography></li>
            <li><Typography variant="body2">Lose any unsaved data in the desktop</Typography></li>
            <li><Typography variant="body2">Hide the task from the board</Typography></li>
          </ul>
        </Alert>
        <Typography variant="body2" color="text.secondary">
          The conversation history will be preserved and you can restore the task later.
        </Typography>
        {taskName && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            Task: <strong>{taskName}</strong>
          </Typography>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button
          onClick={onClose}
          disabled={isArchiving}
          sx={{
            textTransform: 'none',
            fontWeight: 500,
            color: 'text.secondary',
          }}
        >
          Cancel
        </Button>
        <Button
          onClick={onConfirm}
          variant="contained"
          color="warning"
          disabled={isArchiving}
          startIcon={isArchiving ? <CircularProgress size={16} color="inherit" /> : undefined}
        >
          {isArchiving ? 'Archiving...' : 'Archive Task'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ArchiveConfirmDialog
