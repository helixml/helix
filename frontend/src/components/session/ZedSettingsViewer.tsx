import React from 'react';
import { useQuery } from '@tanstack/react-query';
import useApi from '../../hooks/useApi';
import {
  Box,
  Card,
  CardContent,
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
import ExtensionIcon from '@mui/icons-material/Extension';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';

interface ZedSettingsViewerProps {
  sessionId: string;
}

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
  const serverCount = Object.keys(contextServers).length;

  if (serverCount === 0) {
    return null;
  }

  return (
    <Card sx={{ m: 2 }}>
      <CardContent>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
          <ExtensionIcon sx={{ mr: 1, color: 'primary.main' }} />
          <Typography variant="h6" component="h2">
            MCP Tools ({serverCount})
          </Typography>
        </Box>

        <Alert severity="info" icon={<InfoOutlinedIcon />} sx={{ mb: 2 }}>
          These tools are automatically synced from your app configuration and available in Zed's AI assistant.
          Settings sync every 30 seconds and update in real-time when you modify the app configuration.
        </Alert>

        <Stack spacing={1}>
          {Object.entries(contextServers).map(([name, config]: [string, any]) => (
            <Accordion key={name} defaultExpanded={false}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                  <TerminalIcon fontSize="small" color="action" />
                  <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
                    {name}
                  </Typography>
                  {name.startsWith('helix-') && (
                    <Chip label="Helix Native" size="small" color="primary" variant="outlined" />
                  )}
                </Box>
              </AccordionSummary>
              <AccordionDetails>
                <Box sx={{ pl: 2 }}>
                  <Typography variant="body2" color="text.secondary" gutterBottom>
                    <strong>Command:</strong>
                  </Typography>
                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: 'monospace',
                      bgcolor: 'grey.100',
                      p: 1,
                      borderRadius: 1,
                      mb: 2,
                    }}
                  >
                    {config.command}
                  </Typography>

                  {config.args && config.args.length > 0 && (
                    <>
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        <strong>Arguments:</strong>
                      </Typography>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: 'monospace',
                          bgcolor: 'grey.100',
                          p: 1,
                          borderRadius: 1,
                          mb: 2,
                        }}
                      >
                        {config.args.join(' ')}
                      </Typography>
                    </>
                  )}

                  {config.env && Object.keys(config.env).length > 0 && (
                    <>
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        <strong>Environment:</strong>
                      </Typography>
                      <Box
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.875rem',
                          bgcolor: 'grey.100',
                          p: 1,
                          borderRadius: 1,
                        }}
                      >
                        {Object.entries(config.env).map(([key, value]: [string, any]) => (
                          <Typography
                            key={key}
                            variant="body2"
                            sx={{ fontFamily: 'monospace' }}
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

        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 2 }}>
          Settings are managed by the settings-sync daemon running in the container.
          Changes you make in Zed's UI will be preserved and merged with Helix-managed tools.
        </Typography>
      </CardContent>
    </Card>
  );
};

export default ZedSettingsViewer;
