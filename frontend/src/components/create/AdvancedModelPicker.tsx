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
  Button,
  ButtonProps,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import { useListProviders } from '../../services/providersService';
import { TypesProviderEndpoint, TypesOpenAIModel } from '../../api/api';

interface AdvancedModelPickerProps {
  selectedModelId?: string;
  onSelectModel: (model: string) => void;
  buttonProps?: ButtonProps;
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

function fuzzySearch(query: string, models: ModelWithProvider[]) {
  return models.filter((model) => {
    if (model.type !== 'chat') {
      return false;
    }
    return model.id?.toLowerCase().includes(query.toLowerCase()) || model.provider.toLowerCase().includes(query.toLowerCase());
  });
}

export const AdvancedModelPicker: React.FC<AdvancedModelPickerProps> = ({
  selectedModelId,
  onSelectModel,
  buttonProps,
}) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  
  // Fetch providers and models
  const { data: providers, isLoading: isLoadingProviders } = useListProviders(true);

  // Combine models from all providers
  const allModels: ModelWithProvider[] = useMemo(() => {
    return providers?.flatMap((provider) => 
      (provider.available_models || []).map((model): ModelWithProvider => ({
        ...model,
        provider: provider.name || '', 
      }))
    ) ?? [];
  }, [providers]);

  // Find the full name/ID of the selected model, default if not found or not selected
  const displayModelName = useMemo(() => {
    if (!selectedModelId) return "Select Model";
    const selectedModel = allModels.find(model => model.id === selectedModelId);
    let friendlyName = selectedModel?.id || selectedModelId;
    return friendlyName || "Select Model";
  }, [selectedModelId, allModels]);

  // Filter models based on search query
  const filteredModels = useMemo(() => {
    return fuzzySearch(searchQuery, allModels);
  }, [searchQuery, allModels]);

  const handleOpenDialog = () => {
    setSearchQuery('');
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
  };

  const handleSelectModel = (modelId: string) => {
    onSelectModel(modelId);
    handleCloseDialog();
  };

  const isLoading = isLoadingProviders;

  return (
    <>
      <Button
        variant="outlined"
        onClick={handleOpenDialog}
        endIcon={<ArrowDropDownIcon />}
        sx={{
          borderColor: 'rgba(255, 255, 255, 0.7)',
          color: 'rgba(255, 255, 255, 0.9)',
          textTransform: 'none',
          fontSize: '0.875rem',
          padding: '4px 8px',
          height: '32px',
          minWidth: 'auto',
          '&:hover': {
            borderColor: 'rgba(255, 255, 255, 0.9)',
            backgroundColor: 'rgba(255, 255, 255, 0.1)',
          },
          ...buttonProps?.sx,
        }}
        {...buttonProps}
      >
        <Typography 
          variant="caption" 
          component="span"
          sx={{ 
            maxWidth: '150px',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            display: 'inline-block',
            lineHeight: 1.2,
            verticalAlign: 'middle',
          }}
        >
          {displayModelName}
        </Typography>
      </Button>

      <Dialog 
        open={dialogOpen} 
        onClose={handleCloseDialog}
        maxWidth="sm"
        fullWidth
        PaperProps={{
          sx: {
            height: '60vh',
            maxHeight: 600,
            bgcolor: 'background.paper',
          }
        }}
      >
        <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6" component="div">Select Model</Typography>
          <IconButton
            aria-label="close"
            onClick={handleCloseDialog}
            sx={{ color: (theme) => theme.palette.grey[500] }}
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>

        <DialogContent dividers sx={{ p: 2 }}>
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
            maxHeight: 'calc(60vh - 64px - 56px - 32px - 1px)',
            '&::-webkit-scrollbar': {
              width: '8px',
            },
            '&::-webkit-scrollbar-track': {
              background: (theme) => theme.palette.mode === 'dark' ? '#2b2b2b' : '#f1f1f1',
            },
            '&::-webkit-scrollbar-thumb': {
              background: (theme) => theme.palette.mode === 'dark' ? '#555' : '#888',
              borderRadius: '4px',
            },
            '&::-webkit-scrollbar-thumb:hover': {
              background: (theme) => theme.palette.mode === 'dark' ? '#777' : '#555',
            },
            paddingRight: '8px',
          }}>
            {isLoading && filteredModels.length === 0 && (
               <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
                 <CircularProgress />
               </Box>
            )}
            {!isLoading && filteredModels.map((model) => (
              <ListItem
                key={`${model.provider}-${model.id}`}
                button
                onClick={() => model.id && handleSelectModel(model.id)}
                selected={model.id === selectedModelId}
                sx={{
                  '&:hover': {
                    backgroundColor: 'action.hover',
                  },
                  borderRadius: 1,
                  mb: 0.5,
                  ...(model.id === selectedModelId && {
                    backgroundColor: 'action.selected',
                  }),
                }}
              >
                <ListItemIcon sx={{ minWidth: 40 }}>
                  <ProviderIcon providerName={model.provider} />
                </ListItemIcon>
                <ListItemText
                  primary={model.id || 'Unnamed Model'}
                  secondary={model.provider}
                  primaryTypographyProps={{
                    variant: 'body1',
                    sx: { 
                      fontWeight: model.id === selectedModelId ? 500 : 400,
                      overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' 
                    } 
                  }}
                  secondaryTypographyProps={{
                    variant: 'body2',
                    sx: { color: 'text.secondary' }
                  }}
                />
              </ListItem>
            ))}
            {!isLoading && filteredModels.length === 0 && searchQuery && (
              <Box sx={{ p: 2, textAlign: 'center' }}>
                <Typography color="text.secondary">
                  No models found matching "{searchQuery}"
                </Typography>
              </Box>
            )}
             {!isLoading && filteredModels.length === 0 && !searchQuery && (
              <Box sx={{ p: 2, textAlign: 'center' }}>
                <Typography color="text.secondary">
                  No chat models available or still loading.
                </Typography>
              </Box>
            )}
          </List>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default AdvancedModelPicker;
