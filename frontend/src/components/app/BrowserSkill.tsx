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
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { TypesAssistantBrowser } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface BrowserSkillProps {
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

const BrowserSkill: React.FC<BrowserSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [browserConfig, setBrowserConfig] = useState<TypesAssistantBrowser>({
    enabled: false,
    markdown_post_processing: false,
    process_output: false,
    no_browser: false,
  });

  useEffect(() => {
    if (app.browserTool) {
      setBrowserConfig(app.browserTool);
    } else {
      setBrowserConfig({
        enabled: false,
        markdown_post_processing: false,
        process_output: false,
        no_browser: false,
      });
    }
  }, [app.browserTool]);

  const handleChange = async (field: keyof TypesAssistantBrowser, value: boolean) => {
    try {
      setError(null);
      
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));
      
      // Update the browser config
      const updatedConfig = {
        ...browserConfig,
        [field]: value,
      };
      
      // Update the browser tool configuration
      appCopy.browserTool = updatedConfig;
      
      // Update the application
      await onUpdate(appCopy);
      
      // Update local state
      setBrowserConfig(updatedConfig);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update browser configuration');
    }
  };

  const handleEnable = async () => {
    try {
      setError(null);
      
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));

      // Enable the browser skill and default options
      browserConfig.enabled = true;      
      browserConfig.markdown_post_processing = true;
      browserConfig.no_browser = browserConfig.no_browser ?? false;
      
      // Update the browser tool configuration
      appCopy.browserTool = browserConfig;
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save browser configuration');
    }
  };

  const handleDisable = async () => {
    try {
      // Create a copy of the app state
      const appCopy = JSON.parse(JSON.stringify(app));
      
      // Set browser config as disabled
      appCopy.browserTool = {
        enabled: false,
        markdown_post_processing: false,
        process_output: false,
        no_browser: false,
      };
      
      // Update the application
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable browser');
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
          setBrowserConfig({
            enabled: false,
            markdown_post_processing: false,
            process_output: false,
            no_browser: false,
          });
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Browser Skill
          </NameTypography>
          <DescriptionTypography>
            Enable the browser skill to allow the AI to search and browse the web. This skill allows the AI to visit websites and extract information from them.
          </DescriptionTypography>

          <SectionCard>
            <FormControlLabel
              control={
                <Switch
                  checked={browserConfig.markdown_post_processing}
                  onChange={(e) => handleChange('markdown_post_processing', e.target.checked)}
                  color="primary"
                  disabled={!browserConfig.enabled}
                />
              }
              label={
                <Typography sx={{ color: '#F8FAFC' }}>
                  Convert HTML to Markdown
                </Typography>
              }
            />
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              When enabled, the browser will convert HTML content to Markdown format, making it easier for the AI to process and present the information.
            </Typography>
          </SectionCard>

          <SectionCard>
            <FormControlLabel
              control={
                <Switch
                  checked={browserConfig.process_output}
                  onChange={(e) => handleChange('process_output', e.target.checked)}
                  color="primary"
                  disabled={!browserConfig.enabled}
                />
              }
              label={
                <Typography sx={{ color: '#F8FAFC' }}>
                  Summarize Web Page Results
                </Typography>
              }
            />
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              {/* This will use small generation model to summarize the web page results. This can greatly reduce the costs, however some information might be lost */}
              When enabled, the browser will use a small generation model to summarize web page results. This can greatly reduce costs, however some information might be lost.
            </Typography>
          </SectionCard>

          <SectionCard>
            <FormControlLabel
              control={
                <Switch
                  checked={browserConfig.no_browser}
                  onChange={(e) => handleChange('no_browser', e.target.checked)}
                  color="primary"
                  disabled={!browserConfig.enabled}
                />
              }
              label={
                <Typography sx={{ color: '#F8FAFC' }}>
                  No Browser Mode
                </Typography>
              }
            />
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              When enabled, this will not use Chrome browser and will instead use a simple but very fast HTTP client to fetch the text.
              While this works great for static websites or /llms.txt, this approach does not suit SPA applications.
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
              {browserConfig.enabled && (
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
              {!browserConfig.enabled && (
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

export default BrowserSkill; 