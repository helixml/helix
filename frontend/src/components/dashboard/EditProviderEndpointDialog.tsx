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
  SelectChangeEvent,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useEndpointProviders from '../../hooks/useEndpointProviders';
import useAccount from '../../hooks/useAccount';

interface EditProviderEndpointDialogProps {
  open: boolean;
  endpoint: IProviderEndpoint | null;
  onClose: () => void;
}

const EditProviderEndpointDialog: React.FC<EditProviderEndpointDialogProps> = ({
  open,
  endpoint,
  onClose,
}) => {
  const { updateEndpoint } = useEndpointProviders();
  const account = useAccount();
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState({
    base_url: endpoint?.base_url || '',
    api_key: endpoint?.api_key || '',
    api_key_file: endpoint?.api_key_file || '',
    endpoint_type: endpoint?.endpoint_type || 'user',
  });

  React.useEffect(() => {
    if (endpoint) {
      setFormData({       
        base_url: endpoint.base_url,
        api_key: endpoint.api_key,
        api_key_file: endpoint.api_key_file || '',
        endpoint_type: endpoint.endpoint_type,
      });
    }
  }, [endpoint]);

  const handleTextFieldChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name]: value,
    }));
    setError('');
  };

  const handleSelectChange = (e: SelectChangeEvent) => {
    const { name, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name]: value,
    }));
    setError('');
  };

  const validateForm = useCallback(() => {
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
  }, [formData]);

  const handleSubmit = async () => {
    if (!validateForm() || !endpoint) return;

    setLoading(true);
    try {
      await updateEndpoint(endpoint.id, {
        name: endpoint.name,
        base_url: formData.base_url,
        api_key: formData.api_key || undefined,
        api_key_file: formData.api_key_file || undefined,
        endpoint_type: formData.endpoint_type as 'global' | 'user',
      });
      await account.fetchProviderEndpoints();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update endpoint');
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setFormData({
      base_url: endpoint?.base_url || '',
      api_key: '',
      api_key_file: endpoint?.api_key_file || '',
      endpoint_type: endpoint?.endpoint_type || 'user',
    });
    setError('');
    onClose();
  };

  if (!endpoint) return null;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Edit Provider Endpoint: {endpoint.name}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 2 }}>
          {error && <Alert severity="error">{error}</Alert>}

          <TextField
            name="base_url"
            label="Base URL"
            value={formData.base_url}
            onChange={handleTextFieldChange}
            fullWidth
            required
            autoComplete="off"
            placeholder="https://api.example.com"
            helperText="Enter a valid HTTP or HTTPS URL"
          />

          <TextField
            name="api_key"
            label="API Key"
            value={formData.api_key}
            onChange={handleTextFieldChange}
            fullWidth
            type="password"
            autoComplete="off"
            helperText="Leave blank to keep the existing API key"
          />

          <TextField
            name="api_key_file"
            label="API Key File Path"
            value={formData.api_key_file}
            onChange={handleTextFieldChange}
            fullWidth
            helperText="Either provide an API key directly or specify a file path containing the key"
          />

          <FormControl fullWidth>
            <InputLabel>Type</InputLabel>
            <Select
              name="endpoint_type"
              value={formData.endpoint_type}
              onChange={handleSelectChange}
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
          Save Changes
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EditProviderEndpointDialog; 