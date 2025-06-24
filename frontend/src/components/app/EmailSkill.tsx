import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
  TextField,
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { TypesAssistantEmail } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface EmailSkillProps {
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

const EmailSkill: React.FC<EmailSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [emailConfig, setEmailConfig] = useState<TypesAssistantEmail>({
    enabled: false,
    template_example: '',
  });
  const [template, setTemplate] = useState('');
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    if (app.emailTool) {
      setEmailConfig(app.emailTool);
      setTemplate(app.emailTool.template_example || '');
    } else {
      setEmailConfig({
        enabled: false,
        template_example: '',
      });
      setTemplate('');
    }
    setIsDirty(false);
  }, [app.emailTool]);

  const handleTemplateChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newTemplate = event.target.value;
    setTemplate(newTemplate);
    setIsDirty(newTemplate !== (app.emailTool?.template_example || ''));
  };
  
  const handleUpdate = async () => {
    if (!isDirty) {
      return;
    }
    
    try {
      setError(null);
      
      const appCopy = JSON.parse(JSON.stringify(app));

      appCopy.emailTool.template_example = template;
      
      await onUpdate(appCopy);
      
      setEmailConfig(appCopy.emailTool);
      setIsDirty(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update email template');
    }
  };

  const handleEnable = async () => {
    try {
      setError(null);
      
      const appCopy = JSON.parse(JSON.stringify(app));

      const updatedConfig = {
        enabled: true,
        template_example: template,
      };
      
      appCopy.emailTool = updatedConfig;
      
      await onUpdate(appCopy);
      
      setEmailConfig(updatedConfig);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enable email skill');
    }
  };

  const handleDisable = async () => {
    try {
      setError(null);
      const appCopy = JSON.parse(JSON.stringify(app));
      
      appCopy.emailTool = {
        enabled: false,
        template_example: app.emailTool?.template_example,
      };      
      
      await onUpdate(appCopy);
      
      setEmailConfig(appCopy.emailTool);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable email skill');
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
          if (app.emailTool) {
            setEmailConfig(app.emailTool);
            setTemplate(app.emailTool.template_example || '');
          } else {
            setEmailConfig({
              enabled: false,
              template_example: '',
            });
            setTemplate('');
          }
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            {"Email Skill"}
          </NameTypography>
          <DescriptionTypography>
            {"Enable the email skill to allow the agent to send emails to the user (with whom it is currently interacting)."}
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2 }}>
                {"Example Email (Optional)"}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 2 }}>
                {"Provide an example of the email you want the agent to receive. This will help the agent understand the format of the email you expect."}
            </Typography>
            <TextField
                placeholder={`Hello,

Here are top 5 news articles about TSLA:
- [article 1]
- [article 2]
- [article 3]
- [article 4]

General sentiment is good, stock price is going up.`}
                multiline
                rows={10}
                value={template}
                onChange={handleTemplateChange}
                variant="outlined"
                fullWidth
                disabled={!emailConfig.enabled}
                sx={{
                    '& .MuiOutlinedInput-root': {
                      color: '#F8FAFC',
                      '&.Mui-disabled': {
                        color: '#A0AEC0',
                        '& .MuiOutlinedInput-notchedOutline': {
                          borderColor: '#4A5568',
                        },
                      },
                    },
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
              {emailConfig.enabled ? (
                <>                  
                  <Button
                    onClick={handleDisable}
                    size="small"
                    variant="outlined"
                    color="error"
                    sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
                  >
                    Disable
                  </Button>
                  <Button
                    onClick={handleUpdate}
                    size="small"
                    color="secondary"
                    variant="outlined"
                    disabled={!isDirty}
                  >
                    Save
                  </Button>
                </>
              ) : (
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

export default EmailSkill;
