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
import { IAssistantApi } from '../../types';
import Window from '../widgets/Window';
import StringMapEditor from '../widgets/StringMapEditor';
import ClickLink from '../widgets/ClickLink';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import { coindeskSchema } from './coindesk_schema';
import { jobVacanciesSchema } from './jobvacancies_schema';
import DeleteIcon from '@mui/icons-material/Delete';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemText from '@mui/material/ListItemText';
import EditIcon from '@mui/icons-material/Edit';
import ListItemIcon from '@mui/material/ListItemIcon';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';

interface ApiIntegrationsProps {
  apis: IAssistantApi[];
  onSaveApiTool: (tool: IAssistantApi, index?: number) => void;
  onDeleteApiTool: (toolId: string) => void;
  isReadOnly: boolean;
  onEdit: (tool: IAssistantApi, index: number) => void;
}

const ApiIntegrations: React.FC<ApiIntegrationsProps> = ({
  apis,
  onSaveApiTool,
  onDeleteApiTool,
  isReadOnly,
  onEdit
}) => {
  const [editingTool, setEditingTool] = useState<IAssistantApi | null>(null);
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);
  const [schemaTemplate, setSchemaTemplate] = useState<string>('');

  const onAddApiTool = useCallback(() => {
    const newTool: IAssistantApi = {
      name: '',
      description: '',
      schema: '',
      url: '',
      headers: {},
      query: {},
    };
    setEditingTool(newTool);
  }, []);

  const validate = () => {
    if (!editingTool) return false;
    if (!editingTool.name) return false;
    if (!editingTool.description) return false;
    if (!editingTool.url) return false;
    if (!editingTool.schema) return false;
    return true;
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

  const updateEditingTool = (updates: Partial<IAssistantApi>) => {
    if (editingTool) {
      setEditingTool({ ...editingTool, ...updates });
    }
  };

  const handleSchemaTemplateChange = (selectedTemplate: string) => {
    setSchemaTemplate(selectedTemplate);

    if (selectedTemplate === 'coindesk') {
      updateEditingTool({
        name: "CoinDesk API",
        description: "API for CoinDesk",
        schema: coindeskSchema,
        url: "https://api.coindesk.com/v1"
      });
    } else if (selectedTemplate === 'jobvacancies') {
      updateEditingTool({
        name: "Job Vacancies API",
        description: "API for job vacancies",
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
      <Typography variant="body1" sx={{ mt: 1, mb: 0, fontSize: 14 }}>
        Allow Helix to call any 3rd party API to perform tasks such as querying information or updating data. To begin:
      </Typography>
      <List dense>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText primary="Find the OpenAPI schema for the API you want to use." />
        </ListItem>
        <ListItem disableGutters>
          <ListItemIcon sx={{ minWidth: 20 }}>
            <FiberManualRecordIcon sx={{ fontSize: 8 }} />
          </ListItemIcon>
          <ListItemText primary="Click 'Add API Tool' below to add your schema, URL and any additional config such as authentication headers." />
        </ListItem>
      </List>
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
        {apis.map((apiTool, index) => (
          <Box
            key={apiTool.name}
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
                  onClick={() => setEditingTool(apiTool)}
                  sx={{ mr: 1 }}
                  disabled={isReadOnly}
                  startIcon={<EditIcon />}
                >
                  Edit
                </Button>
                <Button
                  variant="outlined"
                  color="error"
                  onClick={() => onDeleteApiTool(apiTool.name)}
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
          title={`${editingTool.name ? 'Edit' : 'Add'} API Tool`}
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
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel id="schema-template-label">Example Schemas</InputLabel>
                  <Select
                    labelId="schema-template-label"
                    value={schemaTemplate}
                    onChange={(e) => handleSchemaTemplateChange(e.target.value)}
                    disabled={isReadOnly}
                  >
                    <MenuItem value="custom">
                      <em>Custom</em>
                    </MenuItem>
                    <MenuItem value="coindesk">CoinDesk</MenuItem>
                    <MenuItem value="jobvacancies">Job Vacancies</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
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
                  value={editingTool.url}
                  onChange={(e) => updateEditingTool({ url: e.target.value })}
                  label="URL"
                  fullWidth
                  error={showErrors && !editingTool.url}
                  helperText={showErrors && !editingTool.url ? 'Please enter a URL' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>                
                <TextField
                  value={editingTool.schema}
                  onChange={(e) => updateEditingTool({ schema: e.target.value })}
                  fullWidth
                  multiline
                  rows={10}
                  label="OpenAPI (Swagger) schema"
                  error={showErrors && !editingTool.schema}
                  helperText={showErrors && !editingTool.schema ? "Please enter a schema" : ""}
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
                  data={editingTool.headers || {}}
                  onChange={(headers) => updateEditingTool({ headers })}
                  entityTitle="header"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <StringMapEditor
                  data={editingTool.query || {}}
                  onChange={(query) => updateEditingTool({ query })}
                  entityTitle="query parameter"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography>Advanced Template Settings</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <TextField
                      value={editingTool.request_prep_template}
                      onChange={(e) => updateEditingTool({ request_prep_template: e.target.value })}
                      label="Request Prep Template"
                      fullWidth
                      multiline
                      rows={4}
                      disabled={isReadOnly}
                      sx={{ mb: 2 }}
                    />
                    <TextField
                      value={editingTool.response_success_template}
                      onChange={(e) => updateEditingTool({ response_success_template: e.target.value })}
                      label="Response Success Template"
                      fullWidth
                      multiline
                      rows={4}
                      disabled={isReadOnly}
                      sx={{ mb: 2 }}
                    />
                    <TextField
                      value={editingTool.response_error_template}
                      onChange={(e) => updateEditingTool({ response_error_template: e.target.value })}
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

      {showBigSchema && editingTool && (
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
              value={editingTool.schema}
              onChange={(e) => updateEditingTool({ schema: e.target.value })}
              fullWidth
              multiline
              label="OpenAPI (Swagger) schema"
              error={showErrors && !editingTool.schema}
              helperText={showErrors && !editingTool.schema ? "Please enter a schema" : ""}
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