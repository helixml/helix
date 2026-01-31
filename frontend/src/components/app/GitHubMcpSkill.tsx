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
import GitHubIcon from '@mui/icons-material/GitHub';
import { IAppFlatState } from '../../types';
import { TypesAssistantMCP } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface GitHubMcpSkillProps {
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

const GITHUB_MCP_NAME = 'GitHub';

const GitHubMcpSkill: React.FC<GitHubMcpSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [accessToken, setAccessToken] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // Find existing GitHub MCP config
  const findExistingConfig = (): { index: number; config: TypesAssistantMCP } | null => {
    const index = app.mcpTools?.findIndex(
      mcp => mcp.name === GITHUB_MCP_NAME && mcp.transport === 'stdio'
    ) ?? -1;
    if (index !== -1 && app.mcpTools) {
      return { index, config: app.mcpTools[index] };
    }
    return null;
  };

  useEffect(() => {
    const existing = findExistingConfig();
    if (existing) {
      setAccessToken(existing.config.env?.GITHUB_PERSONAL_ACCESS_TOKEN || '');
    } else {
      setAccessToken('');
    }
  }, [app.mcpTools, open]);

  const handleEnable = async () => {
    try {
      setError(null);

      // Validate
      if (!accessToken.trim()) {
        setError('Personal Access Token is required');
        return;
      }

      // Create the MCP skill object using official GitHub MCP server
      const mcpSkill: TypesAssistantMCP = {
        name: GITHUB_MCP_NAME,
        description: 'GitHub integration for issues, PRs, repos, and more',
        transport: 'stdio',
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-github'],
        env: {
          GITHUB_PERSONAL_ACCESS_TOKEN: accessToken.trim(),
        },
      };

      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Initialize mcpTools array if it doesn't exist
      if (!appCopy.mcpTools) {
        appCopy.mcpTools = [];
      }

      // Find existing GitHub config
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
      setError(err instanceof Error ? err.message : 'Failed to save GitHub configuration');
    }
  };

  const handleDisable = async () => {
    try {
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Remove the GitHub MCP
      const existing = findExistingConfig();
      if (existing !== null) {
        appCopy.mcpTools = appCopy.mcpTools?.filter((_: TypesAssistantMCP, index: number) => index !== existing.index);
      }

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable GitHub');
    }
  };

  const handleClose = () => {
    onClose();
  };

  const isConfigured = accessToken.trim().length > 0;

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{
        onExited: () => {
          setAccessToken('');
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 1 }}>
            <GitHubIcon sx={{ fontSize: 40 }} />
            <NameTypography sx={{ mb: 0 }}>
              GitHub
            </NameTypography>
          </Box>
          <DescriptionTypography>
            Enable the GitHub skill to allow the AI to interact with GitHub repositories, issues, pull requests, and more.
            Uses the official GitHub MCP server from Anthropic.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2, fontWeight: 600 }}>
              GitHub Configuration
            </Typography>

            <DarkTextField
              fullWidth
              label="Personal Access Token"
              value={accessToken}
              onChange={(e) => {
                setError(null);
                setAccessToken(e.target.value);
              }}
              type={showPassword ? 'text' : 'password'}
              placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
              helperText={
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Create a Personal Access Token with repo, issues, and pull_request scopes.
                  </Typography>
                  <Link
                    href="https://github.com/settings/tokens/new?description=Helix%20AI&scopes=repo,read:org,read:user"
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: '#3182CE', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
                  >
                    Create a new Personal Access Token on GitHub
                  </Link>
                </Box>
              }
              margin="normal"
              required
              autoComplete="new-github-token"
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
                <strong>create_or_update_file</strong> - Create or update a single file in a repository<br />
                <strong>search_repositories</strong> - Search for GitHub repositories<br />
                <strong>create_repository</strong> - Create a new GitHub repository<br />
                <strong>get_file_contents</strong> - Get contents of a file or directory<br />
                <strong>push_files</strong> - Push multiple files in a single commit<br />
                <strong>create_issue</strong> - Create a new issue in a repository<br />
                <strong>create_pull_request</strong> - Create a new pull request<br />
                <strong>fork_repository</strong> - Fork a repository<br />
                <strong>create_branch</strong> - Create a new branch<br />
                <strong>list_commits</strong> - Get list of commits<br />
                <strong>list_issues</strong> - List and filter issues<br />
                <strong>update_issue</strong> - Update an existing issue<br />
                <strong>add_issue_comment</strong> - Add a comment to an issue<br />
                <strong>search_code</strong> - Search for code across repositories<br />
                <strong>search_issues</strong> - Search for issues and pull requests<br />
                <strong>search_users</strong> - Search for GitHub users<br />
                <strong>get_issue</strong> - Get details of a specific issue<br />
                <strong>get_pull_request</strong> - Get details of a specific PR<br />
                <strong>list_pull_requests</strong> - List pull requests in a repository<br />
                <strong>create_pull_request_review</strong> - Create a review on a PR<br />
                <strong>merge_pull_request</strong> - Merge a pull request<br />
                <strong>get_pull_request_files</strong> - Get list of files changed in a PR<br />
                <strong>get_pull_request_status</strong> - Get combined status for a PR<br />
                <strong>update_pull_request_branch</strong> - Update a PR branch<br />
                <strong>get_pull_request_comments</strong> - Get review comments on a PR<br />
                <strong>get_pull_request_reviews</strong> - Get reviews on a PR
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

export default GitHubMcpSkill;
