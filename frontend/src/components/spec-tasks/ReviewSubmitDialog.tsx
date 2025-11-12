import React from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  TextField,
  Button,
  Box,
} from '@mui/material'

interface ReviewSubmitDialogProps {
  open: boolean
  onClose: () => void
  decision: 'approve' | 'request_changes'
  overallComment: string
  onCommentChange: (value: string) => void
  onSubmit: () => void
  isSubmitting: boolean
}

export default function ReviewSubmitDialog({
  open,
  onClose,
  decision,
  overallComment,
  onCommentChange,
  onSubmit,
  isSubmitting,
}: ReviewSubmitDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth sx={{ zIndex: 200000 }}>
      <DialogTitle>
        {decision === 'approve' ? 'Approve Design' : 'Request Changes'}
      </DialogTitle>
      <DialogContent>
        <TextField
          fullWidth
          multiline
          rows={4}
          label="Overall Comment (optional)"
          value={overallComment}
          onChange={e => onCommentChange(e.target.value)}
          sx={{ mt: 2 }}
        />
      </DialogContent>
      <Box p={2} display="flex" gap={2} justifyContent="flex-end">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          color={decision === 'approve' ? 'success' : 'warning'}
          onClick={onSubmit}
          disabled={isSubmitting}
        >
          {decision === 'approve' ? 'Approve' : 'Submit Feedback'}
        </Button>
      </Box>
    </Dialog>
  )
}
