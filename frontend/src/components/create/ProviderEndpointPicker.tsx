import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import ExtensionIcon from '@mui/icons-material/Extension'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import React, { FC, useState } from 'react'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'

import {
  TypesProviderEndpoint,
  TypesProviderEndpointType,
  TypesOwnerType,
} from '../../api/api'

const ProviderEndpointPicker: FC<{  
  providerEndpoint: string | undefined,
  onSetProviderEndpoint: (providerEndpoint: string) => void,
  providerEndpoints: TypesProviderEndpoint[],
}> = ({  
  providerEndpoint,
  onSetProviderEndpoint,
  providerEndpoints,
}) => {
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setModelMenuAnchorEl(undefined)
  }  

  let providerData = providerEndpoints.find(p => p.name === providerEndpoint)

  // If not found, find the provider with "default" = true
  if (!providerData) {
    providerData = providerEndpoints.find(p => p.default)
  }

  // If it still not found, set it as "default"
  if (!providerData) {
    providerData = {
      created: '',
      updated: '',
      name: 'default',
      description: '',
      endpoint_type: ('global' as TypesProviderEndpointType),
      models: [],
      owner: '',
      owner_type: ('user' as TypesOwnerType),
      base_url: '',
      api_key: '',
      id: '',          
      default: true,      
    }
  }

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
          {providerData.name} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
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
            providerEndpoints && providerEndpoints.map(provider => (
              <MenuItem
                key={provider.name}
                sx={{fontSize: "large"}}
                onClick={() => {
                  onSetProviderEndpoint(provider.name || '')
                  handleCloseMenu()
                }}
                selected={provider.name === providerEndpoint}
              >
                {provider.name} {provider.description && <>&nbsp; <small>({provider.description})</small></>}
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ProviderEndpointPicker