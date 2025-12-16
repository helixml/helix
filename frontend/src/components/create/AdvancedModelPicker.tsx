import React, { useState, useMemo, useEffect, useRef } from 'react';
import {
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
  Switch,
  FormControlLabel,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import MemoryIcon from '@mui/icons-material/Memory';
import StarIcon from '@mui/icons-material/Star';
// import ImageIcon from '@mui/icons-material/Image';
import { Image, Cog } from 'lucide-react';

import { useListProviders } from '../../services/providersService';
import { useGetUserTokenUsage } from '../../services/userService';
import { TypesOpenAIModel, TypesProviderEndpoint, TypesModality } from '../../api/api';
import openaiLogo from '../../../assets/img/openai-logo.png'
import togetheraiLogo from '../../../assets/img/together-logo.png'
import vllmLogo from '../../../assets/img/vllm-logo.png'
import helixLogo from '../../../assets/img/logo.png'
import googleLogo from '../../../assets/img/providers/google.svg'
import anthropicLogo from '../../../assets/img/providers/anthropic.png'
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

import { useGetOrgByName } from '../../services/orgService';

import useRouter from '../../hooks/useRouter';

interface AdvancedModelPickerProps {
  selectedModelId?: string;
  selectedProvider?: string;
  onSelectModel: (provider: string, model: string) => void;
  buttonProps?: ButtonProps;
  currentType: string; // Model type (chat, image, etc)
  displayMode?: 'full' | 'short'; // Controls how the model name is displayed
  buttonVariant?: 'text' | 'outlined' | 'contained'; // New prop for button variant
  disabled?: boolean; // New prop to disable the picker
  hint?: string; // Optional hint text to display in the dialog
  recommendedModels?: string[]; // List of recommended model IDs to show at the top
  autoSelectFirst?: boolean; // Whether to auto-select first model when none is selected (default: true)
}

const ProviderIcon: React.FC<{ provider: TypesProviderEndpoint }> = ({ provider }) => {
  if (provider.base_url?.startsWith('https://api.openai.com/')) {
    return <Avatar src={openaiLogo} sx={{ width: 32, height: 32 }} variant="square" />;
  } else if (provider.base_url?.startsWith('https://api.together.xyz/')) {
    return <Avatar src={togetheraiLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
  }

  if (provider.base_url?.startsWith('https://generativelanguage.googleapis.com/')) {
    return <Avatar src={googleLogo} sx={{ width: 32, height: 32 }} variant="square" />;
  }

  if (provider.base_url?.startsWith('https://api.anthropic.com/')) {
    return <Avatar src={anthropicLogo} sx={{ width: 32, height: 32 }} variant="square" />;
  }

  // Check provider models, if it has more than 1 and "owned_by" = "vllm", then show vllm logo
  if (provider.available_models && provider.available_models.length > 0 && provider.available_models[0].owned_by === "vllm") {
    return <Avatar src={vllmLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
  }

  // If owned by helix, show helix logo
  if (provider.available_models && provider.available_models.length > 0 && provider.available_models[0].owned_by === "helix") {
    return <Avatar src={helixLogo} sx={{ width: 32, height: 32, bgcolor: '#fff' }} variant="square" />;
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
    // If provider is togetherai or openai or helix, check model type
    if (model.type && model.type !== "") {
      if (model.provider?.name === "togetherai" || model.provider?.name === "openai" || model.provider?.name === "helix") {
        if (model.type !== modelType) {
          return false;
        }
      }
    }

    // Otherwise it can be a custom vllm/ollama which don't have model types at all
    // Finally, filter by search query against model ID or provider name
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
  buttonVariant = 'outlined', // Default to outlined
  disabled = false, // Default to false
  hint,
  recommendedModels = [], // Default to empty array
  autoSelectFirst = true, // Default to true for backward compatibility
}) => {
  const router = useRouter()
  const lightTheme = useLightTheme();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [showOnlyEnabled, setShowOnlyEnabled] = useState(true);

  // Track the initially selected model when dialog opens (so it stays at top without jumping)
  const initialSelectedModelRef = useRef<string | undefined>(undefined);

  const orgName = router.params.org_id  

  // Get org if orgName is set  
  const { data: org, isLoading: isLoadingOrg } = useGetOrgByName(orgName, orgName !== undefined)  
  
  // Fetch providers and models
  const { data: providers, isLoading: isLoadingProviders } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !isLoadingOrg,
  });  

  const { data: tokenUsage, isLoading: isLoadingTokenUsage } = useGetUserTokenUsage();

  // Combine models from all providers
  const allModels: ModelWithProvider[] = useMemo(() => {
    return providers?.flatMap((provider: TypesProviderEndpoint) => 
      (provider.available_models || []).map((model: TypesOpenAIModel): ModelWithProvider => ({
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
      // If no model is selected, select the first one of the right type (only if autoSelectFirst is enabled)
      if (!selectedModelId && autoSelectFirst) {
        const firstModel = allModels.find(model => model.enabled && model.type === effectiveType);
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allModels, selectedModelId, currentType, autoSelectFirst]);
  

  // Find the full name/ID of the selected model, default if not found or not selected
  const displayModelName = useMemo(() => {
    if (!selectedModelId) return "Select Model";
    const selectedModel = allModels.find(model => model.id === selectedModelId);
    let friendlyName = selectedModel?.id || selectedModelId;
    return friendlyName || "Select Model";
  }, [selectedModelId, allModels]);
  
  // Determine tooltip title based on disabled state - include provider name
  const tooltipTitle = useMemo(() => {
    if (disabled) return "Model selection is disabled";
    const selectedModel = allModels.find(model => model.id === selectedModelId);
    const providerName = selectedModel?.provider?.name || selectedProvider;
    if (providerName) {
      return `${displayModelName} (${providerName})`;
    }
    return displayModelName;
  }, [disabled, displayModelName, allModels, selectedModelId, selectedProvider]);

  // Check if monthly token limit is reached
  const isMonthlyLimitReached = useMemo(() => {
    if (!tokenUsage) return false;

    // If quotas are not enabled, return false
    if (!tokenUsage.quotas_enabled) return false;

    // Otherwise check if usage percentage is >= 100
    return tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 100;
  }, [tokenUsage]);

  // Filter models based on search query and current type
  const filteredModels = useMemo(() => {
    // For text type, we need to use chat models
    const effectiveType = currentType === "text" ? "chat" : currentType;
    let models = fuzzySearch(searchQuery, allModels, effectiveType);

    if (showOnlyEnabled) {
      models = models.filter(model => model.enabled);
    }

    // Sort models: initially selected first, then recommended, then others
    models.sort((a, b) => {
      const initialModel = initialSelectedModelRef.current;

      // Initially selected model always comes first
      if (initialModel) {
        if (a.id === initialModel && b.id !== initialModel) return -1;
        if (b.id === initialModel && a.id !== initialModel) return 1;
      }

      // Then sort by recommended list
      if (recommendedModels.length > 0) {
        const aIndex = recommendedModels.indexOf(a.id || '');
        const bIndex = recommendedModels.indexOf(b.id || '');

        // If both are in recommended list, maintain their order in recommended list
        if (aIndex !== -1 && bIndex !== -1) {
          return aIndex - bIndex;
        }
        // If only one is in recommended list, put it first
        if (aIndex !== -1) return -1;
        if (bIndex !== -1) return 1;
      }

      // If neither is in recommended list, maintain original order
      return 0;
    });

    return models;
  }, [searchQuery, allModels, currentType, showOnlyEnabled, recommendedModels, dialogOpen]);

  const handleOpenDialog = () => {
    setSearchQuery('');
    // Capture the initially selected model so we can pin it to the top without jumping
    initialSelectedModelRef.current = selectedModelId;
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
      <Tooltip title={tooltipTitle} placement="top-start">
        {/* Wrap button in a span if disabled to allow tooltip to show */}
        <span style={{ display: 'inline-block', cursor: disabled ? 'not-allowed' : 'pointer' }}>
          <Button
            variant="text"
            onClick={handleOpenDialog}
            disabled={disabled} // Disable button if picker is disabled
            endIcon={<ArrowDropDownIcon />}
            sx={{
              borderRadius: '8px',
              color: '#F1F1F1',
              textTransform: 'none',
              fontSize: '0.875rem',
              padding: '4px 8px',
              height: '32px',
              minWidth: 'auto',
              maxWidth: '200px',
              display: 'flex',
              alignItems: 'center',
              border: buttonVariant === 'outlined' ? '1px solid #353945' : 'none',
              '&:hover': {
                backgroundColor: '#23262F',
                borderColor: '#6366F1',
              },
              // More explicit styling for disabled state if needed
              ...(disabled && {
                opacity: 0.5, // Example: reduce opacity when disabled
                pointerEvents: 'none', // Ensure no interaction
              }),
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
                fontSize: '0.875rem',
              }}
            >
              {getShortModelName(displayModelName, displayMode)}
            </Typography>
          </Button>
        </span>
      </Tooltip>

      <DarkDialog 
        open={dialogOpen} 
        onClose={handleCloseDialog}
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
          <Typography variant="h6" component="div" sx={{ color: '#F8FAFC' }}>Select Model</Typography>
          <IconButton
            aria-label="close"
            onClick={handleCloseDialog}
            sx={{ color: '#A0AEC0' }}
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
            flexDirection: 'column',
            ...lightTheme.scrollbar,
          }}
        >
          {hint && (
            <Typography 
              variant="body2" 
              sx={{ mb: 2, fontStyle: 'italic', color: '#A0AEC0' }}
            >
              {hint}
            </Typography>
          )}
          <TextField
            fullWidth
            placeholder="Search models..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            sx={{ mb: 1 }}
            autoFocus
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

          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1 }}>
            <Tooltip title="Show only enabled models" placement="left">
              <FormControlLabel
                control={
                  <Switch
                    checked={showOnlyEnabled}
                    onChange={(e) => setShowOnlyEnabled(e.target.checked)}
                    size="small"
                  />
                }
                label={<Typography variant="caption">Enabled models only</Typography>}
                sx={{ mr: 0 }}
              />
            </Tooltip>
          </Box>

          <List sx={{ 
            ...lightTheme.scrollbar,
            overflow: 'auto',
            flexGrow: 1,
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
              const isDisabled = !model.enabled; // Check if the model is disabled
              const isRecommended = recommendedModels.includes(model.id || '');
              
              // Check if this is a global provider and monthly limit is reached
              const isGlobalProvider = model.provider?.endpoint_type === 'global';
              const isGlobalProviderDisabled = isGlobalProvider && isMonthlyLimitReached;
              const isModelDisabled = Boolean(isDisabled || isGlobalProviderDisabled);

              const listItem = (
                <ListItem
                    key={`${model.provider.name}-${model.id}`}
                    onClick={() => !isModelDisabled && model.id && handleSelectModel(model.provider?.name || '', model.id)}
                    disabled={isModelDisabled}
                    sx={{
                      '&:hover': {
                        backgroundColor: isModelDisabled ? 'transparent' : '#23262F',
                      },
                      borderRadius: 1,
                      mb: 0.5,
                      ...(model.id === selectedModelId && !isModelDisabled && {
                        backgroundColor: '#353945',
                      }),
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      opacity: isModelDisabled ? 0.5 : 1,
                      cursor: isModelDisabled ? 'not-allowed' : 'pointer',
                    }}
                  >
                  <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1, overflow: 'hidden' }}>
                    <ListItemIcon sx={{ minWidth: 40 }}>
                      <ProviderIcon provider={model.provider} />
                    </ListItemIcon>
                    <ListItemText
                      primary={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Chip
                            label={model.provider.name}
                            size="small"
                            sx={{
                              backgroundColor: '#353945',
                              color: '#A0AEC0',
                              fontSize: '0.7rem',
                              height: '20px',
                              minWidth: '70px',
                              '& .MuiChip-label': {
                                px: 1,
                              },
                            }}
                          />
                          <Typography variant="body1" component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {model.id || 'Unnamed Model'}
                          </Typography>
                          {isRecommended && (
                            <Tooltip title="Recommended model">
                              <StarIcon
                                sx={{
                                  fontSize: '1rem',
                                  color: '#FFD700',
                                  ml: 0.5,
                                  verticalAlign: 'middle'
                                }}
                              />
                            </Tooltip>
                          )}
                        </Box>
                      }
                      secondary={
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25, ml: '78px' }}>
                          {model.description && (
                            <Typography
                              variant="caption"
                              component="span"
                              sx={{
                                color: isModelDisabled ? '#A0AEC0' : '#94A3B8',
                                fontSize: '0.75rem',
                                lineHeight: 1.2,
                              }}
                            >
                              {model.description}
                            </Typography>
                          )}
                          {model.provider.billing_enabled && model.model_info?.pricing && (
                            <Typography variant="body2" component="span" sx={{ color: '#A0AEC0', fontSize: '0.75rem' }}>
                              {model.model_info.pricing.prompt && `$${(parseFloat(model.model_info.pricing.prompt) * 1000000).toFixed(2)}/M input`}
                              {model.model_info.pricing.prompt && model.model_info.pricing.completion && ' | '}
                              {model.model_info.pricing.completion && `$${(parseFloat(model.model_info.pricing.completion) * 1000000).toFixed(2)}/M output`}
                            </Typography>
                          )}
                        </Box>
                      }
                      primaryTypographyProps={{
                        component: 'div',
                        sx: {
                          fontWeight: model.id === selectedModelId && !isModelDisabled ? 500 : 400,
                          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                          color: isModelDisabled ? '#A0AEC0' : '#F1F1F1',
                        }
                      }}
                      secondaryTypographyProps={{ component: 'div' }}
                      sx={{ mr: 1 }}
                    />
                  </Box>
                  {model.model_info?.input_modalities?.includes(TypesModality.ModalityImage) && (
                    <Tooltip title="This model supports vision">
                      <Chip
                        icon={<Image size={16} />}                        
                        size="small"
                        variant="outlined"
                        sx={{
                          color: '#A0AEC0',
                          borderColor: 'transparent',
                          backgroundColor: 'transparent',
                          mr: 1,
                          '& .MuiChip-icon': {
                             color: '#3B82F6',
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
                  {model.model_info?.supported_parameters?.includes("tools") && (
                    <Tooltip title="This model supports tool use">
                      <Chip
                        icon={<Cog size={16} />}                        
                        size="small"
                        variant="outlined"
                        sx={{
                          color: '#A0AEC0',
                          borderColor: 'transparent',
                          backgroundColor: 'transparent',
                          mr: 1,
                          '& .MuiChip-icon': {
                             color: '#8B5CF6',
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
                  {formattedContextLength && (
                    <Tooltip title="Context Length">
                      <Chip
                        icon={<MemoryIcon sx={{ color: 'success.main' }} />}
                        label={formattedContextLength}
                        size="small"
                        variant="outlined"
                        sx={{
                          color: '#A0AEC0',
                          borderColor: 'transparent',
                          backgroundColor: 'transparent',
                          '& .MuiChip-icon': {
                             color: '#10B981',
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

              // For disabled items, we'll modify the tooltip title to include the disabled reason
              if (isModelDisabled) {
                let disabledReason = '';
                if (isGlobalProviderDisabled) {
                  disabledReason = 'Monthly token limit reached. Upgrade your plan to increase your limit.';
                } else if (isDisabled) {
                  disabledReason = 'This model is not enabled for you';
                }
                
                // Update the tooltip to show the disabled reason instead of description
                const disabledListItem = (
                  <Tooltip 
                    title={disabledReason} 
                    placement="top" 
                    key={`${model.provider.name}-${model.id}-disabled-tooltip`}
                  >
                    <ListItem
                      key={`${model.provider.name}-${model.id}`}
                      onClick={() => !isModelDisabled && model.id && handleSelectModel(model.provider?.name || '', model.id)}
                      disabled={isModelDisabled}
                      sx={{
                        '&:hover': {
                          backgroundColor: isModelDisabled ? 'transparent' : '#23262F',
                        },
                        borderRadius: 1,
                        mb: 0.5,
                        ...(model.id === selectedModelId && !isModelDisabled ? {
                          backgroundColor: '#353945',
                        } : {}),
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        opacity: isModelDisabled ? 0.5 : 1,
                        cursor: isModelDisabled ? 'not-allowed' : 'pointer',
                      }}
                    >
                    <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1, overflow: 'hidden' }}>
                      <ListItemIcon sx={{ minWidth: 40 }}>
                        <ProviderIcon provider={model.provider} />
                      </ListItemIcon>
                      <ListItemText
                        primary={
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Chip
                              label={model.provider.name}
                              size="small"
                              sx={{
                                backgroundColor: '#353945',
                                color: '#A0AEC0',
                                fontSize: '0.7rem',
                                height: '20px',
                                minWidth: '70px',
                                '& .MuiChip-label': {
                                  px: 1,
                                },
                              }}
                            />
                            <Typography variant="body1" component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              {model.id || 'Unnamed Model'}
                            </Typography>
                            {isRecommended && (
                              <Tooltip title="Recommended model">
                                <StarIcon
                                  sx={{
                                    fontSize: '1rem',
                                    color: '#FFD700',
                                    ml: 0.5,
                                    verticalAlign: 'middle'
                                  }}
                                />
                              </Tooltip>
                            )}
                          </Box>
                        }
                        secondary={
                          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25, ml: '78px' }}>
                            {model.description && (
                              <Typography
                                variant="caption"
                                component="span"
                                sx={{
                                  color: isModelDisabled ? '#A0AEC0' : '#94A3B8',
                                  fontSize: '0.75rem',
                                  lineHeight: 1.2,
                                }}
                              >
                                {model.description}
                              </Typography>
                            )}
                          </Box>
                        }
                        primaryTypographyProps={{
                          component: 'div',
                          sx: {
                            fontWeight: model.id === selectedModelId && !isModelDisabled ? 500 : 400,
                            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                            color: isModelDisabled ? '#A0AEC0' : '#F1F1F1',
                          }
                        }}
                        secondaryTypographyProps={{ component: 'div' }}
                        sx={{ mr: 1 }}
                      />
                    </Box>
                    {model.model_info?.input_modalities?.includes(TypesModality.ModalityImage) && (
                      <Tooltip title="This model supports vision">
                        <Chip
                          icon={<Image size={16} />}                        
                          size="small"
                          variant="outlined"
                          sx={{
                            color: '#A0AEC0',
                            borderColor: 'transparent',
                            backgroundColor: 'transparent',
                            mr: 1,
                            '& .MuiChip-icon': {
                               color: '#3B82F6',
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
                    {model.model_info?.supported_parameters?.includes("tools") && (
                      <Tooltip title="This model supports tool use">
                        <Chip
                          icon={<Cog size={16} />}                        
                          size="small"
                          variant="outlined"
                          sx={{
                            color: '#A0AEC0',
                            borderColor: 'transparent',
                            backgroundColor: 'transparent',
                            mr: 1,
                            '& .MuiChip-icon': {
                               color: '#8B5CF6',
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
                    {formattedContextLength && (
                      <Tooltip title="Context Length">
                        <Chip
                          icon={<MemoryIcon sx={{ color: 'success.main' }} />}
                          label={formattedContextLength}
                          size="small"
                          variant="outlined"
                          sx={{
                            color: '#A0AEC0',
                            borderColor: 'transparent',
                            backgroundColor: 'transparent',
                            '& .MuiChip-icon': {
                               color: '#10B981',
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
                  </Tooltip>
                );
                return disabledListItem;
              }

              return listItem;
            })}
            {!isLoading && filteredModels.length === 0 && searchQuery && (
              <Box sx={{ p: 2, textAlign: 'center' }}>
                <Typography sx={{ color: '#A0AEC0' }}>
                  No models found matching "{searchQuery}"
                </Typography>
              </Box>
            )}
             {!isLoading && filteredModels.length === 0 && !searchQuery && (
              <Box sx={{ p: 2, textAlign: 'center' }}>
                <Typography sx={{ color: '#A0AEC0' }}>
                  No {showOnlyEnabled ? 'enabled ' : ''}chat models available or still loading.
                </Typography>
              </Box>
            )}
          </List>
        </DialogContent>
      </DarkDialog>
    </>
  );
};

export default AdvancedModelPicker;
