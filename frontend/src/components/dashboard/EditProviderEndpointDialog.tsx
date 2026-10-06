import React, { useState, useCallback } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  FormControl,
  MenuItem,
  Alert,
  Stack,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormLabel,
  IconButton,
  Tooltip,
  Box,
  Divider,
  Typography,
  Switch,
} from '@mui/material';
import { Check, Copy, Server } from 'lucide-react';
import { TypesProviderEndpointType } from '../../api/api'
import { useUpdateProviderEndpoint } from '../../services/providersService';
import useAccount from '../../hooks/useAccount';
import { getFormSelectSx } from '../../contexts/theme';
import { copyTextToClipboard } from '../../utils/clipboard';
import { APP_MONO_FONT_FAMILY } from '../../styles/typography';
import ProviderEndpointIcon, { PROVIDER_ICON_OPTIONS, ProviderMark } from '../providers/ProviderEndpointIcon';
import {
  CustomHeadersEditor,
  FormRow,
  FormSection,
  ProviderEndpointHeader,
} from './ProviderEndpointFormFields';

// Helper function to determine auth type from endpoint
interface EditableProviderEndpoint {
  id?: string
  name?: string
  description?: string
  icon?: string
  billing_enabled?: boolean
  base_url?: string
  api_key?: string
  api_key_file?: string
  endpoint_type?: string
  models?: string[]
  headers?: Record<string, string>
}

export const getEndpointAuthType = (endpoint: EditableProviderEndpoint | null): AuthType => {
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
  endpoint: EditableProviderEndpoint | null;
  onClose: () => void;
  refreshData: () => void;
}

type AuthType = 'api_key' | 'api_key_file' | 'none';

