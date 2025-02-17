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
}> = ({
  type,
  model,
  provider,
  onSetModel
}) => {
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()
  const { models, fetchModels } = useContext(AccountContext)
  const loadedProviderRef = useRef<string | undefined>()

  useEffect(() => {
    if (loadedProviderRef.current !== provider) {
      console.log('fetching models for provider', provider)
      loadedProviderRef.current = provider
      fetchModels(provider)      
    }
  }, [provider, fetchModels])

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
          variant="h6"
          color="inherit"
          noWrap
          onClick={handleOpenMenu}
          sx={{
            flexGrow: 1,
            mx: 0,
            color: 'text.primary',
            borderRadius: '15px',
            cursor: "pointer",
            "&:hover": {
              backgroundColor: lightTheme.isLight ? "#efefef" : "#13132b",
            },
          }}
        >
          {modelData?.name || 'Default Model'} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
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
            filteredModels.map(model => (
              <MenuItem
                key={model.id}
                sx={{fontSize: "large"}}
                onClick={() => {
                  onSetModel(model.id)
                  handleCloseMenu()
                }}
              >
                {model.name} {model.description && <>&nbsp; <small>({model.description})</small></>}
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ModelPicker