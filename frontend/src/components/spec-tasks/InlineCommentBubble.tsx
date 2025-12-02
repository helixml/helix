import React from 'react'
import { Paper, Box, Chip, IconButton, Typography, CircularProgress } from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import { DesignReviewComment } from '../../services/designReviewService'

interface InlineCommentBubbleProps {
  comment: DesignReviewComment
  yPos: number
  onResolve: (commentId: string) => void
  commentRef?: (el: HTMLDivElement | null) => void
  streamingResponse?: string // Live streaming response content
}

export default function InlineCommentBubble({
  comment,
  yPos,
  onResolve,
  commentRef,
  streamingResponse,
}: InlineCommentBubbleProps) {
  // Use streaming response if available, otherwise fall back to persisted response
  const displayResponse = streamingResponse || comment.agent_response
  const isStreaming = !!streamingResponse && !comment.agent_response
  return (
    <Paper
      ref={commentRef}
      sx={{
        position: 'absolute',
        left: '670px',
        top: `${yPos}px`,
        width: '300px',
        p: 2,
        bgcolor: 'background.paper',
        border: 2,
        borderColor: 'warning.main',
        boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
        zIndex: 10,
        transition: 'top 0.3s ease-in-out',
      }}
    >
      <Box display="flex" alignItems="flex-start" justifyContent="space-between" mb={1}>
        <Chip
          label="Comment"
          size="small"
          color="primary"
        />
        <IconButton size="small" onClick={() => onResolve(comment.id!)}>
          <CloseIcon fontSize="small" />
        </IconButton>
      </Box>

      {comment.quoted_text && (
        <Box
          sx={{
            bgcolor: 'action.hover',
            p: 1,
            borderLeft: '3px solid',
            borderColor: 'primary.main',
            mb: 1,
            fontStyle: 'italic',
            fontSize: '0.75rem',
          }}
        >
          "{comment.quoted_text.length > 100 ? comment.quoted_text.substring(0, 100) + '...' : comment.quoted_text}"
        </Box>
      )}

      <Typography variant="body2" sx={{ mb: 1, fontSize: '0.875rem' }}>
        {comment.comment_text}
      </Typography>

      {/* Show status when comment has been sent to agent but no response yet */}
      {!displayResponse && !comment.resolved && (
        <Box
          sx={{
            mt: 2,
            p: 1,
            bgcolor: 'action.hover',
            borderRadius: 1,
            display: 'flex',
            alignItems: 'center',
            gap: 1,
          }}
        >
          <CircularProgress size={12} />
          <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
            Waiting for agent response...
          </Typography>
        </Box>
      )}

      {displayResponse && (
        <Box
          sx={{
            mt: 2,
            p: 1.5,
            bgcolor: isStreaming ? 'action.hover' : 'info.light',
            borderLeft: '3px solid',
            borderColor: isStreaming ? 'warning.main' : 'info.main',
            borderRadius: 1,
          }}
        >
          <Box display="flex" alignItems="center" gap={1} mb={0.5}>
            <Typography variant="caption" color="primary" fontWeight="bold">
              Agent:
            </Typography>
            {isStreaming && (
              <Box display="flex" alignItems="center" gap={0.5}>
                <CircularProgress size={10} />
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                  typing...
                </Typography>
              </Box>
            )}
          </Box>
          <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontSize: '0.75rem' }}>
            {displayResponse}
          </Typography>
          {comment.agent_response_at && !isStreaming && (
            <Typography variant="caption" color="text.secondary" display="block" mt={0.5}>
              {new Date(comment.agent_response_at).toLocaleString()}
            </Typography>
          )}
        </Box>
      )}

      <Typography variant="caption" color="text.secondary" display="block" mt={1}>
        {new Date(comment.created_at).toLocaleString()}
      </Typography>
    </Paper>
  )
}
