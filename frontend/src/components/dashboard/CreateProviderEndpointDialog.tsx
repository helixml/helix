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
  Checkbox,
  Typography,
  IconButton,
  Box,
  Divider,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';

import { IProviderEndpoint } from '../../types';
import { TypesProviderEndpointType } from '../../api/api'
import { useCreateProviderEndpoint } from '../../services/providersService';

interface CreateProviderEndpointDialogProps {
  open: boolean;
  onClose: () => void;
  existingEndpoints: IProviderEndpoint[];
  providersManagementEnabled?: boolean; // When false, hide "User" option (only admins can create global)
}

type AuthType = 'api_key' | 'api_key_file' | 'none';

const CreateProviderEndpointDialog: React.FC<CreateProviderEndpointDialogProps> = ({
  open,
  onClose,
  existingEndpoints,
  providersManagementEnabled = true,
}) => {
  const { mutate: createProviderEndpoint } = useCreateProviderEndpoint();
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  // When providers management is disabled, default to global (only admins can use this dialog)
  const defaultEndpointType = providersManagementEnabled ? 'user' : 'global';
  const [formData, setFormData] = useState({
    name: '',
    base_url: '',
    api_key: '',
    api_key_file: '',
    endpoint_type: defaultEndpointType as 'user' | 'global',
    description: '',
    auth_type: 'none' as AuthType,
    billing_enabled: false,
    headers: [] as Array<{ key: string; value: string }>,
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

  const handleCheckboxChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, checked } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name]: checked,
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
      // Convert headers array to object, filtering out empty entries
      const headersObj: Record<string, string> = {};
      formData.headers.forEach(({ key, value }) => {
        if (key.trim() && value.trim()) {
          headersObj[key.trim()] = value.trim();
        }
      });

      await createProviderEndpoint({
        name: formData.name,
        base_url: formData.base_url,
        api_key: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key' ? formData.api_key : undefined,
        api_key_file: formData.auth_type === 'none' ? '' : formData.auth_type === 'api_key_file' ? formData.api_key_file : undefined,
        endpoint_type: (formData.endpoint_type as TypesProviderEndpointType),
        description: formData.description,
        billing_enabled: formData.billing_enabled,
        headers: Object.keys(headersObj).length > 0 ? headersObj : undefined,
      });
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
      billing_enabled: false,
      headers: [],
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
              {/* Only show User option when providers management is enabled -
                  when disabled, only admins can create endpoints and "user" type
                  would only be visible to that specific admin, not useful */}
              {providersManagementEnabled && (
                <MenuItem value="user">User (available to you only)</MenuItem>
              )}
              <MenuItem value="global">Global (available to all users in Helix installation)</MenuItem>
            </Select>
          </FormControl>

          <FormControlLabel
            control={
              <Checkbox
                name="billing_enabled"
                checked={formData.billing_enabled}
                onChange={handleCheckboxChange}
              />
            }
            label="Billing Enabled"
          />
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            Users will be using their wallet balance for inference
          </Typography>

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
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default CreateProviderEndpointDialog; 