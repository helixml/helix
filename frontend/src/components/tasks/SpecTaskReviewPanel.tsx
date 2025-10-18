import React, { FC, useState } from 'react';
import {
  Box,
  Card,
  CardHeader,
  CardContent,
  Button,
  Alert,
  Typography,
  Stack,
  IconButton,
  Tooltip,
  Snackbar,
} from '@mui/material';
import {
  Share as ShareIcon,
  Chat as ChatIcon,
  CheckCircle as CheckCircleIcon,
  Edit as EditIcon,
  ContentCopy as ContentCopyIcon,
} from '@mui/icons-material';
import useApi from '../../hooks/useApi';
import useRouter from '../../hooks/useRouter';
import useSnackbar from '../../hooks/useSnackbar';

interface SpecTaskReviewPanelProps {
  taskId: string;
  taskName: string;
  status: string;
  specSessionId: string;
  onApprove?: () => void;
  onRequestChanges?: () => void;
}

const SpecTaskReviewPanel: FC<SpecTaskReviewPanelProps> = ({
  taskId,
  taskName,
  status,
  specSessionId,
  onApprove,
  onRequestChanges,
}) => {
  const api = useApi();
  const router = useRouter();
  const snackbar = useSnackbar();
  const [shareLink, setShareLink] = useState<string | null>(null);
  const [generatingLink, setGeneratingLink] = useState(false);

  const generateShareLink = async () => {
    setGeneratingLink(true);
    try {
      const response = await api.post<{ share_url: string; expires_at: string }>(
        `/api/v1/spec-tasks/${taskId}/design-docs/share`
      );

      if (response && response.share_url) {
        setShareLink(response.share_url);

        // Copy to clipboard
        await navigator.clipboard.writeText(response.share_url);
        snackbar.success('Link copied to clipboard!');
      }
    } catch (err: any) {
      snackbar.error(err.message || 'Failed to generate share link');
    } finally {
      setGeneratingLink(false);
    }
  };

  const copyToClipboard = async () => {
    if (shareLink) {
      await navigator.clipboard.writeText(shareLink);
      snackbar.success('Link copied!');
    }
  };

  const openPlanningSession = () => {
    router.navigate(`/session/${specSessionId}`);
  };

  const isInDesignPhase = status === 'spec_generation' || status === 'spec_review';
  const canApprove = status === 'spec_review';

  return (
    <Stack spacing={2}>
      {/* Shareable Link Card */}
      <Card>
        <CardHeader
          title="📱 View on Any Device"
          subheader="Get a shareable link to review on your phone"
        />
        <CardContent>
          <Button
            fullWidth
            variant="outlined"
            startIcon={<ShareIcon />}
            onClick={generateShareLink}
            disabled={generatingLink}
          >
            {generatingLink ? 'Generating...' : 'Get Shareable Link'}
          </Button>

          {shareLink && (
            <Alert
              severity="success"
              sx={{ mt: 2 }}
              action={
                <Tooltip title="Copy link">
                  <IconButton size="small" onClick={copyToClipboard}>
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              }
            >
              <Typography variant="body2" gutterBottom>
                Link copied to clipboard!
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  wordBreak: 'break-all',
                  display: 'block',
                  mt: 1,
                  fontFamily: 'monospace',
                }}
              >
                {shareLink}
              </Typography>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* Interactive Feedback Card */}
      {isInDesignPhase && (
        <Card>
          <CardHeader
            title="💬 Provide Feedback"
            subheader="Continue conversation with planning agent"
          />
          <CardContent>
            <Alert severity="info" sx={{ mb: 2 }}>
              The planning agent is still working on the design. You can chat with it
              to refine the specs before final approval.
            </Alert>

            <Button
              fullWidth
              variant="contained"
              startIcon={<ChatIcon />}
              onClick={openPlanningSession}
            >
              Open Planning Session
            </Button>

            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              Opens the Helix session where you can chat with the planning agent
            </Typography>
          </CardContent>
        </Card>
      )}

      {/* Approval Card */}
      {canApprove && (
        <Card>
          <CardHeader
            title="✅ Ready to Approve?"
            subheader="Start implementation or request changes"
          />
          <CardContent>
            <Stack spacing={2}>
              <Button
                fullWidth
                variant="contained"
                color="success"
                startIcon={<CheckCircleIcon />}
                onClick={onApprove}
              >
                Approve & Start Implementation
              </Button>

              <Button
                fullWidth
                variant="outlined"
                color="warning"
                startIcon={<EditIcon />}
                onClick={onRequestChanges}
              >
                Request Changes
              </Button>

              <Typography variant="caption" color="text.secondary">
                Approving will start the implementation phase with a coding agent
              </Typography>
            </Stack>
          </CardContent>
        </Card>
      )}
    </Stack>
  );
};

export default SpecTaskReviewPanel;
