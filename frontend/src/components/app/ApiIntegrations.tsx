import React, { useState, useCallback, useEffect } from 'react';
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
import { IAppFlatState, IAssistantApi, ITool } from '../../types';
import Window from '../widgets/Window';
import StringMapEditor from '../widgets/StringMapEditor';
import ClickLink from '../widgets/ClickLink';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import DeleteIcon from '@mui/icons-material/Delete';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemText from '@mui/material/ListItemText';
import EditIcon from '@mui/icons-material/Edit';
import ListItemIcon from '@mui/material/ListItemIcon';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';

import Avatar from '@mui/material/Avatar';
import IconButton from '@mui/material/IconButton';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import useApi from '../../hooks/useApi';
import Link from '@mui/material/Link';


import { jobVacanciesTool } from './examples/jobVacanciesApi';
import { exchangeRatesTool } from './examples/exchangeRatesApi';
import { productsTool } from './examples/productsApi';
import { climateTool } from './examples/climateApi';

interface ApiIntegrationsProps {
  apis: IAssistantApi[];
  tools: ITool[];
  onSaveApiTool: (tool: IAssistantApi, index?: number) => void;
  onDeleteApiTool: (toolIndex: number) => void;
  isReadOnly: boolean;
  app: IAppFlatState,
  onUpdate: (updates: IAppFlatState) => Promise<void>,
}

// Interface for OAuth provider objects from the API
interface OAuthProvider {
  id: string;
  type: string;
  name: string;
  enabled: boolean;
}