const EndpointIdChip: React.FC<{ id: string }> = ({ id }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await copyTextToClipboard(id);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Stack direction="row" alignItems="center" spacing={0.25} sx={{ minWidth: 0 }}>
      <Typography
        variant="caption"
        color="text.secondary"
        noWrap
        sx={{ fontFamily: APP_MONO_FONT_FAMILY }}
      >
        {id}
      </Typography>
      <Tooltip title={copied ? 'Copied' : 'Copy endpoint ID'}>
        <IconButton
          onClick={handleCopy}
          aria-label="Copy endpoint ID"
          sx={{ width: 24, height: 24, color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
        </IconButton>
      </Tooltip>
    </Stack>
  );
};

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
    description: endpoint?.description || '',
    icon: endpoint?.icon || '',
    billing_enabled: endpoint?.billing_enabled || false,
    base_url: endpoint?.base_url || '',
    api_key: '',
    api_key_file: endpoint?.api_key_file || '',
    endpoint_type: endpoint?.endpoint_type || 'user',
    auth_type: getEndpointAuthType(endpoint),
    headers: [] as ProviderEndpointHeader[],
  });

  const { mutateAsync: updateProviderEndpoint } = useUpdateProviderEndpoint();

  // Reset form data when endpoint changes
  React.useEffect(() => {
    if (endpoint) {
      // Convert headers object to array format
      const headersArray = endpoint.headers
        ? Object.entries(endpoint.headers).map(([key, value]) => ({ key, value }))
        : [];

      setFormData({
        name: endpoint.name || '',
        description: endpoint.description || '',
        icon: endpoint.icon || '',
        billing_enabled: endpoint.billing_enabled || false,
        base_url: endpoint.base_url || '',
        api_key: '',
        api_key_file: endpoint.api_key_file || '',
        endpoint_type: endpoint.endpoint_type || 'user',
        auth_type: getEndpointAuthType(endpoint),
        headers: headersArray,
      });
      setError('');
    }
  }, [endpoint]);

  const handleTextFieldChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
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

  const handleHeadersChange = (headers: ProviderEndpointHeader[]) => {
    setFormData((prev) => ({ ...prev, headers }));
    setError('');
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

      // For api_key: only send if user explicitly entered a new key, or '' to clear when auth_type is 'none'
      // If auth_type is 'api_key' but field is empty, send undefined to preserve existing key
      const body = {
        name: formData.name,
        description: formData.description,
        icon: account.admin ? formData.icon : undefined,
        billing_enabled: account.admin ? formData.billing_enabled : undefined,
        models: endpoint.models || [],
        base_url: formData.base_url,
        api_key: formData.auth_type === 'none'
          ? ''
          : (formData.auth_type === 'api_key' && formData.api_key)
            ? formData.api_key
            : undefined,
        api_key_file: formData.auth_type === 'none'
          ? ''
          : (formData.auth_type === 'api_key_file' && formData.api_key_file)
            ? formData.api_key_file
            : undefined,
        endpoint_type: (formData.endpoint_type as TypesProviderEndpointType),
        // Always send the map, including when it is empty: the server treats a
        // missing `headers` as "leave them alone", so omitting it would make
        // removing the last header impossible.
        headers: headersObj,
      }
      await updateProviderEndpoint({ id: endpoint.id, body });
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
      description: endpoint?.description || '',
      icon: endpoint?.icon || '',
      billing_enabled: endpoint?.billing_enabled || false,
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

  const previewEndpoint = {
    icon: formData.icon,
    name: formData.name,
    base_url: formData.base_url,
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5, pb: 1.5 }}>
        <ProviderEndpointIcon endpoint={previewEndpoint} size={28} />
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h6" noWrap>{endpoint.name}</Typography>
          <EndpointIdChip id={endpoint.id} />
        </Box>
      </DialogTitle>

      <DialogContent dividers>
        <Stack spacing={3} sx={{ mt: 1 }}>
          {error && <Alert severity="error">{error}</Alert>}

          <FormSection title="Details">
            <Stack spacing={2}>
              <FormRow>
                <TextField
                  name="name"
                  label="Name"
                  value={formData.name}
                  onChange={handleTextFieldChange}
                  required
                  autoComplete="off"
                  placeholder="my-provider"
                  helperText="A unique name to identify this provider endpoint"
                  sx={{ gridColumn: { sm: account.admin ? 'auto' : '1 / -1' } }}
                />

                {account.admin && (
                  <TextField
                    select
                    name="icon"
                    label="Provider icon"
                    value={formData.icon}
                    onChange={handleTextFieldChange}
                    helperText="Defaults to a mark inferred from the name and base URL"
                    // The empty value means "Automatic", so the label has to stay
                    // shrunk or it would sit on top of the rendered value.
                    InputLabelProps={{ shrink: true }}
                    sx={theme => getFormSelectSx(theme.palette.mode === 'light')}
                    SelectProps={{
                      displayEmpty: true,
                      renderValue: (value: unknown) => {
                        const option = PROVIDER_ICON_OPTIONS.find(item => item.key === value)
                        return (
                          <Stack direction="row" spacing={1} alignItems="center">
                            {option
                              ? <ProviderMark provider={option.provider} size={22} />
                              : <ProviderEndpointIcon endpoint={previewEndpoint} size={22} />}
                            <span>{option?.label || 'Automatic'}</span>
                          </Stack>
                        )
                      },
                    }}
                  >
                    <MenuItem value="">
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Server size={22} />
                        <span>Automatic</span>
                      </Stack>
                    </MenuItem>
                    {PROVIDER_ICON_OPTIONS.map(option => (
                      <MenuItem key={option.key} value={option.key}>
                        <Stack direction="row" spacing={1} alignItems="center">
                          <ProviderMark provider={option.provider} size={22} />
                          <span>{option.label}</span>
                        </Stack>
                      </MenuItem>
                    ))}
                  </TextField>
                )}
              </FormRow>

              <TextField
                name="description"
                label="Description"
                value={formData.description}
                onChange={handleTextFieldChange}
                fullWidth
                multiline
                minRows={2}
                helperText="Explain what this endpoint serves and where it runs"
              />
            </Stack>
          </FormSection>

          <Divider />

          <FormSection title="Connection">
            <Stack spacing={2}>
              <TextField
                name="base_url"
                label="Base URL"
                value={formData.base_url}
                onChange={handleTextFieldChange}
                fullWidth
                required
                autoComplete="off"
                placeholder="https://api.openai.com/v1"
                helperText="OpenAI-compatible (https://api.openai.com/v1), Anthropic (https://api.anthropic.com/v1), or Google (https://generativelanguage.googleapis.com/v1beta/openai)"
              />

              <FormRow>
                <FormControl component="fieldset">
                  <FormLabel component="legend" sx={{ typography: 'body2', mb: 0.5 }}>
                    Authentication
                  </FormLabel>
                  <RadioGroup
                    row
                    name="auth_type"
                    value={formData.auth_type}
                    onChange={handleAuthTypeChange}
                  >
                    <FormControlLabel value="api_key" control={<Radio size="small" />} label="API key" />
                    <FormControlLabel value="api_key_file" control={<Radio size="small" />} label="Key file" />
                    <FormControlLabel value="none" control={<Radio size="small" />} label="None" />
                  </RadioGroup>
                </FormControl>

                {formData.auth_type === 'api_key' && (
                  <TextField
                    name="api_key"
                    label="API key"
                    value={formData.api_key}
                    onChange={handleTextFieldChange}
                    type="password"
                    autoComplete="off"
                    placeholder="••••••••"
                    helperText="Leave blank to keep the existing API key"
                  />
                )}

                {formData.auth_type === 'api_key_file' && (
                  <TextField
                    name="api_key_file"
                    label="API key file path"
                    value={formData.api_key_file}
                    onChange={handleTextFieldChange}
                    autoComplete="off"
                    placeholder="/etc/helix/provider.key"
                    helperText="Path to a file on the Helix server containing the API key"
                  />
                )}
              </FormRow>
            </Stack>
          </FormSection>

          <Divider />

          <FormSection title="Availability">
            <FormRow>
              <TextField
                select
                name="endpoint_type"
                label="Visibility"
                value={formData.endpoint_type}
                onChange={handleTextFieldChange}
                helperText="Who can select this endpoint"
                sx={theme => getFormSelectSx(theme.palette.mode === 'light')}
              >
                <MenuItem value="user">User (available to you only)</MenuItem>
                <MenuItem value="global">Global (available to all users in Helix installation)</MenuItem>
              </TextField>

              {account.admin && (
                <Box>
                  <FormControlLabel
                    control={(
                      <Switch
                        name="billing_enabled"
                        checked={formData.billing_enabled}
                        onChange={event => setFormData(previous => ({
                          ...previous,
                          billing_enabled: event.target.checked,
                        }))}
                      />
                    )}
                    label="Charge users for inference"
                  />
                  <Typography variant="body2" color="text.secondary">
                    Usage through this provider is billed to the user's wallet balance.
                  </Typography>
                </Box>
              )}
            </FormRow>
          </FormSection>

          <Divider />

          <CustomHeadersEditor headers={formData.headers} onChange={handleHeadersChange} />
        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="outlined"
          color="secondary"
          disabled={loading}
        >
          Save changes
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EditProviderEndpointDialog;
