import React from 'react';
import {
  DialogTitle,
  DialogContent,
  IconButton,
  Typography,
  Box,
  Chip,
  Divider,
  Paper,
  useTheme,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesStepInfo } from '../../api/api';
import DarkDialog from '../dialog/DarkDialog';

interface SkillExecutionDialogProps {
  open: boolean;
  onClose: () => void;
  stepInfo: TypesStepInfo | null;
}

const SkillExecutionDialog: React.FC<SkillExecutionDialogProps> = ({
  open,
  onClose,
  stepInfo,
}) => {
  const theme = useTheme();

  if (!stepInfo) return null;

  const formatTime = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
  };

  const renderArguments = (arguments_: Record<string, any>) => {
    return Object.entries(arguments_).map(([key, value]) => (
      <Box key={key} sx={{ mb: 2 }}>
        <Typography variant="subtitle2" color="text.secondary" sx={{ fontWeight: 'bold', mb: 1 }}>
          {key}:
        </Typography>
        <Paper
          sx={{
            p: 2,
            backgroundColor: 'transparent',
            borderRadius: 1,
            border: '1px solid',
            borderColor: 'divider',
            fontFamily: 'monospace',
            fontSize: '0.875rem',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)}
        </Paper>
      </Box>
    ));
  };

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {stepInfo.icon && (
            <Typography variant="h6" component="span">
              {stepInfo.icon}
            </Typography>
          )}
          <Typography variant="h6" component="div">
            Skill Execution: {stepInfo.name}
          </Typography>
        </Box>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" sx={{ mb: 2 }}>
            Execution Details
          </Typography>
          
          <Box sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: 2, mb: 2 }}>
            <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
              Started:
            </Typography>
            <Typography variant="body2">
              {stepInfo.created ? formatTime(stepInfo.created) : 'N/A'}
            </Typography>

            <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
              Duration:
            </Typography>
            <Typography variant="body2">
              {stepInfo.duration_ms ? formatDuration(stepInfo.duration_ms) : 'N/A'}
            </Typography>            

            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5,fontWeight: 'bold' }}>
              Status:
            </Typography>
            <Box>
              {stepInfo.error ? (
                <Chip label="Error" color="error" size="small" />
              ) : (
                <Chip label="Success" color="success" size="small" />
              )}
            </Box>
          </Box>
        </Box>

        {stepInfo.error && (
          <Box sx={{ mb: 3 }}>
            <Typography variant="h6" color="error" sx={{ mb: 1 }}>
              Error
            </Typography>
            <Paper
              sx={{
                p: 2,
                backgroundColor: 'transparent',
                borderRadius: 1,
                border: '1px solid',
                borderColor: 'divider',
              }}
            >
              <Typography variant="body2" color="error">
                {stepInfo.error}
              </Typography>
            </Paper>
          </Box>
        )}

        {stepInfo.details?.arguments && Object.keys(stepInfo.details.arguments).length > 0 && (
          <Box sx={{ mb: 3 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Arguments
            </Typography>
            {renderArguments(stepInfo.details.arguments)}
          </Box>
        )}

        {stepInfo.message && (
          <Box sx={{ mb: 3 }}>
            <Typography variant="h6" sx={{ mb: 1 }}>
              Response
            </Typography>
            <Paper
              sx={{
                p: 2,
                backgroundColor: 'transparent',
                borderRadius: 1,
                border: '1px solid',
                borderColor: 'divider',
                fontFamily: 'monospace',
                fontSize: '0.875rem',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}
            >
              {stepInfo.message}
            </Paper>
          </Box>
        )}

        {stepInfo.id && (
          <Box sx={{ mt: 3, pt: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
            <Typography variant="caption" color="text.secondary">
              Step ID: {stepInfo.id}
            </Typography>
          </Box>
        )}
      </DialogContent>
    </DarkDialog>
  );
};

export default SkillExecutionDialog; 