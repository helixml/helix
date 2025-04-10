import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import ExtensionIcon from '@mui/icons-material/Extension'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import React, { FC, useContext, useState, useMemo, useEffect, useRef } from 'react'
import { AccountContext } from '../../contexts/account'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'

const ModelPicker: FC<{
  type: string,
  model: string,
  provider: string | undefined, // Optional model when non-default provider is selected
  onSetModel: (model: string) => void,
  displayMode?: 'full' | 'short', // Controls how the model name is displayed
  border?: boolean, // Adds a border around the picker
  compact?: boolean, // Reduces the text size
  onLoadingStateChange?: (isLoading: boolean) => void, // Callback for loading state changes
  onProviderModelsLoaded?: (provider: string) => void, // Callback to notify when provider models have loaded
}> = ({
  type,
  model,
  provider,
  onSetModel,
  displayMode = 'full',
  border = false,
  compact = false,
  onLoadingStateChange,
  onProviderModelsLoaded,
}) => {
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()
  const { models, fetchModels } = useContext(AccountContext)
  const loadedProviderRef = useRef<string | undefined>()
  // Track if user has made a selection since provider changed
  const [userSelectedModel, setUserSelectedModel] = useState(false)
  // Track component initialization to handle initial state differently
  const initializedRef = useRef(false)
  // Track when models are being loaded to prevent race conditions
  const [isLoadingModels, setIsLoadingModels] = useState(false)

  const getShortModelName = (name: string): string => {
    if (displayMode === 'full') return name;
    
    // Remove everything before the last '/' if it exists
    let shortName = name.split('/').pop() || name;
    
    // Remove 'Meta-' prefix (case insensitive)
    shortName = shortName.replace(/^Meta-/i, '');
    
    // Remove 'Instruct-' suffix (case insensitive)
    shortName = shortName.replace(/-Instruct-?$/i, '');
    
    return shortName;
  }

  // Run once on initialization to properly handle pre-existing model/provider
  useEffect(() => {
    // If we have a model already set, always respect it as a user selection
    if (!initializedRef.current && model) {
      setUserSelectedModel(true)
      initializedRef.current = true
    }
  }, [model])

  // Track any explicit model setting as a user selection
  useEffect(() => {
    // Only consider it a user selection if:
    // 1. We're not loading models
    // 2. We have the correct provider's models loaded
    // 3. The model exists in our current model list
    if (!isLoadingModels && 
        loadedProviderRef.current === provider && 
        model && 
        models.some(m => m.id === model)) {
      setUserSelectedModel(true)
    }
  }, [model, models, isLoadingModels, provider])

  // Fetch models when provider changes
  useEffect(() => {
    if (loadedProviderRef.current !== provider) {
      console.log('fetching models for provider', provider)
      loadedProviderRef.current = provider
      
      // Mark that we're loading models
      setIsLoadingModels(true)
      onLoadingStateChange?.(true)
      
      fetchModels(provider).then(() => {
        console.log(`Models loaded for provider ${provider}`)
        // Only after models are loaded, mark as initialized and not loading
        initializedRef.current = true
        setIsLoadingModels(false)
        onLoadingStateChange?.(false)
        
        // Notify parent that this provider's models have loaded
        if (provider) {
          console.log(`Calling onProviderModelsLoaded for ${provider}`)
          onProviderModelsLoaded?.(provider)
        }
        
        // We never reset userSelectedModel to ensure we don't overwrite user choices
      }).catch(err => {
        // Log any errors but still consider initialized to prevent blocking UI
        console.error('Error loading models:', err)
        initializedRef.current = true
        setIsLoadingModels(false)
        onLoadingStateChange?.(false)
      })
    }
  }, [provider, fetchModels])

  // Handle type changes with client-side filtering only
  useEffect(() => {
    // Skip this effect if models are still loading to prevent race conditions
    // OR if we haven't loaded models for the current provider yet
    if (isLoadingModels || loadedProviderRef.current !== provider) {
      return
    }
    
    // Skip auto-selection if we already have a valid model and user hasn't changed type
    if (model && userSelectedModel) {
      return
    }

    const currentModels = models.filter(m => 
      m.type === type || (type === "text" && m.type === "chat")
    )
    
    // Always check if current model is compatible with the type
    const isCurrentModelValid = currentModels.some(m => m.id === model)
    
    // Only select a new model if:
    // 1. We have models to choose from AND
    // 2. Either we have no current model OR the current model is invalid for this type
    if (currentModels.length > 0 && (!model || !isCurrentModelValid)) {
      // Force model selection when type changes or model is invalid
      onSetModel(currentModels[0].id)
      // Consider this a deliberate selection to prevent further auto-switching
      setUserSelectedModel(true)
    }
  }, [type, model, models, onSetModel, userSelectedModel, isLoadingModels, provider])

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setModelMenuAnchorEl(undefined)
  }

  // Handle user selecting a model
  const handleModelSelect = (modelId: string) => {
    setUserSelectedModel(true)
    onSetModel(modelId)
    handleCloseMenu()
  }

  const modelData = models.find(m => m.id === model) || models[0];

  const filteredModels = useMemo(() => {
    return models.filter(m => m.type && m.type === type || (type === "text" && m.type === "chat"))
  }, [models, type])

  return (
    <>
      {isBigScreen ? (
        <Typography
          className="inferenceTitle"
          component="h1"
          variant={compact ? "body1" : "h6"}
          color="inherit"
          noWrap
          onClick={handleOpenMenu}
          sx={{
            flexGrow: 1,
            mx: 0,
            px: border ? 2 : 0,
            py: border ? 1 : 0,
            color: 'text.primary',
            borderRadius: '8px',
            cursor: "pointer",
            border: border ? (theme => `1px solid #fff`) : 'none',
            backgroundColor: border ? (theme => theme.palette.background.paper) : 'transparent',
            display: 'flex',
            alignItems: 'center',
            height: compact ? '32px' : 'auto',
            minHeight: compact ? '32px' : 'auto',
            "&:hover": {
              backgroundColor: lightTheme.isLight ? "#efefef" : "#13132b",
            },
          }}
        >
          {modelData?.name ? getShortModelName(modelData.name) : 'Default Model'}
          <KeyboardArrowDownIcon sx={{ ml: 0.5, ...(compact && { fontSize: '1.2rem' }) }} />
        </Typography>
      ) : (
        <IconButton
          onClick={handleOpenMenu}
          sx={{
            color: 'text.primary',
          }}
        >
          <ExtensionIcon />
        </IconButton>
      )}
      <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
        <Menu
          anchorEl={modelMenuAnchorEl}
          open={Boolean(modelMenuAnchorEl)}
          onClose={handleCloseMenu}
          sx={{marginTop: isBigScreen ? "50px" : "0px"}}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'left',
          }}
          transformOrigin={{
            vertical: 'center',
            horizontal: 'left',
          }}
        >
          {
            filteredModels.map(menuModel => (
              <MenuItem
                key={menuModel.id}
                selected={menuModel.id === model}
                sx={{
                  fontSize: "large",
                  '&.Mui-selected': {
                    backgroundColor: theme => lightTheme.isLight ? 'rgba(0, 0, 0, 0.08)' : 'rgba(255, 255, 255, 0.08)',
                    '&:hover': {
                      backgroundColor: theme => lightTheme.isLight ? 'rgba(0, 0, 0, 0.12)' : 'rgba(255, 255, 255, 0.12)',
                    }
                  }
                }}
                onClick={() => handleModelSelect(menuModel.id)}
              >
                {menuModel.name} {menuModel.description && <>&nbsp; <small>({menuModel.description})</small></>}
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ModelPicker