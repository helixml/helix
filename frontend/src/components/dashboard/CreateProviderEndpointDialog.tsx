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
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useEndpointProviders from '../../hooks/useEndpointProviders';
import useAccount from '../../hooks/useAccount';
interface CreateProviderEndpointDialogProps {
  open: boolean;
  onClose: () => void;
  existingEndpoints: IProviderEndpoint[];
}

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

    if (!formData.api_key && !formData.api_key_file) {
      setError('Either API key or API key file path is required');
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
        api_key: formData.api_key,
        api_key_file: formData.api_key_file || undefined,
        endpoint_type: formData.endpoint_type,
        description: formData.description,
      });
      account.fetchProviderEndpoints();
      onClose();
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
            name="provider_base_url"
            label="Base URL"
            value={formData.base_url}
            onChange={handleInputChange}
            fullWidth
            required
            autoComplete="off"
          />

          <TextField
            name="provider_api_key"
            label="API Key"
            value={formData.api_key}
            onChange={handleInputChange}
            fullWidth
            type="password"
            autoComplete="off"
          />

          <TextField
            name="api_key_file"
            label="API Key File Path"
            value={formData.api_key_file}
            onChange={handleInputChange}
            fullWidth
            helperText="Either provide an API key directly or specify a file path containing the key"
          />

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
          variant="contained" 
          disabled={loading}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default CreateProviderEndpointDialog; 