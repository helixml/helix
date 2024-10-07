import React, { useState, useCallback } from 'react';
import { v4 as uuidv4 } from 'uuid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Grid from '@mui/material/Grid';
import AddIcon from '@mui/icons-material/Add';
import { ITool } from '../../types';
import Link from '@mui/material/Link';
import Window from '../widgets/Window';
import DeleteIcon from '@mui/icons-material/Delete';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemText from '@mui/material/ListItemText';
import ListItemIcon from '@mui/material/ListItemIcon';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';

interface ZapierIntegrationsProps {
  tools: ITool[];
  onSaveApiTool: (tool: ITool) => void;
  onDeleteApiTool: (toolId: string) => void;  // Add this line
  isReadOnly: boolean;
}

const ZapierIntegrations: React.FC<ZapierIntegrationsProps> = ({
  tools,
  onSaveApiTool,
  onDeleteApiTool,  // Add this line
  isReadOnly,
}) => {
  const [editingTool, setEditingTool] = useState<ITool | null>(null);
  const [showErrors, setShowErrors] = useState(false);

  // Move onAddZapierTool function here
  const onAddZapierTool = useCallback(() => {
    const newTool: ITool = {
      id: uuidv4(),
      name: '',
      description: '',
      tool_type: 'zapier',
      global: false,
      config: {
        zapier: {
          api_key: '',
          model: '',
          max_iterations: 4,
        }
      },
      created: new Date().toISOString(),
      updated: new Date().toISOString(),
      owner: '', // You might want to set this from a context or prop
      owner_type: 'user',
    };

    setEditingTool(newTool);
  }, []);

  const handleEditTool = (tool: ITool) => {
    setEditingTool(tool);
  };

  const handleSaveTool = () => {
    if (isReadOnly || !editingTool) return;
    if (!validate()) {
      setShowErrors(true);
      return;
    }
    setShowErrors(false);
    onSaveApiTool(editingTool);
    setEditingTool(null);
  };

  const validate = () => {
    if (!editingTool) return false;
    if (!editingTool.name) return false;
    if (!editingTool.description) return false;
    if (!editingTool.config.zapier?.api_key) return false;
    if (!editingTool.config.zapier?.model) return false;
    return true;
  };

  const updateEditingTool = (updates: Partial<ITool>) => {
    if (editingTool) {
      setEditingTool({ ...editingTool, ...updates });
    }
  };

  const updateZapierConfig = (updates: Partial<ITool['config']['zapier']>) => {
    if (editingTool && editingTool.config.zapier) {
      updateEditingTool({
        config: {
          ...editingTool.config,
          zapier: { ...editingTool.config.zapier, ...updates },
        },
      });
    }
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        Zapier Integrations
      </Typography>
      <Typography variant="body1" sx={{ mt: 1, mb: 0, fontSize: 14 }}>
        Zapier integration allows you to use Zapier actions in your Helix chat and apps. To begin:
      </Typography>
      <List dense>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText 
            primary={
              <Typography variant="body2">
                Register to <Link href="https://zapier.com/" target="_blank" rel="noopener noreferrer">Zapier</Link> and connect the apps you want to use.
              </Typography>
            } 
          />
        </ListItem>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText 
            primary={
              <Typography variant="body2">
                Visit <Link href="https://actions.zapier.com/credentials/" target="_blank" rel="noopener noreferrer">https://actions.zapier.com/credentials/</Link> and get your API key.
              </Typography>
            } 
          />
        </ListItem>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText 
            primary={
              <Typography variant="body2">
                Use Zapier <Link href="https://actions.zapier.com/providers/" target="_blank" rel="noopener noreferrer">Providers</Link> to enable actions that will be available to Helix.
              </Typography>
            } 
          />
        </ListItem>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText 
            primary={
              <Typography variant="body2">
                Click "Add Zapier Integration" below to add your API key. Give it a description so that Helix can decide when to use it.
              </Typography>
            } 
          />
        </ListItem>
      </List>
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={onAddZapierTool}
        sx={{ mb: 2 }}
        disabled={isReadOnly}
      >
        Add Zapier Integration
      </Button>
      <Box sx={{ mb: 2, overflowY: 'auto' }}>
        {tools.filter(tool => tool.tool_type === 'zapier').map((apiTool) => (
          <Box
            key={apiTool.id}
            sx={{
              p: 2,
              border: '1px solid #303047',
              mb: 2,
            }}
          >
            <Typography variant="h6">{apiTool.name}</Typography>
            <Typography variant="subtitle2" sx={{ mt: 2 }}>Description: {apiTool.description}</Typography>          
            
            <Box sx={{ mt: 1 }}>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button
                  variant="outlined"
                  onClick={() => handleEditTool(apiTool)}
                  sx={{ mr: 1 }}
                  disabled={isReadOnly}
                >
                  Edit
                </Button>
                <Button
                  variant="outlined"
                  color="error"
                  onClick={() => onDeleteApiTool(apiTool.id)}
                  disabled={isReadOnly}
                  startIcon={<DeleteIcon />}
                >
                  Delete
                </Button>
              </Box>
            </Box>
          </Box>
        ))}
      </Box>
      {editingTool && (
        <Window
          title={`${editingTool.id ? 'Edit' : 'Add'} API tool`}          
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
          onSubmit={handleSaveTool}

        >          
          <Box sx={{ p: 2 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Zapier Integration
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.name}
                  onChange={(e) => updateEditingTool({ name: e.target.value })}
                  label="Name"
                  fullWidth
                  error={showErrors && !editingTool.name}
                  helperText={showErrors && !editingTool.name ? 'Please enter a name' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.description}
                  onChange={(e) => updateEditingTool({ description: e.target.value })}
                  label="Description"
                  fullWidth
                  error={showErrors && !editingTool.description}
                  helperText={showErrors && !editingTool.description ? "Description is required" : ""}
                  disabled={isReadOnly}
                />
                <Typography variant="body2" color="textSecondary" sx={{ mt: 1, mb: 2 }}>
                  Based on the description, Helix will decide when to use this integration. Be concise but descriptive.
                </Typography>
              </Grid>
              <Grid item xs={12}>         
                <TextField
                  value={editingTool.config.zapier?.api_key}
                  onChange={(e) => updateZapierConfig({ api_key: e.target.value })}
                  label="API Key"
                  fullWidth
                  error={showErrors && !editingTool.config.zapier?.api_key}
                  helperText={showErrors && !editingTool.config.zapier?.api_key ? 'Please enter Zapier API Key' : ''}
                  disabled={isReadOnly}
                />
                <Typography variant="body2" color="textSecondary" sx={{ mt: 1, mb: 2 }}>
                  To get your API key, register to Zapier and visit <Link href="https://actions.zapier.com/credentials/" target="_blank" rel="noopener noreferrer">https://actions.zapier.com/credentials/</Link>.
                </Typography>
              </Grid>
              <Grid item xs={12}>                
                <TextField
                  value={editingTool.config.zapier?.model}
                  onChange={(e) => updateZapierConfig({ model: e.target.value })}
                  fullWidth                  
                  label="Model"
                  error={showErrors && !editingTool.config.zapier?.model}
                  helperText={showErrors && !editingTool.config.zapier?.model ? "Please enter a model" : ""}
                  disabled={isReadOnly}
                />

                <Typography variant="body2" color="textSecondary" sx={{ mt: 1, mb: 2 }}>
                  Use strong models for complex tasks. GPT-4o, mistralai/Mixtral-8x7B-Instruct-v0.1, etc.
                </Typography>

              </Grid>              
              <Grid item xs={12}>
                <TextField
                  value={editingTool.config.zapier?.max_iterations}
                  onChange={(e) => updateZapierConfig({ max_iterations: parseInt(e.target.value, 10) })}
                  fullWidth                  
                  label="Max Iterations"
                  error={showErrors && !editingTool.config.zapier?.max_iterations}
                  helperText={showErrors && !editingTool.config.zapier?.max_iterations ? "Please enter a max iterations" : ""}
                  disabled={isReadOnly}
                />
                <Typography variant="body2" color="textSecondary" sx={{ mt: 1, mb: 2 }}>
                  Zapier integration can perform multiple iterations to solve a task. Normally 1-3 is good. Set it to more
                  for complex tasks.
                </Typography>
              </Grid>              
            </Grid>
          </Box>
        </Window>
      )}
    </Box>
  );
};

export default ZapierIntegrations;