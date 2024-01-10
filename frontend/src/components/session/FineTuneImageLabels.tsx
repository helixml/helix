import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'

import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import FineTuneImageLabel from './FineTuneImageLabel'
import InteractionContainer from './InteractionContainer'

export const FineTuneImageLabels: FC<{
  showImageLabelErrors: boolean,
  initialLabels?: Record<string, string>,
  files: File[],
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
  showButton = false,
  showSystemInteraction = true,
  onChange,
  onDone,
}) => {
  const [labels, setLabels] = useState<Record<string, string>>(initialLabels || {})

  return (
    <Box
      sx={{
        mt: 2,
      }}
    >
      {
        showSystemInteraction && (
          <Box
            sx={{
              mt: 4,
              mb: 4,
            }}
          >
            <InteractionContainer
              name="System"
            >
              <Typography className="interactionMessage">
                Now, add a label to each of your images.  Try to add as much detail as possible to each image:
              </Typography>
            </InteractionContainer>
          </Box>
        )
      }

      <Grid container spacing={3} direction="row" justifyContent="flex-start">
        {
          files.length > 0 && files.map((file) => {
            const objectURL = URL.createObjectURL(file)
            return (
              <Grid item xs={4} md={4} key={file.name}>
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#999'
                  }}
                >
                  <Box
                    component="img"
                    src={objectURL}
                    alt={file.name}
                    sx={{
                      height: '100px',
                      border: '1px solid #000000',
                      filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                      mb: 1,
                    }}
                  />
                  <FineTuneImageLabel
                    value={ labels[file.name] || '' }
                    filename={ file.name }
                    error={ showImageLabelErrors && !labels[file.name] }
                    onChange={ (value) => {
                      const newLabels = {...labels}
                      newLabels[file.name] = value
                      setLabels(newLabels)
                      if(onChange) {
                        onChange(newLabels)
                      }
                    }}
                  />
                </Box>
              </Grid>
            )
          })
            
        }
      </Grid>
      {
        files.length > 0 && showButton && onDone && (
          <Button
            sx={{
              width: '100%',
              mt: 4,
            }}
            variant="contained"
            color="secondary"
            endIcon={<ArrowCircleRightIcon />}
            onClick={ onDone }
          >
            Start Training
          </Button>
        )
      }
    </Box>
  )   
}

export default FineTuneImageLabels