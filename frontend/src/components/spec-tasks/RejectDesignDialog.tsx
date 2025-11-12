import React from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  TextField,
  Button,
  Box,
  Alert,
} from '@mui/material'

interface RejectDesignDialogProps {
  open: boolean
  onClose: () => void
  reason: string
  onReasonChange: (value: string) => void
  onReject: () => void
}

export default function RejectDesignDialog({
  open,
  onClose,
  reason,
  onReasonChange,
  onReject,
}: RejectDesignDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth sx={{ zIndex: 200000 }}>
      <DialogTitle>Reject Design?</DialogTitle>
      <DialogContent>
        <Alert severity="warning" sx={{ mb: 2 }}>
          This will archive the spec task and prevent it from being implemented.
        </Alert>
        <TextField
          fullWidth
          multiline
          rows={3}
          label="Reason for rejection (optional)"
          value={reason}
          onChange={e => onReasonChange(e.target.value)}
          placeholder="Explain why this design is being rejected..."
        />
      </DialogContent>
      <Box p={2} display="flex" gap={2} justifyContent="flex-end">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          color="error"
          onClick={onReject}
        >
          Reject Design
        </Button>
      </Box>
    </Dialog>
  )
}
