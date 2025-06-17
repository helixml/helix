import React, { useState } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  TextField,
  Alert,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import { useCreateProviderEndpoint } from '../../services/providersService';
import { TypesProviderEndpointType } from '../../api/api';

interface AddProviderDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  provider: {
    id: string;
    name: string;
    description: string;
    base_url: string;
    setup_instructions: string;
  };
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
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [apiKey, setApiKey] = useState('');
  const { mutate: createProviderEndpoint, isPending } = useCreateProviderEndpoint();

  const handleClose = () => {
    setApiKey('');
    setError(null);
    onClose();
  };

  const handleSubmit = async () => {
    try {
      setError(null);
      
      if (!apiKey.trim()) {
        setError('API key is required');
        return;
      }

      await createProviderEndpoint({
        name: provider.id,
        base_url: provider.base_url,
        api_key: apiKey,
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser,
        description: provider.description,
      });

      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create provider');
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
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            {provider.name}
          </NameTypography>
          <DescriptionTypography>
            {provider.description}
          </DescriptionTypography>

          <SectionCard>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="body2" sx={{ minWidth: 80, mr: 2, color: 'text.primary', fontWeight: 500 }}>
                API Key
              </Typography>
              <TextField
                fullWidth
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                type="password"
                autoComplete="new-password"
                error={!!error}
                helperText={error}
                sx={{ flex: 1 }}
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
          <Button
            onClick={handleSubmit}
            size="small"
            variant="outlined"
            color="secondary"
            disabled={isPending || !apiKey.trim()}
          >
            Connect
          </Button>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddProviderDialog; 