import React, { useState, useCallback } from 'react';
import { v4 as uuidv4 } from 'uuid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Grid from '@mui/material/Grid';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import AddIcon from '@mui/icons-material/Add';
import { ITool, IToolApiAction } from '../types';
import Window from './widgets/Window';
import StringMapEditor from './widgets/StringMapEditor';
import ClickLink from './widgets/ClickLink';

interface ApiIntegrationsProps {
  tools: ITool[];
  onSaveApiTool: (tool: ITool) => void;
  isReadOnly: boolean;
}

const ApiIntegrations: React.FC<ApiIntegrationsProps> = ({
  tools,
  onSaveApiTool,
  isReadOnly,
}) => {
  const [editingTool, setEditingTool] = useState<ITool | null>(null);
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);

  // Move onAddApiTool function here
  const onAddApiTool = useCallback(() => {
    const newTool: ITool = {
      id: uuidv4(),
      name: '',
      description: '',
      tool_type: 'api',
      global: false,
      config: {
        api: {
          url: '',
          schema: '',
          actions: [],
          headers: {},
          query: {},
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
    if (!editingTool.config.api?.url) return false;
    if (!editingTool.config.api?.schema) return false;
    return true;
  };

  const updateEditingTool = (updates: Partial<ITool>) => {
    if (editingTool) {
      setEditingTool({ ...editingTool, ...updates });
    }
  };

  const updateApiConfig = (updates: Partial<ITool['config']['api']>) => {
    if (editingTool && editingTool.config.api) {
      updateEditingTool({
        config: {
          ...editingTool.config,
          api: { ...editingTool.config.api, ...updates },
        },
      });
    }
  };

  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        API Tools
      </Typography>
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={onAddApiTool}
        sx={{ mb: 2 }}
        disabled={isReadOnly}
      >
        Add API Tool
      </Button>
      <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
        {tools.filter(tool => tool.tool_type === 'api').map((apiTool) => (
          <Box
            key={apiTool.id}
            sx={{
              p: 2,
              border: '1px solid #303047',
              mb: 2,
            }}
          >
            <Typography variant="h6">{apiTool.name}</Typography>
            <Typography variant="body1">{apiTool.description}</Typography>
            <Button
              variant="outlined"
              onClick={() => handleEditTool(apiTool)}
              sx={{ mt: 1 }}
              disabled={isReadOnly}
            >
              Edit
            </Button>
          </Box>
        ))}
      </Box>
      {editingTool && (
        <Window
          title={`${editingTool.id ? 'Edit' : 'Add'} API tool`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
        >
          <Box sx={{ p: 2 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              API Tool
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
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.config.api?.url}
                  onChange={(e) => updateApiConfig({ url: e.target.value })}
                  label="URL"
                  fullWidth
                  error={showErrors && !editingTool.config.api?.url}
                  helperText={showErrors && !editingTool.config.api?.url ? 'Please enter a URL' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.config.api?.schema}
                  onChange={(e) => updateApiConfig({ schema: e.target.value })}
                  fullWidth
                  multiline
                  rows={10}
                  label="OpenAPI (Swagger) schema"
                  error={showErrors && !editingTool.config.api?.schema}
                  helperText={showErrors && !editingTool.config.api?.schema ? "Please enter a schema" : ""}
                  disabled={isReadOnly}
                />
                <Box sx={{ textAlign: 'right', mb: 1 }}>
                  <ClickLink onClick={() => setShowBigSchema(true)}>
                    expand schema
                  </ClickLink>
                </Box>
              </Grid>
              <Grid item xs={12}>
                <StringMapEditor
                  data={editingTool.config.api?.headers || {}}
                  onChange={(headers) => updateApiConfig({ headers })}
                  entityTitle="headers"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <StringMapEditor
                  data={editingTool.config.api?.query || {}}
                  onChange={(query) => updateApiConfig({ query })}
                  entityTitle="query parameters"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                {editingTool.config.api?.actions?.map((action, index) => (
                  <Box key={index} sx={{ mb: 2 }}>
                    <TextField
                      label="Name"
                      value={action.name}
                      onChange={(e) => {
                        const newActions = [...(editingTool.config.api?.actions || [])];
                        newActions[index].name = e.target.value;
                        updateApiConfig({ actions: newActions });
                      }}
                      disabled={isReadOnly}
                      sx={{ mr: 2 }}
                    />
                    // ... other action fields (description, method, path) ...
                  </Box>
                ))}
              </Grid>
              <Grid item xs={12}>
                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography>Advanced Template Settings</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <TextField
                      value={editingTool.config.api?.request_prep_template}
                      onChange={(e) => updateApiConfig({ request_prep_template: e.target.value })}
                      label="Request Prep Template"
                      fullWidth
                      multiline
                      rows={4}
                      disabled={isReadOnly}
                      sx={{ mb: 2 }}
                    />
                    <TextField
                    value={editingTool.config.api?.response_success_template}
                    onChange={(e) => updateApiConfig({ response_success_template: e.target.value })}
                    label="Response Success Template"
                    fullWidth
                    multiline
                    rows={4}
                    disabled={isReadOnly}
                    sx={{ mb: 2 }}
                  />
                  <TextField
                    value={editingTool.config.api?.response_error_template}
                    onChange={(e) => updateApiConfig({ response_error_template: e.target.value })}
                    label="Response Error Template"
                    fullWidth
                    multiline
                    rows={4}
                    disabled={isReadOnly}
                  />
                  </AccordionDetails>
                </Accordion>
              </Grid>
            </Grid>
            <Box sx={{ mt: 2 }}>
              <Button
                variant="contained"
                color="primary"
                onClick={handleSaveTool}
                disabled={isReadOnly}
                sx={{ mr: 2 }}
              >
                Save
              </Button>
              <Button 
                variant="contained" 
                color="secondary" 
                onClick={() => setEditingTool(null)}
              >
                Cancel
              </Button>
            </Box>
          </Box>
        </Window>
      )}
      {showBigSchema && (
        <Window
          title="Schema"
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setShowBigSchema(false)}
        >
          <Box sx={{ p: 2, height: '100%' }}>
            <TextField
              value={editingTool?.config.api?.schema}
              onChange={(e) => updateApiConfig({ schema: e.target.value })}
              fullWidth
              multiline
              label="OpenAPI (Swagger) schema"
              error={showErrors && !editingTool?.config.api?.schema}
              helperText={showErrors && !editingTool?.config.api?.schema ? "Please enter a schema" : ""}
              sx={{ height: '100%' }}
              disabled={isReadOnly}
            />
          </Box>
        </Window>
      )}
    </Box>
  );
};

export default ApiIntegrations;