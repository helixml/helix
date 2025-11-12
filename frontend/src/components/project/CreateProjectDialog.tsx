import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
} from '@mui/material'

interface CreateProjectDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (name: string, description: string) => Promise<void>
  isCreating: boolean
}

const CreateProjectDialog: FC<CreateProjectDialogProps> = ({
  open,
  onClose,
  onSubmit,
  isCreating,
}) => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
    }
  }, [open])

  const handleSubmit = async () => {
    await onSubmit(name, description)
    setName('')
    setDescription('')
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create New Project</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label="Project Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <TextField
            label="Description"
            fullWidth
            multiline
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={isCreating}
        >
          {isCreating ? 'Creating...' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateProjectDialog
