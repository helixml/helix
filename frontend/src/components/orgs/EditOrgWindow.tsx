import React, { FC, useState, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'

import { TypesOrganization } from '../../api/api'

export interface EditOrgWindowProps {
  open: boolean
  org?: TypesOrganization
  onClose: () => void
  onSubmit: (org: TypesOrganization) => Promise<void>
}

const EditOrgWindow: FC<EditOrgWindowProps> = ({
  open,
  org,
  onClose,
  onSubmit,
}) => {
  const [name, setName] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (org) {
      setName(org.name || '')
      setDisplayName(org.display_name || '')
    } else {
      setName('')
      setDisplayName('')
    }
  }, [org])

  const handleSubmit = async () => {
    try {
      setLoading(true)
      await onSubmit({
        id: org?.id,
        name,
        display_name: displayName,
      } as TypesOrganization)
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
        {org ? 'Edit Organization' : 'Create Organization'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <TextField
            label="Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={loading}
            required
            helperText="Unique identifier for the organization"
            sx={{ mb: 2 }}
          />
          <TextField
            label="Display Name"
            fullWidth
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            disabled={loading}
            helperText="Human-readable name for the organization"
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
          disabled={loading || !name}
        >
          {org ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default EditOrgWindow 