import React, { useState, useMemo, useEffect } from 'react';
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
  Tooltip,
  Chip,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import MemoryIcon from '@mui/icons-material/Memory';
import { useListProviders } from '../../services/providersService';
import { TypesOpenAIModel, TypesProviderEndpoint } from '../../api/api';
import openaiLogo from '../../../assets/img/openai-logo.png'
import togetheraiLogo from '../../../assets/img/together-logo.png'
import vllmLogo from '../../../assets/img/vllm-logo.png'

interface AdvancedModelPickerProps {
  selectedModelId?: string;
  selectedProvider?: string;
  onSelectModel: (provider: string, model: string) => void;
  buttonProps?: ButtonProps;
  currentType: string; // Model type (chat, image, etc)
  displayMode?: 'full' | 'short'; // Controls how the model name is displayed
}

const ProviderIcon: React.FC<{ provider: TypesProviderEndpoint }> = ({ provider }) => {
  if (provider.base_url?.includes('api.openai.com')) {
    return <Avatar src={openaiLogo} sx={{ width: 32, height: 32 }} variant="square" />;
  } else if (provider.base_url?.includes('api.together.xyz')) {
    return <Avatar src={togetheraiLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
  }

  // Check provider models, if it has more than 1 and "owned_by" = "vllm", then show vllm logo
  if (provider.available_models && provider.available_models.length > 0 && provider.available_models[0].owned_by === "vllm") {
    return <Avatar src={vllmLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
  }

  // Default robot head
  return (
    <Avatar sx={{ bgcolor: '#9E9E9E', width: 32, height: 32 }}>
      <SmartToyIcon />
    </Avatar>
  );  
};

interface ModelWithProvider extends TypesOpenAIModel {
  provider: TypesProviderEndpoint;
  provider_base_url: string;
}

function fuzzySearch(query: string, models: ModelWithProvider[], modelType: string) {
  return models.filter((model) => {
    // If provider is togetherai or openai, check model type
    if (model.provider?.name === "togetherai" || model.provider?.name === "openai") {
      if (model.type !== modelType) {
        return false;
      }
    }
    // Otherwise it can be a custom vllm/ollama which don't have model types at all
    return model.id?.toLowerCase().includes(query.toLowerCase()) || model.provider?.name?.toLowerCase().includes(query.toLowerCase());
  });
}

// Helper function to format context length
const formatContextLength = (length: number | undefined): string | null => {
  if (!length || length <= 0) return null;
  if (length >= 1000) {
    // Round up to the nearest thousand before dividing
    return `${Math.ceil(length / 1000)}k`; 
  }
  return length.toString();
};

const getShortModelName = (name: string, displayMode: 'full' | 'short'): string => {
  if (displayMode === 'full') return name;
  
  // Remove everything before the last '/' if it exists
  let shortName = name.split('/').pop() || name;
  
  // Remove 'Meta-' prefix (case insensitive)
  shortName = shortName.replace(/^Meta-/i, '');
  
  // Remove 'Instruct-' suffix (case insensitive)
  shortName = shortName.replace(/-Instruct-?$/i, '');
  
  return shortName;
}

export const AdvancedModelPicker: React.FC<AdvancedModelPickerProps> = ({
  selectedModelId,
  selectedProvider,
  onSelectModel,
  buttonProps,
  currentType,
  displayMode = 'full',
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
        provider: provider, 
        provider_base_url: provider.base_url || '',
      }))
    ) ?? [];
  }, [providers]);

  // Auto-select first available model when none is selected
  useEffect(() => {
    // For text type, we need to use chat models
    const effectiveType = currentType === "text" ? "chat" : currentType;
    
    // Select first model if none selected or if current model doesn't match the new type
    if (allModels.length > 0) {
      // If no model is selected, select the first one of the right type
      if (!selectedModelId) {
        const firstModel = allModels.find(model => model.type === effectiveType);
        if (firstModel && firstModel.id) {
          onSelectModel(firstModel.provider?.name || '', firstModel.id);
        }
      } 
      // If a model is selected, check if its type matches current type
      else {
        // console.log('selected model, finding it from our list', selectedProvider, selectedModelId)
        const currentModel = allModels.find(model => model.id === selectedModelId);

        // If current model doesn't match the expected type, select a new one
        if (currentModel && currentModel.type && currentModel.type !== effectiveType) {          
          // Try to find a model of the right type from the same provider first
          let newModel = allModels.find(model => 
            model.type === effectiveType && 
            model.provider?.name === selectedProvider
          );
          
          // If no model found from the same provider, fall back to any provider
          if (!newModel) {
            newModel = allModels.find(model => model.type === effectiveType);
          }
          
          if (newModel && newModel.id) {
            onSelectModel(newModel.provider?.name || '', newModel.id);
          }
        }
      }
    }
  }, [allModels, selectedModelId, currentType, onSelectModel]);
  

  // Find the full name/ID of the selected model, default if not found or not selected
  const displayModelName = useMemo(() => {
    if (!selectedModelId) return "Select Model";
    const selectedModel = allModels.find(model => model.id === selectedModelId);
    let friendlyName = selectedModel?.id || selectedModelId;
    return friendlyName || "Select Model";
  }, [selectedModelId, allModels]);
  
  // Filter models based on search query and current type
  const filteredModels = useMemo(() => {
    // For text type, we need to use chat models
    const effectiveType = currentType === "text" ? "chat" : currentType;
    return fuzzySearch(searchQuery, allModels, effectiveType);
  }, [searchQuery, allModels, currentType]);

  const handleOpenDialog = () => {
    setSearchQuery('');
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
  };

  const handleSelectModel = (provider: string, modelId: string) => {
    console.log("selecting model", provider, modelId);
    onSelectModel(provider, modelId);
    handleCloseDialog();
  };

  const isLoading = isLoadingProviders;

  return (
    <>
      <Tooltip title={displayModelName} placement="bottom-start">
        <Button
          variant="text"
          onClick={handleOpenDialog}
          endIcon={<ArrowDropDownIcon />}
          sx={{
            borderRadius: '8px',
            color: 'text.primary',
            textTransform: 'none',
            fontSize: '0.875rem',
            padding: '4px 8px',
            height: '32px',
            minWidth: 'auto',
            maxWidth: '200px',
            display: 'flex',
            alignItems: 'center',
            border: '1px solid #fff',
            '&:hover': {
              backgroundColor: (theme) => theme.palette.mode === 'light' ? "#efefef" : "#13132b",
            },
            ...buttonProps?.sx,
          }}
          {...buttonProps}
        >
          <Typography 
            variant="caption" 
            component="span"
            sx={{ 
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              display: 'inline-block',
              lineHeight: 1.2,
              verticalAlign: 'middle',
            }}
          >
            {getShortModelName(displayModelName, displayMode)}
          </Typography>
        </Button>
      </Tooltip>

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

        <DialogContent 
          dividers 
          sx={{ 
            p: 2, 
            overflow: 'hidden',
            display: 'flex', 
            flexDirection: 'column' 
          }}
        >
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
            flexGrow: 1,
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
            overscrollBehavior: 'contain',
            paddingBottom: '8px',
          }}>
            {isLoading && filteredModels.length === 0 && (
               <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
                 <CircularProgress />
               </Box>
            )}
            {!isLoading && filteredModels.map((model) => {
              const formattedContextLength = formatContextLength(model.context_length);
              return (
                <ListItem
                  key={`${model.provider.name}-${model.id}`}
                  button
                  onClick={() => model.id && handleSelectModel(model.provider?.name || '', model.id)}
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
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1, overflow: 'hidden' }}>
                    <ListItemIcon sx={{ minWidth: 40 }}>
                      <ProviderIcon provider={model.provider} />
                    </ListItemIcon>
                    <ListItemText
                      primary={model.id || 'Unnamed Model'}
                      secondary={model.provider.name}
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
                      sx={{ mr: 1 }}
                    />
                  </Box>
                  {formattedContextLength && (
                    <Tooltip title="Context Length">
                      <Chip 
                        icon={<MemoryIcon sx={{ color: 'success.main' }} />} 
                        label={formattedContextLength}
                        size="small"
                        variant="outlined"
                        sx={{
                          color: 'text.secondary',
                          borderColor: 'transparent',
                          backgroundColor: 'transparent',
                          '& .MuiChip-icon': {
                             color: 'success.main',
                             marginLeft: '4px',
                             marginRight: '-4px',
                          },
                          '& .MuiChip-label': {
                             paddingLeft: '4px',
                          }
                        }}
                       />
                    </Tooltip>
                  )}
                </ListItem>
              );
            })}
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
