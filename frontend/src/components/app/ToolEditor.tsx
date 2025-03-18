import React, { FC, useState, useEffect } from 'react';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Avatar from '@mui/material/Avatar';
import IconButton from '@mui/material/IconButton';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';

import StringMapEditor from '../widgets/StringMapEditor';
import JsonWindowLink from '../widgets/JsonWindowLink';
import Window from '../widgets/Window';
import ClickLink from '../widgets/ClickLink';

import { ITool, IToolApiAction } from '../../types';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';

interface ToolEditorProps {
  initialData: ITool;
  onSave: (data: ITool) => void;
  onCancel: () => void;
  isReadOnly?: boolean;
}

const ToolEditor: FC<ToolEditorProps> = ({ initialData, onSave, onCancel, isReadOnly = false }) => {
  console.log('ToolEditor: Initializing with data:', initialData);

  const [name, setName] = useState(initialData.name || '');
  const [description, setDescription] = useState(initialData.description || '');
  const [url, setURL] = useState(initialData.config.api?.url || '');
  const [gptScriptURL, setGptScriptURL] = useState(initialData.config.gptscript?.script_url || '');
  const [gptScript, setGptScript] = useState(initialData.config.gptscript?.script || '');
  const [schema, setSchema] = useState(initialData.config.api?.schema || '');
  const [headers, setHeaders] = useState<Record<string, string>>(initialData.config.api?.headers || {});
  const [query, setQuery] = useState<Record<string, string>>(initialData.config.api?.query || {});
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);
  const [actions, setActions] = useState<IToolApiAction[]>(initialData.config.api?.actions || []);
  const [requestPrepTemplate, setRequestPrepTemplate] = useState(initialData.config.api?.request_prep_template || '');
  const [responseSuccessTemplate, setResponseSuccessTemplate] = useState(initialData.config.api?.response_success_template || '');
  const [responseErrorTemplate, setResponseErrorTemplate] = useState(initialData.config.api?.response_error_template || '');
  const [useScriptUrl, setUseScriptUrl] = useState(!!initialData.config.gptscript?.script_url);
  
  // New OAuth provider and scopes state
  const [oauthProvider, setOAuthProvider] = useState(initialData.config.api?.oauth_provider || '');
  const [oauthScopes, setOAuthScopes] = useState<string[]>(initialData.config.api?.oauth_scopes || []);

  useEffect(() => {
    console.log('ToolEditor: useEffect triggered with initialData:', initialData);
    if (initialData) {
      setName(initialData.name || '');
      setDescription(initialData.description || '');
      if (initialData.config.api) {
        setURL(initialData.config.api.url || '');
        setSchema(initialData.config.api.schema || '');
        setHeaders(initialData.config.api.headers || {});
        setQuery(initialData.config.api.query || {});
        setActions(initialData.config.api.actions || []);
        setRequestPrepTemplate(initialData.config.api.request_prep_template || '');
        setResponseSuccessTemplate(initialData.config.api.response_success_template || '');
        setResponseErrorTemplate(initialData.config.api.response_error_template || '');
        setOAuthProvider(initialData.config.api.oauth_provider || '');
        setOAuthScopes(initialData.config.api.oauth_scopes || []);
      } else if (initialData.config.gptscript) {
        setGptScriptURL(initialData.config.gptscript.script_url || '');
        setGptScript(initialData.config.gptscript.script || '');
      }
    }
  }, [initialData]);

  const validate = () => {
    console.log('ToolEditor: Validating form data');
    if (!name) return false;
    if (!description) return false;
    if (initialData.config.api) {
      if (!url) return false;
      if (!schema) return false;
    } else if (initialData.config.gptscript) {
      if (!gptScriptURL && !gptScript) return false;
    }
    return true;
  };

  const handleSave = () => {
    if (isReadOnly) return;
    if (!validate()) {
      setShowErrors(true);
      return;
    }
    setShowErrors(false);
    const updatedData: ITool = {
      ...initialData,
      name,
      description,
      config: initialData.tool_type === 'api'
        ? {
            api: {
              url,
              schema,
              actions,
              headers,
              query,
              request_prep_template: requestPrepTemplate,
              response_success_template: responseSuccessTemplate,
              response_error_template: responseErrorTemplate,
              oauth_provider: oauthProvider || undefined,
              oauth_scopes: oauthScopes.filter(s => s.trim() !== '') || [],
            },
          }
        : {
            gptscript: {
              script: gptScript,
              script_url: gptScriptURL,
            },
          },
    };
    console.log('Saving tool:', updatedData);
    onSave(updatedData);
  };

  const handleScopeChange = (index: number, value: string) => {
    const newScopes = [...oauthScopes];
    newScopes[index] = value;
    setOAuthScopes(newScopes);
  };

  const addScope = () => {
    setOAuthScopes([...oauthScopes, '']);
  };

  const removeScope = (index: number) => {
    const newScopes = [...oauthScopes];
    newScopes.splice(index, 1);
    setOAuthScopes(newScopes);
  };

  console.log('ToolEditor: Rendering component');

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        {initialData.tool_type === 'api' ? 'API Tool' : 'GPT Script'}
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12}>
          <TextField
            value={name}
            onChange={(e) => setName(e.target.value)}
            label="Name"
            fullWidth
            id="tool-name"
            name="tool-name"
            error={showErrors && !name}
            helperText={showErrors && !name ? 'Please enter a name' : ''}
            disabled={isReadOnly}
          />
        </Grid>
        <Grid item xs={12}>
          <TextField
            required
            error={showErrors && !description}
            helperText={showErrors && !description ? "Description is required" : ""}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            label="Description"
            fullWidth
            id="tool-description"
            name="tool-description"
            disabled={isReadOnly}
          />
        </Grid>
        {initialData.config.api ? (
          <>
            <Grid item xs={12}>
              <TextField
                value={url}
                onChange={(e) => setURL(e.target.value)}
                label="URL"
                fullWidth
                id="tool-url"
                name="tool-url"
                error={showErrors && !url}
                helperText={showErrors && !url ? 'Please enter a URL' : ''}
                disabled={isReadOnly}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                error={showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                disabled={isReadOnly}
                fullWidth
                multiline
                rows={10}
                label="OpenAPI (Swagger) schema"
                helperText={showErrors && !schema ? "Please enter a schema" : ""}
              />
              <Box
                sx={{
                  textAlign: 'right',
                  mb: 1,
                }}
              >
                <ClickLink
                  onClick={() => setShowBigSchema(true)}
                >
                  expand schema
                </ClickLink>
              </Box>
            </Grid>
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
                <Box
                  sx={{
                    p: 2,
                    height: '100%',
                  }}
                >
                  <TextField
                    error={showErrors && !schema}
                    value={schema}
                    onChange={(e) => setSchema(e.target.value)}
                    fullWidth
                    multiline
                    label="OpenAPI (Swagger) schema"
                    helperText={showErrors && !schema ? "Please enter a schema" : ""}
                    sx={{ height: '100%' }}
                  />
                </Box>
              </Window>
            )}
            <Grid item xs={12}>
              <StringMapEditor
                data={headers}
                onChange={setHeaders}
                entityTitle="Headers"
                disabled={isReadOnly}
              />
            </Grid>
            <Grid item xs={12}>
              <StringMapEditor
                data={query}
                onChange={setQuery}
                entityTitle="Query Parameters"
                disabled={isReadOnly}
              />
            </Grid>
            <Grid item xs={12}>
              {actions.map((action, index) => (
                <Box key={index} sx={{ mb: 2 }}>
                  <TextField
                    label="Name"
                    value={action.name}
                    onChange={(e) => {
                      const newActions = [...actions];
                      newActions[index].name = e.target.value;
                      setActions(newActions);
                    }}
                    disabled={isReadOnly}
                    sx={{ mr: 2 }}
                  />
                  <TextField
                    label="Description"
                    value={action.description}
                    onChange={(e) => {
                      const newActions = [...actions];
                      newActions[index].description = e.target.value;
                      setActions(newActions);
                    }}
                    disabled={isReadOnly}
                    sx={{ mr: 2 }}
                  />
                  <TextField
                    label="Method"
                    value={action.method}
                    onChange={(e) => {
                      const newActions = [...actions];
                      newActions[index].method = e.target.value;
                      setActions(newActions);
                    }}
                    disabled={isReadOnly}
                    sx={{ mr: 2 }}
                  />
                  <TextField
                    label="Path"
                    value={action.path}
                    onChange={(e) => {
                      const newActions = [...actions];
                      newActions[index].path = e.target.value;
                      setActions(newActions);
                    }}
                    disabled={isReadOnly}
                  />
                </Box>
              ))}
            </Grid>

            {/* OAuth Configuration Section */}
            <Grid item xs={12}>
              <Typography variant="h6" sx={{ mb: 2 }}>
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
                  {Object.keys(PROVIDER_ICONS).map((provider) => (
                    <MenuItem key={provider} value={provider}>
                      <Box sx={{ display: 'flex', alignItems: 'center' }}>
                        <Avatar 
                          sx={{ 
                            bgcolor: PROVIDER_COLORS[provider] || PROVIDER_COLORS.custom,
                            color: 'white',
                            mr: 1,
                            width: 24,
                            height: 24
                          }}
                        >
                          {PROVIDER_ICONS[provider]}
                        </Avatar>
                        <span>{provider}</span>
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
                    value={requestPrepTemplate}
                    onChange={(e) => setRequestPrepTemplate(e.target.value)}
                    label="Request Prep Template"
                    fullWidth
                    multiline
                    rows={4}
                    disabled={isReadOnly}
                    sx={{ mb: 2 }}
                  />
                  <TextField
                    value={responseSuccessTemplate}
                    onChange={(e) => setResponseSuccessTemplate(e.target.value)}
                    label="Response Success Template"
                    fullWidth
                    multiline
                    rows={4}
                    disabled={isReadOnly}
                    sx={{ mb: 2 }}
                  />
                  <TextField
                    value={responseErrorTemplate}
                    onChange={(e) => setResponseErrorTemplate(e.target.value)}
                    label="Response Error Template"
                    fullWidth
                    multiline
                    rows={4}
                    disabled={isReadOnly}
                  />
                </AccordionDetails>
              </Accordion>
            </Grid>
          </>
        ) : (
          <>
            <Grid item xs={12}>
              <TextField
                value={gptScriptURL}
                onChange={(e) => setGptScriptURL(e.target.value)}
                label="Script URL"
                fullWidth
                id="tool-script-url"
                name="tool-script-url"
                error={showErrors && !gptScriptURL}
                helperText={showErrors && !gptScriptURL ? 'Please enter a script URL' : ''}
                disabled={isReadOnly}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                value={gptScript}
                onChange={(e) => setGptScript(e.target.value)}
                label="Script"
                fullWidth
                multiline
                rows={10}
                id="tool-script"
                name="tool-script"
                error={showErrors && !gptScript}
                helperText={showErrors && !gptScript ? 'Please enter a script' : ''}
                disabled={isReadOnly}
              />
            </Grid>
          </>
        )}
      </Grid>
      <Box sx={{ mt: 2 }}>
        <Button
          variant="contained"
          color="primary"
          onClick={handleSave}
          disabled={isReadOnly}
          sx={{ mr: 2 }}
        >
          Save
        </Button>
        <Button 
          variant="contained" 
          color="secondary" 
          onClick={onCancel}
        >
          Cancel
        </Button>
      </Box>
    </Box>
  );
};

export default ToolEditor;