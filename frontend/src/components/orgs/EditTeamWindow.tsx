import React, { FC, useState, useEffect, useRef } from 'react'
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
  const nameInputRef = useRef<HTMLInputElement>(null)

  // Focus the name input when the dialog opens
  useEffect(() => {
    if(open) {
      if(team) {
        setName(team.name || '')
      }
      setTimeout(() => {
        if(nameInputRef.current) {
          nameInputRef.current.focus()
        }
      }, 300)
    }
  }, [open]);

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

  // Handle Enter key press with optimistic update
  const handleEnterKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      
      // If we're editing an existing team, update its name optimistically
      if (team) {
        team.name = name;
      }
      
      handleSubmit();
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      // Add this to prevent Dialog from stealing focus
      disableAutoFocus
      disableEnforceFocus
    >
      <DialogTitle>
        {team ? 'Edit Team' : 'Create Team'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          {/* Team name field */}
          <TextField
            inputRef={nameInputRef}
            label="Team Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={loading}
            required
            error={!!errors.name}
            helperText={errors.name || "Name for the team"}
            onKeyDown={handleEnterKeyPress}
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