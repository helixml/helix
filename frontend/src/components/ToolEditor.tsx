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

interface ToolEditorProps {
  initialData: any;
  onSave: (data: any) => void;
  onCancel: () => void;
}

const ToolEditor: FC<ToolEditorProps> = ({ initialData, onSave, onCancel }) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [global, setGlobal] = useState(false);
  const [url, setURL] = useState('');
  const [gptScriptURL, setGptScriptURL] = useState('');
  const [gptScript, setGptScript] = useState('');
  const [schema, setSchema] = useState('');
  const [headers, setHeaders] = useState<Record<string, string>>({});
  const [query, setQuery] = useState<Record<string, string>>({});
  const [showErrors, setShowErrors] = useState(false);

  useEffect(() => {
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
    if (!validate()) {
      setShowErrors(true);
      return;
    }
    setShowErrors(false);
    const updatedData = {
      ...initialData,
      name: name,
      description: description,
      global: global,
      config: initialData.config.api
        ? {
            api: {
              url: url,
              schema: schema,
              headers: headers,
              query: query,
            },
          }
        : {
            gptscript: {
              script_url: gptScriptURL,
              script: gptScript,
            },
          },
    };
    onSave(updatedData);
  };

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        {initialData.config.api ? 'API Tool' : 'GPT Script'}
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12}>
          <TextField
            value={name}
            onChange={(e) => setName(e.target.value)}
            label="Name"
            fullWidth
            error={showErrors && !name}
            helperText={showErrors && !name ? 'Please enter a name' : ''}
          />
        </Grid>
        <Grid item xs={12}>
          <TextField
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            label="Description"
            fullWidth
            error={showErrors && !description}
            helperText={showErrors && !description ? 'Please enter a description' : ''}
          />
        </Grid>
        <Grid item xs={12}>
          <FormControlLabel
            control={
              <Checkbox
                checked={global}
                onChange={(e) => setGlobal(e.target.checked)}
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
                error={showErrors && !url}
                helperText={showErrors && !url ? 'Please enter a URL' : ''}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                label="Schema"
                fullWidth
                error={showErrors && !schema}
                helperText={showErrors && !schema ? 'Please enter a schema' : ''}
              />
            </Grid>
            <Grid item xs={12}>
              <StringMapEditor
                data={headers}
                onChange={setHeaders}
                entityTitle="Headers"
              />
            </Grid>
            <Grid item xs={12}>
              <StringMapEditor
                data={query}
                onChange={setQuery}
                entityTitle="Query Parameters"
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
                error={showErrors && !gptScriptURL && !gptScript}
                helperText={
                  showErrors && !gptScriptURL && !gptScript
                    ? 'Please enter a script URL or script'
                    : ''
                }
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                value={gptScript}
                onChange={(e) => setGptScript(e.target.value)}
                label="Script"
                fullWidth
                error={showErrors && !gptScriptURL && !gptScript}
                helperText={
                  showErrors && !gptScriptURL && !gptScript
                    ? 'Please enter a script URL or script'
                    : ''
                }
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
          sx={{ mr: 2 }}
        >
          Save
        </Button>
        <Button variant="contained" color="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </Box>
    </Box>
  );
};

export default ToolEditor;