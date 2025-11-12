import React from 'react'
import { Paper, Box, TextField, Button, Typography } from '@mui/material'

interface InlineCommentFormProps {
  show: boolean
  yPos: number
  selectedText: string
  commentText: string
  onCommentChange: (value: string) => void
  onCreate: () => void
  onCancel: () => void
}

export default function InlineCommentForm({
  show,
  yPos,
  selectedText,
  commentText,
  onCommentChange,
  onCreate,
  onCancel,
}: InlineCommentFormProps) {
  if (!show || !selectedText) return null

  return (
    <Paper
      sx={{
        position: 'absolute',
        left: '670px',
        top: `${yPos}px`,
        width: '300px',
        p: 2,
        bgcolor: 'background.paper',
        border: '2px solid',
        borderColor: 'primary.main',
        boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
        zIndex: 20,
      }}
    >
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Add Comment
      </Typography>

      <Box
        sx={{
          bgcolor: 'action.hover',
          p: 1,
          borderLeft: '3px solid',
          borderColor: 'primary.main',
          mb: 1.5,
          fontStyle: 'italic',
          fontSize: '0.75rem',
        }}
      >
        "{selectedText.length > 100 ? selectedText.substring(0, 100) + '...' : selectedText}"
      </Box>

      <TextField
        fullWidth
        multiline
        rows={3}
        value={commentText}
        onChange={(e) => onCommentChange(e.target.value)}
        placeholder="Add your comment..."
        autoFocus
        sx={{ mb: 1.5 }}
      />

      <Box display="flex" gap={1} justifyContent="flex-end">
        <Button
          size="small"
          onClick={onCancel}
        >
          Cancel
        </Button>
        <Button
          size="small"
          variant="contained"
          onClick={onCreate}
          disabled={!commentText.trim()}
        >
          Comment
        </Button>
      </Box>
    </Paper>
  )
}
