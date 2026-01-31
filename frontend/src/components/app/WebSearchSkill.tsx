import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Switch,
  FormControlLabel,
  Alert,
  TextField,
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { TypesAssistantWebSearch } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface WebSearchSkillProps {
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

const WebSearchSkill: React.FC<WebSearchSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [webSearchConfig, setWebSearchConfig] = useState<TypesAssistantWebSearch>({
    enabled: false,
    max_results: 10,
  });

  useEffect(() => {
    if (app.webSearchTool) {
      setWebSearchConfig(app.webSearchTool);
    } else {
      setWebSearchConfig({
        enabled: false,
        max_results: 10,
      });
    }
  }, [app.webSearchTool]);

  const handleChange = async (field: keyof TypesAssistantWebSearch, value: boolean | number) => {
    try {
      setError(null);
      
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));
      
      // Update the web search config
      const updatedConfig = {
        ...webSearchConfig,
        [field]: value,
      };
      
      // Update the web search tool configuration
      appCopy.webSearchTool = updatedConfig;
      
      // Update the application
      await onUpdate(appCopy);
      
      // Update local state
      setWebSearchConfig(updatedConfig);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update web search configuration');
    }
  };

  const handleEnable = async () => {
    try {
      setError(null);
      
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Enable the web search skill and default options
      webSearchConfig.enabled = true;      
      webSearchConfig.max_results = 10;
      
      // Update the web search tool configuration
      appCopy.webSearchTool = webSearchConfig;
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save web search configuration');
    }
  };

  const handleDisable = async () => {
    try {
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));
      
      // Set web search config as disabled
      appCopy.webSearchTool = {
        enabled: false,
        max_results: 10,
      };
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable web search');
    }
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      TransitionProps={{
        onExited: () => {
          setWebSearchConfig({
            enabled: false,
            max_results: 10,
          });
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Web Search Skill
          </NameTypography>
          <DescriptionTypography>
            Enable the web search skill to allow the AI to search the web for current information. This skill allows the AI to find recent news, facts, and up-to-date information from search engines.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2, fontWeight: 600 }}>
              Search Configuration
            </Typography>
            <TextField
              label="Maximum Results"
              type="number"
              value={webSearchConfig.max_results || 10}
              onChange={(e) => handleChange('max_results', parseInt(e.target.value) || 10)}
              disabled={!webSearchConfig.enabled}
              sx={{
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
              }}
              inputProps={{
                min: 1,
                max: 50,
              }}
            />
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              The maximum number of search results to return. Higher values provide more comprehensive information but may increase response time.
            </Typography>
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
              {webSearchConfig.enabled && (
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
              {!webSearchConfig.enabled && (
                <Button
                  onClick={handleEnable}
                  size="small"
                  variant="outlined"
                  color="secondary"
                >
                  Enable
                </Button>
              )}
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default WebSearchSkill; 