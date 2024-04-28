import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'

import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';

import { HELIX_TEXT_MODELS } from '../../config'
import useLightTheme from '../../hooks/useLightTheme'

const ModelPicker: FC<{
  model: string,
  onSetModel: (model: string) => void,
}> = ({
  model,
  onSetModel
}) => {
  const lightTheme = useLightTheme()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => [
    setModelMenuAnchorEl(undefined)
  ]

  const modelData = HELIX_TEXT_MODELS.find(m => m.id === model)
  if(!modelData) return null

  return (
    <>
      <Typography
        className="inferenceTitle"
        component="h1"
        variant="h6"
        color="inherit"
        noWrap
        onClick={ handleOpenMenu }
        sx={{
          flexGrow: 1,
          mx: 0,
          color: 'text.primary',
          borderRadius: '15px', // Add rounded corners
          padding: "3px",
          cursor: "pointer",
          "&:hover": {
            backgroundColor: lightTheme.isLight ? "#efefef" : "#13132b",
          },
        }}
      >
        &nbsp;&nbsp;{modelData.title} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
      </Typography>
      <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
        <Menu
          anchorEl={modelMenuAnchorEl}
          open={Boolean(modelMenuAnchorEl)}
          onClose={handleCloseMenu}
          sx={{marginTop:"50px"}}
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
            HELIX_TEXT_MODELS.map(model => (
              <MenuItem
                key={ model.id }
                sx={{fontSize: "large"}}
                onClick={() => {
                  onSetModel(model.id)
                  handleCloseMenu()
                }}
              >
                { model.title } &nbsp; <small>({ model.description })</small>
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ModelPicker
