import React, { useState, useEffect } from 'react';
import { AxiosError } from 'axios';
import {
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,  
  IconButton,
  List,
  ListItem,
  Grid,
  Alert,
  Tooltip,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { IAppFlatState } from '../../types';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import useApi from '../../hooks/useApi';

interface AddMcpSkillDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  skill?: {
    name: string;
    description: string;
    url: string;
    headers?: Record<string, string>;
  };
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
  '& .MuiInputBase-root': {
    background: '#23262F',
    color: '#F1F1F1',
    borderRadius: 8,
  },
  '& .MuiInputLabel-root': {
    color: '#A0AEC0',
  },
  '& .MuiOutlinedInput-notchedOutline': {
    borderColor: '#353945',
  },
  '&:hover .MuiOutlinedInput-notchedOutline': {
    borderColor: '#6366F1',
  },
}));

const DarkButton = styled(Button)(({ theme }) => ({
  background: '#353945',
  color: '#F1F1F1',
  borderRadius: 8,
  '&:hover': {
    background: '#6366F1',
    color: '#fff',
  },
}));

const AddMcpSkillDialog: React.FC<AddMcpSkillDialogProps> = ({
  open,
  onClose,  
  onClosed,
  skill: initialSkill,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const api = useApi();

  const [skill, setSkill] = useState({
    name: '',
    description: '',
    url: '',
    headers: {} as Record<string, string>,
  });
  
  const [existingSkill, setExistingSkill] = useState<typeof skill | null>(null);
  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);

  useEffect(() => {
    if (initialSkill) {
      setSkill({
        name: initialSkill.name,
        description: initialSkill.description,
        url: initialSkill.url,
        headers: initialSkill.headers ?? {},
      });
      
      // Find existing skill in app.mcpTools
      const existingIndex = app.mcpTools?.findIndex(mcp => mcp.name === initialSkill.name) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkill({
        ...initialSkill,
        headers: initialSkill.headers ?? {},
      });
        setExistingSkillIndex(existingIndex);
      }
    } else {
      // Reset form when opening for new skill
      setSkill({
        name: '',
        description: '',
        url: '',
        headers: {},
      });
      setExistingSkill(null);
      setExistingSkillIndex(null);
    }
  }, [initialSkill, open, app.mcpTools]);

  const handleChange = (field: string, value: string) => {
    setSkill((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleUrlChange = (value: string) => {
    if (value && !value.toLowerCase().startsWith('http')) {
      setUrlError('URL must start with http:// or https://');
    } else {
      setUrlError(null);
    }
    
    setSkill((prev) => ({
      ...prev,
      url: value,
    }));
  };

  const handleHeaderChange = (key: string, value: string) => {
    const newHeaders = { ...skill.headers };
    if (value === '') {
      delete newHeaders[key];
    } else {
      newHeaders[key] = value;
    }
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const addHeader = () => {
    const newHeaders = { ...skill.headers, '': '' };
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const removeHeader = (key: string) => {
    const newHeaders = { ...skill.headers };
    delete newHeaders[key];
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const handleSave = async () => {        
    try {
      setError(null);
      
      // Validate URL before saving
      if (!skill.url.toLowerCase().startsWith('http')) {
        setUrlError('URL must start with http:// or https://');
        return;
      }

      // Construct the MCP skill object
      const mcpSkill = {
        name: skill.name,
        description: skill.description,
        url: skill.url,
        headers: skill.headers,
      };

      // Copy app object, has to be deep copy as we have arrays inside
      const appCopy = JSON.parse(JSON.stringify(app));

      // Initialize mcpTools array if it doesn't exist
      if (!appCopy.mcpTools) {
        appCopy.mcpTools = [];
      }

      // Based on index update the app mcpTools array (if set, otherwise add)
      if (existingSkillIndex !== null) {      
        appCopy.mcpTools[existingSkillIndex] = mcpSkill;
      } else {
        appCopy.mcpTools.push(mcpSkill);
      }      

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      console.log(err);
      // Convert to axios error
      const axiosError = err as AxiosError;   
      // If we have response, then show err.response.data, otherwise show err.message      
      const errMessage = axiosError.response?.data ? JSON.stringify(axiosError.response.data) : axiosError.message || 'Failed to save skill';
      
      setError(errMessage);
    }
  };

  const handleDisable = async () => {
    if (existingSkillIndex !== null) {
      // Remove the skill from mcpTools
      app.mcpTools = app.mcpTools?.filter((_, index) => index !== existingSkillIndex);
      await onUpdate(app);
    }
    onClose();
  };

  const handleClose = () => {
    onClose();
  };

  const renderDescriptionWithLinks = (text: string) => {
    // URL regex pattern
    const urlRegex = /(https?:\/\/[^\s]+)/g;
    
    // First split by newlines
    const lines = text.split('\n');
    
    return lines.map((line, lineIndex) => {
      // Then split each line by URLs
      const parts = line.split(urlRegex);
      
      const elements = parts.map((part, partIndex) => {
        if (part.match(urlRegex)) {
          return (
            <a
              key={`${lineIndex}-${partIndex}`}
              href={part}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: '#6366F1', textDecoration: 'underline' }}
            >
              {part}
            </a>
          );
        }
        return part;
      });

      // Add line break after each line except the last one
      return (
        <React.Fragment key={lineIndex}>
          {elements}
          {lineIndex < lines.length - 1 && <br />}
        </React.Fragment>
      );
    });
  };

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      PaperProps={{
        sx: {
          height: '90vh',
          maxHeight: '800px',
          minHeight: '600px'
        }
      }}
      TransitionProps={{
        onExited: () => {
          setSkill({
            name: '',
            description: '',
            url: '',
            headers: {},
          });
          setExistingSkill(null);
          setExistingSkillIndex(null);
          setUrlError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={{ ...lightTheme.scrollbar, height: '100%', display: 'flex', flexDirection: 'column' }}>
        <Box sx={{ 
          mt: 2,
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden'
        }}>
          
          <Box sx={{ flex: 1, overflow: 'auto' }}>
            <Box>
              <NameTypography>
                {skill.name || 'New MCP Skill'}
              </NameTypography>
              <DescriptionTypography>
                {renderDescriptionWithLinks(skill.description || 'No description provided.')}
              </DescriptionTypography>

              <SectionCard>
                <DarkTextField
                  fullWidth
                  label="Name"
                  value={skill.name}
                  helperText="The name of the MCP skill, make it informative and unique for the AI"
                  onChange={(e) => handleChange('name', e.target.value)}
                  margin="normal"
                  required
                />
                <DarkTextField
                  fullWidth
                  label="Description"
                  helperText="A short description of the MCP skill, make it informative and unique for the AI"
                  value={skill.description}
                  onChange={(e) => handleChange('description', e.target.value)}
                  margin="normal"
                  required
                  multiline
                  rows={2}
                />
                <DarkTextField
                  fullWidth
                  label="MCP Server URL"
                  helperText={urlError || "The URL of the MCP server to connect to"}
                  value={skill.url}
                  onChange={(e) => handleUrlChange(e.target.value)}
                  margin="normal"
                  required
                  error={!!urlError}
                  sx={{
                    '& .MuiFormHelperText-root': {
                      color: urlError ? '#EF4444' : '#A0AEC0',
                      fontSize: '0.875rem',
                      marginTop: '4px',
                    },
                    '& .MuiOutlinedInput-root': {
                      '&.Mui-error': {
                        '& fieldset': {
                          borderColor: '#EF4444',
                        },
                      },
                    },
                  }}
                />

                <Box sx={{ mt: 3 }}>
                  <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                    Headers Configuration
                  </Typography>
                  <Tooltip title="Add additional headers that will be sent to the MCP server">
                    <Typography variant="subtitle2" sx={{ mb: 1, color: '#A0AEC0' }}>
                      Headers
                    </Typography>
                  </Tooltip>
                  <List>
                    {Object.entries(skill.headers).map(([key, value], index) => (
                      <ListItem key={`header-${index}`} sx={{ px: 0 }}>
                        <Grid container spacing={1}>
                          <Grid item xs={5}>
                            <DarkTextField
                              size="small"
                              placeholder="Header Name"
                              value={key}
                              onChange={(e) => {
                                const newHeaders = { ...skill.headers };
                                delete newHeaders[key];
                                newHeaders[e.target.value] = value;
                                setSkill((prev) => ({
                                  ...prev,
                                  headers: newHeaders,
                                }));
                              }}
                              fullWidth
                            />
                          </Grid>
                          <Grid item xs={5}>
                            <DarkTextField
                              size="small"
                              placeholder="Header Value"
                              value={value}
                              onChange={(e) => handleHeaderChange(key, e.target.value)}
                              fullWidth
                            />
                          </Grid>
                          <Grid item xs={2}>
                            <IconButton
                              size="small"
                              onClick={() => removeHeader(key)}
                              sx={{ color: '#F87171' }}
                            >
                              <DeleteIcon />
                            </IconButton>
                          </Grid>
                        </Grid>
                      </ListItem>
                    ))}
                  </List>
                  <Button
                    startIcon={<AddIcon />}
                    onClick={addHeader}
                    size="small"
                    sx={{ mt: 1 }}
                  >
                    Add Header
                  </Button>
                </Box>
              </SectionCard>
            </Box>
          </Box>
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
          {/* Add spacer here */}
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
              {existingSkill && (
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
                onClick={handleSave}
                size="small"
                variant="outlined"
                color="secondary"
                disabled={!skill.name.trim() || !skill.description.trim() || !skill.url.trim()}
              >
                {existingSkill ? 'Save' : 'Enable'}
              </Button>
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddMcpSkillDialog;
