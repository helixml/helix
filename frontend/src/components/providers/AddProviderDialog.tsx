import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  TextField,
  Alert,
  IconButton,
  InputAdornment,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import { useCreateProviderEndpoint, useUpdateProviderEndpoint, useDeleteProviderEndpoint } from '../../services/providersService';
import { TypesProviderEndpointType } from '../../api/api';
import VisibilityIcon from '@mui/icons-material/Visibility';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';

import { TypesOwnerType } from '../../api/api';

import { TypesProviderEndpoint } from '../../api/api';
import useSnackbar from '../../hooks/useSnackbar';
interface AddProviderDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  orgId: string;
  provider: {
    id: string;
    name: string;
    description: string;
    base_url: string;
    configurable_base_url?: boolean;
    optional_api_key?: boolean; // If provider doesn't need an API key
    is_custom?: boolean; // If true, the user picks the endpoint name
    setup_instructions: string;
  };
  // Only set if we are editing an existing provider
  existingProvider?: TypesProviderEndpoint;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));

const DescriptionTypography = styled(Typography)(({ theme }) => ({
  fontSize: '1.1rem',
  color: '#A0AEC0',
  marginBottom: theme.spacing(3),
}));

const SectionCard = styled(Box)(({ theme }) => ({
  background: '#23262F',
  borderRadius: 12,
  padding: theme.spacing(3),
  marginBottom: theme.spacing(3),
  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
}));

