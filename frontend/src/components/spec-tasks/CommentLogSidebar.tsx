import React from 'react'
import { Box, Typography, Paper, Chip, IconButton } from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import { DesignReviewComment } from '../../services/designReviewService'

interface CommentLogSidebarProps {
  show: boolean
  comments: DesignReviewComment[]
  onResolveComment: (commentId: string) => void
}

export default function CommentLogSidebar({
  show,
  comments,
  onResolveComment,
}: CommentLogSidebarProps) {
  if (!show) return null

  return (
    <Box
      width="400px"
      borderLeft={1}
      borderColor="divider"
      display="flex"
      flexDirection="column"
      bgcolor="background.paper"
    >
      <Box p={2} borderBottom={1} borderColor="divider">
        <Typography variant="h6">
          Comment Log ({comments.length})
        </Typography>
        <Box mt={1} p={1} bgcolor="action.hover" borderRadius={1}>
          <Typography variant="caption" color="text.secondary" display="block">
            <strong>Shortcuts:</strong> C=Comment, 1/2/3=Switch tabs, Esc=Close
          </Typography>
        </Box>
      </Box>

      <Box flex={1} overflow="auto" p={2}>
        {comments.length === 0 ? (
          <Typography variant="body2" color="text.secondary" align="center" mt={4}>
            No comments yet. Select text in the document to add a comment.
          </Typography>
        ) : (
          comments.map(comment => (
            <Paper key={comment.id} sx={{ mb: 2, p: 2, opacity: comment.resolved ? 0.6 : 1 }}>
              <Box display="flex" alignItems="flex-start" justifyContent="space-between" mb={1}>
                <Chip
                  label={comment.quoted_text ? "Inline" : "General"}
                  size="small"
                  color={comment.quoted_text ? "primary" : "default"}
                />
                {!comment.resolved && (
                  <IconButton size="small" onClick={() => onResolveComment(comment.id!)}>
                    <CloseIcon fontSize="small" />
                  </IconButton>
                )}
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
                    fontSize: '0.875rem',
                  }}
                >
                  "{comment.quoted_text.length > 80 ? comment.quoted_text.substring(0, 80) + '...' : comment.quoted_text}"
                </Box>
              )}

              <Typography variant="body2" sx={{ mb: 1 }}>{comment.comment_text}</Typography>

              {comment.agent_response && (
                <Box
                  sx={{
                    mt: 2,
                    p: 2,
                    bgcolor: 'info.light',
                    borderLeft: '3px solid',
                    borderColor: 'info.main',
                    borderRadius: 1,
                  }}
                >
                  <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={1}>
                    Agent Response:
                  </Typography>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {comment.agent_response}
                  </Typography>
                  {comment.agent_response_at && (
                    <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                      {new Date(comment.agent_response_at).toLocaleString()}
                    </Typography>
                  )}
                </Box>
              )}

              {comment.resolved && (
                <Chip
                  label={comment.resolution_reason === 'auto_text_removed' ? 'Resolved (text updated)' : 'Resolved'}
                  size="small"
                  color="success"
                  icon={<CheckCircleIcon />}
                  sx={{ mt: 1 }}
                />
              )}

              <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                {new Date(comment.created_at).toLocaleString()}
              </Typography>
            </Paper>
          ))
        )}
      </Box>
    </Box>
  )
}
