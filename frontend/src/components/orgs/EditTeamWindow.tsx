import React, { FC, useState, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'

import { TypesTeam } from '../../api/api'

export interface EditTeamWindowProps {
  open: boolean
  team?: TypesTeam
  onClose: () => void
  onSubmit: (team: TypesTeam) => Promise<void>
}

const EditTeamWindow: FC<EditTeamWindowProps> = ({
  open,
  team,
  onClose,
  onSubmit,
}) => {
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [errors, setErrors] = useState<{name?: string}>({})

  // Reset state when modal opens/closes or team changes
  useEffect(() => {
    if (team) {
      setName(team.name || '')
    } else {
      setName('')
    }
    setErrors({})
  }, [team, open])

  // Validate form before submission
  const validateForm = () => {
    const newErrors: {name?: string} = {}
    
    // Validate name (required)
    if (!name) {
      newErrors.name = 'Name is required'
    }
    
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async () => {
    // Validate form before submission
    if (!validateForm()) {
      return
    }
    
    try {
      setLoading(true)
      
      // Create the updated team object
      // If editing existing team, merge with original data
      // If creating new team, create minimal object
      const updatedTeam = team 
        ? {
            ...team,        // Preserve all existing fields
            name: name      // Update the name
          } 
        : {
            name: name      // Just set the name for new teams
          } as TypesTeam;
      
      await onSubmit(updatedTeam)
      onClose()
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        {team ? 'Edit Team' : 'Create Team'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          {/* Team name field */}
          <TextField
            label="Team Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={loading}
            required
            error={!!errors.name}
            helperText={errors.name || "Name for the team"}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={onClose}
          disabled={loading}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={loading}
        >
          {team ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default EditTeamWindow 