const AddProviderDialog: React.FC<AddProviderDialogProps> = ({
  open,
  onClose,
  onClosed,
  provider,
  existingProvider,
  orgId,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [baseUrlError, setBaseUrlError] = useState<string | null>(null);

  const [apiKey, setApiKey] = useState('');
  const [baseUrl, setBaseUrl] = useState('');
  const [customName, setCustomName] = useState('');
  const [customNameError, setCustomNameError] = useState<string | null>(null);
  const [modelName, setModelName] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [isFieldFocused, setIsFieldFocused] = useState(false);
  const { mutateAsync: createProviderEndpoint, isPending: isCreating } = useCreateProviderEndpoint();
  const { mutateAsync: updateProviderEndpoint, isPending: isUpdating } = useUpdateProviderEndpoint();
  const { mutateAsync: deleteProviderEndpoint, isPending: isDeleting } = useDeleteProviderEndpoint();

  const { success: snackbarSuccess } = useSnackbar();

  const isEditing = !!existingProvider;
  const isPending = isCreating || isUpdating || isDeleting;

  useEffect(() => {
    setBaseUrl(existingProvider?.base_url ?? provider.base_url)
    setCustomName(existingProvider?.name ?? '')
    setCustomNameError(null)
    setModelName(existingProvider?.models?.[0] ?? '')
  }, [provider, existingProvider])

  // Generate masked API key for display
  const getMaskedApiKey = () => {
    if (!existingProvider?.api_key) return '';
    const key = existingProvider.api_key;
    if (key.length <= 8) return '•'.repeat(key.length);
    return key.substring(0, 4) + '•'.repeat(key.length - 8) + key.substring(key.length - 4);
  };

  // Get the display value for the text field
  const getDisplayValue = () => {
    if (isEditing && !isFieldFocused && !apiKey) {
      return getMaskedApiKey();
    }
    return apiKey;
  };

  const handleClose = () => {
    setApiKey('');
    setCustomName('');
    setCustomNameError(null);
    setBaseUrlError(null);
    setModelName('');
    setError(null);
    setIsFieldFocused(false);
    onClose();
  };

  const handleFieldFocus = () => {
    setIsFieldFocused(true);
    if (isEditing && !apiKey) {
      setApiKey('');
    }
  };

  const handleFieldBlur = () => {
    setIsFieldFocused(false);
  };

  const handleSubmit = async () => {
    try {
      setError(null);
      setBaseUrlError(null);
      setCustomNameError(null);

      // For editing, if no new API key is provided, use the existing one
      const apiKeyToUse = isEditing && !apiKey.trim() ? existingProvider?.api_key || '' : apiKey;

      if (!apiKeyToUse.trim() && !provider.optional_api_key) {
        setError('API key is required');
        return;
      }

      if (!baseUrl.trim()) {
        setBaseUrlError('Base URL is required');
        return;
      }

      // For brand-new custom providers the user picks the endpoint name.
      // Existing custom providers keep their name (edits to the name aren't exposed here).
      let endpointName = provider.id;
      if (provider.is_custom) {
        if (isEditing && existingProvider?.name) {
          endpointName = existingProvider.name;
        } else {
          const trimmed = customName.trim();
          if (!trimmed) {
            setCustomNameError('Name is required');
            return;
          }
          if (!/^[a-zA-Z0-9._\-/]+$/.test(trimmed)) {
            setCustomNameError('Use only letters, numbers, and . _ - / characters');
            return;
          }
          endpointName = trimmed;
        }
      }

      // Custom providers can pin a single preset model. When set, this is the
      // only model the endpoint exposes (no /v1/models lookup needed upstream).
      const trimmedModel = modelName.trim();
      const presetModels = provider.is_custom && trimmedModel ? [trimmedModel] : undefined;

      if (isEditing && existingProvider?.id) {
        // Update existing provider
        await updateProviderEndpoint({
          id: existingProvider.id,
          body: {
            base_url: baseUrl,
            api_key: apiKeyToUse,
            endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser,
            description: provider.description,
            ...(provider.is_custom ? { models: presetModels ?? [] } : {}),
          },
        });
        snackbarSuccess('Provider updated successfully');
      } else {
        // Create new provider
        await createProviderEndpoint({
          name: endpointName,
          base_url: baseUrl,
          api_key: apiKey,
          endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser,
          description: provider.description,
          // If we are in an org context, set the owner to the org
          owner: orgId || '',
          owner_type: orgId ? TypesOwnerType.OwnerTypeOrg : TypesOwnerType.OwnerTypeUser,
          ...(presetModels ? { models: presetModels } : {}),
        });
        snackbarSuccess('Provider connected successfully');
      }

      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : `Failed to ${isEditing ? 'update' : 'create'} provider`);
    }
  };

  const handleDisconnect = async () => {
    if (!existingProvider?.id) return;
    
    try {
      setError(null);
      await deleteProviderEndpoint(existingProvider.id);
      snackbarSuccess('Provider disconnected successfully');
      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disconnect provider');
    }
  };

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      TransitionProps={{
        onExited: () => {
          setApiKey('');
          setCustomName('');
          setCustomNameError(null);
          setBaseUrlError(null);
          setModelName('');
          setError(null);
          setIsFieldFocused(false);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            {isEditing ? `Update ${provider.name}` : provider.name}
          </NameTypography>
          <DescriptionTypography>
            {provider.description}
          </DescriptionTypography>

          <SectionCard>
            { provider.is_custom && !isEditing ? (
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, color: 'text.primary', fontWeight: 500 }}>
                Name
              </Typography>
              <TextField
                fullWidth
                value={customName}
                onChange={(e) => setCustomName(e.target.value)}
                placeholder="my-provider"
                type="text"
                autoComplete="off"
                error={!!customNameError}
                helperText={customNameError || 'A unique identifier for this provider (letters, numbers, . _ - /).'}
                sx={{ flex: 1 }}
              />
            </Box>
            ) : null}
            { provider.configurable_base_url ? ( <>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, color: 'text.primary', fontWeight: 500 }}>
                Base URL
              </Typography>
              <TextField
                fullWidth
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder="https://api.example.com/v1"
                type="text"
                autoComplete="base-url"
                error={!!baseUrlError}
                helperText={baseUrlError}
                sx={{ flex: 1 }}
              />
            </Box>
            </>) : (<></>)}
            { provider.is_custom ? (
            <Box sx={{ display: 'flex', alignItems: 'flex-start', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, mt: 1, color: 'text.primary', fontWeight: 500 }}>
                Model
              </Typography>
              <TextField
                fullWidth
                value={modelName}
                onChange={(e) => setModelName(e.target.value)}
                placeholder="hermes-agent"
                type="text"
                autoComplete="off"
                helperText="Optional. If your endpoint doesn't list models at /v1/models, set the model name here so it shows up in the picker and can be called via the API."
                sx={{ flex: 1 }}
              />
            </Box>
            ) : null}
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, color: 'text.primary', fontWeight: 500 }}>
                API Key
              </Typography>
              <TextField
                fullWidth
                value={getDisplayValue()}
                onChange={(e) => setApiKey(e.target.value)}
                type={showApiKey ? "text" : "password"}
                autoComplete="new-password"
                error={!!error}
                helperText={error}
                sx={{ flex: 1 }}
                onFocus={handleFieldFocus}
                onBlur={handleFieldBlur}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={() => setShowApiKey(!showApiKey)}
                        edge="end"
                        size="small"
                      >
                        {showApiKey ? <VisibilityOffIcon /> : <VisibilityIcon />}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
              {provider.setup_instructions.split(/(https?:\/\/[^\s]+)/).map((part, index) => {
                if (part.match(/^https?:\/\//)) {
                  return (
                    <a
                      key={index}
                      href={part}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: '#6366F1', textDecoration: 'none' }}
                    >
                      {part}
                    </a>
                  );
                }
                return part;
              })}
            </Typography>
          </SectionCard>
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F', flexDirection: 'column', alignItems: 'stretch' }}>
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button 
            onClick={handleClose} 
            size="small"
            variant="outlined"
            color="primary"
          >
            Cancel
          </Button>
          <Box sx={{ flex: 1 }} />
          {isEditing && (
            <Button
              onClick={handleDisconnect}
              size="small"
              variant="outlined"
              color="error"
              disabled={isPending}
              sx={{ mr: 1 }}
            >
              Disconnect
            </Button>
          )}
          <Button
            onClick={handleSubmit}
            size="small"
            variant="outlined"
            color="secondary"
            disabled={isPending || ((!apiKey.trim() && !provider.optional_api_key) && !isEditing)}
          >
            {isEditing ? 'Update' : 'Connect'}
          </Button>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddProviderDialog; 