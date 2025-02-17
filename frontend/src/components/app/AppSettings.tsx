import Box from '@mui/material/Box';
import Checkbox from '@mui/material/Checkbox';
import FormControlLabel from '@mui/material/FormControlLabel';
import FormGroup from '@mui/material/FormGroup';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import React from 'react';
import ModelPicker from '../create/ModelPicker';
import ProviderEndpointPicker from '../create/ProviderEndpointPicker';
import { IProviderEndpoint } from '../../types';
interface AppSettingsProps {
  name: string;
  setName: (name: string) => void;
  description: string;
  setDescription: (description: string) => void;
  systemPrompt: string;
  setSystemPrompt: (systemPrompt: string) => void;
  avatar: string;
  setAvatar: (avatar: string) => void;
  image: string;
  setImage: (image: string) => void;
  shared: boolean;
  setShared: (shared: boolean) => void;
  global: boolean;
  setGlobal: (global: boolean) => void;
  model: string;
  setModel: (model: string) => void;
  providerEndpoint: IProviderEndpoint | undefined;
  setProviderEndpoint: (providerEndpoint: IProviderEndpoint) => void;
  providerEndpoints: IProviderEndpoint[];
  readOnly: boolean;
  isReadOnly: boolean;
  showErrors: boolean;
  isAdmin: boolean;
}

const AppSettings: React.FC<AppSettingsProps> = ({
  name,
  setName,
  description,
  setDescription,
  systemPrompt,
  setSystemPrompt,
  avatar,
  setAvatar,
  image,
  setImage,
  shared,
  setShared,
  global,
  setGlobal,
  model,
  setModel,
  providerEndpoint,
  setProviderEndpoint,
  providerEndpoints,
  readOnly,
  isReadOnly,
  showErrors,
  isAdmin,
}) => {
  return (
    <Box sx={{ mt: 2 }}>
      <TextField
        sx={{ mb: 3 }}
        id="app-name"
        name="app-name"
        error={showErrors && !name}
        value={name}
        disabled={readOnly || isReadOnly}
        onChange={(e) => setName(e.target.value)}
        fullWidth
        label="Name"
        helperText="Name your app"
      />
      <TextField
        sx={{ mb: 3 }}
        id="app-description"
        name="app-description"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        disabled={readOnly || isReadOnly}
        fullWidth
        rows={2}
        label="Description"
        helperText="Enter a short description of what this app does"
      />
      <Box sx={{ mb: 3 }}>
        <Typography variant="subtitle1" sx={{ mb: 1 }}>Provider</Typography>
        <ProviderEndpointPicker
          providerEndpoint={providerEndpoint}
          onSetProviderEndpoint={setProviderEndpoint}
          providerEndpoints={providerEndpoints}
        />
      </Box>
      <Box sx={{ mb: 3 }}>
        <Typography variant="subtitle1" sx={{ mb: 1 }}>Model</Typography>
        <ModelPicker
          type="text"
          model={model}
          onSetModel={setModel}
        />
      </Box>
      <TextField
        sx={{ mb: 3 }}
        id="app-instructions"
        name="app-instructions"
        value={systemPrompt}
        onChange={(e) => setSystemPrompt(e.target.value)}
        disabled={readOnly || isReadOnly}
        fullWidth
        multiline
        rows={4}
        label="Instructions"
        helperText="What does this app do? How does it behave? What should it avoid doing?"
      />
      <TextField
        sx={{ mb: 3 }}
        id="app-avatar"
        name="app-avatar"
        value={avatar}
        onChange={(e) => setAvatar(e.target.value)}
        disabled={readOnly || isReadOnly}
        fullWidth
        label="Avatar"
        helperText="URL for the app's avatar image"
      />
      <TextField
        sx={{ mb: 3 }}
        id="app-image"
        name="app-image"
        value={image}
        onChange={(e) => setImage(e.target.value)}
        disabled={readOnly || isReadOnly}
        fullWidth
        label="Image"
        helperText="URL for the app's main image"
      />
      <Tooltip title="Share this app with other users in your organization">
        <FormGroup>
          <FormControlLabel
            control={
              <Checkbox
                checked={shared}
                onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                  setShared(event.target.checked)
                }}
                // Never disable share checkbox -- required for github apps and normal apps
              />
            }
            label="Shared?"
          />
        </FormGroup>
      </Tooltip>
      {isAdmin && (
        <Tooltip title="Make this app available to all users">
          <FormGroup>
            <FormControlLabel
              control={
                <Checkbox
                  checked={global}
                  onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                    setGlobal(event.target.checked)
                  }}
                  // Never disable global checkbox -- required for github apps and normal apps
                />
              }
              label="Global?"
            />
          </FormGroup>
        </Tooltip>
      )}
    </Box>
  );
};

export default AppSettings;