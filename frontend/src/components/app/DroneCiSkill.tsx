import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
  TextField,
  Link,
  InputAdornment,
  IconButton,
} from '@mui/material';
import { Visibility, VisibilityOff } from '@mui/icons-material';
import { IAppFlatState } from '../../types';
import { TypesAssistantMCP } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface DroneCiSkillProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  app: IAppFlatState;
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

const SectionCard = styled(Box)(({ theme }) => ({
  background: '#23262F',
  borderRadius: 12,
  padding: theme.spacing(3),
  marginBottom: theme.spacing(3),
  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
}));

const DarkTextField = styled(TextField)(({ theme }) => ({
  '& .MuiOutlinedInput-root': {
    color: '#F8FAFC',
    '& fieldset': {
      borderColor: '#4A5568',
    },
    '&:hover fieldset': {
      borderColor: '#718096',
    },
    '&.Mui-focused fieldset': {
      borderColor: '#3182CE',
    },
  },
  '& .MuiInputLabel-root': {
    color: '#A0AEC0',
    '&.Mui-focused': {
      color: '#3182CE',
    },
  },
  '& .MuiFormHelperText-root': {
    color: '#718096',
  },
}));

const DRONE_CI_MCP_NAME = 'Drone CI';

const DroneCiSkill: React.FC<DroneCiSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [serverUrl, setServerUrl] = useState('');
  const [accessToken, setAccessToken] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // Find existing Drone CI MCP config
  const findExistingConfig = (): { index: number; config: TypesAssistantMCP } | null => {
    const index = app.mcpTools?.findIndex(
      mcp => mcp.name === DRONE_CI_MCP_NAME && mcp.transport === 'stdio'
    ) ?? -1;
    if (index !== -1 && app.mcpTools) {
      return { index, config: app.mcpTools[index] };
    }
    return null;
  };

  useEffect(() => {
    const existing = findExistingConfig();
    if (existing) {
      setServerUrl(existing.config.env?.DRONE_SERVER_URL || '');
      setAccessToken(existing.config.env?.DRONE_ACCESS_TOKEN || '');
    } else {
      setServerUrl('');
      setAccessToken('');
    }
  }, [app.mcpTools, open]);

  const handleEnable = async () => {
    try {
      setError(null);

      // Validate
      if (!serverUrl.trim()) {
        setError('Server URL is required');
        return;
      }
      if (!accessToken.trim()) {
        setError('Access Token is required');
        return;
      }

      // Create the MCP skill object
      // drone-ci-mcp is installed globally in the desktop images
      const mcpSkill: TypesAssistantMCP = {
        name: DRONE_CI_MCP_NAME,
        description: 'Drone CI integration for viewing build status and navigating logs',
        transport: 'stdio',
        command: 'drone-ci-mcp',
        args: [],
        env: {
          DRONE_SERVER_URL: serverUrl.trim(),
          DRONE_ACCESS_TOKEN: accessToken.trim(),
        },
      };

      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Initialize mcpTools array if it doesn't exist
      if (!appCopy.mcpTools) {
        appCopy.mcpTools = [];
      }

      // Find existing Drone CI config
      const existing = findExistingConfig();
      if (existing !== null) {
        appCopy.mcpTools[existing.index] = mcpSkill;
      } else {
        appCopy.mcpTools.push(mcpSkill);
      }

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save Drone CI configuration');
    }
  };

  const handleDisable = async () => {
    try {
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Remove the Drone CI MCP
      const existing = findExistingConfig();
      if (existing !== null) {
        appCopy.mcpTools = appCopy.mcpTools?.filter((_: TypesAssistantMCP, index: number) => index !== existing.index);
      }

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable Drone CI');
    }
  };

  const handleClose = () => {
    onClose();
  };

  const isConfigured = serverUrl.trim() && accessToken.trim();

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{
        onExited: () => {
          setServerUrl('');
          setAccessToken('');
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Drone CI
          </NameTypography>
          <DescriptionTypography>
            Check Drone CI build status, fetch logs, and navigate through CI failures efficiently.
            Large logs are saved to files with search and navigation tools to avoid context overflow.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2, fontWeight: 600 }}>
              Drone CI Configuration
            </Typography>

            <DarkTextField
              fullWidth
              label="Server URL"
              value={serverUrl}
              onChange={(e) => {
                setError(null);
                setServerUrl(e.target.value);
              }}
              placeholder="https://drone.example.com"
              helperText="The URL of your Drone CI server"
              margin="normal"
              required
              autoComplete="new-drone-url"
            />

            <DarkTextField
              fullWidth
              label="Access Token"
              value={accessToken}
              onChange={(e) => {
                setError(null);
                setAccessToken(e.target.value);
              }}
              type={showPassword ? 'text' : 'password'}
              placeholder="Enter your Drone CI access token"
              helperText={
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Get your access token from your Drone CI user settings.
                  </Typography>
                  <Link
                    href="https://docs.drone.io/cli/configure/"
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: '#3182CE', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
                  >
                    Learn how to get your Drone CI access token
                  </Link>
                </Box>
              }
              margin="normal"
              required
              autoComplete="new-drone-token"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle password visibility"
                      onClick={() => setShowPassword(!showPassword)}
                      onMouseDown={(event) => event.preventDefault()}
                      edge="end"
                    >
                      {showPassword ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />

            <Box sx={{ mt: 3, p: 2, bgcolor: '#1A1D24', borderRadius: 1 }}>
              <Typography variant="subtitle2" sx={{ color: '#10B981', mb: 1 }}>
                Available Tools
              </Typography>
              <Typography variant="body2" sx={{ color: '#A0AEC0' }}>
                <strong>drone_build_info</strong> - Get build status, stages, and steps<br />
                <strong>drone_fetch_logs</strong> - Fetch logs to temp file (avoids context overflow)<br />
                <strong>drone_search_logs</strong> - Search for patterns like FAIL:, panic:, error<br />
                <strong>drone_read_logs</strong> - Read specific line ranges<br />
                <strong>drone_tail_logs</strong> - Get last N lines of log file
              </Typography>
            </Box>
          </SectionCard>
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
            onClick={handleClose}
            size="small"
            variant="outlined"
            color="primary"
          >
            Cancel
          </Button>
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
              {initialIsEnabled && (
                <Button
                  onClick={handleDisable}
                  size="small"
                  variant="outlined"
                  color="error"
                  sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
                >
                  Disable
                </Button>
              )}
              <Button
                onClick={handleEnable}
                size="small"
                variant="outlined"
                color="secondary"
                disabled={!isConfigured}
              >
                {initialIsEnabled ? 'Update' : 'Enable'}
              </Button>
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default DroneCiSkill;
