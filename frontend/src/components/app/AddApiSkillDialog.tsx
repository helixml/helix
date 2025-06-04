import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,  
  IconButton,
  List,
  ListItem,
  ListItemSecondaryAction,
  Link,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { IAgentSkill, IRequiredApiParameter, IAppFlatState, IAssistantApi } from '../../types';
import { styled } from '@mui/material/styles';

interface AddApiSkillDialogProps {
  open: boolean;
  onClose: () => void;
  skill?: IAgentSkill;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

// Styled components for dark theme
const DarkDialog = styled(Dialog)(({ theme }) => ({
  '& .MuiPaper-root': {
    background: '#181A20',
    color: '#F1F1F1',
    borderRadius: 16,
    boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
  },
}));

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

const AddApiSkillDialog: React.FC<AddApiSkillDialogProps> = ({
  open,
  onClose,  
  skill: initialSkill,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {

  const [skill, setSkill] = useState<IAgentSkill>({
    name: '',
    description: '',
    systemPrompt: '',
    apiSkill: {
      schema: '',
      url: '',
      requiredParameters: [],
    },
    configurable: true,
  });
  
  const [existingSkill, setExistingSkill] = useState<IAgentSkill | null>(null);
  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [parameterValues, setParameterValues] = useState<Record<string, string>>({});

  useEffect(() => {
    if (initialSkill) {
      setSkill(initialSkill);
      // Find existing skill in app.apiTools
      const existingIndex = app.apiTools?.findIndex(tool => tool.name === initialSkill.name) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkill(initialSkill);
        setExistingSkillIndex(existingIndex);
      }
    } else {
      // Reset form when opening for new skill
      setSkill({
        name: '',
        description: '',
        systemPrompt: '',
        apiSkill: {
          schema: '',
          url: '',
          requiredParameters: [],
        },
        configurable: true,
      });
      setExistingSkill(null);
      setExistingSkillIndex(null);
    }
  }, [initialSkill, open, initialIsEnabled, app.apiTools]);

  useEffect(() => {
    if (skill.apiSkill.requiredParameters) {
      const initialValues: Record<string, string> = {};
      
      // If we have an existing skill, try to find its configuration in app.apiTools
      if (existingSkill) {
        const existingTool = app.apiTools?.find(tool => tool.name === existingSkill.name);
        if (existingTool) {
          skill.apiSkill.requiredParameters.forEach(param => {
            if (param.name) {
              // Check if parameter is in query or headers based on its type
              if (param.type === 'header' && existingTool.headers) {
                initialValues[param.name] = existingTool.headers[param.name] || '';
              } else if (param.type === 'query' && existingTool.query) {
                initialValues[param.name] = existingTool.query[param.name] || '';
              } else {
                initialValues[param.name] = '';
              }
            }
          });
        }
      } else {
        // For new skills, just initialize empty values
        skill.apiSkill.requiredParameters.forEach(param => {
          if (param.name && !(param.name in parameterValues)) {
            initialValues[param.name] = '';
          }
        });
      }
      
      setParameterValues(prev => ({ ...initialValues, ...prev }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skill.apiSkill.requiredParameters, existingSkill, app.apiTools]);

  const handleChange = (field: string, value: string) => {
    setSkill((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleApiSkillChange = (field: string, value: string) => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        [field]: value,
      },
    }));
  };

  const addRequiredParameter = () => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: [
          ...prev.apiSkill.requiredParameters,
          {
            name: '',
            description: '',
            type: 'query' as IRequiredApiParameter,
            required: true,
          },
        ],
      },
    }));
  };

  const removeRequiredParameter = (index: number) => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: prev.apiSkill.requiredParameters.filter((_, i) => i !== index),
      },
    }));
  };

  const updateRequiredParameter = (index: number, field: string, value: string | boolean) => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: prev.apiSkill.requiredParameters.map((param, i) =>
          i === index ? { ...param, [field]: value } : param
        ),
      },
    }));
  };  

  const handleParameterValueChange = (name: string, value: string) => {
    setParameterValues(prev => ({ ...prev, [name]: value }));
  };

  // Check if all required parameters have values, used to ensure
  // user can't save the skill without filling all required parameters
  const areAllParametersFilled = () => {            
    return skill.apiSkill.requiredParameters.every(param => {
      if (!param.required) return true;
      return parameterValues[param.name]?.trim() !== '';
    });
  };

  const handleSave = async () => {    
    console.log('skill config: ', skill);

    // Construct the IAssistantApi object, which will be used 
    // to update the application
    const assistantApi: IAssistantApi = {
      name: skill.name,
      description: skill.description,
      system_prompt: skill.systemPrompt,
      schema: skill.apiSkill.schema,
      url: skill.apiSkill.url,      
      headers: {},
      query: {},
    };

    // Go through required parameters based on parameter type add it to either
    // header or query
    skill.apiSkill.requiredParameters.forEach(param => {
      if (param.type === 'header') {
        assistantApi.headers![param.name] = parameterValues[param.name];
      } else {
        assistantApi.query![param.name] = parameterValues[param.name];
      }
    });

    // Based on index update the app api tools array (if set, otherwise add)
    if (existingSkillIndex !== null) {
      app.apiTools![existingSkillIndex] = assistantApi;
    } else {
      app.apiTools!.push(assistantApi);
    }

    // Update the application
    await onUpdate(app);

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
            <Link
              key={`${lineIndex}-${partIndex}`}
              href={part}
              target="_blank"
              rel="noopener noreferrer"
              sx={{ color: '#6366F1', textDecoration: 'underline' }}
            >
              {part}
            </Link>
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
    <DarkDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            {skill.name || 'New API Skill'}
          </NameTypography>
          <DescriptionTypography>
            {renderDescriptionWithLinks(skill.description || 'No description provided.')}
          </DescriptionTypography>

          {skill.configurable && (
            <SectionCard>
              <DarkTextField
                fullWidth
                label="Name"
                value={skill.name}
                onChange={(e) => handleChange('name', e.target.value)}
                margin="normal"
                required
              />
              <DarkTextField
                fullWidth
                label="Description"
                value={skill.description}
                onChange={(e) => handleChange('description', e.target.value)}
                margin="normal"
                required
                multiline
                rows={2}
              />
              <DarkTextField
                fullWidth
                label="System Prompt"
                value={skill.systemPrompt}
                onChange={(e) => handleChange('systemPrompt', e.target.value)}
                margin="normal"
                required
                multiline
                rows={4}
              />
              <DarkTextField
                fullWidth
                label="URL"
                value={skill.apiSkill.url}
                onChange={(e) => handleApiSkillChange('url', e.target.value)}
                margin="normal"
                required
              />
              <DarkTextField
                fullWidth
                label="Schema"
                value={skill.apiSkill.schema}
                onChange={(e) => handleApiSkillChange('schema', e.target.value)}
                margin="normal"
                required
                multiline
                rows={10}
              />
            </SectionCard>
          )}

          {skill.apiSkill.requiredParameters.length > 0 && (
            <SectionCard>
              <Typography variant="h6" gutterBottom sx={{ color: '#F8FAFC' }}>
                Settings
              </Typography>
              <List>
                {skill.apiSkill.requiredParameters.map((param, index) => (
                  <ListItem key={index} alignItems="flex-start" sx={{ background: '#181A20', borderRadius: 2, mb: 1 }}>
                    <Box sx={{ flex: 1, mb: 2 }}>
                      <Typography variant="subtitle2" sx={{ mb: 0.5, color: '#F1F1F1' }}>
                        {param.name}
                      </Typography>
                      <Typography variant="caption" color="#A0AEC0" sx={{ mb: 1, display: 'block' }}>
                        {renderDescriptionWithLinks(param.description)}
                      </Typography>
                      <DarkTextField                      
                        value={parameterValues[param.name] || ''}
                        onChange={e => handleParameterValueChange(param.name, e.target.value)}
                        size="small"
                        fullWidth
                        required={param.required}
                        sx={{ mt: 0.5 }}
                      />
                    </Box>
                    {param.required === false && skill.configurable && (
                      <ListItemSecondaryAction>
                        <IconButton
                          edge="end"
                          aria-label="delete"
                          onClick={() => removeRequiredParameter(index)}
                          sx={{ color: '#F87171' }}
                        >
                          <DeleteIcon />
                        </IconButton>
                      </ListItemSecondaryAction>
                    )}
                  </ListItem>
                ))}
              </List>
              {skill.configurable && (
                <DarkButton
                  startIcon={<AddIcon />}
                  onClick={addRequiredParameter}
                  variant="outlined"
                  size="small"
                  sx={{ mt: 1, borderColor: '#353945' }}
                >
                  Add Parameter
                </DarkButton>
              )}
            </SectionCard>
          )}
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F' }}>
        <Button 
          onClick={onClose} 
          size="small"
          variant="outlined"
          color="primary"
        >
          Cancel
        </Button>
        <Button
          onClick={handleSave}
          size="small"
          variant="outlined"
          color="secondary"
          disabled={!areAllParametersFilled()}
        >
          {existingSkill ? 'Save' : 'Enable'}
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddApiSkillDialog;
