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
  const [showApiKey, setShowApiKey] = useState(false);
  const [isFieldFocused, setIsFieldFocused] = useState(false);
  const { mutate: createProviderEndpoint, isPending: isCreating } = useCreateProviderEndpoint();
  const { mutate: updateProviderEndpoint, isPending: isUpdating } = useUpdateProviderEndpoint(existingProvider?.id || '');
  const { mutate: deleteProviderEndpoint, isPending: isDeleting } = useDeleteProviderEndpoint();

  const { success: snackbarSuccess } = useSnackbar();

  const isEditing = !!existingProvider;
  const isPending = isCreating || isUpdating || isDeleting;

  useEffect(() => {
    setBaseUrl(provider.base_url)
  }, [provider])

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

      if (isEditing && existingProvider) {
        // Update existing provider
        await updateProviderEndpoint({
          base_url: baseUrl,
          api_key: apiKeyToUse,
          endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser,
          description: provider.description,
        });
        snackbarSuccess('Provider updated successfully');
      } else {
        // Create new provider
        await createProviderEndpoint({
          name: provider.id,
          base_url: baseUrl,
          api_key: apiKey,
          endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser,
          description: provider.description,
          // If we are in an org context, set the owner to the org
          owner: orgId || '',
          owner_type: orgId ? TypesOwnerType.OwnerTypeOrg : TypesOwnerType.OwnerTypeUser,
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
            { provider.configurable_base_url ? ( <>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, color: 'text.primary', fontWeight: 500 }}>
                Base URL
              </Typography>
              <TextField
                fullWidth
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                type="test"
                autoComplete="base-url"
                error={!!baseUrlError}
                helperText={baseUrlError}
                sx={{ flex: 1 }}
              />
            </Box>
            </>) : (<></>)}
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