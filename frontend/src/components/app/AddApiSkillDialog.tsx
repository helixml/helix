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
import { IAgentSkill, IRequiredApiParameter, IAppFlatState } from '../../types';

interface AddApiSkillDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (skill: IAgentSkill) => void;
  skill?: IAgentSkill;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const AddApiSkillDialog: React.FC<AddApiSkillDialogProps> = ({
  open,
  onClose,
  onSave,
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

  const [enabled, setEnabled] = useState(initialIsEnabled);

  useEffect(() => {
    if (initialSkill) {
      setSkill(initialSkill);
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
    }
    setEnabled(initialIsEnabled);
  }, [initialSkill, open, initialIsEnabled]);

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

  const handleSkillToggle = async (enabled: boolean) => {
    const currentTools = app.apiTools || [];
    let updatedTools;

    if (enabled) {
      // If enabling, add the skill with its configuration
      updatedTools = [...currentTools, {
        name: skill.name,
        description: skill.description,
        schema: skill.apiSkill.schema,
        url: skill.apiSkill.url,
      }];
    } else {
      // If disabling, remove the skill
      updatedTools = currentTools.filter(tool => tool.name !== skill.name);
    }

    if (updatedTools) {
      await onUpdate({
        ...app,
        apiTools: updatedTools,
      });
    }
  };

  const handleSave = async () => {
    if (enabled) {
      await handleSkillToggle(true);
    } else {
      await handleSkillToggle(false);
    }
    onClose();
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        {initialSkill ? 'Edit API Skill' : 'Add API Skill'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <FormControlLabel
            control={
              <Switch
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                color="primary"
              />
            }
            label="Enable Skill"
            sx={{ mb: 2 }}
          />
          
          <TextField
            fullWidth
            label="Name"
            value={skill.name}
            onChange={(e) => handleChange('name', e.target.value)}
            margin="normal"
            required
          />
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
                <ListItem key={index}>
                  <ListItemText
                    primary={
                      <Box sx={{ display: 'flex', gap: 2 }}>
                        <TextField
                          label="Name"
                          value={param.name}
                          onChange={(e) => updateRequiredParameter(index, 'name', e.target.value)}
                          size="small"
                          required
                        />
                        <TextField
                          label="Description"
                          value={param.description}
                          onChange={(e) => updateRequiredParameter(index, 'description', e.target.value)}
                          size="small"
                          required
                        />
                        <FormControl size="small" sx={{ minWidth: 120 }}>
                          <InputLabel>Type</InputLabel>
                          <Select
                            value={param.type}
                            label="Type"
                            onChange={(e) => updateRequiredParameter(index, 'type', e.target.value)}
                          >
                            <MenuItem value="query">Query</MenuItem>
                            <MenuItem value="header">Header</MenuItem>
                          </Select>
                        </FormControl>
                      </Box>
                    }
                  />
                  <ListItemSecondaryAction>
                    <IconButton
                      edge="end"
                      aria-label="delete"
                      onClick={() => removeRequiredParameter(index)}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              ))}
            </List>
            <Button
              startIcon={<AddIcon />}
              onClick={addRequiredParameter}
              variant="outlined"
              size="small"
              sx={{ mt: 1 }}
            >
              Add Parameter
            </Button>
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave} variant="contained" color="primary">
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddApiSkillDialog;
