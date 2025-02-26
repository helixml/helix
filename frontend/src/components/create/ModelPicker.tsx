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
}> = ({
  type,
  model,
  provider,
  onSetModel,
  displayMode = 'full',
  border = false,
  compact = false
}) => {
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()
  const { models, fetchModels } = useContext(AccountContext)
  const loadedProviderRef = useRef<string | undefined>()

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

  useEffect(() => {
    if (loadedProviderRef.current !== provider) {
      console.log('fetching models for provider', provider)
      loadedProviderRef.current = provider
      fetchModels(provider)      
    }
  }, [provider, fetchModels])

  // Handle type changes with client-side filtering only
  useEffect(() => {
    const currentModels = models.filter(m => 
      m.type === type || (type === "text" && m.type === "chat")
    )
    
    // Reset selected model if current selection isn't valid for new type
    if (currentModels.length > 0 && !currentModels.find(m => m.id === model)) {
      onSetModel(currentModels[0].id)
    }
  }, [type, model, models, onSetModel])

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setModelMenuAnchorEl(undefined)
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
                onClick={() => {
                  onSetModel(menuModel.id)
                  handleCloseMenu()
                }}
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