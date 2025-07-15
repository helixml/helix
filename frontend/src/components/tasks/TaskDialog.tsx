import React, { FC } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'

interface TaskDialogProps {
  open: boolean
  onClose: () => void
  task?: any // Will be properly typed when we implement the full dialog
}

const TaskDialog: FC<TaskDialogProps> = ({ open, onClose, task }) => {
  return (
    <Dialog 
      open={open} 
      onClose={onClose}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle>
        {task ? 'Edit Task' : 'Create New Task'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ p: 2 }}>
          {/* TODO: Implement task form */}
          <Box sx={{ textAlign: 'center', py: 4 }}>
            Task form will be implemented here
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>
          Cancel
        </Button>
        <Button variant="contained" onClick={onClose}>
          {task ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default TaskDialog 