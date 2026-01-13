import React from 'react';
import { useQuery } from '@tanstack/react-query';
import useApi from '../../hooks/useApi';
import {
  Box,
  Typography,
  Alert,
  CircularProgress,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Chip,
  Stack,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import TerminalIcon from '@mui/icons-material/Terminal';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';

interface ZedSettingsViewerProps {
  sessionId: string;
}

// Hook to check if there are valid MCP tools (non-empty commands)
export const useHasValidMCPTools = (sessionId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  const { data: settings, isLoading } = useQuery({
    queryKey: ['zed-settings', sessionId],
    queryFn: () => apiClient.v1SessionsZedSettingsDetail(sessionId),
    select: (response) => response.data || {},
    refetchInterval: 30000,
    enabled: !!sessionId,
  });

  if (isLoading || !settings) return false;

  const contextServers = settings?.context_servers || {};
  const validServers = Object.entries(contextServers).filter(
    ([_, config]: [string, any]) => config.command && config.command.trim() !== ''
  );

  return validServers.length > 0;
};

const ZedSettingsViewer: React.FC<ZedSettingsViewerProps> = ({ sessionId }) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  const { data: settings, isLoading, error } = useQuery({
    queryKey: ['zed-settings', sessionId],
    queryFn: () => apiClient.v1SessionsZedSettingsDetail(sessionId),
    select: (response) => response.data || {},
    refetchInterval: 30000, // Refresh every 30 seconds to show live changes
  });

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ m: 2 }}>
        {error instanceof Error ? error.message : 'Failed to load Zed settings'}
      </Alert>
    );
  }

  const contextServers = settings?.context_servers || {};

  // Filter to only include servers with non-empty commands
  const validServers = Object.entries(contextServers).filter(
    ([_, config]: [string, any]) => config.command && config.command.trim() !== ''
  );

  // Don't render anything if no valid servers
  if (validServers.length === 0) {
    return null;
  }

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>
        MCP Tools ({validServers.length})
      </Typography>

      <Alert severity="info" icon={<InfoOutlinedIcon />} sx={{ mb: 2, fontSize: '0.75rem' }}>
        Tools synced from app config, available in Zed's AI assistant.
      </Alert>

      <Stack spacing={1}>
        {validServers.map(([name, config]: [string, any]) => (
          <Accordion key={name} defaultExpanded={false} sx={{ bgcolor: 'background.paper' }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                <TerminalIcon fontSize="small" color="action" />
                <Typography variant="body2" sx={{ fontWeight: 500 }}>
                  {name}
                </Typography>
                {name.startsWith('helix-') && (
                  <Chip label="Helix Native" size="small" color="primary" variant="outlined" sx={{ height: 20, fontSize: '0.65rem' }} />
                )}
              </Box>
            </AccordionSummary>
            <AccordionDetails>
              <Box sx={{ pl: 2 }}>
                <Typography variant="caption" color="text.secondary" gutterBottom component="div">
                  <strong>Command:</strong>
                </Typography>
                <Typography
                  variant="caption"
                  sx={{
                    fontFamily: 'monospace',
                    bgcolor: 'action.hover',
                    color: 'text.primary',
                    p: 1,
                    borderRadius: 1,
                    mb: 2,
                    display: 'block',
                    wordBreak: 'break-all',
                  }}
                >
                  {config.command}
                </Typography>

                {config.args && config.args.length > 0 && (
                  <>
                    <Typography variant="caption" color="text.secondary" gutterBottom component="div">
                      <strong>Arguments:</strong>
                    </Typography>
                    <Typography
                      variant="caption"
                      sx={{
                        fontFamily: 'monospace',
                        bgcolor: 'action.hover',
                        color: 'text.primary',
                        p: 1,
                        borderRadius: 1,
                        mb: 2,
                        display: 'block',
                        wordBreak: 'break-all',
                      }}
                    >
                      {config.args.join(' ')}
                    </Typography>
                  </>
                )}

                {config.env && Object.keys(config.env).length > 0 && (
                  <>
                    <Typography variant="caption" color="text.secondary" gutterBottom component="div">
                      <strong>Environment:</strong>
                    </Typography>
                    <Box
                      sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.75rem',
                        bgcolor: 'action.hover',
                        color: 'text.primary',
                        p: 1,
                        borderRadius: 1,
                      }}
                    >
                      {Object.entries(config.env).map(([key, value]: [string, any]) => (
                        <Typography
                          key={key}
                          variant="caption"
                          sx={{ fontFamily: 'monospace', display: 'block', wordBreak: 'break-all' }}
                        >
                          {key}={value === config.env.HELIX_TOKEN || value === config.env.HELIX_API_TOKEN
                            ? '***'
                            : value}
                        </Typography>
                      ))}
                    </Box>
                  </>
                )}
              </Box>
            </AccordionDetails>
          </Accordion>
        ))}
      </Stack>
    </Box>
  );
};

export default ZedSettingsViewer;
