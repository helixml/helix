import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'

import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import FineTuneImageLabel from './FineTuneImageLabel'
import InteractionContainer from './InteractionContainer'
import { SESSION_MODE_INFERENCE } from '../../types'

export const FineTuneImageLabels: FC<{
  showImageLabelErrors: boolean,
  initialLabels?: Record<string, string>,
  files: File[],
  mode?: string, 
  showButton?: boolean,
  showSystemInteraction?: boolean,
  onChange?: {
    (labels: Record<string, string>): void
  },
  onDone?: {
    (): void
  },
}> = ({
  showImageLabelErrors,
  initialLabels,
  files,
  mode,
  showButton = false,
  showSystemInteraction = true,
  onChange,
  onDone,
}) => {
  const [labels, setLabels] = useState<Record<string, string>>(initialLabels || {})

  return (
    <Box sx={{ mt: 2 }}>
      {showSystemInteraction && (
        <Box sx={{ mt: 4, mb: 4 }}>
          <InteractionContainer name="System">
            <Box sx={{ mt: 2 }}>
              <Typography className="interactionMessage" gutterBottom>
                Describe in as much detail as you can, what is present in each image. Try to describe:
              </Typography>
              <Box component="ul" sx={{ pl: 4, mt: -1 }}>
                <Typography component="li">The subject of what the photo is doing</Typography>
                <Typography component="li">What else is visible in the image</Typography>
                <Typography component="li">The attributes of the image itself</Typography>
              </Box>
            </Box>
          </InteractionContainer>
        </Box>
      )}
  
      <Grid container spacing={2} gap={0} direction="row" justifyContent="flex-start">
        {files.length > 0 && files.map((file) => {
          const objectURL = URL.createObjectURL(file);
          return (
            <Grid item xs={4} md={4} key={file.name}>
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#999',
                  p: 0,
                  m: 0,
                }}
              >
                <Box
                  component="img"
                  src={objectURL}
                  alt={file.name}
                  sx={{
                    height: '98px',
                    filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                    p: 0,
                    mb: 1,
                  }}
                />
                <FineTuneImageLabel
                  value={labels[file.name] || ''}
                  filename={file.name}
                  error={showImageLabelErrors && !labels[file.name]}
                  onChange={(value) => {
                    const newLabels = { ...labels };
                    newLabels[file.name] = value;
                    setLabels(newLabels);
                    if (onChange) {
                      onChange(newLabels);
                    }
                  }}
                />
              </Box>
            </Grid>
          );
        })}
      </Grid>
  
      {/* "Return to upload images" button */}
      {files.length > 0 && showButton && (
      <Button
        component="button"
        onClick={() => {}}
        sx={{
          bgcolor: '#3BF959',
          color: 'black',
          position: 'fixed',
          bottom: 16,
          left: 16, 
          textDecoration: 'underline',
        }}
      >
        Return to upload images
      </Button>
    )}
  
      {/* "Start Training" button */}
      {files.length > 0 && showButton && onDone && (
        <Button
          sx={{
            bgcolor: '#3BF959',
            color: 'black',
            position: 'fixed',
            bottom: 16,
            right: 16, // Positioned on the right side
          }}
          variant="contained"
          onClick={onDone}
        >
          Start Training
        </Button>
      )}
    </Box>
  )
}

export default FineTuneImageLabels