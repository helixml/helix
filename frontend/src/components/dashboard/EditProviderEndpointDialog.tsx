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
  IconButton,
  Box,
  Divider,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import { IProviderEndpoint } from '../../types';
import { TypesProviderEndpointType } from '../../api/api'
import { useUpdateProviderEndpoint } from '../../services/providersService';
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
  refreshData: () => void;
}

type AuthType = 'api_key' | 'api_key_file' | 'none';

const EditProviderEndpointDialog: React.FC<EditProviderEndpointDialogProps> = ({
  open,
  endpoint,
  onClose,
  refreshData,
}) => {
  const account = useAccount();
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState({
    name: endpoint?.name || '',
    base_url: endpoint?.base_url || '',
    api_key: '',
    api_key_file: endpoint?.api_key_file || '',
    endpoint_type: endpoint?.endpoint_type || 'user',
    auth_type: getEndpointAuthType(endpoint),
    headers: [] as Array<{ key: string; value: string }>,
  });

  // Only initialize the mutation hook if we have a valid endpoint ID
  const { mutate: updateProviderEndpoint } = useUpdateProviderEndpoint(endpoint?.id || '');

  // Reset form data when endpoint changes
  React.useEffect(() => {
    if (endpoint) {
      // Convert headers object to array format
      const headersArray = endpoint.headers
        ? Object.entries(endpoint.headers).map(([key, value]) => ({ key, value }))
        : [];

      setFormData({
        name: endpoint.name,
        base_url: endpoint.base_url,
        api_key: '',
        api_key_file: endpoint.api_key_file || '',
        endpoint_type: endpoint.endpoint_type,
        auth_type: getEndpointAuthType(endpoint),
        headers: headersArray,
      });
      setError('');
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

  const handleHeaderChange = (index: number, field: 'key' | 'value', value: string) => {
    setFormData((prev) => ({
      ...prev,
      headers: prev.headers.map((header, i) =>
        i === index ? { ...header, [field]: value } : header
      ),
    }));
    setError('');
  };

  const addHeader = () => {
    setFormData((prev) => ({
      ...prev,
      headers: [...prev.headers, { key: '', value: '' }],
    }));
  };

  const removeHeader = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      headers: prev.headers.filter((_, i) => i !== index),
    }));
  };

  const validateForm = useCallback(() => {
    if (!formData.name.trim()) {
      setError('Name is required');
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
  }, [formData]);

  const handleSubmit = async () => {
    if (!validateForm() || !endpoint?.id) {
      setError('Invalid endpoint or missing endpoint ID');
      return;
    }

    setLoading(true);
    try {
      // Convert headers array to object, filtering out empty entries
      const headersObj: Record<string, string> = {};
      formData.headers.forEach(({ key, value }) => {
        if (key.trim() && value.trim()) {
          headersObj[key.trim()] = value.trim();
        }
      });

      const body = {
        name: formData.name,
        base_url: formData.base_url,
        api_key: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key' ? formData.api_key : undefined,
        api_key_file: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key_file' ? formData.api_key_file : undefined,
        endpoint_type: (formData.endpoint_type as TypesProviderEndpointType),
        headers: Object.keys(headersObj).length > 0 ? headersObj : undefined,
      }
      await updateProviderEndpoint(body);
      refreshData();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update endpoint');
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setFormData({
      name: endpoint?.name || '',
      base_url: endpoint?.base_url || '',
      api_key: '',
      api_key_file: endpoint?.api_key_file || '',
      endpoint_type: endpoint?.endpoint_type || 'user',
      auth_type: getEndpointAuthType(endpoint),
      headers: [],
    });
    setError('');
    onClose();
  };

  // Don't render anything if we don't have an endpoint
  if (!endpoint?.id) return null;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Edit Provider Endpoint: {endpoint.name}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 2 }}>
          {error && <Alert severity="error">{error}</Alert>}

          <TextField
            name="id"
            label="Endpoint ID"
            value={endpoint.id}
            fullWidth
            autoComplete="off"
            disabled
          />

          <TextField
            name="name"
            label="Name"
            value={formData.name}
            onChange={handleTextFieldChange}
            fullWidth
            required
            autoComplete="off"
            placeholder="my-provider"
            helperText="A unique name to identify this provider endpoint"
          />

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

          <Divider sx={{ my: 2 }} />

          <Box>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Typography variant="h6">Custom Headers</Typography>
              <Button
                startIcon={<AddIcon />}
                onClick={addHeader}
                variant="outlined"
                size="small"
              >
                Add Header
              </Button>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Add custom headers that will be sent with requests to this endpoint
            </Typography>
            
            {formData.headers.map((header, index) => (
              <Box key={index} sx={{ display: 'flex', gap: 1, mb: 2, alignItems: 'center' }}>
                <TextField
                  label="Header Name"
                  value={header.key}
                  onChange={(e) => handleHeaderChange(index, 'key', e.target.value)}
                  placeholder="e.g., X-API-Key"
                  sx={{ flex: 1 }}
                />
                <TextField
                  label="Header Value"
                  value={header.value}
                  onChange={(e) => handleHeaderChange(index, 'value', e.target.value)}
                  placeholder="e.g., your-api-key"
                  sx={{ flex: 1 }}
                />
                <IconButton
                  onClick={() => removeHeader(index)}
                  color="error"
                  size="small"
                >
                  <DeleteIcon />
                </IconButton>
              </Box>
            ))}
            
            {formData.headers.length === 0 && (
              <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                No custom headers added. Click "Add Header" to add custom headers.
              </Typography>
            )}
          </Box>
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