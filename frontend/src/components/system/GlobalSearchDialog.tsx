import React, { FC } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  Box,
} from '@mui/material'

interface GlobalSearchDialogProps {
  open: boolean
  onClose: () => void
  organizationId: string
}

const GlobalSearchDialog: FC<GlobalSearchDialogProps> = ({
  open,
  onClose,
  organizationId,
}) => {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Search</DialogTitle>
      <DialogContent>
        <Box sx={{ minHeight: 200 }}>
          {/* Placeholder - implementation to follow */}
        </Box>
      </DialogContent>
    </Dialog>
  )
}

export default GlobalSearchDialog
