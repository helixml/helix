import React, { useState } from 'react';
import {
  Box,
  Drawer,
  IconButton,
  Typography,
  Button,
  TextField,
  Stack,
  Divider,
  Paper,
  Alert,
  CircularProgress,
} from '@mui/material';
import {
  Close as CloseIcon,
  CheckCircle as ApproveIcon,
  Cancel as RejectIcon,
  Delete as ArchiveIcon,
  Send as SendIcon,
} from '@mui/icons-material';
import ReactMarkdown from 'react-markdown';
import { useQuery } from '@tanstack/react-query';
import useApi from '../../hooks/useApi';

interface DesignDocViewerProps {
  open: boolean;
  onClose: () => void;
  taskId: string;
  taskName: string;
  sessionId?: string; // Session ID to send comments to
  onApprove?: (comment?: string) => void;
  onReject?: (comment: string) => void;
  onRejectCompletely?: (comment: string) => void;
}

const DesignDocViewer: React.FC<DesignDocViewerProps> = ({
  open,
  onClose,
  taskId,
  taskName,
  sessionId,
  onApprove,
  onReject,
  onRejectCompletely,
}) => {
  const api = useApi();
  const [comment, setComment] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [sendingComment, setSendingComment] = useState(false);

  // Fetch design docs
  const { data, isLoading, error } = useQuery({
    queryKey: ['design-docs', taskId],
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksDesignDocsDetail(taskId);
      return response.data;
    },
    enabled: open && !!taskId,
  });

  const handleApprove = async () => {
    if (onApprove) {
      setSubmitting(true);
      try {
        await onApprove(comment || undefined);
        onClose();
      } finally {
        setSubmitting(false);
      }
    }
  };

  const handleReject = async () => {
    if (onReject && comment.trim()) {
      setSubmitting(true);
      try {
        await onReject(comment);
        onClose();
      } finally {
        setSubmitting(false);
      }
    }
  };

  const handleRejectCompletely = async () => {
    if (onRejectCompletely) {
      setSubmitting(true);
      try {
        await onRejectCompletely(comment || 'Rejected completely');
        onClose();
      } finally {
        setSubmitting(false);
      }
    }
  };

  // Handle sending comment to agent (without approving/rejecting)
  const handleSendComment = async () => {
    if (!comment.trim() || !sessionId) return;

    setSendingComment(true);
    try {
      // Send message to the planning session
      await api.post(`/api/v1/sessions/${sessionId}/chat`, {
        messages: [
          {
            role: 'user',
            content: comment,
          },
        ],
      });

      // Clear comment after sending
      setComment('');
    } catch (err) {
      console.error('Failed to send comment:', err);
    } finally {
      setSendingComment(false);
    }
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: '600px', md: '800px' },
          maxWidth: '100vw',
        },
      }}
    >
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Header */}
        <Box
          sx={{
            p: 2,
            borderBottom: 1,
            borderColor: 'divider',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Typography variant="h6">Review Design Documents</Typography>
          <IconButton onClick={onClose}>
            <CloseIcon />
          </IconButton>
        </Box>

        {/* Task name */}
        <Box sx={{ px: 3, py: 2, backgroundColor: 'background.default' }}>
          <Typography variant="subtitle2" color="text.secondary">
            Task
          </Typography>
          <Typography variant="h6">{taskName}</Typography>
        </Box>

        {/* Documents */}
        <Box sx={{ flex: 1, overflow: 'auto', backgroundColor: '#ffffff', color: '#000000' }}>
          {isLoading && (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
              <CircularProgress />
            </Box>
          )}

          {error && (
            <Box sx={{ p: 3 }}>
              <Alert severity="error">Failed to load design documents</Alert>
            </Box>
          )}

          {data && data.documents && data.documents.length === 0 && (
            <Box sx={{ p: 3 }}>
              <Alert severity="info">
                No design documents found yet. The agent is still working on generating them.
              </Alert>
            </Box>
          )}

          {data && data.documents && data.documents.map((doc, index) => (
            <Paper
              key={index}
              sx={{
                m: 3,
                p: 4,
                backgroundColor: '#ffffff',
                color: '#000000',
                boxShadow: 3,
                '& h1': { fontSize: '2rem', fontWeight: 700, mb: 2, mt: 4, color: '#000000' },
                '& h2': { fontSize: '1.5rem', fontWeight: 600, mb: 1.5, mt: 3, color: '#000000' },
                '& h3': { fontSize: '1.25rem', fontWeight: 600, mb: 1, mt: 2, color: '#000000' },
                '& p': { mb: 1.5, lineHeight: 1.7, color: '#000000' },
                '& ul, & ol': { mb: 1.5, pl: 4, color: '#000000' },
                '& li': { mb: 0.5, color: '#000000' },
                '& code': {
                  backgroundColor: '#f5f5f5',
                  color: '#d63384',
                  padding: '2px 6px',
                  borderRadius: '4px',
                  fontSize: '0.9em',
                },
                '& pre': {
                  backgroundColor: '#f5f5f5',
                  color: '#000000',
                  p: 2,
                  borderRadius: '4px',
                  overflow: 'auto',
                  mb: 2,
                },
                '& pre code': {
                  backgroundColor: 'transparent',
                  color: '#000000',
                  padding: 0,
                },
                '& blockquote': {
                  borderLeft: '4px solid #ddd',
                  pl: 2,
                  ml: 0,
                  fontStyle: 'italic',
                  color: '#666',
                },
                '& a': {
                  color: '#1976d2',
                  textDecoration: 'underline',
                },
              }}
              elevation={2}
            >
              <Typography
                variant="overline"
                sx={{
                  display: 'block',
                  color: '#666',
                  mb: 2,
                  fontWeight: 600,
                }}
              >
                {doc.filename}
              </Typography>
              <Divider sx={{ mb: 3 }} />
              <ReactMarkdown>{doc.content}</ReactMarkdown>
            </Paper>
          ))}
        </Box>

        {/* Comment and Actions */}
        <Box sx={{ borderTop: 1, borderColor: 'divider', p: 3, backgroundColor: 'background.paper' }}>
          <Stack spacing={2}>
            <TextField
              label="Comments (optional for approval, required for rejection)"
              multiline
              rows={3}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="Add your feedback or requested changes..."
              fullWidth
              disabled={submitting}
            />

            {/* Send comment to agent (without approving/rejecting) */}
            {sessionId && (
              <Button
                variant="outlined"
                startIcon={sendingComment ? <CircularProgress size={16} /> : <SendIcon />}
                onClick={handleSendComment}
                disabled={!comment.trim() || sendingComment || submitting}
                fullWidth
              >
                {sendingComment ? 'Sending...' : 'Send Comment to Agent'}
              </Button>
            )}

            <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end' }}>
              <Button
                variant="outlined"
                color="error"
                startIcon={<ArchiveIcon />}
                onClick={handleRejectCompletely}
                disabled={submitting}
              >
                Reject Completely (Archive)
              </Button>
              <Button
                variant="outlined"
                color="warning"
                startIcon={<RejectIcon />}
                onClick={handleReject}
                disabled={submitting || !comment.trim()}
              >
                Request Changes
              </Button>
              <Button
                variant="contained"
                color="success"
                startIcon={<ApproveIcon />}
                onClick={handleApprove}
                disabled={submitting}
              >
                Approve & Start Implementation
              </Button>
            </Stack>
          </Stack>
        </Box>
      </Box>
    </Drawer>
  );
};

export default DesignDocViewer;
