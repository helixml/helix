import React, { useState } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import { useEnableSkill } from '../../hooks/useEnableSkill';

interface CodeIntelligenceSkillProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  app: IAppFlatState;
  appId?: string;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));

const DescriptionTypography = styled(Typography)(({ theme }) => ({
  fontSize: '1.1rem',
  color: '#A0AEC0',
  marginBottom: theme.spacing(3),
}));

const CodeIntelligenceSkill: React.FC<CodeIntelligenceSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  appId,
  onUpdate,
  isEnabled,
}) => {
  const lightTheme = useLightTheme();
  const enableSkill = useEnableSkill();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleEnable = async () => {
    if (!appId) {
      setError('App ID is not available');
      return;
    }
    try {
      setError(null);
      setLoading(true);
      const updatedApp = await enableSkill(appId, 'code-intelligence');
      await onUpdate({
        ...app,
        mcpTools: updatedApp.config?.helix?.assistants?.[0]?.mcps || app.mcpTools,
      });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enable Code Intelligence');
    } finally {
      setLoading(false);
    }
  };

  const handleDisable = async () => {
    try {
      setError(null);
      const updatedMcpTools = (app.mcpTools || []).filter(
        tool => tool.name !== 'Code Intelligence'
      );
      await onUpdate({ ...app, mcpTools: updatedMcpTools });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable Code Intelligence');
    }
  };

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{
        onExited: () => {
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>Code Intelligence</NameTypography>
          <DescriptionTypography>
            Search and navigate your organisation's code repositories using semantic search,
            keyword search, grep, and file browsing. Powered by Kodit — enabled automatically
            using your Helix account. No additional configuration required.
          </DescriptionTypography>
          <Typography variant="body2" color="text.secondary">
            Once enabled, the agent can answer questions about your codebase, find relevant files,
            and understand how different parts of the code relate to each other.
          </Typography>
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F', flexDirection: 'column', alignItems: 'stretch' }}>
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button
            onClick={onClose}
            size="small"
            variant="outlined"
            color="primary"
          >
            Cancel
          </Button>
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', gap: 1 }}>
            {isEnabled ? (
              <Button
                onClick={handleDisable}
                size="small"
                variant="outlined"
                color="error"
                sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
              >
                Disable
              </Button>
            ) : (
              <Button
                onClick={handleEnable}
                size="small"
                variant="outlined"
                color="secondary"
                disabled={loading}
              >
                {loading ? 'Enabling…' : 'Enable'}
              </Button>
            )}
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default CodeIntelligenceSkill;
