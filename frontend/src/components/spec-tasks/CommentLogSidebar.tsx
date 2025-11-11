import React from 'react'
import { Box, Typography, Paper, Chip } from '@mui/material'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import { DesignReviewComment } from '../../services/designReviewService'

const DOCUMENT_LABELS = {
  requirements: 'Requirements Specification',
  technical_design: 'Technical Design',
  implementation_plan: 'Implementation Plan',
}

interface CommentLogSidebarProps {
  comments: DesignReviewComment[]
}

export default function CommentLogSidebar({ comments }: CommentLogSidebarProps) {
  return (
    <Box
      width="300px"
      borderLeft="1px solid rgba(0,0,0,0.12)"
      display="flex"
      flexDirection="column"
      bgcolor="white"
    >
      <Box p={2} borderBottom="1px solid rgba(0,0,0,0.12)">
        <Typography variant="h6">All Comments</Typography>
      </Box>

      <Box flex={1} overflow="auto" p={2}>
        {comments.length === 0 ? (
          <Typography variant="body2" color="text.secondary" align="center" mt={4}>
            No comments yet.
          </Typography>
        ) : (
          comments.map(comment => (
            <Paper key={comment.id} sx={{ mb: 2, p: 2, opacity: comment.resolved ? 0.6 : 1 }}>
              <Typography variant="caption" color="primary" display="block" mb={0.5}>
                {DOCUMENT_LABELS[comment.document_type]}
              </Typography>

              {comment.quoted_text && (
                <Box
                  sx={{
                    bgcolor: '#f5f5f5',
                    p: 1,
                    borderLeft: '3px solid #2196f3',
                    mb: 1,
                    fontStyle: 'italic',
                    fontSize: '0.75rem',
                  }}
                >
                  "{comment.quoted_text.substring(0, 100)}..."
                </Box>
              )}

              <Typography variant="body2" fontSize="0.875rem" sx={{ mb: 1 }}>
                {comment.comment_text}
              </Typography>

              {comment.agent_response && (
                <Box
                  sx={{
                    mt: 1,
                    p: 1,
                    bgcolor: '#e3f2fd',
                    borderLeft: '2px solid #1976d2',
                    borderRadius: 0.5,
                  }}
                >
                  <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={0.5}>
                    Agent:
                  </Typography>
                  <Typography variant="body2" fontSize="0.75rem">
                    {comment.agent_response.substring(0, 150)}...
                  </Typography>
                </Box>
              )}

              {comment.resolved && (
                <Chip
                  label={comment.resolution_reason === 'auto_text_removed' ? 'Auto-resolved' : 'Resolved'}
                  size="small"
                  color="success"
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