const ApiIntegrations: React.FC<ApiIntegrationsProps> = ({
  apis,
  tools,
  onSaveApiTool,
  onDeleteApiTool,
  isReadOnly,
  app,
  onUpdate,
}) => {
  const [editingTool, setEditingTool] = useState<{tool: IAssistantApi, index: number} | null>(null);
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);
  const [schemaTemplate, setSchemaTemplate] = useState<string>('');
  const [oauthProvider, setOAuthProvider] = useState('');
  const [oauthScopes, setOAuthScopes] = useState<string[]>([]);
  const [configuredProviders, setConfiguredProviders] = useState<OAuthProvider[]>([]);
  const [actionableTemplate, setActionableTemplate] = useState(app.is_actionable_template || '');
  
  const [actionableHistoryLength, setActionableHistoryLength] = useState(app.is_actionable_history_length || 0);
  const api = useApi();

  // Initialize local state when app prop changes
  useEffect(() => {
    setActionableTemplate(app.is_actionable_template || '');
    setActionableHistoryLength(app.is_actionable_history_length || 0);
  }, [app]);

  const handleBlur = (field: 'template' | 'history') => {
    // Only update if values have changed
    if (field === 'template' && actionableTemplate !== app.is_actionable_template) {
      onUpdate({...app, is_actionable_template: actionableTemplate});
    } else if (field === 'history' && actionableHistoryLength !== app.is_actionable_history_length) {
      onUpdate({...app, is_actionable_history_length: actionableHistoryLength});
    }
  };

  // Fetch configured OAuth providers from the API
  useEffect(() => {
    const fetchOAuthProviders = async () => {
      try {
        const providers = await api.get('/api/v1/oauth/providers');
        const enabledProviders = Array.isArray(providers) 
          ? providers.filter((p: OAuthProvider) => p.enabled)
          : [];
        setConfiguredProviders(enabledProviders);
      } catch (error) {
        console.error('Error fetching OAuth providers:', error);
        setConfiguredProviders([]);
      }
    };

    fetchOAuthProviders();
  }, []);

  const onAddApiTool = useCallback(() => {
    const newTool: IAssistantApi = {
      name: '',
      description: '',
      system_prompt: '',
      schema: '',
      url: '',
      headers: {},
      query: {},
    };
    setEditingTool({tool: newTool, index: -1});
    setOAuthProvider('');
    setOAuthScopes([]);
  }, []);

  const validate = () => {
    if (!editingTool) return false;
    if (!editingTool.tool.name) return false;
    if (!editingTool.tool.description) return false;
    if (app.agent_mode && !editingTool.tool.system_prompt) return false;
    if (!editingTool.tool.url) return false;
    if (!editingTool.tool.schema) return false;
    return true;
  };

  const handleEditTool = (apiTool: IAssistantApi, index: number) => {
    console.log('ApiIntegrations - editing tool at index:', index);
    
    // Look for OAuth settings directly on the API tool
    let providerName = apiTool.oauth_provider || '';
    let oauthScopes = apiTool.oauth_scopes || [];
    
    setOAuthProvider(providerName);
    setOAuthScopes(oauthScopes);
    setEditingTool({tool: apiTool, index});
  };

  const handleSaveTool = () => {
    if (isReadOnly || !editingTool) return;
    if (!validate()) {
      setShowErrors(true);
      return;
    }
    setShowErrors(false);
    
    // Include OAuth settings directly in the IAssistantApi tool
    const updatedTool = {
      ...editingTool.tool,
      oauth_provider: oauthProvider || undefined,
      oauth_scopes: oauthScopes.filter(s => s.trim() !== '')
    };
    
    console.log('ApiIntegrations - saving tool:', {
      tool: updatedTool,
      index: editingTool.index,
      isNew: editingTool.index === -1,
      oauthSettings: { provider: oauthProvider, scopes: oauthScopes }
    });
    
    onSaveApiTool(updatedTool, editingTool.index >= 0 ? editingTool.index : undefined);
    setEditingTool(null);
  };
  
  const addScope = () => {
    setOAuthScopes([...oauthScopes, '']);
  };

  const removeScope = (index: number) => {
    const newScopes = [...oauthScopes];
    newScopes.splice(index, 1);
    setOAuthScopes(newScopes);
  };

  const handleScopeChange = (index: number, value: string) => {
    const newScopes = [...oauthScopes];
    newScopes[index] = value;
    setOAuthScopes(newScopes);
  };

  const updateEditingTool = (updates: Partial<IAssistantApi>) => {
    if (editingTool) {
      setEditingTool({
        ...editingTool,
        tool: { ...editingTool.tool, ...updates }
      });
    }
  };

  const handleSchemaTemplateChange = (selectedTemplate: string) => {
    setSchemaTemplate(selectedTemplate);

    if (selectedTemplate === 'climate') {
      updateEditingTool(climateTool);
    } else if (selectedTemplate === 'jobvacancies') {
      updateEditingTool(jobVacanciesTool);
    } else if (selectedTemplate === 'exchangerates') {
      updateEditingTool(exchangeRatesTool);
    } else if (selectedTemplate === 'productStore') {
      updateEditingTool(productsTool);
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

      <Accordion sx={{ mb: 2 }}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <Typography>Advanced Configuration</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            You can view default template
            in the <Link href="https://github.com/helixml/helix/blob/a27b8c53cdcfafb6663a6553d23c56ef67ebd50a/api/pkg/tools/informative_or_actionable.go#L232-L321" target="_blank">
              Helix repository
            </Link>. The goal is to help the model decide whether a tool should be used to perform an action or not.
          </Typography>
          <TextField
            value={actionableTemplate}
            onChange={(e) => setActionableTemplate(e.target.value)}
            onBlur={() => handleBlur('template')}
            label="'Is Actionable' template"
            fullWidth
            multiline
            rows={4}
            disabled={isReadOnly}
            sx={{ mb: 2 }}
          />
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            The history length is the number of messages that will be used to determine if the tool should be used to perform an action.
            The more context you provide, the worse results you will get on smaller models. For large models this can have an opposite effect.
          </Typography>          
          <TextField
            value={actionableHistoryLength}
            onChange={(e) => setActionableHistoryLength(Number(e.target.value))}
            onBlur={() => handleBlur('history')}
            label="'Is Actionable' history length"
            type="number"
            fullWidth
            disabled={isReadOnly}
          />
        </AccordionDetails>
      </Accordion>

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

            {(() => {
              const matchingTool = tools.find(t => t.name === apiTool.name);
              const actions = matchingTool?.config?.api?.actions;
              if (!actions || actions.length === 0) return null;
              
              return (
                <Box sx={{ mt: 1 }}>
                  <Typography variant="subtitle2">Actions:</Typography>
                  <ul>
                    {actions.map((action: {name: string, method: string, path: string, description: string}, index: number) => (
                      <li key={index}>
                        {action.name}: {action.method} {action.path} ({action.description})
                      </li>
                    ))}
                  </ul>
                </Box>
              );
            })()}
            
            <Box sx={{ mt: 1 }}>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button
                  variant="outlined"
                  onClick={() => {
                    console.log('ApiIntegrations - editing tool at index:', index);
                    handleEditTool(apiTool, index)
                  }}
                  sx={{ mr: 1 }}
                  disabled={isReadOnly}
                  startIcon={<EditIcon />}
                >
                  Edit
                </Button>
                <Button
                  variant="outlined"
                  color="error"
                  onClick={() => onDeleteApiTool(index)}
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
          title={`${editingTool.tool.name ? 'Edit' : 'Add'} API Tool`}
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
                    <MenuItem value="climate">Climate</MenuItem>                    
                    <MenuItem value="exchangerates">Exchange Rates</MenuItem>

                    <MenuItem value="jobvacancies">Job Vacancies</MenuItem>
                    <MenuItem value="productStore">Laptops Store</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.tool.name}
                  onChange={(e) => updateEditingTool({ name: e.target.value })}
                  label="Name"
                  fullWidth
                  required
                  error={showErrors && !editingTool.tool.name}
                  helperText={showErrors && !editingTool.tool.name ? 'Please enter a name' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.tool.description}
                  onChange={(e) => updateEditingTool({ description: e.target.value })}
                  label="Description"
                  required
                  fullWidth
                  error={showErrors && !editingTool.tool.description}
                  helperText="Description of the API, e.g. 'API for currency exchange rates, can be used to get the latest rates'"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.tool.system_prompt || ''}
                  onChange={(e) => updateEditingTool({ system_prompt: e.target.value })}
                  label="System Prompt"
                  fullWidth
                  required={app.agent_mode}
                  multiline
                  rows={4}
                  helperText="Instructions when using this API. E.g. 'You are an expert at using the currency exchange API to get the latest rates'. Only required when used with the agent mode"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingTool.tool.url}
                  onChange={(e) => updateEditingTool({ url: e.target.value })}
                  label="URL"
                  fullWidth
                  error={showErrors && !editingTool.tool.url}
                  helperText={showErrors && !editingTool.tool.url ? 'Please enter a URL' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>                
                <TextField
                  value={editingTool.tool.schema}
                  onChange={(e) => updateEditingTool({ schema: e.target.value })}
                  fullWidth
                  multiline
                  rows={10}
                  label="OpenAPI (Swagger) schema"
                  error={showErrors && !editingTool.tool.schema}
                  helperText={showErrors && !editingTool.tool.schema ? "Please enter a schema" : ""}
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
                  data={editingTool.tool.headers || {}}
                  onChange={(headers) => updateEditingTool({ headers })}
                  entityTitle="header"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <StringMapEditor
                  data={editingTool.tool.query || {}}
                  onChange={(query) => updateEditingTool({ query })}
                  entityTitle="query parameter"
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2, mt: 2 }}>
                  OAuth Configuration
                </Typography>
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel id="oauth-provider-label">OAuth Provider</InputLabel>
                  <Select
                    labelId="oauth-provider-label"
                    id="oauth-provider"
                    value={oauthProvider}
                    label="OAuth Provider"
                    onChange={(e) => setOAuthProvider(e.target.value)}
                    disabled={isReadOnly}
                  >
                    <MenuItem value="">None</MenuItem>
                    {configuredProviders.map((provider) => (
                      <MenuItem key={provider.id} value={provider.name}>
                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                          <Avatar 
                            sx={{ 
                              bgcolor: PROVIDER_COLORS[provider.type] || PROVIDER_COLORS.custom,
                              color: 'white',
                              mr: 1,
                              width: 24,
                              height: 24
                            }}
                          >
                            {PROVIDER_ICONS[provider.type] || PROVIDER_ICONS.custom}
                          </Avatar>
                          <span>{provider.name}</span>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>

                {oauthProvider && (
                  <Box sx={{ mb: 2 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                      <Typography variant="subtitle1">Required Scopes</Typography>
                      <Button 
                        startIcon={<AddIcon />} 
                        onClick={addScope}
                        disabled={isReadOnly}
                        variant="outlined"
                        size="small"
                      >
                        Add Scope
                      </Button>
                    </Box>
                    
                    {oauthScopes.length === 0 ? (
                      <Typography variant="body2" color="text.secondary">
                        No scopes defined. Add scopes to request specific permissions.
                      </Typography>
                    ) : (
                      oauthScopes.map((scope, index) => (
                        <Box key={index} sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                          <TextField
                            value={scope}
                            onChange={(e) => handleScopeChange(index, e.target.value)}
                            fullWidth
                            placeholder="Enter scope"
                            size="small"
                            disabled={isReadOnly}
                          />
                          <IconButton 
                            onClick={() => removeScope(index)}
                            disabled={isReadOnly}
                            color="error"
                          >
                            <DeleteIcon />
                          </IconButton>
                        </Box>
                      ))
                    )}
                  </Box>
                )}
              </Grid>
              <Grid item xs={12}>
                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography>Advanced Template Settings</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <TextField
                      value={editingTool.tool.request_prep_template}
                      onChange={(e) => updateEditingTool({ request_prep_template: e.target.value })}
                      label="Request Prep Template"
                      fullWidth
                      multiline
                      rows={4}
                      disabled={isReadOnly}
                      sx={{ mb: 2 }}
                    />
                    <TextField
                      value={editingTool.tool.response_success_template}
                      onChange={(e) => updateEditingTool({ response_success_template: e.target.value })}
                      label="Response Success Template"
                      fullWidth
                      multiline
                      rows={4}
                      disabled={isReadOnly}
                      sx={{ mb: 2 }}
                    />
                    <TextField
                      value={editingTool.tool.response_error_template}
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
              value={editingTool.tool.schema}
              onChange={(e) => updateEditingTool({ schema: e.target.value })}
              fullWidth
              multiline
              label="OpenAPI (Swagger) schema"
              error={showErrors && !editingTool.tool.schema}
              helperText={showErrors && !editingTool.tool.schema ? "Please enter a schema" : ""}
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