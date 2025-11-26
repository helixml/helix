import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Alert,
  Stack,
  FormControlLabel,
  Switch,
  Box,
  Typography,
  CircularProgress,
} from '@mui/material'
import { Brain } from 'lucide-react'

interface CreateRepositoryDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (name: string, description: string, koditIndexing: boolean) => Promise<void>
  isCreating: boolean
  error?: string
}

const CreateRepositoryDialog: FC<CreateRepositoryDialogProps> = ({
  open,
  onClose,
  onSubmit,
  isCreating,
  error,
}) => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [koditIndexing, setKoditIndexing] = useState(false)

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
      setKoditIndexing(false)
    }
  }, [open])

  const handleSubmit = async () => {
    await onSubmit(name, description, koditIndexing)
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create New Repository</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          {error && (
            <Alert severity="error">
              {error}
            </Alert>
          )}

          <TextField
            label="Repository Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            helperText="Enter a name for your repository"
            autoFocus
          />

          <TextField
            label="Description"
            fullWidth
            multiline
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            helperText="Describe the purpose of this repository"
          />

          <FormControlLabel
            disabled={true}
            control={
              <Switch
                checked={koditIndexing}
                onChange={(e) => setKoditIndexing(e.target.checked)}
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

          <Alert severity="info">
            {koditIndexing
              ? 'Code Intelligence enabled: Kodit will index this repository to provide code snippets and architectural summaries via MCP server.'
              : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
            }
          </Alert>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={handleSubmit}
          color="secondary"
          variant="contained"
          disabled={!name.trim() || isCreating}
        >
          {isCreating ? <CircularProgress size={20} /> : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateRepositoryDialog
