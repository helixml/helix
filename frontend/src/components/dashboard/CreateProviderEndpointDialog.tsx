import React, { useState, useCallback } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  Stack,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormLabel,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useEndpointProviders from '../../hooks/useEndpointProviders';
import useAccount from '../../hooks/useAccount';

interface CreateProviderEndpointDialogProps {
  open: boolean;
  onClose: () => void;
  existingEndpoints: IProviderEndpoint[];
}

type AuthType = 'api_key' | 'api_key_file' | 'none';

const CreateProviderEndpointDialog: React.FC<CreateProviderEndpointDialogProps> = ({
  open,
  onClose,
  existingEndpoints,
}) => {
  const { createEndpoint } = useEndpointProviders();
  const account = useAccount();
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState({
    name: '',
    base_url: '',
    api_key: '',
    api_key_file: '',
    endpoint_type: 'user' as const,
    description: '',
    auth_type: 'none' as AuthType,
  });

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | { name?: string; value: unknown }>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name as string]: value,
    }));
    // Clear error when user makes changes
    setError('');
  };

  const handleAuthTypeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value as AuthType;
    setFormData((prev) => ({
      ...prev,
      auth_type: value,
      // Clear the other auth fields when switching types
      api_key: value === 'api_key' ? prev.api_key : '',
      api_key_file: value === 'api_key_file' ? prev.api_key_file : '',
    }));
    setError('');
  };

  const validateForm = useCallback(() => {
    if (!formData.name.trim()) {
      setError('Name is required');
      return false;
    }

    if (existingEndpoints.some(endpoint => endpoint.name.toLowerCase() === formData.name.toLowerCase())) {
      setError('An endpoint with this name already exists');
      return false;
    }

    if (!formData.base_url.trim()) {
      setError('Base URL is required');
      return false;
    }

    try {
      const url = new URL(formData.base_url);
      if (!['http:', 'https:'].includes(url.protocol)) {
        setError('Base URL must use HTTP or HTTPS protocol');
        return false;
      }
    } catch (err) {
      setError('Please enter a valid URL');
      return false;
    }

    return true;
  }, [formData, existingEndpoints]);

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setLoading(true);
    try {
      await createEndpoint({
        name: formData.name,
        base_url: formData.base_url,
        api_key: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key' ? formData.api_key : undefined,
        api_key_file: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key_file' ? formData.api_key_file : undefined,
        endpoint_type: formData.endpoint_type,
        description: formData.description,
      });
      account.fetchProviderEndpoints();
      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create endpoint');
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setFormData({
      name: '',
      base_url: '',
      api_key: '',
      api_key_file: '',
      endpoint_type: 'user',
      description: '',
      auth_type: 'none',
    });
    setError('');
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create New Provider Endpoint</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 2 }}>
          {error && <Alert severity="error">{error}</Alert>}
          
          <TextField
            name="name"
            label="Provider name"
            value={formData.name}
            onChange={handleInputChange}
            fullWidth
            required
          />

          <TextField
            name="base_url"
            label="Base URL"
            value={formData.base_url}
            onChange={handleInputChange}
            fullWidth
            required
            autoComplete="off"
            placeholder="https://api.example.com"
            helperText="Enter a valid HTTP or HTTPS URL"
          />

          <FormControl component="fieldset">
            <FormLabel component="legend">Authentication Method</FormLabel>
            <RadioGroup
              name="auth_type"
              value={formData.auth_type}
              onChange={handleAuthTypeChange}
            >
              <FormControlLabel value="api_key" control={<Radio />} label="API Key" />
              <FormControlLabel value="api_key_file" control={<Radio />} label="API Key File" />
              <FormControlLabel value="none" control={<Radio />} label="None" />
            </RadioGroup>
          </FormControl>

          {formData.auth_type === 'api_key' && (
            <TextField
              name="api_key"
              label="API Key"
              value={formData.api_key}
              onChange={handleInputChange}
              fullWidth
              type="password"
              autoComplete="off"
            />
          )}

          {formData.auth_type === 'api_key_file' && (
            <TextField
              name="api_key_file"
              label="API Key File Path"
              value={formData.api_key_file}
              onChange={handleInputChange}
              fullWidth
              helperText="Specify a file path containing the API key"
            />
          )}

          <FormControl fullWidth>
            <InputLabel>Type</InputLabel>
            <Select
              name="endpoint_type"
              value={formData.endpoint_type}
              onChange={(e) => handleInputChange(e as React.ChangeEvent<HTMLInputElement | { name?: string; value: unknown }>)}
              label="Type"
            >
              <MenuItem value="user">User (available to you only)</MenuItem>
              <MenuItem value="global">Global (available to all users in Helix installation)</MenuItem>
            </Select>
          </FormControl>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button 
          onClick={handleSubmit} 
          variant="outlined"
          color="secondary"
          disabled={loading}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default CreateProviderEndpointDialog; 