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
  RadioGroup,
  FormControlLabel,
  Radio,
  FormLabel,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import { TypesProviderEndpointType } from '../../api/api'
import useEndpointProviders from '../../hooks/useEndpointProviders';
import useAccount from '../../hooks/useAccount';

// Helper function to determine auth type from endpoint
export const getEndpointAuthType = (endpoint: IProviderEndpoint | null): AuthType => {
  // If both are empty, return none
  if (!endpoint?.api_key && !endpoint?.api_key_file) {
    return 'none';
  }

  // If api_key_file is set, return api_key_file
  if (endpoint?.api_key_file) {
    return 'api_key_file';
  }

  // If api_key is set, return api_key
  if (endpoint?.api_key) {
    return 'api_key';
  }

  // If neither are set, return none
  return 'none';
}

interface EditProviderEndpointDialogProps {
  open: boolean;
  endpoint: IProviderEndpoint | null;
  onClose: () => void;
}

type AuthType = 'api_key' | 'api_key_file' | 'none';

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
    api_key: '',
    api_key_file: endpoint?.api_key_file || '',
    endpoint_type: endpoint?.endpoint_type || 'user',
    auth_type: getEndpointAuthType(endpoint),
  });

  React.useEffect(() => {
    if (endpoint) {
      setFormData({       
        base_url: endpoint.base_url,
        api_key: '',
        api_key_file: endpoint.api_key_file || '',
        endpoint_type: endpoint.endpoint_type,
        auth_type: getEndpointAuthType(endpoint),
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
        api_key: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key' ? formData.api_key : undefined,
        api_key_file: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key_file' ? formData.api_key_file : undefined,
        endpoint_type: (formData.endpoint_type as TypesProviderEndpointType),
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
      auth_type: getEndpointAuthType(endpoint),
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
              onChange={handleTextFieldChange}
              fullWidth
              type="password"
              autoComplete="off"
              helperText="Leave blank to keep the existing API key"
            />
          )}

          {formData.auth_type === 'api_key_file' && (
            <TextField
              name="api_key_file"
              label="API Key File Path"
              value={formData.api_key_file}
              onChange={handleTextFieldChange}
              fullWidth
              helperText="Specify a file path containing the API key"
            />
          )}

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