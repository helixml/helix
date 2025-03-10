import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import WebAssetIcon from '@mui/icons-material/WebAsset'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import React, { FC, useState } from 'react'
import useLightTheme from '../../hooks/useLightTheme'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import { IApp } from '../../types'

const AppPicker: FC<{
  apps: IApp[],
  selectedApp?: IApp,
  onSelectApp: (app: IApp | undefined) => void,
}> = ({
  apps,
  selectedApp,
  onSelectApp,
}) => {
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const [menuAnchorEl, setMenuAnchorEl] = useState<HTMLElement>()

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setMenuAnchorEl(undefined)
  }

  // Only show the picker if there are apps
  if (apps.length === 0) return null

  return (
    <>
      {isBigScreen ? (
        <Typography
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
          {selectedApp?.config.helix.name || 'Default App'} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
        </Typography>
      ) : (
        <IconButton
          onClick={handleOpenMenu}
          sx={{
            color: 'text.primary',
          }}
        >
          <WebAssetIcon />
        </IconButton>
      )}
      <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
        <Menu
          anchorEl={menuAnchorEl}
          open={Boolean(menuAnchorEl)}
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
          <MenuItem
            key="default"
            sx={{fontSize: "large"}}
            onClick={() => {
              onSelectApp(undefined)
              handleCloseMenu()
            }}
          >
            Default App
          </MenuItem>
          {apps.map(app => (
            <MenuItem
              key={app.id}
              sx={{fontSize: "large"}}
              onClick={() => {
                onSelectApp(app)
                handleCloseMenu()
              }}
            >
              {app.config.helix.name} {app.config.helix.description && <>&nbsp; <small>({app.config.helix.description})</small></>}
            </MenuItem>
          ))}
        </Menu>
      </Box>
    </>
  )
}

export default AppPicker 