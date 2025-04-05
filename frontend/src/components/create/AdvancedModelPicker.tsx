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
  Tooltip,
  Chip,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import MemoryIcon from '@mui/icons-material/Memory';
import { useListProviders } from '../../services/providersService';
import { TypesProviderEndpoint, TypesOpenAIModel } from '../../api/api';
import openaiLogo from '../../../assets/img/openai-logo.png'
import togetheraiLogo from '../../../assets/img/together-logo.png'

interface AdvancedModelPickerProps {
  selectedModelId?: string;
  onSelectModel: (provider: string, model: string) => void;
  buttonProps?: ButtonProps;
  currentType: string; // Model type (chat, image, etc)
  displayMode?: 'full' | 'short'; // Controls how the model name is displayed
}

const ITEMS_TO_SHOW = 50;

const ProviderIcon: React.FC<{ providerBaseUrl: string }> = ({ providerBaseUrl }) => {
  if (providerBaseUrl.includes('api.openai.com')) {
    return <Avatar src={openaiLogo} sx={{ width: 32, height: 32 }} variant="square" />;
  } else if (providerBaseUrl.includes('api.together.xyz')) {
    return <Avatar src={togetheraiLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
  } else {
    return (
      <Avatar sx={{ bgcolor: '#9E9E9E', width: 32, height: 32 }}>
        <SmartToyIcon />
      </Avatar>
    );
  }
};

interface ModelWithProvider extends TypesOpenAIModel {
  provider: string;
  provider_base_url: string;
}

function fuzzySearch(query: string, models: ModelWithProvider[], modelType: string) {
  return models.filter((model) => {
    if (model.type !== modelType) {
      return false;
    }
    return model.id?.toLowerCase().includes(query.toLowerCase()) || model.provider.toLowerCase().includes(query.toLowerCase());
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
        provider: provider.name || '', 
        provider_base_url: provider.base_url || '',
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
                  key={`${model.provider}-${model.id}`}
                  button
                  onClick={() => model.id && handleSelectModel(model.provider, model.id)}
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
                      <ProviderIcon providerBaseUrl={model.provider_base_url} />
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
