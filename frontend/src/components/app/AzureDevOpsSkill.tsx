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
import { TypesAssistantAzureDevOps } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface AzureDevOpsSkillProps {
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

const AzureDevOpsSkill: React.FC<AzureDevOpsSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [azureDevOpsConfig, setAzureDevOpsConfig] = useState<TypesAssistantAzureDevOps>({
    organization_url: '',
    personal_access_token: '',
    enabled: false,
  });
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    if (app.azureDevOpsTool) {
      setAzureDevOpsConfig({
        ...app.azureDevOpsTool,
        enabled: app.azureDevOpsTool.enabled ?? false,
      });
    } else {
      setAzureDevOpsConfig({
        organization_url: '',
        personal_access_token: '',
        enabled: false,
      });
    }
  }, [app.azureDevOpsTool]);

  const handleChange = (field: keyof TypesAssistantAzureDevOps, value: string) => {
    setError(null);
    
    // Only update local state, don't send to server
    const updatedConfig = {
      ...azureDevOpsConfig,
      [field]: value,
    };
    
    setAzureDevOpsConfig(updatedConfig);
  };

  const handleEnable = async () => {
    try {
      setError(null);
      
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Enable the Azure DevOps skill with current configuration
      appCopy.azureDevOpsTool = {
        ...azureDevOpsConfig,
        enabled: true,
      };
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save Azure DevOps configuration');
    }
  };

  const handleDisable = async () => {
    try {
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));
      
      // Set enabled to false but keep the URL and PAT
      appCopy.azureDevOpsTool = {
        ...azureDevOpsConfig,
        enabled: false,
      };
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable Azure DevOps');
    }
  };

  const handleClose = () => {
    onClose();
  };

  const isConfigured = azureDevOpsConfig.organization_url && azureDevOpsConfig.personal_access_token;

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      TransitionProps={{
        onExited: () => {
          setAzureDevOpsConfig({
            organization_url: '',
            personal_access_token: '',
            enabled: false,
          });
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Azure DevOps PRs
          </NameTypography>
          <DescriptionTypography>
            Enable the Azure DevOps skill to allow the AI to interact with Azure DevOps repositories, pipelines, and work items. This skill provides access to pull requests, builds, and project management features.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2, fontWeight: 600 }}>
              Azure DevOps Configuration
            </Typography>
            
            <DarkTextField
              fullWidth
              label="Organization URL"
              value={azureDevOpsConfig.organization_url || ''}
              onChange={(e) => handleChange('organization_url', e.target.value)}
              placeholder="https://dev.azure.com/myorg"
              helperText="The URL of your Azure DevOps organization"
              margin="normal"
              required
              autoComplete="new-azure-org-url"
            />
            
            <DarkTextField
              fullWidth
              label="Personal Access Token"
              value={azureDevOpsConfig.personal_access_token || ''}
              onChange={(e) => handleChange('personal_access_token', e.target.value)}
              type={showPassword ? 'text' : 'password'}
              placeholder="Enter your Azure DevOps Personal Access Token"
              helperText={
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Create a Personal Access Token with appropriate permissions for your Azure DevOps organization.
                  </Typography>
                  <Link
                    href="https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows"
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
              autoComplete="new-azure-pat"
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

export default AzureDevOpsSkill; 