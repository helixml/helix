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
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  Switch,
  FormControlLabel,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { IAgentSkill, IRequiredApiParameter, IAppFlatState, IAssistantApi } from '../../types';

interface AddApiSkillDialogProps {
  open: boolean;
  onClose: () => void;
  skill?: IAgentSkill;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

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
      skill.apiSkill.requiredParameters.forEach(param => {
        if (param.name && !(param.name in parameterValues)) {
          initialValues[param.name] = '';
        }
      });
      setParameterValues(prev => ({ ...initialValues, ...prev }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skill.apiSkill.requiredParameters]);

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

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        {existingSkill ? 'Edit API Skill' : 'Add API Skill'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          {skill.configurable ? (
            <TextField
              fullWidth
              label="Name"
              value={skill.name}
              onChange={(e) => handleChange('name', e.target.value)}
              margin="normal"
              required
            />
          ) : (
            <Typography variant="h6" sx={{ mt: 2, mb: 1 }}>
              {skill.name}
            </Typography>
          )}

          {skill.configurable ? (
            <TextField
              fullWidth
              label="Description"
              value={skill.description}
              onChange={(e) => handleChange('description', e.target.value)}
              margin="normal"
              required
              multiline
              rows={2}
            />
          ) : (
            <Typography variant="body1" sx={{ mb: 2 }}>
              {skill.description}
            </Typography>
          )}

          {skill.configurable && (
            <TextField
              fullWidth
              label="System Prompt"
              value={skill.systemPrompt}
              onChange={(e) => handleChange('systemPrompt', e.target.value)}
              margin="normal"
              required
              multiline
              rows={4}
            />
          )}

          {skill.configurable && (
            <>
              <TextField
                fullWidth
                label="URL"
                value={skill.apiSkill.url}
                onChange={(e) => handleApiSkillChange('url', e.target.value)}
                margin="normal"
                required
              />
              <TextField
                fullWidth
                label="Schema"
                value={skill.apiSkill.schema}
                onChange={(e) => handleApiSkillChange('schema', e.target.value)}
                margin="normal"
                required
                multiline
                rows={10}
              />
            </>
          )}

          <Box sx={{ mt: 2 }}>
            <Typography variant="h6" gutterBottom>
              Required Parameters
            </Typography>
            <List>
              {skill.apiSkill.requiredParameters.map((param, index) => (
                <ListItem key={index} alignItems="flex-start">
                  <Box sx={{ flex: 1 }}>
                    <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                      {param.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                      {param.description}
                    </Typography>
                    <TextField
                      label={`Enter value for ${param.name}`}
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
                      >
                        <DeleteIcon />
                      </IconButton>
                    </ListItemSecondaryAction>
                  )}
                </ListItem>
              ))}
            </List>
            {skill.configurable && (
              <Button
                startIcon={<AddIcon />}
                onClick={addRequiredParameter}
                variant="outlined"
                size="small"
                sx={{ mt: 1 }}
              >
                Add Parameter
              </Button>
            )}
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button 
          onClick={handleSave} 
          variant="outlined" 
          color="secondary"
          disabled={!areAllParametersFilled()}
        >
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddApiSkillDialog;
