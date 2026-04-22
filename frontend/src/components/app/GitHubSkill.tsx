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
import { TypesAssistantGitHub } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface GitHubSkillProps {
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

const GitHubSkill: React.FC<GitHubSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [gitHubConfig, setGitHubConfig] = useState<TypesAssistantGitHub>({
    personal_access_token: '',
    base_url: '',
    enabled: false,
  });
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    if (app.gitHubTool) {
      setGitHubConfig({
        ...app.gitHubTool,
        enabled: app.gitHubTool.enabled ?? false,
      });
    } else {
      setGitHubConfig({
        personal_access_token: '',
        base_url: '',
        enabled: false,
      });
    }
  }, [app.gitHubTool]);

  const handleChange = (field: keyof TypesAssistantGitHub, value: string) => {
    setError(null);
    setGitHubConfig({
      ...gitHubConfig,
      [field]: value,
    });
  };

  const handleEnable = async () => {
    try {
      setError(null);
      const appCopy = JSON.parse(JSON.stringify(app));
      appCopy.gitHubTool = {
        ...gitHubConfig,
        enabled: true,
      };
      await onUpdate(appCopy);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save GitHub configuration');
    }
  };

  const handleDisable = async () => {
    try {
      const appCopy = JSON.parse(JSON.stringify(app));
      appCopy.gitHubTool = {
        ...gitHubConfig,
        enabled: false,
      };
      await onUpdate(appCopy);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable GitHub');
    }
  };

  const handleClose = () => {
    onClose();
  };

  const isConfigured = !!gitHubConfig.personal_access_token;

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{
        onExited: () => {
          setGitHubConfig({
            personal_access_token: '',
            base_url: '',
            enabled: false,
          });
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <GitHubIcon sx={{ fontSize: '2rem', color: '#F8FAFC' }} />
            <NameTypography>
              GitHub PRs
            </NameTypography>
          </Box>
          <DescriptionTypography>
            Enable the GitHub skill to allow the AI to review pull requests on GitHub repositories. The AI can read PR diffs and post inline review comments.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2, fontWeight: 600 }}>
              GitHub Configuration
            </Typography>

            <DarkTextField
              fullWidth
              label="Personal Access Token"
              value={gitHubConfig.personal_access_token || ''}
              onChange={(e) => handleChange('personal_access_token', e.target.value)}
              type={showPassword ? 'text' : 'password'}
              placeholder="Enter your GitHub Personal Access Token"
              helperText={
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Create a Personal Access Token with repo scope for PR review access.
                  </Typography>
                  <Link
                    href="https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens"
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: '#3182CE', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
                  >
                    Learn how to create a Personal Access Token
                  </Link>
                </Box>
              }
              margin="normal"
              required
              autoComplete="new-github-pat"
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

            <DarkTextField
              fullWidth
              label="Base URL (optional)"
              value={gitHubConfig.base_url || ''}
              onChange={(e) => handleChange('base_url', e.target.value)}
              placeholder="https://github.example.com"
              helperText="Only needed for GitHub Enterprise instances. Leave empty for github.com."
              margin="normal"
              autoComplete="new-github-base-url"
            />
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

export default GitHubSkill;
