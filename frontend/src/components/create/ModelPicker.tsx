import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import useLightTheme from '../../hooks/useLightTheme'

interface IHelixModel {
  id: string;
  name: string;
  description: string;
  hide?: boolean;
}

const ModelPicker: FC<{
  model: string,
  onSetModel: (model: string) => void,
}> = ({
  model,
  onSetModel
}) => {
  const lightTheme = useLightTheme()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<HTMLElement>()
  const [models, setModels] = useState<IHelixModel[]>([])

  useEffect(() => {
    fetchModels()
  }, [])

  const fetchModels = async () => {
    try {
      const response = await fetch('/v1/models')
      const responseData = await response.json()
      
      let modelData: IHelixModel[] = [];
      if (responseData && Array.isArray(responseData.data)) {
        modelData = responseData.data.map((m: any) => ({
          id: m.id,
          name: m.name || m.id,
          description: m.description || '',
          hide: m.hide || false
        }));

        // Filter out hidden models
        modelData = modelData.filter(m => !m.hide);

        // If any model starts with 'gpt-', filter to keep only those models
        if (modelData.some(m => m.id.startsWith('gpt-'))) {
          modelData = modelData.filter(m => m.id.startsWith('gpt-'));
        }

        // Set the first model as default if current model is not in the list
        if (modelData.length > 0 && (!model || !modelData.some(m => m.id === model))) {
          onSetModel(modelData[0].id);
        }
      } else {
        console.error('Unexpected API response structure:', responseData)
      }

      setModels(modelData)
    } catch (error) {
      console.error('Error fetching models:', error)
      setModels([])
    }
  }

  const handleOpenMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  }

  const handleCloseMenu = () => {
    setModelMenuAnchorEl(undefined)
  }

  const modelData = models.find(m => m.id === model)
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
          borderRadius: '15px',
          cursor: "pointer",
          "&:hover": {
            backgroundColor: lightTheme.isLight ? "#efefef" : "#13132b",
          },
        }}
      >
        {modelData.name} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
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
                { model.name } &nbsp; <small>({ model.description })</small>
              </MenuItem>
            ))
          }
        </Menu>
      </Box>
    </>
  )
}

export default ModelPicker