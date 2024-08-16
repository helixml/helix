import React, { FC, useState, useEffect } from 'react';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import FormControlLabel from '@mui/material/FormControlLabel';
import Checkbox from '@mui/material/Checkbox';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';

import StringMapEditor from './widgets/StringMapEditor';
import JsonWindowLink from './widgets/JsonWindowLink';
import Window from './widgets/Window';
import ClickLink from './widgets/ClickLink'; // Import ClickLink

import { ITool } from '../types';

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
  const [global, setGlobal] = useState(initialData.global || false);
  const [url, setURL] = useState(initialData.config.api?.url || '');
  const [gptScriptURL, setGptScriptURL] = useState(initialData.config.gptscript?.script_url || '');
  const [gptScript, setGptScript] = useState(initialData.config.gptscript?.script || '');
  const [schema, setSchema] = useState(initialData.config.api?.schema || '');
  const [headers, setHeaders] = useState<Record<string, string>>(initialData.config.api?.headers || {});
  const [query, setQuery] = useState<Record<string, string>>(initialData.config.api?.query || {});
  const [showErrors, setShowErrors] = useState(false);
  const [showBigSchema, setShowBigSchema] = useState(false);

  useEffect(() => {
    console.log('ToolEditor: useEffect triggered with initialData:', initialData);
    if (initialData) {
      setName(initialData.name || '');
      setDescription(initialData.description || '');
      setGlobal(initialData.global || false);
      if (initialData.config.api) {
        setURL(initialData.config.api.url || '');
        setSchema(initialData.config.api.schema || '');
        setHeaders(initialData.config.api.headers || {});
        setQuery(initialData.config.api.query || {});
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
    console.log('ToolEditor: Save button clicked');
    if (!validate()) {
      console.log('ToolEditor: Validation failed');
      setShowErrors(true);
      return;
    }
    setShowErrors(false);
    const updatedData: ITool = {
      ...initialData,
      name,
      description,
      global,
      config: initialData.tool_type === 'api'
        ? {
            api: {
              url,
              schema,
              actions: initialData.config.api?.actions || [],
              headers,
              query,
            },
          }
        : {
            gptscript: {
              script_url: gptScriptURL,
              script: gptScript,
            },
          },
    };
    console.log('ToolEditor: Calling onSave with updated data:', updatedData);
    onSave(updatedData);
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
        <Grid item xs={12}>
          <FormControlLabel
            control={
              <Checkbox
                checked={global}
                onChange={(e) => setGlobal(e.target.checked)}
                disabled={isReadOnly}
              />
            }
            label="Global"
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
                error={showErrors && !gptScriptURL && !gptScript}
                helperText={
                  showErrors && !gptScriptURL && !gptScript
                    ? 'Please enter a script URL or script'
                    : ''
                }
                disabled={isReadOnly}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                value={gptScript}
                onChange={(e) => setGptScript(e.target.value)}
                label="Script"
                fullWidth
                id="tool-script"
                name="tool-script"
                error={showErrors && !gptScriptURL && !gptScript}
                helperText={
                  showErrors && !gptScriptURL && !gptScript
                    ? 'Please enter a script URL or script'
                    : ''
                }
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