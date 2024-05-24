import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Card from '@mui/material/Card'
import CardMedia from '@mui/material/CardMedia'
import IconButton from '@mui/material/IconButton'
import ZoomOutMapIcon from '@mui/icons-material/ZoomOutMap'
import CloseIcon from '@mui/icons-material/Close'

import LabelImagesTextField from './LabelImagesTextField'

import {
  IUploadFile,
} from '../../types'

export const LabelImagesForm: FC<{
  files: IUploadFile[],
  labels: Record<string, string>,
  showEmptyErrors?: boolean,
  onSetLabels: (labels: Record<string, string>) => void,
}> = ({
  files,
  labels,
  showEmptyErrors = false,
  onSetLabels,
}) => {
  const [ zoomedImage, setZoomedImage ] = useState<string>('')

  const updateLabel = (filename: string, label: string) => {
    onSetLabels({
      ...labels,
      [filename]: label
    })
  }

  return (
    <>
      <Box sx={{ flexGrow: 0, mt: 2 }}>
        <Typography className="interactionMessage" gutterBottom>
          Describe in as much detail as you can, what is present in each image. Try to describe:
        </Typography>
        <Box component="ul" sx={{ pl: 4, mt: 1 }}>
          <Typography component="li">The subject of what the photo is doing</Typography>
          <Typography component="li">What else is visible in the image</Typography>
          <Typography component="li">The attributes of the image itself</Typography>
        </Box>
      </Box>
      <Grid container spacing={2} sx={{ flexGrow: 1, overflow: 'auto' }}>
        {
          files.map((file, index) => {
            const objectURL = URL.createObjectURL(file.file)
            return (
              <Grid item xs={12} sm={6} md={4} lg={3} key={index}>
                <Card sx={{ position: 'relative', width: '100%', height: 250, backgroundColor: 'transparent', boxShadow: 'none', mt: 2 }}>
                  <CardMedia component="img" sx={{  height: '100%', width: '100%', objectFit: 'cover', }} image={objectURL} alt="" />
                  <IconButton
                    sx={{
                      position: 'absolute',
                      bottom: 8,
                      right: 8,
                      backgroundColor: 'transparent',
                    }}
                    tabIndex={-1}
                    onClick={ () => {
                      setZoomedImage(objectURL)
                    }}
                  >
                    <ZoomOutMapIcon />
                  </IconButton>
                </Card>
                <LabelImagesTextField
                  value={ labels[file.file.name] || '' }
                  error={ showEmptyErrors && !labels[file.file.name] }
                  onChange={(label) => updateLabel(file.file.name, label)}
                />
              </Grid>
            )
          })
        }
      </Grid>
      {
        zoomedImage && (
          <Box
            sx={{
              position: 'fixed',
              top: 0,
              left: 0,
              width: '100%',
              height: '100%',
              backgroundColor: 'rgba(0, 0, 0, 0.8)',
              zIndex: 1300,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
            onClick={() => setZoomedImage('')} // Close the zoomed image when the background is clicked
          >
            <img 
              src={zoomedImage} 
              alt="Zoomed" 
              onClick={(e) => e.stopPropagation()} // Prevent click from closing the image
              style={{ maxWidth: '70%', maxHeight: '70%' }} 
            />
            <IconButton
              sx={{
                position: 'fixed',
                top: 8,
                right: 8,
                color: '#fff'
              }}
              onClick={() => setZoomedImage('')} // Close button handler
              >
              <CloseIcon />
            </IconButton>
          </Box>
        )
      }
    </>
  )
}

export default LabelImagesForm