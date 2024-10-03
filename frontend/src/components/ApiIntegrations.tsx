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
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import { coindeskSchema } from './coindesk_schema';
import { jobVacanciesSchema } from './jobvacancies_schema';
import DeleteIcon from '@mui/icons-material/Delete';

interface ApiIntegrationsProps {
  tools: ITool[];
  onSaveApiTool: (tool: ITool) => void;
  onDeleteApiTool: (toolId: string) => void;  // Add this line
  isReadOnly: boolean;
}

const ApiIntegrations: React.FC<ApiIntegrationsProps> = ({
  tools,
  onSaveApiTool,
  onDeleteApiTool,  // Add this line
  isReadOnly,
}) => {
  const [editingTool, setEditingTool] = useState<ITool | null>(null);
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);
  const [schemaTemplate, setSchemaTemplate] = useState<string>('');

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

  const handleSchemaTemplateChange = (selectedTemplate: string) => {
    setSchemaTemplate(selectedTemplate);

    if (selectedTemplate === 'coindesk') {
      updateEditingTool({
        name: "CoinDesk API",
        description: "API for CoinDesk",
      });
      updateApiConfig({        
        schema: coindeskSchema,
        url: "https://api.coindesk.com/v1"
      });
    } else if (selectedTemplate === 'jobvacancies') {
      updateEditingTool({
        name: "Job Vacancies API",
        description: "API for job vacancies",
      });
      updateApiConfig({        
        schema: jobVacanciesSchema,
        url: "https://demos.tryhelix.ai"
      });
    }
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        API Tools
      </Typography>
      <Typography variant="body2" color="textSecondary" sx={{ mt: 1, mb: 2 }}>
        Allow Helix to call any 3rd party API to perform tasks such as querying information or updating data. To begin:
        <ul>    
          <li>Find the OpenAPI schema for the API you want to use.</li>      
          <li>Click "Add API Tool" below to add your schema, URL and any additional config such as authentication headers.</li>
        </ul>
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
      <Box sx={{ mb: 2, overflowY: 'auto' }}>
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
            <Typography variant="subtitle2" sx={{ mt: 2 }}>Description: {apiTool.description}</Typography>
            
            {apiTool.config.api?.actions && apiTool.config.api.actions.length > 0 && (
              <Box sx={{ mt: 1 }}>
                <Typography variant="subtitle2">Actions:</Typography>
                <ul>
                  {apiTool.config.api.actions.map((action, index) => (
                    <li key={index}>
                      {action.name}: {action.method} {action.path}
                    </li>
                  ))}
                </ul>
              </Box>
            )}
            
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
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
          onSubmit={handleSaveTool}

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
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel id="schema-template-label">Example Schemas</InputLabel>
                  <Select
                    labelId="schema-template-label"
                    value={schemaTemplate}
                    onChange={(e) => {
                      handleSchemaTemplateChange(e.target.value);
                    }}
                    disabled={isReadOnly}
                  >
                    <MenuItem value="custom">
                      <em>Custom</em>
                    </MenuItem>
                    <MenuItem value="coindesk">CoinDesk</MenuItem>
                    <MenuItem value="jobvacancies">Job Vacancies</MenuItem>
                  </Select>
                </FormControl>
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
                  entityTitle="header"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <StringMapEditor
                  data={editingTool.config.api?.query || {}}
                  onChange={(query) => updateApiConfig({ query })}
                  entityTitle="query parameter"
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
                    <TextField
                      label="Description"
                      value={action.description}
                      onChange={(e) => {
                        const newActions = [...(editingTool.config.api?.actions || [])];
                        newActions[index].description = e.target.value;
                        updateApiConfig({ actions: newActions });
                      }}
                      disabled={isReadOnly}
                      sx={{ mr: 2 }}
                    />
                    <TextField
                      label="Method"
                      value={action.method}
                      onChange={(e) => {
                        const newActions = [...(editingTool.config.api?.actions || [])];
                        newActions[index].method = e.target.value;
                        updateApiConfig({ actions: newActions });
                      }}
                      disabled={isReadOnly}
                      sx={{ mr: 2 }}
                    />
                    <TextField
                      label="Path"
                      value={action.path}
                      onChange={(e) => {
                        const newActions = [...(editingTool.config.api?.actions || [])];
                        newActions[index].path = e.target.value;
                        updateApiConfig({ actions: newActions });
                      }}
                      disabled={isReadOnly}
                    />
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