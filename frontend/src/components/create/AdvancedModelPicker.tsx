import React, { useState, useMemo } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  Box,
  Typography,
  TextField,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  IconButton,
  Avatar,
  InputAdornment,
  CircularProgress,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import { useListProviders } from '../../services/providersService';
import { TypesProviderEndpoint, TypesOpenAIModel } from '../../api/api';

interface AdvancedModelPickerProps {
  open: boolean;
  onClose: () => void;
  onSelectModel: (model: string) => void;
}

const ITEMS_TO_SHOW = 50;

const ProviderIcon: React.FC<{ providerName: string }> = ({ providerName }) => {
  switch (providerName) {
    case "openai":
      return (
        <Avatar sx={{ bgcolor: '#74AA9C', width: 32, height: 32 }}>
          <SmartToyIcon />
        </Avatar>
      );
    case "togetherai":
      return (
        <Avatar sx={{ bgcolor: '#FF69B4', width: 32, height: 32 }}>
          <SmartToyIcon />
        </Avatar>
      );
    default:
      return (
        <Avatar sx={{ bgcolor: '#9E9E9E', width: 32, height: 32 }}>
          <SmartToyIcon />
        </Avatar>
      );
  }
};

interface ModelWithProvider extends TypesOpenAIModel {
  provider: string;
}

export const AdvancedModelPicker: React.FC<AdvancedModelPickerProps> = ({
  open,
  onClose,
  onSelectModel,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  
  // Fetch providers and models
  const { data: providers, isLoading: isLoadingProviders } = useListProviders(true);

  // Fix the creation of allModels
  const allModels: ModelWithProvider[] | undefined = providers?.flatMap((provider) => {
    return (provider.available_models || []).map((model): ModelWithProvider => ({
      ...model,
      provider: provider.name || '', // Correctly assign provider name from the outer scope
    }));
  });

  // Filter models based on search query
  let filteredModels = fuzzySearch(searchQuery, allModels || []);

  console.log("filteredModels", filteredModels);
  console.log("allModels", allModels);

  if (!filteredModels) {
    filteredModels = [];
  }

  const isLoading = isLoadingProviders;

  return (
    <Dialog 
      open={open} 
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{
        sx: {
          height: '60vh',
          maxHeight: 600,
        }
      }}
    >
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="caption">Select Model</Typography>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: (theme) => theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers>
        <TextField
          fullWidth
          placeholder="Search models..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          sx={{ mb: 2 }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon color="action" />
              </InputAdornment>
            ),
            endAdornment: isLoading ? (
              <InputAdornment position="end">
                <CircularProgress size={20} />
              </InputAdornment>
            ) : null,
          }}
        />

        <List sx={{ 
          overflow: 'auto',
          maxHeight: 'calc(60vh - 180px)',
          '&::-webkit-scrollbar': {
            width: '8px',
          },
          '&::-webkit-scrollbar-track': {
            background: '#f1f1f1',
          },
          '&::-webkit-scrollbar-thumb': {
            background: '#888',
            borderRadius: '4px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: '#555',
          },
        }}>
          {filteredModels.map((model) => (
            <ListItem
              key={`${model.provider}-${model.id}`}
              button
              onClick={() => model.id && onSelectModel(model.id)}
              sx={{
                '&:hover': {
                  backgroundColor: 'rgba(0, 0, 0, 0.04)',
                },
                borderRadius: 1,
                mb: 0.5,
              }}
            >
              <ListItemIcon>
                <ProviderIcon providerName={model.provider} />
              </ListItemIcon>
              <ListItemText
                primary={model.id || 'Unnamed Model'}
                secondary={model.provider}
                primaryTypographyProps={{
                  variant: 'subtitle1',
                  sx: { fontWeight: 500 }
                }}
                secondaryTypographyProps={{
                  variant: 'body2',
                  sx: { color: 'text.secondary' }
                }}
              />
            </ListItem>
          ))}
          {filteredModels.length === 0 && !isLoading && (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <Typography color="text.secondary">
                No models found matching your search
              </Typography>
            </Box>
          )}
        </List>
      </DialogContent>
    </Dialog>
  );
};

// Fuzzy search function that returns a list of models when the search query contains
// a substring of the model name or when it contains the provider name
function fuzzySearch(query: string, models: ModelWithProvider[]) {
  return models.filter((model) => {
    if (model.type !== 'chat') {
      return false;
    }

    return model.id?.toLowerCase().includes(query.toLowerCase()) || model.provider.toLowerCase().includes(query.toLowerCase());
  });
}

export default AdvancedModelPicker;
