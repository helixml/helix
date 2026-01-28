import React, { FC, useState } from 'react';
import {
  Card,
  CardHeader,
  CardContent,
  Button,
  Alert,
  Typography,
  Stack,
  IconButton,
  Tooltip,
  Switch,
  FormControlLabel,
  Box,
} from '@mui/material';
import {
  Share as ShareIcon,
  Chat as ChatIcon,
  CheckCircle as CheckCircleIcon,
  Edit as EditIcon,
  ContentCopy as ContentCopyIcon,
  Public as PublicIcon,
  Lock as LockIcon,
} from '@mui/icons-material';
import useAccount from '../../hooks/useAccount';
import useSnackbar from '../../hooks/useSnackbar';
import useApi from '../../hooks/useApi';

interface SpecTaskReviewPanelProps {
  taskId: string;
  taskName: string;
  status: string;
  specSessionId: string;
  publicDesignDocs?: boolean;
  onApprove?: () => void;
  onRequestChanges?: () => void;
  onPublicToggle?: (isPublic: boolean) => void;
}

const SpecTaskReviewPanel: FC<SpecTaskReviewPanelProps> = ({
  taskId,
  taskName,
  status,
  specSessionId,
  publicDesignDocs = false,
  onApprove,
  onRequestChanges,
  onPublicToggle,
}) => {
  const account = useAccount();
  const snackbar = useSnackbar();
  const api = useApi();
  const [isPublic, setIsPublic] = useState(publicDesignDocs);
  const [updating, setUpdating] = useState(false);

  const publicLink = `${window.location.origin}/spec-tasks/${taskId}/view`;

  const handlePublicToggle = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = event.target.checked;
    setUpdating(true);
    try {
      await api.put(`/api/v1/spec-tasks/${taskId}`, {
        public_design_docs: newValue,
      });
      setIsPublic(newValue);
      onPublicToggle?.(newValue);
      snackbar.success(newValue ? 'Design docs are now public' : 'Design docs are now private');
    } catch (err: any) {
      snackbar.error(err.message || 'Failed to update visibility');
    } finally {
      setUpdating(false);
    }
  };

  const copyToClipboard = async () => {
    await navigator.clipboard.writeText(publicLink);
    snackbar.success('Link copied to clipboard!');
  };

  const openPlanningSession = () => {
    account.orgNavigate('session', { session_id: specSessionId });
  };

  const isInDesignPhase = status === 'spec_generation' || status === 'spec_review';
  const canApprove = status === 'spec_review';

  return (
    <Stack spacing={2}>
      {/* Share Design Docs Card */}
      <Card>
        <CardHeader
          title="🔗 Share Design Docs"
          subheader="Share design documents with anyone"
        />
        <CardContent>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
            <FormControlLabel
              control={
                <Switch
                  checked={isPublic}
                  onChange={handlePublicToggle}
                  disabled={updating}
                  color="primary"
                />
              }
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  {isPublic ? (
                    <>
                      <PublicIcon fontSize="small" color="primary" />
                      <Typography variant="body2">Public</Typography>
                    </>
                  ) : (
                    <>
                      <LockIcon fontSize="small" color="action" />
                      <Typography variant="body2">Private</Typography>
                    </>
                  )}
                </Box>
              }
            />
          </Box>

          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
            {isPublic
              ? 'Anyone with the link can view the design documents without logging in.'
              : 'Only users with access to this project can view the design documents.'}
          </Typography>

          {isPublic && (
            <Alert
              severity="success"
              action={
                <Tooltip title="Copy link">
                  <IconButton size="small" onClick={copyToClipboard}>
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              }
            >
              <Typography variant="body2" gutterBottom>
                Public link ready to share
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
                {publicLink}
              </Typography>
            </Alert>
          )}

          {!isPublic && (
            <Button
              fullWidth
              variant="outlined"
              startIcon={<ShareIcon />}
              onClick={copyToClipboard}
            >
              Copy Link (requires login)
            </Button>
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
