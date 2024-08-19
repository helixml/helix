import React, { FC, useState, useEffect, useContext } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import useLightTheme from '../../hooks/useLightTheme'
import { AccountContext } from '../../contexts/account'

const ModelPicker: FC<{
  model: string,
  onSetModel: (model: string) => void,
}> = ({
  model,
  onSetModel
}) => {
  const lightTheme = useLightTheme()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()
  const { models } = useContext(AccountContext)

  useEffect(() => {
    // Set the first model as default if current model is not in the list
    if (models.length > 0 && (!model || !models.some(m => m.id === model))) {
      onSetModel(models[0].id);
    }
  }, [models, model, onSetModel])

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setModelMenuAnchorEl(undefined)
  }

  const modelData = models.find(m => m.id === model) || models[0];
  
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
          borderRadius: '15px',
          cursor: "pointer",
          "&:hover": {
            backgroundColor: lightTheme.isLight ? "#efefef" : "#13132b",
          },
        }}
      >
        {modelData?.name || 'Default Model'} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
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
            models.map(model => (
              <MenuItem
                key={ model.id }
                sx={{fontSize: "large"}}
                onClick={() => {
                  onSetModel(model.id)
                  handleCloseMenu()
                }}
              >
                { model.name } {model.description && <>&nbsp; <small>({model.description})</small></>}
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ModelPicker